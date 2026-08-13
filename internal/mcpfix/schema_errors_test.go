package mcpfix

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSDKSchemaValidationPrefix_Pinned(t *testing.T) {
	t.Parallel()
	// Intentionally register a tool with a required field and call it without
	// that field, WITHOUT the rewrite middleware, so we observe the raw SDK
	// error string. If go-sdk changes its wording this test fails.
	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{Name: "pin", Version: "t"}, nil)
	mcp.AddTool(srv, &mcp.Tool{
		Name: "needs_site",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{"site_url":{"type":"string"}},
			"required":["site_url"]
		}`),
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct {
		SiteURL string `json:"site_url"`
	}) (*mcp.CallToolResult, any, error) {
		t.Fatal("handler must not run when required args are missing")
		return nil, nil, nil
	})

	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "t"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "needs_site",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("protocol error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for missing required arg")
	}
	text := firstTextContent(result.Content)
	if !strings.HasPrefix(text, sdkSchemaValidationPrefix) {
		t.Fatalf("SDK error prefix drift: got %q, want prefix %q", text, sdkSchemaValidationPrefix)
	}
}

func TestStructureSchemaValidationErrors_RewritesMissingRequired(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{Name: "rewrite", Version: "t"}, nil)
	srv.AddReceivingMiddleware(StructureSchemaValidationErrors())
	mcp.AddTool(srv, &mcp.Tool{
		Name: "needs_site",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"additionalProperties":false,
			"properties":{"site_url":{"type":"string"}},
			"required":["site_url"]
		}`),
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct {
		SiteURL string `json:"site_url"`
	}) (*mcp.CallToolResult, any, error) {
		t.Fatal("handler must not run when required args are missing")
		return nil, nil, nil
	})

	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "t"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "needs_site",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("protocol error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError")
	}
	text := firstTextContent(result.Content)
	var body map[string]string
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("expected structured JSON error, got %q: %v", text, err)
	}
	if body["error"] != "invalid_input" {
		t.Fatalf("error = %q, want invalid_input", body["error"])
	}
	if !strings.HasPrefix(body["message"], sdkSchemaValidationPrefix) {
		t.Fatalf("message should preserve SDK detail, got %q", body["message"])
	}
	if body["suggestion"] == "" {
		t.Fatal("expected non-empty suggestion")
	}
	if strings.HasPrefix(text, sdkSchemaValidationPrefix) {
		t.Fatalf("top-level content must not remain a bare SDK string: %q", text)
	}
}

func TestRewriteSchemaValidationResult_Passthrough(t *testing.T) {
	t.Parallel()
	// Non-schema tool error must not be rewritten.
	in := &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: `{"error":"permission_denied","message":"no"}`}},
	}
	out := rewriteSchemaValidationResult(in)
	got, ok := out.(*mcp.CallToolResult)
	if !ok || got != in {
		t.Fatal("expected passthrough of non-schema error")
	}
}
