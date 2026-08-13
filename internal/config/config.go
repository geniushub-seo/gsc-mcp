// Package config resolves Google credentials from multiple sources, plus
// runtime flags for write access, logging, and timeouts.
//
// Credential priority order (SPEC.md §4.2):
//  1. CLI --credentials-file (alias --service-account-file)
//  2. GOOGLE_APPLICATION_CREDENTIALS
//  3. GOOGLE_SERVICE_ACCOUNT_FILE
//  4. GOOGLE_SERVICE_ACCOUNT_JSON
//  5. .env file
//  6. ADC default path (platform-specific)
package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	envVarADC  = "GOOGLE_APPLICATION_CREDENTIALS"
	envVarFile = "GOOGLE_SERVICE_ACCOUNT_FILE"
	envVarJSON = "GOOGLE_SERVICE_ACCOUNT_JSON"
	dotEnvFile = ".env"
)

// CredType is the credential JSON "type" field.
type CredType string

// Supported credential JSON type values (SPEC.md §4.3).
const (
	CredTypeServiceAccount CredType = "service_account"
	CredTypeAuthorizedUser CredType = "authorized_user"
)

// Config holds resolved configuration values.
type Config struct {
	// CredentialsJSON is the raw credential JSON content.
	CredentialsJSON []byte

	// CredType is service_account or authorized_user.
	CredType CredType

	// QuotaProjectID is set for ADC credentials that include quota_project_id.
	QuotaProjectID string

	// Source describes where credentials were loaded from (for errors/logs).
	Source string

	// EnableWrite allows write actions (e.g. sitemap submit).
	EnableWrite bool

	// AllowDestructive allows destructive write actions (e.g. sitemap delete).
	AllowDestructive bool

	// RequestTimeout is the default timeout for upstream Google API calls.
	RequestTimeout time.Duration

	// LogLevel controls the verbosity of slog output.
	LogLevel slog.Level
}

// Resolve returns a Config populated from CLI flags, environment variables,
// .env, and the ADC default path. An explicit but invalid credential source
// returns an error so the caller can exit before entering the MCP loop.
func Resolve(credentialsFile string) (Config, error) {
	cfg := Config{
		EnableWrite:      parseBoolEnv("GSC_ENABLE_WRITE"),
		AllowDestructive: parseBoolEnv("GSC_ALLOW_DESTRUCTIVE"),
		RequestTimeout:   parseDurationEnv("GSC_REQUEST_TIMEOUT", 30*time.Second),
		LogLevel:         parseLevelEnv("GSC_LOG_LEVEL", slog.LevelInfo),
	}

	jsonBytes, source, err := resolveCredentialsJSON(credentialsFile)
	if err != nil {
		return Config{}, err
	}
	cfg.CredentialsJSON = jsonBytes
	cfg.Source = source

	credType, quotaProject, err := parseCredentialMetadata(jsonBytes)
	if err != nil {
		return Config{}, err
	}
	cfg.CredType = credType
	cfg.QuotaProjectID = quotaProject

	return cfg, nil
}

// IsADC reports whether credentials are authorized_user (Application Default Credentials).
func (c Config) IsADC() bool {
	return c.CredType == CredTypeAuthorizedUser
}

// Scopes returns OAuth scopes for service_account credentials.
// For ADC (authorized_user), scopes were fixed at gcloud login time and
// option.WithScopes cannot expand them — callers should not rely on this for ADC write.
func (c Config) Scopes() []string {
	if c.EnableWrite {
		return []string{"https://www.googleapis.com/auth/webmasters"}
	}
	return []string{"https://www.googleapis.com/auth/webmasters.readonly"}
}

