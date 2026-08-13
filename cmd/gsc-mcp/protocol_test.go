package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/geniushub-seo/gsc-mcp/internal/config"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"google.golang.org/api/option"
)

func TestProtocol_ListToolsSchema(t *testing.T) {
	ctx := context.Background()
	client, err := gscclient.New(ctx, config.Config{}, option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	srv := newServer(client, config.Config{})

	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "t"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 6 {
		t.Fatalf("tools = %d, want 6", len(result.Tools))
	}

	// Dump golden schemas and verify no union types remain.
	goldenDir := filepath.Join("testdata")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	schemas := map[string]any{}
	unionFields := []string{}

	for _, tool := range result.Tools {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s schema not object: %v", tool.Name, err)
		}
		schemas[tool.Name] = schema

		if _, ok := schema["properties"]; !ok {
			t.Errorf("%s missing properties", tool.Name)
		}
		if _, ok := schema["required"]; !ok {
			// required may be absent when empty for some tools; list_sites has it.
			if tool.Name == "list_sites" {
				t.Errorf("%s missing required", tool.Name)
			}
		}
		if ap, ok := schema["additionalProperties"]; !ok || ap != false {
			t.Errorf("%s additionalProperties = %v, want false", tool.Name, schema["additionalProperties"])
		}
		if typ, ok := schema["type"].(string); !ok || typ != "object" {
			t.Errorf("%s type = %v, want object", tool.Name, schema["type"])
		}

		findUnions(tool.Name, "", schema, &unionFields)
	}

	if len(unionFields) > 0 {
		t.Errorf("schemas contain union/null types: %v", unionFields)
	}

	out, _ := json.MarshalIndent(map[string]any{
		"tools":        schemas,
		"union_fields": unionFields,
		"note":         "schemas are explicitly written to avoid [null,array] union types",
	}, "", "  ")
	if err := os.WriteFile(filepath.Join(goldenDir, "tool_schemas.json"), append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findUnions(tool, path string, v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		if typ, ok := x["type"]; ok {
			if arr, ok := typ.([]any); ok {
				hasNull, hasArray := false, false
				for _, e := range arr {
					if s, ok := e.(string); ok {
						if s == "null" {
							hasNull = true
						}
						if s == "array" {
							hasArray = true
						}
					}
				}
				if hasNull && hasArray {
					*out = append(*out, tool+":"+path)
				}
			}
		}
		if props, ok := x["properties"].(map[string]any); ok {
			for k, child := range props {
				p := k
				if path != "" {
					p = path + "." + k
				}
				findUnions(tool, p, child, out)
			}
		}
		if items, ok := x["items"]; ok {
			findUnions(tool, path+"[]", items, out)
		}
	case []any:
		for i, child := range x {
			findUnions(tool, path+sprintfIndex(i), child, out)
		}
	}
}

func sprintfIndex(i int) string {
	return fmt.Sprintf("[%d]", i)
}

func TestProtocol_CallToolIsError(t *testing.T) {
	ctx := context.Background()
	client, err := gscclient.New(ctx, config.Config{}, option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	srv := newServer(client, config.Config{})
	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "t"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	// Validation error should set IsError without needing network.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "query_search_analytics",
		Arguments: map[string]any{
			"site_url":   "example.com",
			"start_date": "2026-08-01",
			"end_date":   "2026-07-01", // inverted
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected IsError true for invalid dates")
	}
}

func TestProtocol_StringifiedDimensionsMiddleware(t *testing.T) {
	// Middleware is on the server; use mock endpoint so the call can succeed after coerce.
	// Without network the call fails upstream — we only assert middleware doesn't reject schema.
	ctx := context.Background()
	client, err := gscclient.New(ctx, config.Config{}, option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	srv := newServer(client, config.Config{})
	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "t"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	// If stringified arrays were not coerced, SDK schema validation would fail
	// before the handler (protocol error). Getting a tool result (even IsError
	// from upstream) proves coercion worked.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "query_search_analytics",
		Arguments: map[string]any{
			"site_url":   "sc-domain:example.com",
			"start_date": "2026-07-01",
			"end_date":   "2026-07-07",
			"dimensions": `["query"]`,
		},
	})
	if err != nil {
		t.Fatalf("protocol error (coercion may have failed): %v", err)
	}
	// Expect tool-level error (no real creds/endpoint), not a panic.
	if result == nil {
		t.Fatal("nil result")
	}
}
