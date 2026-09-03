package tools

import (
	"math"
	"testing"
)

func floatPtr(v float64) *float64 { return &v }

func TestCTRBaseline_NeighbourWindowStaysNarrowWhenSamplesSuffice(t *testing.T) {
	t.Parallel()

	// Nine rows at rounded position 11 and nine at 20. With min_samples=5 the
	// window never has to leave position 11, so the baseline is position-matched.
	rows := make([]queryPageRow, 0, 18)
	for _, ctr := range []float64{0.01, 0.02, 0.03, 0.04, 0.05, 0.06, 0.07, 0.08, 0.09} {
		rows = append(rows, queryPageRow{Impressions: 1000, Ctr: ctr, Position: floatPtr(11.2)})
		rows = append(rows, queryPageRow{Impressions: 1000, Ctr: 0.9, Position: floatPtr(20.0)})
	}

	idx := newCTRBaselineIndex(rows, 100, 5)
	got, ok := idx.baseline(11.4, 0.05)
	if !ok {
		t.Fatal("baseline not available")
	}
	if got.Window != 0 {
		t.Errorf("window = %d, want 0 (enough peers at the exact position)", got.Window)
	}
	if got.Scope != baselineScopeNeighborhood {
		t.Errorf("scope = %q, want %q", got.Scope, baselineScopeNeighborhood)
	}
	if got.Samples != 8 {
		t.Errorf("samples = %d, want 8 (nine rows at position 11, minus the row being scored)", got.Samples)
	}
	if math.Abs(got.Value-0.05) > 1e-9 {
		t.Errorf("baseline = %v, want 0.05 (median of nine, not the mean pulled up by position 20)", got.Value)
	}
}

func TestCTRBaseline_WindowExpandsWhenSamplesAreShort(t *testing.T) {
	t.Parallel()

	// Two rows at 11, two at 12, two at 13. min_samples=5 forces the window out
	// to w=1 (positions 10–12 gives 4, still short) and then w=2.
	rows := []queryPageRow{
		{Impressions: 500, Ctr: 0.010, Position: floatPtr(11.0)},
		{Impressions: 500, Ctr: 0.020, Position: floatPtr(11.4)},
		{Impressions: 500, Ctr: 0.030, Position: floatPtr(12.0)},
		{Impressions: 500, Ctr: 0.040, Position: floatPtr(12.3)},
		{Impressions: 500, Ctr: 0.050, Position: floatPtr(13.0)},
		{Impressions: 500, Ctr: 0.060, Position: floatPtr(13.4)},
	}

	idx := newCTRBaselineIndex(rows, 100, 5)
	got, ok := idx.baseline(11.2, 0.010)
	if !ok {
		t.Fatal("baseline not available")
	}
	if got.Window != 2 {
		t.Errorf("window = %d, want 2 (needed positions 9–13 to reach 5 peers)", got.Window)
	}
	if got.Samples != 5 {
		t.Errorf("samples = %d, want 5 (all six rows minus the one being scored)", got.Samples)
	}
	// The window reached both ends of the observed range, so the baseline is no
	// longer position-matched and must say so.
	if got.Scope != baselineScopeSiteWide {
		t.Errorf("scope = %q, want %q", got.Scope, baselineScopeSiteWide)
	}
	if math.Abs(got.Value-0.04) > 1e-9 {
		t.Errorf("baseline = %v, want 0.04 (median of the five peers, excluding the scored row's own 0.010)", got.Value)
	}
}

