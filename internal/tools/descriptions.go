package tools

// Finalized tool descriptions. LLM clients only see these strings; keep facts
// accurate and include misuse boundaries.
//
// Search Analytics freshness rules are spelled out only in descListSites and
// descQuerySearchAnalytics; the other tools point there.

const (
	descListSites = "List all Google Search Console properties the authenticated credentials can access. " +
		"Returns site_url and permission_level for each property (including sc-domain: forms). " +
		"Call this first when you are unsure which property format to use for other tools. " +
		"Search Analytics dates are in PT timezone, data is typically delayed 2–4 days (recent dates may be incomplete), and history is retained about 16 months."

	descGetSite = "Get the permission level for a single Google Search Console property. " +
		"site_url accepts flexible input (bare domain, full URL, or sc-domain:/URL-prefix forms) and is normalized automatically; " +
		"it is never switched to another property; on 403 the error lists accessible properties. " +
		"If unsure of the property format, call list_sites first."

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
		"Dates are PT timezone; full date rules: see list_sites. " +
		"The two periods must be the same length (inclusive day counts); unequal lengths return invalid_input. " +
		"Output includes period_a_days and period_b_days. " +
		"Per-key output includes A/B metrics plus absolute deltas and relative *_change_pct. " +
		"ctr_a/ctr_b are 0–1 fractions; ctr_delta_pp is the change in percentage points (e.g. 0.10→0.125 yields 2.5). " +
		"ctr_change_pct is relative percent change of CTR. " +
		"position_change is B−A (negative means ranking improved because lower position numbers are better); position_improved is true when B < A. " +
		"Keys only in one period have only_in='a'|'b' and omit position_change/position_improved (missing side is not a rank of 0). " +
		"dimension_filter_groups apply to both periods (same operators as query_search_analytics, including excludingRegex for non-brand). " +
		"Rows are sorted by sort_by before truncation: clicks_delta desc (default) for biggest gains, clicks_delta asc for biggest drops, position_change asc for biggest rank improvements. " +
		"This matters because the GSC API can only order by clicks, and rank movement is uncorrelated with click volume — asking for 'biggest rank improvement' without sort_by would return whichever high-click rows happened to come back. " +
		"Both periods are therefore scanned to 25,000 rows before joining, which makes this tool slower than query_search_analytics on large properties. " +
		"row_limit defaults to 100 and caps the joined output (not each period); read 'truncated', 'rows_examined', and 'scan_capped' to know whether the top-N is complete."

	descInspectURL = "Inspect Google's indexed version of one to ten known URLs under a Search Console property. " +
		"Returns a compact index-status summary plus optional mobile usability, rich results, and AMP verdicts when present. " +
		"This does NOT: test the live on-page version, request indexing, list all indexed URLs on the site, or replace the bulk Page Indexing report in Search Console. " +
		"Index status reflects what Google has stored and may lag the live page. " +
		"Google applies URL Inspection API quotas — see https://developers.google.com/webmaster-tools/limits (do not assume a fixed daily number). " +
		"Calls are sequential with a short delay. site_url accepts flexible input; on 403 the error lists accessible properties."

	descManageSitemaps = "List, get, submit, or delete sitemaps for a Search Console property. " +
		"action=list and action=get are always available (read-only). " +
		"action=submit requires GSC_ENABLE_WRITE=true. " +
		"action=delete requires both GSC_ENABLE_WRITE=true and GSC_ALLOW_DESTRUCTIVE=true. " +
		"When write flags are off this tool still appears in tools/list but write actions return write_disabled without calling the API. " +
		"feedpath is required for get, submit, and delete. site_url accepts flexible input; on 403 the error lists accessible properties."

	descFindCTRGaps = "Find query+page pairs that rank well but are clicked less than their position peers — the shortlist for title and meta-description rewrites. " +
		"Scans up to 25,000 query+page rows server-side and returns them ranked by opportunity_clicks (impressions x (baseline_ctr - ctr)), largest first. " +
		"baseline_ctr is the MEDIAN CTR of the OTHER scanned rows whose rounded average position is near this row's position — the row being scored is never its own peer, so the comparison cannot be circular. The neighbour window widens from 0 until baseline_samples (the peer count) reaches min_samples; a row with no peers at all is left out rather than compared against itself. " +
		"Median, not mean, because CTR is right-skewed and a few branded queries would otherwise make normal pages look broken. " +
		"Every row carries baseline_ctr, baseline_samples, baseline_window_positions, and baseline_scope; baseline_scope=site_wide means the window had to cover every observed position, so the comparison is property-wide rather than position-matched (small properties). " +
		"cannibalization and pages_for_query appear only when one query draws impressions for more than one of your pages. " +
		"All thresholds (position_min 1, position_max 20, min_impressions 100, min_samples 10) are parameters and the values actually applied are echoed in 'assumptions'. " +
		"Pass dimension_filter_groups with operator excludingRegex on dimension query to get non-brand CTR gaps. " +
		"This tool returns facts and arithmetic, not advice: it does not tell you what to write. " +
		"Dates are PT timezone. Google only returns metadata.firstIncompleteDate on a date-grouped request, so this tool issues its own dimensions=[date] probe and reports the result in assumptions.first_incomplete_date instead of assuming a fixed lag; that is why it makes two API calls (one under data_state=final, where Google never reports it). " +
		"Read scan_capped: true means the 25,000-row scan was incomplete, so both the ranking and the baseline pool may be missing rows."

	descFindPageTwoQueries = "Find queries ranking just off page one (default average position 11–20) with real impression volume — the shortlist for adding content or internal links. " +
		"Scans up to 25,000 query+page rows, aggregates them to one row per query, and returns them ranked by impressions, largest first. " +
		"Each row names top_page: the page with the most impressions for that query, i.e. the one to improve. pages_for_query > 1 means several of your pages compete for the query. " +
		"Query-level clicks and impressions are summed across pages and position is the impression-weighted mean of the per-page positions, so it can differ slightly from a query-only query_search_analytics call; this is stated in 'assumptions'. " +
		"Thresholds (position_min 11, position_max 20, min_impressions 50) are parameters and the applied values are echoed in 'assumptions'. " +
		"Pass dimension_filter_groups with operator excludingRegex on dimension query to exclude brand terms. " +
		"Returns facts only, no recommendations. Dates are PT timezone. Google only returns metadata.firstIncompleteDate on a date-grouped request, so this tool issues its own dimensions=[date] probe and reports it in assumptions; that is why it makes two API calls (one under data_state=final, where Google never reports it). " +
		"Read scan_capped before treating the list as complete."

	descFindDecliningPages = "Find pages that lost clicks between two equal-length periods AND classify why: attribution is position_drop, impressions_drop, ctr_drop, or unclear. " +
		"The attribution is the point of this tool — the three causes need opposite responses. position_drop means look at competitors; impressions_drop usually means search demand or a SERP layout change and is often not your fault; only ctr_drop points at the title. " +
		"unclear includes any page missing a position on one side (appeared or vanished): that is 'we do not know why', not 'no cause'. " +
		"Incomplete days are excluded automatically: the tool reads Google's metadata.firstIncompleteDate and trims the end of BOTH periods by the same number of days, so the periods stay equal length. " +
		"Counting the last few days of the current period against a complete baseline is the classic way to invent a decline that does not exist. " +
		"assumptions reports the dates actually queried, excluded_incomplete_days, and (when trimmed) the originally requested ends. " +
		"Period A is the baseline/earlier period, period B the current/later one; every delta is B minus A. Periods must be the same length and period A must end before period B starts (reversed or overlapping ranges return invalid_input). " +
		"Rows are ranked by clicks_delta ascending, i.e. biggest absolute loss first. " +
		"Thresholds (min_click_drop 10, position_drop_threshold 1, impressions_drop_pct_threshold -10) are parameters and the applied values are echoed in 'assumptions'; min_click_drop is inclusive, so a page down exactly 10 clicks is reported. " +
		"Returns facts and a classification, not advice on what to change. " +
		"Dates are PT timezone. Makes three API calls (a date-grouped freshness probe, skipped under data_state=final, plus one page-level scan per period, each up to 25,000 rows); read scan_capped before treating the list as complete."
)
