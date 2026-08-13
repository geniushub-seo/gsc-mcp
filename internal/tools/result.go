package tools

import (
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
)

// toolResult returns a successful CallToolResult with structured JSON content.
func toolResult(data any) *mcp.CallToolResult {
	b, err := json.Marshal(data)
	if err != nil {
		return toolError(gscclient.NewError(
			gscclient.ErrUpstreamError,
			"failed to marshal tool result",
			"retry the request; if it persists, report a server bug",
		))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(b)},
		},
	}
}

// toolError returns a CallToolResult with IsError set to true and a structured
// error body. It is used for all tool-level failures so the MCP client can
// distinguish them from protocol errors.
func toolError(err gscclient.Error) *mcp.CallToolResult {
	body := map[string]string{
		"error":      string(err.Code),
		"message":    err.Message,
		"suggestion": err.Suggestion,
	}
	b, _ := json.Marshal(body)

	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(b)},
		},
	}
}

// nowRFC3339 returns the current UTC time formatted as RFC3339 for the
// queried_at field included in every successful tool response.
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
