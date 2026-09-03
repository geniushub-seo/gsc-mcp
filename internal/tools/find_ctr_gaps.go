package tools

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"google.golang.org/api/searchconsole/v1"
)

// findCTRGapsInput is the input schema for find_ctr_gaps. Every threshold has a
// default and is echoed back in assumptions.
type findCTRGapsInput struct {
	SiteURL               string                 `json:"site_url" jsonschema:"The GSC property to query."`
	StartDate             string                 `json:"start_date" jsonschema:"Start date (YYYY-MM-DD, PT)."`
	EndDate               string                 `json:"end_date" jsonschema:"End date (YYYY-MM-DD, PT)."`
	SearchType            string                 `json:"search_type,omitempty" jsonschema:"Optional search type: web, image, video, news, discover, or googleNews."`
	DimensionFilterGroups []DimensionFilterGroup `json:"dimension_filter_groups,omitempty" jsonschema:"Optional dimension filters. Use operator excludingRegex on dimension query to get non-brand CTR gaps."`
	PositionMin           float64                `json:"position_min,omitempty" jsonschema:"Optional lowest average position to consider. Default 1."`
	PositionMax           float64                `json:"position_max,omitempty" jsonschema:"Optional highest average position to consider. Default 20."`
	MinImpressions        int                    `json:"min_impressions,omitempty" jsonschema:"Optional minimum impressions for a row to be reported and to enter the baseline pool. Default 100."`
	MinSamples            int                    `json:"min_samples,omitempty" jsonschema:"Optional minimum number of OTHER neighbouring rows the median CTR baseline needs before the position window stops widening. Default 10."`
	RowLimit              int                    `json:"row_limit,omitempty" jsonschema:"Optional maximum ranked rows to return. Default 50, maximum 1000."`
	DataState             string                 `json:"data_state,omitempty" jsonschema:"Optional data freshness: all (default) or final."`
}

type findCTRGapsOutput struct {
	QueriedAt string `json:"queried_at"`
	SiteURL   string `json:"site_url"`
	// Note is a soft advisory (dropped path, freshness probe failure), not an error.
	Note string `json:"note,omitempty"`
	// Ordering names the key the rows are ranked by.
	Ordering string `json:"ordering"`
	// RowsExamined is how many query+page rows were scanned before filtering.
	RowsExamined int `json:"rows_examined"`
	// ScanCapped is true when the scan hit the API's 25,000-row ceiling, so the
	// candidate set is incomplete and both the ranking and the baseline pool may
	// be missing rows.
	ScanCapped bool `json:"scan_capped,omitempty"`
	// RowsMatched is how many rows met every filter before truncation.
	RowsMatched int                `json:"rows_matched"`
	Truncated   bool               `json:"truncated"`
	RowCount    int                `json:"row_count"`
	Assumptions ctrGapsAssumptions `json:"assumptions"`
	Rows        []findCTRGapsRow   `json:"rows"`
}

// ctrGapsAssumptions records every number this tool chose on the caller's
// behalf. Without it the output is a black box: correct arithmetic nobody can
// check is the one failure mode a computed tool cannot recover from.
type ctrGapsAssumptions struct {
	StartDate      string  `json:"start_date"`
	EndDate        string  `json:"end_date"`
	PositionMin    float64 `json:"position_min"`
	PositionMax    float64 `json:"position_max"`
	MinImpressions int     `json:"min_impressions"`
	MinSamples     int     `json:"min_samples"`
	RowLimit       int     `json:"row_limit"`
	DataState      string  `json:"data_state"`
	Dimensions     string  `json:"dimensions"`
	// FirstIncompleteDate is Google's own marker for where the range stops being
	// final. Empty means the whole range is complete (or data_state=final).
	FirstIncompleteDate string `json:"first_incomplete_date,omitempty"`
	Baseline            string `json:"baseline"`
	BaselinePool        string `json:"baseline_pool"`
	BaselinePoolRows    int    `json:"baseline_pool_rows"`
	OpportunityFormula  string `json:"opportunity_formula"`
}

