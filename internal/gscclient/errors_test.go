package gscclient

import (
	"errors"
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
