package tools

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"google.golang.org/api/searchconsole/v1"
)

// Shared machinery for the computed tools (find_ctr_gaps,
// find_page_two_queries, find_declining_pages). Those three differ from the
// six pass-through tools: they scan up to 25,000 rows and return a ranked,
// reasoned conclusion instead of raw rows. The scan is why they are tools and
// not skills — 25,000 rows do not fit in an LLM context.
//
// Every threshold is a parameter with a default, and every default actually
// used is echoed back in the tool's assumptions block. A computed answer whose
// arithmetic cannot be audited is the failure mode these tools have to avoid.

// Defaults for the computed tools. Exposed as parameters so a caller can
// disagree, echoed in assumptions so a caller can see what was applied.
const (
	// ctrGapsDefaultPositionMin/Max bound the ranks worth a title rewrite:
	// below 1 is impossible, past 20 the CTR signal is dominated by noise.
	ctrGapsDefaultPositionMin = 1.0
	ctrGapsDefaultPositionMax = 20.0
	// ctrGapsDefaultMinImpressions keeps single-visit rows out: a row with 3
	// impressions and 0 clicks has a CTR of 0 that means nothing.
	ctrGapsDefaultMinImpressions = 100
	// ctrGapsDefaultMinSamples is how many OTHER rows the median baseline needs
	// before the position window stops widening.
	ctrGapsDefaultMinSamples = 10

	// Page two is literally ranks 11–20 on a ten-result SERP.
	pageTwoDefaultPositionMin    = 11.0
	pageTwoDefaultPositionMax    = 20.0
	pageTwoDefaultMinImpressions = 50

	// decliningDefaultMinClickDrop filters noise: a 2-click move is weather.
	decliningDefaultMinClickDrop = 10
	// decliningDefaultPositionDrop is the rank movement that counts as a real
	// ranking loss rather than average-position jitter.
	decliningDefaultPositionDrop = 1.0
	// decliningDefaultImpressionsDropPct is the impressions change (percent,
	// negative) below which a decline is attributed to lost impressions.
	decliningDefaultImpressionsDropPct = -10.0

	// computedDefaultRowLimit / maxComputedRowLimit bound the ranked output.
	// These tools exist to hand back a short list; the ceiling is far below the
	// 25,000-row scan because 25,000 computed rows is not an answer.
	computedDefaultRowLimit = 50
	maxComputedRowLimit     = 1000
)

// Baseline scope labels. site_wide means the neighbour window had to widen
// until it covered every observed position, so B(p) is not position-matched
// any more — it is the whole property's median. Saying so is the difference
// between a weak comparison and a dishonest one.
const (
	baselineScopeNeighborhood = "position_neighborhood"
	baselineScopeSiteWide     = "site_wide"
)

// Decline attribution labels. The three causes need opposite responses: a rank
// loss means looking at competitors, an impressions loss is usually search
// demand or a SERP layout change (not your fault), and only a CTR loss points
// at the title.
const (
	attributionPositionDrop    = "position_drop"
	attributionImpressionsDrop = "impressions_drop"
	attributionCtrDrop         = "ctr_drop"
	attributionUnclear         = "unclear"
)

// firstIncomplete returns the data-freshness markers Google ships with a
// Search Analytics response. Google names the first date (or hour) whose
// numbers are still being collected, so nothing here has to guess at a "2–4
// day delay". Both fields are empty unless the request grouped by DATE with
// data_state=all (or by HOUR with hourly_all).
func firstIncomplete(resp *searchconsole.SearchAnalyticsQueryResponse) (date, hour string) {
	if resp == nil || resp.Metadata == nil {
		return "", ""
	}
	return resp.Metadata.FirstIncompleteDate, resp.Metadata.FirstIncompleteHour
}

// probeFirstIncompleteDate asks for the same range grouped by date purely to
// read back metadata.firstIncompleteDate. Metadata is only populated on a
// DATE-grouped request, so a query+page or page-only scan can never see it.
//
// Dimension filters are deliberately not passed: data freshness is a property
// of the property, not of the filtered slice, and a filter that matches
// nothing would return no metadata at all.
//
// Google fills firstIncompleteDate only when dataState is ALL (or HOURLY_ALL
// with an HOUR grouping), so under FINAL the probe would be a guaranteed-empty
// extra API call; skip it.
func probeFirstIncompleteDate(ctx context.Context, client *gscclient.Client, resolvedSiteURL, start, end, searchType, dataState string) (string, error) {
	if dataState == "FINAL" {
		return "", nil
	}
	resp, err := client.QuerySearchAnalytics(ctx, resolvedSiteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:  start,
		EndDate:    end,
		Dimensions: []string{"DATE"},
		Type:       searchType,
		DataState:  dataState,
	})
	if err != nil {
		return "", err
	}
	date, _ := firstIncomplete(resp)
	return date, nil
}

