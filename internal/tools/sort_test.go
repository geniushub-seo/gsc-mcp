package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
)

// TestSortCompareRows_BestImproverSurvivesTruncation is the regression for a
// defect found in acceptance testing: asking for the biggest rank improvement
// used to return whichever high-click rows Google happened to send back. The
// shape here mirrors a measured real-world case — the true best improver has
// almost no clicks, so it sits far down GSC's clicks-desc order and was cut by
// row_limit before anything looked at position_change.
func TestSortCompareRows_BestImproverSurvivesTruncation(t *testing.T) {
	t.Parallel()

	// Ordered as GSC returns them: clicks descending.
	rows := []comparePeriodRow{
		{Keys: []string{"high-clicks-flat"}, ClicksDelta: -131, PositionChange: f64ptr(0.10)},
		{Keys: []string{"mid-clicks-small-gain"}, ClicksDelta: -67, PositionChange: f64ptr(-2.80)},
		{Keys: []string{"low-clicks-big-gain"}, ClicksDelta: 0, PositionChange: f64ptr(-90.78)},
		{Keys: []string{"only-in-b"}, ClicksDelta: 5, PositionChange: nil, OnlyIn: "b"},
	}

	sortCompareRows(rows, sortPositionChange, sortAsc)

	if got := rows[0].Keys[0]; got != "low-clicks-big-gain" {
		t.Fatalf("first row = %q, want low-clicks-big-gain (position_change -90.78)", got)
	}
	if got := *rows[0].PositionChange; got != -90.78 {
		t.Errorf("first row position_change = %v, want -90.78", got)
	}
	// A missing side is not a rank movement of zero; it must not outrank a real
	// improvement, so only_in rows sort last on a position key.
	if got := rows[len(rows)-1].Keys[0]; got != "only-in-b" {
		t.Errorf("last row = %q, want only-in-b", got)
	}
}

func TestSortCompareRows_ClicksDeltaBothDirections(t *testing.T) {
	t.Parallel()

	base := []comparePeriodRow{
		{Keys: []string{"grew"}, ClicksDelta: 40},
		{Keys: []string{"dropped"}, ClicksDelta: -25},
		{Keys: []string{"flat"}, ClicksDelta: 0},
	}

	desc := append([]comparePeriodRow(nil), base...)
	sortCompareRows(desc, sortClicksDelta, sortDesc)
	if desc[0].Keys[0] != "grew" {
		t.Errorf("desc first = %q, want grew", desc[0].Keys[0])
	}

	asc := append([]comparePeriodRow(nil), base...)
	sortCompareRows(asc, sortClicksDelta, sortAsc)
	if asc[0].Keys[0] != "dropped" {
		t.Errorf("asc first = %q, want dropped", asc[0].Keys[0])
	}
}

func TestSortAnalyticsRows_PositionAscAndNilsLast(t *testing.T) {
	t.Parallel()

	rows := []querySearchAnalyticsRow{
		{Keys: []string{"no-signal"}, Clicks: 0, Impressions: 0, Position: nil},
		{Keys: []string{"rank-9"}, Clicks: 500, Impressions: 9000, Position: f64ptr(9.0)},
		{Keys: []string{"rank-2"}, Clicks: 3, Impressions: 40, Position: f64ptr(2.0)},
	}

	sortAnalyticsRows(rows, sortPosition, sortAsc)

	if rows[0].Keys[0] != "rank-2" {
		t.Errorf("first = %q, want rank-2 (best rank despite fewest clicks)", rows[0].Keys[0])
	}
	// A row with no clicks and no impressions has no rank signal at all; ranking
	// it "best" would invent a position of 0.
	if rows[len(rows)-1].Keys[0] != "no-signal" {
		t.Errorf("last = %q, want no-signal", rows[len(rows)-1].Keys[0])
	}
}

