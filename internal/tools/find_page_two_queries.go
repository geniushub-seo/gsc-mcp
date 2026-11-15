package tools

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/api/searchconsole/v1"
)

// findPageTwoQueriesInput is the input schema for find_page_two_queries.
type findPageTwoQueriesInput struct {
	SiteURL               string                 `json:"site_url" jsonschema:"The GSC property to query."`
	StartDate             string                 `json:"start_date" jsonschema:"Start date (YYYY-MM-DD, PT)."`
	EndDate               string                 `json:"end_date" jsonschema:"End date (YYYY-MM-DD, PT)."`
	SearchType            string                 `json:"search_type,omitempty" jsonschema:"Optional search type: web, image, video, news, discover, or googleNews."`
	DimensionFilterGroups []DimensionFilterGroup `json:"dimension_filter_groups,omitempty" jsonschema:"Optional dimension filters. Use operator excludingRegex on dimension query to exclude brand terms."`
	PositionMin           float64                `json:"position_min,omitempty" jsonschema:"Optional lowest average position to consider. Default 11."`
	PositionMax           float64                `json:"position_max,omitempty" jsonschema:"Optional highest average position to consider. Default 20."`
	MinImpressions        int                    `json:"min_impressions,omitempty" jsonschema:"Optional minimum impressions a query needs to be reported. Default 50."`
	RowLimit              int                    `json:"row_limit,omitempty" jsonschema:"Optional maximum ranked rows to return. Default 50, maximum 1000."`
	DataState             string                 `json:"data_state,omitempty" jsonschema:"Optional data freshness: all (default) or final."`
}

type findPageTwoQueriesOutput struct {
	QueriedAt string `json:"queried_at"`
	SiteURL   string `json:"site_url"`
	Note      string `json:"note,omitempty"`
	Ordering  string `json:"ordering"`
	// RowsExamined is how many query+page rows were scanned before aggregation.
	RowsExamined int `json:"rows_examined"`
	// QueriesExamined is how many distinct queries those rows aggregated into.
	QueriesExamined int  `json:"queries_examined"`
	ScanCapped      bool `json:"scan_capped,omitempty"`
	// RowsMatched is how many queries met every filter before truncation.
	RowsMatched int                     `json:"rows_matched"`
	Truncated   bool                    `json:"truncated"`
	RowCount    int                     `json:"row_count"`
	Assumptions pageTwoAssumptions      `json:"assumptions"`
	Rows        []findPageTwoQueriesRow `json:"rows"`
}

type pageTwoAssumptions struct {
	StartDate           string  `json:"start_date"`
	EndDate             string  `json:"end_date"`
	PositionMin         float64 `json:"position_min"`
	PositionMax         float64 `json:"position_max"`
	MinImpressions      int     `json:"min_impressions"`
	RowLimit            int     `json:"row_limit"`
	DataState           string  `json:"data_state"`
	Dimensions          string  `json:"dimensions"`
	FirstIncompleteDate string  `json:"first_incomplete_date,omitempty"`
	Aggregation         string  `json:"aggregation"`
}

