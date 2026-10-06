package github

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// write_edges_test.go covers the Forge-write edges the primary tests reach
// only on their happy paths (E10-S10): the note create/edit status mappings,
// the marker-thread path derivation, and the transport failure on writes.

func TestCreateThreadRequiresFileShapedEntryRef(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pulls/7") {
			// The PR GET (the thread head pin consults the pinned read):
			// a same-repo PR.
			_, _ = io.WriteString(w, sameRepoPR)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":5,"node_id":"PRRC_5"}`)
	})
	thread, err := c.CreateThread("octo-org/base-repo", "7", threadMarker(), "body")
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if thread.Author != botUser {
		t.Fatalf("thread author = %q, want the configured identity", thread.Author)
	}

	// A non-file entryRef is a fail-closed error: the live POST cannot carry a
	// path, so a thread the port cannot address is refused, never guessed.
	marker := threadMarker()
	marker.Slot.EntryRef = "topic-registry:orders.events.v1"
	if _, err := c.CreateThread("octo-org/base-repo", "7", marker, "body"); err == nil {
		t.Fatal("a non-file entryRef must fail closed (the live POST needs a path)")
	}
}

func TestUpsertCommentRequiresSummaryKind(t *testing.T) {
	c, _ := newServer(t, unexpectedEndpoint)
	marker := threadMarker()
	marker.Artifact.Kind = "finding-thread"
	if _, err := c.UpsertComment("octo-org/base-repo", "7", marker, "body"); err == nil {
		t.Fatal("UpsertComment requires the summary-comment artifact kind")
	}
}

func TestNoteWritesStatusMapping(t *testing.T) {
	// A 500 on the create/edit endpoints is a hard error, never retried
	// (writes are never retried — E10-S05).
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := c.createNote("octo-org/base-repo", "7", threadMarker(), "body"); err == nil {
		t.Fatal("a 500 note create must error")
	}
	if _, err := c.editNote("octo-org/base-repo", "7", "issue-comment/9", threadMarker(), "body"); err == nil {
		t.Fatal("a 500 note edit must error")
	}
}

func TestListBotNotesErrorsAreNotAbsence(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	})
	if _, err := c.ListBotNotes("octo-org/base-repo", "7"); err == nil || !strings.Contains(err.Error(), "unauthorized (401/403)") {
		t.Fatalf("a forbidden note listing must carry forge.ErrUnauthorized, got %v", err)
	}
}

func TestResolveThreadRequiresParseableID(t *testing.T) {
	c, _ := newServer(t, unexpectedEndpoint)
	if err := c.ResolveThread("octo-org/base-repo", "7", "unparseable"); err == nil {
		t.Fatal("an unparseable thread id must fail closed before any request")
	}
}

func TestWarningsAreDeduplicatedAndSorted(t *testing.T) {
	c := New("https://api.github.example", "tok", "bot")
	c.warn("b second")
	c.warn("a first")
	c.warn("a first") // duplicate collapses
	got := c.Warnings()
	if len(got) != 2 || got[0] != "a first" {
		t.Fatalf("Warnings = %v, want dedup+sort", got)
	}
}

func TestReadBoundedRefusesOversized(t *testing.T) {
	if _, err := readBounded(strings.NewReader(strings.Repeat("x", maxResponseBytes+1)), maxResponseBytes); !errors.Is(err, errBodyTooLarge) {
		t.Fatalf("an over-limit body must carry errBodyTooLarge, got %v", err)
	}
	raw, err := readBounded(strings.NewReader("hello"), maxResponseBytes)
	if err != nil || string(raw) != "hello" {
		t.Fatalf("a within-limit body reads intact, got (%q, %v)", raw, err)
	}
}

func TestThreadPathFromEntryRef(t *testing.T) {
	m := threadMarker()
	path, err := threadPathFromEntryRef(m)
	if err != nil || path != "topics/orders.yaml" {
		t.Fatalf("threadPathFromEntryRef(file:...) = (%q, %v)", path, err)
	}
	m.Slot.EntryRef = "topic-registry:orders"
	if _, err := threadPathFromEntryRef(m); err == nil {
		t.Fatal("a non-file entryRef is unaddressable — the live POST needs a path")
	}
}

func TestMergeCASRefusesOnTransportError(t *testing.T) {
	// A CurrentHeads transport failure inside MergeCAS must NOT render as a
	// SHA refusal (the round-1 review's fixed bug, pinned here).
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/merge") {
			t.Fatal("MergeCAS must refuse BEFORE the PUT when the heads read fails")
		}
		w.WriteHeader(http.StatusInternalServerError)
	})
	_, err := c.MergeCAS("octo-org/base-repo", "7", forge.DesiredMerge{
		SourceSha: "src", TargetSha: "tgt", MergeResultDigest: "dig",
	})
	if err == nil {
		t.Fatal("MergeCAS on a broken forge must error")
	}
	if errors.Is(err, forge.ErrSHAMoved) {
		t.Fatal("a transport failure is not a SHA move — conflating them mislabels the refusal")
	}
}

func TestApproveStatusMapping(t *testing.T) {
	// The 401/403 row of Approve's status mapping (the sentinels the fail-
	// closed arming path surfaces).
	for _, tc := range []struct {
		status  int
		wantSub string
	}{
		{http.StatusForbidden, "unauthorized (401/403)"},
		{http.StatusUnprocessableEntity, "unexpected status 422"},
	} {
		c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/pulls/7") {
				_, _ = io.WriteString(w, sameRepoPR)
				return
			}
			w.WriteHeader(tc.status)
		})
		if _, err := c.Approve("octo-org/base-repo", "7"); err == nil {
			t.Fatalf("status %d must error", tc.status)
		} else if !strings.Contains(err.Error(), tc.wantSub) {
			t.Fatalf("status %d error = %v, want %q", tc.status, err, tc.wantSub)
		}
	}
}

func TestApproveReturnsTheReviewID(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls/7"):
			_, _ = io.WriteString(w, sameRepoPR)
		case strings.HasSuffix(r.URL.Path, "/reviews"):
			_, _ = io.WriteString(w, `{"id":77,"state":"APPROVED"}`)
		default:
			http.Error(w, "unexpected route", http.StatusInternalServerError)
		}
	})
	id, err := c.Approve("octo-org/base-repo", "7")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if id != "review/77" {
		t.Fatalf("approve id = %q, want review/77 (the forge-assigned review id)", id)
	}
}
