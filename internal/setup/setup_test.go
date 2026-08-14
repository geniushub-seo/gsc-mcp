package setup

import (
	"encoding/json"
	"errors"
	"fmt"
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

func TestMergeMCPConfigFile_NullRootIsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	original := `null`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := MergeMCPConfigFile(path, "gsc", "/new/gsc-mcp", true)
	if err == nil {
		t.Fatal("expected error when the document root is JSON null")
	}
	if !strings.Contains(err.Error(), "not an object") {
		t.Fatalf("error = %v, want root not an object", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Fatalf("file must be unchanged on merge error; got %s", got)
	}

	_, _, err = MergeMCPConfigFile(path, "gsc", "/new/gsc-mcp", false)
	if err == nil {
		t.Fatal("expected error on non-dry-run null root")
	}
	got, _ = os.ReadFile(path)
	if string(got) != original {
		t.Fatalf("non-dry-run must not write a null root; got %s", got)
	}
}

func TestMergeMCPConfigFile_NonObjectMCPServersIsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	original := `{"mcpServers":["not","an","object"]}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := MergeMCPConfigFile(path, "gsc", "/new/gsc-mcp", false)
	if err == nil {
		t.Fatal("expected error when mcpServers is not an object")
	}
	if !strings.Contains(err.Error(), "mcpServers") || !strings.Contains(err.Error(), "not an object") {
		t.Fatalf("error = %v, want mcpServers not an object", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Fatalf("file must be unchanged on merge error; got %s", got)
	}
}

func TestMergeMCPConfigFile_NonObjectGSCEntryIsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	original := `{"mcpServers":{"gsc":"custom-wrapper"}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := MergeMCPConfigFile(path, "gsc", "/new/gsc-mcp", false)
	if err == nil {
		t.Fatal("expected error when gsc entry is not an object")
	}
	if !strings.Contains(err.Error(), "mcpServers.gsc") || !strings.Contains(err.Error(), "not an object") {
		t.Fatalf("error = %v, want mcpServers.gsc not an object", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Fatalf("file must be unchanged on merge error; got %s", got)
	}
}

func TestMergeMCPConfigFile_PreservesExistingGSCFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	original := `{
  "mcpServers": {
    "gsc": {
      "command": "/old/gsc-mcp",
      "args": ["--log-level", "debug"],
      "env": {
        "GOOGLE_SERVICE_ACCOUNT_FILE": "/secret/sa.json",
        "GSC_LOG_LEVEL": "debug"
      },
      "supports_parallel_tool_calls": false
    }
  }
}
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	merged, _, err := MergeMCPConfigFile(path, "gsc", "/new/gsc-mcp", false)
	if err != nil {
		t.Fatal(err)
	}

	var root map[string]any
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatal(err)
	}
	gsc := root["mcpServers"].(map[string]any)["gsc"].(map[string]any)
	if gsc["command"] != "/new/gsc-mcp" {
		t.Fatalf("command = %v, want /new/gsc-mcp", gsc["command"])
	}
	args, _ := gsc["args"].([]any)
	if len(args) != 2 || args[0] != "--log-level" || args[1] != "debug" {
		t.Fatalf("args were not preserved: %#v", gsc["args"])
	}
	env, _ := gsc["env"].(map[string]any)
	if env["GOOGLE_SERVICE_ACCOUNT_FILE"] != "/secret/sa.json" || env["GSC_LOG_LEVEL"] != "debug" {
		t.Fatalf("env was not preserved: %#v", gsc["env"])
	}
	if gsc["supports_parallel_tool_calls"] != false {
		t.Fatalf("client-specific field dropped: %#v", gsc)
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
	if !strings.Contains(output, "but failed to run") {
		t.Fatalf("expected diagnostic about gcloud failing, got:\n%s", output)
	}
	if !strings.Contains(output, "CLOUDSDK_PYTHON") {
		t.Fatalf("expected CLOUDSDK_PYTHON suggestion, got:\n%s", output)
	}
	if strings.Contains(output, "NOT FOUND") {
		t.Fatalf("should have found fake gcloud, got:\n%s", output)
	}
}

// runCapture runs setup with a temp binary path and returns everything logged
// plus the Run error. DryRun is forced on so no test can touch the
// developer's real MCP configs.
func runCapture(t *testing.T, opts Options) (string, error) {
	t.Helper()
	dir := t.TempDir()

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

	opts.DryRun = true
	opts.BinaryPath = bin
	opts.Stderr = stderr
	_, runErr := Run(opts)

	out, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), runErr
}

func TestGcloudCandidates_WindowsUsesAppDataAndCmdSuffix(t *testing.T) {
	// The POSIX-only candidate list never matched a Windows install, which is
	// what sent an agent on a recursive disk search for gcloud.cmd.
	orig := goos
	goos = "windows"
	t.Cleanup(func() { goos = orig })

	t.Setenv("LOCALAPPDATA", `C:\Users\Tester\AppData\Local`)
	t.Setenv("ProgramFiles", `C:\Program Files`)

	got := gcloudCandidates()
	if len(got) == 0 {
		t.Fatal("no Windows candidates returned")
	}
	for _, c := range got {
		if !strings.HasSuffix(c, "gcloud.cmd") {
			t.Errorf("Windows candidate must be the .cmd launcher, got %q", c)
		}
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{`C:\Users\Tester\AppData\Local`, `C:\Program Files`} {
		if !strings.Contains(joined, want) {
			t.Errorf("candidates missing %q; got:\n%s", want, joined)
		}
	}
}

func TestClaudeDesktopConfigPath_PerOS(t *testing.T) {
	orig := goos
	t.Cleanup(func() { goos = orig })

	t.Setenv("APPDATA", `C:\Users\Tester\AppData\Roaming`)

	goos = "windows"
	if got := claudeDesktopConfigPath(); !strings.Contains(got, "Claude") || !strings.Contains(got, "AppData") {
		t.Errorf("windows: want a path under AppData/Claude, got %q", got)
	}

	goos = "darwin"
	if got := claudeDesktopConfigPath(); !strings.Contains(got, "Application Support") {
		t.Errorf("darwin: want a path under Application Support, got %q", got)
	}

	goos = "linux"
	if got := claudeDesktopConfigPath(); got != "" {
		t.Errorf("linux: Claude Desktop does not ship there, want \"\", got %q", got)
	}
}

func TestExplainVerifyFailure_QuotaProjectIsNotAPermissionProblem(t *testing.T) {
	t.Parallel()
	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, strings.TrimSpace(strings.ToLower(fmt.Sprintf(format, args...))))
	}

	explainVerifyFailure(
		errors.New("googleapi: Error 403: Search Console API requires a quota project"),
		"", // gcloud unavailable: must still print the fix
		logf,
	)

	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "set-quota-project") {
		t.Errorf("missing the actual fix command; got:\n%s", out)
	}
	// The whole point of this branch: stop the caller from chasing GSC permissions.
	if !strings.Contains(out, "not a property") {
		t.Errorf("must say this is not a permission problem; got:\n%s", out)
	}
}

func TestExplainVerifyFailure_TokenRejectionSaysDoNotRetry(t *testing.T) {
	t.Parallel()
	var lines []string
	logf := func(format string, args ...any) {
		lines = append(lines, strings.ToLower(fmt.Sprintf(format, args...)))
	}

	explainVerifyFailure(errors.New("oauth2: cannot fetch token: 400 Bad Request"), "", logf)

	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "application-default login") {
		t.Errorf("must point at re-login; got:\n%s", out)
	}
	if !strings.Contains(out, "will not help") {
		t.Errorf("must say retrying is futile; got:\n%s", out)
	}
}

func TestLiveCheck_ChangesWhetherTheAPICallHappens(t *testing.T) {
	// Guards the flag itself: without this, LiveCheck could be parsed and
	// threaded through while never reaching the verify branch.
	// An invalid inline credential makes the call fail locally, so the
	// assertion never depends on network access or real credentials.
	isolateCredentialEnv(t)
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_JSON", `{"type":"service_account","project_id":"x"}`)

	const skipped = "skip live API call"

	off, err := runCapture(t, Options{LiveCheck: false})
	if err != nil {
		t.Fatalf("LiveCheck=false should not fail: %v", err)
	}
	if !strings.Contains(off, skipped) {
		t.Errorf("LiveCheck=false should skip the API call; got:\n%s", off)
	}

	on, err := runCapture(t, Options{LiveCheck: true})
	if err == nil {
		t.Fatal("LiveCheck=true with invalid credentials must return an error")
	}
	if strings.Contains(on, skipped) {
		t.Errorf("LiveCheck=true must not skip the API call; got:\n%s", on)
	}
	if !strings.Contains(on, "list_sites") {
		t.Errorf("LiveCheck=true should report a list_sites attempt; got:\n%s", on)
	}
}

func TestRun_LiveCheckFailureReturnsError(t *testing.T) {
	isolateCredentialEnv(t)
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_JSON", `{"type":"service_account","project_id":"x"}`)

	out, err := runCapture(t, Options{LiveCheck: true})
	if err == nil {
		t.Fatal("doctor-style live check must return an error when list_sites fails")
	}
	if !strings.Contains(err.Error(), "live verification failed") {
		t.Fatalf("error = %v, want live verification failed", err)
	}
	if strings.Contains(out, "\nDone.") || strings.HasSuffix(strings.TrimSpace(out), "Done.") {
		t.Fatalf("live-check failure must not print Done.; got:\n%s", out)
	}
	if !strings.Contains(out, "Failed:") {
		t.Fatalf("live-check failure must print Failed; got:\n%s", out)
	}
}

func TestRun_MergeFailureReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	out, err := runCapture(t, Options{LiveCheck: false})
	if err == nil {
		t.Fatal("setup must return an error when MCP config merge fails")
	}
	if !strings.Contains(err.Error(), "merge MCP config") {
		t.Fatalf("error = %v, want merge MCP config", err)
	}
	if strings.Contains(out, "\nDone.") || strings.HasSuffix(strings.TrimSpace(out), "Done.") {
		t.Fatalf("merge failure must not print Done.; got:\n%s", out)
	}
	if !strings.Contains(out, "Failed:") {
		t.Fatalf("merge failure must print Failed; got:\n%s", out)
	}
}

func TestRun_SetupSkipsWriteWhenLiveFails(t *testing.T) {
	isolateCredentialEnv(t)
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_JSON", `{"type":"service_account","project_id":"x"}`)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	original := `{"mcpServers":{"other":{"command":"keep-me"}}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	stderrPath := filepath.Join(dir, "stderr.txt")
	stderr, err := os.Create(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stderr.Close() })

	bin := filepath.Join(dir, "gsc-mcp")
	if err := os.WriteFile(bin, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = Run(Options{
		DryRun:     false,
		LiveCheck:  false,
		BinaryPath: bin,
		Stderr:     stderr,
	})
	if err == nil {
		t.Fatal("non-dry-run setup must return an error when live verification fails")
	}
	if !strings.Contains(err.Error(), "live verification failed") {
		t.Fatalf("error = %v, want live verification failed", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("live failure must not write MCP config; got %s", got)
	}
}

func TestRun_MissingDefaultADCIsNotFailure(t *testing.T) {
	isolateCredentialEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	out, err := runCapture(t, Options{LiveCheck: false})
	if err != nil {
		t.Fatalf("missing default ADC path must not fail dry-run setup: %v\n%s", err, out)
	}
}

func isolateCredentialEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_FILE", "")
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_JSON", "")
}

func TestRun_DetectsProjectMCPConfigInCwd(t *testing.T) {
	// Codex and project-scoped Claude Code read ./.mcp.json; setup used to
	// ignore it entirely, leaving the caller to hand-edit the file.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	out, err := runCapture(t, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out, "Project config found") {
		t.Errorf("existing .mcp.json should be detected; got:\n%s", out)
	}
}

func TestRun_DoesNotCreateProjectMCPConfigWhenAbsent(t *testing.T) {
	// Creating .mcp.json in whatever directory setup runs from would scatter
	// stray config files across the user's machine.
	dir := t.TempDir()
	t.Chdir(dir)

	out, err := runCapture(t, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out, "not in the current directory") {
		t.Errorf("absent .mcp.json should be reported as skipped; got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); !os.IsNotExist(err) {
		t.Error("setup must not create .mcp.json when none exists")
	}
}

func TestMergeMCPConfig_DryRunDoesNotEchoOtherServers(t *testing.T) {
	// Sibling entries routinely hold API keys in their env block, and this
	// output gets pasted into chat transcripts and bug reports.
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	existing := `{"mcpServers":{"other":{"command":"x","env":{"OTHER_API_KEY":"sk-secret-value"}}}}`
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	var lines []string
	logf := func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }

	if err := mergeMCPConfig(path, "/bin/gsc-mcp", true, logf); err != nil {
		t.Fatal(err)
	}

	out := strings.Join(lines, "\n")
	if strings.Contains(out, "sk-secret-value") || strings.Contains(out, "OTHER_API_KEY") {
		t.Errorf("dry-run leaked a sibling server's secret:\n%s", out)
	}
	if !strings.Contains(out, "gsc-mcp") {
		t.Errorf("dry-run should still show the entry it would add; got:\n%s", out)
	}
}
