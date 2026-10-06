package github

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// pin_test.go — the evaluation-pin discipline (E10 branch-review round 2,
// findings 2/6): the pin is written by GetMR with FIRST WRITE WINS, so the
// CAS's CurrentHeads re-read returns the forge's CURRENT heads without moving
// the pin the evaluation judged at. The governed reads and Approve's commit_id
// keep following the PINNED (evaluated) SHA even after the heads moved.

// prWithHead is sameRepoPR with the head SHA replaced — the moved-head shape
// the second PR read serves.
func prWithHead(sha string) string {
	return `{"number":7,"sha":"` + sha + `","user":{"login":"octocat"},"labels":[],` +
		`"base":{"ref":"main","sha":"tgtTIP","repo":{"full_name":"octo-org/base-repo"}},` +
		`"head":{"ref":"feature","sha":"` + sha + `","repo":{"full_name":"octo-org/base-repo"}},"mergeable_state":"clean"}`
}

// TestCurrentHeadsReReadDoesNotMoveTheEvaluationPin: GetMR pins head A;
// CurrentHeads re-reads the forge at head B; the governed reads (FileAtBase/
// FileAtHead) still read at A — the evaluation pin — and Approve's commit_id
// is the PINNED (evaluated) SHA, not the re-read one. The content handler
// serves only at the pinned SHAs, so a read at the moved head would 404 and
// the assertion would fail loudly rather than silently.
func TestCurrentHeadsReReadDoesNotMoveTheEvaluationPin(t *testing.T) {
	prReads := 0
	approveCommitID := ""
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
			prReads++
			if prReads == 1 {
				_, _ = io.WriteString(w, sameRepoPR) // the evaluation read: head A
			} else {
				_, _ = io.WriteString(w, prWithHead("srcMoved")) // the CAS re-read: head B
			}
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/contents":
			_, _ = io.WriteString(w, "[]") // the content-scope probe
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/repos/octo-org/base-repo/contents/"):
			switch r.URL.Query().Get("ref") {
			case "tgtTIP":
				_, _ = io.WriteString(w, `{"content":"YmFzZSBieXRlcw==","encoding":"base64","size":11}`)
			case "srcSHA":
				_, _ = io.WriteString(w, `{"content":"aGVhZCBieXRlcw==","encoding":"base64","size":11}`)
			default:
				http.Error(w, "unexpected ref "+r.URL.Query().Get("ref"), http.StatusNotFound)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/git/ref/refs/pull/7/merge":
			_, _ = io.WriteString(w, `{"object":{"sha":"mrgSHA","type":"commit"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/reviews":
			var posted map[string]string
			_ = json.NewDecoder(r.Body).Decode(&posted)
			approveCommitID = posted["commit_id"]
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":9001,"state":"APPROVED"}`)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	info, err := c.GetMR(baseRepo, "7")
	if err != nil {
		t.Fatalf("GetMR (the evaluation read): %v", err)
	}
	if info.SourceSHA != "srcSHA" {
		t.Fatalf("evaluation read head = %q, want srcSHA", info.SourceSHA)
	}

	// The CAS re-read moves the forge's CURRENT heads (srcMoved) — but must not
	// move the evaluation pin.
	source, _, _, err := c.CurrentHeads(baseRepo, "7")
	if err != nil {
		t.Fatalf("CurrentHeads: %v", err)
	}
	if source != "srcMoved" {
		t.Fatalf("CurrentHeads source = %q, want the forge's current head srcMoved", source)
	}

	// The governed reads follow the PIN, not the re-read: FileAtHead still reads
	// at the evaluated srcSHA (the harness serves head content ONLY at srcSHA,
	// so a pin that followed the re-read would 404), and FileAtBase at the
	// pinned target tip.
	if _, err := c.FileAtHead(baseRepo, "7", "topics/orders.yaml"); err != nil {
		t.Fatalf("FileAtHead must read at the PINNED (evaluated) head srcSHA, not the moved one: %v", err)
	}
	if _, err := c.FileAtBase(baseRepo, "7", "topics/orders.yaml"); err != nil {
		t.Fatalf("FileAtBase must read at the pinned target tip: %v", err)
	}

	// Approve's commit_id is the PINNED (evaluated) SHA, even after the heads
	// moved under the CAS re-read.
	if _, err := c.Approve(baseRepo, "7"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if approveCommitID != "srcSHA" {
		t.Fatalf("Approve commit_id = %q, want the PINNED (evaluated) head srcSHA even after the heads moved", approveCommitID)
	}
}
