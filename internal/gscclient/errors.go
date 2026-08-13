package gscclient

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/googleapi"
)

// ErrorCode is a fixed, LLM-actionable error code.
type ErrorCode string

// Error codes used across all tool responses.
const (
	ErrInvalidInput     ErrorCode = "invalid_input"
	ErrAuthFailed       ErrorCode = "auth_failed"
	ErrPermissionDenied ErrorCode = "permission_denied"
	ErrNotFound         ErrorCode = "not_found"
	ErrQuotaExceeded    ErrorCode = "quota_exceeded"
	ErrUpstreamError    ErrorCode = "upstream_error"
	ErrWriteDisabled    ErrorCode = "write_disabled"
)

// Error is a structured error for tool responses.
type Error struct {
	Code       ErrorCode
	Message    string
	Suggestion string
}

// Error implements the error interface.
func (e Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewError creates a structured error from a known code and a human-readable
// message. The suggestion is optional.
func NewError(code ErrorCode, message, suggestion string) Error {
	return Error{
		Code:       code,
		Message:    truncate(message, 300),
		Suggestion: suggestion,
	}
}

// Markers that identify a failure as something other than what its HTTP status
// or error type suggests. Compared against a lower-cased error string, so they
// must stay lower case. Every marker here comes from an observed failure; the
// response body that produced it is recorded in the corresponding test.
const quotaProjectMarker = "requires a quota project"

// authFailureMarkers identify credential rejections that happen during the
// OAuth token exchange, before any Search Console call is made. Those never
// arrive as a *googleapi.Error, so without this check they fall through to
// upstream_error and the caller is told to retry — which can never succeed.
var authFailureMarkers = []string{
	"cannot fetch token",
	"invalid_grant",
}

const quotaProjectSuggestion = "this is not a property permission problem: the credentials have no quota project set for the Search Console API. " +
	"Run: gcloud auth application-default set-quota-project YOUR_PROJECT_ID (that project must have the Search Console API enabled)."

const credentialsRejectedSuggestion = "the credentials were rejected before any API call, so retrying will not help. " +
	"Service account: the key may have been deleted or disabled — generate a new one. " +
	"ADC: re-run gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform"

// MapGoogleAPIError maps a googleapi.Error (or wrapped googleapi.Error) to a
// structured Error. Messages are truncated to 300 characters and never include
// the raw request body, which may contain customer URLs or brand terms.
func MapGoogleAPIError(err error) Error {
	var clientErr Error
	if errors.As(err, &clientErr) {
		return clientErr
	}

	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		msg := truncate(err.Error(), 300)
		if matchesAny(msg, authFailureMarkers...) {
			return Error{Code: ErrAuthFailed, Message: msg, Suggestion: credentialsRejectedSuggestion}
		}
		return Error{
			Code:       ErrUpstreamError,
			Message:    msg,
			Suggestion: "retry the request; if it persists, check the Google Search Console API status",
		}
	}

	msg := truncate(gerr.Message, 300)
	switch gerr.Code {
	case 400:
		return Error{Code: ErrInvalidInput, Message: msg, Suggestion: "check the request parameters against the tool schema"}
	case 401:
		return Error{Code: ErrAuthFailed, Message: msg, Suggestion: "verify the service account key is valid and the Search Console API is enabled"}
	case 403:
		if matchesAny(msg, quotaProjectMarker) {
			return Error{Code: ErrPermissionDenied, Message: msg, Suggestion: quotaProjectSuggestion}
		}
		return Error{Code: ErrPermissionDenied, Message: msg, Suggestion: "ask the property owner to add the service account email to this Search Console property"}
	case 404:
		return Error{Code: ErrNotFound, Message: msg, Suggestion: "check the site_url or feedpath; use list_sites to see accessible properties"}
	case 429:
		return Error{Code: ErrQuotaExceeded, Message: msg, Suggestion: "reduce the date range, remove the 'page' dimension, or wait before retrying"}
	default:
		if gerr.Code >= 500 {
			return Error{Code: ErrUpstreamError, Message: msg, Suggestion: "this is a Google API error; retry after a short wait"}
		}
		return Error{Code: ErrUpstreamError, Message: msg, Suggestion: "unexpected response from Google; retry or check the request"}
	}
}

// matchesAny reports whether s contains any of the markers, case-insensitively.
// Markers must already be lower case.
func matchesAny(s string, markers ...string) bool {
	lower := strings.ToLower(s)
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return strings.TrimSpace(s[:limit]) + "..."
}
