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

Then print the following commands for the user to run themselves. They open a
browser:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
gcloud auth application-default set-quota-project YOUR_PROJECT_ID
gsc-mcp setup
```

Run `gsc-mcp doctor` after installation. See [INSTALL.md](INSTALL.md) for the
full procedure.

### Four common mistakes

1. **Running browser-based login for the user.** Print the command and wait for
   the user to report completion.
2. **Skipping `set-quota-project`.** ADC belongs to a personal account, not a
   GCP project; without this command every query returns 403.
3. **Downloading a Windows executable manually.** Use `install.ps1`: a manual
   download skips SHA-256 verification and SmartScreen unblocking and often
   lands outside PATH.
4. **Printing credential contents.** Use `ls` only to confirm a file exists;
   never use `cat` or paste credentials into chat.

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

**ADC note:** `GSC_ENABLE_WRITE=true` cannot add scopes to an existing ADC
token. To write, re-run login using `webmasters`, not `webmasters.readonly`.

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
| Codex | This root `AGENTS.md` and `.agents/skills/` |
| Cursor | `.cursor/mcp.json` and `.cursor/rules/gsc-mcp.mdc` |
| Hermes | Hermes reads only the user's `~/.hermes/config.yaml`; merge the snippet in [INSTALL.md](INSTALL.md). It has no automatically discovered repo-local directory. |

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
| `submit` / `delete` rejected | Write flag absent or ADC scope is read-only | Set `GSC_ENABLE_WRITE=true` for ADC | See **Writes are disabled by default** |
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