func TestCTRBaseline_DegradesToSiteWideOnTinyProperty(t *testing.T) {
	t.Parallel()

	// Three queries total: the window swallows the whole property and the label
	// has to admit the baseline is not position-matched.
	rows := []queryPageRow{
		{Impressions: 200, Ctr: 0.01, Position: floatPtr(3.0)},
		{Impressions: 200, Ctr: 0.02, Position: floatPtr(14.0)},
		{Impressions: 200, Ctr: 0.03, Position: floatPtr(28.0)},
	}
	idx := newCTRBaselineIndex(rows, 100, 10)
	got, ok := idx.baseline(14.0, 0.02)
	if !ok {
		t.Fatal("baseline not available")
	}
	if got.Scope != baselineScopeSiteWide {
		t.Fatalf("scope = %q, want %q", got.Scope, baselineScopeSiteWide)
	}
	if got.Samples != 2 || math.Abs(got.Value-0.02) > 1e-9 {
		t.Errorf("samples/value = %d/%v, want 2/0.02 (the two other rows)", got.Samples, got.Value)
	}
}

func TestCTRBaseline_PoolExcludesLowImpressionRows(t *testing.T) {
	t.Parallel()

	// A single-impression row that happened to get a click has CTR 1.0. Letting
	// it into the pool would drag the median up and mark healthy rows as gaps.
	rows := []queryPageRow{
		{Impressions: 1, Ctr: 1.0, Position: floatPtr(11.0)},
		{Impressions: 500, Ctr: 0.02, Position: floatPtr(11.0)},
		{Impressions: 500, Ctr: 0.03, Position: floatPtr(11.0)},
	}
	idx := newCTRBaselineIndex(rows, 100, 2)
	if idx.pool != 2 {
		t.Fatalf("pool = %d, want 2 (the 1-impression row must be excluded)", idx.pool)
	}
	// Scoring the 0.02 row leaves only the 0.03 row as a peer.
	got, ok := idx.baseline(11.0, 0.02)
	if !ok {
		t.Fatal("baseline not available")
	}
	if math.Abs(got.Value-0.03) > 1e-9 {
		t.Errorf("baseline = %v, want 0.03", got.Value)
	}
}

func TestCTRBaseline_EmptyPoolReportsUnavailable(t *testing.T) {
	t.Parallel()

	rows := []queryPageRow{{Impressions: 5, Ctr: 0.5, Position: floatPtr(11.0)}}
	idx := newCTRBaselineIndex(rows, 100, 5)
	if _, ok := idx.baseline(11.0, 0.5); ok {
		t.Error("baseline reported as available with an empty pool; it must not invent a number")
	}
}

func TestCTRBaseline_RowIsNeverItsOwnPeer(t *testing.T) {
	t.Parallel()

	// The degenerate case: one row at its position, min_samples=1. Counting the
	// row itself would hand back baseline_ctr == ctr, which reads as "exactly
	// average" and can never be reported as a gap. There is no peer, so the
	// honest answer is "no baseline".
	lone := []queryPageRow{{Impressions: 500, Ctr: 0.004, Position: floatPtr(11.0)}}
	idx := newCTRBaselineIndex(lone, 100, 1)
	if got, ok := idx.baseline(11.0, 0.004); ok {
		t.Errorf("baseline = %+v, want unavailable: the only row in the property has no peers", got)
	}

	// With one genuine peer, the peer alone is the baseline — not the average of
	// the pair, which would halve the measured gap.
	pair := []queryPageRow{
		{Impressions: 500, Ctr: 0.004, Position: floatPtr(11.0)},
		{Impressions: 500, Ctr: 0.050, Position: floatPtr(11.0)},
	}
	idx = newCTRBaselineIndex(pair, 100, 1)
	got, ok := idx.baseline(11.0, 0.004)
	if !ok {
		t.Fatal("baseline not available")
	}
	if math.Abs(got.Value-0.05) > 1e-9 || got.Samples != 1 {
		t.Errorf("baseline = %v from %d peers, want 0.05 from 1", got.Value, got.Samples)
	}
}

