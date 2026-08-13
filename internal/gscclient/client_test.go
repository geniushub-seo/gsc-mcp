package gscclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/geniushub-seo/gsc-mcp/internal/config"
	"google.golang.org/api/option"
)

func TestRequestTimeout_AppliesToCalls(t *testing.T) {
	t.Parallel()
	// Server that sleeps longer than the timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx := context.Background()
	cfg := config.Config{
		RequestTimeout: 50 * time.Millisecond,
	}
	client, err := New(ctx, cfg, option.WithoutAuthentication(), option.WithEndpoint(srv.URL+"/"))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	_, err = client.ListSites(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}

	if ctx.Err() != nil {
		t.Fatalf("parent context should not be cancelled, got %v", ctx.Err())
	}

	// The error should be a context deadline exceeded from the timeout applied
	// inside the wrapper method.
	if !isDeadlineExceeded(err) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func isDeadlineExceeded(err error) bool {
	if err == nil {
		return false
	}
	if err == context.DeadlineExceeded {
		return true
	}
	// URL errors from net/http wrap context.DeadlineExceeded.
	if uerr, ok := err.(interface{ Unwrap() error }); ok {
		return isDeadlineExceeded(uerr.Unwrap())
	}
	return false
}
