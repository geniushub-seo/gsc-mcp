<div align="center">

# gsc-mcp

### A local MCP server for Google Search Console — one Go binary, no runtime to install

[![ci](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/geniushub-seo/gsc-mcp/actions/workflows/ci.yml)
[![GitHub release](https://img.shields.io/github/v/release/geniushub-seo/gsc-mcp)](https://github.com/geniushub-seo/gsc-mcp/releases)

English | [繁體中文](README_ZH_TW.md) | [简体中文](README_ZH_CN.md)

</div>

Built for SEO and content teams. Sign in once with your own Google account (ADC), and your local agent — Claude Code, Codex, Cursor — can query the Search Console data you already have access to. You do **not** need to add a service account to every property.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gsc-mcp setup
```

`install.sh` downloads the binary for your platform, verifies its SHA-256, and installs it to `~/.local/bin`. The other two lines you run yourself — they open a browser and write your MCP config. See [INSTALL.md](INSTALL.md) and [Releases](https://github.com/geniushub-seo/gsc-mcp/releases).

## Getting started with ADC (recommended)

Sign in once with your own Google account. Your MCP config needs **no** credential env vars at all.

Easiest path: run the three lines above. Or hand the repo to an AI agent and say: install this for me, follow [INSTALL.md](INSTALL.md).

### Doing it by hand (after the binary is installed)

1. **Install gcloud** (once):
   - macOS: `brew install --cask google-cloud-sdk`
   - Linux: `curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-latest-linux-x86_64.tar.gz && tar -xf google-cloud-cli-latest-linux-x86_64.tar.gz && ./google-cloud-sdk/install.sh` (or your distro's `google-cloud-sdk` package)
   - Windows: `winget install Google.CloudSDK`, or download [GoogleCloudSDKInstaller.exe](https://dl.google.com/dl/cloudsdk/channels/rapid/GoogleCloudSDKInstaller.exe)
   - **Warning:** do not just unpack the tarball without running `install.sh`. The bundled Python never gets installed, the launcher falls back to your system `python3` (macOS ships 3.9; gcloud needs 3.10–3.14), and the resulting error looks like a bug in this project. That 713 MB is the Google Cloud SDK, not gsc-mcp.
2. **Sign in with ADC** (opens a browser) — the second command above.
3. **Enable the Search Console API** if you haven't:
   https://console.cloud.google.com/apis/library/searchconsole.googleapis.com
4. **Run `gsc-mcp setup`** — merges your MCP config and test-calls `list_sites`.
5. Call `list_sites` from your agent to confirm which properties you can see.

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

### Writes (sitemap submit/delete) under ADC

**`GSC_ENABLE_WRITE=true` does nothing under ADC.** An ADC token's OAuth scopes are fixed at `gcloud auth application-default login` time and cannot be widened afterwards. To write, sign in again with:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters,https://www.googleapis.com/auth/cloud-platform
```

(`webmasters` is read-write; use `webmasters.readonly` for read-only.)

## Environment variables

| Variable | Default | What it does |
|---|---|---|
| `GOOGLE_APPLICATION_CREDENTIALS` | — | Official ADC path override (precedence 2) |
| `GOOGLE_SERVICE_ACCOUNT_FILE` | — | Path to a service account key (precedence 3) |
| `GOOGLE_SERVICE_ACCOUNT_JSON` | — | Inline JSON (precedence 4, handy in CI) |
| `GSC_ENABLE_WRITE` | `false` | **Service account only**: upgrades the scope to `webmasters` and allows `submit`. Warns and does nothing under ADC |
| `GSC_ALLOW_DESTRUCTIVE` | `false` | Allows `delete`; requires `GSC_ENABLE_WRITE=true` as well (service account) |
| `GSC_REQUEST_TIMEOUT` | `30s` | Timeout for a single API call |
| `GSC_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

Credential precedence: `--credentials-file` (alias `--service-account-file`) → `GOOGLE_APPLICATION_CREDENTIALS` → `GOOGLE_SERVICE_ACCOUNT_FILE` → `GOOGLE_SERVICE_ACCOUNT_JSON` → `.env` → **the default ADC path**.

### macOS Gatekeeper

`install.sh` tries to remove the quarantine flag. If you downloaded the binary manually through a browser you may still see "cannot verify the developer": right-click the file in Finder → Open → Open. This project is not Apple code-signed.

## Advanced: service account (headless / CI)

For non-human identities, CI, or anywhere a personal Google login isn't possible.

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

## Skills (analyses that work out of the box)

Four skills ship with the server, so you don't have to invent the prompt yourself:

| Skill | What a user would ask | What it does |
|---|---|---|
| `nonbrand-performance` | "Who finds me *without* searching my company name?" | Splits branded from non-branded traffic using regex query filters (`excludingRegex`), which catches every spelling variant of your brand |
| `monthly-report` | "How did the site do this month?" | A monthly report with the branded/non-branded split built in |
| `index-health` | "Has Google seen my new pages yet?" | Sitemap status, per-URL diagnosis, and canonical-conflict detection |
| `gsc-recipes` | Anything else | A parameter routing table: question → the exact tool call |

Claude Code loads `skills/` automatically. Other clients can use the contents of each `SKILL.md` as a prompt.

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

Built by **[Genius Hub](https://geniushub.cc/)** · Licensed under MIT

</div>
