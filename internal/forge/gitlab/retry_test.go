package gitlab

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
)

// AUD-S11 (audit finding REL-04): idempotent GitLab READS are retried with a
// bounded, jittered backoff under a context deadline, so one transient 5xx or
// timeout does not fail a run that would have decided correctly. WRITES are
// never auto-retried — retrying a POST duplicates a note or an approval, and
// CAS/idempotence discipline stays the caller's (ADR-0019).
//
// Retries change AVAILABILITY, never DECISIONS: exhausting the budget is still
// a hard error, exactly as an un-retried failure is today.
//
// Determinism: every test injects a recording sleeper and a fixed jitter
// source. Nothing here reads the wall clock or math/rand.

// recorder is a deterministic sleeper: it records the backoff durations the
// client asked for and returns instantly, so assertions pin the SCHEDULE
// without spending it.
type recorder struct {
	mu     sync.Mutex
	slept  []time.Duration
	jitter float64
}

func (r *recorder) sleep(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.slept = append(r.slept, d)
}

func (r *recorder) jit() float64 { return r.jitter }

func (r *recorder) durations() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Duration(nil), r.slept...)
}

// retryClient builds a client against handler h through the PRODUCTION
// constructor, with a deterministic backoff injected via the same Option seam
// the CLI could use. Extra options are appended, so a case can also pin the
// context.
func retryClient(t *testing.T, jitter float64, h http.HandlerFunc, extra ...Option) (*Client, *recorder) {
	t.Helper()
	rec := &recorder{jitter: jitter}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	opts := append([]Option{WithSleeper(rec.sleep), WithJitter(rec.jit)}, extra...)
	return New(srv.URL, "test-token", botUser, opts...), rec
}

// TestRetryDefaults pins the SHIPPED policy. Every other test in this file
// injects a sleeper, so without this the production defaults would be
// unasserted — the classic "the seam is tested, the wiring is not".
func TestRetryDefaults(t *testing.T) {
	c := New("https://gitlab.example", "tok", botUser)
	p := c.retry
	if p.MaxAttempts != defaultMaxAttempts {
		t.Fatalf("MaxAttempts = %d, want the default %d", p.MaxAttempts, defaultMaxAttempts)
	}
	if defaultMaxAttempts != 3 {
		t.Fatalf("defaultMaxAttempts = %d — the budget must stay small and bounded", defaultMaxAttempts)
	}
	if p.BaseBackoff != defaultBaseBackoff {
		t.Fatalf("BaseBackoff = %v, want %v", p.BaseBackoff, defaultBaseBackoff)
	}
	if p.MaxBackoff < p.BaseBackoff {
		t.Fatalf("MaxBackoff %v must not be below BaseBackoff %v", p.MaxBackoff, p.BaseBackoff)
	}
	if p.RequestTimeout <= 0 {
		t.Fatal("a per-request context deadline must be configured by default")
	}
	if p.Sleep == nil || p.Jitter == nil {
		t.Fatal("New must install a default sleeper and jitter source")
	}
}

// TestWithRetryOverridesOnlyWhatIsSet pins the option's partial-override
// contract in BOTH polarities: a set field wins, an unset (zero) field keeps
// the shipped default. Without the second half, WithRetry{} would silently zero
// the budget and disable retrying altogether.
func TestWithRetryOverridesOnlyWhatIsSet(t *testing.T) {
	noop := func(time.Duration) {}
	zero := func() float64 { return 0 }

	full := New("https://gitlab.example", "tok", botUser, WithRetry(RetryPolicy{
		MaxAttempts:    5,
		BaseBackoff:    10 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
		RequestTimeout: time.Second,
		Sleep:          noop,
		Jitter:         zero,
	}))
	if full.retry.MaxAttempts != 5 || full.retry.BaseBackoff != 10*time.Millisecond ||
		full.retry.MaxBackoff != 20*time.Millisecond || full.retry.RequestTimeout != time.Second {
		t.Fatalf("WithRetry must apply every set field, got %+v", full.retry)
	}

	// The exponential is CLAMPED at MaxBackoff: attempt 3 would ask for 40ms.
	if got := full.backoff(3); got != 10*time.Millisecond {
		t.Fatalf("backoff(3) = %v, want the clamped half-window 10ms", got)
	}

	empty := New("https://gitlab.example", "tok", botUser, WithRetry(RetryPolicy{}))
	if empty.retry.MaxAttempts != defaultMaxAttempts ||
		empty.retry.BaseBackoff != defaultBaseBackoff ||
		empty.retry.MaxBackoff != defaultMaxBackoff ||
		empty.retry.RequestTimeout != defaultRequestTimeout {
		t.Fatalf("an empty RetryPolicy must keep every default, got %+v", empty.retry)
	}
	if empty.retry.Sleep == nil || empty.retry.Jitter == nil {
		t.Fatal("an empty RetryPolicy must not drop the default sleeper/jitter")
	}

	// Nil-valued options are no-ops rather than a nil-panic at the first retry.
	var nilCtx context.Context
	safe := New("https://gitlab.example", "tok", botUser,
		WithSleeper(nil), WithJitter(nil), WithContext(nilCtx))
	if safe.retry.Sleep == nil || safe.retry.Jitter == nil || safe.ctx == nil {
		t.Fatal("nil options must leave the defaults installed")
	}
}

