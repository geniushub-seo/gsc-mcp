package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/api/searchconsole/v1"
)

// comparePeriodsInput is the input schema for compare_periods.
// Period A is the baseline (earlier); period B is the current (later).
// All deltas and *_change_pct are B minus A.
type comparePeriodsInput struct {
	SiteURL               string                 `json:"site_url" jsonschema:"The GSC property to query."`
	PeriodAStart          string                 `json:"period_a_start" jsonschema:"Start date of period A — baseline/earlier period (YYYY-MM-DD, PT)."`
	PeriodAEnd            string                 `json:"period_a_end" jsonschema:"End date of period A — baseline/earlier period (YYYY-MM-DD, PT)."`
	PeriodBStart          string                 `json:"period_b_start" jsonschema:"Start date of period B — current/later period (YYYY-MM-DD, PT)."`
	PeriodBEnd            string                 `json:"period_b_end" jsonschema:"End date of period B — current/later period (YYYY-MM-DD, PT)."`
	Dimensions            []string               `json:"dimensions,omitempty" jsonschema:"Optional dimensions to group by: query, page, country, device, date, hour, or searchAppearance."`
	SearchType            string                 `json:"search_type,omitempty" jsonschema:"Optional search type: web, image, video, news, discover, or googleNews. Omit to let Google default to web."`
	DimensionFilterGroups []DimensionFilterGroup `json:"dimension_filter_groups,omitempty" jsonschema:"Optional dimension filters applied to both periods. Each group contains groupType='and' and an array of filters with dimension, operator, and expression. Use excludingRegex to exclude branded terms."`
	RowLimit              int                    `json:"row_limit,omitempty" jsonschema:"Optional max rows in the response. Default 100, maximum 25000. This caps the joined output, not just each period."`
	DataState             string                 `json:"data_state,omitempty" jsonschema:"Optional data freshness: all (default), final, or hourly_all."`
	SortBy                string                 `json:"sort_by,omitempty" jsonschema:"Optional sort key: clicks_delta (default), impressions_delta, ctr_delta_pp, or position_change. Every delta ordering requires scanning up to 25,000 rows per period because the GSC API can only order by clicks."`
	SortOrder             string                 `json:"sort_order,omitempty" jsonschema:"Optional sort direction: asc or desc. Defaults to desc for the delta keys (biggest gains first; use asc for biggest drops) and asc for position_change (biggest rank improvement first)."`
}

type comparePeriodsOutput struct {
	QueriedAt    string   `json:"queried_at"`
	SiteURL      string   `json:"site_url"`
	PeriodAStart string   `json:"period_a_start"`
	PeriodAEnd   string   `json:"period_a_end"`
	PeriodBStart string   `json:"period_b_start"`
	PeriodBEnd   string   `json:"period_b_end"`
	PeriodADays  int      `json:"period_a_days"`
	PeriodBDays  int      `json:"period_b_days"`
	Dimensions   []string `json:"dimensions,omitempty"`
	SearchType   string   `json:"search_type,omitempty"`
	DataState    string   `json:"data_state,omitempty"`
	// Note is a soft advisory (dropped path), not an error.
	Note string `json:"note,omitempty"`
	// Ordering names the key and direction the joined rows are sorted by.
	Ordering string `json:"ordering"`
	// RowsExamined is the size of the joined set before truncation to row_limit.
	RowsExamined int `json:"rows_examined"`
	// Truncated is true when the join produced more rows than row_limit.
	Truncated bool `json:"truncated"`
	// ScanCapped is true when either period hit the API's 25,000-row ceiling, so
	// the joined set is incomplete and the top-N may be missing entries.
	ScanCapped bool               `json:"scan_capped,omitempty"`
	RowCount   int                `json:"row_count"`
	Rows       []comparePeriodRow `json:"rows"`
}

