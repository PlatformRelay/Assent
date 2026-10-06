package github

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
)

// content_test.go — the S00 Q4 status→sentinel mapping unit cases at the
// adapter level (the forge-level conformance cases arrive later via the
// factory): absent-vs-forbidden discrimination, the metadata-only token
// trap, and the rate-limited 403 transport shape.

// TestContentReadAbsentIsAbsent proves the positive control (S00 Q4): with
// the content-scope probe answered 200, a 404 on the specific path is
// GENUINE absence — forge.ErrNotFound (so real ADD/DELETE detection works) —
// and a present file decodes from the base64 Contents payload.
func TestContentReadAbsentIsAbsent(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/octo-org/base-repo/contents":
			_, _ = io.WriteString(w, "[]")
		case "/repos/octo-org/base-repo/contents/.assent/config.yaml":
			http.Error(w, "absent", http.StatusNotFound)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	_, err := c.FileAtRef(baseRepo, ".assent/config.yaml", "tgtSHA")
	if !errors.Is(err, forge.ErrNotFound) {
		t.Fatalf("error = %v, want forge.ErrNotFound (genuine absence)", err)
	}

	// The same probe also licenses a successful read (the base64 path).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/octo-org/base-repo/contents":
			_, _ = io.WriteString(w, "[]")
		case "/repos/octo-org/base-repo/contents/.assent/config.yaml":
			_, _ = io.WriteString(w, `{"content":"Zm9v","encoding":"base64","size":3}`)
		default:
			unexpectedEndpoint(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c2 := New(srv.URL, patToken, botUser, WithSleeper(func(time.Duration) {}))
	b, err := c2.FileAtRef(baseRepo, ".assent/config.yaml", "tgtSHA")
	if err != nil {
		t.Fatalf("FileAtRef present: %v", err)
	}
	if string(b) != "foo" {
		t.Errorf("content = %q, want foo", b)
	}
}

// TestMetadataOnlyTokenNeverRendersAsAbsent proves the metadata-only
// conformance shape (S00 Q4): a token that can read the repo object but NOT
// contents (fine-grained PAT with metadata:read only) must yield
// ErrUnauthorized on a governed read — NEVER absence — because the probe that
// failed is the same permission the read needs.
func TestMetadataOnlyTokenNeverRendersAsAbsent(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/octo-org/base-repo/contents":
			http.Error(w, "metadata only", http.StatusNotFound)
		case strings.HasPrefix(r.URL.Path, "/repos/octo-org/base-repo/contents/"):
			http.Error(w, "contents denied", http.StatusNotFound)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	_, err := c.FileAtRef(baseRepo, ".assent/config.yaml", "tgtSHA")
	if err == nil {
		t.Fatal("metadata-only token must fail closed on a governed read")
	}
	if errors.Is(err, forge.ErrNotFound) {
		t.Errorf("a 404 content read under a failed probe must NEVER render absent, got %v", err)
	}
	if !errors.Is(err, forge.ErrUnauthorized) {
		t.Errorf("error must wrap forge.ErrUnauthorized, got %v", err)
	}
}

// TestRateLimit403IsTransportError proves S00 Q4's rate-limit row: a 403
// carrying the rate-limit marker is a transport failure (retried within the
// budget), never a sentinel and never absence.
func TestRateLimit403IsTransportError(t *testing.T) {
	attempts := 0
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		attempts++
		_, _ = io.WriteString(w, `{"message":"rate limit exceeded"}`)
	})

	_, err := c.FileAtRef(baseRepo, ".assent/config.yaml", "tgtSHA")
	if err == nil {
		t.Fatal("rate-limited 403 must fail, got nil")
	}
	if !errors.Is(err, errRateLimited) {
		t.Errorf("error must wrap the adapter's errRateLimited, got %v", err)
	}
	if errors.Is(err, forge.ErrNotFound) || errors.Is(err, forge.ErrUnauthorized) {
		t.Errorf("a rate-limited 403 is a transport failure, never a sentinel: %v", err)
	}
	if attempts != defaultMaxAttempts {
		t.Errorf("rate-limited attempts = %d, want %d (transport failure, retried)", attempts, defaultMaxAttempts)
	}
}

