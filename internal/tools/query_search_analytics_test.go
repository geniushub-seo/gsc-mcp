package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geniushub-seo/gsc-mcp/internal/config"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"
)

func newTestClient(t *testing.T, endpoint string) *gscclient.Client {
	t.Helper()
	ctx := context.Background()
	client, err := gscclient.New(ctx, config.Config{}, option.WithoutAuthentication(), option.WithEndpoint(endpoint))
	if err != nil {
		t.Fatalf("create test client: %v", err)
	}
	return client
}

func TestQuerySearchAnalytics_ExcludingRegexRequestBody(t *testing.T) {
	t.Parallel()
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/searchAnalytics/query") {
			gotBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&searchconsole.SearchAnalyticsQueryResponse{
				Rows: []*searchconsole.ApiDataRow{
					{Keys: []string{"non-branded query"}, Clicks: 42, Impressions: 1000, Ctr: 0.042, Position: 8.5},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	input := querySearchAnalyticsInput{
		SiteURL:    "sc-domain:example.com",
		StartDate:  "2026-07-01",
		EndDate:    "2026-07-31",
		Dimensions: []string{"query"},
		DimensionFilterGroups: []DimensionFilterGroup{{
			GroupType: "and",
			Filters: []DimensionFilter{{
				Dimension:  "query",
				Operator:   "excludingRegex",
				Expression: "(?i)brand",
			}},
		}},
	}

	result, _, err := querySearchAnalytics(context.Background(), client, input)
	if err != nil {
		t.Fatalf("querySearchAnalytics returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("querySearchAnalytics returned tool error: %s", extractText(t, result.Content))
	}

	var got searchconsole.SearchAnalyticsQueryRequest
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}

	if len(got.DimensionFilterGroups) != 1 {
		t.Fatalf("expected 1 filter group, got %d", len(got.DimensionFilterGroups))
	}
	filters := got.DimensionFilterGroups[0].Filters
	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Operator != "EXCLUDING_REGEX" {
		t.Errorf("operator = %q, want EXCLUDING_REGEX", filters[0].Operator)
	}
	if filters[0].Dimension != "QUERY" {
		t.Errorf("dimension = %q, want QUERY", filters[0].Dimension)
	}
	if filters[0].Expression != "(?i)brand" {
		t.Errorf("expression = %q, want (?i)brand", filters[0].Expression)
	}

	body := extractText(t, result.Content)
	if !strings.Contains(body, "\"row_count\":1") {
		t.Errorf("expected row_count 1 in response, got %s", body)
	}
}

func TestQuerySearchAnalytics_SearchTypeUsesTypeField(t *testing.T) {
	t.Parallel()
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/searchAnalytics/query") {
			gotBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&searchconsole.SearchAnalyticsQueryResponse{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	result, _, err := querySearchAnalytics(context.Background(), client, querySearchAnalyticsInput{
		SiteURL:    "sc-domain:example.com",
		StartDate:  "2026-07-01",
		EndDate:    "2026-07-31",
		SearchType: "image",
	})
	if err != nil {
		t.Fatalf("querySearchAnalytics returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("querySearchAnalytics returned tool error: %s", extractText(t, result.Content))
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(gotBody, &raw); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if _, ok := raw["type"]; !ok {
		t.Fatalf("expected body to contain \"type\", got %s", gotBody)
	}
	if _, ok := raw["searchType"]; ok {
		t.Fatalf("body must not contain \"searchType\", got %s", gotBody)
	}
	var typeVal string
	if err := json.Unmarshal(raw["type"], &typeVal); err != nil {
		t.Fatalf("unmarshal type: %v", err)
	}
	if typeVal != "IMAGE" {
		t.Errorf("type = %q, want IMAGE", typeVal)
	}
}

func TestQuerySearchAnalytics_OmitsSearchTypeWhenEmpty(t *testing.T) {
	t.Parallel()
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/searchAnalytics/query") {
			gotBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&searchconsole.SearchAnalyticsQueryResponse{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	result, _, err := querySearchAnalytics(context.Background(), client, querySearchAnalyticsInput{
		SiteURL:   "sc-domain:example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-31",
	})
	if err != nil {
		t.Fatalf("querySearchAnalytics returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("querySearchAnalytics returned tool error: %s", extractText(t, result.Content))
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(gotBody, &raw); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if _, ok := raw["type"]; ok {
		t.Fatalf("body must not contain \"type\" when search_type is empty, got %s", gotBody)
	}
	if _, ok := raw["searchType"]; ok {
		t.Fatalf("body must not contain \"searchType\" when search_type is empty, got %s", gotBody)
	}
}

func TestQuerySearchAnalytics_ValidationError(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, "http://unused.example/")
	input := querySearchAnalyticsInput{
		SiteURL:   "example.com",
		StartDate: "2026-08-01",
		EndDate:   "2026-07-01",
	}

	result, _, err := querySearchAnalytics(context.Background(), client, input)
	if err != nil {
		t.Fatalf("querySearchAnalytics returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected validation error")
	}
	body := extractText(t, result.Content)
	if !strings.Contains(body, "invalid_input") {
		t.Errorf("expected invalid_input code, got %s", body)
	}
}

func TestQuerySearchAnalytics_DefaultRowLimit150(t *testing.T) {
	t.Parallel()
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/searchAnalytics/query") {
			gotBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&searchconsole.SearchAnalyticsQueryResponse{})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	result, _, err := querySearchAnalytics(context.Background(), client, querySearchAnalyticsInput{
		SiteURL:   "sc-domain:example.com",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-31",
		// RowLimit omitted → default 150
	})
	if err != nil {
		t.Fatalf("querySearchAnalytics returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", extractText(t, result.Content))
	}
	var got searchconsole.SearchAnalyticsQueryRequest
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got.RowLimit != 151 {
		t.Fatalf("rowLimit in request = %d, want 151 (default 150 plus a truncation peek)", got.RowLimit)
	}
}

func TestToAnalyticsRows_RoundsAndOmitsZeroPosition(t *testing.T) {
	t.Parallel()
	rows := toAnalyticsRows([]*searchconsole.ApiDataRow{
		{Keys: []string{"noisy"}, Clicks: 5, Impressions: 121, Ctr: 0.04132231404958678, Position: 8.323494687131051},
		{Keys: []string{"empty"}, Clicks: 0, Impressions: 0, Ctr: 0, Position: 0},
	})
	if len(rows) != 2 {
		t.Fatalf("len = %d, want 2", len(rows))
	}
	if rows[0].Ctr != 0.0413 {
		t.Errorf("ctr rounded = %v, want 0.0413", rows[0].Ctr)
	}
	if rows[0].Position == nil || *rows[0].Position != 8.32 {
		t.Errorf("position rounded = %v, want 8.32", rows[0].Position)
	}
	if rows[1].Position != nil {
		t.Errorf("zero-traffic row must omit position, got %v", *rows[1].Position)
	}

	raw, err := json.Marshal(rows[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "position") {
		t.Fatalf("JSON for zero-traffic row must not contain position key: %s", raw)
	}
	// No long float tails on the noisy row either.
	raw0, _ := json.Marshal(rows[0])
	s := string(raw0)
	if strings.Contains(s, "041322") || strings.Contains(s, "323494") {
		t.Fatalf("float noise remains in JSON: %s", s)
	}
}

func TestQuerySearchAnalytics_EchoesCanonicalDomainProperty(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&searchconsole.SearchAnalyticsQueryResponse{})
	}))
	defer srv.Close()

	result, _, err := querySearchAnalytics(context.Background(), newTestClient(t, srv.URL+"/"), querySearchAnalyticsInput{
		SiteURL:   "SC-DOMAIN:EXAMPLE.COM",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-31",
	})
	if err != nil {
		t.Fatalf("querySearchAnalytics error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", extractText(t, result.Content))
	}
	var out querySearchAnalyticsOutput
	if err := json.Unmarshal([]byte(extractText(t, result.Content)), &out); err != nil {
		t.Fatal(err)
	}
	if out.SiteURL != "sc-domain:example.com" {
		t.Fatalf("site_url = %q, want canonical domain property", out.SiteURL)
	}
}

func extractText(t *testing.T, content []mcp.Content) string {
	t.Helper()
	if len(content) == 0 {
		t.Fatal("no content")
	}
	text, ok := content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", content[0])
	}
	return text.Text
}
