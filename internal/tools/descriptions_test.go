package tools

import (
	"strings"
	"testing"
)

func TestDescriptions_ContainRequiredGuidance(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		desc string
		want []string
	}{
		{"list_sites", descListSites, []string{"Call this first", "PT", "2–4", "16"}},
		{"get_site", descGetSite, []string{"list_sites", "PT"}},
		{"query", descQuerySearchAnalytics, []string{"25,000", "150", "export", "weighted average", "data_state", "PT", "2–4", "RE2", "10 days"}},
		{"compare", descComparePeriods, []string{"B minus A", "ctr_delta_pp", "percentage points", "position_change", "only_in", "PT", "period_a_days"}},
		{"inspect", descInspectURL, []string{"does NOT", "request indexing", "webmaster-tools/limits"}},
		{"sitemaps", descManageSitemaps, []string{"GSC_ENABLE_WRITE", "GSC_ALLOW_DESTRUCTIVE", "write_disabled"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, w := range tc.want {
				if !strings.Contains(tc.desc, w) {
					t.Errorf("description missing %q", w)
				}
			}
		})
	}
}
