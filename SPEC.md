# SPEC — gsc-mcp v1

Final technical specification.

## 1. Goals and non-goals

**Goals**

- One native binary with no runtime dependency for end users.
- MCP stdio transport (JSON-RPC over stdin/stdout).
- Service-account authentication for multi-client agencies, without an OAuth
  browser flow.
- Read-only by default; writes require explicit flags.
- Transport-independent business logic so HTTP can be added later.

**Non-goals in v1**

- HTTP or Streamable HTTP transport.
- Interactive user OAuth flow.
- `sites.add` / `sites.delete`.
- Indexing API.
- Reports the official API does not expose: coverage, CWV, and external links.

## 2. Official API coverage

| Service | Method | Exposed as v1 tool |
|---|---|---|
| Search Analytics | `searchanalytics.query` | Yes |
| Sitemaps | `sitemaps.list` | Yes |
| Sitemaps | `sitemaps.get` | Yes |
| Sitemaps | `sitemaps.submit` | Yes, flag-gated |
| Sitemaps | `sitemaps.delete` | Yes, double-flag-gated |
| Sites | `sites.list` | Yes |
| Sites | `sites.get` | Yes |
| Sites | `sites.add` | No |
| Sites | `sites.delete` | No |
| URL Inspection | `urlInspection.index.inspect` | Yes |

**Implementation detail:** `urlInspection` uses
`searchconsole.googleapis.com/v1`; the other services use
`www.googleapis.com/webmasters/v3`. The generated
`google.golang.org/api/searchconsole/v1` client handles this distinction; do not
build URLs manually.

## 3. Tool specification

### 3.0 Shared behaviour for all six tools

#### `site_url` normalization

Every tool with `site_url` calls `NormalizeSiteURL` first:

| Input | Output | Reason |
|---|---|---|
| `sc-domain:example.com` | unchanged | Already canonical. |
| `example.com` | `sc-domain:example.com` | A bare domain implies a Domain property. |
| `https://example.com` | `sc-domain:example.com` | A root URL without trailing slash implies a Domain property. |
| `https://example.com/` | `https://example.com/` | A trailing slash explicitly signals URL-prefix intent. |
| `https://example.com/blog` | `https://example.com/blog/` | A non-root path is URL-prefix; add trailing slash. |

Remove `www.` and ports while deriving the apex domain.

#### 403 property-discovery recovery

When the normalized request receives **403 only** (all other errors propagate):

1. Call `sites.list`.
2. Match the input apex domain and prefer `sc-domain:` over URL-prefix among
   candidates that have not failed with 403.