// TestIdempotentRetry — REQ-AUD-S11-01. A GET that 503s twice then 200s
// succeeds; the attempt count and the backoff schedule are both pinned; and
// exhausting the budget or the context deadline is a HARD error.
func TestIdempotentRetry(t *testing.T) {
	t.Run("get_succeeds_after_transient_5xx", func(t *testing.T) {
		attempts := 0
		c, rec := retryClient(t, 0, func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			if attempts < 3 {
				http.Error(w, "upstream wobble", http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `[]`)
		})

		threads, err := c.ListBotThreads("42", "7")
		if err != nil {
			t.Fatalf("a GET that recovers within the budget must succeed: %v", err)
		}
		if len(threads) != 0 {
			t.Fatalf("got %d threads, want 0", len(threads))
		}
		if attempts != 3 {
			t.Fatalf("attempts = %d, want 3 (two retries then success)", attempts)
		}
		// jitter 0 → the low half of each exponential window.
		want := []time.Duration{defaultBaseBackoff / 2, defaultBaseBackoff}
		if got := rec.durations(); !sameDurations(got, want) {
			t.Fatalf("backoff schedule = %v, want %v (bounded exponential)", got, want)
		}
	})

	t.Run("jitter_widens_the_window", func(t *testing.T) {
		// POSITIVE CONTROL for the jitter term: the same schedule with the
		// jitter source at its maximum is the UPPER half of each window. A
		// constant (unjittered) backoff cannot satisfy both this and the case
		// above.
		attempts := 0
		c, rec := retryClient(t, 1, func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			if attempts < 3 {
				http.Error(w, "upstream wobble", http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `[]`)
		})
		if _, err := c.ListBotThreads("42", "7"); err != nil {
			t.Fatal(err)
		}
		want := []time.Duration{defaultBaseBackoff, 2 * defaultBaseBackoff}
		if got := rec.durations(); !sameDurations(got, want) {
			t.Fatalf("backoff schedule = %v, want %v", got, want)
		}
	})

	t.Run("429_is_retried", func(t *testing.T) {
		attempts := 0
		c, _ := retryClient(t, 0, func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			if attempts < 2 {
				http.Error(w, "slow down", http.StatusTooManyRequests)
				return
			}
			_, _ = io.WriteString(w, `[]`)
		})
		if _, err := c.ListBotNotes("42", "7"); err != nil {
			t.Fatalf("429 must be retried, not surfaced: %v", err)
		}
		if attempts != 2 {
			t.Fatalf("attempts = %d, want 2", attempts)
		}
	})

	t.Run("4xx_is_not_retried", func(t *testing.T) {
		// NEGATIVE CONTROL: a deterministic client error is not transient.
		// Retrying it would waste the budget and mask the real cause.
		attempts := 0
		c, rec := retryClient(t, 0, func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			http.Error(w, "gone", http.StatusNotFound)
		})
		if _, err := c.FileAtRef("42", "policy.yaml", "main"); err == nil {
			t.Fatal("404 must still surface as ErrNotFound")
		}
		if attempts != 1 {
			t.Fatalf("attempts = %d, want 1 (404 is not transient)", attempts)
		}
		if got := rec.durations(); len(got) != 0 {
			t.Fatalf("no backoff may be spent on a 404, slept %v", got)
		}
	})

	t.Run("budget_exhaustion_is_a_hard_error", func(t *testing.T) {
		attempts := 0
		c, rec := retryClient(t, 0, func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			http.Error(w, "still broken", http.StatusServiceUnavailable)
		})
		if _, err := c.ListBotThreads("42", "7"); err == nil {
			t.Fatal("a GET failing past the budget must fail closed, not degrade")
		}
		if attempts != defaultMaxAttempts {
			t.Fatalf("attempts = %d, want the bounded budget %d", attempts, defaultMaxAttempts)
		}
		if got := len(rec.durations()); got != defaultMaxAttempts-1 {
			t.Fatalf("slept %d times, want %d (one gap between attempts)", got, defaultMaxAttempts-1)
		}
	})

	t.Run("transport_error_is_retried_then_fails_hard", func(t *testing.T) {
		rec := &recorder{}
		c := New("http://127.0.0.1:0", "tok", botUser,
			WithSleeper(rec.sleep), WithJitter(rec.jit))
		if _, err := c.ListBotThreads("42", "7"); err == nil {
			t.Fatal("an unreachable endpoint must still fail closed")
		}
		if got := len(rec.durations()); got != defaultMaxAttempts-1 {
			t.Fatalf("slept %d times, want %d", got, defaultMaxAttempts-1)
		}
	})

	t.Run("expired_context_is_a_hard_error_with_no_request", func(t *testing.T) {
		attempts := 0
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c, rec := retryClient(t, 0, func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			_, _ = io.WriteString(w, `[]`)
		}, WithContext(ctx))

		if _, err := c.ListBotThreads("42", "7"); err == nil {
			t.Fatal("an expired context must fail closed")
		}
		if attempts != 0 {
			t.Fatalf("an expired context must issue zero requests, made %d", attempts)
		}
		if got := rec.durations(); len(got) != 0 {
			t.Fatalf("an expired context must spend no backoff, slept %v", got)
		}
	})

	t.Run("context_cancelled_mid_budget_stops_retrying", func(t *testing.T) {
		attempts := 0
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c, _ := retryClient(t, 0, func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			cancel() // the deadline blows during the first attempt
			http.Error(w, "wobble", http.StatusServiceUnavailable)
		}, WithContext(ctx))

		if _, err := c.ListBotThreads("42", "7"); err == nil {
			t.Fatal("a cancelled context must fail closed")
		}
		if attempts != 1 {
			t.Fatalf("retrying must stop at cancellation, made %d attempts", attempts)
		}
	})
}

