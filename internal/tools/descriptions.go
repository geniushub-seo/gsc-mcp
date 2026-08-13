package tools

// Finalized tool descriptions. LLM clients only see these strings; keep facts
// accurate and include misuse boundaries. Data freshness notes apply to all
// Search Analytics tools (PT timezone, 2–4 day delay, 16-month retention).

const (
	descListSites = "List all Google Search Console properties the service account can access. " +
		"Returns site_url and permission_level for each property (including sc-domain: forms). " +
		"Call this first when you are unsure which property format to use for other tools. " +
		"Search Analytics dates are in PT timezone, data is typically delayed 2–4 days (recent dates may be incomplete), and history is retained about 16 months."

	descGetSite = "Get the permission level for a single Google Search Console property. " +
		"site_url accepts flexible input (bare domain, full URL, or sc-domain:/URL-prefix forms) and is normalized automatically; " +
		"on 403 the server resolves against accessible properties. " +
		"If unsure of the property format, call list_sites first. " +
		"Search Analytics dates are in PT timezone with a typical 2–4 day delay and ~16 months retention."

	descQuerySearchAnalytics = "Query Google Search Console search analytics. Returns clicks, impressions, CTR (clicks/impressions), and average position grouped by dimensions. " +
		"Dates are PT timezone; data is typically delayed 2–4 days (recent dates may be incomplete) and retained about 16 months. " +
		"Future dates and ranges that may be outside retention get a soft 'note' field (not an error); empty rows can mean 'no data' rather than 'no traffic'. " +
		"dimension hour requires data_state=hourly_all and a start_date within the last 10 days (PT); older hour ranges return invalid_input. " +
		"includingRegex/excludingRegex expressions are validated as RE2 before the API call (lookbehind/lookahead/backreferences are rejected). " +
		"row_limit defaults to 150 (kept small for token efficiency); for export set row_limit explicitly, maximum 25,000 (the GSC console UI only shows 1,000 rows at a time). " +
		"Rows are ordered by sort_by (default clicks, descending) and the applied order is echoed in the 'ordering' output field. " +
		"The GSC API itself can only order by clicks, so any other sort_by makes the server scan up to 25,000 rows and sort locally — that is slower but is the only way to get a correct top-N by impressions, ctr, or position. " +
		"Always read 'truncated' and 'rows_examined': truncated=true means the rows are the top row_limit of a larger set, and scan_capped=true means even the scan was incomplete so the top-N may be missing entries. " +
		"The API returns only top rows — row sums will not match console totals. " +
		"Average position is a weighted average across all impressions for that dimension combination, not proof that a specific query ranks at that exact position. " +
		"data_state defaults to all (includes unfinalized data, matching the console); use final for finalized-only. " +
		"Common use: dimensions=['query'] with dimension_filter_groups operator excludingRegex to drop branded queries. " +
		"search_type=video is Google Video search performance, not the Video indexing report."

	descComparePeriods = "Compare Search Console search analytics between two date ranges: all deltas and *_change_pct are B minus A (period A = baseline/earlier, period B = current/later). " +
		"Dates are PT timezone; data is typically delayed 2–4 days and retained about 16 months. " +
		"The two periods must be the same length (inclusive day counts); unequal lengths return invalid_input. " +
		"Output includes period_a_days and period_b_days. " +
		"Per-key output includes A/B metrics plus absolute deltas and relative *_change_pct. " +
		"ctr_a/ctr_b are 0–1 fractions; ctr_delta_pp is the change in percentage points (e.g. 0.10→0.125 yields 2.5). " +
		"ctr_change_pct is relative percent change of CTR. " +
		"position_change is B−A (negative means ranking improved because lower position numbers are better); position_improved is true when B < A. " +
		"Keys only in one period have only_in='a'|'b' and omit position_change/position_improved (missing side is not a rank of 0). " +
		"Rows are sorted by sort_by before truncation: clicks_delta desc (default) for biggest gains, clicks_delta asc for biggest drops, position_change asc for biggest rank improvements. " +
		"This matters because the GSC API can only order by clicks, and rank movement is uncorrelated with click volume — asking for 'biggest rank improvement' without sort_by would return whichever high-click rows happened to come back. " +
		"Both periods are therefore scanned to 25,000 rows before joining, which makes this tool slower than query_search_analytics on large properties. " +
		"row_limit defaults to 100 and caps the joined output (not each period); read 'truncated', 'rows_examined', and 'scan_capped' to know whether the top-N is complete."

	descInspectURL = "Inspect Google's indexed version of one to ten known URLs under a Search Console property. " +
		"Returns a compact index-status summary plus optional mobile usability, rich results, and AMP verdicts when present. " +
		"This does NOT: test the live on-page version, request indexing, list all indexed URLs on the site, or replace the bulk Page Indexing report in Search Console. " +
		"Index status reflects what Google has stored and may lag the live page. " +
		"Google applies URL Inspection API quotas — see https://developers.google.com/webmaster-tools/limits (do not assume a fixed daily number). " +
		"Calls are sequential with a short delay. site_url accepts flexible input and resolves on 403. " +
		"Search Analytics (separate tools) uses PT dates with a typical 2–4 day delay and ~16 months retention."

	descManageSitemaps = "List, get, submit, or delete sitemaps for a Search Console property. " +
		"action=list and action=get are always available (read-only). " +
		"action=submit requires GSC_ENABLE_WRITE=true. " +
		"action=delete requires both GSC_ENABLE_WRITE=true and GSC_ALLOW_DESTRUCTIVE=true. " +
		"When write flags are off this tool still appears in tools/list but write actions return write_disabled without calling the API. " +
		"feedpath is required for get, submit, and delete. site_url accepts flexible input and resolves on 403. " +
		"Search Analytics dates (other tools) are PT timezone with a typical 2–4 day delay and ~16 months retention."
)
