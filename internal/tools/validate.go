package tools

import (
	"encoding/json"
	"fmt"
	"regexp/syntax"
	"strings"
	"time"

	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
)

// maxFilterJSONLength is the Search Console API documented max length for a
// filter (characters). We measure the serialized filter object conservatively
// (not just the expression). Official wording is ambiguous between filter vs
// expression; exceeding this has been observed to yield HTTP 500 upstream.
const maxFilterJSONLength = 4096

// hourlyDataMaxDays is from the Search Console discovery document on HOUR:
// "Data is available up to 10 days.".
const hourlyDataMaxDays = 10

// DimensionFilterGroup is the tool-level representation of a group of
// dimension filters. It mirrors the Google API shape but uses LLM-friendly
// lower-case keys.
type DimensionFilterGroup struct {
	GroupType string            `json:"groupType,omitempty" jsonschema:"Filter logic within the group. Only 'and' is supported by the GSC API."`
	Filters   []DimensionFilter `json:"filters,omitempty"`
}

// DimensionFilter is a single dimension filter inside a group.
type DimensionFilter struct {
	Dimension  string `json:"dimension" jsonschema:"Dimension to filter on: query, page, country, device, or searchAppearance."`
	Operator   string `json:"operator" jsonschema:"Filter operator: equals, notEquals, contains, notContains, includingRegex, excludingRegex."`
	Expression string `json:"expression" jsonschema:"The value or regex to filter by."`
}

var (
	searchTypeNames = map[string]string{
		"web":        "WEB",
		"image":      "IMAGE",
		"video":      "VIDEO",
		"news":       "NEWS",
		"discover":   "DISCOVER",
		"googlenews": "GOOGLE_NEWS",
	}

	dataStateNames = map[string]string{
		"all":        "ALL",
		"final":      "FINAL",
		"hourly_all": "HOURLY_ALL",
	}

	aggregationTypeNames = map[string]string{
		"auto":                "AUTO",
		"byproperty":          "BY_PROPERTY",
		"bypage":              "BY_PAGE",
		"bynewsshowcasepanel": "BY_NEWS_SHOWCASE_PANEL",
	}

	dimensionNames = map[string]string{
		"query":            "QUERY",
		"page":             "PAGE",
		"country":          "COUNTRY",
		"device":           "DEVICE",
		"date":             "DATE",
		"hour":             "HOUR",
		"searchappearance": "SEARCH_APPEARANCE",
	}

	filterOperatorNames = map[string]string{
		"equals":          "EQUALS",
		"notequals":       "NOT_EQUALS",
		"contains":        "CONTAINS",
		"notcontains":     "NOT_CONTAINS",
		"includingregex":  "INCLUDING_REGEX",
		"excludingregex":  "EXCLUDING_REGEX",
	}

	filterDimensionNames = map[string]string{
		"query":            "QUERY",
		"page":             "PAGE",
		"country":          "COUNTRY",
		"device":           "DEVICE",
		"searchappearance": "SEARCH_APPEARANCE",
	}
)

// validateSearchAnalytics validates and normalizes a query_search_analytics
// input. It applies defaults only for data_state=all and row_limit=150 (and
// aggregation_type=auto when provided empty for constraint checks). search_type
// is left empty when omitted so the request body does not inject a local default.
// Enforces the five constraints from SPEC.md §3.1. The returned gscclient.Error
// has an empty Code when validation succeeds.
func validateSearchAnalytics(input querySearchAnalyticsInput) (querySearchAnalyticsInput, gscclient.Error) {
	out := input

	if input.SearchType != "" {
		norm, err := normalizeEnum("search_type", searchTypeNames, input.SearchType, "")
		if err.Code != "" {
			return out, err
		}
		out.SearchType = norm
	}

	norm, err := normalizeEnum("data_state", dataStateNames, input.DataState, "all")
	if err.Code != "" {
		return out, err
	}
	out.DataState = norm

	norm, err = normalizeEnum("aggregation_type", aggregationTypeNames, input.AggregationType, "auto")
	if err.Code != "" {
		return out, err
	}
	out.AggregationType = norm

	dims, err := normalizeDimensions(input.Dimensions)
	if err.Code != "" {
		return out, err
	}
	out.Dimensions = dims

	groups, err := normalizeFilterGroups(input.DimensionFilterGroups)
	if err.Code != "" {
		return out, err
	}
	out.DimensionFilterGroups = groups

	if err := validateRowLimit(&out); err.Code != "" {
		return out, err
	}

	if err := validateStartRow(&out); err.Code != "" {
		return out, err
	}

	sortBy, err := normalizeSortBy(input.SortBy, sortClicks, analyticsSortKeys)
	if err.Code != "" {
		return out, err
	}
	out.SortBy = sortBy
	sortOrder, err := normalizeSortOrder(input.SortOrder, sortBy)
	if err.Code != "" {
		return out, err
	}
	out.SortOrder = sortOrder

	if err := validateDates(input.StartDate, input.EndDate); err.Code != "" {
		return out, err
	}

	if err := validateHourDataState(out.Dimensions, out.DataState); err.Code != "" {
		return out, err
	}

	if err := validateHourDateWindow(out.Dimensions, input.StartDate, input.EndDate); err.Code != "" {
		return out, err
	}

	if err := validateFilterDimensions(out.DimensionFilterGroups); err.Code != "" {
		return out, err
	}

	if err := validatePageAggregation(out.Dimensions, out.DimensionFilterGroups, out.AggregationType); err.Code != "" {
		return out, err
	}

	return out, gscclient.Error{}
}

