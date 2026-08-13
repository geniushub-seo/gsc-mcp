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
		return Error{
			Code:       ErrUpstreamError,
			Message:    truncate(err.Error(), 300),
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

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return strings.TrimSpace(s[:limit]) + "..."
}
