package tools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"google.golang.org/api/searchconsole/v1"
)

// querySearchAnalyticsInput is the input schema for the query_search_analytics
// tool. Field names are lower-case for the LLM; validate.go maps them to the
// upper-case enums required by the Google API.
type querySearchAnalyticsInput struct {
	SiteURL               string                 `json:"site_url" jsonschema:"The GSC property to query. Supports bare domain (example.com), full URL (https://www.example.com/), or canonical GSC format (sc-domain:example.com)."`
	StartDate             string                 `json:"start_date" jsonschema:"Start date of the query range in YYYY-MM-DD format. Dates are in PT (Pacific Time) timezone."`
	EndDate               string                 `json:"end_date" jsonschema:"End date of the query range in YYYY-MM-DD format. Dates are in PT (Pacific Time) timezone."`
	Dimensions            []string               `json:"dimensions,omitempty" jsonschema:"Optional dimensions to group by: query, page, country, device, date, hour, or searchAppearance. Use hour only with data_state=hourly_all."`
	SearchType            string                 `json:"search_type,omitempty" jsonschema:"Optional search type filter: web, image, video, news, discover, or googleNews. Omit to let Google apply its own default (web)."`
	DimensionFilterGroups []DimensionFilterGroup `json:"dimension_filter_groups,omitempty" jsonschema:"Optional dimension filters. Each group contains groupType='and' and an array of filters with dimension, operator, and expression. Use excludingRegex to exclude branded terms."`
	AggregationType       string                 `json:"aggregation_type,omitempty" jsonschema:"Optional aggregation type: auto (default), byProperty, byPage, or byNewsShowcasePanel. Cannot be byProperty when grouping or filtering by page."`
	RowLimit              int                    `json:"row_limit,omitempty" jsonschema:"Optional maximum number of rows to return. Default 150, maximum 25000. For export set explicitly."`
	StartRow              int                    `json:"start_row,omitempty" jsonschema:"Optional zero-based offset for pagination."`
	DataState             string                 `json:"data_state,omitempty" jsonschema:"Optional data freshness: all (default, matches the GSC console), final (only finalized data), or hourly_all (required for hour dimension)."`
	SortBy                string                 `json:"sort_by,omitempty" jsonschema:"Optional sort key: clicks (default), impressions, ctr, or position. Anything other than clicks desc makes the server scan up to 25,000 rows and sort locally, because the GSC API can only order by clicks."`
	SortOrder             string                 `json:"sort_order,omitempty" jsonschema:"Optional sort direction: asc or desc. Defaults to desc for clicks/impressions/ctr and asc for position (lower position is a better rank)."`
}

// querySearchAnalyticsOutput is the successful response shape for
// query_search_analytics.
type querySearchAnalyticsOutput struct {
	QueriedAt       string                    `json:"queried_at"`
	SiteURL         string                    `json:"site_url"`
	StartDate       string                    `json:"start_date"`
	EndDate         string                    `json:"end_date"`
	Dimensions      []string                  `json:"dimensions,omitempty"`
	SearchType      string                    `json:"search_type,omitempty"`
	DataState       string                    `json:"data_state,omitempty"`
	AggregationType string                    `json:"aggregation_type,omitempty"`
	// Note is a soft advisory (future dates, possible retention, dropped path),
	// not an error.
	Note string `json:"note,omitempty"`
	// Ordering names the key and direction the rows are actually sorted by, so a
	// caller can never mistake GSC's native clicks-desc order for what it asked.
	Ordering string `json:"ordering"`
	// RowsExamined is how many rows were considered before truncation to
	// row_limit. Equal to row_count when nothing was dropped.
	RowsExamined int `json:"rows_examined"`
	// Truncated is true when rows_examined exceeded row_limit, i.e. the returned
	// rows are the top row_limit of a larger set under the stated ordering.
	Truncated bool `json:"truncated"`
	// ScanCapped is true when the scan itself hit the API's 25,000-row ceiling,
	// so rows_examined is incomplete and the top-N may be missing entries.
	ScanCapped bool                      `json:"scan_capped,omitempty"`
	RowCount   int                       `json:"row_count"`
	Rows       []querySearchAnalyticsRow `json:"rows"`
}

type querySearchAnalyticsRow struct {
	Keys        []string `json:"keys"`
	Clicks      float64  `json:"clicks"`
	Impressions float64  `json:"impressions"`
	Ctr         float64  `json:"ctr"`
	// Position is omitted when both clicks and impressions are 0 (no rank signal).
	Position *float64 `json:"position,omitempty"`
}

