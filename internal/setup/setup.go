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
	// DryRun suppresses every file write.
	DryRun bool

	// LiveCheck forces the read-only list_sites verification even when DryRun
	// is set. Kept separate from DryRun because "don't touch my files" and
	// "don't tell me whether my credentials work" are unrelated requests, and
	// bundling them left `setup --dry-run` unable to answer the only question
	// most callers have. `doctor` sets both.
	LiveCheck bool

	BinaryPath string // absolute path to gsc-mcp binary
	Stdout     *os.File
	Stderr     *os.File
}

// serverName is the key this server is registered under in every MCP client config.
const serverName = "gsc"

// goos is runtime.GOOS, overridable so the per-OS path logic can be tested
// from any host. Same pattern as config.adcPathFn.
var goos = runtime.GOOS

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
		// doctor sets DryRun and LiveCheck together. Calling that "dry-run"
		// reads as "a real run is still pending", and an assisting agent then
		// hunts for the command that would perform it. There is none: doctor
		// is the whole check.
		if opts.LiveCheck {
			logf("mode: read-only (diagnostic — no files will be modified)")
		} else {
			logf("mode: dry-run (no files will be modified)")
		}
	}

	// 1. gcloud
	gcloudPath := checkGcloud(logf)

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

	// 3. Live verification first. Writing MCP config before credentials work
	// leaves clients pointing at an unusable server. Missing default ADC is
	// not itself a failure: env / service-account JSON are still valid.
	var liveErr error
	needLive := opts.LiveCheck || !opts.DryRun
	logf("verify: attempting list_sites with resolved credentials...")
	switch {
	case !needLive:
		logf("  dry-run: skip live API call (run `gsc-mcp doctor` to verify without writing files)")
	default:
		if err := verifyListSites(logf); err != nil {
			logf("  list_sites failed: %v", err)
			explainVerifyFailure(err, gcloudPath, logf)
			liveErr = err
		}
	}

	// 4. MCP clients. Skip file writes when a required live check failed.
	var mergeErr error
	snippet := mcpServerSnippet(bin)
	if liveErr != nil && !opts.DryRun {
		logf("MCP clients: skipped writes because live verification failed")
	} else {
		logf("MCP clients:")

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

		// Claude Desktop — path differs per OS; "" means we have no location for it.
		if desktopPath := claudeDesktopConfigPath(); desktopPath != "" {
			logf("  Claude Desktop config: %s", desktopPath)
			if err := mergeMCPConfig(desktopPath, bin, opts.DryRun, logf); err != nil {
				logf("    error: %v", err)
				if mergeErr == nil {
					mergeErr = err
				}
			}
		}

		// Cursor
		if home, err := os.UserHomeDir(); err == nil {
			cursorPath := filepath.Join(home, ".cursor", "mcp.json")
			if _, err := os.Stat(filepath.Dir(cursorPath)); err == nil {
				logf("  Cursor config: %s", cursorPath)
				if err := mergeMCPConfig(cursorPath, bin, opts.DryRun, logf); err != nil {
					logf("    error: %v", err)
					if mergeErr == nil {
						mergeErr = err
					}
				}
			} else {
				logf("  Cursor: config dir not found (skip)")
			}
		}

		// Project-scoped .mcp.json (Codex, Claude Code project config, and others).
		// Only touched when it already exists: creating one in whatever directory
		// setup happens to run from would scatter config files around.
		const projectConfig = ".mcp.json"
		if _, err := os.Stat(projectConfig); err == nil {
			abs, absErr := filepath.Abs(projectConfig)
			if absErr != nil {
				abs = projectConfig
			}
			logf("  Project config found: %s", abs)
			if err := mergeMCPConfig(projectConfig, bin, opts.DryRun, logf); err != nil {
				logf("    error: %v", err)
				if mergeErr == nil {
					mergeErr = err
				}
			}
		} else {
			logf("  Project .mcp.json: not in the current directory (skip)")
			logf("    If your client uses one (Codex, project-scoped Claude Code), create it there with:")
			logf("%s", indent(snippet, "      "))
		}

		// VS Code — don't guess path
		logf("  VS Code: path not auto-detected — paste this into your MCP settings:")
		logf("%s", indent(snippet, "    "))

		logf("Note: ADC users need no env block in MCP config — credentials come from the ADC default path.")
	}

	if liveErr != nil {
		logf("Failed: live verification did not succeed.")
		return res, fmt.Errorf("live verification failed: %w", liveErr)
	}
	if mergeErr != nil {
		logf("Failed: MCP config merge did not succeed.")
		return res, fmt.Errorf("merge MCP config: %w", mergeErr)
	}
	logf("Done.")
	return res, nil
}

