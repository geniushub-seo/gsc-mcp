# INSTALL — instructions for AI agents

**This document is for an AI agent helping a user install this MCP server.** Follow it when a user gives you this repository and asks you to set it up.

> If you are modifying this project's source code, read [AGENTS.md](AGENTS.md) instead.

## Preferred: one-command installation

macOS / Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
```

**Always use `install.ps1` on Windows; do not download the binary with `Invoke-WebRequest`.** Manual downloads skip SHA-256 verification and SmartScreen unblocking and commonly install outside PATH, forcing absolute paths into every later MCP configuration.

To preview the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash -s -- --dry-run
```

```powershell
$env:DRY_RUN = '1'; irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
```

The installer detects the platform, downloads the release, verifies SHA-256, installs to `~/.local/bin` (`%LOCALAPPDATA%\Programs\gsc-mcp` on Windows), removes the quarantine / SmartScreen block, and reports PATH instructions. It does **not** run `gcloud login`, change MCP configuration, or edit a shell rc file.

When an agent is performing the installation, the **agent executes all of the
following commands** through its terminal tool. The user must not be sent to
Terminal. For the login command, tell the user that Google will open in their
browser; the user only chooses an account and approves access.

```bash
gcloud auth login
gcloud projects list
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project PROJECT_ID
gcloud services enable searchconsole.googleapis.com --project=PROJECT_ID
gsc-mcp doctor
gsc-mcp setup
```

`PROJECT_ID` is an id from the `gcloud projects list` output; the agent substitutes it and never sends the placeholder. The last four commands are all required: see step 1c for the quota project, step 1d for enabling the API. `doctor` must print `list_sites OK` before `setup` counts as working; run it again after any command above that fails.

## Responsibility boundary

| The agent must do | The user must do personally |
|---|---|
| Execute every shell / PowerShell command, including starting `gcloud auth application-default login` | Choose the Google account and approve access in the browser page opened by gcloud |
| Install dependencies, set the quota project, merge MCP configuration, and verify `list_sites` | Select or create a GCP project in browser UI when no safe automated choice exists |
| Keep the OAuth process running and resume automatically when it exits | Approve administrator or OS security prompts when required |
| Diagnose failures without displaying credential contents | Restart a GUI client when the agent cannot do it |

**ADC with the user's Google account is the preferred path.** It does not require a service account or adding an email address to every property.

## Step 0: ask first

1. Which AI client will they use? Claude Desktop, Claude Code, Codex, Cursor, Hermes, or VS Code?
2. Is gcloud installed? Check with `which gcloud`.
3. Does ADC already exist? Check without reading its contents. If it does not,
   the agent starts `gcloud auth application-default login` itself.

**Do not present the user with a choice of credential type.** ADC is the only path
you propose, configure, or explain. Do not mention, list, or compare service accounts
unless the user first says one of the following:

- they already hold a service-account JSON key,
- this runs in CI or on a headless machine,
- no browser can be opened on the target machine,
- a non-human identity is required.

Only then read **Advanced: service account** at the end of this document. Absent one
of those statements, a service account is out of scope for the installation, even if
the user's setup would technically support it.

## Step 1: gcloud and ADC login

### 1a. Install gcloud if absent

- macOS with homebrew: `brew install --cask google-cloud-sdk`
- macOS without homebrew: **do not install homebrew for this.** Its installer
  requires the account to be a local Administrator and stops at `Need sudo access
  on macOS` otherwise, which is common on managed laptops. Follow *No-homebrew
  macOS* below instead.
- Linux:
  ```bash
  curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-latest-linux-x86_64.tar.gz
  tar -xf google-cloud-cli-latest-linux-x86_64.tar.gz
  ./google-cloud-sdk/install.sh
  ```
  Or use the distribution's `google-cloud-sdk` package.