// queryPageRow is one query+page pair from a Search Analytics scan. Position is
// a pointer because a row with no clicks and no impressions carries no rank.
type queryPageRow struct {
	Query       string
	Page        string
	Clicks      float64
	Impressions float64
	Ctr         float64
	Position    *float64
}

// toQueryPageRows converts API rows keyed by [query, page]. Rows with fewer
// keys are dropped rather than guessed at.
func toQueryPageRows(rows []*searchconsole.ApiDataRow) []queryPageRow {
	out := make([]queryPageRow, 0, len(rows))
	for _, r := range rows {
		if r == nil || len(r.Keys) < 2 {
			continue
		}
		row := queryPageRow{
			Query:       r.Keys[0],
			Page:        r.Keys[1],
			Clicks:      r.Clicks,
			Impressions: r.Impressions,
			Ctr:         r.Ctr,
		}
		if r.Clicks != 0 || r.Impressions != 0 {
			pos := r.Position
			row.Position = &pos
		}
		out = append(out, row)
	}
	return out
}

// pagesPerQuery counts how many distinct pages each query drew impressions
// for, across the whole scanned set. More than one page for a query is
// cannibalisation: two of your own URLs competing for the same search.
func pagesPerQuery(rows []queryPageRow) map[string]int {
	seen := make(map[string]map[string]struct{}, len(rows))
	for _, r := range rows {
		if seen[r.Query] == nil {
			seen[r.Query] = make(map[string]struct{}, 2)
		}
		seen[r.Query][r.Page] = struct{}{}
	}
	out := make(map[string]int, len(seen))
	for q, pages := range seen {
		out[q] = len(pages)
	}
	return out
}

// ctrBaseline is B(p) for one row, plus everything needed to audit it: how many
// OTHER rows went into the median, how far the window had to widen, and whether
// it stayed position-matched at all.
type ctrBaseline struct {
	Value   float64
	Samples int
	Window  int
	Scope   string
}

// ctrBaselineIndex answers B(p) = median{ r_j : |round(p_j) − round(p)| ≤ w },
// with w growing from 0 until minSamples peers are in the window.
//
// The row being scored is never its own peer. Including it would make the
// comparison circular: at min_samples=1 a lone row at some position would get
// baseline_ctr == ctr and could never be reported, and in general a row deep
// enough below its neighbours drags down the very median it is measured
// against, shrinking its own opportunity score.
//
// Median, not mean: CTR is right-skewed, and a handful of branded queries at
// 40% CTR would drag a mean up until normal pages look broken.
//
// Nearest-neighbour window, not fixed buckets: CTR falls steeply across ranks
// 1–5 and flatly across 11–20, so one bucket width cannot fit both, and any
// chosen boundary is unjustifiable. A per-row window also explains itself in
// plain language — "the other 23 queries ranked 11–13 get 2.1%, this one gets
// 0.4%".
//
// The pool is restricted to rows meeting min_impressions: a median over rows
// with three impressions each measures rounding, not behaviour. That
// restriction is reported in assumptions.
type ctrBaselineIndex struct {
	byPosition map[int][]float64
	minPos     int
	maxPos     int
	pool       int
	minSamples int
	windows    map[int]baselineWindow
}

// baselineWindow is the widened neighbour set for one rounded position, sorted
// once so every row at that position can take a median without re-sorting.
type baselineWindow struct {
	sorted  []float64
	width   int
	covered bool
}

func newCTRBaselineIndex(rows []queryPageRow, minImpressions, minSamples int) *ctrBaselineIndex {
	idx := &ctrBaselineIndex{
		byPosition: make(map[int][]float64),
		minSamples: minSamples,
		windows:    make(map[int]baselineWindow),
	}
	first := true
	for _, r := range rows {
		if r.Position == nil || r.Impressions < float64(minImpressions) {
			continue
		}
		rp := int(math.Round(*r.Position))
		idx.byPosition[rp] = append(idx.byPosition[rp], r.Ctr)
		idx.pool++
		if first || rp < idx.minPos {
			idx.minPos = rp
		}
		if first || rp > idx.maxPos {
			idx.maxPos = rp
		}
		first = false
	}
	return idx
}