type comparePeriodRow struct {
	Keys []string `json:"keys"`

	ClicksA      float64  `json:"clicks_a"`
	ClicksB      float64  `json:"clicks_b"`
	ClicksDelta  float64  `json:"clicks_delta"`
	ClicksChange *float64 `json:"clicks_change_pct,omitempty"`

	ImpressionsA      float64  `json:"impressions_a"`
	ImpressionsB      float64  `json:"impressions_b"`
	ImpressionsDelta  float64  `json:"impressions_delta"`
	ImpressionsChange *float64 `json:"impressions_change_pct,omitempty"`

	CtrA         float64  `json:"ctr_a"`
	CtrB         float64  `json:"ctr_b"`
	CtrDeltaPP   float64  `json:"ctr_delta_pp"`
	CtrChangePct *float64 `json:"ctr_change_pct,omitempty"`

	// Position fields use pointers so only_in rows can omit meaningless zeros
	// and so position_change / position_improved are absent when only one period exists.
	PositionA        *float64 `json:"position_a,omitempty"`
	PositionB        *float64 `json:"position_b,omitempty"`
	PositionChange   *float64 `json:"position_change,omitempty"`
	PositionImproved *bool    `json:"position_improved,omitempty"`

	OnlyIn string `json:"only_in,omitempty"`
}