func normalizeEnum(name string, table map[string]string, value, defaultValue string) (string, gscclient.Error) {
	if value == "" {
		if defaultValue == "" {
			return "", gscclient.Error{}
		}
		return table[defaultValue], gscclient.Error{}
	}
	norm, ok := table[strings.ToLower(value)]
	if !ok {
		return "", gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("invalid %s: %q", name, value),
			fmt.Sprintf("use one of the supported %s values", name),
		)
	}
	return norm, gscclient.Error{}
}

func normalizeDimensions(dims []string) ([]string, gscclient.Error) {
	if len(dims) == 0 {
		return nil, gscclient.Error{}
	}
	out := make([]string, len(dims))
	seen := make(map[string]struct{}, len(dims))
	for i, d := range dims {
		norm, ok := dimensionNames[strings.ToLower(d)]
		if !ok {
			return nil, gscclient.NewError(
				gscclient.ErrInvalidInput,
				fmt.Sprintf("invalid dimension: %q", d),
				"supported dimensions are: query, page, country, device, date, hour, searchAppearance",
			)
		}
		if _, dup := seen[norm]; dup {
			return nil, gscclient.NewError(
				gscclient.ErrInvalidInput,
				fmt.Sprintf("duplicate dimension %q", strings.ToLower(d)),
				"each dimension may appear at most once in the dimensions array",
			)
		}
		seen[norm] = struct{}{}
		out[i] = norm
	}
	return out, gscclient.Error{}
}

func normalizeFilterGroups(groups []DimensionFilterGroup) ([]DimensionFilterGroup, gscclient.Error) {
	if len(groups) == 0 {
		return nil, gscclient.Error{}
	}
	out := make([]DimensionFilterGroup, len(groups))
	for i, g := range groups {
		groupType := g.GroupType
		if groupType == "" {
			groupType = "and"
		}
		normGroupType, ok := map[string]string{"and": "AND"}[strings.ToLower(groupType)]
		if !ok {
			return nil, gscclient.NewError(
				gscclient.ErrInvalidInput,
				fmt.Sprintf("invalid dimension_filter_groups groupType: %q", g.GroupType),
				"only 'and' is supported",
			)
		}
		out[i].GroupType = normGroupType

		if len(g.Filters) == 0 {
			return nil, gscclient.NewError(
				gscclient.ErrInvalidInput,
				"dimension_filter_groups contains a group with no filters",
				"each group must include at least one filter",
			)
		}

		filters := make([]DimensionFilter, len(g.Filters))
		for j, f := range g.Filters {
			if f.Dimension == "" {
				return nil, gscclient.NewError(
					gscclient.ErrInvalidInput,
					"dimension_filter_groups filter is missing dimension",
					"",
				)
			}
			if f.Operator == "" {
				return nil, gscclient.NewError(
					gscclient.ErrInvalidInput,
					"dimension_filter_groups filter is missing operator",
					"",
				)
			}
			if f.Expression == "" {
				return nil, gscclient.NewError(
					gscclient.ErrInvalidInput,
					"dimension_filter_groups filter is missing expression",
					"",
				)
			}

			dim, ok := filterDimensionNames[strings.ToLower(f.Dimension)]
			if !ok {
				return nil, gscclient.NewError(
					gscclient.ErrInvalidInput,
					fmt.Sprintf("invalid filter dimension: %q", f.Dimension),
					"filter dimension must be one of: query, page, country, device, searchAppearance",
				)
			}
			op, ok := filterOperatorNames[strings.ToLower(f.Operator)]
			if !ok {
				return nil, gscclient.NewError(
					gscclient.ErrInvalidInput,
					fmt.Sprintf("invalid filter operator: %q", f.Operator),
					"filter operator must be one of: equals, notEquals, contains, notContains, includingRegex, excludingRegex",
				)
			}

			if op == "INCLUDING_REGEX" || op == "EXCLUDING_REGEX" {
				if _, perr := syntax.Parse(f.Expression, syntax.Perl); perr != nil {
					return nil, gscclient.NewError(
						gscclient.ErrInvalidInput,
						fmt.Sprintf("invalid regex in dimension filter: %v", perr),
						"GSC filters use RE2 syntax (no lookbehind, lookahead, or backreferences); fix the expression or use equals/contains",
					)
				}
			}

			filt := DimensionFilter{
				Dimension:  dim,
				Operator:   op,
				Expression: f.Expression,
			}
			if n := serializedFilterLength(filt); n > maxFilterJSONLength {
				return nil, gscclient.NewError(
					gscclient.ErrInvalidInput,
					fmt.Sprintf("dimension filter is %d characters when serialized; maximum is %d", n, maxFilterJSONLength),
					"shorten the filter expression or split into multiple queries",
				)
			}
			filters[j] = filt
		}
		out[i].Filters = filters
	}
	return out, gscclient.Error{}
}

