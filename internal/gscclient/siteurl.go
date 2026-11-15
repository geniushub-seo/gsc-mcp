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

// SiteLister is the minimal interface WithSiteURL needs to list accessible
// Search Console properties for a permission error.
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

// WithSiteURL wraps a site-scoped API call. It normalizes siteURL and calls
// exactly once against that property. It never retries against a different
// property: which property a call reads from or writes to is the caller's
// decision (SPEC.md §3.0), and a silent switch can turn a /blog/ query into a
// whole-domain one or land a sitemap delete on a property nobody named.
//
// On a property-permission 403 the error lists every accessible property with
// its permission level, so the caller can choose one explicitly. A 403 about a
// missing quota project, any other error, or a failed sites.list returns the
// original error unchanged.
func WithSiteURL[T any](ctx context.Context, lister SiteLister, siteURL string, call func(ctx context.Context, siteURL string) (T, error)) (T, error) {
	var zero T

	normalized := NormalizeSiteURL(siteURL)
	result, err := call(ctx, normalized)
	if err == nil {
		return result, nil
	}

	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr.Code != 403 || matchesAny(gerr.Message, quotaProjectMarker) {
		return zero, err
	}

	sites, listErr := lister.ListSites(ctx)
	if listErr != nil {
		return zero, err
	}
	accessible := make([]string, len(sites.SiteEntry))
	for i, s := range sites.SiteEntry {
		accessible[i] = fmt.Sprintf("%s (%s)", s.SiteUrl, s.PermissionLevel)
	}
	return zero, NewError(
		ErrPermissionDenied,
		fmt.Sprintf("access to %q was denied; no other property was tried. accessible properties: %v", normalized, accessible),
		"pick the property for this site from the list (skip any marked siteUnverifiedUser) and pass its site_url verbatim; "+
			"before a sitemap submit or delete on a property the user did not name, confirm with the user. "+
			"If the site is not listed, ask the property owner to grant this credential access",
	)
}

func extractApexDomain(host string) string {
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	host = strings.ToLower(host)
	return strings.TrimPrefix(host, "www.")
}
