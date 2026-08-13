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

Print the following commands for the user to run themselves; do not run their browser-based login for them:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project YOUR_PROJECT_ID
gsc-mcp setup
```

The second command is required; see step 1c. After installation, or whenever setup fails, run `gsc-mcp doctor` (a complete, non-writing check plus a real `list_sites` call).

## What you may and may not do

| You may do yourself | The user must do personally |
|---|---|
| Run `install.sh` / `gsc-mcp setup` | Complete browser-based `gcloud auth application-default login` |
| Merge MCP configuration after backing it up | Enable the Search Console API in Cloud Console, if needed |
| Verify `list_sites` | Restart GUI clients such as Claude Desktop |
| Detect whether gcloud / ADC exists | — |

**ADC with the user's Google account is the preferred path.** It does not require a service account or adding an email address to every property.

## Step 0: ask first

1. Which AI client will they use? Claude Desktop, Claude Code, Codex, Cursor, Hermes, or VS Code?
2. Is gcloud installed? Check with `which gcloud`.
3. Have they already run `gcloud auth application-default login`?

For an explicit CI or headless requirement, use **Advanced: service account** at the end of this document.

## Step 1: gcloud and ADC login

### 1a. Install gcloud if absent

- macOS: `brew install --cask google-cloud-sdk`
- Linux:
  ```bash
  curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-latest-linux-x86_64.tar.gz
  tar -xf google-cloud-cli-latest-linux-x86_64.tar.gz
  ./google-cloud-sdk/install.sh
  ```
  Or use the distribution's `google-cloud-sdk` package.
- Windows: `winget install Google.CloudSDK`, or download [GoogleCloudSDKInstaller.exe](https://dl.google.com/dl/cloudsdk/channels/rapid/GoogleCloudSDKInstaller.exe).

**Warning:** do not only unpack the tarball. Without its `install.sh`, bundled Python is not installed and the launcher falls back to system `python3` (macOS ships 3.9; gcloud requires 3.10–3.14). The resulting error looks like a `gsc-mcp` problem. The ~713 MB download is the Google Cloud SDK, not this project.

Never automatically run a command that opens a browser; print it for the user.

### 1b. Ask the user to log in to ADC

Send this verbatim:

> Run this in a terminal. It opens a browser so you can sign in to your Google account:
>
> ```bash
> gcloud auth application-default login \
>   --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
> ```
>
> Tell me when it finishes. If the Search Console API is not enabled, open https://console.cloud.google.com/apis/library/searchconsole.googleapis.com and select **Enable**.

Confirm only that the credential file exists:

```bash
ls -la ~/.config/gcloud/application_default_credentials.json
```

On Windows the path is `%APPDATA%\gcloud\application_default_credentials.json`. Never print its contents in chat or logs.

### 1c. Set a quota project (required for ADC)

ADC belongs to a personal account, not a GCP project. Google therefore needs an explicit project to charge this request's quota to. A service-account key embeds project information; ADC does not.

Ask the user to run this, replacing `YOUR_PROJECT_ID` with the lowercase project ID where they enabled the Search Console API (not its display name):

```bash
gcloud auth application-default set-quota-project YOUR_PROJECT_ID
```

This writes `quota_project_id` into ADC JSON, which `gsc-mcp` passes through `option.WithQuotaProject`.

**Symptom of skipping it:** `list_sites` returns 403 `permission_denied` with `requires a quota project`. This is **not** a GSC property-permission problem. Do not ask the user to add another user in Search Console; run the command above.

### 1d. ADC recovery table

Use this table as an agent. Start with the symptom and issue the listed fix; misdiagnosis leads to useless permission changes, reinstalls, or environment variables.

| Symptom | Actual cause | Do not | Fix |
|---|---|---|---|
| 403 `permission_denied` containing `requires a quota project` | ADC has no quota project | Ask the user to add a GSC user | Return to 1c and run `set-quota-project` |
| Sudden `auth_failed` containing `cannot fetch token` or `invalid_grant` | Refresh token expired after a password change, inactivity, or org policy | Reinstall the binary or rerun `install.sh` | Rerun 1b's `application-default login` |
| Sitemap `submit` / `delete` is rejected or returns 403 | ADC scopes are fixed at login | Set `GSC_ENABLE_WRITE=true`; it has no effect for ADC | Log in again with `webmasters`, below |
| `gcloud projects list` says credentials expired after successful ADC login | A separate token is used | Rerun `application-default login` | Run `gcloud auth login` |
| Windows reports `gcloud` is not recognized although it is installed | gcloud is absent from PATH | Rerun `winget install` | Use `%LOCALAPPDATA%\Google\Cloud SDK\google-cloud-sdk\bin\gcloud.cmd` |

`gcloud auth login` and `gcloud auth application-default login` are separate credentials with separate expiration:

| Command | Used by | Stored in |
|---|---|---|
| `gcloud auth login` | The `gcloud` CLI itself (`projects list`, `services list`, etc.) | gcloud's own config directory |
| `gcloud auth application-default login` | Programs you run, including `gsc-mcp` | `application_default_credentials.json` |

After ADC login, `gcloud projects list` can still fail because it uses the first credential. `gsc-mcp` needs the second credential for queries, while an agent using gcloud to find a quota project needs the first. When an account appears to have just logged in but is reported expired, use the other login command.

### ADC writes require re-login with write scope

`GSC_ENABLE_WRITE=true` cannot modify scopes on an already issued ADC OAuth token. It is effective only for service accounts. If gsc-mcp detects ADC with that flag, it writes a warning to stderr; follow it instead of investigating further.

The only way to enable ADC writes is to log in again with the write scope:

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

Hermes has no repo-local MCP discovery. **Merge**, do not replace, this entry under `mcp_servers` in `~/.hermes/config.yaml`:

```yaml
mcp_servers:
  gsc:
    command: gsc-mcp
    supports_parallel_tool_calls: false