type findPageTwoQueriesRow struct {
	Query       string  `json:"query"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	Ctr         float64 `json:"ctr"`
	Position    float64 `json:"position"`
	// TopPage is the page with the most impressions for this query — the one
	// that would be improved to move the query onto page one.
	TopPage            string  `json:"top_page"`
	TopPageImpressions float64 `json:"top_page_impressions"`
	// PagesForQuery is how many of your pages drew impressions for this query.
	// More than one means your own URLs are competing for it.
	PagesForQuery int `json:"pages_for_query"`
}

// findPageTwoQueriesInputSchema is written by hand so array fields have
// "type":"array" instead of the SDK-inferred ["null","array"] union.
var findPageTwoQueriesInputSchema = json.RawMessage(`{
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
      "description": "Optional dimension filters. Use operator excludingRegex on dimension query to exclude brand terms."
    },
    "position_min": {
      "type": "number",
      "description": "Optional lowest average position to consider. Default 11."
    },
    "position_max": {
      "type": "number",
      "description": "Optional highest average position to consider. Default 20."
    },
    "min_impressions": {
      "type": "integer",
      "description": "Optional minimum impressions a query needs to be reported. Default 50."
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

func registerFindPageTwoQueries(srv *mcp.Server, client *gscclient.Client) {
	mcp.AddTool(srv,
		&mcp.Tool{
			Name:        "find_page_two_queries",
			Description: descFindPageTwoQueries,
			InputSchema: findPageTwoQueriesInputSchema,
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, input findPageTwoQueriesInput) (*mcp.CallToolResult, any, error) {
			return findPageTwoQueries(ctx, client, input)
		},
	)
}

func findPageTwoQueries(ctx context.Context, client *gscclient.Client, input findPageTwoQueriesInput) (*mcp.CallToolResult, any, error) {
	normalized, vErr := validateFindPageTwoQueries(input)
	if vErr.Code != "" {
		return toolError(vErr), nil, nil
	}

	var resolvedSiteURL string
	var scanned []*searchconsole.ApiDataRow
	var firstIncompleteDate, probeNote string

	_, err := gscclient.WithSiteURL(ctx, client, normalized.SiteURL, func(ctx context.Context, resolved string) (struct{}, error) {
		resolvedSiteURL = resolved

		date, perr := probeFirstIncompleteDate(ctx, client, resolved, normalized.StartDate, normalized.EndDate, normalized.SearchType, normalized.DataState)
		if perr != nil {
			probeNote = "could not read first_incomplete_date from Google (freshness probe failed); recent days in this range may still be incomplete"
		} else {
			firstIncompleteDate = date
		}

		// query+page rather than query alone: the unit of the answer is the query,
		// but the actionable part is which page carries it.
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
	aggregated := aggregateByQuery(rows)

	matched := make([]findPageTwoQueriesRow, 0, 64)
	for _, agg := range aggregated {
		// Position 0 means no row for this query carried a rank signal; real
		// positions start at 1, so such a query cannot be a page-two candidate.
		if agg.Position <= 0 {
			continue
		}
		if agg.Position < normalized.PositionMin || agg.Position > normalized.PositionMax {
			continue
		}
		if agg.Impressions < float64(normalized.MinImpressions) {
			continue
		}
		matched = append(matched, agg)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].Impressions > matched[j].Impressions
	})
	rowsMatched := len(matched)
	truncated := rowsMatched > normalized.RowLimit
	if truncated {
		matched = matched[:normalized.RowLimit]
	}

	out := findPageTwoQueriesOutput{
		QueriedAt: nowRFC3339(),
		SiteURL:   resolvedSiteURL,
		Note: joinNotes(
			gscclient.DroppedPathNote(input.SiteURL),
			dateRangeNotes(normalized.StartDate, normalized.EndDate),
			probeNote,
		),
		Ordering:        "impressions desc",
		RowsExamined:    len(rows),
		QueriesExamined: len(aggregated),
		ScanCapped:      len(scanned) >= sortScanRowLimit,
		RowsMatched:     rowsMatched,
		Truncated:       truncated,
		RowCount:        len(matched),
		Assumptions: pageTwoAssumptions{
			StartDate:           normalized.StartDate,
			EndDate:             normalized.EndDate,
			PositionMin:         normalized.PositionMin,
			PositionMax:         normalized.PositionMax,
			MinImpressions:      normalized.MinImpressions,
			RowLimit:            normalized.RowLimit,
			DataState:           normalized.DataState,
			Dimensions:          "query+page, then aggregated to one row per query",
			FirstIncompleteDate: firstIncompleteDate,
			Aggregation: "clicks and impressions are summed across the query's pages; ctr is clicks/impressions; position is the impression-weighted mean of the per-page positions, " +
				"so it can differ slightly from a query-only query_search_analytics call",
		},
		Rows: matched,
	}
	return toolResult(out), nil, nil
}

// aggregateByQuery rolls query+page rows up to one row per query, keeping the
// highest-impression page as the one to act on. Returned in first-seen order so
// the result is deterministic before sorting.
func aggregateByQuery(rows []queryPageRow) []findPageTwoQueriesRow {
	type acc struct {
		clicks         float64
		impressions    float64
		weightedPos    float64
		posImpressions float64
		topPage        string
		topPageImps    float64
		pages          map[string]struct{}
	}
	byQuery := make(map[string]*acc, len(rows))
	order := make([]string, 0, len(rows))

	for _, r := range rows {
		a, ok := byQuery[r.Query]
		if !ok {
			a = &acc{pages: make(map[string]struct{}, 2)}
			byQuery[r.Query] = a
			order = append(order, r.Query)
		}
		a.clicks += r.Clicks
		a.impressions += r.Impressions
		a.pages[r.Page] = struct{}{}
		if r.Position != nil && r.Impressions > 0 {
			a.weightedPos += *r.Position * r.Impressions
			a.posImpressions += r.Impressions
		}
		if r.Impressions > a.topPageImps || a.topPage == "" {
			a.topPage = r.Page
			a.topPageImps = r.Impressions
		}
	}

	out := make([]findPageTwoQueriesRow, 0, len(order))
	for _, q := range order {
		a := byQuery[q]
		row := findPageTwoQueriesRow{
			Query:              q,
			Clicks:             a.clicks,
			Impressions:        a.impressions,
			TopPage:            a.topPage,
			TopPageImpressions: a.topPageImps,
			PagesForQuery:      len(a.pages),
		}
		if a.impressions > 0 {
			row.Ctr = roundN(a.clicks/a.impressions, 4)
		}
		if a.posImpressions > 0 {
			row.Position = roundN(a.weightedPos/a.posImpressions, 2)
		}
		out = append(out, row)
	}
	return out
}

func validateFindPageTwoQueries(input findPageTwoQueriesInput) (findPageTwoQueriesInput, gscclient.Error) {
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
			"data_state 'hourly_all' is not supported by find_page_two_queries",
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
		out.PositionMin = pageTwoDefaultPositionMin
	}
	if out.PositionMax == 0 {
		out.PositionMax = pageTwoDefaultPositionMax
	}
	if err := validatePositionRange(out.PositionMin, out.PositionMax); err.Code != "" {
		return out, err
	}

	if out.MinImpressions == 0 {
		out.MinImpressions = pageTwoDefaultMinImpressions
	}
	if err := validateMinImpressions(out.MinImpressions); err.Code != "" {
		return out, err
	}

	rowLimit, err := validateComputedRowLimit(input.RowLimit)
	if err.Code != "" {
		return out, err
	}
	out.RowLimit = rowLimit

	return out, gscclient.Error{}
}
