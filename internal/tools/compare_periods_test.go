package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
)

func f64ptr(v float64) *float64 { return &v }

func TestCompareMetricRows_DeltasAndOnlyIn(t *testing.T) {
	t.Parallel()
	a := []querySearchAnalyticsRow{
		{Keys: []string{"foo"}, Clicks: 100, Impressions: 1000, Ctr: 0.10, Position: f64ptr(5.0)},
		{Keys: []string{"only-a"}, Clicks: 10, Impressions: 100, Ctr: 0.10, Position: f64ptr(8.0)},
	}
	b := []querySearchAnalyticsRow{
		{Keys: []string{"foo"}, Clicks: 150, Impressions: 1200, Ctr: 0.125, Position: f64ptr(3.0)},
		{Keys: []string{"only-b"}, Clicks: 20, Impressions: 200, Ctr: 0.10, Position: f64ptr(4.0)},
	}

	rows := compareMetricRows(a, b)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	byKey := map[string]comparePeriodRow{}
	for _, r := range rows {
		byKey[r.Keys[0]] = r
	}

	foo := byKey["foo"]
	if foo.ClicksDelta != 50 {
		t.Errorf("clicks_delta = %v, want 50", foo.ClicksDelta)
	}
	if foo.ClicksChange == nil || *foo.ClicksChange != 50 {
		t.Errorf("clicks_change_pct = %v, want 50", foo.ClicksChange)
	}
	if foo.ImpressionsDelta != 200 {
		t.Errorf("impressions_delta = %v, want 200", foo.ImpressionsDelta)
	}
	// 0.10 → 0.125 is +2.5 percentage points
	if foo.CtrDeltaPP != 2.5 {
		t.Errorf("ctr_delta_pp = %v, want 2.5", foo.CtrDeltaPP)
	}
	if foo.PositionChange == nil || *foo.PositionChange != -2.0 {
		t.Errorf("position_change = %v, want -2", foo.PositionChange)
	}
	if foo.PositionImproved == nil || !*foo.PositionImproved {
		t.Error("position_improved should be true when rank improves (lower number)")
	}
	if foo.OnlyIn != "" {
		t.Errorf("only_in = %q, want empty", foo.OnlyIn)
	}

	onlyA := byKey["only-a"]
	if onlyA.OnlyIn != "a" {
		t.Errorf("only-a only_in = %q, want a", onlyA.OnlyIn)
	}
	if onlyA.ClicksB != 0 || onlyA.ClicksDelta != -10 {
		t.Errorf("only-a clicks B/delta = %v/%v", onlyA.ClicksB, onlyA.ClicksDelta)
	}
	if onlyA.PositionChange != nil || onlyA.PositionImproved != nil {
		t.Error("only-a must omit position_change and position_improved")
	}
	if onlyA.PositionB != nil {
		t.Error("only-a must omit position_b")
	}
	if onlyA.PositionA == nil {
		t.Error("only-a should keep position_a")
	}

	onlyB := byKey["only-b"]
	if onlyB.OnlyIn != "b" {
		t.Errorf("only-b only_in = %q, want b", onlyB.OnlyIn)
	}
	if onlyB.ClicksA != 0 || onlyB.ClicksDelta != 20 {
		t.Errorf("only-b clicks A/delta = %v/%v", onlyB.ClicksA, onlyB.ClicksDelta)
	}
	if onlyB.PositionChange != nil || onlyB.PositionImproved != nil {
		t.Error("only-b must omit position_change and position_improved")
	}
	if onlyB.PositionA != nil {
		t.Error("only-b must omit position_a")
	}
}

func TestCompareMetricRows_CtrDeltaPPIsPercentagePoints(t *testing.T) {
	t.Parallel()
	a := []querySearchAnalyticsRow{{Keys: []string{"q"}, Ctr: 0.10}}
	b := []querySearchAnalyticsRow{{Keys: []string{"q"}, Ctr: 0.125}}
	rows := compareMetricRows(a, b)
	if rows[0].CtrDeltaPP != 2.5 {
		t.Fatalf("ctr_delta_pp = %v, want 2.5 percentage points", rows[0].CtrDeltaPP)
	}
}