func TestNormalizeSortBy_RejectsUnknownKey(t *testing.T) {
	t.Parallel()

	if _, err := normalizeSortBy("position_change", sortClicks, analyticsSortKeys); err.Code == "" {
		t.Error("compare_periods key accepted on query_search_analytics; keys must not be interchangeable")
	}
	got, err := normalizeSortBy("", sortClicks, analyticsSortKeys)
	if err.Code != "" || got != sortClicks {
		t.Errorf("empty sort_by = (%q, %v), want (clicks, no error)", got, err.Code)
	}
	if _, err := normalizeSortOrder("sideways", sortClicks); err.Code != gscclient.ErrInvalidInput {
		t.Errorf("sort_order error code = %q, want invalid_input", err.Code)
	}
	if got, _ := normalizeSortOrder("", sortPosition); got != sortAsc {
		t.Errorf("default order for position = %q, want asc", got)
	}
}

// TestComparePeriods_TruncationIsDeclared is a guard: it fails if the tool ever
// returns a truncated row set without saying so. Proven by injection in R14 —
// removing the `Truncated: truncated` assignment in compare_periods.go makes
// this fail with "truncated = false, want true".
func TestComparePeriods_TruncationIsDeclared(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		// Period A ranks "deep" worst; period B ranks it best. Its click volume
		// keeps it at the bottom of GSC's own ordering in both periods.
		pos := 95.11
		if n == 2 {
			pos = 4.33
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"rows":[
		  {"keys":["loud"],"clicks":500,"impressions":9000,"ctr":0.055,"position":3.0},
		  {"keys":["mid"],"clicks":50,"impressions":900,"ctr":0.055,"position":6.0},
		  {"keys":["deep"],"clicks":1,"impressions":80,"ctr":0.0125,"position":%v}
		]}`, pos)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	res, _, err := comparePeriods(context.Background(), client, comparePeriodsInput{
		SiteURL:      "sc-domain:example.com",
		PeriodAStart: "2026-06-15", PeriodAEnd: "2026-06-16",
		PeriodBStart: "2026-06-17", PeriodBEnd: "2026-06-18",
		Dimensions: []string{"query"},
		RowLimit:   1,
		SortBy:     sortPositionChange,
	})
	if err != nil {
		t.Fatalf("comparePeriods: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", extractText(t, res.Content))
	}

	var out comparePeriodsOutput
	if err := json.Unmarshal([]byte(extractText(t, res.Content)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if !out.Truncated {
		t.Error("truncated = false, want true (3 joined rows cut to row_limit 1)")
	}
	if out.RowsExamined != 3 {
		t.Errorf("rows_examined = %d, want 3", out.RowsExamined)
	}
	if out.RowCount != 1 || len(out.Rows) != 1 {
		t.Fatalf("row_count = %d / len(rows) = %d, want 1", out.RowCount, len(out.Rows))
	}
	if out.Ordering != "position_change asc" {
		t.Errorf("ordering = %q, want %q", out.Ordering, "position_change asc")
	}
	// The single surviving row must be the real best improver, not the loudest.
	if got := out.Rows[0].Keys[0]; got != "deep" {
		t.Errorf("surviving row = %q, want deep", got)
	}
}

// TestQuerySearchAnalytics_NonNativeSortScansBeyondRowLimit guards the
// over-fetch: sorting by anything other than clicks desc is only correct if the
// request asks Google for more rows than the caller wants back.
func TestQuerySearchAnalytics_NonNativeSortScansBeyondRowLimit(t *testing.T) {
	t.Parallel()

	var gotRowLimit int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RowLimit int64 `json:"rowLimit"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotRowLimit = body.RowLimit
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"rows":[{"keys":["a"],"clicks":5,"impressions":100,"ctr":0.05,"position":7.5}]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	res, _, err := querySearchAnalytics(context.Background(), client, querySearchAnalyticsInput{
		SiteURL:   "sc-domain:example.com",
		StartDate: "2026-06-15", EndDate: "2026-06-16",
		Dimensions: []string{"query"},
		RowLimit:   10,
		SortBy:     sortPosition,
	})
	if err != nil || res.IsError {
		t.Fatalf("querySearchAnalytics failed: %v", err)
	}
	if gotRowLimit != sortScanRowLimit {
		t.Errorf("request rowLimit = %d, want %d (over-fetch for non-native sort)", gotRowLimit, sortScanRowLimit)
	}

	var out querySearchAnalyticsOutput
	if err := json.Unmarshal([]byte(extractText(t, res.Content)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Ordering != "position asc" {
		t.Errorf("ordering = %q, want %q", out.Ordering, "position asc")
	}
	if out.Truncated {
		t.Error("truncated = true, want false (1 row examined, row_limit 10)")
	}
}

// TestQuerySearchAnalytics_NativeSortDoesNotOverFetch keeps the default path
// cheap: clicks desc is what GSC already returns, so there is nothing to sort.
func TestQuerySearchAnalytics_NativeSortDoesNotOverFetch(t *testing.T) {
	t.Parallel()

	var gotRowLimit int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RowLimit int64 `json:"rowLimit"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotRowLimit = body.RowLimit
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"rows":[]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	_, _, err := querySearchAnalytics(context.Background(), client, querySearchAnalyticsInput{
		SiteURL:   "sc-domain:example.com",
		StartDate: "2026-06-15", EndDate: "2026-06-16",
		RowLimit: 25,
	})
	if err != nil {
		t.Fatalf("querySearchAnalytics: %v", err)
	}
	if gotRowLimit != 26 {
		t.Errorf("request rowLimit = %d, want 26 (native clicks-desc peeks one extra row)", gotRowLimit)
	}
}

// TestQuerySearchAnalytics_BareDomainWithPathIsAnnounced guards the silent
// scope change: "example.com/blog" normalizes to the whole sc-domain property,
// so the output has to say the path was dropped.
func TestQuerySearchAnalytics_BareDomainWithPathIsAnnounced(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"rows":[{"keys":null,"clicks":618,"impressions":57881,"ctr":0.0107,"position":10.46}]}`)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	res, _, err := querySearchAnalytics(context.Background(), client, querySearchAnalyticsInput{
		SiteURL:   "example.com/blog",
		StartDate: "2026-06-15", EndDate: "2026-06-16",
	})
	if err != nil || res.IsError {
		t.Fatalf("querySearchAnalytics failed: %v", err)
	}

	var out querySearchAnalyticsOutput
	if err := json.Unmarshal([]byte(extractText(t, res.Content)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !strings.Contains(out.Note, "path was discarded") {
		t.Errorf("note = %q, want it to state the path was discarded", out.Note)
	}
	if !strings.Contains(out.Note, "sc-domain:example.com") {
		t.Errorf("note = %q, want it to name the property actually queried", out.Note)
	}
}

func TestQuerySearchAnalytics_NativePeekSetsTruncated(t *testing.T) {
	t.Parallel()

	var gotRowLimit, gotStartRow int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RowLimit int64 `json:"rowLimit"`
			StartRow int64 `json:"startRow"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotRowLimit = body.RowLimit
		gotStartRow = body.StartRow
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"rows":[
			{"keys":["a"],"clicks":9,"impressions":90,"ctr":0.1,"position":3},
			{"keys":["b"],"clicks":8,"impressions":80,"ctr":0.1,"position":4},
			{"keys":["c"],"clicks":7,"impressions":70,"ctr":0.1,"position":5}
		]}`)
	}))
	defer srv.Close()

	res, _, err := querySearchAnalytics(context.Background(), newTestClient(t, srv.URL+"/"), querySearchAnalyticsInput{
		SiteURL:   "sc-domain:example.com",
		StartDate: "2026-06-15", EndDate: "2026-06-16",
		RowLimit: 2,
	})
	if err != nil {
		t.Fatalf("querySearchAnalytics: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", extractText(t, res.Content))
	}
	if gotRowLimit != 3 {
		t.Errorf("request rowLimit = %d, want 3 (row_limit 2 + peek)", gotRowLimit)
	}
	if gotStartRow != 0 {
		t.Errorf("request startRow = %d, want 0", gotStartRow)
	}

	var out querySearchAnalyticsOutput
	if err := json.Unmarshal([]byte(extractText(t, res.Content)), &out); err != nil {
		t.Fatal(err)
	}
	if !out.Truncated {
		t.Error("truncated = false, want true (peek row proves more data exists)")
	}
	if out.ScanCapped {
		t.Error("scan_capped should be false when the API ceiling was not hit")
	}
	if out.RowCount != 2 || len(out.Rows) != 2 {
		t.Fatalf("row_count = %d / len = %d, want 2", out.RowCount, len(out.Rows))
	}
	if out.RowsExamined != 3 {
		t.Errorf("rows_examined = %d, want 3", out.RowsExamined)
	}
}

