# AGENTS.md — gsc-mcp

For an AI agent working with this repository. This is a local Google Search
Console MCP server: one Go binary over stdio, using the user's own Google
account to query Search Console properties they are already allowed to access.

Choose the right path first:

| Task | Read |
|---|---|
| Help a user install the server | The **Install** section here, then [INSTALL.md](INSTALL.md) for the complete procedure. |
| Analyse data after it is installed | The **Use** section here. |

---

## Install

### Fast path

```bash
curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
```

```powershell
irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
```

Do not jump directly from binary installation to OAuth. First establish these
facts without reading credential contents:

1. Which MCP client the user wants to configure.
2. Whether `gcloud --version` works. If not, follow [INSTALL.md](INSTALL.md)
   step 1a before continuing.
3. The real GCP project ID where the Search Console API is enabled. If the user
   has no project, direct them to https://console.cloud.google.com/projectcreate,
   then to https://console.cloud.google.com/apis/library/searchconsole.googleapis.com.
4. Whether the same Google account can see at least one property at
   https://search.google.com/search-console. An empty account is not an MCP
   installation problem.

### Browser consent checkpoint: agent owns the terminal

The agent must execute every shell / PowerShell command. Never tell a novice
user to open Terminal, copy a command, or paste terminal output back into chat.
Before starting OAuth, tell the user that a Google sign-in page will open and
that their only task is to choose an account and approve access. Then run this
command yourself through the terminal tool with enough time for browser
consent:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
```

Keep the command running while the user completes Google's browser UI. The
human owns the consent click; the agent owns command entry, process management,
and every follow-up command. When the command exits successfully, confirm the
credential file exists with `ls`, never `cat`, then run the following commands
yourself:

```bash
gcloud auth application-default set-quota-project YOUR_PROJECT_ID
"$HOME/.local/bin/gsc-mcp" doctor
"$HOME/.local/bin/gsc-mcp" setup
```

On Windows, use the absolute path printed by `install.ps1`. On POSIX, the
absolute path above avoids a false `command not found` when `~/.local/bin` is
not yet on `PATH`. Replace `YOUR_PROJECT_ID`; never send the placeholder.

`doctor` must print `list_sites OK` before configuration is considered usable.
After `setup`, finish the selected client's configuration, restart that client
or open a new session, and verify that its MCP server list contains `gsc`.
Only then ask the agent to call `list_sites`. See [INSTALL.md](INSTALL.md) for
the full client-specific procedure.

For Codex, do not use the repository `.mcp.json`; Codex MCP configuration lives
in `~/.codex/config.toml` or a trusted project's `.codex/config.toml`. The safe
CLI path is:

```bash
codex mcp list
# Only when the first list does not contain gsc:
codex mcp add gsc -- "$HOME/.local/bin/gsc-mcp"
codex mcp list
```

The current Codex session will not gain the new tool. Restart Codex or start a
new session after `gsc` appears in the list.

### Six common mistakes

1. **Sending the user to Terminal for OAuth.** The agent starts the login
   command; the user interacts only with the Google page that opens.
2. **Skipping `set-quota-project`.** ADC belongs to a personal account, not a
   GCP project; without this command every query returns 403.
3. **Downloading a Windows executable manually.** Use `install.ps1`: a manual
   download skips SHA-256 verification and SmartScreen unblocking and often
   lands outside PATH.
4. **Printing credential contents.** Use `ls` only to confirm a file exists;
   never use `cat` or paste credentials into chat.
5. **Stopping after printing an OAuth command.** Keep the terminal process
   alive, wait for browser consent, then continue automatically from
   quota-project setup.
6. **Assuming a running client hot-loads MCP configuration.** Verify the server
   list, then restart or open a new session before calling `list_sites`.

### Use `gsc-mcp doctor`, not hand-written JSON-RPC, for verification

```bash
gsc-mcp doctor
```

It checks the full environment and performs one real `list_sites` call without
writing files. On failure, it prints the corrective command.

`gsc-mcp setup --dry-run` is **not** a credential check: it skips API calls.

---

## Use

### Six tools

| Tool | Purpose | Key constraint |
|---|---|---|
| `list_sites` | List accessible properties | No arguments. Call it first if the property format is unclear. |
| `get_site` | Get a property's permission level | — |
| `query_search_analytics` | Core clicks / impressions / CTR / position query | `row_limit` defaults to **150**, maximum 25,000. |
| `compare_periods` | Compare two periods and calculate deltas | Periods must have equal length; otherwise `invalid_input`. Default `row_limit` is **100**. |
| `inspect_url` | Inspect Google's indexed state for URLs | 1–10 URLs per call, issued serially. |
| `manage_sitemaps` | List / get / submit / delete sitemaps | `submit` and `delete` are disabled by default. |

### Flexible `site_url` input

The server normalizes input; do not ask the user to pre-format it.

| Input | Sent as |
|---|---|
| `example.com` | `sc-domain:example.com` |
| `https://example.com` | `sc-domain:example.com` |
| `https://example.com/` | `https://example.com/` — trailing slash explicitly means URL-prefix. |
| `https://example.com/blog` | `https://example.com/blog/` |