// TestWritesNeverRetried — REQ-AUD-S11-02 (fail-safe · writes). Every
// non-idempotent endpoint gets EXACTLY ONE attempt on a transient failure.
// Retrying a POST would duplicate a thread, a summary note or an approval; a
// merge PUT is CAS-guarded and must not be replayed behind the caller's back.
//
// The table is driven through the real port methods, and every case's write
// path is deliberately given the SAME 503 that the GET cases above retry — so
// the only thing distinguishing them is the method predicate.
func TestWritesNeverRetried(t *testing.T) {
	for _, tc := range []struct {
		name      string
		method    string
		writePath string
		call      func(c *Client) error
	}{
		{
			name: "POST create thread", method: http.MethodPost,
			writePath: "/api/v4/projects/42/merge_requests/7/discussions",
			call: func(c *Client) error {
				_, err := c.CreateThread("42", "7", botMarker(), "body")
				return err
			},
		},
		{
			name: "POST create summary note", method: http.MethodPost,
			writePath: "/api/v4/projects/42/merge_requests/7/notes",
			call: func(c *Client) error {
				_, err := c.UpsertComment("42", "7", summaryMarkerFor("42", "7"), "body")
				return err
			},
		},
		{
			name: "PUT resolve thread", method: http.MethodPut,
			writePath: "/api/v4/projects/42/merge_requests/7/discussions/d1",
			call: func(c *Client) error {
				return c.ResolveThread("42", "7", "d1")
			},
		},
		{
			name: "POST approve", method: http.MethodPost,
			writePath: "/api/v4/projects/42/merge_requests/7/approve",
			call: func(c *Client) error {
				_, err := c.Approve("42", "7")
				return err
			},
		},
		{
			name: "PUT merge CAS", method: http.MethodPut,
			writePath: "/api/v4/projects/42/merge_requests/7/merge",
			call: func(c *Client) error {
				_, err := c.MergeCAS("42", "7", forge.DesiredMerge{
					SourceSha: "srcSHA", TargetSha: "tgtTIP",
					MergeResultDigest: SyntheticDigest("srcSHA", "tgtTIP"),
				})
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			c, rec := retryClient(t, 0, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == tc.method && r.URL.Path == tc.writePath {
					writes++
					http.Error(w, "transient", http.StatusServiceUnavailable)
					return
				}
				serveWriteTestReads(w, r)
			})

			if err := tc.call(c); err == nil {
				t.Fatal("a 503 on a write must surface as an error")
			}
			if writes != 1 {
				t.Fatalf("%s %s attempted %d times — writes must NEVER be auto-retried",
					tc.method, tc.writePath, writes)
			}
			if got := rec.durations(); len(got) != 0 {
				t.Fatalf("no backoff may be spent on a write, slept %v", got)
			}
		})
	}
}