func TestMedianExcludingOne(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		sorted  []float64
		exclude float64
		want    float64
		wantN   int
		wantOK  bool
	}{
		{"drops the lowest", []float64{0.01, 0.02, 0.03, 0.04}, 0.01, 0.03, 3, true},
		{"drops the highest", []float64{0.01, 0.02, 0.03, 0.04}, 0.04, 0.02, 3, true},
		{"drops a middle value", []float64{0.01, 0.02, 0.03, 0.04, 0.05}, 0.03, 0.03, 4, true},
		{"drops one of a duplicated pair", []float64{0.02, 0.02, 0.02}, 0.02, 0.02, 2, true},
		{"absent value removes nothing", []float64{0.01, 0.02, 0.03}, 0.09, 0.02, 3, true},
		{"nothing left", []float64{0.02}, 0.02, 0, 0, false},
	}
	for _, tc := range cases {
		got, n, ok := medianExcludingOne(tc.sorted, tc.exclude)
		if ok != tc.wantOK || n != tc.wantN || (ok && math.Abs(got-tc.want) > 1e-9) {
			t.Errorf("%s: median=%v n=%d ok=%v, want %v/%d/%v", tc.name, got, n, ok, tc.want, tc.wantN, tc.wantOK)
		}
	}
}

func TestIsReportableDecline_IncludesTheBoundary(t *testing.T) {
	t.Parallel()

	// min_click_drop is documented as a minimum click loss, so a page down
	// exactly that many clicks qualifies.
	if !isReportableDecline(-10, 10) {
		t.Error("a page down exactly min_click_drop clicks must be reported")
	}
	if !isReportableDecline(-11, 10) {
		t.Error("a page down more than min_click_drop clicks must be reported")
	}
	if isReportableDecline(-9, 10) {
		t.Error("a page down less than min_click_drop clicks must not be reported")
	}
	if isReportableDecline(50, 10) {
		t.Error("a page that gained clicks is not a decline")
	}
}