If a guessed property receives 403, the server calls `sites.list`, finds a
matching property, and retries once. If none is usable, its error lists every
accessible property; show that list to the user rather than guessing.

### Four output traps

- Always read `truncated` and `scan_capped`. `truncated=true` means the result
  is the first N rows of a larger set. `scan_capped=true` means even the scan
  was incomplete, so top-N results can be missing candidates.
- The API returns top rows only. Their sum does not equal the Search Console UI
  total.
- Average position is a weighted average of impressions, not a literal rank.
- Every `compare_periods` delta is **B minus A**. `ctr_a` and `ctr_b` are 0–1
  fractions; `ctr_delta_pp` is percentage points. Negative `position_change`
  means improvement. Keys that occur in only one period use `only_in` and omit
  `position_change`; never treat an omitted field as zero.

### Parameters that silently give the wrong answer

- To exclude brand terms, use `excludingRegex` in
  `dimension_filter_groups`. It uses RE2 and rejects lookarounds and
  backreferences.
- To rank by impressions, CTR, or position, set `sort_by`. GSC itself only
  orders by clicks. The server scans up to 25,000 rows and sorts locally when
  `sort_by` is set.

  | Tool | `sort_by` values | Default direction |
  |---|---|---|
  | `query_search_analytics` | `clicks` (default), `impressions`, `ctr`, `position` | First three descending; `position` ascending. |
  | `compare_periods` | `clicks_delta` (default), `impressions_delta`, `ctr_delta_pp`, `position_change` | Deltas descending; `position_change` ascending. |

  For the biggest ranking improvements, call `compare_periods` with
  `sort_by=position_change`.
- If `dimensions` contains `hour`, use `data_state=hourly_all` and a
  `start_date` within the most recent 10 days.
- `data_state` defaults to `all`, matching the GSC UI including provisional
  data. Use `final` only when the analysis requires finalized data.
- Dates are in Pacific Time. Data is delayed 2–4 days and retained for about
  16 months; an empty result from the newest days usually means data is not yet
  available.

### Writes are disabled by default

| Action | Requirement |
|---|---|
| `list` / `get` | Always available |
| `submit` | `GSC_ENABLE_WRITE=true` |
| `delete` | `GSC_ENABLE_WRITE=true` **and** `GSC_ALLOW_DESTRUCTIVE=true` |

The tool remains visible in `tools/list` when flags are absent, but write calls
return `write_disabled` without calling the API.

**ADC note:** `GSC_ENABLE_WRITE=true` is still required as the local write
gate, even after re-login with write scope. The flag cannot add scopes to an
existing ADC token, so writes also need a token issued with `webmasters`, not
`webmasters.readonly`.

