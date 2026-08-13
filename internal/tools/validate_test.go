package tools

import (
	"slices"
	"strings"
	"testing"

	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
)

func TestValidateSearchAnalytics_EnumNormalization(t *testing.T) {
	t.Parallel()
	input := querySearchAnalyticsInput{
		SiteURL:               "example.com",
		StartDate:             "2026-07-01",
		EndDate:               "2026-07-31",
		Dimensions:            []string{"query", "page", "searchAppearance"},
		SearchType:            "googleNews",
		DataState:             "hourly_all",
		AggregationType:       "byPage",
		DimensionFilterGroups: []DimensionFilterGroup{{Filters: []DimensionFilter{{Dimension: "query", Operator: "excludingRegex", Expression: "brand"}}}},
	}

	out, err := validateSearchAnalytics(input)
	if err.Code != "" {
		t.Fatalf("unexpected validation error: %+v", err)
	}

	if out.SearchType != "GOOGLE_NEWS" {
		t.Errorf("SearchType = %q, want GOOGLE_NEWS", out.SearchType)
	}
	if out.DataState != "HOURLY_ALL" {
		t.Errorf("DataState = %q, want HOURLY_ALL", out.DataState)
	}
	if out.AggregationType != "BY_PAGE" {
		t.Errorf("AggregationType = %q, want BY_PAGE", out.AggregationType)
	}
	wantDims := []string{"QUERY", "PAGE", "SEARCH_APPEARANCE"}
	if !slices.Equal(out.Dimensions, wantDims) {
		t.Errorf("Dimensions = %v, want %v", out.Dimensions, wantDims)
	}
	if out.DimensionFilterGroups[0].Filters[0].Operator != "EXCLUDING_REGEX" {
		t.Errorf("Filter operator = %q, want EXCLUDING_REGEX", out.DimensionFilterGroups[0].Filters[0].Operator)
	}
}

func TestValidateSearchAnalytics_Defaults(t *testing.T) {
	t.Parallel()
	input := querySearchAnalyticsInput{
		SiteURL:   "example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-31",
	}

	out, err := validateSearchAnalytics(input)
	if err.Code != "" {
		t.Fatalf("unexpected validation error: %+v", err)
	}

	if out.SearchType != "" {
		t.Errorf("SearchType should stay empty when omitted, got %q", out.SearchType)
	}
	if out.DataState != "ALL" {
		t.Errorf("DataState default = %q, want ALL", out.DataState)
	}
	if out.AggregationType != "AUTO" {
		t.Errorf("AggregationType default = %q, want AUTO", out.AggregationType)
	}
	if out.RowLimit != 150 {
		t.Errorf("RowLimit default = %d, want 150", out.RowLimit)
	}
}

func TestValidateSearchAnalytics_RowLimitOutOfRange(t *testing.T) {
	t.Parallel()
	base := querySearchAnalyticsInput{
		SiteURL:   "example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-31",
	}

	cases := []struct {
		name     string
		rowLimit int
		wantErr  bool
	}{
		{"zero defaults", 0, false},
		{"minimum", 1, false},
		{"maximum", 25000, false},
		{"too high", 25001, true},
		{"negative", -1, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			input.RowLimit = tc.rowLimit
			_, err := validateSearchAnalytics(input)
			if tc.wantErr {
				if err.Code != gscclient.ErrInvalidInput {
					t.Fatalf("expected invalid_input, got %q", err.Code)
				}
				return
			}
			if err.Code != "" {
				t.Fatalf("unexpected error: %+v", err)
			}
		})
	}
}

func TestValidateSearchAnalytics_HourRequiresHourlyAll(t *testing.T) {
	t.Parallel()
	// Use a window inside the last 10 days so the hour-date guard does not fire first.
	today := gscTodayPT()
	end := today.Format("2006-01-02")
	start := today.AddDate(0, 0, -3).Format("2006-01-02")
	base := querySearchAnalyticsInput{
		SiteURL:    "example.com",
		StartDate:  start,
		EndDate:    end,
		Dimensions: []string{"hour"},
	}

	input := base
	input.DataState = ""
	_, err := validateSearchAnalytics(input)
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input without hourly_all, got %q", err.Code)
	}

	input.DataState = "hourly_all"
	_, err = validateSearchAnalytics(input)
	if err.Code != "" {
		t.Fatalf("expected valid with hourly_all, got %+v", err)
	}
}

