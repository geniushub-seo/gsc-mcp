# Independent R-gate handover

## Judgement

R-gate: monthly-report and nonbrand-performance derive brand/non-brand shares from capped grouped query rows instead of filtered aggregate totals.

## Defects

### 1. Release blocker: brand/non-brand KPI recipes cannot produce the totals they promise

- `.agents/skills/monthly-report/SKILL.md:34-46` requests `compare_periods` with `dimensions: ["query"]`, then `.agents/skills/monthly-report/SKILL.md:104-105` requires clicks, share, and change for the brand/non-brand split.
- `.agents/skills/nonbrand-performance/SKILL.md:34-50` likewise requests grouped query rows with `row_limit: 1000`, then `.agents/skills/nonbrand-performance/SKILL.md:60-65` presents those results as clicks, share, and period change.
- `internal/tools/compare_periods.go:25-31` defines dimensions as grouping keys and the default output cap as 100; `internal/tools/compare_periods.go:228-235` truncates the joined grouped rows to that cap. The monthly skill itself demonstrates the correct totals shape at `.agents/skills/monthly-report/SKILL.md:18-26`: omit dimensions.

How to reproduce: use a mocked property with more than 100 matching non-brand queries (or more than 1000 for `nonbrand-performance`), with material clicks below the cap. Follow the skill calls exactly and sum the returned query rows. The reported clicks/share omit the lower rows; `truncated=true` warns that the list is incomplete but cannot recover the missing total. Run the same filter with dimensions omitted and the API returns the aggregate row needed for the KPI.

Required correction: use separate filtered aggregate calls with dimensions omitted for branded and non-brand clicks/share/change. Keep dimensioned calls only for query detail/rankings. Add regression coverage that the KPI recipe does not derive totals from a top-N query list, and retain a caveat for anonymized-query effects.

### 2. P0 conservative merge is incomplete for non-object entries

- `internal/setup/setup.go:495-500` silently replaces any existing non-object `mcpServers` value with a new object.
- `internal/setup/setup.go:502-509` silently replaces any existing non-object `mcpServers.gsc` value with a new object. That destroys the existing value instead of conservatively preserving it or returning a merge error.
- Existing preservation coverage at `internal/setup/setup_test.go:57-103` only exercises an object-valued `gsc` entry.

How to reproduce: create a config containing `{"mcpServers":{"gsc":"custom-wrapper"}}`, call `MergeMCPConfigFile(path, "gsc", "/new/gsc-mcp", false)`, and inspect the file. The string is replaced by `{"command":"/new/gsc-mcp"}` with no error. The same loss occurs to the entire section for `{"mcpServers":[...]}`.

Required correction: when an existing `mcpServers` or named `gsc` entry is present but is not an object, return a descriptive error and leave the original file byte-for-byte intact. Add regression tests for both shapes.

### 3. `doctor` exits 1 correctly but prints a success-like `Done.` after a failed live check

- `internal/setup/setup.go:113-125` records the failed live check.
- Because `doctor` is `DryRun=true`, the condition at `internal/setup/setup.go:127-133` continues into MCP-config preview output after that failure.
- `internal/setup/setup.go:204-207` prints `Done.` unconditionally and only then returns the live error.

How reproduced during this gate:

```text
env GOOGLE_APPLICATION_CREDENTIALS= GOOGLE_SERVICE_ACCOUNT_FILE= GOOGLE_SERVICE_ACCOUNT_JSON='{"type":"service_account","project_id":"x"}' go run ./cmd/gsc-mcp doctor
```

Observed result: exit 1 (correct), but the output printed `MCP clients:`, several dry-run snippets, and `Done.` after `list_sites failed`. This is misleading diagnostic sequencing even though the exit status satisfies P0.

Required correction: preserve any useful diagnostics, but never print an unconditional success word after a recorded error; end with an explicit failed summary or return before `Done.`. Add an output assertion as well as the existing error assertion.

### 4. `rows_examined` under-reports a non-native sorted scan after `start_row`

- `internal/tools/query_search_analytics.go:47-50` defines `rows_examined` as the number of rows considered before truncation.
- `internal/tools/query_search_analytics.go:245-255` sorts and then removes the first `start_row` rows, but `internal/tools/query_search_analytics.go:260-265` records `examined` only after that removal.

How to reproduce: return three rows from the mock API, request `sort_by: position`, `start_row: 1`, `row_limit: 1`. All three rows are scanned and sorted, but the response reports `rows_examined: 2`. With a full 25,000-row scan and `start_row: 24999`, it reports 1 while `scan_capped=true` says the scan hit 25,000.

Required correction: define and test the contract explicitly. If `rows_examined` means scanned candidates, capture it before applying the offset; if it means post-offset candidates, rename/re-document it consistently. Existing `truncated` and `scan_capped` selection logic is otherwise correct.

## P0/P1 claims independently verified

