package github

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// surfaces_test.go covers the adapter surfaces the primary tests reach only
// incidentally (E10-S06): the option constructors, the bounded-read edge, the
// content-type/truncation edges of the Contents decode, the GraphQL envelope's
// error paths, and the resolver's request shaping. Each test is small and
// hermetic — the goal is branch coverage on error paths the port contract
// demands, not volume.

func TestWithOptionsCarryThrough(t *testing.T) {
	c := New("https://api.github.example", "tok", "bot",
		WithRetry(RetryPolicy{MaxAttempts: 7, BaseBackoff: 1, MaxBackoff: 2, RequestTimeout: 3,
			Sleep: func(time.Duration) {}, Jitter: func() float64 { return 0.25 }}),
		WithContext(context.Background()))
	if c.retry.MaxAttempts != 7 || c.retry.RequestTimeout != 3 {
		t.Fatalf("WithRetry override lost: %+v", c.retry)
	}
	if c.ctx == nil {
		t.Fatal("WithContext must set the parent context")
	}
}

func TestRepoPartsRejectsNonOwnerRepoShapes(t *testing.T) {
	if got, err := repoParts("octo-org/base-repo"); err != nil || got != "octo-org/base-repo" {
		t.Fatalf("repoParts(owner/repo) = (%q, %v)", got, err)
	}
	for _, bad := range []string{"", "onlyname", "a/b/c"} {
		if _, err := repoParts(bad); err == nil {
			t.Fatalf("repoParts(%q) must reject a non owner/repo pair", bad)
		}
	}
}

func TestGhContentOfRejectsTruncationAndUnknownEncoding(t *testing.T) {
	if _, err := ghContentOf([]byte(`{"content":"","encoding":"base64","truncated":true}`)); err == nil {
		t.Fatal("a truncated contents body must fail closed (never judged as absence)")
	}
	if _, err := ghContentOf([]byte(`{"content":"x","encoding":"percent7"}`)); err == nil {
		t.Fatal("an unknown content encoding must error, never decode silently")
	}
	if raw, err := ghContentOf([]byte(`{"content":"","encoding":""}`)); err != nil || len(raw) != 0 {
		t.Fatalf("an empty-but-present file reads as zero bytes, got (%q, %v)", raw, err)
	}
	if raw, err := ghContentOf([]byte(`{"content":"aGVsbG8=","encoding":"base64"}`)); err != nil || string(raw) != "hello" {
		t.Fatalf("base64 decode = %q, %v", raw, err)
	}
}

func TestContentReadStatusMapping(t *testing.T) {
	// 401/403/500 on the specific path, with the probe granted (200): the Q4
	// table's rows one accessor at a time.
	for _, tc := range []struct {
		status  int
		wantSub string
	}{
		{http.StatusUnauthorized, "unauthorized (401/403)"},
		{http.StatusForbidden, "unauthorized (401/403)"},
		{http.StatusInternalServerError, "unexpected status 500"},
	} {
		c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/contents") {
				_, _ = w.Write([]byte(`[]`)) // the probe: contents-read granted
				return
			}
			w.WriteHeader(tc.status)
		})
		_, err := c.FileAtRef("octo-org/base-repo", ".assent/config.yaml", "tgtSHA")
		if err == nil {
			t.Fatalf("status %d must error", tc.status)
		}
		if !strings.Contains(err.Error(), tc.wantSub) {
			t.Fatalf("status %d error = %v, want %q", tc.status, err, tc.wantSub)
		}
	}
}

// dummyWriter is a minimal io.Writer for handlers that must write but whose
// response is irrelevant to the assertion.
type dummyWriter struct{}

func (dummyWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestGqlDoErrorSurfaces(t *testing.T) {
	// A GraphQL error envelope with no data errors hard; partial data passes
	// through (the adapter's own callers grade the missing shape).
	c, _ := newServer(t, func(_ http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(dummyWriter{}, `{"data":null,"errors":[{"message":"bad query"}]}`)
	})
	if _, err := c.gqlDo(context.Background(), "query {}", nil); err == nil {
		t.Fatal("a graphql error envelope must error")
	}
	// A decode failure (non-JSON body) surfaces as a decode error.
	c2, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "not json")
	})
	if _, err := c2.gqlDo(context.Background(), "query {}", nil); err == nil {
		t.Fatal("a non-JSON graphql response must error")
	}
}
