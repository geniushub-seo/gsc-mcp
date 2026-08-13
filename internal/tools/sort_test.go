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
	if gotRowLimit != 25 {
		t.Errorf("request rowLimit = %d, want 25 (no over-fetch on the default ordering)", gotRowLimit)
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