type findCTRGapsRow struct {
	Query       string  `json:"query"`
	Page        string  `json:"page"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	Ctr         float64 `json:"ctr"`
	Position    float64 `json:"position"`
	// BaselineCtr is B(p) for this row's rounded position, computed from the
	// OTHER rows in the window — a row is never its own peer. The remaining
	// baseline_* fields are what it was computed from.
	BaselineCtr             float64 `json:"baseline_ctr"`
	BaselineSamples         int     `json:"baseline_samples"`
	BaselineWindowPositions int     `json:"baseline_window_positions"`
	BaselineScope           string  `json:"baseline_scope"`
	// CtrGapPP is (baseline_ctr − ctr) in percentage points.
	CtrGapPP float64 `json:"ctr_gap_pp"`
	// OpportunityClicks is impressions × (baseline_ctr − ctr): the clicks this
	// row would have earned at its own position's median CTR.
	OpportunityClicks float64 `json:"opportunity_clicks"`
	// Cannibalization and PagesForQuery are omitted unless this query drew
	// impressions for more than one page in the scanned set.
	Cannibalization bool `json:"cannibalization,omitempty"`
	PagesForQuery   int  `json:"pages_for_query,omitempty"`
}

// findCTRGapsInputSchema is written by hand so array fields have "type":"array"
// instead of the SDK-inferred ["null","array"] union. Some MCP clients reject
// union types and would drop the entire tool list.
var findCTRGapsInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "site_url": {
      "type": "string",
      "description": "The GSC property to query. Accepts bare domain, full URL, or sc-domain: form."
    },
    "start_date": {
      "type": "string",
      "description": "Start date (YYYY-MM-DD, PT)."
    },
    "end_date": {
      "type": "string",
      "description": "End date (YYYY-MM-DD, PT)."
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
      "description": "Optional dimension filters. Use operator excludingRegex on dimension query to get non-brand CTR gaps."
    },
    "position_min": {
      "type": "number",
      "description": "Optional lowest average position to consider. Default 1."
    },
    "position_max": {
      "type": "number",
      "description": "Optional highest average position to consider. Default 20."
    },
    "min_impressions": {
      "type": "integer",
      "description": "Optional minimum impressions for a row to be reported and to enter the baseline pool. Default 100."
    },
    "min_samples": {
      "type": "integer",
      "description": "Optional minimum number of OTHER neighbouring rows the median CTR baseline needs before the position window stops widening. Default 10."
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
  "required": ["site_url", "start_date", "end_date"]
}`)

func registerFindCTRGaps(srv *mcp.Server, client *gscclient.Client) {
	mcp.AddTool(srv,
		&mcp.Tool{
			Name:        "find_ctr_gaps",
			Description: descFindCTRGaps,
			InputSchema: findCTRGapsInputSchema,
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, input findCTRGapsInput) (*mcp.CallToolResult, any, error) {
			return findCTRGaps(ctx, client, input)
		},
	)
}