// explainVerifyFailure turns a failed list_sites into the specific next action.
// Each branch exists because the raw error points somewhere unhelpful: the
// quota-project 403 reads as a Search Console permission problem, and a token
// rejection reads as a broken install.
func explainVerifyFailure(err error, gcloudPath string, logf func(string, ...any)) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "quota project"):
		logf("  Diagnosis: the credentials have no quota project. This is NOT a property")
		logf("  permission problem — do not add users in the Search Console UI.")
		suggestQuotaProject(gcloudPath, logf)
	case strings.Contains(msg, "cannot fetch token"), strings.Contains(msg, "invalid_grant"):
		logf("  Diagnosis: the credentials were rejected before any API call. Retrying will not help.")
		logf("  Re-run the ADC login:")
		logf("    gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform")
	case strings.Contains(msg, "accessnotconfigured"), strings.Contains(msg, "has not been used in project"):
		logf("  Diagnosis: the Search Console API is not enabled on the quota project.")
		logf("  Enable it: https://console.cloud.google.com/apis/library/searchconsole.googleapis.com")
		logf("  (select the quota project in the top-left project picker first)")
	default:
		logf("  (MCP config may still be fine — fix credentials/scopes and retry)")
	}
}

// suggestQuotaProject lists candidate GCP projects so the caller picks a real
// one instead of guessing from names. Guessing is the observed failure mode:
// an agent with no list to work from picks whichever project id looks
// topical and only finds out it was wrong on the next error.
func suggestQuotaProject(gcloudPath string, logf func(string, ...any)) {
	logf("  Fix: gcloud auth application-default set-quota-project YOUR_PROJECT_ID")

	if gcloudPath == "" {
		logf("  (gcloud is not usable here, so the project list cannot be shown —")
		logf("   run `gcloud projects list` in a working terminal to find the id)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, gcloudPath, "projects", "list",
		"--format=value(projectId)", "--limit=25").Output()
	if err != nil {
		logf("  Could not list projects (%v). Run this yourself to find the id:", err)
		logf("    gcloud projects list")
		logf("  If it reports expired credentials, note that `gcloud auth login` and")
		logf("  `gcloud auth application-default login` are separate logins — you need both.")
		return
	}

	ids := strings.Fields(strings.TrimSpace(string(out)))
	if len(ids) == 0 {
		logf("  This account has no GCP projects. Create one, then enable the Search Console API:")
		logf("    https://console.cloud.google.com/projectcreate")
		return
	}

	logf("  Projects available to this account (%d):", len(ids))
	for _, id := range ids {
		logf("    %s", id)
	}
	logf("  Pick the one with the Search Console API enabled — verify before setting it:")
	logf("    gcloud services list --enabled --filter=searchconsole.googleapis.com --project=PROJECT_ID")
	logf("  Empty output means that project is the wrong one (or the API is not enabled yet).")
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
		// An off-PATH install is still usable — every gcloud command we suggest
		// works with an absolute path. Only tell the user to install if the
		// disk search also comes up empty.
		path = findGcloudOffPath(logf)
		if path == "" {
			printGcloudInstallInstructions(logf)
			return ""
		}
	} else {
		logf("gcloud: found at %s", path)
	}

	// Locating gcloud is not the same as being able to run it: a sandboxed
	// shell can see the launcher and still be denied execution, and a
	// tarball install can be missing its bundled Python. Both surface here.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		logf("gcloud: %s exists but failed to run (%v)", path, err)
		msg := strings.ToLower(string(out) + " " + err.Error())
		switch {
		case strings.Contains(msg, "python") || strings.Contains(msg, "cloudsdk_python"):
			logf("  This usually means gcloud cannot find a compatible Python (3.10–3.14).")
			logf("    Try: export CLOUDSDK_PYTHON=/path/to/python3.12 && gcloud --version")
			logf("    If that works, add it to your shell profile.")
			logf("  If you installed by extracting a tarball without running install.sh, bundled Python may be missing.")
			logf("    Recommended fix: reinstall with the official installer.")
		case strings.Contains(msg, "access is denied") || strings.Contains(msg, "permission denied"):
			logf("  gcloud exists but this shell is not allowed to execute it.")
			logf("  This is an environment restriction, not a gsc-mcp problem — reinstalling will not help.")
			logf("  If you are an AI agent in a sandbox: ask the user to run the gcloud commands")
			logf("  in their own terminal and report back. Do not keep retrying here.")
			return ""
		default:
			logf("  Recommended fix: reinstall with the official installer.")
		}
		switch goos {
		case "darwin":
			logf("      with homebrew:    brew install --cask google-cloud-sdk")
			logf("      without homebrew: see INSTALL.md, section \"No-homebrew macOS\"")
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

// claudeDesktopConfigPath returns the Claude Desktop config location for this
// OS, or "" where the app does not ship. Windows was previously skipped even
// though INSTALL.md documents its path, so Windows users were told setup would
// handle a file it never touched.
func claudeDesktopConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "Claude", "claude_desktop_config.json")
	default:
		return ""
	}
}