// comparePeriodsInputSchema is written by hand so the dimensions array has
// "type":"array" instead of the SDK-inferred ["null","array"] union.
var comparePeriodsInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "site_url": {
      "type": "string",
      "description": "The GSC property to query."
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
      "description": "End date of period B — current/later period (YYYY-MM-DD, PT)."
    },
    "dimensions": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional dimensions to group by: query, page, country, device, date, hour, or searchAppearance."
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
      "description": "Optional dimension filters applied to both periods. Each group contains groupType='and' and an array of filters with dimension, operator, and expression. Use excludingRegex to exclude branded terms."
    },
    "row_limit": {
      "type": "integer",
      "description": "Optional max rows in the response. Default 100, maximum 25000. This caps the joined output, not just each period."
    },
    "data_state": {
      "type": "string",
      "description": "Optional data freshness: all (default), final, or hourly_all."
    },
    "sort_by": {
      "type": "string",
      "description": "Optional sort key: clicks_delta (default), impressions_delta, ctr_delta_pp, or position_change. Every delta ordering requires scanning up to 25,000 rows per period because the GSC API can only order by clicks."
    },
    "sort_order": {
      "type": "string",
      "description": "Optional sort direction: asc or desc. Defaults to desc for the delta keys (biggest gains first; use asc for biggest drops) and asc for position_change (biggest rank improvement first)."
    }
  },
  "required": ["site_url", "period_a_start", "period_a_end", "period_b_start", "period_b_end"]
}`)

func registerComparePeriods(srv *mcp.Server, client *gscclient.Client) {
	mcp.AddTool(srv,
		&mcp.Tool{
			Name:        "compare_periods",
			Description: descComparePeriods,
			InputSchema: comparePeriodsInputSchema,
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, input comparePeriodsInput) (*mcp.CallToolResult, any, error) {
			return comparePeriods(ctx, client, input)
		},
	)
}

func comparePeriods(ctx context.Context, client *gscclient.Client, input comparePeriodsInput) (*mcp.CallToolResult, any, error) {
	normalized, vErr := validateComparePeriods(input)
	if vErr.Code != "" {
		return toolError(vErr), nil, nil
	}

	var resolvedSiteURL string
	var rowsA, rowsB []*searchconsole.ApiDataRow

	_, err := gscclient.WithResolvedSiteURL(ctx, client, normalized.SiteURL, func(ctx context.Context, resolved string) (struct{}, error) {
		resolvedSiteURL = resolved

		// Both periods are scanned to the API maximum, not to row_limit: every
		// delta ordering depends on rows Google would not have returned in its
		// own clicks-desc top-N, so a row_limit-sized fetch cannot produce a
		// correct top-N by any delta key.
		reqA := buildCompareQueryRequest(normalized, normalized.PeriodAStart, normalized.PeriodAEnd)
		respA, err := client.QuerySearchAnalytics(ctx, resolved, reqA)
		if err != nil {
			return struct{}{}, err
		}
		rowsA = respA.Rows

		reqB := buildCompareQueryRequest(normalized, normalized.PeriodBStart, normalized.PeriodBEnd)
		respB, err := client.QuerySearchAnalytics(ctx, resolved, reqB)
		if err != nil {
			return struct{}{}, err
		}
		rowsB = respB.Rows
		return struct{}{}, nil
	})
	if err != nil {
		return toolError(gscclient.MapGoogleAPIError(err)), nil, nil
	}

	scanCapped := len(rowsA) >= sortScanRowLimit || len(rowsB) >= sortScanRowLimit
	compared := compareMetricRows(toAnalyticsRows(rowsA), toAnalyticsRows(rowsB))
	sortCompareRows(compared, normalized.SortBy, normalized.SortOrder)
	examined := len(compared)
	truncated := examined > normalized.RowLimit
	if truncated {
		compared = compared[:normalized.RowLimit]
	}

	// Days were validated equal; recompute for output (inclusive).
	periodADays, _ := inclusiveDays(normalized.PeriodAStart, normalized.PeriodAEnd)
	periodBDays, _ := inclusiveDays(normalized.PeriodBStart, normalized.PeriodBEnd)
	out := comparePeriodsOutput{
		QueriedAt:    nowRFC3339(),
		SiteURL:      resolvedSiteURL,
		PeriodAStart: normalized.PeriodAStart,
		PeriodAEnd:   normalized.PeriodAEnd,
		PeriodBStart: normalized.PeriodBStart,
		PeriodBEnd:   normalized.PeriodBEnd,
		PeriodADays:  periodADays,
		PeriodBDays:  periodBDays,
		Dimensions:   normalized.Dimensions,
		SearchType:   normalized.SearchType,
		DataState:    normalized.DataState,
		Note:         gscclient.DroppedPathNote(input.SiteURL),
		Ordering:     orderingLabel(normalized.SortBy, normalized.SortOrder),
		RowsExamined: examined,
		Truncated:    truncated,
		ScanCapped:   scanCapped,
		RowCount:     len(compared),
		Rows:         compared,
	}
	return toolResult(out), nil, nil
}

func validateComparePeriods(input comparePeriodsInput) (comparePeriodsInput, gscclient.Error) {
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

	if err := validateFilterDimensions(out.DimensionFilterGroups); err.Code != "" {
		return out, err
	}

	if out.RowLimit == 0 {
		out.RowLimit = 100
	} else if out.RowLimit < 1 || out.RowLimit > 25000 {
		return out, gscclient.NewError(
			gscclient.ErrInvalidInput,
			"row_limit must be between 1 and 25,000",
			"omit row_limit to use the default of 100",
		)
	}

	sortBy, err := normalizeSortBy(input.SortBy, sortClicksDelta, compareSortKeys)
	if err.Code != "" {
		return out, err
	}
	out.SortBy = sortBy
	sortOrder, err := normalizeSortOrder(input.SortOrder, sortBy)
	if err.Code != "" {
		return out, err
	}
	out.SortOrder = sortOrder

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
	if daysA != daysB {
		return out, gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("period lengths must match: period A is %d days, period B is %d days", daysA, daysB),
			"use equal-length date ranges (e.g. both 28 days); unequal percentages are not meaningful",
		)
	}
	if err := validateHourDataState(out.Dimensions, out.DataState); err.Code != "" {
		return out, err
	}
	// Hour window applies to each period independently.
	if err := validateHourDateWindow(out.Dimensions, input.PeriodAStart, input.PeriodAEnd); err.Code != "" {
		return out, err
	}
	if err := validateHourDateWindow(out.Dimensions, input.PeriodBStart, input.PeriodBEnd); err.Code != "" {
		return out, err
	}

	return out, gscclient.Error{}
}

// inclusiveDays returns the number of calendar days from start to end inclusive
// (YYYY-MM-DD). A single-day range returns 1.
func inclusiveDays(start, end string) (int, error) {
	const layout = "2006-01-02"
	s, err := time.Parse(layout, start)
	if err != nil {
		return 0, err
	}
	e, err := time.Parse(layout, end)
	if err != nil {
		return 0, err
	}
	return int(e.Sub(s).Hours()/24) + 1, nil
}

func buildCompareQueryRequest(input comparePeriodsInput, start, end string) *searchconsole.SearchAnalyticsQueryRequest {
	return &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:             start,
		EndDate:               end,
		Dimensions:            input.Dimensions,
		Type:                  input.SearchType,
		RowLimit:              sortScanRowLimit,
		DataState:             input.DataState,
		DimensionFilterGroups: toAPIFilterGroups(input.DimensionFilterGroups),
	}
}

func toAnalyticsRows(rows []*searchconsole.ApiDataRow) []querySearchAnalyticsRow {
	out := make([]querySearchAnalyticsRow, 0, len(rows))
	for _, r := range rows {
		if r == nil {
			continue
		}
		row := querySearchAnalyticsRow{
			Keys:        r.Keys,
			Clicks:      r.Clicks,
			Impressions: r.Impressions,
			Ctr:         roundN(r.Ctr, 4),
		}
		// No impressions and no clicks → position is not meaningful; omit it.
		if r.Clicks != 0 || r.Impressions != 0 {
			pos := roundN(r.Position, 2)
			row.Position = &pos
		}
		out = append(out, row)
	}
	return out
}

// compareMetricRows merges period A and B rows by dimension keys and computes
// deltas. Keys are joined with a NUL separator to avoid collisions.
func compareMetricRows(aRows, bRows []querySearchAnalyticsRow) []comparePeriodRow {
	type both struct {
		keys []string
		a, b querySearchAnalyticsRow
		inA  bool
		inB  bool
	}
	merged := make(map[string]*both, len(aRows)+len(bRows))
	keysOrder := make([]string, 0, len(aRows)+len(bRows))

	for _, row := range aRows {
		key := strings.Join(row.Keys, "\x00")
		if _, ok := merged[key]; !ok {
			merged[key] = &both{keys: append([]string(nil), row.Keys...)}
			keysOrder = append(keysOrder, key)
		}
		m := merged[key]
		m.a = row
		m.inA = true
	}
	for _, row := range bRows {
		key := strings.Join(row.Keys, "\x00")
		if _, ok := merged[key]; !ok {
			merged[key] = &both{keys: append([]string(nil), row.Keys...)}
			keysOrder = append(keysOrder, key)
		}
		m := merged[key]
		m.b = row
		m.inB = true
	}

	out := make([]comparePeriodRow, 0, len(keysOrder))
	for _, key := range keysOrder {
		m := merged[key]
		row := comparePeriodRow{
			Keys:             m.keys,
			ClicksA:          roundN(m.a.Clicks, 4),
			ClicksB:          roundN(m.b.Clicks, 4),
			ImpressionsA:     roundN(m.a.Impressions, 4),
			ImpressionsB:     roundN(m.b.Impressions, 4),
			CtrA:             roundN(m.a.Ctr, 4),
			CtrB:             roundN(m.b.Ctr, 4),
			ClicksDelta:      roundN(m.b.Clicks-m.a.Clicks, 4),
			ImpressionsDelta: roundN(m.b.Impressions-m.a.Impressions, 4),
			// ctr_delta_pp is percentage points: (0.125-0.10)*100 = 2.5
			CtrDeltaPP: roundN((m.b.Ctr-m.a.Ctr)*100, 4),
		}
		row.ClicksChange = pctChange(m.a.Clicks, m.b.Clicks)
		row.ImpressionsChange = pctChange(m.a.Impressions, m.b.Impressions)
		row.CtrChangePct = pctChange(m.a.Ctr, m.b.Ctr)

		switch {
		case m.inA && m.inB:
			if m.a.Position != nil && m.b.Position != nil {
				posA := *m.a.Position
				posB := *m.b.Position
				change := roundN(posB-posA, 2)
				improved := posB < posA
				row.PositionA = &posA
				row.PositionB = &posB
				row.PositionChange = &change
				row.PositionImproved = &improved
			} else {
				if m.a.Position != nil {
					posA := *m.a.Position
					row.PositionA = &posA
				}
				if m.b.Position != nil {
					posB := *m.b.Position
					row.PositionB = &posB
				}
			}
		case m.inA:
			row.OnlyIn = "a"
			if m.a.Position != nil {
				posA := *m.a.Position
				row.PositionA = &posA
			}
			// omit position_b / position_change / position_improved
		default:
			row.OnlyIn = "b"
			if m.b.Position != nil {
				posB := *m.b.Position
				row.PositionB = &posB
			}
		}
		out = append(out, row)
	}
	return out
}

func pctChange(from, to float64) *float64 {
	if from == 0 {
		if to == 0 {
			z := 0.0
			return &z
		}
		// Undefined relative change from a zero baseline — omit rather than +Inf.
		return nil
	}
	v := roundN((to-from)/math.Abs(from)*100, 2)
	return &v
}

func roundN(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}