// serializedFilterLength returns the JSON length of a normalized filter object.
// Official docs cap filter length at 4096 characters; measuring the whole
// object is the conservative reading of that limit.
func serializedFilterLength(f DimensionFilter) int {
	b, err := json.Marshal(map[string]string{
		"dimension":  f.Dimension,
		"operator":   f.Operator,
		"expression": f.Expression,
	})
	if err != nil {
		// Fall back to expression length if marshal fails (should not happen).
		return len(f.Expression)
	}
	return len(b)
}

func validateRowLimit(out *querySearchAnalyticsInput) gscclient.Error {
	if out.RowLimit == 0 {
		out.RowLimit = 150
		return gscclient.Error{}
	}
	if out.RowLimit < 1 || out.RowLimit > 25000 {
		return gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("row_limit must be between 1 and 25,000, got %d", out.RowLimit),
			"reduce row_limit or omit it to use the default of 150; for export set row_limit explicitly up to 25000",
		)
	}
	return gscclient.Error{}
}

func validateStartRow(out *querySearchAnalyticsInput) gscclient.Error {
	if out.StartRow < 0 {
		return gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("start_row must be non-negative, got %d", out.StartRow),
			"",
		)
	}
	return gscclient.Error{}
}

func validateDates(start, end string) gscclient.Error {
	const layout = "2006-01-02"
	if start == "" {
		return gscclient.NewError(gscclient.ErrInvalidInput, "start_date is required", "use YYYY-MM-DD format")
	}
	if end == "" {
		return gscclient.NewError(gscclient.ErrInvalidInput, "end_date is required", "use YYYY-MM-DD format")
	}
	startTime, err := time.Parse(layout, start)
	if err != nil {
		return gscclient.NewError(gscclient.ErrInvalidInput, fmt.Sprintf("invalid start_date %q: expected YYYY-MM-DD", start), "")
	}
	endTime, err := time.Parse(layout, end)
	if err != nil {
		return gscclient.NewError(gscclient.ErrInvalidInput, fmt.Sprintf("invalid end_date %q: expected YYYY-MM-DD", end), "")
	}
	if startTime.After(endTime) {
		return gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("start_date %s must be before or equal to end_date %s", start, end),
			"",
		)
	}
	return gscclient.Error{}
}

func validateHourDataState(dims []string, dataState string) gscclient.Error {
	if !hasDimension(dims, "HOUR") {
		return gscclient.Error{}
	}
	if dataState != "HOURLY_ALL" {
		return gscclient.NewError(
			gscclient.ErrInvalidInput,
			"dimension 'hour' requires data_state='hourly_all'",
			"hourly data is only available for the most recent 10 days; set data_state to hourly_all",
		)
	}
	return gscclient.Error{}
}