func printGcloudInstallInstructions(logf func(string, ...any)) {
	logf("  Install Google Cloud SDK (~713 MB download from Google), then re-run setup:")
	switch goos {
	case "darwin":
		logf("    with homebrew:    brew install --cask google-cloud-sdk")
		logf("    without homebrew: do NOT install homebrew for this — its installer requires")
		logf("      a local Administrator account and stops at \"Need sudo access on macOS\".")
		logf("      gcloud needs Python 3.10+, macOS ships 3.9.6, and install.sh is itself a")
		logf("      Python program, so running it does not fix this. Point CLOUDSDK_PYTHON at")
		logf("      a Python 3.10+ first; uv python install 3.12 needs no Administrator rights.")
		logf("      Full steps: INSTALL.md, section \"No-homebrew macOS\".")
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
	logf("  Warning: always run install.sh after extracting a tarball. On macOS it additionally")
	logf("  needs CLOUDSDK_PYTHON set to a Python 3.10+ interpreter, or it aborts on system python3.")
}

// gcloudCandidates lists install locations to probe when gcloud is not in PATH.
// Windows needs its own list: the winget and official-installer paths live under
// AppData/Program Files and the launcher is gcloud.cmd, so a POSIX-only list
// misses every Windows install — the exact case that sends an agent on a
// recursive disk search.
func gcloudCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	if goos == "windows" {
		const sdkBin = `Google\Cloud SDK\google-cloud-sdk\bin\gcloud.cmd`
		var out []string
		for _, base := range []string{
			os.Getenv("LOCALAPPDATA"),
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
		} {
			if base != "" {
				out = append(out, filepath.Join(base, sdkBin))
			}
		}
		if home != "" {
			// Covers the case where LOCALAPPDATA is unset in a stripped environment.
			out = append(out, filepath.Join(home, "AppData", "Local", sdkBin))
		}
		return out
	}

	var out []string
	if home != "" {
		out = append(out,
			filepath.Join(home, "google-cloud-sdk", "bin", "gcloud"),
			filepath.Join(home, "google-cloud-cli", "bin", "gcloud"),
		)
	}
	return append(out,
		"/usr/local/google-cloud-sdk/bin/gcloud",
		"/opt/google-cloud-sdk/bin/gcloud",
		"/opt/homebrew/share/google-cloud-sdk/bin/gcloud",
		"/usr/local/share/google-cloud-sdk/bin/gcloud",
	)
}

// findGcloudOffPath returns the first gcloud found outside PATH, or "".
func findGcloudOffPath(logf func(string, ...any)) string {
	for _, c := range gcloudCandidates() {
		if _, err := os.Stat(c); err != nil {
			continue
		}
		dir := filepath.Dir(c)
		logf("  Found an installation at %s but it is not in PATH.", dir)
		logf("  Using the absolute path for the checks below; run any gcloud command the same way.")
		if goos == "windows" {
			logf("    To fix PATH for this shell:  $env:Path = \"%s;$env:Path\"", dir)
			logf("    To fix it permanently, add that directory in System Settings > Environment Variables.")
		} else {
			logf("    Add it: export PATH=%q:$PATH  (restart terminal after)", dir)
		}
		return c
	}
	return ""
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
		if root == nil {
			return nil, "", fmt.Errorf("root of %s is not an object; fix the file before re-running setup", path)
		}
	}

	var servers map[string]any
	if raw, exists := root["mcpServers"]; exists {
		typed, ok := raw.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("mcpServers in %s is not an object; fix the file before re-running setup", path)
		}
		servers = typed
	} else {
		// Some configs use "servers" — only touch mcpServers to stay conservative.
		servers = map[string]any{}
		root["mcpServers"] = servers
	}

	// Update only command. Replacing the whole object would drop env, args,
	// and any other client-specific fields the user already configured.
	// A present non-object entry is left untouched and reported as an error.
	var entry map[string]any
	if raw, exists := servers[name]; exists {
		typed, ok := raw.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("mcpServers.%s in %s is not an object; fix the file before re-running setup", name, path)
		}
		entry = typed
	} else {
		entry = map[string]any{}
	}
	entry["command"] = command
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

// mergeMCPConfig adds the gsc server entry to an MCP client config file.
// The server name is fixed: every client we touch keys this server as "gsc".
func mergeMCPConfig(path, command string, dryRun bool, logf func(string, ...any)) error {
	_, backup, err := MergeMCPConfigFile(path, serverName, command, dryRun)
	if err != nil {
		return err
	}
	if dryRun {
		// Print only the entry we would add, never the merged document. Other
		// servers in the same file routinely carry API keys in their env block,
		// and this output gets pasted into chat transcripts and bug reports.
		logf("    dry-run: would add this entry under mcpServers (existing entries untouched):")
		logf("%s", indent(mcpServerSnippet(command), "      "))
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
