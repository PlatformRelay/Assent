package gitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// AUD-S10 (audit findings REL-03 / SEC-08): every forge response read is
// BOUNDED and every pagination loop is CAPPED. Both are availability guards on
// the write edge, and both are FAIL-CLOSED: an over-limit body is an error (the
// truncated prefix is never parsed as if it were the complete document), and a
// paginator that never shortens its pages errors at the cap instead of spinning
// (an incomplete thread/note list must never drive reconcile).
//
// The tests below drive the REAL client methods (GetMR / FileAtRef /
// ListBotThreads / ListBotNotes) against an httptest server, not the private
// helpers, so an unwired limit fails them.

// writeFiller writes exactly n bytes of filler to w.
func writeFiller(w io.Writer, n int) {
	const chunk = 64 << 10
	buf := strings.Repeat("a", chunk)
	for n > 0 {
		if n < chunk {
			_, _ = io.WriteString(w, buf[:n])
			return
		}
		_, _ = io.WriteString(w, buf)
		n -= chunk
	}
}

// TestBoundedReadOverLimitFailsClosed — REQ-AUD-S10-01 (fail-closed half): a
// response body larger than maxResponseBytes errors, naming the limit; the
// truncated prefix is NEVER handed to the JSON decoder.
func TestBoundedReadOverLimitFailsClosed(t *testing.T) {
	// A JSON string field padded past the limit: a truncating read would yield
	// invalid JSON, so the test also asserts the error is the LIMIT error and
	// not an incidental decode failure.
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"sha":"`)
		writeFiller(w, maxResponseBytes)
		_, _ = io.WriteString(w, `","source_branch":"f","target_branch":"main"}`)
	})

	_, err := c.GetMR("42", "7")
	if err == nil {
		t.Fatal("over-limit response body must fail closed, got nil error")
	}
	if !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("error must name the byte limit, got %v", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprint(maxResponseBytes)) {
		t.Fatalf("error must state the limit %d, got %v", maxResponseBytes, err)
	}
	if strings.Contains(err.Error(), "decode") {
		t.Fatalf("truncated bytes must never reach the decoder, got %v", err)
	}
}

// TestBoundedReadAtLimitStillSucceeds is the POSITIVE CONTROL for the bound: a
// body of exactly maxResponseBytes is legitimate traffic and must still parse.
// Without this, shrinking the limit to zero would pass the fail-closed test.
func TestBoundedReadAtLimitStillSucceeds(t *testing.T) {
	head := `{"iid":7,"project_id":42,"sha":"`
	tail := `","source_branch":"f","target_branch":"main"}`
	pad := maxResponseBytes - len(head) - len(tail)
	if pad <= 0 {
		t.Fatalf("maxResponseBytes %d is too small for this fixture", maxResponseBytes)
	}

	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/branches/") {
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtTIP"}}`)
			return
		}
		_, _ = io.WriteString(w, head)
		writeFiller(w, pad)
		_, _ = io.WriteString(w, tail)
	})

	info, err := c.GetMR("42", "7")
	if err != nil {
		t.Fatalf("a body of exactly the limit is legitimate traffic: %v", err)
	}
	if len(info.SourceSHA) != pad {
		t.Fatalf("body truncated: SourceSHA len = %d, want %d", len(info.SourceSHA), pad)
	}
}

// TestBoundedReadAppliesToRawFileReads pins that the bound sits at the shared
// `do` seam, so the raw-file read (which returns bytes rather than JSON) is
// bounded too — a hostile repository blob cannot OOM the run.
func TestBoundedReadAppliesToRawFileReads(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeFiller(w, maxResponseBytes+1)
	})
	if _, err := c.FileAtRef("42", "policy.yaml", "main"); err == nil {
		t.Fatal("over-limit raw file read must fail closed")
	} else if !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("error must name the byte limit, got %v", err)
	}
}

// TestPaginationCapFailsClosed — REQ-AUD-S10-02 (fail-closed half): a paginator
// that never returns a short page errors at maxListPages for BOTH list loops.
// Reconcile therefore never proceeds on a partial list.
func TestPaginationCapFailsClosed(t *testing.T) {
	rendered, err := renderMarker(botMarker())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("discussions", func(t *testing.T) {
		pages := 0
		c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
			pages++
			writeFullDiscussionPage(w, rendered)
		})
		_, err := c.ListBotThreads("42", "7")
		if err == nil {
			t.Fatal("a never-shortening discussions paginator must fail closed at the cap")
		}
		if !strings.Contains(err.Error(), "pagination cap") {
			t.Fatalf("error must name the pagination cap, got %v", err)
		}
		if pages != maxListPages {
			t.Fatalf("paginator must stop at exactly %d pages, made %d requests", maxListPages, pages)
		}
	})

	t.Run("notes", func(t *testing.T) {
		pages := 0
		c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
			pages++
			writeFullNotePage(w, rendered)
		})
		_, err := c.ListBotNotes("42", "7")
		if err == nil {
			t.Fatal("a never-shortening notes paginator must fail closed at the cap")
		}
		if !strings.Contains(err.Error(), "pagination cap") {
			t.Fatalf("error must name the pagination cap, got %v", err)
		}
		if pages != maxListPages {
			t.Fatalf("paginator must stop at exactly %d pages, made %d requests", maxListPages, pages)
		}
	})
}