- `compare_periods` filter groups are in the Go input and handwritten schema, use shared validation/normalization, are added to both request bodies, and are listed in the stringified-array middleware map. Focused body and protocol tests passed.
- `monthly-report` now specifies separate `clicks_delta desc` and `asc` calls for both query and page winners/losers and tells the caller to read `truncated`, `scan_capped`, and `ordering`. Defect 1 concerns its separate KPI split.
- Native clicks-desc below 25,000 requests `row_limit+1`, crops the peek, and sets `truncated`; a 25,000-row full native response sets `scan_capped`. Native `start_row` is passed to Google together with the peek.
- Non-native sorting requests 25,000 rows from `startRow=0`, sorts locally, then applies `start_row`; a full scan retains `scan_capped=true`, including when the offset consumes most/all rows.
- Non-dry-run setup verifies before config writes and skips writes on live failure. `doctor` returns an error that the CLI maps to exit 1. Missing default ADC is informational rather than an immediate error, so process-env/service-account sources can still resolve.
- Object-valued existing `gsc` entries preserve `env`, `args`, and other fields while updating only `command`; defect 2 covers non-object shapes.
- P1 text/code checks passed: ADC still requires `GSC_ENABLE_WRITE=true`; no user-facing claim says the flag has no ADC effect; index-health specifies `sort_by: impressions`; all four skills mention completeness/order fields; list-sites/plain-403 language is credential-neutral; uppercase `WWW` is normalized; `install.sh` has `sha256sum` fallback; INSTALL uses `gsc-mcp doctor` rather than hand JSON-RPC.

## Checks actually run

- `git status --short`, `git rev-parse --short HEAD`, `git describe --tags --exact-match HEAD`, complete per-file diffs, and line-numbered source reads: confirmed `main`, `d77f15f`, `v0.4.3`, and reviewed the full uncommitted P0/P1 surface.
- `git diff --check`: pass.
- `go vet ./...`: pass.
- `go test ./...`: pass for all seven packages.
- `make check` with normal host cache permissions: pass; vet and tests passed, golangci-lint 2.12.2 reported `0 issues`.
- `go test -race -count=1 ./...` with normal host cache permissions: pass for all seven packages.
- Focused uncached tool tests for both-period filters, native peek, 25,000 native cap, and non-native offset-after-sort: pass.
- Focused uncached setup tests for error propagation, verify-before-write, missing ADC handling, and map-field preservation: pass.
- Focused uncached protocol/stringified-filter and ADC warning tests: pass.
- Direct invalid-credential `go run ./cmd/gsc-mcp doctor`: exit 1; also reproduced defect 3.
- `go mod tidy -diff` with normal host cache permissions: pass with no diff.
- `sh -n install.sh`: pass.
- Current-tree text search for obsolete ADC no-effect and service-account-only list/403 guidance: no user-facing matches; only negative test/spec wording remained.
- `gofmt -d cmd/gsc-mcp internal`: non-empty only for files unchanged from `d77f15f` (current Go 1.26 formatting drift in baseline files); no Grok-modified file appeared in that diff.

Environment note: the first sandboxed `make check` emitted `no go files to analyze`, while direct `go list ./...` also reported denied writes to Go's module stat cache. The unrestricted rerun passed lint with zero issues. The same sandbox-cache artifact initially blocked race and tidy checks; both passed when rerun with normal cache permissions. Therefore there is no evidence of a code lint miss, and the sandbox result is not a repository gate failure.

## Checks not run

- No live `list_sites` call with valid user credentials; that would access the user's real Google account. Only a deliberately invalid inline credential was used to verify the failure exit path.
- No non-dry-run setup against real MCP client configs; writes outside this handover were forbidden. The existing temp-directory regression test covered verify-before-write.
- No full `install.sh` execution; it would download/install artifacts and write outside the allowed handover path. Syntax and source flow were inspected instead.
- No GitHub-hosted Actions run; local equivalents, lint, race, and tidy-diff were run.
- No regression tests were authored for the defects above because this review was expressly limited to writing this handover file.
- No commit, push, tag, release, or modification/read of any file in `../new-go-mcp` was performed. An initial repository-instruction path scan listed `../new-go-mcp/AGENTS.md`; that file was not opened.

## Next-round kickoff prompt

```text
Fix these defects first in /Users/roy-mac/Documents/1.GitHub/gsc-mcp/public-release; do not push, tag, or release, and do not touch ../new-go-mcp:

1. monthly-report and nonbrand-performance must obtain branded/non-brand KPI totals from filtered aggregate calls with dimensions omitted; dimensioned top-N calls are detail only. Add a regression/contract check that shares and changes are never derived from capped grouped query rows.
2. MergeMCPConfigFile must error without modifying the file when existing mcpServers or mcpServers.gsc is non-object. Add tests for both shapes.
3. doctor must not print unconditional `Done.` after a live-check failure. Keep exit 1 and add an output assertion.
4. Resolve the rows_examined contract for non-native sorting with start_row and add start_row+scan_capped coverage.

Preserve the already verified P0/P1 behavior: filters in both compare requests and middleware, winners/losers desc+asc, native +1 peek/crop, 25k scan cap, local sort before offset, verify before write, map-valued env/args preservation, ADC local write gate, neutral credential wording, WWW normalization, checksum fallback, and doctor-based install verification.

Then run from the module root: go vet ./..., go test ./..., go test -race -count=1 ./..., make check, go mod tidy -diff, sh -n install.sh, and git diff --check. Report changed files, regression mapping, command results, and remaining issues. Do not commit.
```
