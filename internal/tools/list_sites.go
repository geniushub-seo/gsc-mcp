package tools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
)

// listSitesInputSchema is the explicit no-argument JSON Schema for the
// list_sites tool. SDK inference from struct{} omits properties/required,
// which breaks strict MCP clients (e.g. Copilot CLI) and causes the entire
// tool list to be rejected.
var listSitesInputSchema = json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`)

// listSitesInput is the typed input for list_sites (no fields).
type listSitesInput struct{}

// listSitesOutput is the successful response shape for list_sites.
type listSitesOutput struct {
	QueriedAt string     `json:"queried_at"`
	Sites     []siteInfo `json:"sites"`
}

type siteInfo struct {
	SiteURL         string `json:"site_url"`
	PermissionLevel string `json:"permission_level"`
}

func registerListSites(srv *mcp.Server, client *gscclient.Client) {
	mcp.AddTool(srv,
		&mcp.Tool{
			Name:        "list_sites",
			Description: descListSites,
			InputSchema: listSitesInputSchema,
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ listSitesInput) (*mcp.CallToolResult, any, error) {
			return listSites(ctx, client)
		},
	)
}

func listSites(ctx context.Context, client *gscclient.Client) (*mcp.CallToolResult, any, error) {
	resp, err := client.ListSites(ctx)
	if err != nil {
		return toolError(gscclient.MapGoogleAPIError(err)), nil, nil
	}

	out := listSitesOutput{
		QueriedAt: nowRFC3339(),
		Sites:     make([]siteInfo, 0, len(resp.SiteEntry)),
	}
	for _, s := range resp.SiteEntry {
		out.Sites = append(out.Sites, siteInfo{
			SiteURL:         s.SiteUrl,
			PermissionLevel: s.PermissionLevel,
		})
	}
	return toolResult(out), nil, nil
}
