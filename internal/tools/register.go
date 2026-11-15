package tools

import (
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"github.com/geniushub-seo/gsc-mcp/internal/mcpfix"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register adds all GSC tools to the MCP server and installs compatibility
// middleware. This is the single place where tools and middleware are wired
// together.
func Register(srv *mcp.Server, client *gscclient.Client, flags WriteFlags) {
	// Coerce runs first (outermost next() path after schema check still sees
	// repaired args). Schema-error rewrite inspects next()'s CallToolResult.
	srv.AddReceivingMiddleware(mcpfix.StructureSchemaValidationErrors())
	srv.AddReceivingMiddleware(mcpfix.CoerceStringifiedArrayArgs())

	quota := gscclient.NewDailyQuota()

	registerListSites(srv, client)
	registerGetSite(srv, client)
	registerQuerySearchAnalytics(srv, client)
	registerInspectURL(srv, client, quota)
	registerComparePeriods(srv, client)
	registerFindCTRGaps(srv, client)
	registerFindPageTwoQueries(srv, client)
	registerFindDecliningPages(srv, client)
	registerManageSitemaps(srv, client, flags)
}
