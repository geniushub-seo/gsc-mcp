package gscclient

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/api/googleapi"
)

func TestMapGoogleAPIError_AllCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want ErrorCode
	}{
		{"400", &googleapi.Error{Code: 400, Message: "bad"}, ErrInvalidInput},
		{"401", &googleapi.Error{Code: 401, Message: "auth"}, ErrAuthFailed},
		{"403", &googleapi.Error{Code: 403, Message: "denied"}, ErrPermissionDenied},
		{"404", &googleapi.Error{Code: 404, Message: "missing"}, ErrNotFound},
		{"429", &googleapi.Error{Code: 429, Message: "slow down"}, ErrQuotaExceeded},
		{"500", &googleapi.Error{Code: 500, Message: "boom"}, ErrUpstreamError},
		{"plain", errors.New("network down"), ErrUpstreamError},
		{"client", NewError(ErrWriteDisabled, "off", "set flag"), ErrWriteDisabled},
		{"permission client", NewError(ErrPermissionDenied, "no match", "list_sites"), ErrPermissionDenied},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapGoogleAPIError(tc.err)
			if got.Code != tc.want {
				t.Fatalf("code = %q, want %q", got.Code, tc.want)
			}
		})
	}
}

// TestMapGoogleAPIError_QuotaProject403 covers the first error a new ADC user
// hits: the credentials are fine and the property is accessible, but no quota
// project is associated, so Search Console answers 403. The generic 403
// suggestion ("ask the property owner to add the service account email") sends
// an ADC user to do something that cannot possibly help — they have no service
// account. Message body reproduced locally by removing quota_project_id from
// ~/.config/gcloud/application_default_credentials.json and calling list_sites.
func TestMapGoogleAPIError_QuotaProject403(t *testing.T) {
	t.Parallel()
	const body = "Your application is authenticating by using local Application Default Credentials. " +
		"The searchconsole.googleapis.com API requires a quota project, which is not set by default."

	got := MapGoogleAPIError(&googleapi.Error{Code: 403, Message: body})

	if got.Code != ErrPermissionDenied {
		t.Fatalf("code = %q, want %q", got.Code, ErrPermissionDenied)
	}
	if !strings.Contains(got.Suggestion, "set-quota-project") {
		t.Errorf("suggestion must name the set-quota-project command, got %q", got.Suggestion)
	}
	if strings.Contains(got.Suggestion, "property owner") {
		t.Errorf("suggestion still tells an ADC user to contact the property owner: %q", got.Suggestion)
	}
}

// TestMapGoogleAPIError_Plain403KeepsPropertySuggestion is the counterpart: a
// 403 that is genuinely about property access must keep the original advice, so
// the quota-project branch cannot swallow the common case.
func TestMapGoogleAPIError_Plain403KeepsPropertySuggestion(t *testing.T) {
	t.Parallel()
	got := MapGoogleAPIError(&googleapi.Error{Code: 403, Message: "User does not have sufficient permission for site"})

	if got.Code != ErrPermissionDenied {
		t.Fatalf("code = %q, want %q", got.Code, ErrPermissionDenied)
	}
	if !strings.Contains(got.Suggestion, "property owner") {
		t.Errorf("suggestion = %q, want the property-owner advice", got.Suggestion)
	}
}

// TestMapGoogleAPIError_TokenExchangeRejected covers credential rejections that
// happen during the OAuth token exchange, before any Search Console call. They
// are not *googleapi.Error, so they used to fall through to upstream_error with
// "retry the request" — advice that can never succeed, so an agent retries
// forever. Bodies are the oauth2 package's wording for a deleted service
// account and for an expired ADC refresh token.
func TestMapGoogleAPIError_TokenExchangeRejected(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"deleted service account": `oauth2: cannot fetch token: 400 Bad Request` + "\n" +
			`Response: {"error":"invalid_grant","error_description":"Invalid grant: account not found"}`,
		"revoked ADC refresh token": `oauth2: cannot fetch token: 400 Bad Request` + "\n" +
			`Response: {"error":"invalid_grant","error_description":"Token has been expired or revoked."}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got := MapGoogleAPIError(errors.New(body))

			if got.Code != ErrAuthFailed {
				t.Fatalf("code = %q, want %q", got.Code, ErrAuthFailed)
			}
			if strings.Contains(strings.ToLower(got.Suggestion), "retry the request") {
				t.Errorf("suggestion still tells the caller to retry: %q", got.Suggestion)
			}
			if !strings.Contains(got.Suggestion, "will not help") {
				t.Errorf("suggestion must say retrying cannot help, got %q", got.Suggestion)
			}
		})
	}
}

// TestMapGoogleAPIError_PlainErrorStaysUpstream guards the other direction: a
// transport failure with no auth marker must still map to upstream_error and
// still advise a retry, because that one genuinely can succeed.
func TestMapGoogleAPIError_PlainErrorStaysUpstream(t *testing.T) {
	t.Parallel()
	got := MapGoogleAPIError(errors.New("dial tcp: lookup searchconsole.googleapis.com: no such host"))

	if got.Code != ErrUpstreamError {
		t.Fatalf("code = %q, want %q", got.Code, ErrUpstreamError)
	}
	if !strings.Contains(got.Suggestion, "retry the request") {
		t.Errorf("suggestion = %q, want the retry advice", got.Suggestion)
	}
}

func TestMapGoogleAPIError_TruncatesMessage(t *testing.T) {
	t.Parallel()
	long := make([]byte, 400)
	for i := range long {
		long[i] = 'x'
	}
	got := MapGoogleAPIError(&googleapi.Error{Code: 500, Message: string(long)})
	if len(got.Message) > 303 { // 300 + "..."
		t.Fatalf("message length %d exceeds limit", len(got.Message))
	}
}

func TestNewError_WriteDisabled(t *testing.T) {
	t.Parallel()
	err := NewError(ErrWriteDisabled, "disabled", "set GSC_ENABLE_WRITE=true")
	if err.Code != ErrWriteDisabled {
		t.Fatalf("code = %q", err.Code)
	}
	if err.Error() == "" {
		t.Fatal("Error() empty")
	}
}
