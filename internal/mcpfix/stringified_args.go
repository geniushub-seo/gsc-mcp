// Package mcpfix contains MCP-sdk compatibility fixes that must run as
// middleware, before the SDK validates tool arguments against their JSON
// Schema.
package mcpfix

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolArrayFields declares, for each tool name, which top-level argument fields
// are array-typed. coerceStringifiedArrayArgs uses this to decide which fields
// to repair.
var toolArrayFields = map[string][]string{
	"query_search_analytics": {"dimensions", "dimension_filter_groups"},
	"compare_periods":        {"dimensions", "dimension_filter_groups"},
	"inspect_url":            {"urls"},
}

// CoerceStringifiedArrayArgs returns a receiving middleware that repairs a
// widespread MCP client bug: some clients JSON-encode an array-typed tool
// argument as a string (e.g. `"[\"query\"]"` instead of `["query"]`).
//
// This must run as middleware, not inside a tool handler: the SDK validates
// arguments against the tool's JSON Schema before a registered handler is ever
// invoked, so a malformed argument never reaches the handler to be fixed there.
// Middleware runs earlier, while the arguments are still raw JSON.
func CoerceStringifiedArrayArgs() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			call, ok := req.(*mcp.CallToolRequest)
			if !ok || method != "tools/call" {
				return next(ctx, method, req)
			}

			fields := toolArrayFields[call.Params.Name]
			if len(fields) == 0 || len(call.Params.Arguments) == 0 {
				return next(ctx, method, req)
			}

			var args map[string]json.RawMessage
			if err := json.Unmarshal(call.Params.Arguments, &args); err != nil {
				return next(ctx, method, req)
			}

			changed := false
			for _, field := range fields {
				if coerced, ok := coerceStringifiedArray(args[field]); ok {
					args[field] = coerced
					changed = true
				}
			}

			if changed {
				if rewritten, err := json.Marshal(args); err == nil {
					call.Params.Arguments = rewritten
				}
			}

			return next(ctx, method, req)
		}
	}
}

// coerceStringifiedArray reports whether raw is a JSON string that itself
// decodes to a JSON array, returning that array's raw JSON if so. It returns
// ok=false for a raw value that is missing, already an array, or a string that
// doesn't decode to an array -- in all of those cases the caller should leave
// the value untouched and let normal schema validation handle it.
func coerceStringifiedArray(raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err != nil {
		return nil, false
	}

	var probe []json.RawMessage
	if err := json.Unmarshal([]byte(asString), &probe); err != nil {
		return nil, false
	}

	return json.RawMessage(asString), true
}
