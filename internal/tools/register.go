package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"github.com/geniushub-seo/gsc-mcp/internal/mcpfix"
)

// Register adds all GSC tools to the MCP server and installs compatibility
// middleware. This is the single place where tools and middleware are wired
// together.
func Register(srv *mcp.Server, client *gscclient.Client, flags WriteFlags) {
	// Coerce runs first (outermost next() path after schema check still sees
	// repaired args). Schema-error rewrite inspects next()'s CallToolResult.
	srv.AddReceivingMiddleware(mcpfix.StructureSchemaValidationErrors())
	srv.AddReceivingMiddleware(mcpfix.CoerceStringifiedArrayArgs())

	quota := gscclient.NewDailyQuota(0)

	registerListSites(srv, client)
	registerGetSite(srv, client)
	registerQuerySearchAnalytics(srv, client)
	registerInspectURL(srv, client, quota)
	registerComparePeriods(srv, client)
	registerManageSitemaps(srv, client, flags)
}