### Included skills

Four skills live under `.agents/skills/`:

| Skill | Typical request |
|---|---|
| `nonbrand-performance` | “Who finds me without searching for my company name?” |
| `monthly-report` | “How did the site perform this month?” |
| `index-health` | “Has Google seen my new pages?” |
| `gsc-recipes` | Any other question: routes the question to an exact tool call. |

## Native agent configuration

This repository places each agent's configuration in the location that agent
recognizes. Do not duplicate these instructions or add credentials to the repo.

| Agent | Automatically read content |
|---|---|
| Claude Code | `.mcp.json`, `.claude-plugin/`, and the plugin's `.agents/skills/` |
| Codex | This root `AGENTS.md` and `.agents/skills/`; MCP itself must be added through Codex `config.toml` or `codex mcp add` |
| Cursor | `.cursor/mcp.json` and `.cursor/rules/gsc-mcp.mdc` |
| Hermes | Run `bash .hermes/setup.sh`; it uses `hermes mcp add/test` to update the active profile, then requires a new session |

For any agent, the first real query should be `list_sites` when the property is
not unambiguous. Never guess a property or read or print credentials.

---

## Troubleshooting

Diagnose the symptom first instead of inferring a cause.

| Symptom | Actual cause | Do not | Fix |
|---|---|---|---|
| 403 containing `requires a quota project` | ADC has no quota project | Ask the user to add a GSC user | `gcloud auth application-default set-quota-project YOUR_PROJECT_ID` |
| `auth_failed` containing `cannot fetch token` or `invalid_grant` | Refresh token expired | Reinstall the binary | Re-run `application-default login` |
| `gcloud projects list` says credentials expired immediately after ADC login | It uses a separate token | Re-run `application-default login` | Run `gcloud auth login` |
| Windows says `gcloud` is not recognized | Installed but absent from PATH | Re-run `winget install` | Use `%LOCALAPPDATA%\Google\Cloud SDK\google-cloud-sdk\bin\gcloud.cmd` |
| `submit` / `delete` rejected | Local write gate closed, or ADC token is still readonly | Skip `GSC_ENABLE_WRITE=true` after re-login | Set `GSC_ENABLE_WRITE=true` **and** re-login with `webmasters` if using ADC |
| A query is empty despite known traffic | Date range is in the 2–4 day lag or outside retention | Diagnose permissions | Move `end_date` back several days |

`gcloud auth login` and `gcloud auth application-default login` store separate
credentials. The first serves the `gcloud` CLI; the second serves `gsc-mcp`.

The fixed error codes are `invalid_input`, `auth_failed`, `permission_denied`,
`not_found`, `quota_exceeded`, `upstream_error`, and `write_disabled`.

Start diagnosis with `gsc-mcp doctor`. To see which credential source was
selected (for example, a forgotten `GOOGLE_APPLICATION_CREDENTIALS` overriding
ADC), use:

```bash
GSC_LOG_LEVEL=debug gsc-mcp
```

---

## Deliberate exclusions

State that the server does not support these requests; do not approximate them
with another tool:

- Requesting indexing: Google's Indexing API is limited to JobPosting and
  BroadcastEvent.
- Adding or deleting properties: adding still requires UI verification and
  deletion is too risky.
- Coverage, Core Web Vitals, and link reports: the official API has no such
  endpoints.
- Listing all indexed URLs: `inspect_url` can inspect only known URLs, up to 10
  at a time.
- Testing the live version of a page: `inspect_url` reports Google's indexed
  version, which can lag the live page.

## Hard boundaries

- Never print credential contents. Use `ls`, not `cat`, to verify a file.
- Errors can include client domains and brand terms; review them before sharing.
- Never test writes on a real client property. `submit` and `delete` have real
  effects.
- stdout must contain MCP JSON-RPC only. All logs go to stderr.