func findCTRGaps(ctx context.Context, client *gscclient.Client, input findCTRGapsInput) (*mcp.CallToolResult, any, error) {
	normalized, vErr := validateFindCTRGaps(input)
	if vErr.Code != "" {
		return toolError(vErr), nil, nil
	}

	var resolvedSiteURL string
	var scanned []*searchconsole.ApiDataRow
	var firstIncompleteDate, probeNote string

	_, err := gscclient.WithResolvedSiteURL(ctx, client, normalized.SiteURL, func(ctx context.Context, resolved string) (struct{}, error) {
		resolvedSiteURL = resolved

		// The freshness probe is a separate DATE-grouped call because metadata is
		// only populated on that grouping. A probe failure is reported as a note,
		// not an error: it costs the caller the freshness marker, not the answer.
		date, perr := probeFirstIncompleteDate(ctx, client, resolved, normalized.StartDate, normalized.EndDate, normalized.SearchType, normalized.DataState)
		if perr != nil {
			probeNote = "could not read first_incomplete_date from Google (freshness probe failed); recent days in this range may still be incomplete"
		} else {
			firstIncompleteDate = date
		}

		resp, qerr := client.QuerySearchAnalytics(ctx, resolved, &searchconsole.SearchAnalyticsQueryRequest{
			StartDate:             normalized.StartDate,
			EndDate:               normalized.EndDate,
			Dimensions:            []string{"QUERY", "PAGE"},
			Type:                  normalized.SearchType,
			RowLimit:              sortScanRowLimit,
			DataState:             normalized.DataState,
			DimensionFilterGroups: toAPIFilterGroups(normalized.DimensionFilterGroups),
		})
		if qerr != nil {
			return struct{}{}, qerr
		}
		scanned = resp.Rows
		return struct{}{}, nil
	})
	if err != nil {
		return toolError(gscclient.MapGoogleAPIError(err)), nil, nil
	}

	rows := toQueryPageRows(scanned)
	index := newCTRBaselineIndex(rows, normalized.MinImpressions, normalized.MinSamples)
	pageCounts := pagesPerQuery(rows)

	matched := make([]findCTRGapsRow, 0, 64)
	for _, r := range rows {
		if r.Position == nil {
			continue
		}
		if *r.Position < normalized.PositionMin || *r.Position > normalized.PositionMax {
			continue
		}
		if r.Impressions < float64(normalized.MinImpressions) {
			continue
		}
		baseline, ok := index.baseline(*r.Position, r.Ctr)
		if !ok || r.Ctr >= baseline.Value {
			continue
		}
		out := findCTRGapsRow{
			Query:                   r.Query,
			Page:                    r.Page,
			Clicks:                  r.Clicks,
			Impressions:             r.Impressions,
			Ctr:                     roundN(r.Ctr, 4),
			Position:                roundN(*r.Position, 2),
			BaselineCtr:             roundN(baseline.Value, 4),
			BaselineSamples:         baseline.Samples,
			BaselineWindowPositions: baseline.Window,
			BaselineScope:           baseline.Scope,
			CtrGapPP:                roundN((baseline.Value-r.Ctr)*100, 2),
			OpportunityClicks:       roundN(r.Impressions*(baseline.Value-r.Ctr), 1),
		}
		if n := pageCounts[r.Query]; n > 1 {
			out.Cannibalization = true
			out.PagesForQuery = n
		}
		matched = append(matched, out)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].OpportunityClicks > matched[j].OpportunityClicks
	})
	rowsMatched := len(matched)
	truncated := rowsMatched > normalized.RowLimit
	if truncated {
		matched = matched[:normalized.RowLimit]
	}

	out := findCTRGapsOutput{
		QueriedAt: nowRFC3339(),
		SiteURL:   resolvedSiteURL,
		Note: joinNotes(
			gscclient.DroppedPathNote(input.SiteURL),
			dateRangeNotes(normalized.StartDate, normalized.EndDate),
			probeNote,
		),
		Ordering:     "opportunity_clicks desc",
		RowsExamined: len(rows),
		ScanCapped:   len(scanned) >= sortScanRowLimit,
		RowsMatched:  rowsMatched,
		Truncated:    truncated,
		RowCount:     len(matched),
		Assumptions: ctrGapsAssumptions{
			StartDate:           normalized.StartDate,
			EndDate:             normalized.EndDate,
			PositionMin:         normalized.PositionMin,
			PositionMax:         normalized.PositionMax,
			MinImpressions:      normalized.MinImpressions,
			MinSamples:          normalized.MinSamples,
			RowLimit:            normalized.RowLimit,
			DataState:           normalized.DataState,
			Dimensions:          "query+page (one row per query and landing page pair)",
			FirstIncompleteDate: firstIncompleteDate,
			Baseline: "baseline_ctr is the median CTR of the OTHER scanned rows whose rounded average position is within baseline_window_positions of this row's rounded position (the row itself is excluded, so the comparison is never circular); " +
				"the window widens from 0 until baseline_samples reaches min_samples. baseline_scope=site_wide means it had to widen across every observed position, so the comparison is property-wide, not position-matched. A row with no peers at all is not reported rather than compared against itself",
			BaselinePool:       "scanned rows with a position and impressions >= min_impressions (a median over near-zero-impression rows measures rounding, not behaviour)",
			BaselinePoolRows:   index.pool,
			OpportunityFormula: "opportunity_clicks = impressions * (baseline_ctr - ctr)",
		},
		Rows: matched,
	}
	return toolResult(out), nil, nil
}

func validateFindCTRGaps(input findCTRGapsInput) (findCTRGapsInput, gscclient.Error) {
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
			"data_state 'hourly_all' is not supported by find_ctr_gaps",
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

	if err := validateDates(input.StartDate, input.EndDate); err.Code != "" {
		return out, err
	}

	if out.PositionMin == 0 {
		out.PositionMin = ctrGapsDefaultPositionMin
	}
	if out.PositionMax == 0 {
		out.PositionMax = ctrGapsDefaultPositionMax
	}
	if err := validatePositionRange(out.PositionMin, out.PositionMax); err.Code != "" {
		return out, err
	}

	if out.MinImpressions == 0 {
		out.MinImpressions = ctrGapsDefaultMinImpressions
	}
	if err := validateMinImpressions(out.MinImpressions); err.Code != "" {
		return out, err
	}

	if out.MinSamples == 0 {
		out.MinSamples = ctrGapsDefaultMinSamples
	}
	if out.MinSamples < 1 {
		return out, gscclient.NewError(
			gscclient.ErrInvalidInput,
			"min_samples must be at least 1",
			"omit min_samples to use the default of 10",
		)
	}

	rowLimit, err := validateComputedRowLimit(input.RowLimit)
	if err.Code != "" {
		return out, err
	}
	out.RowLimit = rowLimit

	return out, gscclient.Error{}
}
