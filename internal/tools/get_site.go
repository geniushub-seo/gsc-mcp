package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"google.golang.org/api/searchconsole/v1"
)

// getSiteInput is the input schema for the get_site tool.
type getSiteInput struct {
	SiteURL string `json:"site_url" jsonschema:"The GSC property to look up. Supports bare domain (example.com), full URL (https://www.example.com/), or canonical GSC property format (sc-domain:example.com)."`
}

// getSiteOutput is the successful response shape for get_site.
type getSiteOutput struct {
	QueriedAt       string `json:"queried_at"`
	SiteURL         string `json:"site_url"`
	PermissionLevel string `json:"permission_level"`
}

func registerGetSite(srv *mcp.Server, client *gscclient.Client) {
	mcp.AddTool(srv,
		&mcp.Tool{
			Name:        "get_site",
			Description: descGetSite,
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, input getSiteInput) (*mcp.CallToolResult, any, error) {
			return getSite(ctx, client, input.SiteURL)
		},
	)
}

func getSite(ctx context.Context, client *gscclient.Client, siteURL string) (*mcp.CallToolResult, any, error) {
	resp, err := gscclient.WithResolvedSiteURL(ctx, client, siteURL, func(ctx context.Context, resolved string) (*searchconsole.WmxSite, error) {
		return client.GetSite(ctx, resolved)
	})
	if err != nil {
		return toolError(gscclient.MapGoogleAPIError(err)), nil, nil
	}

	out := getSiteOutput{
		QueriedAt:       nowRFC3339(),
		SiteURL:         resp.SiteUrl,
		PermissionLevel: resp.PermissionLevel,
	}
	return toolResult(out), nil, nil
}