func TestValidateSearchAnalytics_FilterDimensionInvalid(t *testing.T) {
	t.Parallel()
	base := querySearchAnalyticsInput{
		SiteURL:   "example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-31",
	}

	input := base
	input.DimensionFilterGroups = []DimensionFilterGroup{{
		Filters: []DimensionFilter{{Dimension: "date", Operator: "equals", Expression: "2026-07-01"}},
	}}
	_, err := validateSearchAnalytics(input)
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input for date filter, got %q", err.Code)
	}

	input.DimensionFilterGroups[0].Filters[0].Dimension = "query"
	_, err = validateSearchAnalytics(input)
	if err.Code != "" {
		t.Fatalf("expected valid for query filter, got %+v", err)
	}
}

func TestValidateSearchAnalytics_PageAggregationInvalid(t *testing.T) {
	t.Parallel()
	base := querySearchAnalyticsInput{
		SiteURL:         "example.com",
		StartDate:       "2026-07-01",
		EndDate:         "2026-07-31",
		Dimensions:      []string{"page"},
		AggregationType: "byProperty",
	}

	_, err := validateSearchAnalytics(base)
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input for page + byProperty, got %q", err.Code)
	}

	base.AggregationType = "byPage"
	_, err = validateSearchAnalytics(base)
	if err.Code != "" {
		t.Fatalf("expected valid for page + byPage, got %+v", err)
	}

	// Filtering by page also blocks byProperty.
	base = querySearchAnalyticsInput{
		SiteURL:   "example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-31",
		DimensionFilterGroups: []DimensionFilterGroup{{
			Filters: []DimensionFilter{{Dimension: "page", Operator: "equals", Expression: "https://example.com/"}},
		}},
		AggregationType: "byProperty",
	}
	_, err = validateSearchAnalytics(base)
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input for page filter + byProperty, got %q", err.Code)
	}
}

func TestValidateSearchAnalytics_DateRangeInvalid(t *testing.T) {
	t.Parallel()
	base := querySearchAnalyticsInput{
		SiteURL:   "example.com",
		StartDate: "2026-08-01",
		EndDate:   "2026-07-01",
	}

	_, err := validateSearchAnalytics(base)
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input for inverted dates, got %q", err.Code)
	}

	base.StartDate = "not-a-date"
	_, err = validateSearchAnalytics(base)
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input for malformed date, got %q", err.Code)
	}
}

func TestValidateSearchAnalytics_InvalidEnum(t *testing.T) {
	t.Parallel()
	base := querySearchAnalyticsInput{
		SiteURL:   "example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-31",
	}

	input := base
	input.SearchType = "social"
	_, err := validateSearchAnalytics(input)
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input for bad search_type, got %q", err.Code)
	}
}

func TestValidateSearchAnalytics_RegexOperators(t *testing.T) {
	t.Parallel()
	base := querySearchAnalyticsInput{
		SiteURL:   "example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-31",
	}

	// Four bad patterns that Google silently mishandles (§6-A).
	bad := []struct {
		name string
		expr string
	}{
		{"lookbehind", `(?<=查詢詞C)查詢詞D`},
		{"lookahead", `查詢詞C(?=查詢詞D)`},
		{"unclosed", `([unclosed`},
		{"backreference", `(a)\1`},
	}
	for _, tc := range bad {
		t.Run("bad_"+tc.name, func(t *testing.T) {
			input := base
			input.DimensionFilterGroups = []DimensionFilterGroup{{
				Filters: []DimensionFilter{{
					Dimension:  "query",
					Operator:   "excludingRegex",
					Expression: tc.expr,
				}},
			}}
			_, err := validateSearchAnalytics(input)
			if err.Code != gscclient.ErrInvalidInput {
				t.Fatalf("expected invalid_input for %s, got %+v", tc.name, err)
			}
			if err.Message == "" {
				t.Fatal("expected syntax detail in message")
			}
		})
	}

	// Four good patterns (§6-A).
	good := []struct {
		name string
		expr string
	}{
		{"chinese_alt", `品牌詞A|品牌詞B`},
		{"case_insensitive", `(?i)genius|seo`},
		{"class_quant", `^ig\s?(字體|reels)$`},
		{"han", `\p{Han}+`},
	}
	for _, tc := range good {
		t.Run("good_"+tc.name, func(t *testing.T) {
			input := base
			input.DimensionFilterGroups = []DimensionFilterGroup{{
				Filters: []DimensionFilter{{
					Dimension:  "query",
					Operator:   "includingRegex",
					Expression: tc.expr,
				}},
			}}
			_, err := validateSearchAnalytics(input)
			if err.Code != "" {
				t.Fatalf("expected OK for %s, got %+v", tc.name, err)
			}
		})
	}
}

