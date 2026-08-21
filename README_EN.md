<div align="center">

# gsc-mcp

### A local MCP server for Google Search Console — one Go binary, no runtime to install

[![ci](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/geniushub-seo/gsc-mcp)](https://github.com/geniushub-seo/gsc-mcp/releases)

[繁體中文](README.md) | English | [简体中文](README_ZH_CN.md)

<p align="center">
  <a href="#install">Install</a> ·
  <a href="#tools">Tools</a> ·
  <a href="SPEC.md">Spec</a> ·
  <a href="https://geniushub.cc/seo/">SEO / GEO services</a> ·
  <a href="https://www.youtube.com/@super-simple-marketing">YouTube</a>
</p>

</div>

Built for SEO and content teams. Sign in once with your own Google account (ADC), and your local agent — Claude Code, Codex, Cursor, or Hermes — can query the Search Console data you already have access to. You do **not** need to add a service account to every property.

## Install

### Before you start

The binary is only one part of the setup. You also need:

- A Google account that can already see at least one property in
  [Search Console](https://search.google.com/search-console). If it cannot,
  ask the property owner for access before expecting `list_sites` to return a
  usable site.
- A GCP project where you can enable the Search Console API. If you do not have
  one, [create a project](https://console.cloud.google.com/projectcreate), copy
  its **project ID** (not its display name), then enable the
  [Search Console API](https://console.cloud.google.com/apis/library/searchconsole.googleapis.com)
  in that project.
- The Google Cloud CLI. Check with `gcloud --version`; if it is missing, install
  it using the platform instructions under **Doing it by hand** below.
- The MCP client you intend to use. Adding an MCP server does not inject tools
  into an already-running session; restart the client or start a new session
  after configuration.

The browser sign-in is an intentional human-in-the-loop checkpoint, but it is
**not a terminal handoff**. During an agent-assisted installation, the agent
executes every terminal command, including starting gcloud login and keeping it
running. The user only chooses a Google account and approves access in the
browser page that opens; the agent then continues automatically.

The command blocks below are manual-installation references. When an AI agent
is helping, the agent runs them; it must not ask a novice user to open Terminal
or copy shell commands.

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
gcloud auth login
gcloud projects list
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project PROJECT_ID
gcloud services enable searchconsole.googleapis.com --project=PROJECT_ID
"$HOME/.local/bin/gsc-mcp" doctor
"$HOME/.local/bin/gsc-mcp" setup
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
gcloud auth login
gcloud projects list
gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project PROJECT_ID
gcloud services enable searchconsole.googleapis.com --project=PROJECT_ID
$GscMcp = Join-Path $env:LOCALAPPDATA 'Programs\gsc-mcp\gsc-mcp.exe'
& $GscMcp doctor
& $GscMcp setup
```

The install script downloads the binary for your platform, verifies its SHA-256, and installs it to `~/.local/bin` (`%LOCALAPPDATA%\Programs\gsc-mcp` on Windows), clearing the macOS quarantine flag or the Windows SmartScreen block marker. In an agent-assisted flow, the agent starts ADC login and the user only handles Google's browser consent; `doctor` verifies credentials without writing files; `setup` writes only the client configurations it explicitly reports. See [INSTALL.md](INSTALL.md) and [Releases](https://github.com/geniushub-seo/gsc-mcp/releases).

The commands above use the installed binary's absolute path so they still work
when the install directory is not on `PATH`. If `command -v gsc-mcp` (or
`Get-Command gsc-mcp` on Windows) succeeds, the shorter `gsc-mcp` command is
also fine.

Replace `PROJECT_ID` with an id from the `gcloud projects list` output above; never run the literal placeholder. **Neither of those two lines is optional.** ADC is your personal account and belongs to no project, so without a quota project every query fails with a 403 that reads exactly like a permissions problem and isn't one — and a project that has never enabled the Search Console API returns the same 403 with `"reason": "SERVICE_DISABLED"` in its details, which is what `gcloud services enable` clears. `gcloud auth login` is a separate login from the ADC one: without it, `gcloud projects list` reports expired credentials.

If any step misbehaves, run the installed binary with `doctor` (for example,
`"$HOME/.local/bin/gsc-mcp" doctor`): every check plus one real `list_sites`
call, and it writes no files.

### Handoff and next steps

| State | Who acts | Completion evidence | Next step |
|---|---|---|---|
| Prerequisites | Agent; user only supplies choices it cannot infer | `gcloud --version` works; a real project ID is known; the Google account has GSC access | Agent installs the binary |
| Binary installed | Agent | The absolute binary path runs | Agent starts ADC login |
| Browser authorization | Agent runs the process; **user only clicks in Google UI** | The browser flow finishes and ADC is saved | Agent sets the quota project |
| Credentials ready | Agent | `doctor` prints `list_sites OK` and a property count | Agent configures the chosen MCP client |
| Client configured | Agent | The client lists a `gsc` MCP server | Restart the client or start a new session |
| Tools loaded | Agent | `list_sites` is visible and returns properties | Begin GSC analysis |

If a step fails, stay at that row and apply the corrective instruction from
`doctor`; do not report later rows as complete.

## Getting started with ADC (recommended)

Sign in once with your own Google account. Your MCP config needs **no** credential env vars at all.

Easiest manual path: run the lines above. With an AI agent, say: install this
for me and follow [INSTALL.md](INSTALL.md). The agent owns every terminal
command. Your only OAuth task is approving access in the Google browser page it
opens.

### Doing it by hand (after the binary is installed)

1. **Install gcloud** (once):
   - macOS with homebrew: `brew install --cask google-cloud-sdk`
   - macOS without homebrew: **do not install homebrew for this.** Its installer requires a local Administrator account and stops at `Need sudo access on macOS`, which is common on managed laptops. See *No-homebrew macOS* below.
   - Linux: `curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-latest-linux-x86_64.tar.gz && tar -xf google-cloud-cli-latest-linux-x86_64.tar.gz && ./google-cloud-sdk/install.sh` (or your distro's `google-cloud-sdk` package)
   - Windows: `winget install Google.CloudSDK`, or download [GoogleCloudSDKInstaller.exe](https://dl.google.com/dl/cloudsdk/channels/rapid/GoogleCloudSDKInstaller.exe)
   - That 713 MB is the Google Cloud SDK, not gsc-mcp.

   **No-homebrew macOS** — gcloud is written in Python and needs 3.10+. macOS ships `/usr/bin/python3` 3.9.6, and `install.sh` is itself a Python program, so running it does not fix this — it aborts with `TypeError: unsupported operand type(s) for |`. There is no bundled-python tarball for macOS. Provide a Python first:
   ```bash
   # 1. locate a python3 >= 3.10
   for p in python3.13 python3.12 python3.11 python3.10 "$HOME"/.local/share/uv/python/*/bin/python3; do
     c=$(command -v "$p" 2>/dev/null) || { [ -x "$p" ] && c="$p"; } || continue
     "$c" -c 'import sys;sys.exit(0 if sys.version_info>=(3,10) else 1)' 2>/dev/null && { echo "FOUND: $c"; break; }
   done

   # 2. none found: install one with uv (no Administrator rights needed), then rerun step 1
   curl -LsSf https://astral.sh/uv/install.sh | sh
   "$HOME/.local/bin/uv" python install 3.12

   # 3. install gcloud with it (Intel Mac: replace darwin-arm with darwin-x86_64)
   export CLOUDSDK_PYTHON=<path from step 1>
   tmp=$(mktemp -d)
   curl -fsSL https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-darwin-arm.tar.gz -o "$tmp/gcloud.tar.gz"
   mkdir -p "$HOME/.local/share" && tar -xzf "$tmp/gcloud.tar.gz" -C "$HOME/.local/share"
   "$HOME/.local/share/google-cloud-sdk/install.sh" --quiet --path-update=false
   ```

   Installed this way, **every gcloud command below must carry `CLOUDSDK_PYTHON`** (export it in your shell profile, or prefix each command). A homebrew install does not need it.
2. **Create or select a GCP project.** If none exists, use the [project creation page](https://console.cloud.google.com/projectcreate). Copy the project ID, not the display name. You need permission to enable APIs and use that project for quota; otherwise ask its administrator.
3. **Enable the Search Console API** on that project — `gcloud services enable searchconsole.googleapis.com --project=PROJECT_ID`, or hit Enable in the [API Library](https://console.cloud.google.com/apis/library/searchconsole.googleapis.com) after checking the project picker top-left. A newly created project never has it on.
4. **Confirm GSC access.** Open [Search Console](https://search.google.com/search-console) with the same Google account and confirm that at least one property is visible.
5. **Sign in with ADC** (opens a browser) — during agent-assisted setup, the agent runs the login command and waits; the user only completes Google's browser consent.
6. **Set the quota project** — the agent replaces `PROJECT_ID` with the ID from step 2 and runs the command. Required under ADC; skipping it means a 403 on every query.
7. **Run `gsc-mcp doctor`** — the agent runs it and requires an explicit `list_sites OK` line. Do not infer success only from the presence of the credential file.
8. **Run `gsc-mcp setup`** — the agent runs it, then finishes the client-specific step below. `setup` can merge Claude Desktop, Cursor, and an existing project `.mcp.json`; it prints manual instructions for clients it does not configure directly.
9. Restart the MCP client or start a new session, then call `list_sites` from the agent.

If something breaks, run `gsc-mcp doctor`. It runs every check plus one real `list_sites` call, **writes no files**, and prints the specific fix for what it finds — a missing quota project, an expired token, and a disabled API each get their own instructions. `setup --dry-run` skips the API call entirely, so it cannot tell you whether your credentials work; don't use it to diagnose.

### What the MCP config looks like (ADC: just a command)

```json
{
  "mcpServers": {
    "gsc": {
      "command": "/Users/YOUR_USER/.local/bin/gsc-mcp"
    }
  }
}
```

Credentials are read automatically from `~/.config/gcloud/application_default_credentials.json`.

### When your ADC refresh token expires

It will, eventually — after a password change, a long idle period, or an org policy kicking in. Sign in again; nothing is broken:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
```

### Writes (sitemap submit/delete)

Two independent gates. Both are required for ADC:

1. Set `GSC_ENABLE_WRITE=true`. This is a local gsc-mcp gate and applies to **every** credential type, including ADC. Re-logging with write scope is not enough on its own.
2. ADC tokens also need the `webmasters` scope at login time (scopes cannot be widened later):

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters,https://www.googleapis.com/auth/cloud-platform
```

(`webmasters` is read-write; use `webmasters.readonly` for read-only.)

## Environment variables

ADC needs no credential variable at all — install it as described above and the
binary reads `~/.config/gcloud/application_default_credentials.json` on its own.
Everything below is optional.

| Variable | Default | What it does |
|---|---|---|
| `GOOGLE_APPLICATION_CREDENTIALS` | — | Override the ADC credential-file path; unnecessary when the default path works |
| `GSC_ENABLE_WRITE` | `false` | Local write gate for `submit`. Under ADC the token must also carry `webmasters` scope (see [Writes](#writes-sitemap-submitdelete)) |
| `GSC_ALLOW_DESTRUCTIVE` | `false` | Allows `delete`; requires `GSC_ENABLE_WRITE=true` as well |
| `GSC_REQUEST_TIMEOUT` | `30s` | Timeout for a single API call |
| `GSC_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

Service-account variables and the full credential precedence live in
**Advanced: service account (headless / CI)** below; a normal install does not need them.

### macOS Gatekeeper

`install.sh` tries to remove the quarantine flag. If you downloaded the binary manually through a browser you may still see "cannot verify the developer": right-click the file in Finder → Open → Open. This project is not Apple code-signed.

## Advanced: service account (headless / CI)

**Most users do not need this section.** Take this path only when one of the following
holds: you already have a service-account JSON key, this runs in CI or on an unattended
machine, no browser can be opened on the target machine, or a non-human identity is
required. Otherwise use ADC above — it never asks you to add a user to a property.

The cost, stated up front: a service account is a machine identity that GSC does not
recognise on its own, so its `client_email` must be added as a user on **every** property
you want to query. What you get back is simpler writes — `GSC_ENABLE_WRITE=true` upgrades
the scope to `webmasters` directly, with no repeat of the login flow ADC requires.

1. Enable the Search Console API in GCP → create a service account → download the JSON key.
2. Add its `client_email` as a user on **every** property in Search Console.
3. Add the env var to your MCP config:

```json
{
  "mcpServers": {
    "gsc": {
      "command": "/Users/YOUR_USER/bin/gsc-mcp",
      "env": {
        "GOOGLE_SERVICE_ACCOUNT_FILE": "/Users/YOUR_USER/.config/gsc-mcp/service-account.json"
      }
    }
  }
}
```

```bash
mkdir -p ~/.config/gsc-mcp && chmod 700 ~/.config/gsc-mcp
chmod 600 ~/.config/gsc-mcp/service-account.json
```

### Service-account variables and credential precedence

| Variable | What it does |
|---|---|
| `GOOGLE_SERVICE_ACCOUNT_FILE` | Path to the key file |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | Inline JSON, for CI systems that store secrets as strings |

Full precedence — the first five layers are all explicit, and only when none is set
does the binary fall back to ADC: `--credentials-file` (alias `--service-account-file`)
→ `GOOGLE_APPLICATION_CREDENTIALS` → `GOOGLE_SERVICE_ACCOUNT_FILE`
→ `GOOGLE_SERVICE_ACCOUNT_JSON` → `.env` → **the default ADC path**.
See [SPEC.md](SPEC.md) section 4.2 for the authoritative definition.

## Agent-native onboarding

Clone the repo when you want an agent to get its own project configuration and guidance. No config here contains credentials or an absolute path.

| Agent | Native repo files | What to do |
|---|---|---|
| Claude Code | `.mcp.json`, `.claude-plugin/`, `.agents/skills/` | Open the repo or install the plugin. |
| Codex | `AGENTS.md`, `.agents/skills/`; MCP config is separate | Run `codex mcp add` below, then restart Codex or start a new session. |
| Cursor | `.cursor/mcp.json`, `.cursor/rules/gsc-mcp.mdc` | Open the repo as a project. |
| Hermes | [`.hermes/`](.hermes/) onboarding bundle | Run `bash .hermes/setup.sh`, then start a new Hermes session. |

For every client, the assisting agent runs `gsc-mcp doctor`, then calls
`list_sites`. Never put an OAuth token or service-account key in these files.

For Codex, `.mcp.json` is not the MCP configuration file. Codex uses
`~/.codex/config.toml`, a trusted project's `.codex/config.toml`, or the CLI:

```bash
codex mcp list
# Only when the first list does not contain gsc:
codex mcp add gsc -- "$HOME/.local/bin/gsc-mcp"
codex mcp list
```

After the second `list` shows `gsc`, restart Codex or open a new session before
asking it to call `list_sites`.

## Skills (analyses that work out of the box)

Four skills ship with the server, so you don't have to invent the prompt yourself:

| Skill | What a user would ask | What it does |
|---|---|---|
| `nonbrand-performance` | "Who finds me *without* searching my company name?" | Splits branded from non-branded traffic using regex query filters (`excludingRegex`), which catches every spelling variant of your brand |
| `monthly-report` | "How did the site do this month?" | A monthly report with the branded/non-branded split built in |
| `index-health` | "Has Google seen my new pages yet?" | Sitemap status, per-URL diagnosis, and canonical-conflict detection |
| `gsc-recipes` | Anything else | A parameter routing table: question → the exact tool call |

Claude Code's plugin and Codex discover the shared skills in `.agents/skills/`. Cursor receives the same safe defaults from its project rule; its agent can read a `SKILL.md` when a specific analysis is requested.

## Tools

| Tool | Underlying API | Writes | Notes |
|---|---|---|---|
| `list_sites` | `sites.list` | — | Every property you're authorized for — the multi-client entry point |
| `get_site` | `sites.get` | — | `permissionLevel` for one property |
| `query_search_analytics` | `searchanalytics.query` | — | The core query, full parameter pass-through |
| `compare_periods` | `searchanalytics.query` ×2 | — | Server-side wrapper: two periods plus deltas |
| `inspect_url` | `urlInspection.index.inspect` | — | 1–10 URLs per call, issued sequentially |
| `manage_sitemaps` | `sitemaps.list/get/submit/delete` | conditional | Four actions in one; submit/delete are flag-gated |

Six tools is deliberate. Overviews, by-page breakdowns and "advanced" analytics are all the *same* API call with different default parameters — shipping each as its own tool inflates the schema payload sent to the LLM on every request and raises the odds it picks the wrong one. The `gsc-recipes` routing table covers those cases instead, at zero context cost. A wrapper only earns its place when the LLM genuinely cannot do the work itself, such as computing a median baseline across 25,000 rows.

### Deliberately not implemented

- `sites.add` / `sites.delete` — add still requires verification in the UI, and delete is too easy to fire by accident.
- Indexing API — Google only opened it for `JobPosting` and `BroadcastEvent`.
- Coverage reports, Core Web Vitals, link reports — no official API exists.

## How the data behaves (all of this is in the tool descriptions the LLM reads)

- Dates are in **PT** (UTC−7/−8) — not UTC, not your local time.
- Data lags **2–4 days** and is retained for **16 months**. Recent dates can come back incomplete.
- The API returns top rows only and **does not guarantee completeness**. Sums will not match the totals in the web UI.
- `rowLimit` caps at **25,000** (the web UI gives you 1,000 at a time — this is the main reason to use the API). Page past it with `startRow`.
- The `HOUR` dimension requires `dataState: HOURLY_ALL` and only covers the last **10 days**.
- Filters only apply to `QUERY` / `PAGE` / `COUNTRY` / `DEVICE` / `SEARCH_APPEARANCE` — you **cannot** filter on `DATE` or `HOUR`.
- If you group or filter by `page`, `aggregationType` cannot be `BY_PROPERTY`.
- Average position is a weighted average, not the factual answer to "what position does this keyword rank at".

`site_url` accepts a bare domain (`example.com`), a full URL, or the canonical `sc-domain:` form — the server normalizes it. If the guess is wrong and Google returns 403, it looks up your accessible properties and retries once.

## Development

```bash
go build -trimpath -ldflags="-s -w -X main.version=dev" -o ./bin/gsc-mcp ./cmd/gsc-mcp
go test ./... && go vet ./... && golangci-lint run
```

## See also

- [INSTALL.md](INSTALL.md) — installation guide written for an AI agent to follow
- [SPEC.md](SPEC.md) — the frozen technical spec

---

<div align="center">

Built by **[Genius Hub](https://geniushub.cc/seo/)**, an SEO / GEO agency, and run
daily on our own client work · Licensed under MIT

How we think about SEO and GEO (getting AI answers to cite you) →
**[Super Simple Marketing](https://www.youtube.com/@super-simple-marketing)**

</div>