func resolveCredentialsJSON(credentialsFile string) ([]byte, string, error) {
	tried := []string{
		"CLI --credentials-file / --service-account-file",
		envVarADC,
		envVarFile,
		envVarJSON,
		".env (" + envVarADC + " / " + envVarFile + " / " + envVarJSON + ")",
		"ADC default path (" + defaultADCPath() + ")",
	}

	if credentialsFile != "" {
		data, err := os.ReadFile(credentialsFile)
		if err != nil {
			return nil, "", fmt.Errorf("read credentials file from CLI flag: %w", err)
		}
		slog.Debug("credentials loaded from CLI flag", slog.String("path", credentialsFile))
		return data, "CLI flag", nil
	}

	if v := os.Getenv(envVarADC); v != "" {
		data, err := os.ReadFile(v)
		if err != nil {
			return nil, "", fmt.Errorf("read credentials file from %s: %w", envVarADC, err)
		}
		slog.Debug("credentials loaded from env var", slog.String("env", envVarADC))
		return data, envVarADC, nil
	}

	if v := os.Getenv(envVarFile); v != "" {
		data, err := os.ReadFile(v)
		if err != nil {
			return nil, "", fmt.Errorf("read credentials file from %s: %w", envVarFile, err)
		}
		slog.Debug("credentials loaded from env var", slog.String("env", envVarFile))
		return data, envVarFile, nil
	}

	if v := os.Getenv(envVarJSON); v != "" {
		slog.Debug("credentials loaded from env var", slog.String("env", envVarJSON))
		return []byte(v), envVarJSON, nil
	}

	if data, src := loadFromDotEnv(); len(data) > 0 {
		slog.Debug("credentials loaded from .env file", slog.String("key", src))
		return data, ".env (" + src + ")", nil
	}

	adcPath := defaultADCPath()
	if data, err := os.ReadFile(adcPath); err == nil && len(data) > 0 {
		slog.Debug("credentials loaded from ADC default path", slog.String("path", adcPath))
		return data, "ADC default path", nil
	}

	return nil, "", fmt.Errorf("no credentials found; checked: %s", strings.Join(tried, "; "))
}

// adcPathFn is the ADC default path resolver; tests may override it.
var adcPathFn = platformADCPath

// DefaultADCPath returns the platform-specific Application Default Credentials path.
func DefaultADCPath() string {
	return adcPathFn()
}

func defaultADCPath() string {
	return adcPathFn()
}

func platformADCPath() string {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return filepath.Join("AppData", "Roaming", "gcloud", "application_default_credentials.json")
		}
		return filepath.Join(appData, "gcloud", "application_default_credentials.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
}

func parseCredentialMetadata(data []byte) (CredType, string, error) {
	var meta struct {
		Type           string `json:"type"`
		QuotaProjectID string `json:"quota_project_id"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", "", fmt.Errorf("parse credential JSON: %w", err)
	}
	switch CredType(meta.Type) {
	case CredTypeServiceAccount, CredTypeAuthorizedUser:
		return CredType(meta.Type), meta.QuotaProjectID, nil
	case "":
		return "", "", fmt.Errorf("credential JSON missing \"type\" field; supported: %q, %q", CredTypeServiceAccount, CredTypeAuthorizedUser)
	default:
		return "", "", fmt.Errorf("unsupported credential type %q; supported: %q, %q", meta.Type, CredTypeServiceAccount, CredTypeAuthorizedUser)
	}
}

func loadFromDotEnv() ([]byte, string) {
	f, err := os.Open(dotEnvFile)
	if err != nil {
		return nil, ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	// Prefer higher-priority names if multiple appear.
	var (
		adcPath  string
		filePath string
		inline   string
	)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if after, ok := strings.CutPrefix(line, envVarADC+"="); ok {
			adcPath = strings.Trim(after, `"'`)
		}
		if after, ok := strings.CutPrefix(line, envVarFile+"="); ok {
			filePath = strings.Trim(after, `"'`)
		}
		if after, ok := strings.CutPrefix(line, envVarJSON+"="); ok {
			inline = strings.Trim(after, `"'`)
		}
	}
	if adcPath != "" {
		if data, err := os.ReadFile(adcPath); err == nil {
			return data, envVarADC
		}
	}
	if filePath != "" {
		if data, err := os.ReadFile(filePath); err == nil {
			return data, envVarFile
		}
	}
	if inline != "" {
		return []byte(inline), envVarJSON
	}
	return nil, ""
}

func parseBoolEnv(name string) bool {
	v := os.Getenv(name)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		slog.Warn("invalid boolean env var, treating as false", slog.String("env", name), slog.String("value", v), slog.Any("err", err))
		return false
	}
	return b
}

func parseDurationEnv(name string, fallback time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		slog.Warn("invalid duration env var, using default", slog.String("env", name), slog.String("value", v), slog.Any("err", err))
		return fallback
	}
	return d
}

func parseLevelEnv(name string, fallback slog.Level) slog.Level {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch v {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "":
		return fallback
	default:
		slog.Warn("unknown log level env var, using default", slog.String("env", name), slog.String("value", v))
		return fallback
	}
}
