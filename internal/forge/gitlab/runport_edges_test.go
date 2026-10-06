package gitlab

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// runport_edges_test.go covers the RunPort edges the primary tests reach only
// incidentally (E10-S02): the identity contract and the port-level accessors'
// fork fail-closed branch.

func TestIdentityIsTheFilterIdentity(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		_, _ = io.WriteString(w, `{"id":999,"username":"assent-bot"}`)
	})
	ident, err := c.Identity()
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if ident.Kind != forge.IdentityUser || ident.Login != botUser {
		t.Fatalf("Identity = %+v, want the configured bot identity as a user", ident)
	}
}

func TestFileAtHeadWithoutSourceProjectIDFailsClosed(t *testing.T) {
	// A fork MR whose source_project_id is 0: the accessor must ERROR, never
	// degrade to a non-fork read (the absent-means-trusted trap).
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/merge_requests/7"):
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"source_project_id":0,"sha":"srcSHA","source_branch":"feature","target_branch":"main"}`)
		case strings.Contains(r.URL.Path, "/repository/branches/main"):
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtSHA"}}`)
		default:
			http.Error(w, "unexpected route", http.StatusInternalServerError)
		}
	})
	if _, err := c.FileAtHead("42", "7", "topics/orders.yaml"); err == nil {
		t.Fatal("a fork MR without a source project id must fail closed at the head accessor")
	}
}

func TestFileAtBaseReadsAtThePinnedTargetSHAViaPin(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/merge_requests/7"):
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"source_project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main"}`)
		case strings.Contains(r.URL.Path, "/repository/branches/main"):
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtSHA"}}`)
		case strings.HasSuffix(r.URL.Path, "/raw"):
			if r.URL.Query().Get("ref") != "tgtSHA" {
				t.Errorf("FileAtBase must read at the pinned target SHA, got ref %q", r.URL.Query().Get("ref"))
			}
			_, _ = io.WriteString(w, "BASE-CONTENT")
		default:
			http.Error(w, "unexpected route", http.StatusInternalServerError)
		}
	})
	raw, err := c.FileAtBase("42", "7", "topics/orders.yaml")
	if err != nil {
		t.Fatalf("FileAtBase: %v", err)
	}
	if string(raw) != "BASE-CONTENT" {
		t.Fatalf("FileAtBase = %q", raw)
	}
}

func TestFileAtHeadSameRepoReadsThePinnedSourceSHAViaPin(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/merge_requests/7"):
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"source_project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main"}`)
		case strings.Contains(r.URL.Path, "/repository/branches/main"):
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtSHA"}}`)
		case strings.HasSuffix(r.URL.Path, "/raw"):
			if r.URL.Query().Get("ref") != "srcSHA" {
				t.Errorf("FileAtHead must read at the pinned source SHA, got ref %q", r.URL.Query().Get("ref"))
			}
			_, _ = io.WriteString(w, "HEAD-CONTENT")
		default:
			http.Error(w, "unexpected route", http.StatusInternalServerError)
		}
	})
	raw, err := c.FileAtHead("42", "7", "topics/orders.yaml")
	if err != nil {
		t.Fatalf("FileAtHead: %v", err)
	}
	if string(raw) != "HEAD-CONTENT" {
		t.Fatalf("FileAtHead = %q", raw)
	}
}

func TestGetMRPinIsFirstWriteWins(t *testing.T) {
	// GetMR IS the run's pinned read: it fills the pin, and a later GetMR
	// (CurrentHeads' re-read) must NOT move it — first-write-wins (E10 branch
	// review round 2). The CAS reads deliberately go fresh; the judged bytes
	// stay at the evaluation pin.
	var reads int
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/merge_requests/7"):
			reads++
			// The SECOND read reports a MOVED head; the pin must keep the
			// first read's SHA.
			sha := "firstSHA"
			if reads > 1 {
				sha = "movedSHA"
			}
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"source_project_id":42,"sha":"`+sha+`","source_branch":"feature","target_branch":"main"}`)
		case strings.Contains(r.URL.Path, "/repository/branches/main"):
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtSHA"}}`)
		case strings.HasSuffix(r.URL.Path, "/raw"):
			if r.URL.Query().Get("ref") != "firstSHA" {
				t.Errorf("FileAtHead must read at the FIRST read's pinned SHA, got ref %q", r.URL.Query().Get("ref"))
			}
			_, _ = io.WriteString(w, "PINNED-CONTENT")
		default:
			http.Error(w, "unexpected route", http.StatusInternalServerError)
		}
	})
	if _, err := c.GetMR("42", "7"); err != nil {
		t.Fatalf("GetMR 1: %v", err)
	}
	if _, _, _, err := c.CurrentHeads("42", "7"); err != nil {
		t.Fatalf("CurrentHeads (the fresh re-read): %v", err)
	}
	raw, err := c.FileAtHead("42", "7", "topics/orders.yaml")
	if err != nil {
		t.Fatalf("FileAtHead: %v", err)
	}
	if string(raw) != "PINNED-CONTENT" {
		t.Fatalf("FileAtHead judged at %q, want the FIRST read's pin", raw)
	}
}
