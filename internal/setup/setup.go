// Package setup implements the `gsc-mcp setup` subcommand: check gcloud/ADC,
// merge MCP client config, and optionally verify list_sites.
package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/geniushub-seo/gsc-mcp/internal/config"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
)

// Options controls setup behaviour.
type Options struct {
	DryRun     bool
	BinaryPath string // absolute path to gsc-mcp binary
	Stdout     *os.File
	Stderr     *os.File
}

// Result is a human-readable report of what setup did or would do.
type Result struct {
	Lines []string
}

func (r *Result) add(format string, args ...any) {
	r.Lines = append(r.Lines, fmt.Sprintf(format, args...))
}

// Run performs setup checks and optional MCP config writes.
// All progress lines go to opts.Stderr (or os.Stderr). Never writes MCP protocol.
func Run(opts Options) (Result, error) {
	out := opts.Stderr
	if out == nil {
		out = os.Stderr
	}
	var res Result

	logf := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		res.add("%s", line)
		_, _ = fmt.Fprintln(out, line)
	}

	bin := opts.BinaryPath
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return res, fmt.Errorf("resolve binary path: %w", err)
		}
		bin, err = filepath.Abs(exe)
		if err != nil {
			return res, err
		}
	} else {
		abs, err := filepath.Abs(bin)
		if err != nil {
			return res, err
		}
		bin = abs
	}
	logf("binary: %s", bin)
	if opts.DryRun {
		logf("mode: dry-run (no files will be modified)")
	}

	// 1. gcloud
	checkGcloud(logf)

	// 2. ADC
	adcPath := config.DefaultADCPath()
	logf("ADC path: %s", adcPath)
	if data, err := os.ReadFile(adcPath); err != nil {
		logf("ADC: missing — run:")
		logf("  gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform")
		logf("  (do NOT auto-run login; it opens a browser)")
	} else {
		logf("ADC: present (%d bytes)", len(data))
		if !strings.Contains(string(data), "authorized_user") && !strings.Contains(string(data), "service_account") {
			logf("ADC: warning — unexpected content (no type field)")
		}
		// Scope cannot be read reliably from the file; remind user.
		logf("ADC: ensure login included webmasters scope. If list_sites fails with 403, re-run:")
		logf("  gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform")
	}

	// 3. MCP clients
	logf("MCP clients:")
	snippet := mcpServerSnippet(bin)

	// Claude Code via CLI
	if _, err := exec.LookPath("claude"); err == nil {
		logf("  Claude Code: claude CLI found")
		cmd := fmt.Sprintf("claude mcp add --transport stdio gsc -- %s", shellQuote(bin))
		if opts.DryRun {
			logf("    dry-run would run: %s", cmd)
		} else {
			logf("    run manually (opens nothing): %s", cmd)
		}
	} else {
		logf("  Claude Code: claude CLI not found")
	}

	// Claude Desktop (macOS)
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		desktopPath := filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
		logf("  Claude Desktop config: %s", desktopPath)
		if err := mergeMCPConfig(desktopPath, "gsc", bin, opts.DryRun, logf); err != nil {
			logf("    error: %v", err)
		}
	}

	// Cursor
	if home, err := os.UserHomeDir(); err == nil {
		cursorPath := filepath.Join(home, ".cursor", "mcp.json")
		if _, err := os.Stat(filepath.Dir(cursorPath)); err == nil {
			logf("  Cursor config: %s", cursorPath)
			if err := mergeMCPConfig(cursorPath, "gsc", bin, opts.DryRun, logf); err != nil {
				logf("    error: %v", err)
			}
		} else {
			logf("  Cursor: config dir not found (skip)")
		}
	}

	// VS Code — don't guess path
	logf("  VS Code: path not auto-detected — paste this into your MCP settings:")
	logf("%s", indent(snippet, "    "))

	logf("Note: ADC users need no env block in MCP config — credentials come from the ADC default path.")

	// 4. Optional live check (uses ADC default path / env; never prints secrets)
	logf("verify: attempting list_sites with resolved credentials...")
	if opts.DryRun {
		logf("  dry-run: skip live API call")
	} else if err := verifyListSites(logf); err != nil {
		logf("  list_sites failed: %v", err)
		logf("  (setup config may still be fine — fix credentials/scopes and retry)")
	}

	logf("Done.")
	return res, nil
}

func verifyListSites(logf func(string, ...any)) error {
	cfg, err := config.Resolve("")
	if err != nil {
		return err
	}
	logf("  credentials source: %s (type=%s)", cfg.Source, cfg.CredType)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := gscclient.New(ctx, cfg)
	if err != nil {
		return err
	}
	resp, err := client.ListSites(ctx)
	if err != nil {
		return err
	}
	n := 0
	if resp != nil {
		n = len(resp.SiteEntry)
	}
	logf("  list_sites OK — %d propert(y/ies) visible", n)
	return nil
}