func TestMedian(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		values []float64
		want   float64
	}{
		{"odd", []float64{0.03, 0.01, 0.02}, 0.02},
		{"even", []float64{0.04, 0.01, 0.02, 0.03}, 0.025},
		{"single", []float64{0.07}, 0.07},
		{"empty", nil, 0},
		{"ignores right-skew", []float64{0.01, 0.02, 0.03, 0.9, 0.95}, 0.03},
	}
	for _, tc := range cases {
		if got := median(tc.values); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: median = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPagesPerQuery_FlagsCannibalization(t *testing.T) {
	t.Parallel()

	rows := []queryPageRow{
		{Query: "blue widgets", Page: "https://example.com/a/"},
		{Query: "blue widgets", Page: "https://example.com/b/"},
		{Query: "blue widgets", Page: "https://example.com/b/"},
		{Query: "red widgets", Page: "https://example.com/c/"},
	}
	got := pagesPerQuery(rows)
	if got["blue widgets"] != 2 {
		t.Errorf("blue widgets pages = %d, want 2 (duplicate page must not double-count)", got["blue widgets"])
	}
	if got["red widgets"] != 1 {
		t.Errorf("red widgets pages = %d, want 1", got["red widgets"])
	}
}

func TestAttributeDecline(t *testing.T) {
	t.Parallel()

	const (
		posThreshold = 1.0
		impThreshold = -10.0
	)
	cases := []struct {
		name       string
		posA, posB *float64
		impPct     *float64
		ctrA, ctrB float64
		want       string
	}{
		{
			name: "rank loss beats everything downstream of it",
			posA: floatPtr(5.0), posB: floatPtr(9.5), impPct: floatPtr(-2),
			ctrA: 0.05, ctrB: 0.03, want: attributionPositionDrop,
		},
		{
			name: "impressions collapsed while rank held",
			posA: floatPtr(8.0), posB: floatPtr(8.2), impPct: floatPtr(-50),
			ctrA: 0.05, ctrB: 0.05, want: attributionImpressionsDrop,
		},
		{
			name: "rank and impressions held, ctr fell",
			posA: floatPtr(6.0), posB: floatPtr(6.1), impPct: floatPtr(-2.5),
			ctrA: 0.05, ctrB: 0.031, want: attributionCtrDrop,
		},
		{
			name: "page vanished: no position on one side",
			posA: floatPtr(7.0), posB: nil, impPct: floatPtr(-100),
			ctrA: 0.05, ctrB: 0, want: attributionUnclear,
		},
		{
			name: "nothing moved except clicks",
			posA: floatPtr(6.0), posB: floatPtr(6.0), impPct: floatPtr(0),
			ctrA: 0.05, ctrB: 0.05, want: attributionUnclear,
		},
		{
			name: "rank improved sharply yet clicks fell",
			posA: floatPtr(9.0), posB: floatPtr(4.0), impPct: floatPtr(-5),
			ctrA: 0.05, ctrB: 0.04, want: attributionUnclear,
		},
		{
			name: "exactly at the position threshold is not a rank loss",
			posA: floatPtr(5.0), posB: floatPtr(6.0), impPct: floatPtr(-40),
			ctrA: 0.05, ctrB: 0.05, want: attributionImpressionsDrop,
		},
	}
	for _, tc := range cases {
		got := attributeDecline(tc.posA, tc.posB, tc.impPct, tc.ctrA, tc.ctrB, posThreshold, impThreshold)
		if got != tc.want {
			t.Errorf("%s: attribution = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestClampPeriodsToComplete(t *testing.T) {
	t.Parallel()

	t.Run("trims both ends by the same number of days", func(t *testing.T) {
		aEnd, bEnd, removed, err := clampPeriodsToComplete(
			"2026-06-24", "2026-07-21", "2026-07-22", "2026-08-18", "2026-08-17")
		if err.Code != "" {
			t.Fatalf("unexpected error: %v", err)
		}
		if removed != 2 {
			t.Errorf("removed = %d, want 2 (2026-08-17 and 2026-08-18)", removed)
		}
		if bEnd != "2026-08-16" {
			t.Errorf("period B end = %q, want 2026-08-16", bEnd)
		}
		if aEnd != "2026-07-19" {
			t.Errorf("period A end = %q, want 2026-07-19 (equal length matters more than the requested date)", aEnd)
		}
	})

	t.Run("no metadata means no change", func(t *testing.T) {
		aEnd, bEnd, removed, err := clampPeriodsToComplete(
			"2026-06-24", "2026-07-21", "2026-07-22", "2026-08-18", "")
		if err.Code != "" || removed != 0 || aEnd != "2026-07-21" || bEnd != "2026-08-18" {
			t.Errorf("got %s/%s removed=%d err=%v, want the requested range untouched", aEnd, bEnd, removed, err)
		}
	})

	t.Run("range already complete means no change", func(t *testing.T) {
		_, bEnd, removed, err := clampPeriodsToComplete(
			"2026-06-24", "2026-07-21", "2026-07-22", "2026-08-14", "2026-08-17")
		if err.Code != "" || removed != 0 || bEnd != "2026-08-14" {
			t.Errorf("got %s removed=%d err=%v, want untouched", bEnd, removed, err)
		}
	})

	t.Run("period entirely inside the incomplete window is an error", func(t *testing.T) {
		_, _, _, err := clampPeriodsToComplete(
			"2026-08-15", "2026-08-16", "2026-08-17", "2026-08-18", "2026-08-17")
		if err.Code == "" {
			t.Fatal("expected invalid_input: there is no complete data to compare")
		}
	})
}

func TestValidateComputedRowLimit(t *testing.T) {
	t.Parallel()

	if got, err := validateComputedRowLimit(0); err.Code != "" || got != computedDefaultRowLimit {
		t.Errorf("default = %d err=%v, want %d", got, err, computedDefaultRowLimit)
	}
	if _, err := validateComputedRowLimit(maxComputedRowLimit + 1); err.Code == "" {
		t.Error("expected invalid_input above the ceiling")
	}
	if _, err := validateComputedRowLimit(-1); err.Code == "" {
		t.Error("expected invalid_input below 1")
	}
}
