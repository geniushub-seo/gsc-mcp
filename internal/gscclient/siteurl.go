package gscclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/searchconsole/v1"
)

// SiteLister is the minimal interface required by ResolveSiteURL to look up
// accessible Search Console properties.
type SiteLister interface {
	ListSites(ctx context.Context) (*searchconsole.SitesListResponse, error)
}

// NormalizeSiteURL converts a user-supplied site reference into its canonical
// GSC format according to SPEC.md §3.0.
//
// Rules:
//   - "sc-domain:..." is returned unchanged.
//   - Bare domain (e.g. "example.com") becomes "sc-domain:example.com".
//   - "https://example.com" (root, no trailing slash) becomes "sc-domain:example.com".
//   - "https://example.com/" (root with trailing slash) stays a URL-prefix property.
//   - "https://example.com/blog" becomes "https://example.com/blog/".
//
// www. prefixes and ports are stripped when deriving the apex domain.
func NormalizeSiteURL(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return input
	}

	if len(input) >= len("sc-domain:") && strings.EqualFold(input[:len("sc-domain:")], "sc-domain:") {
		return "sc-domain:" + strings.ToLower(input[len("sc-domain:"):])
	}

	if strings.Contains(input, "://") {
		u, err := url.Parse(input)
		if err == nil {
			hasNonRootPath := u.Path != "" && u.Path != "/"
			hasTrailingSlash := strings.HasSuffix(input, "/")

			if hasNonRootPath || hasTrailingSlash {
				if !hasTrailingSlash {
					return input + "/"
				}
				return input
			}
			return "sc-domain:" + extractApexDomain(u.Host)
		}
	}

	// Bare domain (no scheme): strip any path before deriving the apex, because
	// sc-domain: properties never contain a path.
	if idx := strings.Index(input, "/"); idx != -1 {
		input = input[:idx]
	}
	return "sc-domain:" + extractApexDomain(input)
}

// DroppedPathNote reports, for a bare domain that carried a path, that the path
// was discarded during normalization. A bare domain always becomes an
// sc-domain: property, which covers the whole domain — so "example.com/blog"
// returns whole-property numbers, not blog numbers. Without this note the scope
// change is invisible in the output and the caller reports site-wide figures as
// section figures. Inputs with a scheme keep their path (they normalize to a
// URL-prefix property) and produce no note.
func DroppedPathNote(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || strings.HasPrefix(trimmed, "sc-domain:") || strings.Contains(trimmed, "://") {
		return ""
	}
	idx := strings.Index(trimmed, "/")
	if idx == -1 || idx == len(trimmed)-1 {
		return ""
	}
	return fmt.Sprintf(
		"site_url %q is a bare domain with a path; the path was discarded and the query covers the whole property %q. "+
			"To query a section, pass a full URL-prefix property such as \"https://%s\" instead",
		trimmed, NormalizeSiteURL(trimmed), trimmed)
}

// ResolveSiteURL resolves a normalized site URL against the accessible GSC
// properties. It is used as a fallback after the normalized form fails with a
// 403.
//
// Resolution preference among candidates that are not equal to exclude:
// sc-domain property first, then URL-prefix property for the same apex domain.
// exclude is the URL that already returned 403; both match loops skip it so
// rescue cannot re-select the failed property. If no match is found the error
// message lists all accessible properties.
func ResolveSiteURL(ctx context.Context, lister SiteLister, input, exclude string) (string, error) {
	sites, err := lister.ListSites(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving site URL: listing accessible properties: %w", err)
	}

	apex := extractApexFromInput(input)

	for _, s := range sites.SiteEntry {
		if exclude != "" && strings.EqualFold(s.SiteUrl, exclude) {
			continue
		}
		if strings.EqualFold(s.SiteUrl, "sc-domain:"+apex) {
			return s.SiteUrl, nil
		}
	}

	for _, s := range sites.SiteEntry {
		if exclude != "" && strings.EqualFold(s.SiteUrl, exclude) {
			continue
		}
		if !strings.HasPrefix(s.SiteUrl, "http") {
			continue
		}
		u, parseErr := url.Parse(s.SiteUrl)
		if parseErr != nil {
			continue
		}
		if extractApexDomain(u.Host) == apex {
			return s.SiteUrl, nil
		}
	}

	accessible := make([]string, len(sites.SiteEntry))
	for i, s := range sites.SiteEntry {
		accessible[i] = s.SiteUrl
	}
	if exclude != "" && siteURLMatchesApex(exclude, apex) {
		return "", NewError(
			ErrPermissionDenied,
			fmt.Sprintf("a matching GSC property was found for %q, but access to it was denied; accessible properties: %v", input, accessible),
			"ask the property owner to grant this credential access to the matching Search Console property",
		)
	}
	return "", NewError(
		ErrPermissionDenied,
		fmt.Sprintf("no matching GSC property found for %q; accessible properties: %v", input, accessible),
		"use list_sites to see accessible properties or ask the property owner to grant this credential access",
	)
}

func siteURLMatchesApex(siteURL, apex string) bool {
	if strings.EqualFold(siteURL, "sc-domain:"+apex) {
		return true
	}
	if !strings.HasPrefix(strings.ToLower(siteURL), "http") {
		return false
	}
	u, err := url.Parse(siteURL)
	return err == nil && strings.EqualFold(extractApexDomain(u.Host), apex)
}

// WithResolvedSiteURL wraps a site-scoped API call. It first uses the normalized
// site URL; if the call returns a 403, it asks ResolveSiteURL to find an
// accessible property for the same apex (excluding the URL that just failed)
// and retries exactly once. Any other error, or a 403 that cannot be resolved,
// is returned unchanged.
func WithResolvedSiteURL[T any](ctx context.Context, lister SiteLister, siteURL string, call func(ctx context.Context, resolvedSiteURL string) (T, error)) (T, error) {
	var zero T

	resolved := NormalizeSiteURL(siteURL)
	result, err := call(ctx, resolved)
	if err == nil {
		return result, nil
	}

	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr.Code != 403 {
		return zero, err
	}

	retryURL, resolveErr := ResolveSiteURL(ctx, lister, siteURL, resolved)
	if resolveErr != nil {
		return zero, resolveErr
	}

	return call(ctx, retryURL)
}

func extractApexFromInput(input string) string {
	input = strings.TrimSpace(input)
	if len(input) >= len("sc-domain:") && strings.EqualFold(input[:len("sc-domain:")], "sc-domain:") {
		return strings.ToLower(input[len("sc-domain:"):])
	}
	if strings.Contains(input, "://") {
		if u, err := url.Parse(input); err == nil {
			return extractApexDomain(u.Host)
		}
	}
	if idx := strings.Index(input, "/"); idx != -1 {
		input = input[:idx]
	}
	return extractApexDomain(input)
}

func extractApexDomain(host string) string {
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	host = strings.ToLower(host)
	return strings.TrimPrefix(host, "www.")
}
