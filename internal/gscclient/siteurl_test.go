package gscclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/searchconsole/v1"
)

type fakeSiteLister struct {
	sites []*searchconsole.WmxSite
	err   error
}

func (f *fakeSiteLister) ListSites(_ context.Context) (*searchconsole.SitesListResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &searchconsole.SitesListResponse{SiteEntry: f.sites}, nil
}

func TestNormalizeSiteURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"sc-domain:example.com", "sc-domain:example.com"},
		{"example.com", "sc-domain:example.com"},
		{"https://example.com", "sc-domain:example.com"},
		{"https://example.com/", "https://example.com/"},
		{"https://example.com/blog", "https://example.com/blog/"},
		{"https://www.example.com", "sc-domain:example.com"},
		{"https://example.com:8080/", "https://example.com:8080/"},
		{"example.com:8080", "sc-domain:example.com"},
		{"", ""},
	}

	for _, tc := range cases {
		tc := tc
		got := NormalizeSiteURL(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeSiteURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestResolveSiteURL_PrefersDomainProperty(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{
		sites: []*searchconsole.WmxSite{
			{SiteUrl: "https://example.com/", PermissionLevel: "SITE_OWNER"},
			{SiteUrl: "sc-domain:example.com", PermissionLevel: "SITE_OWNER"},
		},
	}

	resolved, err := ResolveSiteURL(context.Background(), lister, "example.com", "")
	if err != nil {
		t.Fatalf("ResolveSiteURL error: %v", err)
	}
	if resolved != "sc-domain:example.com" {
		t.Fatalf("expected sc-domain property, got %q", resolved)
	}
}

func TestResolveSiteURL_FallsBackToURLPrefix(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{
		sites: []*searchconsole.WmxSite{
			{SiteUrl: "https://example.com/", PermissionLevel: "SITE_OWNER"},
		},
	}

	resolved, err := ResolveSiteURL(context.Background(), lister, "example.com", "")
	if err != nil {
		t.Fatalf("ResolveSiteURL error: %v", err)
	}
	if resolved != "https://example.com/" {
		t.Fatalf("expected URL-prefix property, got %q", resolved)
	}
}

func TestResolveSiteURL_NotFoundListsAccessible(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{
		sites: []*searchconsole.WmxSite{
			{SiteUrl: "sc-domain:other.com", PermissionLevel: "SITE_OWNER"},
		},
	}

	_, err := ResolveSiteURL(context.Background(), lister, "example.com", "")
	if err == nil {
		t.Fatal("expected error when no property found")
	}
	if !strings.Contains(err.Error(), "sc-domain:other.com") {
		t.Fatalf("error should list accessible properties, got %q", err.Error())
	}
}

// Same apex with unauthorized sc-domain: and authorized URL-prefix — the case
// that previously re-selected the failed sc-domain and looped on 403.
func TestResolveSiteURL_ExcludeFailedDomainPrefersURLPrefix(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{
		sites: []*searchconsole.WmxSite{
			{SiteUrl: "sc-domain:example.net", PermissionLevel: "SITE_UNVERIFIED_USER"},
			{SiteUrl: "https://example.net/", PermissionLevel: "SITE_OWNER"},
		},
	}

	resolved, err := ResolveSiteURL(context.Background(), lister, "example.net", "sc-domain:example.net")
	if err != nil {
		t.Fatalf("ResolveSiteURL error: %v", err)
	}
	if resolved != "https://example.net/" {
		t.Fatalf("expected URL-prefix after excluding failed sc-domain, got %q", resolved)
	}
}

func TestWithResolvedSiteURL_SkipsFailedScDomainOnSameApex(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{
		sites: []*searchconsole.WmxSite{
			{SiteUrl: "sc-domain:example.net", PermissionLevel: "SITE_UNVERIFIED_USER"},
			{SiteUrl: "https://example.net/", PermissionLevel: "SITE_OWNER"},
		},
	}

	var tried []string
	result, err := WithResolvedSiteURL(context.Background(), lister, "example.net", func(_ context.Context, resolved string) (string, error) {
		tried = append(tried, resolved)
		if resolved == "sc-domain:example.net" {
			return "", &googleapi.Error{Code: 403, Message: "forbidden"}
		}
		return resolved, nil
	})
	if err != nil {
		t.Fatalf("WithResolvedSiteURL error: %v", err)
	}
	if result != "https://example.net/" {
		t.Fatalf("expected URL-prefix success, got %q", result)
	}
	if len(tried) != 2 {
		t.Fatalf("expected 2 attempts, got %v", tried)
	}
	if tried[0] != "sc-domain:example.net" || tried[1] != "https://example.net/" {
		t.Fatalf("attempt order = %v, want sc-domain then URL-prefix", tried)
	}
}

func TestWithResolvedSiteURL_RetriesOn403(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{
		sites: []*searchconsole.WmxSite{
			{SiteUrl: "https://example.com/", PermissionLevel: "SITE_OWNER"},
		},
	}

	calls := 0
	result, err := WithResolvedSiteURL(context.Background(), lister, "example.com", func(_ context.Context, resolved string) (string, error) {
		calls++
		if resolved == "sc-domain:example.com" {
			return "", &googleapi.Error{Code: 403, Message: "forbidden"}
		}
		return resolved, nil
	})
	if err != nil {
		t.Fatalf("WithResolvedSiteURL error: %v", err)
	}
	if result != "https://example.com/" {
		t.Fatalf("expected resolved URL-prefix, got %q", result)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestNormalizeSiteURL_BareDomainStripsPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"example.com/blog", "sc-domain:example.com"},
		{"www.example.com/shop", "sc-domain:example.com"},
		{"example.com:8080/blog", "sc-domain:example.com"},
	}

	for _, tc := range cases {
		tc := tc
		got := NormalizeSiteURL(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeSiteURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestWithResolvedSiteURL_UsesOriginalInputForResolve(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{
		sites: []*searchconsole.WmxSite{
			{SiteUrl: "sc-domain:example.com", PermissionLevel: "SITE_OWNER"},
		},
	}

	calls := 0
	result, err := WithResolvedSiteURL(context.Background(), lister, "https://example.com/blog", func(_ context.Context, resolved string) (string, error) {
		calls++
		if resolved == "https://example.com/blog/" {
			return "", &googleapi.Error{Code: 403, Message: "forbidden"}
		}
		return resolved, nil
	})
	if err != nil {
		t.Fatalf("WithResolvedSiteURL error: %v", err)
	}
	if result != "sc-domain:example.com" {
		t.Fatalf("expected resolved domain property, got %q", result)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestResolveSiteURL_NotFound_ReturnsPermissionDenied(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{
		sites: []*searchconsole.WmxSite{
			{SiteUrl: "sc-domain:other.com", PermissionLevel: "SITE_OWNER"},
		},
	}

	_, err := ResolveSiteURL(context.Background(), lister, "example.com", "")
	if err == nil {
		t.Fatal("expected error when no property found")
	}

	var clientErr Error
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected gscclient.Error, got %T", err)
	}
	if clientErr.Code != ErrPermissionDenied {
		t.Fatalf("expected permission_denied, got %q", clientErr.Code)
	}
	if !strings.Contains(clientErr.Message, "sc-domain:other.com") {
		t.Fatalf("error should list accessible properties, got %q", clientErr.Message)
	}
}

func TestMapGoogleAPIError_PassthroughClientError(t *testing.T) {
	t.Parallel()
	original := NewError(ErrPermissionDenied, "no access", "ask owner")
	got := MapGoogleAPIError(original)
	if got.Code != ErrPermissionDenied {
		t.Fatalf("expected code %q, got %q", ErrPermissionDenied, got.Code)
	}
	if got.Message != original.Message {
		t.Fatalf("expected message %q, got %q", original.Message, got.Message)
	}
}

func TestWithResolvedSiteURL_DoesNotRetryNon403(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{}
	wantErr := errors.New("some other error")
	_, err := WithResolvedSiteURL(context.Background(), lister, "example.com", func(_ context.Context, _ string) (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected original error, got %v", err)
	}
}