// window widens symmetrically around a rounded position until there are enough
// peers, or until it has swallowed every observed position. The second exit is
// the graceful degradation path for tiny properties: B(p) becomes the site-wide
// median and is labelled as such.
func (idx *ctrBaselineIndex) window(rp int) baselineWindow {
	if cached, ok := idx.windows[rp]; ok {
		return cached
	}
	for w := 0; ; w++ {
		lo, hi := rp-w, rp+w
		var vals []float64
		for p := lo; p <= hi; p++ {
			vals = append(vals, idx.byPosition[p]...)
		}
		covered := lo <= idx.minPos && hi >= idx.maxPos
		// len(vals)-1 discounts the row that will be scored against this window.
		if len(vals)-1 >= idx.minSamples || covered {
			sort.Float64s(vals)
			out := baselineWindow{sorted: vals, width: w, covered: covered}
			idx.windows[rp] = out
			return out
		}
	}
}

// baseline returns B(p) for a row at the given position with the given CTR,
// excluding that row itself. ok is false when the row has no peers at all —
// either the pool is empty (no row met min_impressions) or this is the only
// row in the property. Both cases must be reported as "no baseline" rather
// than compared against itself.
func (idx *ctrBaselineIndex) baseline(position, ownCtr float64) (ctrBaseline, bool) {
	if idx.pool == 0 {
		return ctrBaseline{}, false
	}
	win := idx.window(int(math.Round(position)))
	value, peers, ok := medianExcludingOne(win.sorted, ownCtr)
	if !ok {
		return ctrBaseline{}, false
	}
	scope := baselineScopeNeighborhood
	if win.covered {
		scope = baselineScopeSiteWide
	}
	return ctrBaseline{Value: value, Samples: peers, Window: win.width, Scope: scope}, true
}

// medianExcludingOne returns the median of sorted with one occurrence of
// exclude removed, and how many values remained. It indexes around the removed
// element instead of copying, so a wide window stays cheap per row. When
// exclude is absent (the caller was not part of the pool) nothing is removed.
func medianExcludingOne(sorted []float64, exclude float64) (value float64, n int, ok bool) {
	skip := -1
	if i := sort.SearchFloat64s(sorted, exclude); i < len(sorted) && sorted[i] == exclude {
		skip = i
	}
	n = len(sorted)
	if skip >= 0 {
		n--
	}
	if n == 0 {
		return 0, 0, false
	}
	at := func(i int) float64 {
		if skip >= 0 && i >= skip {
			return sorted[i+1]
		}
		return sorted[i]
	}
	mid := n / 2
	if n%2 == 1 {
		return at(mid), n, true
	}
	return (at(mid-1) + at(mid)) / 2, n, true
}

// median returns the middle value, averaging the two middles for an even count.
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

// attributeDecline classifies why clicks fell. Order matters: a rank loss
// explains everything downstream of it, so it is tested first, and the other
// two branches only apply when rank held steady.
//
// A row missing a position on either side (a page that appeared or vanished
// entirely) cannot satisfy the |Δposition| tests and comes back unclear. That
// is the honest answer: we know it stopped getting clicks, not why.
func attributeDecline(posA, posB *float64, impressionsChangePct *float64, ctrA, ctrB, positionDropThreshold, impressionsDropPct float64) string {
	if posA == nil || posB == nil {
		return attributionUnclear
	}
	posDelta := *posB - *posA
	if posDelta > positionDropThreshold {
		return attributionPositionDrop
	}
	rankHeld := math.Abs(posDelta) <= positionDropThreshold
	if !rankHeld {
		// Rank improved by more than the threshold yet clicks fell — none of the
		// three causes fits.
		return attributionUnclear
	}
	if impressionsChangePct != nil && *impressionsChangePct < impressionsDropPct {
		return attributionImpressionsDrop
	}
	if ctrB < ctrA && (impressionsChangePct == nil || *impressionsChangePct >= impressionsDropPct) {
		return attributionCtrDrop
	}
	return attributionUnclear
}