func TestQuerySearchAnalytics_NativeMaxRowLimitSetsScanCapped(t *testing.T) {
	t.Parallel()

	var gotRowLimit int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RowLimit int64 `json:"rowLimit"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotRowLimit = body.RowLimit
		rows := make([]map[string]any, sortScanRowLimit)
		for i := range rows {
			rows[i] = map[string]any{
				"keys": []string{fmt.Sprintf("q-%d", i)}, "clicks": sortScanRowLimit - i,
				"impressions": 10, "ctr": 0.1, "position": 5,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows})
	}))
	defer srv.Close()

	res, _, err := querySearchAnalytics(context.Background(), newTestClient(t, srv.URL+"/"), querySearchAnalyticsInput{
		SiteURL:   "sc-domain:example.com",
		StartDate: "2026-06-15", EndDate: "2026-06-16",
		RowLimit: sortScanRowLimit,
	})
	if err != nil {
		t.Fatalf("querySearchAnalytics: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", extractText(t, res.Content))
	}
	if gotRowLimit != sortScanRowLimit {
		t.Errorf("request rowLimit = %d, want %d (cannot peek past the API ceiling)", gotRowLimit, sortScanRowLimit)
	}

	var out querySearchAnalyticsOutput
	if err := json.Unmarshal([]byte(extractText(t, res.Content)), &out); err != nil {
		t.Fatal(err)
	}
	if !out.ScanCapped {
		t.Error("scan_capped = false, want true when native path fills the 25,000 cap")
	}
	if out.Truncated {
		t.Error("truncated = true, want false: we returned the full requested window")
	}
	if out.RowCount != sortScanRowLimit {
		t.Errorf("row_count = %d, want %d", out.RowCount, sortScanRowLimit)
	}
}

func TestQuerySearchAnalytics_StartRowNonNativeSortScansFromStart(t *testing.T) {
	t.Parallel()

	var gotRowLimit, gotStartRow int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RowLimit int64 `json:"rowLimit"`
			StartRow int64 `json:"startRow"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotRowLimit = body.RowLimit
		gotStartRow = body.StartRow
		// Clicks-desc order as Google returns it. The best rank is last.
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"rows":[
			{"keys":["loud"],"clicks":500,"impressions":9000,"ctr":0.055,"position":9.0},
			{"keys":["mid"],"clicks":50,"impressions":900,"ctr":0.055,"position":6.0},
			{"keys":["best"],"clicks":1,"impressions":80,"ctr":0.0125,"position":2.0}
		]}`)
	}))
	defer srv.Close()

	res, _, err := querySearchAnalytics(context.Background(), newTestClient(t, srv.URL+"/"), querySearchAnalyticsInput{
		SiteURL:   "sc-domain:example.com",
		StartDate: "2026-06-15", EndDate: "2026-06-16",
		Dimensions: []string{"query"},
		RowLimit:   1,
		StartRow:   1,
		SortBy:     sortPosition,
	})
	if err != nil {
		t.Fatalf("querySearchAnalytics: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", extractText(t, res.Content))
	}
	if gotStartRow != 0 {
		t.Errorf("request startRow = %d, want 0 (offset must be applied after local sort)", gotStartRow)
	}
	if gotRowLimit != sortScanRowLimit {
		t.Errorf("request rowLimit = %d, want %d", gotRowLimit, sortScanRowLimit)
	}

	var out querySearchAnalyticsOutput
	if err := json.Unmarshal([]byte(extractText(t, res.Content)), &out); err != nil {
		t.Fatal(err)
	}
	if out.RowCount != 1 || len(out.Rows) != 1 {
		t.Fatalf("row_count = %d / len = %d, want 1", out.RowCount, len(out.Rows))
	}
	// Full set sorted by position asc is best, mid, loud. Offset 1, limit 1 → mid.
	// Sending start_row to Google first would have dropped "loud" and returned "best".
	if got := out.Rows[0].Keys[0]; got != "mid" {
		t.Errorf("surviving row = %q, want mid (local sort then offset)", got)
	}
	if !out.Truncated {
		t.Error("truncated = false, want true (2 rows remained after offset)")
	}
	if out.ScanCapped {
		t.Error("scan_capped should be false for a 3-row fixture")
	}
	if out.RowsExamined != 3 {
		t.Errorf("rows_examined = %d, want 3 (scanned candidates before start_row)", out.RowsExamined)
	}
}

func TestApplyAnalyticsWindow_StartRowDoesNotShrinkExamined(t *testing.T) {
	t.Parallel()

	base := []querySearchAnalyticsRow{
		{Keys: []string{"a"}, Clicks: 3, Impressions: 30, Position: f64ptr(9)},
		{Keys: []string{"b"}, Clicks: 2, Impressions: 20, Position: f64ptr(6)},
		{Keys: []string{"c"}, Clicks: 1, Impressions: 10, Position: f64ptr(2)},
	}

	in := querySearchAnalyticsInput{RowLimit: 1, StartRow: 1, SortBy: sortPosition, SortOrder: sortAsc}
	rows := append([]querySearchAnalyticsRow(nil), base...)
	out, examined, truncated, scanCapped := applyAnalyticsWindow(rows, in, false)
	if examined != 3 {
		t.Errorf("examined = %d, want 3", examined)
	}
	if !truncated {
		t.Error("truncated = false, want true")
	}
	if scanCapped {
		t.Error("scan_capped should be false")
	}
	if len(out) != 1 || out[0].Keys[0] != "b" {
		t.Errorf("window = %+v, want the middle row after position sort + offset", out)
	}

	in.StartRow = 2
	rows = append([]querySearchAnalyticsRow(nil), base...)
	_, examined, truncated, _ = applyAnalyticsWindow(rows, in, false)
	if examined != 3 {
		t.Errorf("examined after last-page offset = %d, want 3", examined)
	}
	if truncated {
		t.Error("truncated should be false when the post-offset page fits row_limit")
	}
}

func TestApplyAnalyticsWindow_StartRowWithScanCap(t *testing.T) {
	t.Parallel()

	rows := make([]querySearchAnalyticsRow, sortScanRowLimit)
	for i := range rows {
		pos := float64(sortScanRowLimit - i)
		rows[i] = querySearchAnalyticsRow{
			Keys: []string{fmt.Sprintf("q-%d", i)}, Clicks: float64(i), Impressions: 10, Position: &pos,
		}
	}
	in := querySearchAnalyticsInput{
		RowLimit: 1, StartRow: sortScanRowLimit - 1, SortBy: sortPosition, SortOrder: sortAsc,
	}
	out, examined, truncated, scanCapped := applyAnalyticsWindow(rows, in, false)
	if examined != sortScanRowLimit {
		t.Errorf("examined = %d, want %d (scan size, not the leftover page)", examined, sortScanRowLimit)
	}
	if !scanCapped {
		t.Error("scan_capped = false, want true")
	}
	if truncated {
		t.Error("truncated should be false: only one row remains after offset")
	}
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
}

func TestDroppedPathNote_OnlyForBareDomainWithPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{"example.com/blog", true},
		{"example.com/blog/2026", true},
		{"example.com", false},
		{"example.com/", false},
		{"https://example.com/blog", false}, // keeps its path as a URL-prefix property
		{"sc-domain:example.com", false},
		{"", false},
	}
	for _, tc := range cases {
		got := gscclient.DroppedPathNote(tc.in) != ""
		if got != tc.want {
			t.Errorf("DroppedPathNote(%q) produced note = %v, want %v", tc.in, got, tc.want)
		}
	}
}