func mcpServerSnippet(bin string) string {
	// ADC: no env required
	obj := map[string]any{
		"mcpServers": map[string]any{
			"gsc": map[string]any{
				"command": bin,
			},
		},
	}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return string(b)
}

func checkGcloud(logf func(string, ...any)) string {
	path, err := exec.LookPath("gcloud")
	if err != nil {
		logf("gcloud: NOT FOUND in PATH")
		printGcloudInstallInstructions(logf)
		checkCommonGcloudDirs(logf)
		return ""
	}
	logf("gcloud: found at %s", path)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		logf("gcloud: found but failed to run (%v)", err)
		msg := strings.ToLower(string(out))
		if strings.Contains(msg, "python") || strings.Contains(msg, "cloudsdk_python") {
			logf("  This usually means gcloud cannot find a compatible Python (3.10–3.14).")
			logf("    Try: export CLOUDSDK_PYTHON=/path/to/python3.12 && gcloud --version")
			logf("    If that works, add it to your shell profile.")
		}
		logf("  If you installed by extracting a tarball without running install.sh, bundled Python may be missing.")
		logf("    Recommended fix: reinstall with the official installer.")
		switch runtime.GOOS {
		case "darwin":
			logf("      brew install --cask google-cloud-sdk")
		case "linux":
			logf("      https://cloud.google.com/sdk/docs/install#linux")
		case "windows":
			logf("      https://cloud.google.com/sdk/docs/install#windows")
		}
		return ""
	}

	first := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
	logf("gcloud: %s", first)
	return path
}

func printGcloudInstallInstructions(logf func(string, ...any)) {
	logf("  Install Google Cloud SDK (~713 MB download from Google), then re-run setup:")
	switch runtime.GOOS {
	case "darwin":
		logf("    brew install --cask google-cloud-sdk")
	case "linux":
		logf("    curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-latest-linux-x86_64.tar.gz")
		logf("    tar -xf google-cloud-cli-latest-linux-x86_64.tar.gz && ./google-cloud-sdk/install.sh")
		logf("    (or use your distribution's google-cloud-sdk package)")
	case "windows":
		logf("    https://dl.google.com/dl/cloudsdk/channels/rapid/GoogleCloudSDKInstaller.exe")
		logf("    (or winget install Google.CloudSDK)")
	default:
		logf("    https://cloud.google.com/sdk/docs/install")
	}
	logf("  Warning: do not just extract a tarball without running install.sh — bundled Python will not be set up and gcloud may fail with a system Python version error.")
}

func checkCommonGcloudDirs(logf func(string, ...any)) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	candidates := []string{
		filepath.Join(home, "google-cloud-sdk", "bin", "gcloud"),
		filepath.Join(home, "google-cloud-cli", "bin", "gcloud"),
		"/usr/local/google-cloud-sdk/bin/gcloud",
		"/opt/google-cloud-sdk/bin/gcloud",
	}
	for _, c := range candidates {
		if _, sterr := os.Stat(c); sterr == nil {
			dir := filepath.Dir(c)
			logf("  Found an installation at %s but it is not in PATH.", dir)
			if runtime.GOOS == "windows" {
				logf("    Add it to PATH in System Settings, or run:")
				logf("      $env:Path = \"%s;$env:Path\"", dir)
			} else {
				logf("    Add it: export PATH=%q:$PATH  (restart terminal after)", dir)
			}
			return
		}
	}
}

// MergeMCPConfigFile reads path, merges server entry under mcpServers[name], writes back with backup.
// Exported for tests.
func MergeMCPConfigFile(path, name, command string, dryRun bool) (merged []byte, backupPath string, err error) {
	var root map[string]any
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return nil, "", readErr
		}
		root = map[string]any{}
	} else {
		if err := json.Unmarshal(raw, &root); err != nil {
			return nil, "", fmt.Errorf("parse %s: %w (fix JSON before re-running setup)", path, err)
		}
	}

	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		// Some configs use "servers" — only touch mcpServers to stay conservative.
		servers = map[string]any{}
		root["mcpServers"] = servers
	}

	entry := map[string]any{
		"command": command,
	}
	servers[name] = entry

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, "", err
	}
	out = append(out, '\n')

	if dryRun {
		return out, "", nil
	}

	if _, err := os.Stat(path); err == nil {
		backupPath = fmt.Sprintf("%s.bak-%s", path, time.Now().UTC().Format("20060102T150405Z"))
		if err := copyFile(path, backupPath); err != nil {
			return nil, "", fmt.Errorf("backup: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return nil, backupPath, err
	}
	return out, backupPath, nil
}

func mergeMCPConfig(path, name, command string, dryRun bool, logf func(string, ...any)) error {
	merged, backup, err := MergeMCPConfigFile(path, name, command, dryRun)
	if err != nil {
		return err
	}
	if dryRun {
		logf("    dry-run merged config would be:")
		logf("%s", indent(string(merged), "      "))
		return nil
	}
	if backup != "" {
		logf("    backup: %s", backup)
	}
	logf("    wrote %s", path)
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
