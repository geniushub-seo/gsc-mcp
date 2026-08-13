// Package gscclient wraps the generated Google Search Console API client.
// It keeps transport-specific options (HTTP endpoint, auth override, custom
// HTTP client) injectable so tests can run against httptest servers without
// touching global state.
package gscclient

import (
	"context"
	"fmt"
	"time"

	"github.com/geniushub-seo/gsc-mcp/internal/config"
	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"
)

// Client is a thin wrapper around searchconsole.Service.
type Client struct {
	svc     *searchconsole.Service
	timeout time.Duration
	// sleep is injectable for tests. In production it is nil and
	// sleepWithContext falls back to time.NewTimer.
	sleep func(time.Duration) <-chan time.Time
}

// New creates a Client from config. Optional ClientOptions can override
// credentials, endpoint, or HTTP client for testing.
func New(ctx context.Context, cfg config.Config, opts ...option.ClientOption) (*Client, error) {
	baseOpts, err := credentialOptions(cfg)
	if err != nil {
		return nil, err
	}
	baseOpts = append(baseOpts, opts...)

	svc, err := searchconsole.NewService(ctx, baseOpts...)
	if err != nil {
		return nil, fmt.Errorf("create searchconsole service: %w", err)
	}

	return &Client{svc: svc, timeout: cfg.RequestTimeout}, nil
}

// credentialOptions builds auth options from config. Exported for tests via
// the package-level helper below.
func credentialOptions(cfg config.Config) ([]option.ClientOption, error) {
	var opts []option.ClientOption

	if len(cfg.CredentialsJSON) == 0 {
		return opts, nil
	}

	var credType option.CredentialsType
	switch cfg.CredType {
	case config.CredTypeServiceAccount:
		credType = option.ServiceAccount
		// Service account scopes are applied at token mint time.
		opts = append(opts, option.WithScopes(cfg.Scopes()...))
	case config.CredTypeAuthorizedUser:
		credType = option.AuthorizedUser
		// ADC scopes are fixed at gcloud login; WithScopes cannot expand them.
	default:
		return nil, fmt.Errorf("unsupported credential type %q; supported: %q, %q",
			cfg.CredType, config.CredTypeServiceAccount, config.CredTypeAuthorizedUser)
	}

	opts = append(opts, option.WithAuthCredentialsJSON(credType, cfg.CredentialsJSON))
	if cfg.QuotaProjectID != "" {
		opts = append(opts, option.WithQuotaProject(cfg.QuotaProjectID))
	}
	return opts, nil
}

// CredentialOptionsForTest exposes credentialOptions for package tests in tools/cmd.
func CredentialOptionsForTest(cfg config.Config) ([]option.ClientOption, error) {
	return credentialOptions(cfg)
}

// Service returns the underlying searchconsole.Service for tool handlers.
func (c *Client) Service() *searchconsole.Service {
	return c.svc
}

// ListSites returns the list of GSC properties accessible to the credential,
// honouring the configured request timeout and retry policy.
func (c *Client) ListSites(ctx context.Context) (*searchconsole.SitesListResponse, error) {
	return doWithRetry(ctx, c.sleep, func(ctx context.Context) (*searchconsole.SitesListResponse, error) {
		ctx, cancel := c.withTimeout(ctx)
		defer cancel()
		return c.svc.Sites.List().Context(ctx).Do()
	})
}

// GetSite returns a single GSC property, honouring timeout and retry.
func (c *Client) GetSite(ctx context.Context, siteURL string) (*searchconsole.WmxSite, error) {
	return doWithRetry(ctx, c.sleep, func(ctx context.Context) (*searchconsole.WmxSite, error) {
		ctx, cancel := c.withTimeout(ctx)
		defer cancel()
		return c.svc.Sites.Get(siteURL).Context(ctx).Do()
	})
}

// QuerySearchAnalytics runs a searchAnalytics/query call with timeout and retry.
func (c *Client) QuerySearchAnalytics(ctx context.Context, siteURL string, req *searchconsole.SearchAnalyticsQueryRequest) (*searchconsole.SearchAnalyticsQueryResponse, error) {
	return doWithRetry(ctx, c.sleep, func(ctx context.Context) (*searchconsole.SearchAnalyticsQueryResponse, error) {
		ctx, cancel := c.withTimeout(ctx)
		defer cancel()
		return c.svc.Searchanalytics.Query(siteURL, req).Context(ctx).Do()
	})
}

// InspectURL runs urlInspection.index.inspect for one URL with timeout and retry.
func (c *Client) InspectURL(ctx context.Context, siteURL, inspectionURL, languageCode string) (*searchconsole.InspectUrlIndexResponse, error) {
	return doWithRetry(ctx, c.sleep, func(ctx context.Context) (*searchconsole.InspectUrlIndexResponse, error) {
		ctx, cancel := c.withTimeout(ctx)
		defer cancel()
		req := &searchconsole.InspectUrlIndexRequest{
			SiteUrl:       siteURL,
			InspectionUrl: inspectionURL,
			LanguageCode:  languageCode,
		}
		return c.svc.UrlInspection.Index.Inspect(req).Context(ctx).Do()
	})
}

// ListSitemaps lists sitemaps for a property. sitemapIndex is optional.
func (c *Client) ListSitemaps(ctx context.Context, siteURL, sitemapIndex string) (*searchconsole.SitemapsListResponse, error) {
	return doWithRetry(ctx, c.sleep, func(ctx context.Context) (*searchconsole.SitemapsListResponse, error) {
		ctx, cancel := c.withTimeout(ctx)
		defer cancel()
		call := c.svc.Sitemaps.List(siteURL).Context(ctx)
		if sitemapIndex != "" {
			call = call.SitemapIndex(sitemapIndex)
		}
		return call.Do()
	})
}

// GetSitemap returns one sitemap by feedpath.
func (c *Client) GetSitemap(ctx context.Context, siteURL, feedpath string) (*searchconsole.WmxSitemap, error) {
	return doWithRetry(ctx, c.sleep, func(ctx context.Context) (*searchconsole.WmxSitemap, error) {
		ctx, cancel := c.withTimeout(ctx)
		defer cancel()
		return c.svc.Sitemaps.Get(siteURL, feedpath).Context(ctx).Do()
	})
}

// SubmitSitemap submits a sitemap feedpath.
func (c *Client) SubmitSitemap(ctx context.Context, siteURL, feedpath string) error {
	_, err := doWithRetry(ctx, c.sleep, func(ctx context.Context) (struct{}, error) {
		ctx, cancel := c.withTimeout(ctx)
		defer cancel()
		return struct{}{}, c.svc.Sitemaps.Submit(siteURL, feedpath).Context(ctx).Do()
	})
	return err
}

// DeleteSitemap deletes a sitemap feedpath.
func (c *Client) DeleteSitemap(ctx context.Context, siteURL, feedpath string) error {
	_, err := doWithRetry(ctx, c.sleep, func(ctx context.Context) (struct{}, error) {
		ctx, cancel := c.withTimeout(ctx)
		defer cancel()
		return struct{}{}, c.svc.Sitemaps.Delete(siteURL, feedpath).Context(ctx).Do()
	})
	return err
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.timeout)
}
