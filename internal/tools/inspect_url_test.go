package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"google.golang.org/api/searchconsole/v1"
)

func TestInspectURL_RejectsBadURLCounts(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, "http://unused.example/")
	quota := gscclient.NewDailyQuota(100)

	for _, urls := range [][]string{nil, {}, make([]string, 11)} {
		urls := urls
		result, _, err := inspectURL(context.Background(), client, quota, inspectURLInput{
			SiteURL: "sc-domain:example.com",
			URLs:    urls,
		}, time.Sleep, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected invalid_input for bad urls length")
		}
		if !strings.Contains(extractText(t, result.Content), "invalid_input") {
			t.Fatalf("expected invalid_input, got %s", extractText(t, result.Content))
		}
	}
}

func TestInspectURL_AbsentOptionalSections(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "urlInspection") || strings.Contains(r.URL.Path, "inspect") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"inspectionResult": map[string]any{
					"inspectionResultLink": "https://search.google.com/search-console/inspect/example",
					"indexStatusResult": map[string]any{
						"verdict":       "PASS",
						"coverageState": "Submitted and indexed",
						"indexingState": "INDEXING_ALLOWED",
						"crawledAs":     "MOBILE",
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	quota := gscclient.NewDailyQuota(100)
	result, _, err := inspectURL(context.Background(), client, quota, inspectURLInput{
		SiteURL: "sc-domain:example.com",
		URLs:    []string{"https://example.com/page"},
	}, time.Sleep, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("inspectURL error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", extractText(t, result.Content))
	}

	body := extractText(t, result.Content)
	if !strings.Contains(body, "Submitted and indexed") {
		t.Fatalf("expected index status in body, got %s", body)
	}
	for _, absent := range []string{"mobile_usability_verdict", "rich_results_verdict", "amp_verdict", "detectedItems"} {
		if strings.Contains(body, absent) {
			t.Errorf("body should not contain %q: %s", absent, body)
		}
	}
	if !strings.Contains(body, `"quota_used_today":1`) {
		t.Errorf("expected quota_used_today 1, got %s", body)
	}
}

func TestInspectURL_BatchCallCountAndInterval(t *testing.T) {
	t.Parallel()
	var (
		mu        sync.Mutex
		callTimes []time.Time
		bodies    [][]byte
	)

	// Disable real sleep; record synthetic timestamps via a stub.
	var last time.Time
	sleep := func(d time.Duration) {
		mu.Lock()
		last = last.Add(d)
		mu.Unlock()
	}
	interval := 100 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "urlInspection") || strings.Contains(r.URL.Path, "inspect") {
			mu.Lock()
			if last.IsZero() {
				last = time.Now()
			}
			callTimes = append(callTimes, last)
			b, _ := io.ReadAll(r.Body)
			bodies = append(bodies, b)
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&searchconsole.InspectUrlIndexResponse{
				InspectionResult: &searchconsole.UrlInspectionResult{
					InspectionResultLink: "https://search.google.com/search-console/inspect/example",
					IndexStatusResult: &searchconsole.IndexStatusInspectionResult{
						Verdict: "PASS",
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	quota := gscclient.NewDailyQuota(100)
	urls := []string{
		"https://example.com/a",
		"https://example.com/b",
		"https://example.com/c",
	}
	result, _, err := inspectURL(context.Background(), client, quota, inspectURLInput{
		SiteURL: "sc-domain:example.com",
		URLs:    urls,
	}, sleep, interval)
	if err != nil {
		t.Fatalf("inspectURL error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", extractText(t, result.Content))
	}

	mu.Lock()
	defer mu.Unlock()
	if len(callTimes) != 3 {
		t.Fatalf("expected 3 inspect calls, got %d", len(callTimes))
	}
	// After stub sleep, gaps between recorded times should be >= interval.
	for i := 1; i < len(callTimes); i++ {
		gap := callTimes[i].Sub(callTimes[i-1])
		if gap < interval {
			t.Fatalf("gap between call %d and %d = %v, want >= %v", i-1, i, gap, interval)
		}
	}

	// language_code default zh-TW should appear in request bodies.
	for i, b := range bodies {
		var req searchconsole.InspectUrlIndexRequest
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("unmarshal body %d: %v", i, err)
		}
		if req.LanguageCode != "zh-TW" {
			t.Errorf("body %d languageCode = %q, want zh-TW", i, req.LanguageCode)
		}
		if req.InspectionUrl != urls[i] {
			t.Errorf("body %d inspectionUrl = %q, want %q", i, req.InspectionUrl, urls[i])
		}
	}

	body := extractText(t, result.Content)
	if !strings.Contains(body, `"quota_used_today":3`) {
		t.Errorf("expected quota_used_today 3, got %s", body)
	}
}

func TestMapInspectResult_NilOptionalSections(t *testing.T) {
	t.Parallel()
	got := mapInspectResult("https://example.com/", &searchconsole.InspectUrlIndexResponse{
		InspectionResult: &searchconsole.UrlInspectionResult{
			InspectionResultLink: "link",
			IndexStatusResult: &searchconsole.IndexStatusInspectionResult{
				Verdict: "PASS",
			},
			// MobileUsabilityResult, RichResultsResult, AmpResult intentionally nil
		},
	})
	if got.MobileUsabilityVerdict != "" || got.RichResultsVerdict != "" || got.AmpVerdict != "" {
		t.Fatalf("expected empty optional verdicts, got %+v", got)
	}
	if got.IndexStatus == nil || got.IndexStatus.Verdict != "PASS" {
		t.Fatalf("expected index status PASS, got %+v", got.IndexStatus)
	}
}
