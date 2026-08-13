package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeServiceAccountJSON() []byte {
	return []byte(`{
		"type": "service_account",
		"project_id": "fake-project",
		"private_key_id": "fake-key-id",
		"private_key": "-----BEGIN PRIVATE KEY-----\nMIIBVQIBADANBgkqhkiG9w0BAQEFAASCAT8wggE7AgEAAkEA\n-----END PRIVATE KEY-----\n",
		"client_email": "fake@fake-project.iam.gserviceaccount.com",
		"client_id": "123456789",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`)
}

func fakeADCJSON(quota string) []byte {
	if quota == "" {
		return []byte(`{"type":"authorized_user","client_id":"x","client_secret":"y","refresh_token":"z"}`)
	}
	return []byte(`{"type":"authorized_user","client_id":"x","client_secret":"y","refresh_token":"z","quota_project_id":"` + quota + `"}`)
}

func clearCredEnv(t *testing.T) {
	t.Helper()
	t.Setenv(envVarADC, "")
	t.Setenv(envVarFile, "")
	t.Setenv(envVarJSON, "")
	// Point ADC default path at a missing file so layer 6 does not pick up the
	// developer's real credentials during unit tests.
	missing := filepath.Join(t.TempDir(), "no-such-adc.json")
	orig := adcPathFn
	adcPathFn = func() string { return missing }
	t.Cleanup(func() { adcPathFn = orig })
}

