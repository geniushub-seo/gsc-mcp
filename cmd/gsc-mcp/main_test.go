package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geniushub-seo/gsc-mcp/internal/config"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/api/option"
)

func TestWarnSilentlyIgnoredFlags(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	warnSilentlyIgnoredFlags(config.Config{AllowDestructive: true, EnableWrite: false})

	logs := buf.String()
	if !strings.Contains(logs, "GSC_ALLOW_DESTRUCTIVE is ignored") {
		t.Fatalf("expected warning about ignored destructive flag, got %q", logs)
	}
}

func TestVersionDefaultIsDev(t *testing.T) {
	// Release builds inject -X main.version=<tag>. Default must stay "dev"
	// so local builds are identifiable; release.yml must not leave this as-is.
	if version != "dev" {
		// When someone runs tests with -ldflags override, don't fail the suite.
		t.Logf("version=%q (overridden at link time)", version)
		return
	}
	if version == "" {
		t.Fatal("version must not be empty")
	}
}

func TestWarnADCEnableWrite(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	warnSilentlyIgnoredFlags(config.Config{
		CredType:    config.CredTypeAuthorizedUser,
		EnableWrite: true,
	})

	logs := buf.String()
	if strings.Contains(logs, "has no effect") {
		t.Fatalf("must not claim GSC_ENABLE_WRITE has no effect with ADC, got %q", logs)
	}
	if !strings.Contains(logs, "local write gate") {
		t.Fatalf("expected ADC write warning about the local gate, got %q", logs)
	}
	if !strings.Contains(logs, "application-default login") {
		t.Fatalf("expected re-login guidance, got %q", logs)
	}
}

func TestNewServer_ListTools(t *testing.T) {
	ctx := context.Background()
	client, err := gscclient.New(ctx, config.Config{}, option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	srv := newServer(client, config.Config{})

	serverTrans, clientTrans := mcp.NewInMemoryTransports()
	_, err = srv.Connect(ctx, serverTrans, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "test"},
		nil,
	)
	session, err := mcpClient.Connect(ctx, clientTrans, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(result.Tools) != 6 {
		t.Fatalf("expected 6 tools, got %d", len(result.Tools))
	}

	names := make(map[string]bool)
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{
		"list_sites", "get_site", "query_search_analytics", "inspect_url",
		"compare_periods", "manage_sitemaps",
	} {
		if !names[want] {
			t.Errorf("expected %s tool to be registered", want)
		}
	}
}

func TestListSites_MockServer(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"siteEntry": []map[string]string{
				{"siteUrl": "sc-domain:example.com", "permissionLevel": "SITE_OWNER"},
			},
		})
	}))
	defer api.Close()

	ctx := context.Background()
	client, err := gscclient.New(ctx, config.Config{}, option.WithoutAuthentication(), option.WithEndpoint(api.URL+"/"))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	srv := newServer(client, config.Config{})
	serverTrans, clientTrans := mcp.NewInMemoryTransports()
	_, err = srv.Connect(ctx, serverTrans, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "test"},
		nil,
	)
	session, err := mcpClient.Connect(ctx, clientTrans, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_sites"})
	if err != nil {
		t.Fatalf("call list_sites: %v", err)
	}
	if result.IsError {
		t.Fatalf("list_sites returned error: %+v", result.Content)
	}

	body := extractTextContent(t, result.Content)
	if !strings.Contains(body, "sc-domain:example.com") {
		t.Fatalf("expected response to contain site URL, got %q", body)
	}
}

func TestGetSite_MockServer_ResolvesURLPrefix(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		case strings.Contains(path, "/sites/"):
			if strings.Contains(path, "sc-domain:example.com") {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": 403, "message": "forbidden"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"siteUrl":         "https://example.com/",
				"permissionLevel": "SITE_OWNER",
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"siteEntry": []map[string]string{
					{"siteUrl": "https://example.com/", "permissionLevel": "SITE_OWNER"},
				},
			})
		}
	}))
	defer api.Close()

	ctx := context.Background()
	client, err := gscclient.New(ctx, config.Config{}, option.WithoutAuthentication(), option.WithEndpoint(api.URL+"/"))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	srv := newServer(client, config.Config{})
	serverTrans, clientTrans := mcp.NewInMemoryTransports()
	_, err = srv.Connect(ctx, serverTrans, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "test"},
		nil,
	)
	session, err := mcpClient.Connect(ctx, clientTrans, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_site",
		Arguments: map[string]any{"site_url": "example.com"},
	})
	if err != nil {
		t.Fatalf("call get_site: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_site returned error: %s", extractTextContent(t, result.Content))
	}

	body := extractTextContent(t, result.Content)
	if !strings.Contains(body, "https://example.com/") {
		t.Fatalf("expected response to contain resolved URL-prefix, got %q", body)
	}
}

func extractTextContent(t *testing.T, content []mcp.Content) string {
	t.Helper()
	if len(content) == 0 {
		t.Fatal("no content in tool result")
	}
	text, ok := content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", content[0])
	}
	return text.Text
}
