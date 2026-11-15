package gscclient

import (
	"context"
	"encoding/json"
	"errors"
	"os"
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
		{"WWW.example.com", "sc-domain:example.com"},
		{"https://WWW.example.com", "sc-domain:example.com"},
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

// recordingCall returns a site-scoped call that fails with err and records
// every site URL it was invoked with.
func recordingCall(err error) (func(context.Context, string) (string, error), *[]string) {
	var calls []string
	return func(_ context.Context, siteURL string) (string, error) {
		calls = append(calls, siteURL)
		return "", err
	}, &calls
}

// TestWithSiteURL_403DoesNotSwitchProperty is the core guarantee: a 403 never
// sends the call to another property, even when an accessible property for the
// same domain exists. The error lists that property instead.
func TestWithSiteURL_403DoesNotSwitchProperty(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{sites: []*searchconsole.WmxSite{
		{SiteUrl: "sc-domain:example.com", PermissionLevel: "siteUnverifiedUser"},
		{SiteUrl: "https://example.com/", PermissionLevel: "siteOwner"},
	}}
	call, calls := recordingCall(&googleapi.Error{Code: 403, Message: "forbidden"})

	_, err := WithSiteURL(context.Background(), lister, "example.com", call)

	if len(*calls) != 1 || (*calls)[0] != "sc-domain:example.com" {
		t.Fatalf("calls = %v, want exactly one call to sc-domain:example.com", *calls)
	}
	var cerr Error
	if !errors.As(err, &cerr) || cerr.Code != ErrPermissionDenied {
		t.Fatalf("err = %v, want permission_denied", err)
	}
	for _, want := range []string{"https://example.com/ (siteOwner)", "sc-domain:example.com (siteUnverifiedUser)"} {
		if !strings.Contains(cerr.Message, want) {
			t.Errorf("message %q does not list %q", cerr.Message, want)
		}
	}
}

// TestWithSiteURL_ExplicitPrefixIsNotWidened covers the read-side harm: a
// trailing-slash URL-prefix input must not be widened to the whole domain.
func TestWithSiteURL_ExplicitPrefixIsNotWidened(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{sites: []*searchconsole.WmxSite{
		{SiteUrl: "sc-domain:example.com", PermissionLevel: "siteOwner"},
		{SiteUrl: "https://example.com/shop/", PermissionLevel: "siteOwner"},
	}}
	call, calls := recordingCall(&googleapi.Error{Code: 403, Message: "forbidden"})

	_, err := WithSiteURL(context.Background(), lister, "https://example.com/blog/", call)

	if len(*calls) != 1 || (*calls)[0] != "https://example.com/blog/" {
		t.Fatalf("calls = %v, want exactly one call to https://example.com/blog/", *calls)
	}
	if err == nil {
		t.Fatal("expected an error")
	}
}

// TestWithSiteURL_QuotaProject403PassesThrough: a 403 about a missing quota
// project is not a property problem, so listing properties would mislead.
func TestWithSiteURL_QuotaProject403PassesThrough(t *testing.T) {
	t.Parallel()
	lister := &fakeSiteLister{err: errors.New("lister must not be called")}
	orig := &googleapi.Error{Code: 403, Message: "Your application is authenticating by using local Application Default Credentials. The searchconsole.googleapis.com API requires a quota project, which is not set by default."}
	call, calls := recordingCall(orig)

	_, err := WithSiteURL(context.Background(), lister, "example.com", call)

	var gerr *googleapi.Error
	if len(*calls) != 1 || !errors.As(err, &gerr) || gerr != orig {
		t.Fatalf("calls = %v, err = %v; want one call and the original error", *calls, err)
	}
}

func TestWithSiteURL_ListFailureReturnsOriginal403(t *testing.T) {
	t.Parallel()
	orig := &googleapi.Error{Code: 403, Message: "forbidden"}
	call, _ := recordingCall(orig)

	_, err := WithSiteURL(context.Background(), &fakeSiteLister{err: errors.New("list failed")}, "example.com", call)

	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr != orig {
		t.Fatalf("err = %v, want the original 403", err)
	}
}

func TestWithSiteURL_DoesNotRetryNon403(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("some other error")
	call, calls := recordingCall(wantErr)

	_, err := WithSiteURL(context.Background(), &fakeSiteLister{}, "example.com", call)

	if !errors.Is(err, wantErr) || len(*calls) != 1 {
		t.Fatalf("calls = %v, err = %v; want one call and the original error", *calls, err)
	}
}

// TestNormalizeSiteURL_IdempotentOnListedProperties guards the premise the
// no-switch design rests on: a site_url copied verbatim from list_sites must
// reach the API unchanged, or a caller following the 403 advice can never hit
// the property it picked.
func TestNormalizeSiteURL_IdempotentOnListedProperties(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/sites_list.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var resp searchconsole.SitesListResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(resp.SiteEntry) == 0 {
		t.Fatal("fixture has no siteEntry")
	}
	extra := []string{"http://example.org/", "https://www.example.org/", "https://example.org/blog/", "https://example.org:8080/"}
	for _, e := range resp.SiteEntry {
		extra = append(extra, e.SiteUrl)
	}
	for _, siteURL := range extra {
		if got := NormalizeSiteURL(siteURL); got != siteURL {
			t.Errorf("NormalizeSiteURL(%q) = %q; a listed site_url must pass through unchanged", siteURL, got)
		}
	}
}
