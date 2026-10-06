package conformance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	github "github.com/PlatformRelay/assent/internal/forge/github"
	gitlab "github.com/PlatformRelay/assent/internal/forge/gitlab"
)

// transport_cases_test.go holds the PORT-LEVEL TRANSPORT cases (E10-S05 /
// REQ-E10-S05-01/02, ADR-0021 item 4): bounded response reads with
// fail-closed exhaustion, pagination caps with fail-closed exhaustion,
// idempotent-GET-only retry, writes never retried, and per-request context
// deadlines. These are requirements of the PORT — every adapter must satisfy
// them — asserted here against BOTH transport-bearing factories: the GitLab
// harness and, since E10-S10, the GitHub harness (the fake has no transport
// to violate). The catalog rows carry that disposition.
//
// They are DIRECT entry tests rather than Cases() rows: the fake backend has no
// transport to violate (its content is in memory), so these cases are
// adapter-factory-shaped by nature — the catalog rows carry that disposition.

func transportHarness(t *testing.T) *gitlabHarness {
	t.Helper()
	h := newGitLabHarness("42", "7")
	h.botAuthor = "assent-bot"
	h.sourceSHA = pinSource
	h.targetSHA = pinTarget
	return h
}

// githubTransportHarness builds the GitHub harness with the transport knobs
// the transport cases drive (the mirror of transportHarness).
func githubTransportHarness(t *testing.T) *githubHarness {
	t.Helper()
	h := newGitHubHarness("platform/orders-service", "482")
	h.botAuthor = botID
	h.sourceSHA = pinSource
	h.targetSHA = pinTarget
	return h
}

// TestConformanceBoundedReads is REQ-E10-S05-01 (AUD-S10 stated at the port):
// a single response over the read bound is an ERROR (the truncated prefix is
// never parsed as if it were the complete document), and a paginator that
// never shortens errors AT THE CAP instead of spinning — exhaustion fails
// closed, never a silent truncation.
func TestConformanceBoundedReads(t *testing.T) {
	t.Run("oversized_body_is_error_not_silent_truncation", func(t *testing.T) {
		t.Run("gitlab", func(t *testing.T) {
			h := newGitLabHarness("42", "7")
			h.sourceSHA = pinSource
			h.targetSHA = pinTarget
			h.baseSet = true
			// 9 MiB: over the GitLab adapter's own 8 MiB read bound (maxResponseBytes
			// in internal/forge/gitlab), whatever that bound's exact value is — the
			// case pins the REQUIREMENT ("a read over the bound errors"), not the
			// number; the adapter's own unit tests pin the constant.
			h.baseFile = make([]byte, 9<<20)
			srv := httptest.NewServer(http.HandlerFunc(h.handle))
			t.Cleanup(srv.Close)
			c := gitlab.New(srv.URL, "test-token", h.botAuthor, gitlab.WithSleeper(func(time.Duration) {}))

			raw, err := c.FileAtRef("42", "topics/orders.yaml", h.targetSHA)
			if err == nil {
				t.Fatalf("a %d-byte body must be refused, got %d bytes of content", len(h.baseFile), len(raw))
			}
		})

		t.Run("github", func(t *testing.T) {
			h := newGitHubHarness("platform/orders-service", "482")
			h.sourceSHA = pinSource
			h.targetSHA = pinTarget
			h.botAuthor = botID
			h.baseSet = true
			// 9 MiB: over the GitHub adapter's own 8 MiB read bound. The case
			// pins the REQUIREMENT ("a read over the bound errors"), not the
			// number; the adapter's own unit tests pin the constant.
			h.baseFile = make([]byte, 9<<20)
			c := h.client(t)

			raw, err := c.FileAtRef("platform/orders-service", "topics/orders.yaml", h.targetSHA)
			if err == nil {
				t.Fatalf("a %d-byte body must be refused, got %d bytes of content", len(h.baseFile), len(raw))
			}
		})
	})

	t.Run("pagination_cap_fails_closed", func(t *testing.T) {
		t.Run("gitlab", func(t *testing.T) {
			h := newGitLabHarness("42", "7")
			h.pageStorm = true // every page serves a FULL page, forever
			c := h.client(t)

			if _, err := c.ListBotThreads("42", "7"); err == nil {
				t.Fatal("a paginator that never shortens must ERROR at the cap, never silently truncate")
			} else if !strings.Contains(err.Error(), "pagination cap") {
				t.Fatalf("the cap error must name its cause, got %v", err)
			}
		})

		t.Run("github", func(t *testing.T) {
			h := newGitHubHarness("platform/orders-service", "482")
			h.botAuthor = botID
			h.pageStorm = true // every page serves a FULL page, forever
			c := h.client(t)

			if _, err := c.ListBotThreads("platform/orders-service", "482"); err == nil {
				t.Fatal("a paginator that never shortens must ERROR at the cap, never silently truncate")
			} else if !strings.Contains(err.Error(), "pagination cap") {
				t.Fatalf("the cap error must name its cause, got %v", err)
			}
		})
	})
}

