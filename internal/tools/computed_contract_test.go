package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/searchconsole/v1"
)

// These run the computed tools end to end over captured Search Analytics
// response shapes, so the ranking, truncation, threshold filtering and the
// parsing of Google's metadata.firstIncompleteDate are all exercised against
// real JSON rather than hand-built structs.

func computedFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// analyticsRouter serves a Search Analytics fixture per request, chosen by the
// request body, and records every request it saw.
func analyticsRouter(t *testing.T, pick func(req searchconsole.SearchAnalyticsQueryRequest) []byte) (*httptest.Server, *[]searchconsole.SearchAnalyticsQueryRequest) {
	t.Helper()
	var seen []searchconsole.SearchAnalyticsQueryRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/searchAnalytics/query") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var req searchconsole.SearchAnalyticsQueryRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("unmarshal request: %v", err)
		}
		seen = append(seen, req)
		body := pick(req)
		if body == nil {
			t.Errorf("no fixture for request %+v", req)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func decodeResult[T any](t *testing.T, body string) T {
	t.Helper()
	var out T
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("unmarshal tool output: %v\nbody: %s", err, body)
	}
	return out
}

func TestContract_FindCTRGaps_RanksByOpportunityAndTruncates(t *testing.T) {
	t.Parallel()

	queryPage := computedFixture(t, "searchanalytics_query_page.json")
	probe := computedFixture(t, "searchanalytics_date_probe.json")
	srv, seen := analyticsRouter(t, func(req searchconsole.SearchAnalyticsQueryRequest) []byte {
		if len(req.Dimensions) == 1 && req.Dimensions[0] == "DATE" {
			return probe
		}
		return queryPage
	})

	client := newTestClient(t, srv.URL+"/")
	result, _, err := findCTRGaps(context.Background(), client, findCTRGapsInput{
		SiteURL:    "sc-domain:example.com",
		StartDate:  "2026-07-01",
		EndDate:    "2026-07-28",
		MinSamples: 3,
		RowLimit:   2,
	})
	if err != nil {
		t.Fatalf("findCTRGaps returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("findCTRGaps returned tool error: %s", extractText(t, result.Content))
	}
	out := decodeResult[findCTRGapsOutput](t, extractText(t, result.Content))

	if out.Ordering != "opportunity_clicks desc" {
		t.Errorf("ordering = %q", out.Ordering)
	}
	if out.RowsExamined != 13 {
		t.Errorf("rows_examined = %d, want 13 (every scanned row)", out.RowsExamined)
	}
	// Six rows sit below their position peers' median CTR; row_limit=2 keeps the top two.
	if out.RowsMatched != 6 {
		t.Errorf("rows_matched = %d, want 6", out.RowsMatched)
	}
	if !out.Truncated || out.RowCount != 2 {
		t.Errorf("truncated=%v row_count=%d, want true/2", out.Truncated, out.RowCount)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(out.Rows))
	}

	top := out.Rows[0]
	if top.Query != "blue widgets" || top.Page != "https://example.com/blue-widgets/" {
		t.Errorf("top row = %q / %q, want the blue widgets gap", top.Query, top.Page)
	}
	// min_samples=3 peers is not met by the two other rows at position 11, so the
	// window widens to positions 10–12 (six peers, this row excluded).
	if top.OpportunityClicks != 90 {
		t.Errorf("opportunity_clicks = %v, want 90 (5000 impressions x (0.02 - 0.002))", top.OpportunityClicks)
	}
	if top.BaselineCtr != 0.02 || top.BaselineSamples != 6 || top.BaselineWindowPositions != 1 {
		t.Errorf("baseline = %v from %d peers, window %d; want 0.02 / 6 / 1",
			top.BaselineCtr, top.BaselineSamples, top.BaselineWindowPositions)
	}
	if top.BaselineScope != baselineScopeNeighborhood {
		t.Errorf("baseline_scope = %q, want %q", top.BaselineScope, baselineScopeNeighborhood)
	}
	if top.CtrGapPP != 1.8 {
		t.Errorf("ctr_gap_pp = %v, want 1.8", top.CtrGapPP)
	}
	// The same query also ranks with a second page of ours.
	if !top.Cannibalization || top.PagesForQuery != 2 {
		t.Errorf("cannibalization=%v pages_for_query=%d, want true/2", top.Cannibalization, top.PagesForQuery)
	}
	if out.Rows[0].OpportunityClicks < out.Rows[1].OpportunityClicks {
		t.Error("rows are not in descending opportunity order")
	}

	// The 20-impression row must not appear, and must not be in the baseline pool.
	for _, r := range out.Rows {
		if r.Query == "tiny query" {
			t.Error("a row below min_impressions was reported")
		}
	}
	if out.Assumptions.BaselinePoolRows != 12 {
		t.Errorf("baseline_pool_rows = %d, want 12 (13 scanned minus the 20-impression row)", out.Assumptions.BaselinePoolRows)
	}

	// Assumptions must state every threshold actually applied.
	if out.Assumptions.MinSamples != 3 || out.Assumptions.MinImpressions != ctrGapsDefaultMinImpressions {
		t.Errorf("assumptions thresholds = min_samples %d / min_impressions %d",
			out.Assumptions.MinSamples, out.Assumptions.MinImpressions)
	}
	if out.Assumptions.PositionMin != ctrGapsDefaultPositionMin || out.Assumptions.PositionMax != ctrGapsDefaultPositionMax {
		t.Errorf("assumptions position window = %v..%v", out.Assumptions.PositionMin, out.Assumptions.PositionMax)
	}
	if out.Assumptions.RowLimit != 2 {
		t.Errorf("assumptions row_limit = %d, want 2 — the cap that produced this shortlist is a threshold like any other", out.Assumptions.RowLimit)
	}
	if out.Assumptions.FirstIncompleteDate != "2026-08-17" {
		t.Errorf("assumptions first_incomplete_date = %q, want 2026-08-17 from the response metadata", out.Assumptions.FirstIncompleteDate)
	}

	// One freshness probe grouped by date, one query+page scan at the API ceiling.
	if len(*seen) != 2 {
		t.Fatalf("api calls = %d, want 2", len(*seen))
	}
	scan := (*seen)[1]
	if len(scan.Dimensions) != 2 || scan.Dimensions[0] != "QUERY" || scan.Dimensions[1] != "PAGE" {
		t.Errorf("scan dimensions = %v, want [QUERY PAGE]", scan.Dimensions)
	}
	if scan.RowLimit != sortScanRowLimit {
		t.Errorf("scan row_limit = %d, want %d", scan.RowLimit, sortScanRowLimit)
	}
	if scan.DataState != "ALL" {
		t.Errorf("scan data_state = %q, want ALL", scan.DataState)
	}
}

