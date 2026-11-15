package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/api/searchconsole/v1"
)

// findDecliningPagesInput is the input schema for find_declining_pages.
// Period A is the baseline (earlier); period B is the current (later). All
// deltas are B minus A, matching compare_periods.
type findDecliningPagesInput struct {
	SiteURL                     string                 `json:"site_url" jsonschema:"The GSC property to query."`
	PeriodAStart                string                 `json:"period_a_start" jsonschema:"Start date of period A — baseline/earlier period (YYYY-MM-DD, PT)."`
	PeriodAEnd                  string                 `json:"period_a_end" jsonschema:"End date of period A — baseline/earlier period (YYYY-MM-DD, PT)."`
	PeriodBStart                string                 `json:"period_b_start" jsonschema:"Start date of period B — current/later period (YYYY-MM-DD, PT)."`
	PeriodBEnd                  string                 `json:"period_b_end" jsonschema:"End date of period B — current/later period (YYYY-MM-DD, PT)."`
	SearchType                  string                 `json:"search_type,omitempty" jsonschema:"Optional search type: web, image, video, news, discover, or googleNews."`
	DimensionFilterGroups       []DimensionFilterGroup `json:"dimension_filter_groups,omitempty" jsonschema:"Optional dimension filters applied to both periods. Use operator excludingRegex on dimension query for non-brand declines."`
	MinClickDrop                int                    `json:"min_click_drop,omitempty" jsonschema:"Optional minimum click loss (positive number) for a page to be reported. Default 10."`
	PositionDropThreshold       float64                `json:"position_drop_threshold,omitempty" jsonschema:"Optional average-position worsening that counts as a ranking loss. Default 1."`
	ImpressionsDropPctThreshold float64                `json:"impressions_drop_pct_threshold,omitempty" jsonschema:"Optional impressions change percent (negative) below which a decline is attributed to lost impressions. Default -10."`
	RowLimit                    int                    `json:"row_limit,omitempty" jsonschema:"Optional maximum ranked rows to return. Default 50, maximum 1000."`
	DataState                   string                 `json:"data_state,omitempty" jsonschema:"Optional data freshness: all (default) or final."`
}

type findDecliningPagesOutput struct {
	QueriedAt string `json:"queried_at"`
	SiteURL   string `json:"site_url"`
	Note      string `json:"note,omitempty"`
	Ordering  string `json:"ordering"`
	// RowsExamined is the size of the joined page set before filtering.
	RowsExamined int  `json:"rows_examined"`
	ScanCapped   bool `json:"scan_capped,omitempty"`
	// RowsMatched is how many pages met the click-drop threshold before truncation.
	RowsMatched int                     `json:"rows_matched"`
	Truncated   bool                    `json:"truncated"`
	RowCount    int                     `json:"row_count"`
	Assumptions decliningAssumptions    `json:"assumptions"`
	Rows        []findDecliningPagesRow `json:"rows"`
}

type decliningAssumptions struct {
	// The period_* fields are the ranges actually queried, after any incomplete
	// days were trimmed. Compare them with what was requested.
	PeriodAStart string `json:"period_a_start"`
	PeriodAEnd   string `json:"period_a_end"`
	PeriodBStart string `json:"period_b_start"`
	PeriodBEnd   string `json:"period_b_end"`
	PeriodDays   int    `json:"period_days"`
	// RequestedPeriodAEnd / RequestedPeriodBEnd are only present when the ends
	// were trimmed, so a caller can see exactly what was dropped.
	RequestedPeriodAEnd string `json:"requested_period_a_end,omitempty"`
	RequestedPeriodBEnd string `json:"requested_period_b_end,omitempty"`
	// FirstIncompleteDate is Google's own marker for where the data stops being
	// final; ExcludedIncompleteDays is how many days each period lost to it.
	FirstIncompleteDate         string  `json:"first_incomplete_date,omitempty"`
	ExcludedIncompleteDays      int     `json:"excluded_incomplete_days"`
	MinClickDrop                int     `json:"min_click_drop"`
	RowLimit                    int     `json:"row_limit"`
	PositionDropThreshold       float64 `json:"position_drop_threshold"`
	ImpressionsDropPctThreshold float64 `json:"impressions_drop_pct_threshold"`
	DataState                   string  `json:"data_state"`
	Dimensions                  string  `json:"dimensions"`
	Attribution                 string  `json:"attribution"`
}

