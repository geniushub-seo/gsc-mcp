package gscclient

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
)

func immediateSleep(_ time.Duration) <-chan time.Time {
	ch := make(chan time.Time)
	close(ch)
	return ch
}

func TestDoWithRetry_Retries429ThenSucceeds(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	result, err := doWithRetry(context.Background(), immediateSleep, func(context.Context) (string, error) {
		n := calls.Add(1)
		if n < 3 {
			return "", &googleapi.Error{Code: 429, Message: "rate limited"}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("result = %q, want ok", result)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
}

func TestDoWithRetry_Exhausted5xxReturnsUpstreamError(t *testing.T) {
	t.Parallel()

	var sleeps atomic.Int32
	sleep := func(time.Duration) <-chan time.Time {
		sleeps.Add(1)
		return immediateSleep(0)
	}

	var calls atomic.Int32
	_, err := doWithRetry(context.Background(), sleep, func(context.Context) (string, error) {
		calls.Add(1)
		return "", &googleapi.Error{Code: 503, Message: "unavailable"}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var clientErr Error
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected gscclient.Error, got %T %v", err, err)
	}
	if clientErr.Code != ErrUpstreamError {
		t.Fatalf("code = %q, want upstream_error", clientErr.Code)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
	if sleeps.Load() != 2 {
		t.Fatalf("sleeps = %d, want 2 (between 3 attempts)", sleeps.Load())
	}
}

func TestDoWithRetry_Exhausted429ReturnsQuotaExceeded(t *testing.T) {
	t.Parallel()

	_, err := doWithRetry(context.Background(), immediateSleep, func(context.Context) (string, error) {
		return "", &googleapi.Error{Code: 429, Message: "rate limited"}
	})
	var clientErr Error
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected gscclient.Error, got %T %v", err, err)
	}
	if clientErr.Code != ErrQuotaExceeded {
		t.Fatalf("code = %q, want quota_exceeded", clientErr.Code)
	}
	if strings.Contains(clientErr.Suggestion, "page") {
		t.Fatalf("suggestion must not invent request parameters: %q", clientErr.Suggestion)
	}
}

func TestDoWithRetry_NoRetryOn400(t *testing.T) {
	t.Parallel()

	var sleeps atomic.Int32
	sleep := func(time.Duration) <-chan time.Time {
		sleeps.Add(1)
		return immediateSleep(0)
	}

	var calls atomic.Int32
	_, err := doWithRetry(context.Background(), sleep, func(context.Context) (string, error) {
		calls.Add(1)
		return "", &googleapi.Error{Code: 400, Message: "bad request"}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	if sleeps.Load() != 0 {
		t.Fatalf("sleeps = %d, want 0", sleeps.Load())
	}
	mapped := MapGoogleAPIError(err)
	if mapped.Code != ErrInvalidInput {
		t.Fatalf("mapped code = %q, want invalid_input", mapped.Code)
	}
}

func TestDoWithRetry_NoRetryOn403(t *testing.T) {
	t.Parallel()

	sleep := func(time.Duration) <-chan time.Time {
		t.Fatal("should not sleep on 403")
		return nil
	}

	var calls atomic.Int32
	_, err := doWithRetry(context.Background(), sleep, func(context.Context) (string, error) {
		calls.Add(1)
		return "", &googleapi.Error{Code: 403, Message: "forbidden"}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	mapped := MapGoogleAPIError(err)
	if mapped.Code != ErrPermissionDenied {
		t.Fatalf("mapped code = %q, want permission_denied", mapped.Code)
	}
}

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		code int
		want bool
	}{
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{429, true},
		{500, true},
		{503, true},
		{200, false},
	}
	for _, tc := range cases {
		got := isRetryable(&googleapi.Error{Code: tc.code})
		if got != tc.want {
			t.Errorf("code %d: got %v, want %v", tc.code, got, tc.want)
		}
	}
	if isRetryable(errors.New("plain")) {
		t.Error("plain error should not be retryable")
	}
}

func TestSleepWithContext_ProductionUsesTimer(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := sleepWithContext(ctx, nil, 10*time.Second)
	if err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("cancelled sleepWithContext waited too long")
	}
}

func TestSleepWithContext_InjectedSleepRespectsContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sleep := func(time.Duration) <-chan time.Time {
		return make(chan time.Time) // never fires
	}
	if err := sleepWithContext(ctx, sleep, 0); err != context.Canceled {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