// validateHourDateWindow rejects hour-dimension queries whose start is older
// than the API's documented "up to 10 days" window. Uses PT (America/Los_Angeles)
// to match Search Console date semantics.
func validateHourDateWindow(dims []string, start, end string) gscclient.Error {
	if !hasDimension(dims, "HOUR") {
		return gscclient.Error{}
	}
	const layout = "2006-01-02"
	startTime, err := time.Parse(layout, start)
	if err != nil {
		return gscclient.Error{} // validateDates already covers format
	}
	endTime, err := time.Parse(layout, end)
	if err != nil {
		return gscclient.Error{}
	}

	today := gscTodayPT()
	// Earliest day that still has hourly data: today − 10 (inclusive window of 10 days of history plus today depends on wording;
	// discovery says "up to 10 days" — reject when start is before today−10).
	cutoff := today.AddDate(0, 0, -hourlyDataMaxDays)
	if startTime.Before(cutoff) {
		return gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("dimension 'hour' only has data for the most recent %d days; start_date %s is before %s (PT)",
				hourlyDataMaxDays, start, cutoff.Format(layout)),
			"narrow start_date/end_date to the last 10 days (PT) or remove the hour dimension",
		)
	}
	if endTime.After(today) {
		return gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("dimension 'hour' end_date %s is after today %s (PT); hourly data has no future days",
				end, today.Format(layout)),
			"set end_date to today or earlier in PT",
		)
	}
	return gscclient.Error{}
}

func hasDimension(dims []string, want string) bool {
	for _, d := range dims {
		if d == want {
			return true
		}
	}
	return false
}

// gscTodayPT returns today's calendar date in Search Console's PT timezone.
func gscTodayPT() time.Time {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PST", -8*60*60)
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
}

// dateRangeNotes returns soft advisories for ranges that often yield empty
// results without the API erroring. Does not hard-block: retention window is
// still [需確認] in official sources, so we never reject on "16 months".
// joinNotes concatenates non-empty advisory notes with "; ", so callers can
// pass several independent sources without emitting empty separators.
func joinNotes(notes ...string) string {
	kept := make([]string, 0, len(notes))
	for _, n := range notes {
		if n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, "; ")
}

func dateRangeNotes(start, end string) string {
	const layout = "2006-01-02"
	startTime, err1 := time.Parse(layout, start)
	endTime, err2 := time.Parse(layout, end)
	if err1 != nil || err2 != nil {
		return ""
	}
	today := gscTodayPT()
	var notes []string
	if endTime.After(today) {
		notes = append(notes, "end_date is in the future (PT); Search Console has no data for future dates, so results may be empty")
	}
	if startTime.After(today) {
		notes = append(notes, "start_date is in the future (PT); Search Console has no data for future dates, so results may be empty")
	}
	// Soft retention hint without asserting an official constant as a hard limit.
	// ~500 days is intentionally wider than the commonly cited ~16 months so we
	// only warn on clearly historical ranges.
	softRetentionDays := 500
	if startTime.Before(today.AddDate(0, 0, -softRetentionDays)) {
		notes = append(notes, "start_date may be outside Search Console's data retention window; older ranges often return zero rows with no error")
	}
	return strings.Join(notes, "; ")
}

func validateFilterDimensions(groups []DimensionFilterGroup) gscclient.Error {
	for _, g := range groups {
		for _, f := range g.Filters {
			if f.Dimension == "DATE" || f.Dimension == "HOUR" {
				return gscclient.NewError(
					gscclient.ErrInvalidInput,
					fmt.Sprintf("filter dimension cannot be %q", strings.ToLower(f.Dimension)),
					"filter by date or hour is not supported; use start_date/end_date or the hour dimension instead",
				)
			}
		}
	}
	return gscclient.Error{}
}

func validatePageAggregation(dims []string, groups []DimensionFilterGroup, aggregationType string) gscclient.Error {
	if aggregationType != "BY_PROPERTY" {
		return gscclient.Error{}
	}
	for _, d := range dims {
		if d == "PAGE" {
			return gscclient.NewError(
				gscclient.ErrInvalidInput,
				"aggregation_type 'byProperty' is not allowed when grouping by page",
				"use 'auto' or 'byPage', or remove the 'page' dimension",
			)
		}
	}
	for _, g := range groups {
		for _, f := range g.Filters {
			if f.Dimension == "PAGE" {
				return gscclient.NewError(
					gscclient.ErrInvalidInput,
					"aggregation_type 'byProperty' is not allowed when filtering by page",
					"use 'auto' or 'byPage', or remove the 'page' filter",
				)
			}
		}
	}
	return gscclient.Error{}
}
