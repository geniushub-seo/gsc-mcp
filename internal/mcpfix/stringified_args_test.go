package mcpfix

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestToolArrayFields_ComparePeriodsIncludesFilterGroups(t *testing.T) {
	t.Parallel()
	fields := toolArrayFields["compare_periods"]
	for _, want := range []string{"dimensions", "dimension_filter_groups"} {
		if !slices.Contains(fields, want) {
			t.Errorf("compare_periods middleware field map missing %q; got %v", want, fields)
		}
	}
	if !slices.Contains(toolArrayFields["query_search_analytics"], "dimension_filter_groups") {
		t.Error("query_search_analytics middleware field map missing dimension_filter_groups")
	}
}

func TestToolArrayFields_CoversEveryToolTakingFilterGroups(t *testing.T) {
	t.Parallel()
	// A tool that accepts dimension_filter_groups but is absent from this map
	// rejects any client that stringifies the array, and the failure surfaces as
	// a schema error the handler never sees.
	for _, tool := range []string{
		"query_search_analytics", "compare_periods",
		"find_ctr_gaps", "find_page_two_queries", "find_declining_pages",
	} {
		if !slices.Contains(toolArrayFields[tool], "dimension_filter_groups") {
			t.Errorf("%s middleware field map missing dimension_filter_groups; got %v", tool, toolArrayFields[tool])
		}
	}
}

func TestCoerceStringifiedArray(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{
			name: "already array",
			raw:  `["query","page"]`,
			want: "",
			ok:   false,
		},
		{
			name: "string containing array",
			raw:  `"[\"query\",\"page\"]"`,
			want: `["query","page"]`,
			ok:   true,
		},
		{
			name: "string not containing array",
			raw:  `"not an array"`,
			want: "",
			ok:   false,
		},
		{
			name: "missing field",
			raw:  "",
			want: "",
			ok:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := coerceStringifiedArray(json.RawMessage(tc.raw))
			if ok != tc.ok {
				t.Fatalf("coerceStringifiedArray(%q) ok = %v, want %v", tc.raw, ok, tc.ok)
			}
			if string(got) != tc.want {
				t.Fatalf("coerceStringifiedArray(%q) = %q, want %q", tc.raw, string(got), tc.want)
			}
		})
	}
}