// TestPaginationCapFailsClosedApprovalRules extends the REL-03 cap to the third
// (and last) uncapped page loop in the adapter: the approval-rules capability
// probe. The AUD-S10 REQ names only the discussion/note loops, but the finding
// is "every pagination loop capped", and an uncapped probe spins forever on a
// broken paginator. Erroring is strictly safer than the previous behaviour: the
// run aborts with ZERO forge writes instead of hanging.
//
// The 404/403 → Free-tier FAIL-SAFE is deliberately untouched (a cap hit is a
// forge anomaly, not "this instance has no approval-rules API"); the positive
// control below and TestSnapshotCapabilities pin that.
func TestPaginationCapFailsClosedApprovalRules(t *testing.T) {
	const rulesPath = "/api/v4/projects/42/merge_requests/7/approval_rules"
	base := snapshotHandler(t, renameCassette())

	pages := 0
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != rulesPath {
			base(w, r)
			return
		}
		// A page that is always FULL and always uninteresting (no rule requires
		// approvals) — the probe can neither conclude nor terminate.
		pages++
		var full []map[string]any
		for i := 0; i < listPerPage; i++ {
			full = append(full, map[string]any{"id": i, "name": "noise", "approvals_required": 0})
		}
		_ = json.NewEncoder(w).Encode(full)
	})

	_, err := c.Snapshot(snapshotProject, snapshotMR)
	if err == nil {
		t.Fatal("a never-shortening approval-rules paginator must fail closed at the cap")
	}
	if !strings.Contains(err.Error(), "pagination cap") {
		t.Fatalf("error must name the pagination cap, got %v", err)
	}
	if pages != maxListPages {
		t.Fatalf("probe must stop at exactly %d pages, made %d requests", maxListPages, pages)
	}
}

// TestApprovalRulesFailSafeSurvivesCap is the POSITIVE CONTROL for the probe's
// pre-existing fail-safe: a 403/404 still degrades to Free tier (no invented
// Premium features) rather than erroring. Without it, capping the loop could
// silently turn every unavailable approval-rules API into a hard run failure.
func TestApprovalRulesFailSafeSurvivesCap(t *testing.T) {
	const rulesPath = "/api/v4/projects/42/merge_requests/7/approval_rules"
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			base := snapshotHandler(t, renameCassette())
			c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == rulesPath {
					http.Error(w, "nope", status)
					return
				}
				base(w, r)
			})
			snap, err := c.Snapshot(snapshotProject, snapshotMR)
			if err != nil {
				t.Fatalf("approval-rules %d must fail SAFE to Free tier, not error: %v", status, err)
			}
			if snap.Capabilities.HasApprovalRulesAPI {
				t.Fatal("approval-rules probe must report false when the API is unavailable")
			}
		})
	}
}

// TestPaginationBelowCapUnchanged is the POSITIVE CONTROL for the cap: a
// multi-page listing that terminates with a short page still returns every
// artifact. Without it, a cap of 1 would pass TestPaginationCapFailsClosed.
func TestPaginationBelowCapUnchanged(t *testing.T) {
	rendered, err := renderMarker(botMarker())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("discussions", func(t *testing.T) {
		c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("page") == "1" {
				writeFullDiscussionPage(w, rendered)
				return
			}
			_, _ = io.WriteString(w, fmt.Sprintf(
				`[{"id":"tail","notes":[{"body":%q,"resolved":false,"author":{"username":%q}}]}]`,
				rendered, botUser))
		})
		threads, err := c.ListBotThreads("42", "7")
		if err != nil {
			t.Fatalf("normal pagination must be unaffected by the cap: %v", err)
		}
		if len(threads) != listPerPage+1 {
			t.Fatalf("got %d threads, want %d", len(threads), listPerPage+1)
		}
	})

	t.Run("notes", func(t *testing.T) {
		c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("page") == "1" {
				writeFullNotePage(w, rendered)
				return
			}
			_, _ = io.WriteString(w, fmt.Sprintf(
				`[{"id":999999,"body":%q,"author":{"username":%q}}]`, rendered, botUser))
		})
		notes, err := c.ListBotNotes("42", "7")
		if err != nil {
			t.Fatalf("normal pagination must be unaffected by the cap: %v", err)
		}
		if len(notes) != listPerPage+1 {
			t.Fatalf("got %d notes, want %d", len(notes), listPerPage+1)
		}
	})
}

// writeFullDiscussionPage emits a FULL page (listPerPage entries) of
// bot-authored discussions — the shape that makes the client ask for one more.
func writeFullDiscussionPage(w io.Writer, body string) {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < listPerPage; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"d%d","notes":[{"body":%q,"resolved":false,"author":{"username":%q}}]}`,
			i, body, botUser)
	}
	b.WriteByte(']')
	_, _ = io.WriteString(w, b.String())
}

// writeFullNotePage emits a FULL page (listPerPage entries) of bot-authored MR
// notes.
func writeFullNotePage(w io.Writer, body string) {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < listPerPage; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":%d,"body":%q,"author":{"username":%q}}`, i+1, body, botUser)
	}
	b.WriteByte(']')
	_, _ = io.WriteString(w, b.String())
}