- Windows: `winget install Google.CloudSDK`, or download [GoogleCloudSDKInstaller.exe](https://dl.google.com/dl/cloudsdk/channels/rapid/GoogleCloudSDKInstaller.exe).

The ~713 MB download is the Google Cloud SDK, not this project.

**No-homebrew macOS.** gcloud is written in Python and needs 3.10+. macOS ships
`/usr/bin/python3` 3.9.6, and `install.sh` is itself a Python program, so running
it does not fix this — it aborts with `TypeError: unsupported operand type(s) for
|`. There is no bundled-python tarball for macOS. Provide a Python first:

```bash
# 1. locate a python3 >= 3.10
for p in python3.13 python3.12 python3.11 python3.10 "$HOME"/.local/share/uv/python/*/bin/python3; do
  c=$(command -v "$p" 2>/dev/null) || { [ -x "$p" ] && c="$p"; } || continue
  "$c" -c 'import sys;sys.exit(0 if sys.version_info>=(3,10) else 1)' 2>/dev/null && { echo "FOUND: $c"; break; }
done

# 2. none found: install one with uv (no Administrator rights needed)
curl -LsSf https://astral.sh/uv/install.sh | sh
"$HOME/.local/bin/uv" python install 3.12   # then rerun step 1 for the path

# 3. install gcloud with it (Intel Mac: replace darwin-arm with darwin-x86_64)
export CLOUDSDK_PYTHON=<path from step 1>
tmp=$(mktemp -d)
curl -fsSL https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-darwin-arm.tar.gz -o "$tmp/gcloud.tar.gz"
mkdir -p "$HOME/.local/share" && tar -xzf "$tmp/gcloud.tar.gz" -C "$HOME/.local/share"
"$HOME/.local/share/google-cloud-sdk/install.sh" --quiet --path-update=false
```

Installed this way, **every subsequent gcloud command must carry
`CLOUDSDK_PYTHON`** (export it in the shell profile, or prefix each command).
A homebrew install does not need it.

The agent is responsible for starting commands that open a browser. Tell the
user what browser interaction is about to occur, then execute the command and
keep it running while they approve access.

### 1b. Agent starts ADC login; user only approves in the browser

Tell the user:

> I will open Google's sign-in page. Choose the Google account that has Search
> Console access and approve the requested read-only access. You do not need to
> open Terminal or copy any command.

Then the agent runs this through its terminal tool and waits for it to finish:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
```

If the normal launch does not open a browser, first try the platform's browser
or OS-open capability. Do not fall back to repeatedly asking the user to paste
the shell command.

Confirm only that the credential file exists:

```bash
ls -la ~/.config/gcloud/application_default_credentials.json
```

On Windows the path is `%APPDATA%\gcloud\application_default_credentials.json`. Never print its contents in chat or logs.

### 1c. Set a quota project (required for ADC)

ADC belongs to a personal account, not a GCP project. Google therefore needs an explicit project to charge this request's quota to. A service-account key embeds project information; ADC does not.

The agent runs this itself, replacing `PROJECT_ID` with an id read from
`gcloud projects list` (the lowercase project ID, not its display name). Never
send the placeholder to the user as a command:

```bash
gcloud auth application-default set-quota-project PROJECT_ID
```

This writes `quota_project_id` into ADC JSON, which `gsc-mcp` passes through `option.WithQuotaProject`.

**If this command exits 1** with `does not have the "serviceusage.services.use" permission on this project`, the signed-in account has no rights on that project and retrying will not help. Either repeat both logins with an account that does, or have the project administrator grant the current account `roles/serviceusage.serviceUsageConsumer`; then re-run `gcloud auth application-default login`, which attaches the quota project on its own.

**Symptom of skipping it:** `list_sites` returns 403 `permission_denied` with `requires a quota project`. This is **not** a GSC property-permission problem. Do not ask the user to add another user in Search Console; run the command above.

### 1d. Enable the Search Console API on that project

A GCP project does not have the Search Console API enabled by default, and the quota project set in 1c is the one that needs it:

```bash
gcloud services enable searchconsole.googleapis.com --project=PROJECT_ID
```

Use the same id as 1c. **Symptom of skipping it:** a 403 whose `Details` block contains `"reason": "SERVICE_DISABLED"`. Setting the quota project again will not clear it.

To check before running it:

```bash
gcloud services list --enabled --filter=searchconsole.googleapis.com --project=PROJECT_ID
```

Empty output means the API is not enabled on that project.

### 1e. ADC recovery table

Use this table as an agent. Start with the symptom and issue the listed fix; misdiagnosis leads to useless permission changes, reinstalls, or environment variables.

| Symptom | Actual cause | Do not | Fix |
|---|---|---|---|
| 403 `permission_denied` containing `requires a quota project` | ADC has no quota project | Ask the user to add a GSC user | Return to 1c and run `set-quota-project` |
| Sudden `auth_failed` containing `cannot fetch token` or `invalid_grant` | Refresh token expired after a password change, inactivity, or org policy | Reinstall the binary or rerun `install.sh` | Rerun 1b's `application-default login` |
| Sitemap `submit` / `delete` is rejected or returns 403 | Local write gate closed, or ADC token is still readonly | Skip `GSC_ENABLE_WRITE=true` after re-login, or skip re-login | Set `GSC_ENABLE_WRITE=true` **and**, for ADC, log in again with `webmasters` below |
| `gcloud projects list` says credentials expired after successful ADC login | A separate token is used | Rerun `application-default login` | Run `gcloud auth login` |
| Windows reports `gcloud` is not recognized although it is installed | gcloud is absent from PATH | Rerun `winget install` | Use `%LOCALAPPDATA%\Google\Cloud SDK\google-cloud-sdk\bin\gcloud.cmd` |

`gcloud auth login` and `gcloud auth application-default login` are separate credentials with separate expiration:

| Command | Used by | Stored in |
|---|---|---|
| `gcloud auth login` | The `gcloud` CLI itself (`projects list`, `services list`, etc.) | gcloud's own config directory |
| `gcloud auth application-default login` | Programs you run, including `gsc-mcp` | `application_default_credentials.json` |

After ADC login, `gcloud projects list` can still fail because it uses the first credential. `gsc-mcp` needs the second credential for queries, while an agent using gcloud to find a quota project needs the first. When an account appears to have just logged in but is reported expired, use the other login command.

### ADC writes require re-login with write scope

`GSC_ENABLE_WRITE=true` is the local write gate and applies to ADC as well as service accounts. Re-logging with write scope is not enough: without the flag, `submit` / `delete` still return `write_disabled`. The flag also cannot add scopes to an already issued ADC token.

ADC writes therefore need both the flag **and** a token issued with the write scope:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters,https://www.googleapis.com/auth/cloud-platform
```

`delete` additionally requires `GSC_ALLOW_DESTRUCTIVE=true`. That flag is a local gsc-mcp guard and applies to both ADC and service accounts.

### Diagnose with doctor, then stderr

Run this first for ADC failures:

```bash
gsc-mcp doctor
```

It performs all checks, makes a real `list_sites` call, writes no files, and prints a targeted fix. `gsc-mcp setup --dry-run` skips API calls and cannot verify credentials.

All gsc-mcp logs use stderr; stdout is MCP protocol data only. To identify the selected credential source—for example, an old `GOOGLE_APPLICATION_CREDENTIALS` overriding ADC—enable debug logging:

```bash
GSC_LOG_LEVEL=debug gsc-mcp
```

The `credentials loaded from ...` line identifies the winning credential source.

## Step 2: obtain the binary

### Preferred: `install.sh`

After installation, the binary is normally `~/.local/bin/gsc-mcp`.

### Advanced: build from source

Use this only when there is no release for the platform:

```bash
go version   # Go 1.26+ required
go build -trimpath -ldflags="-s -w" -o ./bin/gsc-mcp ./cmd/gsc-mcp
mkdir -p ~/.local/bin && cp ./bin/gsc-mcp ~/.local/bin/gsc-mcp && chmod +x ~/.local/bin/gsc-mcp
```

Then run:

```bash
gsc-mcp setup --dry-run
gsc-mcp setup
```

`setup` writes its output to stderr, not MCP stdio.

### macOS Gatekeeper

`install.sh` runs `xattr -d com.apple.quarantine`. A browser-downloaded binary may still be blocked: right-click it in Finder, choose **Open**, then confirm.

## Step 3: add MCP configuration

### The most important rule

**Merge; never replace.** Read the existing JSON, add `gsc` under `mcpServers`, and write valid JSON back. Stop if existing JSON is invalid. Back up the file as `<file>.bak-<timestamp>` before changing it.

### ADC: no `env` block

```json
{
  "mcpServers": {
    "gsc": {
      "command": "/Users/YOUR_USER/bin/gsc-mcp"
    }
  }
}
```

ADC uses its default credential path and requires no `GOOGLE_*` variable.

### Claude Code

```bash
claude mcp add --transport stdio gsc -- /Users/YOUR_USER/bin/gsc-mcp
```

ADC needs no credential `-e` option.

### Claude Desktop

| System | Path |
|---|---|
| macOS | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |

Merge the same `gsc` object. Fully quit and restart Claude Desktop.

### Cursor

When this repository is opened, Cursor reads `.cursor/mcp.json`; no separate configuration is necessary. For global use in other projects, merge the same `gsc` object into `~/.cursor/mcp.json`.

### Codex

When this repository is opened, Codex reads root `AGENTS.md` and `.agents/skills/`. Add the same `gsc` MCP server using Codex MCP configuration. Do not put credentials in the repository's `.codex/` directory.

### Hermes

Hermes keeps MCP configuration in the active profile returned by
`hermes config path`; it does not auto-load a repository `.hermes/` config.
This repository provides an onboarding bundle that uses the Hermes CLI to
preserve existing settings, add `gsc`, and test all six tools:

```bash
bash .hermes/setup.sh
```

Then start a new Hermes session and call `list_sites`. See
[.hermes/README.md](.hermes/README.md) for the completion evidence and recovery
steps.

For a manual merge, **merge**, do not replace, this entry under `mcp_servers`
in the file printed by `hermes config path`:

```yaml
mcp_servers:
  gsc:
    command: gsc-mcp
    supports_parallel_tool_calls: false
```

ADC needs no `env` block. Restart Hermes, then first ask it to call `list_sites`.

### VS Code and Copilot

Their configuration path varies. Do not guess it. Consult that client's local
or official configuration instructions and perform the merge through the
agent's tools. Do not send a novice user to Terminal to edit or paste JSON.

## Step 4: verify

### Terminal verification

Do not hand-write JSON-RPC against stdio. Use:

```bash
gsc-mcp doctor
```

It checks gcloud / ADC / MCP config and makes one real `list_sites` call without writing files. A non-zero exit means the environment is not ready.

Then test from the agent:

> Use gsc `list_sites` to list my Search Console properties.

An empty list means the account has no GSC properties, the API is disabled, or the ADC login lacks the webmasters scope. Re-run the login if needed.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `no credentials found; checked: ...` | ADC is not logged in; run `application-default login`. |
| `list_sites` returns 403 / auth error | Scope is insufficient or API is disabled; rerun login with `--scopes`. |
| Empty property list | The Google account has no GSC property or is the wrong account. |
| `GSC_ENABLE_WRITE` still cannot submit | ADC also needs a `webmasters` token; re-login with that scope. The flag is still required as the local gate. |
| GUI does not show the tool | Configuration was not merged or the app was not fully restarted. |
| Refresh suddenly fails | Token expired; rerun login. |

## Security rules

- Never print ADC or service-account JSON contents.
- Never overwrite an entire MCP config file; always merge and back up.
- The agent starts `gcloud auth login` and `application-default login`; the
  human performs only the Google browser consent. Never automate the consent
  click or display the resulting credentials.
- Keep the server read-only by default; do not proactively enable write flags.

## Advanced: service account (headless / CI)

Use this only when the user explicitly requires a non-human identity.

1. Enable the API in Cloud Console, create a service account, and download its JSON key.
2. Add its `client_email` as a user to **every** GSC property.
3. Store it safely:

   ```bash
   mkdir -p ~/.config/gsc-mcp && chmod 700 ~/.config/gsc-mcp
   cp <key.json> ~/.config/gsc-mcp/service-account.json
   chmod 600 ~/.config/gsc-mcp/service-account.json
   ```

4. Its MCP configuration requires `env`:

   ```json
   {
     "mcpServers": {
       "gsc": {
         "command": "/absolute/path/to/gsc-mcp",
         "env": {
           "GOOGLE_SERVICE_ACCOUNT_FILE": "/absolute/path/to/service-account.json"
         }
       }
     }
   }
   ```

For service accounts, `GSC_ENABLE_WRITE=true` upgrades the scope to `webmasters`; `delete` additionally requires `GSC_ALLOW_DESTRUCTIVE=true`.

## Environment variable quick reference

These are the only variables an ADC installation ever needs. ADC itself needs none
of them — an ADC `mcpServers` entry carries no `env` block at all (step 3).

| Variable | Purpose |
|---|---|
| `GOOGLE_APPLICATION_CREDENTIALS` | Override the ADC credential-file path. Rarely needed. |
| `GSC_LOG_LEVEL` | `warn` is a useful default. |
| `GSC_ENABLE_WRITE` | Local write gate for every credential type. Under ADC the token must also carry `webmasters` scope (see "ADC writes require re-login with write scope"). |
| `GSC_ALLOW_DESTRUCTIVE` | Permits delete. Requires `GSC_ENABLE_WRITE=true`. |

Service-account variables are deliberately not listed here; they belong to
**Advanced: service account** above and are only in scope once the user has stated
one of the conditions in step 0.

See [README_EN.md](README_EN.md) and [SPEC.md](SPEC.md) section 4 for full details.
