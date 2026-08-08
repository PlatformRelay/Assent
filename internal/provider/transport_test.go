package provider_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestBoundedReadOverLimitFailsClosed — AUD-S10 / REQ-AUD-S10-01 (findings
// REL-03, SEC-08). A provider response larger than provider.MaxResponseBytes is
// an ERROR, not a truncated parse: a hostile or broken provider must not be
// able to OOM the run, and the prefix it did send must never be mistaken for a
// complete FactSet. Driven through ResolveFacts (the production entry point) so
// an unwired limit fails the test.
func TestBoundedReadOverLimitFailsClosed(t *testing.T) {
	q := isolationQuery()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Syntactically OPEN JSON: a truncating read would still be handed to
		// the decoder, so only the byte bound can produce the limit error.
		_, _ = io.WriteString(w, `{"facts":{"groups":{"state":"resolved","value":"`)
		const chunk = 64 << 10
		filler := strings.Repeat("x", chunk)
		for n := provider.MaxResponseBytes + 1; n > 0; n -= chunk {
			if n < chunk {
				_, _ = io.WriteString(w, filler[:n])
				break
			}
			_, _ = io.WriteString(w, filler)
		}
		_, _ = io.WriteString(w, `"}}}`)
	}))
	defer srv.Close()

	raw, err := provider.CallHTTP(t.Context(), srv.URL, q, 30*time.Second)
	if err == nil {
		t.Fatal("over-limit provider body must fail closed, got nil error")
	}
	if raw != nil {
		t.Fatalf("over-limit read must return no bytes, got %d", len(raw))
	}
	if !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("error must name the byte limit, got %v", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprint(provider.MaxResponseBytes)) {
		t.Fatalf("error must state the limit %d, got %v", provider.MaxResponseBytes, err)
	}

	call := func(ctx context.Context) ([]byte, error) {
		return provider.CallHTTP(ctx, srv.URL, q, 30*time.Second)
	}
	got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
	f, ok := got.Facts["groups"]
	if !ok {
		t.Fatal("fact key silently absent — must never happen")
	}
	if f.State != provider.StateUnavailable {
		t.Fatalf("over-limit state = %q, want unavailable (reason: %q)", f.State, f.Reason)
	}
	if f.Value != nil {
		t.Fatal("an over-limit body must never carry a value")
	}
	if got.AutoMergeEligible() {
		t.Fatal("over-limit path must keep auto-merge disarmed")
	}
}

// TestBoundedReadUnderLimitUnaffected is the POSITIVE CONTROL for the bound: a
// legitimate (small) provider payload still resolves. Without it, a limit of
// zero would pass TestBoundedReadOverLimitFailsClosed.
func TestBoundedReadUnderLimitUnaffected(t *testing.T) {
	q := isolationQuery()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"facts":{"groups":{"state":"resolved","value":["a"]}}}`)
	}))
	defer srv.Close()

	raw, err := provider.CallHTTP(t.Context(), srv.URL, q, 30*time.Second)
	if err != nil {
		t.Fatalf("a legitimate payload must be unaffected by the bound: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("legitimate payload read returned no bytes")
	}
	if provider.MaxResponseBytes < 1<<20 {
		t.Fatalf("MaxResponseBytes = %d — the bound must stay MB-order, generously above "+
			"legitimate provider payloads", provider.MaxResponseBytes)
	}
}