func TestResolve_Priority_CLIFlag(t *testing.T) {
	clearCredEnv(t)
	dir := t.TempDir()
	flagFile := filepath.Join(dir, "flag.json")
	envFile := filepath.Join(dir, "env.json")

	if err := os.WriteFile(flagFile, fakeServiceAccountJSON(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envFile, []byte(`{"type":"service_account","client_email":"env@example.com"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(envVarFile, envFile)

	cfg, err := Resolve(flagFile)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if cfg.CredType != CredTypeServiceAccount {
		t.Fatalf("cred type = %q", cfg.CredType)
	}
	if !strings.Contains(string(cfg.CredentialsJSON), "fake@fake-project") {
		t.Fatalf("expected flag JSON, got %s", cfg.CredentialsJSON)
	}
}

func TestResolve_Priority_ApplicationCredentials(t *testing.T) {
	clearCredEnv(t)
	dir := t.TempDir()
	adcFile := filepath.Join(dir, "adc.json")
	saFile := filepath.Join(dir, "sa.json")
	if err := os.WriteFile(adcFile, fakeADCJSON("proj-1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(saFile, fakeServiceAccountJSON(), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(envVarADC, adcFile)
	t.Setenv(envVarFile, saFile)

	cfg, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if cfg.CredType != CredTypeAuthorizedUser {
		t.Fatalf("cred type = %q, want authorized_user", cfg.CredType)
	}
	if cfg.QuotaProjectID != "proj-1" {
		t.Fatalf("quota = %q", cfg.QuotaProjectID)
	}
	if cfg.Source != envVarADC {
		t.Fatalf("source = %q", cfg.Source)
	}
}

func TestResolve_Priority_EnvFileOverEnvJSON(t *testing.T) {
	clearCredEnv(t)
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.json")
	if err := os.WriteFile(envFile, fakeServiceAccountJSON(), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(envVarFile, envFile)
	t.Setenv(envVarJSON, string(fakeADCJSON("")))

	cfg, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if cfg.CredType != CredTypeServiceAccount {
		t.Fatalf("expected service_account from env file, got %q", cfg.CredType)
	}
}

func TestResolve_Priority_EnvJSONOverDotEnv(t *testing.T) {
	clearCredEnv(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("GOOGLE_SERVICE_ACCOUNT_JSON={\"type\":\"service_account\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	t.Setenv(envVarJSON, string(fakeADCJSON("q")))

	cfg, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if cfg.CredType != CredTypeAuthorizedUser {
		t.Fatalf("expected ADC from env JSON, got %q", cfg.CredType)
	}
}

func TestResolve_Priority_DotEnvFallback(t *testing.T) {
	clearCredEnv(t)
	dir := t.TempDir()
	compact := `{"type":"service_account","client_email":"dotenv@example.com"}`
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("GOOGLE_SERVICE_ACCOUNT_JSON="+compact+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	cfg, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if cfg.CredType != CredTypeServiceAccount {
		t.Fatalf("expected service_account from .env, got %q", cfg.CredType)
	}
}

func TestResolve_Priority_ADCDefaultPath(t *testing.T) {
	clearCredEnv(t)
	dir := t.TempDir()
	adcFile := filepath.Join(dir, "application_default_credentials.json")
	if err := os.WriteFile(adcFile, fakeADCJSON("quota-x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Override after clearCredEnv so layer 6 hits our file.
	adcPathFn = func() string { return adcFile }

	// Ensure no .env in cwd picks something up.
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	cfg, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if cfg.CredType != CredTypeAuthorizedUser {
		t.Fatalf("cred type = %q", cfg.CredType)
	}
	if cfg.QuotaProjectID != "quota-x" {
		t.Fatalf("quota = %q", cfg.QuotaProjectID)
	}
	if cfg.Source != "ADC default path" {
		t.Fatalf("source = %q", cfg.Source)
	}
}

func TestResolve_NoCredentials(t *testing.T) {
	clearCredEnv(t)
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve("")
	if err == nil {
		t.Fatal("expected error when no credentials found")
	}
	if !strings.Contains(err.Error(), "checked:") {
		t.Fatalf("error should list sources checked, got %v", err)
	}
}

func TestResolve_InvalidCLIFile(t *testing.T) {
	clearCredEnv(t)
	_, err := Resolve("/nonexistent/path/to/key.json")
	if err == nil {
		t.Fatal("expected error for invalid CLI file")
	}
}

func TestResolve_UnsupportedCredType(t *testing.T) {
	clearCredEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{"type":"external_account"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Resolve(path)
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	if !strings.Contains(err.Error(), "unsupported credential type") {
		t.Fatalf("got %v", err)
	}
}

func TestConfig_Scopes(t *testing.T) {
	readonly := Config{}
	if got := readonly.Scopes(); len(got) != 1 || got[0] != "https://www.googleapis.com/auth/webmasters.readonly" {
		t.Fatalf("unexpected readonly scopes: %v", got)
	}

	write := Config{EnableWrite: true}
	if got := write.Scopes(); len(got) != 1 || got[0] != "https://www.googleapis.com/auth/webmasters" {
		t.Fatalf("unexpected write scopes: %v", got)
	}
}

func TestResolve_Flags(t *testing.T) {
	clearCredEnv(t)
	dir := t.TempDir()
	flagFile := filepath.Join(dir, "key.json")
	if err := os.WriteFile(flagFile, fakeServiceAccountJSON(), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GSC_ENABLE_WRITE", "true")
	t.Setenv("GSC_ALLOW_DESTRUCTIVE", "true")
	t.Setenv("GSC_REQUEST_TIMEOUT", "45s")
	t.Setenv("GSC_LOG_LEVEL", "debug")

	cfg, err := Resolve(flagFile)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	if !cfg.EnableWrite {
		t.Error("expected EnableWrite=true")
	}
	if !cfg.AllowDestructive {
		t.Error("expected AllowDestructive=true")
	}
	if cfg.RequestTimeout != 45*time.Second {
		t.Fatalf("expected timeout 45s, got %v", cfg.RequestTimeout)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("expected debug level, got %v", cfg.LogLevel)
	}
}

func TestResolve_LogLevelFallback(t *testing.T) {
	clearCredEnv(t)
	dir := t.TempDir()
	flagFile := filepath.Join(dir, "key.json")
	if err := os.WriteFile(flagFile, fakeServiceAccountJSON(), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GSC_LOG_LEVEL", "nonsense")

	cfg, err := Resolve(flagFile)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("expected default info level, got %v", cfg.LogLevel)
	}
}

func TestParseBoolEnv_InvalidLogsWarning(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	t.Setenv("GSC_ENABLE_WRITE", "yes")
	got := parseBoolEnv("GSC_ENABLE_WRITE")
	if got {
		t.Fatal("expected invalid bool to be treated as false")
	}

	logs := buf.String()
	if !strings.Contains(logs, "invalid boolean env var") {
		t.Fatalf("expected warning for invalid boolean, got %q", logs)
	}
}

func TestIsADC(t *testing.T) {
	if (Config{CredType: CredTypeAuthorizedUser}).IsADC() != true {
		t.Fatal("expected IsADC true")
	}
	if (Config{CredType: CredTypeServiceAccount}).IsADC() {
		t.Fatal("expected IsADC false")
	}
}