// isReportableDecline is true when a page lost at least minClickDrop clicks.
// "At least" includes the boundary: a page down exactly min_click_drop clicks
// is a page down min_click_drop clicks.
func isReportableDecline(clicksDelta float64, minClickDrop int) bool {
	return clicksDelta <= -float64(minClickDrop)
}

// lastCompleteDate returns the day before firstIncompleteDate, i.e. the last
// day whose numbers Google has finished collecting.
func lastCompleteDate(firstIncomplete string) (string, error) {
	const layout = "2006-01-02"
	t, err := time.Parse(layout, firstIncomplete)
	if err != nil {
		return "", err
	}
	return t.AddDate(0, 0, -1).Format(layout), nil
}

// clampPeriodsToComplete trims the end of both periods so no incomplete day is
// counted. Counting the last few days of a range produces a fake decline, and
// the answer is in the response: Google's firstIncompleteDate.
//
// Both ends move by the same number of days so the periods stay equal length —
// a shorter current period compared against a longer baseline would invent the
// very drop this is meant to prevent. Trimming the end (not the start) leaves
// both start dates, and therefore the weekday alignment between the windows,
// untouched.
func clampPeriodsToComplete(aStart, aEnd, bStart, bEnd, firstIncompleteDate string) (newAEnd, newBEnd string, removed int, err gscclient.Error) {
	if firstIncompleteDate == "" {
		return aEnd, bEnd, 0, gscclient.Error{}
	}
	lastComplete, perr := lastCompleteDate(firstIncompleteDate)
	if perr != nil {
		// Unparseable metadata is not worth failing the call over; report the
		// range untouched and let the note carry the raw value.
		return aEnd, bEnd, 0, gscclient.Error{}
	}
	const layout = "2006-01-02"
	bEndTime, perr := time.Parse(layout, bEnd)
	if perr != nil {
		return aEnd, bEnd, 0, gscclient.Error{}
	}
	lastCompleteTime, _ := time.Parse(layout, lastComplete)
	if !bEndTime.After(lastCompleteTime) {
		return aEnd, bEnd, 0, gscclient.Error{}
	}

	removed = int(bEndTime.Sub(lastCompleteTime).Hours() / 24)
	aEndTime, perr := time.Parse(layout, aEnd)
	if perr != nil {
		return aEnd, bEnd, 0, gscclient.Error{}
	}
	newBEndTime := lastCompleteTime
	newAEndTime := aEndTime.AddDate(0, 0, -removed)

	bStartTime, _ := time.Parse(layout, bStart)
	aStartTime, _ := time.Parse(layout, aStart)
	if newBEndTime.Before(bStartTime) || newAEndTime.Before(aStartTime) {
		return aEnd, bEnd, 0, gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("period B (%s..%s) lies entirely inside Google's incomplete window: data from %s onward is still being collected",
				bStart, bEnd, firstIncompleteDate),
			fmt.Sprintf("move both periods earlier so period B ends on or before %s (PT)", lastComplete),
		)
	}
	return newAEndTime.Format(layout), newBEndTime.Format(layout), removed, gscclient.Error{}
}

// validatePositionRange rejects impossible or inverted rank windows.
func validatePositionRange(posMin, posMax float64) gscclient.Error {
	if posMin < 1 {
		return gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("position_min must be at least 1, got %v", posMin),
			"search result positions start at 1",
		)
	}
	if posMax < posMin {
		return gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("position_max (%v) must be greater than or equal to position_min (%v)", posMax, posMin),
			"",
		)
	}
	return gscclient.Error{}
}

// validateComputedRowLimit applies the default and caps the ranked output.
func validateComputedRowLimit(rowLimit int) (int, gscclient.Error) {
	if rowLimit == 0 {
		return computedDefaultRowLimit, gscclient.Error{}
	}
	if rowLimit < 1 || rowLimit > maxComputedRowLimit {
		return 0, gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("row_limit must be between 1 and %d, got %d", maxComputedRowLimit, rowLimit),
			fmt.Sprintf("omit row_limit to use the default of %d; this tool returns a ranked shortlist, not an export", computedDefaultRowLimit),
		)
	}
	return rowLimit, gscclient.Error{}
}

// validateMinImpressions rejects a negative floor.
func validateMinImpressions(minImpressions int) gscclient.Error {
	if minImpressions < 0 {
		return gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("min_impressions must be non-negative, got %d", minImpressions),
			"",
		)
	}
	return gscclient.Error{}
}