```

ADC needs no `env` block. Restart Hermes, then first ask it to call `list_sites`.

### VS Code and Copilot

Their configuration path varies. Do not guess it; print the JSON snippet for the user to paste into the location their client documents.

## Step 4: verify

### Terminal verification (ADC, no environment variables)

```bash
{ printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 2; } \
  | ~/.local/bin/gsc-mcp 2>/tmp/gsc-mcp-err.log
```

`sleep 2` is required: otherwise stdin can reach EOF too early and create a false zero-output failure.

Expected result: stdout contains JSON-RPC, `tools/list` contains six tools, and stderr does not contain `no credentials`.

Then test from the agent:

> Use gsc `list_sites` to list my Search Console properties.

An empty list means the account has no GSC properties, the API is disabled, or the ADC login lacks the webmasters scope. Re-run the login if needed.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `no credentials found; checked: ...` | ADC is not logged in; run `application-default login`. |
| `list_sites` returns 403 / auth error | Scope is insufficient or API is disabled; rerun login with `--scopes`. |
| Empty property list | The Google account has no GSC property or is the wrong account. |
| `GSC_ENABLE_WRITE` still cannot submit | It has no effect for ADC; log in again with the `webmasters` scope. |
| GUI does not show the tool | Configuration was not merged or the app was not fully restarted. |
| Refresh suddenly fails | Token expired; rerun login. |

## Security rules

- Never print ADC or service-account JSON contents.
- Never overwrite an entire MCP config file; always merge and back up.
- Never automatically run `gcloud auth login` or `application-default login`.
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

| Variable | Purpose |
|---|---|
| None | ADC requires no credential environment variable. |
| `GOOGLE_APPLICATION_CREDENTIALS` | Override the ADC credential-file path. |
| `GOOGLE_SERVICE_ACCOUNT_FILE` | Service-account key file. |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | Inline JSON for CI. |
| `GSC_LOG_LEVEL` | `warn` is a useful default. |
| `GSC_ENABLE_WRITE` | Upgrades scope only for service accounts. |
| `GSC_ALLOW_DESTRUCTIVE` | Permits delete. |

See [README.md](README.md) and [SPEC.md](SPEC.md) section 4 for full details.
