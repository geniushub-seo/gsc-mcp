package mcpfix

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sdkSchemaValidationPrefix is the fixed prefix the go-sdk v1.7.0 uses when
// applySchema fails inside a tool handler (see mcp/server.go SetError path).
// A dedicated pin test asserts live SDK errors still start with this string so
// upgrades that change the wording fail loudly instead of silently disabling
// the rewrite below.
const sdkSchemaValidationPrefix = `validating "arguments":`

// StructureSchemaValidationErrors returns a receiving middleware that rewrites
// SDK JSON-Schema validation failures into this project's structured tool
// error shape ({"error":"invalid_input",...}).
//
// Schema validation happens inside the tool handler after middleware is
// entered, and failures return a CallToolResult with IsError=true (not a Go
// error). Middleware must therefore inspect next()'s result, not only its
// error return value.
func StructureSchemaValidationErrors() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil {
				return res, err
			}
			return rewriteSchemaValidationResult(res), nil
		}
	}
}

// rewriteSchemaValidationResult converts a bare SDK schema-validation
// CallToolResult into a structured invalid_input error. Other results pass
// through unchanged.
func rewriteSchemaValidationResult(res mcp.Result) mcp.Result {
	call, ok := res.(*mcp.CallToolResult)
	if !ok || call == nil || !call.IsError {
		return res
	}

	text := firstTextContent(call.Content)
	if !strings.HasPrefix(text, sdkSchemaValidationPrefix) {
		return res
	}

	body, _ := json.Marshal(map[string]string{
		"error":      "invalid_input",
		"message":    text,
		"suggestion": "check the request parameters against the tool schema",
	})
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(body)},
		},
	}
}

func firstTextContent(content []mcp.Content) string {
	for _, c := range content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