type findDecliningPagesRow struct {
	Page string `json:"page"`

	ClicksA         float64  `json:"clicks_a"`
	ClicksB         float64  `json:"clicks_b"`
	ClicksDelta     float64  `json:"clicks_delta"`
	ClicksChangePct *float64 `json:"clicks_change_pct,omitempty"`

	ImpressionsA         float64  `json:"impressions_a"`
	ImpressionsB         float64  `json:"impressions_b"`
	ImpressionsDelta     float64  `json:"impressions_delta"`
	ImpressionsChangePct *float64 `json:"impressions_change_pct,omitempty"`

	CtrA       float64 `json:"ctr_a"`
	CtrB       float64 `json:"ctr_b"`
	CtrDeltaPP float64 `json:"ctr_delta_pp"`

	// Position fields are omitted when the page has no rank signal on that side.
	PositionA      *float64 `json:"position_a,omitempty"`
	PositionB      *float64 `json:"position_b,omitempty"`
	PositionChange *float64 `json:"position_change,omitempty"`

	// Attribution is position_drop, impressions_drop, ctr_drop, or unclear.
	// The three causes need opposite responses, which is why this tool exists.
	Attribution string `json:"attribution"`
	// OnlyIn is 'a' when the page vanished from period B, 'b' when it is new.
	OnlyIn string `json:"only_in,omitempty"`
}