// querySearchAnalyticsInputSchema is written by hand so array fields have
// "type":"array" instead of the SDK-inferred ["null","array"] union. Some
// MCP clients reject union types and would drop the entire tool list.
var querySearchAnalyticsInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "site_url": {
      "type": "string",
      "description": "The GSC property to query. Supports bare domain (example.com), full URL (https://www.example.com/), or canonical GSC format (sc-domain:example.com)."
    },
    "start_date": {
      "type": "string",
      "description": "Start date of the query range in YYYY-MM-DD format. Dates are in PT (Pacific Time) timezone."
    },
    "end_date": {
      "type": "string",
      "description": "End date of the query range in YYYY-MM-DD format. Dates are in PT (Pacific Time) timezone."
    },
    "dimensions": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional dimensions to group by: query, page, country, device, date, hour, or searchAppearance. Use hour only with data_state=hourly_all."
    },
    "search_type": {
      "type": "string",
      "description": "Optional search type filter: web, image, video, news, discover, or googleNews. Omit to let Google apply its own default (web)."
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
      "description": "Optional dimension filters. Each group contains groupType='and' and an array of filters with dimension, operator, and expression. Use excludingRegex to exclude branded terms."
    },
    "aggregation_type": {
      "type": "string",
      "description": "Optional aggregation type: auto (default), byProperty, byPage, or byNewsShowcasePanel. Cannot be byProperty when grouping or filtering by page."
    },
    "row_limit": {
      "type": "integer",
      "description": "Optional maximum number of rows to return. Default 150, maximum 25000. For export set row_limit explicitly (up to 25000)."
    },
    "start_row": {
      "type": "integer",
      "description": "Optional zero-based offset for pagination."
    },
    "data_state": {
      "type": "string",
      "description": "Optional data freshness: all (default, matches the GSC console), final (only finalized data), or hourly_all (required for hour dimension)."
    },
    "sort_by": {
      "type": "string",
      "description": "Optional sort key: clicks (default), impressions, ctr, or position. Anything other than clicks desc makes the server scan up to 25,000 rows and sort locally, because the GSC API can only order by clicks."
    },
    "sort_order": {
      "type": "string",
      "description": "Optional sort direction: asc or desc. Defaults to desc for clicks/impressions/ctr and asc for position (lower position is a better rank)."
    }
  },
  "required": ["site_url", "start_date", "end_date"]
}`)

func registerQuerySearchAnalytics(srv *mcp.Server, client *gscclient.Client) {
	mcp.AddTool(srv,
		&mcp.Tool{
			Name:        "query_search_analytics",
			Description: descQuerySearchAnalytics,
			InputSchema: querySearchAnalyticsInputSchema,
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, input querySearchAnalyticsInput) (*mcp.CallToolResult, any, error) {
			return querySearchAnalytics(ctx, client, input)
		},
	)
}

func querySearchAnalytics(ctx context.Context, client *gscclient.Client, input querySearchAnalyticsInput) (*mcp.CallToolResult, any, error) {
	normalized, vErr := validateSearchAnalytics(input)
	if vErr.Code != "" {
		return toolError(vErr), nil, nil
	}

	// GSC always returns rows ordered by clicks descending. Any other ordering
	// has to be produced here, which means fetching more rows than we return.
	nativeOrder := normalized.SortBy == sortClicks && normalized.SortOrder == sortDesc
	fetchLimit := normalized.RowLimit
	if !nativeOrder {
		fetchLimit = sortScanRowLimit
	}
	req := buildSearchAnalyticsRequest(normalized, fetchLimit)

	var resolvedSiteURL string
	resp, err := gscclient.WithResolvedSiteURL(ctx, client, normalized.SiteURL, func(ctx context.Context, resolved string) (*searchconsole.SearchAnalyticsQueryResponse, error) {
		resolvedSiteURL = resolved
		return client.QuerySearchAnalytics(ctx, resolved, req)
	})
	if err != nil {
		return toolError(gscclient.MapGoogleAPIError(err)), nil, nil
	}

	rows := toAnalyticsRows(resp.Rows)
	scanCapped := !nativeOrder && len(rows) >= sortScanRowLimit
	if !nativeOrder {
		sortAnalyticsRows(rows, normalized.SortBy, normalized.SortOrder)
	}
	examined := len(rows)
	truncated := examined > normalized.RowLimit
	if truncated {
		rows = rows[:normalized.RowLimit]
	}

	out := querySearchAnalyticsOutput{
		QueriedAt:       nowRFC3339(),
		SiteURL:         resolvedSiteURL,
		StartDate:       normalized.StartDate,
		EndDate:         normalized.EndDate,
		Dimensions:      normalized.Dimensions,
		SearchType:      normalized.SearchType,
		DataState:       normalized.DataState,
		AggregationType: normalized.AggregationType,
		Note: joinNotes(
			gscclient.DroppedPathNote(input.SiteURL),
			dateRangeNotes(normalized.StartDate, normalized.EndDate),
		),
		Ordering:     orderingLabel(normalized.SortBy, normalized.SortOrder),
		RowsExamined: examined,
		Truncated:    truncated,
		ScanCapped:   scanCapped,
		RowCount:     len(rows),
		Rows:         rows,
	}
	return toolResult(out), nil, nil
}

func buildSearchAnalyticsRequest(input querySearchAnalyticsInput, fetchLimit int) *searchconsole.SearchAnalyticsQueryRequest {
	// Use Type (json:"type"), not SearchType (json:"searchType"). Official REST
	// docs and SPEC.md §3.1 name the field "type"; the generated client exposes
	// both and picking the wrong one silently fails to filter.
	req := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:       input.StartDate,
		EndDate:         input.EndDate,
		Dimensions:      input.Dimensions,
		Type:            input.SearchType,
		AggregationType: input.AggregationType,
		RowLimit:        int64(fetchLimit),
		StartRow:        int64(input.StartRow),
		DataState:       input.DataState,
	}

	if len(input.DimensionFilterGroups) > 0 {
		groups := make([]*searchconsole.ApiDimensionFilterGroup, len(input.DimensionFilterGroups))
		for i, g := range input.DimensionFilterGroups {
			filters := make([]*searchconsole.ApiDimensionFilter, len(g.Filters))
			for j, f := range g.Filters {
				filters[j] = &searchconsole.ApiDimensionFilter{
					Dimension:  f.Dimension,
					Operator:   f.Operator,
					Expression: f.Expression,
				}
			}
			groups[i] = &searchconsole.ApiDimensionFilterGroup{
				GroupType: g.GroupType,
				Filters:   filters,
			}
		}
		req.DimensionFilterGroups = groups
	}

	return req
}
