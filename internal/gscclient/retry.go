package gscclient

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"google.golang.org/api/googleapi"
)

const (
	retryMaxAttempts = 3
	retryBaseDelay   = 5 * time.Second
)

// doWithRetry runs fn up to retryMaxAttempts times. It retries only on HTTP 429
// and 5xx. After exhausting retries it returns a structured quota_exceeded error.
// 4xx validation/permission errors are returned immediately without retry.
func doWithRetry[T any](ctx context.Context, sleep func(time.Duration) <-chan time.Time, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error

	for attempt := 1; attempt <= retryMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}

		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if !isRetryable(err) {
			return zero, err
		}

		if attempt == retryMaxAttempts {
			break
		}

		if err := sleepWithContext(ctx, sleep, retryDelay(attempt)); err != nil {
			return zero, err
		}
	}

	return zero, NewError(
		ErrQuotaExceeded,
		"upstream request failed after retries: "+truncate(lastErr.Error(), 200),
		"reduce the date range, remove the 'page' dimension, or wait before retrying",
	)
}

func isRetryable(err error) bool {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		return false
	}
	if gerr.Code == 429 {
		return true
	}
	return gerr.Code >= 500 && gerr.Code <= 599
}

// retryDelay returns base * 2^(attempt-1) plus up to 20% jitter.
// attempt is 1-based for the failure that just occurred.
func retryDelay(attempt int) time.Duration {
	exp := retryBaseDelay
	for i := 1; i < attempt; i++ {
		exp *= 2
	}
	jitter := time.Duration(rand.Int63n(int64(exp/5) + 1))
	return exp + jitter
}

// sleepWithContext waits for d unless ctx is cancelled first. In production
// (sleep == nil) it uses time.NewTimer so there is no background goroutine
// leaked when ctx is cancelled. Tests may inject a custom sleep channel.
func sleepWithContext(ctx context.Context, sleep func(time.Duration) <-chan time.Time, d time.Duration) error {
	if sleep == nil {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-sleep(d):
		return nil
	}
}