func TestCompareMetricRows_OnlyInOmitsPositionDeltasInJSON(t *testing.T) {
	t.Parallel()
	a := []querySearchAnalyticsRow{{Keys: []string{"only-a"}, Position: f64ptr(8)}}
	b := []querySearchAnalyticsRow{{Keys: []string{"only-b"}, Position: f64ptr(12)}}
	rows := compareMetricRows(a, b)
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "position_change") {
		t.Fatalf("JSON must not contain position_change for only_in rows: %s", s)
	}
	if strings.Contains(s, "position_improved") {
		t.Fatalf("JSON must not contain position_improved for only_in rows: %s", s)
	}
}

func TestCompareMetricRows_NoFloatNoise(t *testing.T) {
	t.Parallel()
	// Positions already rounded as toAnalyticsRows would leave them.
	// Classic float residue on CTR: 0.125 - 0.10 = 0.024999...
	a := []querySearchAnalyticsRow{{Keys: []string{"q"}, Ctr: 0.10, Position: f64ptr(5.56)}}
	b := []querySearchAnalyticsRow{{Keys: []string{"q"}, Ctr: 0.125, Position: f64ptr(3.33)}}
	rows := compareMetricRows(a, b)
	raw, err := json.Marshal(rows[0])
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, "99999") || strings.Contains(s, "00000") {
		t.Fatalf("float noise in JSON: %s", s)
	}
	if rows[0].CtrDeltaPP != 2.5 {
		t.Errorf("ctr_delta_pp = %v, want 2.5", rows[0].CtrDeltaPP)
	}
	if rows[0].PositionChange == nil || *rows[0].PositionChange != -2.23 {
		t.Errorf("position_change = %v, want -2.23", rows[0].PositionChange)
	}
}

func TestCompareMetricRows_ZeroDenominator(t *testing.T) {
	t.Parallel()
	a := []querySearchAnalyticsRow{
		{Keys: []string{"zero"}, Clicks: 0, Impressions: 0, Ctr: 0, Position: nil},
	}
	b := []querySearchAnalyticsRow{
		{Keys: []string{"zero"}, Clicks: 10, Impressions: 100, Ctr: 0.1, Position: f64ptr(4)},
	}
	rows := compareMetricRows(a, b)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.ClicksChange != nil {
		t.Errorf("clicks_change_pct from zero baseline should be omitted, got %v", *r.ClicksChange)
	}
	if r.ClicksDelta != 10 {
		t.Errorf("clicks_delta = %v, want 10", r.ClicksDelta)
	}
}

func TestCompareMetricRows_PositionWorsened(t *testing.T) {
	t.Parallel()
	a := []querySearchAnalyticsRow{{Keys: []string{"x"}, Position: f64ptr(3)}}
	b := []querySearchAnalyticsRow{{Keys: []string{"x"}, Position: f64ptr(7)}}
	rows := compareMetricRows(a, b)
	if rows[0].PositionChange == nil || *rows[0].PositionChange != 4 {
		t.Errorf("position_change = %v, want 4", rows[0].PositionChange)
	}
	if rows[0].PositionImproved == nil || *rows[0].PositionImproved {
		t.Error("position_improved should be false when rank worsens")
	}
}

func TestComparePeriods_OutputIncludesPeriodDays(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rows": []map[string]any{
				{"keys": []string{"q"}, "clicks": 10, "impressions": 100, "ctr": 0.1, "position": 5},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	result, _, err := comparePeriods(context.Background(), client, comparePeriodsInput{
		SiteURL:      "sc-domain:example.com",
		PeriodAStart: "2026-06-01",
		PeriodAEnd:   "2026-06-28",
		PeriodBStart: "2026-07-01",
		PeriodBEnd:   "2026-07-28",
	})
	if err != nil {
		t.Fatalf("Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", extractText(t, result.Content))
	}
	body := extractText(t, result.Content)
	if !strings.Contains(body, `"period_a_days":28`) {
		t.Fatalf("missing period_a_days=28 in %s", body)
	}
	if !strings.Contains(body, `"period_b_days":28`) {
		t.Fatalf("missing period_b_days=28 in %s", body)
	}
}