// TestRetryableRequestWithBodyIsNotReplayed — review finding F5. An io.Reader
// body can be consumed only once, so replaying it would send an empty second
// request. Every retryable call site passes nil today; this pins that the
// safety is STRUCTURAL rather than conventional, and fails in the safe
// direction (one attempt, never a corrupted replay).
//
// It calls the `do` seam directly because no production call site can construct
// this shape — that is exactly the invariant under test.
func TestRetryableRequestWithBodyIsNotReplayed(t *testing.T) {
	attempts := 0
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		http.Error(w, "wobble", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "test-token", botUser, WithSleeper(rec.sleep), WithJitter(rec.jit))

	if _, _, err := c.do(http.MethodGet, "/api/v4/x", strings.NewReader("payload"), "text/plain"); err != nil {
		t.Fatalf("the request itself must still round-trip: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("a GET carrying a body must not be replayed: attempts = %d, want 1", attempts)
	}

	// POSITIVE CONTROL: the identical GET with a nil body DOES retry, so the
	// guard above is the body check and not a disabled retry path.
	attempts = 0
	if _, _, err := c.do(http.MethodGet, "/api/v4/x", nil, ""); err != nil {
		t.Fatal(err)
	}
	if attempts != defaultMaxAttempts {
		t.Fatalf("a nil-body GET must still retry: attempts = %d, want %d", attempts, defaultMaxAttempts)
	}
}

// TestWriteMethodsAreNotRetryable is the table half of REQ-AUD-S11-02: the
// method predicate itself, pinned in BOTH polarities so neither "retry
// everything" nor "retry nothing" passes.
func TestWriteMethodsAreNotRetryable(t *testing.T) {
	for _, m := range []string{http.MethodGet, http.MethodHead} {
		if !retryableMethod(m) {
			t.Errorf("%s is idempotent and MUST be retryable", m)
		}
	}
	for _, m := range []string{
		http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
	} {
		if retryableMethod(m) {
			t.Errorf("%s is non-idempotent and MUST NEVER be auto-retried", m)
		}
	}
}

// serveWriteTestReads answers the READS a write path performs first (the notes
// listing UpsertComment consults, and the heads MergeCAS re-reads) so the only
// failing request in each table case is the write itself.
func serveWriteTestReads(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/v4/projects/42/merge_requests/7":
		_, _ = io.WriteString(w,
			`{"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"f","target_branch":"main"}`)
	case r.URL.Path == "/api/v4/projects/42/repository/branches/main":
		_, _ = io.WriteString(w, `{"commit":{"id":"tgtTIP"}}`)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/notes"):
		_, _ = io.WriteString(w, `[]`)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
	}
}

func summaryMarkerFor(project, mr string) forge.Marker {
	m := botMarker()
	m.Slot.Project = project
	m.Slot.MR = mr
	m.Slot.Effect = "comment"
	m.Artifact.Kind = "summary-comment"
	return m
}

func sameDurations(got, want []time.Duration) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
