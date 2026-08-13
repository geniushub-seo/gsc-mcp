package tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
)

// sortScanRowLimit is how many rows we pull from Google before sorting locally.
// The Search Console API has no orderBy: it always returns rows ordered by
// clicks descending and silently ignores unknown request fields. A correct
// top-N by any other key therefore requires over-fetching and sorting here.
// 25,000 is the API's own per-request maximum (SPEC.md §3.1).
const sortScanRowLimit = 25000

// Sort keys for query_search_analytics.
const (
	sortClicks      = "clicks"
	sortImpressions = "impressions"
	sortCtr         = "ctr"
	sortPosition    = "position"
)

// Sort keys for compare_periods.
const (
	sortClicksDelta      = "clicks_delta"
	sortImpressionsDelta = "impressions_delta"
	sortCtrDeltaPP       = "ctr_delta_pp"
	sortPositionChange   = "position_change"
)

const (
	sortAsc  = "asc"
	sortDesc = "desc"
)

// defaultSortOrder is the direction applied when sort_order is omitted.
// Position keys default to ascending because a lower position number is a
// better rank; every other key defaults to descending (largest first).
var defaultSortOrder = map[string]string{
	sortClicks:           sortDesc,
	sortImpressions:      sortDesc,
	sortCtr:              sortDesc,
	sortPosition:         sortAsc,
	sortClicksDelta:      sortDesc,
	sortImpressionsDelta: sortDesc,
	sortCtrDeltaPP:       sortDesc,
	sortPositionChange:   sortAsc,
}

var analyticsSortKeys = []string{sortClicks, sortImpressions, sortCtr, sortPosition}

var compareSortKeys = []string{sortClicksDelta, sortImpressionsDelta, sortCtrDeltaPP, sortPositionChange}

// normalizeSortBy validates sort_by against the allowed keys for a tool and
// applies the default when omitted. Keys are lower-case in both the schema and
// the output; unlike the Google enums there is no upper-case form to map to.
func normalizeSortBy(value, defaultKey string, allowed []string) (string, gscclient.Error) {
	if strings.TrimSpace(value) == "" {
		return defaultKey, gscclient.Error{}
	}
	key := strings.ToLower(strings.TrimSpace(value))
	for _, a := range allowed {
		if key == a {
			return key, gscclient.Error{}
		}
	}
	return "", gscclient.NewError(
		gscclient.ErrInvalidInput,
		fmt.Sprintf("sort_by must be one of: %s (got %q)", strings.Join(allowed, ", "), value),
		"omit sort_by to use the default of "+defaultKey,
	)
}

// normalizeSortOrder validates sort_order and applies the per-key default.
func normalizeSortOrder(value, sortBy string) (string, gscclient.Error) {
	if strings.TrimSpace(value) == "" {
		return defaultSortOrder[sortBy], gscclient.Error{}
	}
	order := strings.ToLower(strings.TrimSpace(value))
	if order != sortAsc && order != sortDesc {
		return "", gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("sort_order must be asc or desc (got %q)", value),
			"omit sort_order to use the default for this sort_by",
		)
	}
	return order, gscclient.Error{}
}

// orderingLabel is the human- and LLM-readable description of the applied
// ordering, emitted so a caller can never mistake the row order for something
// it asked for but did not get.
func orderingLabel(sortBy, sortOrder string) string {
	return sortBy + " " + sortOrder
}

// sortAnalyticsRows orders rows in place. Rows with no position (no clicks and
// no impressions) sort last regardless of direction: they carry no rank signal
// and must not occupy the "best rank" slots.
func sortAnalyticsRows(rows []querySearchAnalyticsRow, sortBy, sortOrder string) {
	less := func(i, j int) bool {
		a, b := rows[i], rows[j]
		if sortBy == sortPosition {
			switch {
			case a.Position == nil && b.Position == nil:
				return false
			case a.Position == nil:
				return false
			case b.Position == nil:
				return true
			}
			return orderFloat(*a.Position, *b.Position, sortOrder)
		}
		var av, bv float64
		switch sortBy {
		case sortImpressions:
			av, bv = a.Impressions, b.Impressions
		case sortCtr:
			av, bv = a.Ctr, b.Ctr
		default:
			av, bv = a.Clicks, b.Clicks
		}
		return orderFloat(av, bv, sortOrder)
	}
	sort.SliceStable(rows, less)
}

// sortCompareRows orders joined comparison rows in place. Rows that exist in
// only one period have no position_change; on a position_change sort they go
// last because a missing side is not a rank movement of zero.
func sortCompareRows(rows []comparePeriodRow, sortBy, sortOrder string) {
	less := func(i, j int) bool {
		a, b := rows[i], rows[j]
		if sortBy == sortPositionChange {
			switch {
			case a.PositionChange == nil && b.PositionChange == nil:
				return false
			case a.PositionChange == nil:
				return false
			case b.PositionChange == nil:
				return true
			}
			return orderFloat(*a.PositionChange, *b.PositionChange, sortOrder)
		}
		var av, bv float64
		switch sortBy {
		case sortImpressionsDelta:
			av, bv = a.ImpressionsDelta, b.ImpressionsDelta
		case sortCtrDeltaPP:
			av, bv = a.CtrDeltaPP, b.CtrDeltaPP
		default:
			av, bv = a.ClicksDelta, b.ClicksDelta
		}
		return orderFloat(av, bv, sortOrder)
	}
	sort.SliceStable(rows, less)
}

func orderFloat(a, b float64, order string) bool {
	if order == sortAsc {
		return a < b
	}
	return a > b
}