func TestComparePeriods_UnequalLengthIsError(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, "http://unused.example/")
	result, _, err := comparePeriods(context.Background(), client, comparePeriodsInput{
		SiteURL:      "sc-domain:example.com",
		PeriodAStart: "2026-06-01",
		PeriodAEnd:   "2026-06-28",
		PeriodBStart: "2026-07-01",
		PeriodBEnd:   "2026-07-07",
	})
	if err != nil {
		t.Fatalf("Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for unequal periods")
	}
	body := extractText(t, result.Content)
	if !strings.Contains(body, "invalid_input") {
		t.Fatalf("expected invalid_input, got %s", body)
	}
}

func TestValidateComparePeriods_DefaultRowLimit(t *testing.T) {
	t.Parallel()
	out, err := validateComparePeriods(comparePeriodsInput{
		SiteURL:      "example.com",
		PeriodAStart: "2026-06-01",
		PeriodAEnd:   "2026-06-28",
		PeriodBStart: "2026-07-01",
		PeriodBEnd:   "2026-07-28",
	})
	if err.Code != "" {
		t.Fatalf("unexpected error: %+v", err)
	}
	if out.RowLimit != 100 {
		t.Errorf("row_limit default = %d, want 100", out.RowLimit)
	}
	if out.DataState != "ALL" {
		t.Errorf("data_state default = %q, want ALL", out.DataState)
	}
}

func TestValidateComparePeriods_UnequalLengthsRejected(t *testing.T) {
	t.Parallel()
	_, err := validateComparePeriods(comparePeriodsInput{
		SiteURL:      "example.com",
		PeriodAStart: "2026-06-01",
		PeriodAEnd:   "2026-06-28", // 28 days
		PeriodBStart: "2026-07-01",
		PeriodBEnd:   "2026-07-07", // 7 days
	})
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input, got %+v", err)
	}
	if !strings.Contains(err.Message, "period lengths must match") {
		t.Fatalf("message should mention period lengths, got %q", err.Message)
	}
	if !strings.Contains(err.Message, "28") || !strings.Contains(err.Message, "7") {
		t.Fatalf("message should include both day counts, got %q", err.Message)
	}
}

func TestValidateComparePeriods_EqualLengthsOK(t *testing.T) {
	t.Parallel()
	_, err := validateComparePeriods(comparePeriodsInput{
		SiteURL:      "example.com",
		PeriodAStart: "2026-06-01",
		PeriodAEnd:   "2026-06-28",
		PeriodBStart: "2026-07-01",
		PeriodBEnd:   "2026-07-28",
	})
	if err.Code != "" {
		t.Fatalf("unexpected error: %+v", err)
	}
}

func TestInclusiveDays(t *testing.T) {
	t.Parallel()
	days, err := inclusiveDays("2026-07-01", "2026-07-28")
	if err != nil {
		t.Fatal(err)
	}
	if days != 28 {
		t.Fatalf("days = %d, want 28", days)
	}
	days, err = inclusiveDays("2026-07-01", "2026-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if days != 1 {
		t.Fatalf("single day = %d, want 1", days)
	}
}

func TestDescComparePeriods_FirstSentenceIsBMinusA(t *testing.T) {
	t.Parallel()
	first := descComparePeriods
	if i := strings.Index(first, ". "); i >= 0 {
		first = first[:i+1]
	}
	if !strings.Contains(first, "B minus A") && !strings.Contains(first, "B − A") {
		t.Fatalf("first sentence must state B minus A, got %q", first)
	}
	if !strings.Contains(descComparePeriods, "baseline") {
		t.Error("description should mention baseline")
	}
}

func TestComparePeriodsSchema_LabelsBaselineAndCurrent(t *testing.T) {
	t.Parallel()
	schema := string(comparePeriodsInputSchema)
	for _, want := range []string{"baseline", "current", "B minus A"} {
		if !strings.Contains(schema, want) {
			t.Errorf("schema missing %q", want)
		}
	}
}
