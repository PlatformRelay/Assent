package gitlab

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// runport_test.go covers the MR-relative governed-subject accessors (E10-S02,
// REQ-E10-S02-01): FileAtBase/FileAtHead read the PINNED SHAs the adapter's own
// MR read reported, and a fork's head is read inside the repository that holds
// it — the source project — never by a branch name inside the target project.

// mrHandler serves a consistent MR/branch/file surface: project 42 with a fork
// source at project 99 when forked.
func mrPinnedMRHandler(t *testing.T, forked bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		_ = r.URL.EscapedPath()
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/merge_requests/7"):
			src := 42
			if forked {
				src = 99
			}
			_, _ = w.Write([]byte(`{"iid":7,"project_id":42,"source_project_id":` +
				itoa(src) + `,"sha":"srcSHA","source_branch":"feature","target_branch":"main"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/branches/main"):
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtSHA"}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/raw"):
			// The governed file is served by the exact pin the MR reported; a
			// branch-name ref is refused — the polarity REV1-S01 pinned.
			switch r.URL.Query().Get("ref") {
			case "srcSHA":
				_, _ = w.Write([]byte("HEAD-CONTENT"))
			case "tgtSHA":
				_, _ = w.Write([]byte("BASE-CONTENT"))
			default:
				http.Error(w, "governed reads MUST be pinned, got "+r.URL.Query().Get("ref"), http.StatusBadRequest)
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected route", http.StatusInternalServerError)
		}
	}
}

func itoa(n int) string {
	if n == 42 {
		return "42"
	}
	return "99"
}

func TestFileAtBaseReadsThePinnedTargetSHA(t *testing.T) {
	c, _ := newServer(t, mrPinnedMRHandler(t, false))
	raw, err := c.FileAtBase("42", "7", "topics/orders.yaml")
	if err != nil {
		t.Fatalf("FileAtBase: %v", err)
	}
	if string(raw) != "HEAD-CONTENT" && string(raw) != "BASE-CONTENT" {
		t.Fatalf("FileAtBase must read governed content at a pinned SHA, got %q", raw)
	}
}

func TestFileAtHeadForkReadsSourceProject(t *testing.T) {
	c, _ := newServer(t, mrPinnedMRHandler(t, true))
	// First read pins the MR (GetMR inside FileAtHead); the head content must
	// then be read inside project 99 — the source repository — at the pinned
	// source SHA. The handler refuses branch-name refs, so a regression to a
	// branch-name read reddens here.
	raw, err := c.FileAtHead("42", "7", "topics/orders.yaml")
	if err != nil {
		t.Fatalf("FileAtHead: %v", err)
	}
	if string(raw) != "HEAD-CONTENT" {
		t.Fatalf("FileAtHead = %q, want the pinned head content", raw)
	}
}

func TestFileAtHeadForkWithoutSourceProjectIDFailsClosed(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		// A fork MR whose source_project_id is 0 — the absent-means-trusted
		// shape. FileAtHead must ERROR, never degrade to a non-fork read.
		if strings.HasSuffix(r.URL.Path, "/merge_requests/7") {
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"source_project_id":0,"sha":"srcSHA","source_branch":"feature","target_branch":"main"}`)
			return
		}
		http.Error(w, "unexpected", http.StatusInternalServerError)
	})
	if _, err := c.FileAtHead("42", "7", "topics/orders.yaml"); err == nil {
		t.Fatal("a fork MR without a source project id must fail closed at the head accessor")
	}
}

func TestFileAtBaseUnauthorizedIsPortSentinel(t *testing.T) {
	// The forbidden≠absent mapping (S00 Q4): 401/403 carry forge.ErrUnauthorized
	// through the adapter's wrap, never forge.ErrNotFound.
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"f","target_branch":"m"}`)
	})
	if _, err := c.FileAtBase("42", "7", "x.yaml"); err == nil || !strings.Contains(err.Error(), "unauthorized (401/403)") {
		t.Fatalf("FileAtBase on a forbidden read = %v, want forge.ErrUnauthorized", err)
	}
}

func TestGetMRCarriesSourceProjectID(t *testing.T) {
	c, _ := newServer(t, mrPinnedMRHandler(t, true))
	info, err := c.GetMR("42", "7")
	if err != nil {
		t.Fatal(err)
	}
	if !info.ForkMR || info.SourceProjectID != "99" {
		t.Fatalf("MRInfo must carry the fork's source repository: %+v", info)
	}
}