func TestContract_FindCTRGaps_ExcludingRegexReachesBothCalls(t *testing.T) {
	t.Parallel()

	queryPage := computedFixture(t, "searchanalytics_query_page.json")
	probe := computedFixture(t, "searchanalytics_date_probe.json")
	srv, seen := analyticsRouter(t, func(req searchconsole.SearchAnalyticsQueryRequest) []byte {
		if len(req.Dimensions) == 1 && req.Dimensions[0] == "DATE" {
			return probe
		}
		return queryPage
	})

	client := newTestClient(t, srv.URL+"/")
	result, _, err := findCTRGaps(context.Background(), client, findCTRGapsInput{
		SiteURL:   "sc-domain:example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-28",
		DimensionFilterGroups: []DimensionFilterGroup{{
			GroupType: "and",
			Filters: []DimensionFilter{{
				Dimension:  "query",
				Operator:   "excludingRegex",
				Expression: "(?i)brandname",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("findCTRGaps returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("findCTRGaps returned tool error: %s", extractText(t, result.Content))
	}

	scan := (*seen)[len(*seen)-1]
	if len(scan.DimensionFilterGroups) != 1 || len(scan.DimensionFilterGroups[0].Filters) != 1 {
		t.Fatalf("filters did not reach the scan: %+v", scan.DimensionFilterGroups)
	}
	f := scan.DimensionFilterGroups[0].Filters[0]
	if f.Operator != "EXCLUDING_REGEX" || f.Dimension != "QUERY" {
		t.Errorf("filter = %s %s, want QUERY EXCLUDING_REGEX", f.Dimension, f.Operator)
	}
	// The freshness probe is deliberately unfiltered: data freshness is a
	// property-level fact, and a filter matching nothing returns no metadata.
	if len((*seen)[0].DimensionFilterGroups) != 0 {
		t.Errorf("freshness probe was filtered: %+v", (*seen)[0].DimensionFilterGroups)
	}
}

// Google only reports firstIncompleteDate under dataState=all, so a FINAL
// request must not spend an extra API call on a probe that cannot answer.
func TestContract_FindCTRGaps_FinalSkipsFreshnessProbe(t *testing.T) {
	t.Parallel()

	queryPage := computedFixture(t, "searchanalytics_query_page.json")
	srv, seen := analyticsRouter(t, func(_ searchconsole.SearchAnalyticsQueryRequest) []byte {
		return queryPage
	})

	client := newTestClient(t, srv.URL+"/")
	result, _, err := findCTRGaps(context.Background(), client, findCTRGapsInput{
		SiteURL:   "sc-domain:example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-28",
		DataState: "final",
	})
	if err != nil {
		t.Fatalf("findCTRGaps returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("findCTRGaps returned tool error: %s", extractText(t, result.Content))
	}
	if len(*seen) != 1 {
		t.Fatalf("api calls = %d, want 1 (no freshness probe under final)", len(*seen))
	}
	if d := (*seen)[0].Dimensions; len(d) == 1 && d[0] == "DATE" {
		t.Error("a DATE probe was sent under data_state=final")
	}
	out := decodeResult[findCTRGapsOutput](t, extractText(t, result.Content))
	if strings.Contains(out.Note, "freshness probe failed") {
		t.Errorf("skipped probe must not be reported as a failure, got note %q", out.Note)
	}
}

func TestContract_FindPageTwoQueries_AggregatesAndRanksByImpressions(t *testing.T) {
	t.Parallel()

	queryPage := computedFixture(t, "searchanalytics_query_page.json")
	probe := computedFixture(t, "searchanalytics_date_probe.json")
	srv, _ := analyticsRouter(t, func(req searchconsole.SearchAnalyticsQueryRequest) []byte {
		if len(req.Dimensions) == 1 && req.Dimensions[0] == "DATE" {
			return probe
		}
		return queryPage
	})

	client := newTestClient(t, srv.URL+"/")
	result, _, err := findPageTwoQueries(context.Background(), client, findPageTwoQueriesInput{
		SiteURL:   "sc-domain:example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-28",
		RowLimit:  3,
	})
	if err != nil {
		t.Fatalf("findPageTwoQueries returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("findPageTwoQueries returned tool error: %s", extractText(t, result.Content))
	}
	out := decodeResult[findPageTwoQueriesOutput](t, extractText(t, result.Content))

	if out.Ordering != "impressions desc" {
		t.Errorf("ordering = %q", out.Ordering)
	}
	if out.QueriesExamined != 12 {
		t.Errorf("queries_examined = %d, want 12 distinct queries from 13 rows", out.QueriesExamined)
	}
	// Eight queries sit in 11–20 with at least 50 impressions; row_limit keeps three.
	if out.RowsMatched != 8 || !out.Truncated || out.RowCount != 3 {
		t.Errorf("rows_matched=%d truncated=%v row_count=%d, want 8/true/3", out.RowsMatched, out.Truncated, out.RowCount)
	}

	top := out.Rows[0]
	if top.Query != "blue widgets" {
		t.Fatalf("top row = %q, want blue widgets (5,800 impressions across two pages)", top.Query)
	}
	if top.Impressions != 5800 || top.Clicks != 26 {
		t.Errorf("aggregate = %v clicks / %v impressions, want 26/5800", top.Clicks, top.Impressions)
	}
	if top.Position != 11.51 {
		t.Errorf("position = %v, want 11.51 (impression-weighted across both pages)", top.Position)
	}
	if top.TopPage != "https://example.com/blue-widgets/" || top.TopPageImpressions != 5000 {
		t.Errorf("top_page = %q (%v impressions), want the blue-widgets page with 5000", top.TopPage, top.TopPageImpressions)
	}
	if top.PagesForQuery != 2 {
		t.Errorf("pages_for_query = %d, want 2", top.PagesForQuery)
	}
	for i := 1; i < len(out.Rows); i++ {
		if out.Rows[i-1].Impressions < out.Rows[i].Impressions {
			t.Errorf("rows are not in descending impression order at %d", i)
		}
	}
	// Page one and the low-volume query must be filtered out.
	for _, r := range out.Rows {
		if r.Position < pageTwoDefaultPositionMin || r.Position > pageTwoDefaultPositionMax {
			t.Errorf("row %q at position %v escaped the position filter", r.Query, r.Position)
		}
	}
	if out.Assumptions.FirstIncompleteDate != "2026-08-17" {
		t.Errorf("assumptions first_incomplete_date = %q, want 2026-08-17", out.Assumptions.FirstIncompleteDate)
	}
	if out.Assumptions.RowLimit != 3 {
		t.Errorf("assumptions row_limit = %d, want 3", out.Assumptions.RowLimit)
	}
}

func TestContract_FindDecliningPages_AttributesAndExcludesIncompleteDays(t *testing.T) {
	t.Parallel()

	probe := computedFixture(t, "searchanalytics_date_probe.json")
	periodA := computedFixture(t, "searchanalytics_pages_period_a.json")
	periodB := computedFixture(t, "searchanalytics_pages_period_b.json")
	srv, seen := analyticsRouter(t, func(req searchconsole.SearchAnalyticsQueryRequest) []byte {
		if len(req.Dimensions) == 1 && req.Dimensions[0] == "DATE" {
			return probe
		}
		if req.StartDate == "2026-06-24" {
			return periodA
		}
		if req.StartDate == "2026-07-22" {
			return periodB
		}
		return nil
	})

	client := newTestClient(t, srv.URL+"/")
	result, _, err := findDecliningPages(context.Background(), client, findDecliningPagesInput{
		SiteURL:      "sc-domain:example.com",
		PeriodAStart: "2026-06-24",
		PeriodAEnd:   "2026-07-21",
		PeriodBStart: "2026-07-22",
		PeriodBEnd:   "2026-08-18",
		RowLimit:     3,
	})
	if err != nil {
		t.Fatalf("findDecliningPages returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("findDecliningPages returned tool error: %s", extractText(t, result.Content))
	}
	out := decodeResult[findDecliningPagesOutput](t, extractText(t, result.Content))

	// Google says data from 2026-08-17 is still being collected, so both periods
	// lose their last two days — otherwise every page looks like it collapsed.
	if out.Assumptions.FirstIncompleteDate != "2026-08-17" || out.Assumptions.ExcludedIncompleteDays != 2 {
		t.Errorf("first_incomplete_date=%q excluded_incomplete_days=%d, want 2026-08-17/2",
			out.Assumptions.FirstIncompleteDate, out.Assumptions.ExcludedIncompleteDays)
	}
	if out.Assumptions.PeriodBEnd != "2026-08-16" || out.Assumptions.PeriodAEnd != "2026-07-19" {
		t.Errorf("queried ends = A %s / B %s, want 2026-07-19 / 2026-08-16",
			out.Assumptions.PeriodAEnd, out.Assumptions.PeriodBEnd)
	}
	if out.Assumptions.RequestedPeriodBEnd != "2026-08-18" || out.Assumptions.RequestedPeriodAEnd != "2026-07-21" {
		t.Errorf("requested ends not reported back: A %q / B %q",
			out.Assumptions.RequestedPeriodAEnd, out.Assumptions.RequestedPeriodBEnd)
	}
	if out.Assumptions.PeriodDays != 26 {
		t.Errorf("period_days = %d, want 26", out.Assumptions.PeriodDays)
	}
	if out.Assumptions.RowLimit != 3 || out.Assumptions.MinClickDrop != decliningDefaultMinClickDrop {
		t.Errorf("assumptions row_limit=%d min_click_drop=%d, want 3/%d",
			out.Assumptions.RowLimit, out.Assumptions.MinClickDrop, decliningDefaultMinClickDrop)
	}
	if !strings.Contains(out.Note, "2026-08-16") {
		t.Errorf("note should explain the shortened range, got %q", out.Note)
	}

	// The clamped dates, not the requested ones, must be what Google was asked for.
	if len(*seen) != 3 {
		t.Fatalf("api calls = %d, want 3 (freshness probe + one scan per period)", len(*seen))
	}
	for _, req := range (*seen)[1:] {
		if req.EndDate != "2026-07-19" && req.EndDate != "2026-08-16" {
			t.Errorf("period scan asked for end_date %q, which includes incomplete days", req.EndDate)
		}
		if len(req.Dimensions) != 1 || req.Dimensions[0] != "PAGE" {
			t.Errorf("period scan dimensions = %v, want [PAGE]", req.Dimensions)
		}
	}

	// Four pages lost at least 10 clicks; the 2-click loss is below the threshold.
	if out.RowsMatched != 4 || !out.Truncated || out.RowCount != 3 {
		t.Fatalf("rows_matched=%d truncated=%v row_count=%d, want 4/true/3",
			out.RowsMatched, out.Truncated, out.RowCount)
	}
	if out.Ordering != "clicks_delta asc (biggest loss first)" {
		t.Errorf("ordering = %q", out.Ordering)
	}

	want := []struct {
		page        string
		clicksDelta float64
		attribution string
	}{
		{"https://example.com/alpha/", -200, attributionPositionDrop},
		{"https://example.com/beta/", -150, attributionImpressionsDrop},
		{"https://example.com/delta/", -100, attributionUnclear},
	}
	for i, w := range want {
		got := out.Rows[i]
		if got.Page != w.page || got.ClicksDelta != w.clicksDelta || got.Attribution != w.attribution {
			t.Errorf("row %d = %s %v %s, want %s %v %s",
				i, got.Page, got.ClicksDelta, got.Attribution, w.page, w.clicksDelta, w.attribution)
		}
	}
	// The page that vanished from period B is flagged as such, not silently zeroed.
	if out.Rows[2].OnlyIn != "a" || out.Rows[2].PositionB != nil {
		t.Errorf("vanished page: only_in=%q position_b=%v, want a/absent", out.Rows[2].OnlyIn, out.Rows[2].PositionB)
	}
	for _, r := range out.Rows {
		if strings.Contains(r.Page, "epsilon") {
			t.Error("a 2-click loss passed min_click_drop=10")
		}
	}
}

func TestContract_FindDecliningPages_RejectsRangeInsideIncompleteWindow(t *testing.T) {
	t.Parallel()

	probe := computedFixture(t, "searchanalytics_date_probe.json")
	srv, _ := analyticsRouter(t, func(req searchconsole.SearchAnalyticsQueryRequest) []byte {
		if len(req.Dimensions) == 1 && req.Dimensions[0] == "DATE" {
			return probe
		}
		t.Error("period scans must not run when there is no complete data to compare")
		return []byte(`{}`)
	})

	client := newTestClient(t, srv.URL+"/")
	result, _, err := findDecliningPages(context.Background(), client, findDecliningPagesInput{
		SiteURL:      "sc-domain:example.com",
		PeriodAStart: "2026-08-15",
		PeriodAEnd:   "2026-08-16",
		PeriodBStart: "2026-08-17",
		PeriodBEnd:   "2026-08-18",
	})
	if err != nil {
		t.Fatalf("findDecliningPages returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected a tool error: period B is entirely inside Google's incomplete window")
	}
	body := extractText(t, result.Content)
	if !strings.Contains(body, "invalid_input") || !strings.Contains(body, "2026-08-17") {
		t.Errorf("error should name the incomplete boundary, got %s", body)
	}
}

func TestFindCTRGaps_FreshnessProbeFailureIsANoteNotAnError(t *testing.T) {
	t.Parallel()

	queryPage := computedFixture(t, "searchanalytics_query_page.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req searchconsole.SearchAnalyticsQueryRequest
		_ = json.Unmarshal(raw, &req)
		if len(req.Dimensions) == 1 && req.Dimensions[0] == "DATE" {
			// 400, not 500: a retryable status would make this test sit through
			// the real backoff schedule.
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"bad request"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(queryPage)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	result, _, err := findCTRGaps(context.Background(), client, findCTRGapsInput{
		SiteURL:    "sc-domain:example.com",
		StartDate:  "2026-07-01",
		EndDate:    "2026-07-28",
		MinSamples: 3,
	})
	if err != nil {
		t.Fatalf("findCTRGaps returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("a failed freshness probe must not fail the whole tool: %s", extractText(t, result.Content))
	}
	out := decodeResult[findCTRGapsOutput](t, extractText(t, result.Content))
	if out.Assumptions.FirstIncompleteDate != "" {
		t.Errorf("first_incomplete_date = %q, want empty when the probe failed", out.Assumptions.FirstIncompleteDate)
	}
	if !strings.Contains(out.Note, "freshness probe failed") {
		t.Errorf("note must admit the probe failed, got %q", out.Note)
	}
	if out.RowsMatched == 0 {
		t.Error("the answer itself should still be computed")
	}
}

func TestFirstIncompleteDate_ReachesQuerySearchAnalyticsAndComparePeriods(t *testing.T) {
	t.Parallel()

	// Google names the first incomplete date in the response itself. Both
	// pass-through analytics tools must surface it instead of leaving callers to
	// assume a fixed lag.
	probe := computedFixture(t, "searchanalytics_date_probe.json")
	srv, _ := analyticsRouter(t, func(searchconsole.SearchAnalyticsQueryRequest) []byte {
		return probe
	})
	client := newTestClient(t, srv.URL+"/")

	result, _, err := querySearchAnalytics(context.Background(), client, querySearchAnalyticsInput{
		SiteURL:    "sc-domain:example.com",
		StartDate:  "2026-08-15",
		EndDate:    "2026-08-18",
		Dimensions: []string{"date"},
	})
	if err != nil {
		t.Fatalf("querySearchAnalytics returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", extractText(t, result.Content))
	}
	qOut := decodeResult[querySearchAnalyticsOutput](t, extractText(t, result.Content))
	if qOut.FirstIncompleteDate != "2026-08-17" {
		t.Errorf("query_search_analytics first_incomplete_date = %q, want 2026-08-17", qOut.FirstIncompleteDate)
	}

	result, _, err = comparePeriods(context.Background(), client, comparePeriodsInput{
		SiteURL:      "sc-domain:example.com",
		PeriodAStart: "2026-08-11",
		PeriodAEnd:   "2026-08-14",
		PeriodBStart: "2026-08-15",
		PeriodBEnd:   "2026-08-18",
		Dimensions:   []string{"date"},
	})
	if err != nil {
		t.Fatalf("comparePeriods returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", extractText(t, result.Content))
	}
	cOut := decodeResult[comparePeriodsOutput](t, extractText(t, result.Content))
	if cOut.PeriodBFirstIncompleteDate != "2026-08-17" || cOut.PeriodAFirstIncompleteDate != "2026-08-17" {
		t.Errorf("compare_periods first_incomplete_date = A %q / B %q, want 2026-08-17 on both (same fixture)",
			cOut.PeriodAFirstIncompleteDate, cOut.PeriodBFirstIncompleteDate)
	}
}

func TestFindDecliningPages_RejectsPeriodsOutOfOrder(t *testing.T) {
	t.Parallel()

	srv, seen := analyticsRouter(t, func(searchconsole.SearchAnalyticsQueryRequest) []byte {
		t.Error("no API call should be made for periods that are not baseline-then-current")
		return []byte(`{}`)
	})
	client := newTestClient(t, srv.URL+"/")

	cases := []struct {
		name                       string
		aStart, aEnd, bStart, bEnd string
	}{
		{"reversed", "2026-08-01", "2026-08-14", "2026-07-18", "2026-07-31"},
		{"overlapping", "2026-07-01", "2026-07-28", "2026-07-15", "2026-08-11"},
		{"identical", "2026-07-01", "2026-07-28", "2026-07-01", "2026-07-28"},
	}
	for _, tc := range cases {
		result, _, err := findDecliningPages(context.Background(), client, findDecliningPagesInput{
			SiteURL:      "sc-domain:example.com",
			PeriodAStart: tc.aStart,
			PeriodAEnd:   tc.aEnd,
			PeriodBStart: tc.bStart,
			PeriodBEnd:   tc.bEnd,
		})
		if err != nil {
			t.Fatalf("%s: returned error: %v", tc.name, err)
		}
		if !result.IsError {
			t.Errorf("%s: expected invalid_input; period A must end before period B starts", tc.name)
			continue
		}
		if body := extractText(t, result.Content); !strings.Contains(body, "invalid_input") {
			t.Errorf("%s: %s", tc.name, body)
		}
	}
	if len(*seen) != 0 {
		t.Errorf("api calls = %d, want 0", len(*seen))
	}
}
