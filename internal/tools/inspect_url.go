package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/geniushub-seo/gsc-mcp/internal/gscclient"
	"google.golang.org/api/searchconsole/v1"
)

// inspectURLInputSchema is written by hand so the urls array has
// "type":"array" instead of the SDK-inferred ["null","array"] union.
var inspectURLInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "site_url": {
      "type": "string",
      "description": "The GSC property that owns the URLs. Supports bare domain, full URL, or canonical GSC format."
    },
    "urls": {
      "type": "array",
      "items": {"type": "string"},
      "description": "One to ten fully qualified URLs to inspect under the property."
    },
    "language_code": {
      "type": "string",
      "description": "Optional IETF BCP-47 language code for translated issue messages. Defaults to zh-TW."
    }
  },
  "required": ["site_url", "urls"]
}`)

// inspectURLInput is the input schema for the inspect_url tool.
type inspectURLInput struct {
	SiteURL      string   `json:"site_url" jsonschema:"The GSC property that owns the URLs. Supports bare domain, full URL, or canonical GSC format."`
	URLs         []string `json:"urls" jsonschema:"One to ten fully qualified URLs to inspect under the property."`
	LanguageCode string   `json:"language_code,omitempty" jsonschema:"Optional IETF BCP-47 language code for translated issue messages. Defaults to zh-TW."`
}

type inspectURLOutput struct {
	QueriedAt      string             `json:"queried_at"`
	SiteURL        string             `json:"site_url"`
	QuotaUsedToday int                `json:"quota_used_today"`
	QuotaWarning   string             `json:"quota_warning,omitempty"`
	Results        []inspectURLResult `json:"results"`
}

type inspectURLResult struct {
	URL                    string             `json:"url"`
	InspectionResultLink   string             `json:"inspection_result_link,omitempty"`
	IndexStatus            *indexStatusResult `json:"index_status,omitempty"`
	MobileUsabilityVerdict string             `json:"mobile_usability_verdict,omitempty"`
	RichResultsVerdict     string             `json:"rich_results_verdict,omitempty"`
	AmpVerdict             string             `json:"amp_verdict,omitempty"`
	Error                  *inspectURLError   `json:"error,omitempty"`
}

// inspectURLError is the per-URL counterpart to a top-level tool error. A
// batch may contain successful and failed inspections, so failures live beside
// the affected URL rather than collapsing into an ambiguous flat string.
type inspectURLError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

type indexStatusResult struct {
	Verdict         string   `json:"verdict,omitempty"`
	CoverageState   string   `json:"coverage_state,omitempty"`
	IndexingState   string   `json:"indexing_state,omitempty"`
	CrawledAs       string   `json:"crawled_as,omitempty"`
	LastCrawlTime   string   `json:"last_crawl_time,omitempty"`
	PageFetchState  string   `json:"page_fetch_state,omitempty"`
	RobotsTxtState  string   `json:"robots_txt_state,omitempty"`
	GoogleCanonical string   `json:"google_canonical,omitempty"`
	UserCanonical   string   `json:"user_canonical,omitempty"`
	ReferringURLs   []string `json:"referring_urls,omitempty"`
	Sitemap         []string `json:"sitemap,omitempty"`
}

func registerInspectURL(srv *mcp.Server, client *gscclient.Client, quota *gscclient.DailyQuota) {
	mcp.AddTool(srv,
		&mcp.Tool{
			Name:        "inspect_url",
			Description: descInspectURL,
			InputSchema: inspectURLInputSchema,
		},
		func(ctx context.Context, _ *mcp.CallToolRequest, input inspectURLInput) (*mcp.CallToolResult, any, error) {
			return inspectURL(ctx, client, quota, input, time.Sleep, 100*time.Millisecond)
		},
	)
}

func inspectURL(ctx context.Context, client *gscclient.Client, quota *gscclient.DailyQuota, input inspectURLInput, sleep func(time.Duration), interval time.Duration) (*mcp.CallToolResult, any, error) { //nolint:unparam // interval is overridden in tests
	if len(input.URLs) < 1 || len(input.URLs) > 10 {
		return toolError(gscclient.NewError(
			gscclient.ErrInvalidInput,
			fmt.Sprintf("urls must contain between 1 and 10 items, got %d", len(input.URLs)),
			"pass 1–10 fully qualified URLs",
		)), nil, nil
	}

	languageCode := input.LanguageCode
	if languageCode == "" {
		languageCode = "zh-TW"
	}

	var resolvedSiteURL string
	var lastWarning string
	results := make([]inspectURLResult, 0, len(input.URLs))
	quotaUsed := 0

	for i, u := range input.URLs {
		if i > 0 {
			sleep(interval)
		}
		if err := ctx.Err(); err != nil {
			return toolError(gscclient.NewError(
				gscclient.ErrUpstreamError,
				err.Error(),
				"retry the request",
			)), nil, nil
		}

		var (
			resp    *searchconsole.InspectUrlIndexResponse
			callErr error
		)
		if resolvedSiteURL != "" {
			resp, callErr = client.InspectURL(ctx, resolvedSiteURL, u, languageCode)
		} else {
			resp, callErr = gscclient.WithResolvedSiteURL(ctx, client, input.SiteURL, func(ctx context.Context, resolved string) (*searchconsole.InspectUrlIndexResponse, error) {
				resolvedSiteURL = resolved
				return client.InspectURL(ctx, resolved, u, languageCode)
			})
			if callErr != nil {
				mapped := gscclient.MapGoogleAPIError(callErr)
				// Site-level failures on the first URL should fail the whole tool call
				// so the LLM sees accessible properties in the message.
				if mapped.Code == gscclient.ErrPermissionDenied || mapped.Code == gscclient.ErrAuthFailed {
					return toolError(mapped), nil, nil
				}
			}
		}

		count, warning := quota.Inc(resolvedSiteURL)
		quotaUsed = count
		if warning != "" {
			lastWarning = warning
		}

		if callErr != nil {
			mapped := gscclient.MapGoogleAPIError(callErr)
			results = append(results, inspectURLResult{
				URL: u,
				Error: &inspectURLError{
					Code:       string(mapped.Code),
					Message:    mapped.Message,
					Suggestion: mapped.Suggestion,
				},
			})
			continue
		}
		results = append(results, mapInspectResult(u, resp))
	}

	out := inspectURLOutput{
		QueriedAt:      nowRFC3339(),
		SiteURL:        resolvedSiteURL,
		QuotaUsedToday: quotaUsed,
		QuotaWarning:   lastWarning,
		Results:        results,
	}
	return toolResult(out), nil, nil
}

func mapInspectResult(url string, resp *searchconsole.InspectUrlIndexResponse) inspectURLResult {
	out := inspectURLResult{URL: url}
	if resp == nil || resp.InspectionResult == nil {
		return out
	}
	ir := resp.InspectionResult
	out.InspectionResultLink = ir.InspectionResultLink

	if ir.IndexStatusResult != nil {
		isr := ir.IndexStatusResult
		out.IndexStatus = &indexStatusResult{
			Verdict:         isr.Verdict,
			CoverageState:   isr.CoverageState,
			IndexingState:   isr.IndexingState,
			CrawledAs:       isr.CrawledAs,
			LastCrawlTime:   isr.LastCrawlTime,
			PageFetchState:  isr.PageFetchState,
			RobotsTxtState:  isr.RobotsTxtState,
			GoogleCanonical: isr.GoogleCanonical,
			UserCanonical:   isr.UserCanonical,
			ReferringURLs:   isr.ReferringUrls,
			Sitemap:         isr.Sitemap,
		}
	}

	// Optional sections are pointers; Google omits them when not applicable.
	if ir.MobileUsabilityResult != nil {
		out.MobileUsabilityVerdict = ir.MobileUsabilityResult.Verdict
	}
	if ir.RichResultsResult != nil {
		out.RichResultsVerdict = ir.RichResultsResult.Verdict
		// Intentionally omit DetectedItems — too large for LLM context.
	}
	if ir.AmpResult != nil {
		out.AmpVerdict = ir.AmpResult.Verdict
	}
	return out
}
