package provider_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
)

// TestTransportTimeoutUnavailable — REQ-E5-S03-04: HTTP transport timeout
// classifies as unavailable, never resolved.
func TestTransportTimeoutUnavailable(t *testing.T) {
	q := isolationQuery()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer func() {
		close(release)
		srv.Close()
	}()

	call := func(ctx context.Context) ([]byte, error) {
		return provider.CallHTTP(ctx, srv.URL, q, 50*time.Millisecond)
	}
	got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
	f, ok := got.Facts["groups"]
	if !ok {
		t.Fatal("fact key silently absent — must never happen")
	}
	if f.State != provider.StateUnavailable {
		t.Fatalf("timeout state = %q, want unavailable (reason: %q)", f.State, f.Reason)
	}
	if f.State == provider.StateResolved || f.Value != nil {
		t.Fatal("transport timeout must never resolve or carry a value")
	}
	if got.AutoMergeEligible() {
		t.Fatal("timeout path must keep auto-merge disarmed")
	}
}
