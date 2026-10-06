package gitlab

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// pin_test.go — the evaluation-pin discipline (E10 branch-review round 2,
// findings 2/6): the pin is written by GetMR with FIRST WRITE WINS, so the
// CAS's CurrentHeads re-read returns the forge's CURRENT heads without moving
// the pin the evaluation judged at. The MR-relative governed reads keep
// reading at the PINNED (evaluated) SHAs even after the heads moved.

// TestCurrentHeadsReReadDoesNotMoveTheEvaluationPin: GetMR pins head A;
// CurrentHeads re-reads the forge at head B; FileAtBase/FileAtHead still read
// at the pinned SHAs. The file handler serves only at the pinned SHAs, so a
// read at the moved head would 404 and the assertion would fail loudly.
func TestCurrentHeadsReReadDoesNotMoveTheEvaluationPin(t *testing.T) {
	mrReads := 0
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7":
			mrReads++
			sha := "srcSHA"
			if mrReads > 1 {
				sha = "srcMoved"
			}
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"source_project_id":42,"sha":"`+sha+
				`","source_branch":"feature","target_branch":"main"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtTIP"}}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/repository/files/") && strings.HasSuffix(r.URL.Path, "/raw"):
			switch r.URL.Query().Get("ref") {
			case "tgtTIP":
				_, _ = io.WriteString(w, "base bytes")
			case "srcSHA":
				_, _ = io.WriteString(w, "head bytes")
			default:
				http.Error(w, "unexpected ref "+r.URL.Query().Get("ref"), http.StatusNotFound)
			}
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	info, err := c.GetMR("42", "7")
	if err != nil {
		t.Fatalf("GetMR (the evaluation read): %v", err)
	}
	if info.SourceSHA != "srcSHA" {
		t.Fatalf("evaluation read head = %q, want srcSHA", info.SourceSHA)
	}

	// The CAS re-read moves the forge's CURRENT heads — but must not move the
	// evaluation pin.
	source, target, digest, err := c.CurrentHeads("42", "7")
	if err != nil {
		t.Fatalf("CurrentHeads: %v", err)
	}
	if source != "srcMoved" {
		t.Fatalf("CurrentHeads source = %q, want the forge's current head srcMoved", source)
	}
	if target != "tgtTIP" || digest == "" {
		t.Fatalf("CurrentHeads = %q/%q, want the current target tip and the synthesised digest", source, target)
	}

	// The governed reads follow the PIN, not the re-read: FileAtHead still reads
	// at the evaluated srcSHA (the harness serves head content ONLY at srcSHA,
	// so a pin that followed the re-read would 404), and FileAtBase at the
	// pinned target tip.
	if _, err := c.FileAtHead("42", "7", "topics/orders.yaml"); err != nil {
		t.Fatalf("FileAtHead must read at the PINNED (evaluated) head srcSHA, not the moved one: %v", err)
	}
	if _, err := c.FileAtBase("42", "7", "topics/orders.yaml"); err != nil {
		t.Fatalf("FileAtBase must read at the pinned target tip: %v", err)
	}
}
