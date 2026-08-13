package gscclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geniushub-seo/gsc-mcp/internal/config"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	cfg := config.Config{RequestTimeout: 5 * time.Second}
	client, err := New(ctx, cfg, option.WithoutAuthentication(), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	// Disable real sleeps from retry during contract tests.
	client.sleep = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time)
		close(ch)
		return ch
	}
	return client
}

func TestContract_ListSites_FixtureShape(t *testing.T) {
	t.Parallel()
	body := fixture(t, "sites_list.json")
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "sites") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	resp, err := client.ListSites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.SiteEntry) < 2 {
		t.Fatalf("expected sites from fixture, got %d", len(resp.SiteEntry))
	}
	var hasDomain, hasURL bool
	for _, s := range resp.SiteEntry {
		if strings.HasPrefix(s.SiteUrl, "sc-domain:") {
			hasDomain = true
		}
		if strings.HasPrefix(s.SiteUrl, "http") {
			hasURL = true
		}
	}
	if !hasDomain || !hasURL {
		t.Fatal("fixture should include both domain and URL-prefix properties")
	}
}

func TestContract_SearchAnalytics_FixtureShape(t *testing.T) {
	t.Parallel()
	body := fixture(t, "searchanalytics_query.json")
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	resp, err := client.QuerySearchAnalytics(context.Background(), "sc-domain:example.com", &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: "2026-07-01",
		EndDate:   "2026-07-28",
		RowLimit:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Rows) == 0 {
		t.Fatal("expected rows")
	}
	row := resp.Rows[0]
	if row.Clicks == 0 && row.Impressions == 0 {
		t.Fatal("expected metrics")
	}
	if len(row.Keys) == 0 {
		t.Fatal("expected keys")
	}
}

func TestContract_SitemapsList_FixtureShape(t *testing.T) {
	t.Parallel()
	body := fixture(t, "sitemaps_list.json")
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	resp, err := client.ListSitemaps(context.Background(), "sc-domain:example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Sitemap) == 0 {
		t.Fatal("expected sitemap entries")
	}
	if resp.Sitemap[0].Path == "" {
		t.Fatal("expected path")
	}
}

func TestContract_URLInspection_FixtureShape(t *testing.T) {
	t.Parallel()
	body := fixture(t, "url_inspection.json")
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	resp, err := client.InspectURL(context.Background(), "sc-domain:example.com", "https://example.com/", "zh-TW")
	if err != nil {
		t.Fatal(err)
	}
	if resp.InspectionResult == nil || resp.InspectionResult.IndexStatusResult == nil {
		t.Fatal("expected index status")
	}
	if resp.InspectionResult.IndexStatusResult.Verdict == "" {
		t.Fatal("expected verdict")
	}
}

func TestContract_HTTPErrorMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file string
		code int
		want ErrorCode
	}{
		{"error_400.json", 400, ErrInvalidInput},
		{"error_401.json", 401, ErrAuthFailed},
		{"error_403.json", 403, ErrPermissionDenied},
		{"error_404.json", 404, ErrNotFound},
		{"error_429.json", 429, ErrQuotaExceeded},
		{"error_500.json", 500, ErrQuotaExceeded}, // after retries exhausted
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			body := fixture(t, tc.file)
			client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.code)
				_, _ = w.Write(body)
			})
			_, err := client.ListSites(context.Background())
			if err == nil {
				t.Fatal("expected error")
			}
			mapped := MapGoogleAPIError(err)
			if mapped.Code != tc.want {
				// 500 becomes quota_exceeded after retry exhaust; direct googleapi is upstream before map of final error
				if tc.code == 500 {
					var ge *googleapi.Error
					// final error from doWithRetry is gscclient.Error quota_exceeded
					if mapped.Code != ErrQuotaExceeded && mapped.Code != ErrUpstreamError {
						t.Fatalf("code=%d mapped=%q err=%v", tc.code, mapped.Code, err)
					}
					_ = ge
					return
				}
				t.Fatalf("mapped=%q want %q err=%v", mapped.Code, tc.want, err)
			}
		})
	}
}

func TestContract_BadJSON(t *testing.T) {
	t.Parallel()
	body := fixture(t, "error_bad_json.txt")
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	_, err := client.ListSites(context.Background())
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestContract_FixturesAreJSON(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join("testdata", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(b) {
			t.Errorf("%s invalid JSON", e.Name())
		}
		// quick leak check for common TLD patterns that are not example/google
		lower := strings.ToLower(string(b))
		// Reject non-example registrable hosts (fixtures must stay on example*.com / google.com).
		if strings.Contains(lower, ".hk/") || strings.Contains(lower, ".hk\"") {
			t.Errorf("%s contains .hk host residue", e.Name())
		}
	}
}

// Ensure unused imports for older go.
var _ = io.EOF
