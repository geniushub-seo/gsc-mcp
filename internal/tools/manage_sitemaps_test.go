package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestManageSitemaps_WriteFlags(t *testing.T) {
	t.Parallel()
	var apiCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")

	cases := []struct {
		name    string
		flags   WriteFlags
		action  string
		wantMsg string
	}{
		{
			name:    "submit without write",
			flags:   WriteFlags{},
			action:  "submit",
			wantMsg: "GSC_ENABLE_WRITE",
		},
		{
			name:    "delete without write",
			flags:   WriteFlags{},
			action:  "delete",
			wantMsg: "GSC_ENABLE_WRITE",
		},
		{
			name:    "delete with write only",
			flags:   WriteFlags{EnableWrite: true},
			action:  "delete",
			wantMsg: "GSC_ALLOW_DESTRUCTIVE",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiCalls.Store(0)
			result, _, err := manageSitemaps(context.Background(), client, tc.flags, manageSitemapsInput{
				SiteURL:  "sc-domain:example.com",
				Action:   tc.action,
				Feedpath: "https://example.com/sitemap.xml",
			})
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected write_disabled tool error")
			}
			body := extractText(t, result.Content)
			if !strings.Contains(body, "write_disabled") {
				t.Errorf("expected write_disabled, got %s", body)
			}
			if !strings.Contains(body, tc.wantMsg) {
				t.Errorf("expected suggestion mentioning %s, got %s", tc.wantMsg, body)
			}
			if apiCalls.Load() != 0 {
				t.Fatalf("expected no API calls, got %d", apiCalls.Load())
			}
		})
	}
}

func TestManageSitemaps_ListSuccess(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sitemap": []map[string]any{
				{
					"path":            "https://example.com/sitemap.xml",
					"lastSubmitted":   "2026-07-01T00:00:00Z",
					"isPending":       false,
					"isSitemapsIndex": false,
					"warnings":        "1",
					"errors":          "0",
					"contents": []map[string]any{
						{"type": "WEB", "submitted": "42"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	result, _, err := manageSitemaps(context.Background(), client, WriteFlags{}, manageSitemapsInput{
		SiteURL: "sc-domain:example.com",
		Action:  "list",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", extractText(t, result.Content))
	}
	body := extractText(t, result.Content)
	if !strings.Contains(body, "https://example.com/sitemap.xml") {
		t.Fatalf("expected sitemap path in body, got %s", body)
	}
}

func TestManageSitemaps_ListEmptyReturnsEmptyArray(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// No sitemap field — Google returns empty when the property has none.
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	result, _, err := manageSitemaps(context.Background(), client, WriteFlags{}, manageSitemapsInput{
		SiteURL: "sc-domain:example.com",
		Action:  "list",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", extractText(t, result.Content))
	}
	body := extractText(t, result.Content)
	if !strings.Contains(body, `"sitemaps":[]`) && !strings.Contains(body, `"sitemaps": []`) {
		t.Fatalf("expected sitemaps key with empty array, got %s", body)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["sitemaps"]; !ok {
		t.Fatal("sitemaps key must be present")
	}
}

func TestManageSitemaps_FeedpathRequired(t *testing.T) {
	t.Parallel()
	client := newTestClient(t, "http://unused.example/")
	result, _, err := manageSitemaps(context.Background(), client, WriteFlags{EnableWrite: true}, manageSitemapsInput{
		SiteURL: "sc-domain:example.com",
		Action:  "get",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected invalid_input")
	}
	if !strings.Contains(extractText(t, result.Content), "feedpath") {
		t.Fatalf("expected feedpath error, got %s", extractText(t, result.Content))
	}
}

func TestManageSitemaps_DeleteAllowedMakesAPICall(t *testing.T) {
	t.Parallel()
	var apiCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL+"/")
	result, _, err := manageSitemaps(context.Background(), client, WriteFlags{
		EnableWrite:      true,
		AllowDestructive: true,
	}, manageSitemapsInput{
		SiteURL:  "sc-domain:example.com",
		Action:   "delete",
		Feedpath: "https://example.com/sitemap.xml",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", extractText(t, result.Content))
	}
	if apiCalls.Load() == 0 {
		t.Fatal("expected API call when flags allow delete")
	}
	if !strings.Contains(extractText(t, result.Content), "deleted") {
		t.Fatalf("expected deleted message, got %s", extractText(t, result.Content))
	}
}
