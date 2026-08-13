package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeMCPConfigFile_PreservesExistingServers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	original := `{
  "mcpServers": {
    "other": {
      "command": "/usr/bin/other"
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	merged, backup, err := MergeMCPConfigFile(path, "gsc", "/Users/me/bin/gsc-mcp", false)
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("expected backup path")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatal(err)
	}
	servers := root["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Fatal("existing server 'other' was removed")
	}
	gsc := servers["gsc"].(map[string]any)
	if gsc["command"] != "/Users/me/bin/gsc-mcp" {
		t.Fatalf("gsc command = %v", gsc["command"])
	}
	if _, hasEnv := gsc["env"]; hasEnv {
		t.Fatal("ADC setup must not inject env block")
	}
}

func TestMergeMCPConfigFile_DryRunDoesNotWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)

	merged, backup, err := MergeMCPConfigFile(path, "gsc", "/bin/gsc-mcp", true)
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Fatal("dry-run must not create backup")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("dry-run modified file")
	}
	if !strings.Contains(string(merged), "gsc") {
		t.Fatalf("merged missing gsc: %s", merged)
	}
}

func TestMergeMCPConfigFile_BadJSONStops(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := MergeMCPConfigFile(path, "gsc", "/bin/gsc-mcp", false)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("got %v", err)
	}
	// Original must be intact
	raw, _ := os.ReadFile(path)
	if string(raw) != `{not json` {
		t.Fatal("bad JSON file was modified")
	}
}

func TestMergeMCPConfigFile_CreatesWhenMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "new", "mcp.json")
	_, backup, err := MergeMCPConfigFile(path, "gsc", "/abs/gsc-mcp", false)
	if err != nil {
		t.Fatal(err)
	}
	if backup != "" {
		t.Fatal("no backup for new file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "/abs/gsc-mcp") {
		t.Fatalf("got %s", raw)
	}
}

func TestRun_GcloudExistsButFails(t *testing.T) {

	dir := t.TempDir()
	// Fake gcloud that is in PATH but exits non-zero with a Python-related message.
	fakeGcloud := filepath.Join(dir, "gcloud")
	script := "#!/bin/sh\necho \"Python 3.12 required\" >&2\nexit 1\n"
	if err := os.WriteFile(fakeGcloud, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir+string(filepath.ListSeparator)+os.Getenv("PATH"))

	stderrPath := filepath.Join(dir, "stderr.txt")
	stderr, err := os.Create(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stderr.Close() })

	bin := filepath.Join(dir, "gsc-mcp")
	if err := os.WriteFile(bin, []byte("fake binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = Run(Options{
		DryRun:     true,
		BinaryPath: bin,
		Stderr:     stderr,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	out, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	output := string(out)
	if !strings.Contains(output, "found but failed to run") {
		t.Fatalf("expected diagnostic about gcloud failing, got:\n%s", output)
	}
	if !strings.Contains(output, "CLOUDSDK_PYTHON") {
		t.Fatalf("expected CLOUDSDK_PYTHON suggestion, got:\n%s", output)
	}
	if strings.Contains(output, "NOT FOUND") {
		t.Fatalf("should have found fake gcloud, got:\n%s", output)
	}
}