// findDecliningPagesInputSchema is written by hand so array fields have
// "type":"array" instead of the SDK-inferred ["null","array"] union.
var findDecliningPagesInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "site_url": {
      "type": "string",
      "description": "The GSC property to query. Accepts bare domain, full URL, or sc-domain: form."
    },
    "period_a_start": {
      "type": "string",
      "description": "Start date of period A — baseline/earlier period (YYYY-MM-DD, PT). All deltas are B minus A."
    },
    "period_a_end": {
      "type": "string",
      "description": "End date of period A — baseline/earlier period (YYYY-MM-DD, PT)."
    },
    "period_b_start": {
      "type": "string",
      "description": "Start date of period B — current/later period (YYYY-MM-DD, PT). All deltas are B minus A."
    },
    "period_b_end": {
      "type": "string",
      "description": "End date of period B — current/later period (YYYY-MM-DD, PT). Trimmed automatically when it reaches into Google's incomplete window."
    },
    "search_type": {
      "type": "string",
      "description": "Optional search type: web, image, video, news, discover, or googleNews. Omit to let Google default to web."
    },
    "dimension_filter_groups": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "groupType": {
            "type": "string",
            "description": "Filter logic within the group. Only 'and' is supported by the GSC API."
          },
          "filters": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "dimension": {
                  "type": "string",
                  "description": "Dimension to filter on: query, page, country, device, or searchAppearance."
                },
                "operator": {
                  "type": "string",
                  "description": "Filter operator: equals, notEquals, contains, notContains, includingRegex, excludingRegex."
                },
                "expression": {
                  "type": "string",
                  "description": "The value or regex to filter by."
                }
              },
              "required": ["dimension", "operator", "expression"]
            }
          }
        }
      },
      "description": "Optional dimension filters applied to both periods. Use operator excludingRegex on dimension query for non-brand declines."
    },
    "min_click_drop": {
      "type": "integer",
      "description": "Optional minimum click loss (positive number) for a page to be reported. Default 10."
    },
    "position_drop_threshold": {
      "type": "number",
      "description": "Optional average-position worsening that counts as a ranking loss. Default 1."
    },
    "impressions_drop_pct_threshold": {
      "type": "number",
      "description": "Optional impressions change percent (negative) below which a decline is attributed to lost impressions. Default -10."
    },
    "row_limit": {
      "type": "integer",
      "description": "Optional maximum ranked rows to return. Default 50, maximum 1000."
    },
    "data_state": {
      "type": "string",
      "description": "Optional data freshness: all (default, matches the GSC console) or final."
    }
  },
  "required": ["site_url", "period_a_start", "period_a_end", "period_b_start", "period_b_end"]
}`)

func registerFindDecliningPages(srv *mcp.Server, client *gscclient.Client) {
	mcp.AddTool(srv,
		&mcp.Tool{
			Name:        "find_declining_pages",
			Description: descFindDecliningPages,
			InputSchema: findDecliningPagesInputSchema,
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, input findDecliningPagesInput) (*mcp.CallToolResult, any, error) {
			return findDecliningPages(ctx, client, input)
		},
	)
}

func findDecliningPages(ctx context.Context, client *gscclient.Client, input findDecliningPagesInput) (*mcp.CallToolResult, any, error) {
	normalized, vErr := validateFindDecliningPages(input)
	if vErr.Code != "" {
		return toolError(vErr), nil, nil
	}

	var resolvedSiteURL string
	var rowsA, rowsB []*searchconsole.ApiDataRow
	var firstIncompleteDate string
	var clampErr gscclient.Error
	aEnd, bEnd := normalized.PeriodAEnd, normalized.PeriodBEnd
	excludedDays := 0

	_, err := gscclient.WithSiteURL(ctx, client, normalized.SiteURL, func(ctx context.Context, resolved string) (struct{}, error) {
		resolvedSiteURL = resolved

		// Incomplete days are the most likely way this tool produces a wrong
		// answer: counting the last two or three days of period B against a
		// complete period A manufactures a decline. Google returns the exact
		// boundary, so ask for it before querying anything else.
		date, perr := probeFirstIncompleteDate(ctx, client, resolved, normalized.PeriodAStart, normalized.PeriodBEnd, normalized.SearchType, normalized.DataState)
		if perr != nil {
			// Unlike the other computed tools this is fatal: without the boundary
			// the whole premise ("this is a real decline") is unverified.
			return struct{}{}, perr
		}
		firstIncompleteDate = date

		newAEnd, newBEnd, removed, cerr := clampPeriodsToComplete(
			normalized.PeriodAStart, normalized.PeriodAEnd,
			normalized.PeriodBStart, normalized.PeriodBEnd, date)
		if cerr.Code != "" {
			clampErr = cerr
			return struct{}{}, nil
		}
		aEnd, bEnd, excludedDays = newAEnd, newBEnd, removed

		reqA := buildDecliningQueryRequest(normalized, normalized.PeriodAStart, aEnd)
		respA, qerr := client.QuerySearchAnalytics(ctx, resolved, reqA)
		if qerr != nil {
			return struct{}{}, qerr
		}
		rowsA = respA.Rows

		reqB := buildDecliningQueryRequest(normalized, normalized.PeriodBStart, bEnd)
		respB, qerr := client.QuerySearchAnalytics(ctx, resolved, reqB)
		if qerr != nil {
			return struct{}{}, qerr
		}
		rowsB = respB.Rows
		return struct{}{}, nil
	})
	if err != nil {
		return toolError(gscclient.MapGoogleAPIError(err)), nil, nil
	}
	if clampErr.Code != "" {
		return toolError(clampErr), nil, nil
	}

	// The delta arithmetic is compare_periods'; only the filtering, attribution
	// and ranking are new here.
	compared := compareMetricRows(toAnalyticsRows(rowsA), toAnalyticsRows(rowsB))
	matched := make([]findDecliningPagesRow, 0, 64)
	for _, c := range compared {
		if !isReportableDecline(c.ClicksDelta, normalized.MinClickDrop) {
			continue
		}
		page := ""
		if len(c.Keys) > 0 {
			page = c.Keys[0]
		}
		matched = append(matched, findDecliningPagesRow{
			Page:                 page,
			ClicksA:              c.ClicksA,
			ClicksB:              c.ClicksB,
			ClicksDelta:          c.ClicksDelta,
			ClicksChangePct:      c.ClicksChange,
			ImpressionsA:         c.ImpressionsA,
			ImpressionsB:         c.ImpressionsB,
			ImpressionsDelta:     c.ImpressionsDelta,
			ImpressionsChangePct: c.ImpressionsChange,
			CtrA:                 c.CtrA,
			CtrB:                 c.CtrB,
			CtrDeltaPP:           c.CtrDeltaPP,
			PositionA:            c.PositionA,
			PositionB:            c.PositionB,
			PositionChange:       c.PositionChange,
			Attribution: attributeDecline(c.PositionA, c.PositionB, c.ImpressionsChange,
				c.CtrA, c.CtrB, normalized.PositionDropThreshold, normalized.ImpressionsDropPctThreshold),
			OnlyIn: c.OnlyIn,
		})
	}

	// Biggest absolute loss first. Every clicks_delta here is negative, so
	// ascending clicks_delta is descending |clicks_delta|.
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].ClicksDelta < matched[j].ClicksDelta
	})
	rowsMatched := len(matched)
	truncated := rowsMatched > normalized.RowLimit
	if truncated {
		matched = matched[:normalized.RowLimit]
	}

	periodDays, _ := inclusiveDays(normalized.PeriodBStart, bEnd)
	assumptions := decliningAssumptions{
		PeriodAStart:                normalized.PeriodAStart,
		PeriodAEnd:                  aEnd,
		PeriodBStart:                normalized.PeriodBStart,
		PeriodBEnd:                  bEnd,
		PeriodDays:                  periodDays,
		FirstIncompleteDate:         firstIncompleteDate,
		ExcludedIncompleteDays:      excludedDays,
		MinClickDrop:                normalized.MinClickDrop,
		RowLimit:                    normalized.RowLimit,
		PositionDropThreshold:       normalized.PositionDropThreshold,
		ImpressionsDropPctThreshold: normalized.ImpressionsDropPctThreshold,
		DataState:                   normalized.DataState,
		Dimensions:                  "page (one row per URL)",
		Attribution: "position_drop when position_b − position_a > position_drop_threshold; " +
			"impressions_drop when rank held (|position change| <= threshold) and impressions_change_pct < impressions_drop_pct_threshold; " +
			"ctr_drop when rank held, impressions held, and ctr fell; " +
			"unclear otherwise, including any page missing a position on either side (appeared or vanished) — that is 'we do not know why', not 'no cause'",
	}
	if excludedDays > 0 {
		assumptions.RequestedPeriodAEnd = normalized.PeriodAEnd
		assumptions.RequestedPeriodBEnd = normalized.PeriodBEnd
	}

	out := findDecliningPagesOutput{
		QueriedAt: nowRFC3339(),
		SiteURL:   resolvedSiteURL,
		Note: joinNotes(
			gscclient.DroppedPathNote(input.SiteURL),
			decliningClampNote(excludedDays, firstIncompleteDate, aEnd, bEnd),
		),
		Ordering:     "clicks_delta asc (biggest loss first)",
		RowsExamined: len(compared),
		ScanCapped:   len(rowsA) >= sortScanRowLimit || len(rowsB) >= sortScanRowLimit,
		RowsMatched:  rowsMatched,
		Truncated:    truncated,
		RowCount:     len(matched),
		Assumptions:  assumptions,
		Rows:         matched,
	}
	return toolResult(out), nil, nil
}

// decliningClampNote explains a trimmed range in words, because a caller
// comparing the output against the GSC console needs to know why the dates moved.
func decliningClampNote(excludedDays int, firstIncompleteDate, aEnd, bEnd string) string {
	if excludedDays == 0 {
		return ""
	}
	return fmt.Sprintf("Google reports data from %s onward as still being collected, so both periods were shortened by %d day(s) to keep them equal length and complete: period A now ends %s, period B now ends %s",
		firstIncompleteDate, excludedDays, aEnd, bEnd)
}

func buildDecliningQueryRequest(input findDecliningPagesInput, start, end string) *searchconsole.SearchAnalyticsQueryRequest {
	return &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:             start,
		EndDate:               end,
		Dimensions:            []string{"PAGE"},
		Type:                  input.SearchType,
		RowLimit:              sortScanRowLimit,
		DataState:             input.DataState,
		DimensionFilterGroups: toAPIFilterGroups(input.DimensionFilterGroups),
	}
}

func validateFindDecliningPages(input findDecliningPagesInput) (findDecliningPagesInput, gscclient.Error) {
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
	if norm == "HOURLY_ALL" {
		return out, gscclient.NewError(
			gscclient.ErrInvalidInput,
			"data_state 'hourly_all' is not supported by find_declining_pages",
			"use all (default) or final; hourly data only exists for the hour dimension",
		)
	}
	out.DataState = norm

	groups, err := normalizeFilterGroups(input.DimensionFilterGroups)
	if err.Code != "" {
		return out, err
	}
	out.DimensionFilterGroups = groups
	if err := validateFilterDimensions(out.DimensionFilterGroups); err.Code != "" {
		return out, err
	}

	if err := validateDates(input.PeriodAStart, input.PeriodAEnd); err.Code != "" {
		return out, err
	}
	if err := validateDates(input.PeriodBStart, input.PeriodBEnd); err.Code != "" {
		return out, err
	}
	daysA, errA := inclusiveDays(input.PeriodAStart, input.PeriodAEnd)
	if errA != nil {
		return out, gscclient.NewError(gscclient.ErrInvalidInput, errA.Error(), "use YYYY-MM-DD dates")
	}
	daysB, errB := inclusiveDays(input.PeriodBStart, input.PeriodBEnd)
	if errB != nil {
		return out, gscclient.NewError(gscclient.ErrInvalidInput, errB.Error(), "use YYYY-MM-DD dates")
	}
	// Period A is the baseline and B the current window: the freshness probe spans
	// period_a_start..period_b_end and the incomplete-day trim keys off B's end,
	// so a reversed or overlapping pair would silently compare the wrong things.
	aEndTime, _ := time.Parse("2006-01-02", input.PeriodAEnd)
	bStartTime, _ := time.Parse("2006-01-02", input.PeriodBStart)
	if !aEndTime.Before(bStartTime) {
		return out, gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("period A must end before period B starts: period A ends %s, period B starts %s", input.PeriodAEnd, input.PeriodBStart),
			"period A is the earlier baseline and period B the later current window; swap them if reversed, and do not let the ranges overlap",
		)
	}
	if daysA != daysB {
		return out, gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("period lengths must match: period A is %d days, period B is %d days", daysA, daysB),
			"use equal-length date ranges (e.g. both 28 days); a shorter current period always looks like a decline",
		)
	}

	if out.MinClickDrop == 0 {
		out.MinClickDrop = decliningDefaultMinClickDrop
	}
	if out.MinClickDrop < 0 {
		return out, gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("min_click_drop must be a positive click count, got %d", out.MinClickDrop),
			"pass the size of the drop to report, e.g. 10, not -10",
		)
	}

	if out.PositionDropThreshold == 0 {
		out.PositionDropThreshold = decliningDefaultPositionDrop
	}
	if out.PositionDropThreshold < 0 {
		return out, gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("position_drop_threshold must be non-negative, got %v", out.PositionDropThreshold),
			"a worsening rank is a positive number of places, e.g. 1",
		)
	}

	if out.ImpressionsDropPctThreshold == 0 {
		out.ImpressionsDropPctThreshold = decliningDefaultImpressionsDropPct
	}
	if out.ImpressionsDropPctThreshold > 0 {
		return out, gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("impressions_drop_pct_threshold must be negative, got %v", out.ImpressionsDropPctThreshold),
			"a drop is a negative percent change, e.g. -10",
		)
	}

	rowLimit, err := validateComputedRowLimit(input.RowLimit)
	if err.Code != "" {
		return out, err
	}
	out.RowLimit = rowLimit

	return out, gscclient.Error{}
}