// TestRateLimitHeadersOnSuccessDoNotDiscardResult proves the transport
// policy's other polarity: the rate-limit markers are consulted ONLY on a
// 403/429. GitHub reports X-RateLimit-Remaining: 0 on the request that
// CONSUMED the budget's last unit — a 200/201 carrying that header is a
// SUCCESSFUL response, and discarding it would throw away a completed read or
// a performed write (a discarded 201 on a write is indistinguishable from
// never issuing it).
func TestRateLimitHeadersOnSuccessDoNotDiscardResult(t *testing.T) {
	t.Run("201 write succeeds", func(t *testing.T) {
		c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
				// CreateThread reads the pinned head via mrPinned first.
				_, _ = io.WriteString(w, sameRepoPR)
			case r.Method == http.MethodPost && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/comments":
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"id":401,"node_id":"PRRC_node1","user":{"login":"assent-bot"}}`)
			default:
				unexpectedEndpoint(w, r)
			}
		})

		created, err := c.CreateThread(baseRepo, "7", threadMarker(), "body")
		if err != nil {
			t.Fatalf("a 201 carrying X-RateLimit-Remaining: 0 is the request that exhausted the budget — it must succeed, got %v", err)
		}
		if created.ID != "comment/401" {
			t.Errorf("created thread id = %q, want comment/401", created.ID)
		}
	})

	t.Run("200 read succeeds", func(t *testing.T) {
		c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/octo-org/base-repo/contents":
				_, _ = io.WriteString(w, "[]")
			case "/repos/octo-org/base-repo/contents/.assent/config.yaml":
				w.Header().Set("X-RateLimit-Remaining", "0")
				_, _ = io.WriteString(w, `{"content":"Zm9v","encoding":"base64","size":3}`)
			default:
				unexpectedEndpoint(w, r)
			}
		})

		got, err := c.FileAtRef(baseRepo, ".assent/config.yaml", "tgtSHA")
		if err != nil {
			t.Fatalf("a 200 carrying X-RateLimit-Remaining: 0 must not be discarded, got %v", err)
		}
		if string(got) != "foo" {
			t.Errorf("content = %q, want foo", got)
		}
	})
}

// TestPlain403IsUnauthorized — a 403 WITHOUT the rate-limit marker inside a
func TestPlain403IsUnauthorized(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/octo-org/base-repo/contents":
			_, _ = io.WriteString(w, "[]")
		case "/repos/octo-org/base-repo/contents/.assent/config.yaml":
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			unexpectedEndpoint(w, r)
		}
	})
	_, err := c.FileAtRef(baseRepo, ".assent/config.yaml", "tgtSHA")
	if !errors.Is(err, forge.ErrUnauthorized) {
		t.Errorf("plain 403 must wrap ErrUnauthorized, got %v", err)
	}
	if errors.Is(err, forge.ErrNotFound) {
		t.Errorf("forbidden must never render as absent, got %v", err)
	}
}

// TestForkHeadAddressesByHeadSHA proves the S00 Q1 discipline at the adapter:
// FileAtHead reads the fork PR's head at the PINNED head SHA in the base repo
// — a ref the harness answers ONLY at that SHA (or the target SHA for the
// base side), never at a branch name (which does not exist in the base repo
// for a fork).
func TestForkHeadAddressesByHeadSHA(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
			_, _ = io.WriteString(w, forkPR)
		case strings.HasPrefix(r.URL.Path, "/repos/octo-org/base-repo/contents"):
			ref := r.URL.Query().Get("ref")
			if ref == "srcSHA" || ref == "tgtTIP" {
				_, _ = io.WriteString(w, `{"content":"Zm9v","encoding":"base64","size":3}`)
				return
			}
			// Any OTHER ref — a branch-name read inside the base repo — 404s:
			// the fork's head branch does not exist there.
			http.Error(w, "no such ref", http.StatusNotFound)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	// Pin the fork PR first (the accessor's read chain).
	if _, err := c.GetMR(baseRepo, "7"); err != nil {
		t.Fatalf("GetMR: %v", err)
	}
	got, err := c.FileAtHead(baseRepo, "7", ".assent/config.yaml")
	if err != nil {
		t.Fatalf("FileAtHead must address the fork head BY SHA in the base repo: %v", err)
	}
	if string(got) != "foo" {
		t.Errorf("FileAtHead content = %q, want foo", got)
	}

	// FileAtBase reads the pinned TARGET SHA of the base repo.
	got, err = c.FileAtBase(baseRepo, "7", ".assent/config.yaml")
	if err != nil {
		t.Fatalf("FileAtBase: %v", err)
	}
	if string(got) != "foo" {
		t.Errorf("FileAtBase content = %q, want foo", got)
	}
}

// TestContentScopeUnresolvableRefFailsClosed — a bad ref 404s the root
// listing too: the sentinel is fail-closed (ErrUnauthorized, never path
// absence), and the wording must not be a bare authorization claim.
func TestContentScopeUnresolvableRefFailsClosed(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/octo-org/base-repo/contents":
			http.Error(w, "bad ref", http.StatusNotFound)
		default:
			unexpectedEndpoint(w, r)
		}
	})
	_, err := c.FileAtRef(baseRepo, ".assent/config.yaml", "nosuchref")
	if !errors.Is(err, forge.ErrUnauthorized) {
		t.Errorf("an unresolvable ref must fail closed, got %v", err)
	}
	if !strings.Contains(err.Error(), "could not be distinguished") {
		t.Errorf("the wording must not be a bare authorization claim: %v", err)
	}
}