3. Exclude the URL that just failed (`ResolveSiteURL`'s `exclude`) and retry
   once with the resolved property when one exists.
4. Otherwise return `permission_denied` and list every accessible property in
   the message.

When a same-apex `sc-domain:` property lacks permission but a URL-prefix
property works, recovery must select the URL-prefix property rather than retry
the known-failing property.

Implement this with generic `WithResolvedSiteURL[T]`; all six tools share the
same path.

#### Stringified array argument repair

Use `srv.AddReceivingMiddleware` before SDK schema validation to repair clients
that encode arrays as strings. Maintain this map:

| Tool | Fields |
|---|---|
| `query_search_analytics` | `dimensions`, `dimension_filter_groups` |
| `compare_periods` | `dimensions`, `dimension_filter_groups` |
| `inspect_url` | `urls` |

Only rewrite a value if it is a JSON string whose own contents parse as a JSON
array. Let normal validation handle every other value.

#### Shared output fields

Every successful tool output includes `queried_at` in UTC RFC3339 and the
actual `site_url` sent after normalization or recovery, not the user's raw
input.

### 3.1 `query_search_analytics`

`POST searchAnalytics/query`

| Parameter | Type | Required | Default | Notes |
|---|---|---|---|---|
| `site_url` | string | Yes | — | Supports `sc-domain:`. |
| `start_date` / `end_date` | string | Yes | — | `YYYY-MM-DD`, Pacific Time. |
| `dimensions` | string[] | No | `[]` | `query`, `page`, `country`, `device`, `date`, `hour`, `searchAppearance`. |
| `search_type` | string | No | `web` | `web`, `image`, `video`, `news`, `discover`, `googleNews`. |
| `dimension_filter_groups` | object[] | No | — | Full pass-through; operators below. |
| `aggregation_type` | string | No | `auto` | `auto`, `byProperty`, `byPage`, `byNewsShowcasePanel`. |
| `row_limit` | int | No | `150` | **1–25,000**; state it explicitly for exports. |
| `start_row` | int | No | `0` | Pagination offset. Applied at Google only for native `clicks desc`. Any other `sort_by`/`sort_order` scans from row 0, sorts locally, then applies the offset. |
| `data_state` | string | No | **`all`** | `all`, `final`, `hourly_all`. |

Filter operators: `equals`, `notEquals`, `contains`, `notContains`,
`includingRegex`, `excludingRegex` (RE2 syntax).

**Constraints validated by the handler, not delegated to Google:**

- `row_limit` above 25,000 returns `invalid_input`.
- `hour` in `dimensions` without `data_state=hourly_all` returns
  `invalid_input`, explaining the 10-day HOUR limit.
- A filter `dimension` must be `query`, `page`, `country`, `device`, or
  `searchAppearance`; `date` and `hour` return `invalid_input`.
- Grouping or filtering by `page` forbids `aggregation_type=byProperty`.
- `start_date > end_date` returns `invalid_input`.

`data_state` defaults to `all` to match the GSC UI and avoid reports that users
cannot reconcile with it. The LLM can explicitly choose `final` for a strict
analysis.

**The description must state:**

- Dates use Pacific Time; data lags 2–4 days and is retained for 16 months.
- The API returns top rows only; sums are not guaranteed to match UI totals.
- `row_limit` is at most 25,000 (the UI gives 1,000 per page).
- Average position is a weighted average, not a literal ranking for a query.
- Non-brand example: `excludingRegex` for brand terms.

**Output example**

```json
{
  "site_url": "sc-domain:example.com",
  "start_date": "2026-07-01",
  "end_date": "2026-07-31",
  "dimensions": ["query", "page"],
  "row_count": 842,
  "rows": [
    { "keys": ["keyword", "https://example.com/page/"], "clicks": 42, "impressions": 1200, "ctr": 0.035, "position": 8.4 }
  ]
}
```

### 3.2 `compare_periods`

A server-side wrapper that calls `searchanalytics.query` twice; it is not an
official Google API.

Arguments: `site_url`, `period_a_start`, `period_a_end`, `period_b_start`,
`period_b_end`, `dimensions`, `search_type`, `dimension_filter_groups` (same
shape and operators as `query_search_analytics`; applied to both periods),
`row_limit` (default 100), `data_state`, `sort_by`, and `sort_order`.

Each row includes A/B clicks, impressions, CTR, and position plus absolute and
percentage deltas. Keys in only one period are retained with zero for the other
period and `only_in` set.

This exists because SEO reporting is frequent: asking an LLM to call twice and
calculate locally is token-expensive and error-prone.

### 3.3 `inspect_url`

`POST urlInspection/index:inspect`

| Parameter | Type | Required | Default |
|---|---|---|---|
| `site_url` | string | Yes | — |
| `urls` | string[] | Yes | — |
| `language_code` | string | No | `zh-TW` |

`urls` has length 1–10. Calls are **serial**, at least 100 ms apart; do not
parallelize them.

Return a compact result rather than the large raw response. From the generated
client, `UrlInspectionResult` has `indexStatusResult`,
`inspectionResultLink`, `mobileUsabilityResult`, `richResultsResult`, and
`ampResult`. The final three are optional and may be omitted by Google, so they
must be nil-checked.

Keep all `IndexStatusInspectionResult` fields: `verdict`, `coverageState`,
`indexingState`, `crawledAs`, `lastCrawlTime`, `pageFetchState`,
`robotsTxtState`, `googleCanonical`, `userCanonical`, `referringUrls[]`, and
`sitemap[]`. Add `inspectionResultLink` and verdict summaries for the three
optional sections. Do not return detailed `richResultsResult.detectedItems`.

The description links to the official limits page rather than hard-coding quota
numbers. It explicitly excludes live-page testing, indexing requests, all-site
indexed-URL enumeration, and equivalence to the UI indexing report.

### 3.4 `list_sites`

`GET sites`. No arguments. Return `siteUrl` and `permissionLevel`. It is the
multi-client entry tool; the description directs an LLM to use it first when
property format is uncertain.

**Its InputSchema must be explicit**, never inferred from `struct{}`:

```go
var listSitesInputSchema = json.RawMessage(
    `{"type":"object","properties":{},"required":[],"additionalProperties":false}`)
```

The inferred schema omits `properties`, `required`, and
`additionalProperties`. Strict clients such as Copilot CLI then reject the full
tool list, making every tool unavailable.

### 3.5 `get_site`

`GET sites/{siteUrl}`. Takes `site_url`; returns `permissionLevel`.

### 3.6 `manage_sitemaps`

One tool for four actions.

| Parameter | Type | Required | Notes |
|---|---|---|---|
| `site_url` | string | Yes | |
| `action` | string | Yes | `list` / `get` / `submit` / `delete` |
| `feedpath` | string | Conditional | Required for `get` / `submit` / `delete`. |
| `sitemap_index` | string | No | Used only by `list`. |

`list` / `get` return `path`, `lastSubmitted`, `lastDownloaded`, `isPending`,
`isSitemapsIndex`, `warnings`, `errors`, and `contents[]`.

Check flags **before** calling the API. Return `write_disabled` and state the
missing environment variable in the message.

## 4. Authentication

**Distribution model: users bring their own credentials.** This project does
not run a centralized OAuth app. External Google verification would require a
privacy policy, homepage, domain verification, demo video, and weeks of review;
testing mode requires a maintained tester list and refresh tokens expire after
seven days. Neither fits zero-maintenance distribution.

Two credential forms are both supported.

### 4.1 Credential types

| | `authorized_user` (ADC) | `service_account` |
|---|---|---|
| Positioning | **Recommended**, documented first | Advanced, documented later |
| Use case | General users and personal analysis | Headless / CI, non-human identity, future HTTP transport |
| Identity | The user's own Google account | Dedicated machine account |
| Acquisition | Browser login through `gcloud auth application-default login --scopes=...` | Create account in GCP Console and download JSON key |
| Add to GSC property | **No**; it already sees the user's permitted properties | Add `client_email` to every property |
| Prerequisite | gcloud CLI, once | None |
| Credential life | Refresh token can expire after password changes, inactivity, or policy | Key does not expire |
| Coupling risk | One person's account change can stop everything | None |

ADC removes the painful step of adding an email address to every property. That
is the only reason it is the recommended route.

### 4.2 Credential precedence

| # | Source | Notes |
|---|---|---|
| 1 | CLI `--credentials-file` | Explicit and highest priority. Keep `--service-account-file` as a compatibility alias. |
| 2 | `GOOGLE_APPLICATION_CREDENTIALS` | Google's standard ADC environment-variable name. |
| 3 | `GOOGLE_SERVICE_ACCOUNT_FILE` | Existing project name, retained for compatibility. |
| 4 | `GOOGLE_SERVICE_ACCOUNT_JSON` | Inline JSON for containers and CI. |
| 5 | `.env` file | Development convenience; reads any source variable above. |
| 6 | **Default ADC path** | `$HOME/.config/gcloud/application_default_credentials.json` (Windows: `%APPDATA%\gcloud\...`). ADC therefore needs no environment variable. |

After `gcloud auth application-default login`, an MCP config needs only
`command`, without an `env` block.

### 4.3 Type dispatch

Read `type` from credential JSON; never hard-code a type:

| `type` | `option.CredentialsType` |
|---|---|
| `service_account` | `option.ServiceAccount` |
| `authorized_user` | `option.AuthorizedUser` |
| Other | Return a clear error listing both supported types. |

Use `option.WithAuthCredentialsJSON(credType, json)`. Do **not** use deprecated
`option.WithCredentialsJSON`, which is marked as a security risk.

### 4.4 `quota_project_id`

ADC credentials have no implicit quota project. If credential JSON contains
`quota_project_id`, pass `option.WithQuotaProject(id)` or Google rejects the
request. Service-account credentials already carry a project.

### 4.5 Scopes differ by credential type

| Condition | `service_account` | `authorized_user` (ADC) |
|---|---|---|
| Default | `webmasters.readonly` | Set at login time |
| `GSC_ENABLE_WRITE=true` | Opens the local write gate **and** upgrades the requested scope to `webmasters` | Opens the local write gate only. ADC scopes stay whatever was issued at login. |

`GSC_ENABLE_WRITE=true` is the **local** write gate for every credential type.
Without it, `submit` / `delete` return `write_disabled` even if the token
already has the `webmasters` scope. Re-logging ADC with write scope does not
bypass this flag.

ADC scopes are fixed into the refresh token by `gcloud auth
application-default login`. Refresh grants cannot expand scopes, so a later
`option.WithScopes` call cannot enable writes. ADC writes therefore need both
the local flag **and** a token issued with
`https://www.googleapis.com/auth/webmasters`.

With ADC plus `GSC_ENABLE_WRITE=true`, write a `slog.Warn` at startup explaining
that the flag opened the local gate but ADC still needs a write-scoped token.
Do **not** claim the flag has no effect. Document this in `README.md` and
`INSTALL.md`, and test that the warning occurs.

ADC read access requires at least:

```
https://www.googleapis.com/auth/webmasters.readonly
```

### 4.6 Other authentication rules

`golang.org/x/oauth2` manages token caching and refresh. Do **not** implement
JWT signing or token exchange yourself.

When startup credentials are missing or invalid, print a clear stderr error
listing the sources checked and exit non-zero; do not enter the MCP loop.

## 5. Error model

Tool errors return `mcp.CallToolResult{IsError: true}` and this JSON body, **not
a Go error**:

- A Go error becomes a protocol error that an LLM cannot act on.
- A successful result without `IsError` prevents the client from distinguishing
  failure at protocol level.

Reserve Go errors for cases where even a result cannot be constructed.

```json
{
  "error": "<code>",
  "message": "<human-readable; upstream message sanitized and ≤300 characters, local property list may be complete>",
  "suggestion": "<next action>"
}
```

For upstream errors, extract only HTTP status and Google's `message`; never
return a full request/error body, which can include client brand terms in
`dimension_filter_groups` or customer URLs in `urls`.

| Code | Trigger |
|---|---|
| `invalid_input` | Argument or constraint validation fails. |
| `auth_failed` | 401. |
| `permission_denied` | 403; for example, the credential lacks the property. |
| `not_found` | 404. |
| `quota_exceeded` | 429, including after retry/backoff is exhausted. |
| `upstream_error` | 5xx, including after retry/backoff is exhausted, or an unparsable response. |
| `write_disabled` | A write action is requested without required flags. |

## 6. v2 candidates, only after v1 is stable

These are local computation orchestration, not new Google APIs:

| Tool | Purpose |
|---|---|
| `find_ctr_opportunities` | High-impression queries ranked 4–15 whose CTR is below a position baseline. |
| `find_content_decay` | Pages with click/impression decline between two periods. |
| `find_keyword_cannibalization` | Queries served by several pages with substantial overlapping impressions. |
| `find_low_hanging_keywords` | Queries near page one with sufficient impressions. |
| `get_page_query_matrix` | Query × page matrix. |

Such tools must document their scoring formula and assumptions in output
metadata, include the raw or summarized rows supporting a recommendation, never
misstate average position as literal query rank, and have deterministic
fixture-based tests.

## 7. Official-document index

- API reference: `developers.google.com/webmaster-tools/v1/api_reference_index`
- `searchAnalytics.query`: `developers.google.com/webmaster-tools/v1/searchanalytics/query`
- Limits: `developers.google.com/webmaster-tools/limits`
- `urlInspection.index.inspect`: `developers.google.com/webmaster-tools/v1/urlInspection.index/inspect`
- Go client: `pkg.go.dev/google.golang.org/api/searchconsole/v1`
- MCP Go SDK: `pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp`