// TestConformanceWritesNeverRetried is REQ-E10-S05-02's write-polarity case:
// an injected 5xx on a WRITE yields EXACTLY ONE attempt — the write is never
// replayed (a duplicate thread or a replayed merge PUT behind the caller's
// back is the defect). The GET side retries by contract; the WRITE side is
// counted, not trusted.
func TestConformanceWritesNeverRetried(t *testing.T) {
	t.Run("gitlab", func(t *testing.T) {
		h := newGitLabHarness("42", "7")
		h.sourceSHA = pinSource
		h.targetSHA = pinTarget
		h.writeStatus = 503
		c := h.client(t)

		if _, err := c.CreateThread("42", "7", rerunChallengeMarker(), "body"); err == nil {
			t.Fatal("a 5xx write must error")
		}
		if got := h.discPOSTs; got != 1 {
			t.Fatalf("a transient write failure must be attempted EXACTLY once, got %d attempt(s)", got)
		}
	})

	t.Run("github", func(t *testing.T) {
		h := newGitHubHarness("platform/orders-service", "482")
		h.sourceSHA = pinSource
		h.targetSHA = pinTarget
		h.botAuthor = botID
		h.writeStatus = 503
		c := h.client(t)

		if _, err := c.CreateThread("platform/orders-service", "482", rerunChallengeMarker(), "body"); err == nil {
			t.Fatal("a 5xx write must error")
		}
		if got := h.reviewPOSTs; got != 1 {
			t.Fatalf("a transient write failure must be attempted EXACTLY once, got %d attempt(s)", got)
		}
	})
}

// TestConformanceDeadlineBounded is ADR-0021 item 4's deadline requirement: a
// hung connection errors within the per-request deadline — it consumes one
// attempt, not the whole run. The RequestTimeout is deliberately overridden to
// a small value so the case proves the deadline is enforced, not the exact
// number.
func TestConformanceDeadlineBounded(t *testing.T) {
	t.Run("gitlab", func(t *testing.T) {
		h := newGitLabHarness("42", "7")
		h.sourceSHA = pinSource
		h.targetSHA = pinTarget
		h.slowAfter = 1500 * time.Millisecond
		c := h.clientWith(t, gitlab.WithRetry(gitlab.RetryPolicy{
			MaxAttempts:    2,
			BaseBackoff:    time.Millisecond,
			MaxBackoff:     time.Millisecond,
			RequestTimeout: 100 * time.Millisecond,
			Sleep:          func(time.Duration) {},
			Jitter:         func() float64 { return 0 },
		}))
		start := time.Now()
		_, err := c.GetMR("42", "7")
		if err == nil {
			t.Fatal("a response slower than the per-request deadline must error, never hang")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("the deadline bounded the request: %d attempts still took %s", 30, elapsed)
		}
	})

	t.Run("github", func(t *testing.T) {
		h := newGitHubHarness("platform/orders-service", "482")
		h.botAuthor = botID
		h.sourceSHA = pinSource
		h.targetSHA = pinTarget
		h.slowAfter = 1500 * time.Millisecond
		c := h.clientWith(t, github.WithRetry(github.RetryPolicy{
			MaxAttempts:    2,
			BaseBackoff:    time.Millisecond,
			MaxBackoff:     time.Millisecond,
			RequestTimeout: 100 * time.Millisecond,
			Sleep:          func(time.Duration) {},
			Jitter:         func() float64 { return 0 },
		}))
		start := time.Now()
		_, err := c.GetMR("platform/orders-service", "482")
		if err == nil {
			t.Fatal("a response slower than the per-request deadline must error, never hang")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("the deadline bound the request: the attempts still took %s", elapsed)
		}
	})
}
