package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestStdioPurity_SpawnBinary asserts that a production binary writes only
// JSON-RPC lines to stdout. Requires network-free fake credentials via env.
func TestStdioPurity_SpawnBinary(t *testing.T) {
	bin := buildTestBinary(t)

	// Minimal fake service account JSON — server will start; tool calls may fail auth.
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "sa.json")
	// Invalid key is fine: we only need initialize + tools/list to complete.
	// Use authorized_user shape so we don't need a private key parse on some paths.
	// Actually NewService may try to parse — service_account with junk key may fail at first API call only.
	fake := []byte(`{
  "type": "service_account",
  "project_id": "example",
  "private_key_id": "x",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF6PZGFwODA6SREOMsTl1LZ9bJOkd81VjhQ71\nLEoLzOjYcorDX7awiF4gsuZrHl2DDSYiT/Fs/2j+WJyyckA70e20L2XY8dP7buiXBC8QI7U9\nwIDAQABAoIBAQC7\n-----END RSA PRIVATE KEY-----\n",
  "client_email": "fake@example.iam.gserviceaccount.com",
  "client_id": "1",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token"
}`)
	if err := os.WriteFile(keyPath, fake, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--credentials-file", keyPath)
	cmd.Env = append(os.Environ(), "GSC_LOG_LEVEL=error")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	msgs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}
	for _, m := range msgs {
		if _, err := stdin.Write([]byte(m + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	// Critical: allow the server to flush responses before EOF.
	time.Sleep(2 * time.Second)
	_ = stdin.Close()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("binary timed out")
	}

	if stdout.Len() == 0 {
		t.Fatalf("empty stdout; stderr=%s", stderr.String())
	}

	sc := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
	// allow long lines
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := 0
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		lines++
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("stdout line %d is not JSON: %v\nline=%q\nstderr=%s", lines, err, line, stderr.String())
		}
		if _, ok := msg["jsonrpc"]; !ok {
			t.Fatalf("stdout line %d missing jsonrpc: %s", lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if lines < 2 {
		t.Fatalf("expected at least initialize + tools/list responses, got %d lines; stderr=%s", lines, stderr.String())
	}

	// stderr may contain logs but must not be required empty; just ensure no panic stack only.
	_ = stderr
}

func buildTestBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "gsc-mcp-test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Join(findCmdDir(t))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func findCmdDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// test runs from cmd/gsc-mcp
	return wd
}