func TestValidateSearchAnalytics_FilterTooLong(t *testing.T) {
	t.Parallel()
	// Expression large enough that the full serialized filter exceeds 4096.
	expr := strings.Repeat("a", 4100)
	input := querySearchAnalyticsInput{
		SiteURL:   "example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-31",
		DimensionFilterGroups: []DimensionFilterGroup{{
			Filters: []DimensionFilter{{
				Dimension:  "query",
				Operator:   "equals",
				Expression: expr,
			}},
		}},
	}
	_, err := validateSearchAnalytics(input)
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input for oversize filter, got %+v", err)
	}
	if !strings.Contains(err.Message, "4096") && !strings.Contains(err.Message, "characters") {
		t.Fatalf("message should mention length limit, got %q", err.Message)
	}
}

func TestValidateSearchAnalytics_DuplicateDimensions(t *testing.T) {
	t.Parallel()
	input := querySearchAnalyticsInput{
		SiteURL:    "example.com",
		StartDate:  "2026-07-01",
		EndDate:    "2026-07-31",
		Dimensions: []string{"query", "query"},
	}
	_, err := validateSearchAnalytics(input)
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input for duplicate dimensions, got %+v", err)
	}
	if !strings.Contains(err.Message, "duplicate") {
		t.Fatalf("message should mention duplicate, got %q", err.Message)
	}
}

func TestValidateSearchAnalytics_HourOutsideTenDays(t *testing.T) {
	t.Parallel()
	today := gscTodayPT()
	// Start 20 days ago — outside the documented 10-day hourly window.
	start := today.AddDate(0, 0, -20).Format("2006-01-02")
	end := today.AddDate(0, 0, -15).Format("2006-01-02")
	input := querySearchAnalyticsInput{
		SiteURL:    "example.com",
		StartDate:  start,
		EndDate:    end,
		Dimensions: []string{"hour"},
		DataState:  "hourly_all",
	}
	_, err := validateSearchAnalytics(input)
	if err.Code != gscclient.ErrInvalidInput {
		t.Fatalf("expected invalid_input for hour outside 10 days, got %+v", err)
	}
	if !strings.Contains(err.Message, "10") {
		t.Fatalf("message should mention 10 days, got %q", err.Message)
	}
}

func TestDateRangeNotes_FutureAndOld(t *testing.T) {
	t.Parallel()
	today := gscTodayPT()
	futureEnd := today.AddDate(0, 0, 30).Format("2006-01-02")
	start := today.AddDate(0, 0, -7).Format("2006-01-02")
	note := dateRangeNotes(start, futureEnd)
	if note == "" || !strings.Contains(note, "future") {
		t.Fatalf("expected future-date note, got %q", note)
	}

	oldStart := today.AddDate(-2, 0, 0).Format("2006-01-02")
	end := today.AddDate(0, 0, -4).Format("2006-01-02")
	note = dateRangeNotes(oldStart, end)
	if note == "" || !strings.Contains(note, "retention") {
		t.Fatalf("expected retention soft note for very old start, got %q", note)
	}

	// Recent range: no note.
	note = dateRangeNotes(start, end)
	if note != "" {
		t.Fatalf("expected no note for recent range, got %q", note)
	}
}
