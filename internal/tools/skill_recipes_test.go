package tools

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var groupedQuery = regexp.MustCompile(`dimensions:\s*\[\s*"query"\s*\]`)

func TestSkillRecipes_KPITotalsOmitDimensions(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	cases := []struct {
		rel           string
		wantPeriodKPI bool
	}{
		{filepath.Join(".agents", "skills", "monthly-report", "SKILL.md"), true},
		{filepath.Join(".agents", "skills", "nonbrand-performance", "SKILL.md"), true},
	}
	for _, tc := range cases {
		path := filepath.Join(root, tc.rel)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(raw)
		if !strings.Contains(body, "omit dimensions") {
			t.Errorf("%s: KPI recipe must say to omit dimensions", path)
		}
		if !strings.Contains(body, "Never derive brand/non-brand") {
			t.Errorf("%s: must forbid deriving share/change from a capped grouped list", path)
		}

		fences := recipeFences(body)
		if !hasAggregateFilter(fences, "excludingRegex") {
			t.Errorf("%s: missing aggregate (no query dimension) excludingRegex call", path)
		}
		if !hasAggregateFilter(fences, "includingRegex") {
			t.Errorf("%s: missing aggregate (no query dimension) includingRegex call", path)
		}
		if tc.wantPeriodKPI {
			if !hasAggregateCompare(fences, "excludingRegex") || !hasAggregateCompare(fences, "includingRegex") {
				t.Errorf("%s: KPI period change needs compare_periods aggregates for both excludingRegex and includingRegex", path)
			}
		}

		for _, fence := range fences {
			if !groupedQuery.MatchString(fence) {
				continue
			}
			if !strings.Contains(fence, "excludingRegex") && !strings.Contains(fence, "includingRegex") {
				continue
			}
			// Grouped + filtered is allowed only as labelled detail/top-N.
			if !isDetailFence(fence) {
				t.Errorf("%s: grouped+filtered fence must be detail (row_limit or sort_by), not a KPI total:\n%s", path, fence)
			}
		}
	}
}

func recipeFences(body string) []string {
	var out []string
	parts := strings.Split(body, "```")
	for i := 1; i < len(parts); i += 2 {
		out = append(out, parts[i])
	}
	return out
}

func hasAggregateFilter(fences []string, op string) bool {
	for _, fence := range fences {
		if strings.Contains(fence, op) && !groupedQuery.MatchString(fence) {
			return true
		}
	}
	return false
}

func hasAggregateCompare(fences []string, op string) bool {
	for _, fence := range fences {
		if strings.Contains(fence, "compare_periods") && strings.Contains(fence, op) && !groupedQuery.MatchString(fence) {
			return true
		}
	}
	return false
}

func isDetailFence(fence string) bool {
	return strings.Contains(fence, "row_limit") || strings.Contains(fence, "sort_by")
}
