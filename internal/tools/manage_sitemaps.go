package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"google.golang.org/api/searchconsole/v1"
)

// WriteFlags controls which manage_sitemaps actions are allowed.
type WriteFlags struct {
	EnableWrite      bool
	AllowDestructive bool
}

type manageSitemapsInput struct {
	SiteURL      string `json:"site_url" jsonschema:"The GSC property. Supports bare domain, full URL, or canonical GSC format."`
	Action       string `json:"action" jsonschema:"One of: list, get, submit, delete."`
	Feedpath     string `json:"feedpath,omitempty" jsonschema:"Sitemap URL. Required for get, submit, and delete."`
	SitemapIndex string `json:"sitemap_index,omitempty" jsonschema:"Optional sitemap index URL; only used with action=list."`
}

type manageSitemapsOutput struct {
	QueriedAt string         `json:"queried_at"`
	SiteURL   string         `json:"site_url"`
	Action    string         `json:"action"`
	// No omitempty: empty list must serialize as [] so callers can tell
	// "property has no sitemaps" from "field missing / tool broken".
	Sitemaps []sitemapInfo `json:"sitemaps"`
	Sitemap   *sitemapInfo   `json:"sitemap,omitempty"`
	Message   string         `json:"message,omitempty"`
}

type sitemapInfo struct {
	Path            string                `json:"path"`
	LastSubmitted   string                `json:"last_submitted,omitempty"`
	LastDownloaded  string                `json:"last_downloaded,omitempty"`
	IsPending       bool                  `json:"is_pending"`
	IsSitemapsIndex bool                  `json:"is_sitemaps_index"`
	Type            string                `json:"type,omitempty"`
	Warnings        int64                 `json:"warnings"`
	Errors          int64                 `json:"errors"`
	Contents        []sitemapContentInfo  `json:"contents,omitempty"`
}

type sitemapContentInfo struct {
	Type      string `json:"type,omitempty"`
	Submitted int64  `json:"submitted"`
}

func registerManageSitemaps(srv *mcp.Server, client *gscclient.Client, flags WriteFlags) {
	mcp.AddTool(srv,
		&mcp.Tool{
			Name: "manage_sitemaps",
			Description: descManageSitemaps,
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, input manageSitemapsInput) (*mcp.CallToolResult, any, error) {
			return manageSitemaps(ctx, client, flags, input)
		},
	)
}

func manageSitemaps(ctx context.Context, client *gscclient.Client, flags WriteFlags, input manageSitemapsInput) (*mcp.CallToolResult, any, error) {
	action := strings.ToLower(strings.TrimSpace(input.Action))
	switch action {
	case "list", "get", "submit", "delete":
	default:
		return toolError(gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("invalid action %q", input.Action),
			"use one of: list, get, submit, delete",
		)), nil, nil
	}

	if action != "list" && strings.TrimSpace(input.Feedpath) == "" {
		return toolError(gscclient.NewError(
			gscclient.ErrInvalidInput,
			"feedpath is required for get, submit, and delete",
			"",
		)), nil, nil
	}

	switch action {
	case "submit":
		if !flags.EnableWrite {
			return toolError(gscclient.NewError(
				gscclient.ErrWriteDisabled,
				"sitemap submit is disabled",
				"set GSC_ENABLE_WRITE=true to allow submit",
			)), nil, nil
		}
	case "delete":
		if !flags.EnableWrite {
			return toolError(gscclient.NewError(
				gscclient.ErrWriteDisabled,
				"sitemap delete is disabled",
				"set GSC_ENABLE_WRITE=true and GSC_ALLOW_DESTRUCTIVE=true to allow delete",
			)), nil, nil
		}
		if !flags.AllowDestructive {
			return toolError(gscclient.NewError(
				gscclient.ErrWriteDisabled,
				"sitemap delete requires the destructive flag",
				"set GSC_ALLOW_DESTRUCTIVE=true (in addition to GSC_ENABLE_WRITE=true) to allow delete",
			)), nil, nil
		}
	}

	var resolvedSiteURL string
	var out manageSitemapsOutput

	_, err := gscclient.WithResolvedSiteURL(ctx, client, input.SiteURL, func(ctx context.Context, resolved string) (struct{}, error) {
		resolvedSiteURL = resolved
		switch action {
		case "list":
			resp, err := client.ListSitemaps(ctx, resolved, input.SitemapIndex)
			if err != nil {
				return struct{}{}, err
			}
			out.Sitemaps = mapSitemaps(resp.Sitemap)
		case "get":
			sm, err := client.GetSitemap(ctx, resolved, input.Feedpath)
			if err != nil {
				return struct{}{}, err
			}
			info := mapSitemap(sm)
			out.Sitemap = &info
		case "submit":
			if err := client.SubmitSitemap(ctx, resolved, input.Feedpath); err != nil {
				return struct{}{}, err
			}
			out.Message = "sitemap submitted"
		case "delete":
			if err := client.DeleteSitemap(ctx, resolved, input.Feedpath); err != nil {
				return struct{}{}, err
			}
			out.Message = "sitemap deleted"
		}
		return struct{}{}, nil
	})
	if err != nil {
		return toolError(gscclient.MapGoogleAPIError(err)), nil, nil
	}

	out.QueriedAt = nowRFC3339()
	out.SiteURL = resolvedSiteURL
	out.Action = action
	return toolResult(out), nil, nil
}

func mapSitemaps(in []*searchconsole.WmxSitemap) []sitemapInfo {
	out := make([]sitemapInfo, 0, len(in))
	for _, sm := range in {
		if sm == nil {
			continue
		}
		out = append(out, mapSitemap(sm))
	}
	return out
}

func mapSitemap(sm *searchconsole.WmxSitemap) sitemapInfo {
	info := sitemapInfo{
		Path:            sm.Path,
		LastSubmitted:   sm.LastSubmitted,
		LastDownloaded:  sm.LastDownloaded,
		IsPending:       sm.IsPending,
		IsSitemapsIndex: sm.IsSitemapsIndex,
		Type:            sm.Type,
		Warnings:        sm.Warnings,
		Errors:          sm.Errors,
	}
	if len(sm.Contents) > 0 {
		info.Contents = make([]sitemapContentInfo, 0, len(sm.Contents))
		for _, c := range sm.Contents {
			if c == nil {
				continue
			}
			info.Contents = append(info.Contents, sitemapContentInfo{
				Type:      c.Type,
				Submitted: c.Submitted,
			})
		}
	}
	return info
}
