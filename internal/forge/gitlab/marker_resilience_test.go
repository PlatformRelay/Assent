package gitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/render"
)

// AUD-S12 (audit finding REL-06) — ⚠️ RECONCILE-PROTOCOL BEHAVIOUR CHANGE
// against ADR-0019. Before this story a single bot note whose marker JSON had
// been corrupted made ListBotThreads/ListBotNotes return a hard error, so every
// subsequent reconcile on that MR failed until a human deleted the note: it
// failed closed, but it BRICKED the MR.
//
// Now a bot-authored note with a malformed marker is SKIPPED — treated as
// not-a-slot-note — with a warning carried on the PublicationReceipt, and
// reconcile proceeds. The worst case is that the slot is re-posted ONCE, after
// which every later run reuses the healthy artifact. Step-8 duplicate-repair
// plays no part — the skipped artifact is absent from the listing, so it is
// never a visible duplicate (proved by
// TestMalformedBotThreadConvergesWithoutDuplicateRepair). A wrongly-parsed
// marker can never approve anything either way, because markers are correlation
// metadata only and never decision input.
//
// The AUTHOR-IDENTITY filter is UNTOUCHED and runs BEFORE the marker is even
// looked at: a contributor note is invisible whether its marker is perfect or
// garbage, so the spoof surface is unchanged. TestContributorMarkersAreInvisible
// below and TestSpoofedMarkerStillIgnored in the conformance suite pin that in
// both polarities.

// markerFixture is one seeded discussion or note in the fake GitLab.
type markerFixture struct {
	id     string
	author string
	body   string
}

// malformedMarkerBody is a note body carrying the real marker SENTINEL with a
// payload that is syntactically JSON-shaped (so the extraction regexp matches)
// but semantically undecodable — the exact corruption REL-06 describes.
func malformedMarkerBody() string {
	return "<!-- " + render.MarkerSentinel + ` {"slot": 12345} --># heading` + "\n\nbody"
}

func healthyBody(t *testing.T, m forge.Marker) string {
	t.Helper()
	body, err := render.Envelope(m, "seed body")
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// markerHarness is a fake GitLab exposing only what Reconcile touches, plus
// counters for the writes that must (or must not) happen.
type markerHarness struct {
	project, mr string
	discussions []markerFixture
	notes       []markerFixture

	discussionPosts int
	notePosts       int
	notePuts        int
	nextID          int
}

func (h *markerHarness) client(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(h.handle))
	t.Cleanup(srv.Close)
	return New(srv.URL, "test-token", botUser, WithSleeper(func(time.Duration) {}))
}

func (h *markerHarness) handle(w http.ResponseWriter, r *http.Request) {
	base := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s", h.project, h.mr)
	switch {
	case r.Method == http.MethodGet && r.URL.Path == base+"/discussions":
		h.serveList(w, r, h.discussions, discussionJSON)
	case r.Method == http.MethodPost && r.URL.Path == base+"/discussions":
		h.discussionPosts++
		h.nextID++
		id := fmt.Sprintf("d-new-%d", h.nextID)
		h.discussions = append(h.discussions, markerFixture{id: id, author: botUser, body: formBody(r)})
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, fmt.Sprintf(`{"id":%q}`, id))
	case r.Method == http.MethodGet && r.URL.Path == base+"/notes":
		h.serveList(w, r, h.notes, noteJSON)
	case r.Method == http.MethodPost && r.URL.Path == base+"/notes":
		h.notePosts++
		h.nextID++
		id := fmt.Sprintf("%d", 500+h.nextID)
		h.notes = append(h.notes, markerFixture{id: id, author: botUser, body: formBody(r)})
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, fmt.Sprintf(`{"id":%s}`, id))
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, base+"/notes/"):
		h.notePuts++
		id := strings.TrimPrefix(r.URL.Path, base+"/notes/")
		for i := range h.notes {
			if h.notes[i].id == id {
				h.notes[i].body = formBody(r)
			}
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
	}
}

func (h *markerHarness) serveList(w http.ResponseWriter, r *http.Request,
	items []markerFixture, enc func(markerFixture) map[string]any,
) {
	if r.URL.Query().Get("page") != "1" {
		_, _ = io.WriteString(w, `[]`)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, enc(it))
	}
	_ = json.NewEncoder(w).Encode(out)
}

func discussionJSON(f markerFixture) map[string]any {
	return map[string]any{
		"id": f.id,
		"notes": []map[string]any{{
			"body":     f.body,
			"resolved": false,
			"author":   map[string]any{"username": f.author},
		}},
	}
}

func noteJSON(f markerFixture) map[string]any {
	var id int
	_, _ = fmt.Sscanf(f.id, "%d", &id)
	return map[string]any{
		"id":     id,
		"body":   f.body,
		"author": map[string]any{"username": f.author},
	}
}

func formBody(r *http.Request) string {
	raw, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(raw))
	return form.Get("body")
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC) }

func threadMarker(project, mr string) forge.Marker {
	m := botMarker()
	m.Slot.Project = project
	m.Slot.MR = mr
	return m
}

// TestMalformedBotMarkerSkipsWithWarning — REQ-AUD-S12-01. A bot note whose
// marker JSON is corrupted sits alongside a healthy slot. Reconcile COMPLETES,
// the malformed note is skipped, a warning reaches the receipt, and the healthy
// slot reconciles exactly as it would without the corruption.
func TestMalformedBotMarkerSkipsWithWarning(t *testing.T) {
	const project, mr = "42", "7"
	m := threadMarker(project, mr)

	h := &markerHarness{
		project: project, mr: mr,
		discussions: []markerFixture{
			{id: "d-healthy", author: botUser, body: healthyBody(t, m)},
			{id: "d-corrupt", author: botUser, body: malformedMarkerBody()},
		},
	}
	c := h.client(t)

	receipt, err := forge.Reconcile(c, fixedClock{}, forge.DesiredReviewState{
		Project: project, MR: mr,
		Thread: &forge.DesiredThread{Marker: m, Body: "finding body"},
	}, forge.Preconditions{})
	if err != nil {
		t.Fatalf("a malformed bot marker must NOT brick reconcile: %v", err)
	}

	// The healthy slot is reused in place — the corruption changes nothing here.
	if len(receipt.Operations) != 1 || receipt.Operations[0].TargetID != "d-healthy" {
		t.Fatalf("healthy slot must reconcile unchanged, got %+v", receipt.Operations)
	}
	if h.discussionPosts != 0 {
		t.Fatalf("the existing healthy thread must be reused, %d thread(s) posted", h.discussionPosts)
	}

	// ... and the skip is SURFACED, not silent.
	if len(receipt.Warnings) != 1 {
		t.Fatalf("the skip must surface exactly one warning, got %v", receipt.Warnings)
	}
	warning := receipt.Warnings[0]
	if !strings.Contains(warning, "d-corrupt") {
		t.Fatalf("the warning must name the skipped artifact, got %q", warning)
	}
	if !strings.Contains(warning, "malformed marker") {
		t.Fatalf("the warning must say why the artifact was skipped, got %q", warning)
	}
}

// TestHealthyReconcileEmitsNoWarning is the POSITIVE CONTROL for the warning
// channel: without the corruption the receipt carries NO warnings, so the field
// cannot be a constant and no golden receipt gains a spurious entry.
func TestHealthyReconcileEmitsNoWarning(t *testing.T) {
	const project, mr = "42", "7"
	m := threadMarker(project, mr)

	h := &markerHarness{
		project: project, mr: mr,
		discussions: []markerFixture{{id: "d-healthy", author: botUser, body: healthyBody(t, m)}},
	}
	receipt, err := forge.Reconcile(h.client(t), fixedClock{}, forge.DesiredReviewState{
		Project: project, MR: mr,
		Thread: &forge.DesiredThread{Marker: m, Body: "finding body"},
	}, forge.Preconditions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Warnings) != 0 {
		t.Fatalf("a clean reconcile must carry no warnings, got %v", receipt.Warnings)
	}
	if len(receipt.Operations) != 1 || receipt.Operations[0].TargetID != "d-healthy" {
		t.Fatalf("operations = %+v", receipt.Operations)
	}
}

// TestMalformedBotMarkerDoubleRunConverges — REQ-AUD-S12-02 (convergence half).
//
// Duplicate-repair covers NEITHER artifact kind here — see
// TestMalformedBotThreadConvergesWithoutDuplicateRepair for the thread case.
// For a SUMMARY note there is the additional reason that the protocol never
// deletes notes (write minimisation, "not in scope" of this story).
// Convergence comes from the upsert itself: run 1 cannot see the corrupt note,
// so it posts one healthy summary; run 2 finds THAT one and edits it in place.
// Exactly one junk note lingers, no new duplicate is ever created, and the
// warning is stable across runs — so a double run is byte-identical.
func TestMalformedBotMarkerDoubleRunConverges(t *testing.T) {
	const project, mr = "42", "7"
	sm := summaryMarkerFor(project, mr)
	tm := threadMarker(project, mr)

	h := &markerHarness{
		project: project, mr: mr,
		discussions: []markerFixture{{id: "d-healthy", author: botUser, body: healthyBody(t, tm)}},
		notes:       []markerFixture{{id: "900", author: botUser, body: malformedMarkerBody()}},
	}
	c := h.client(t)

	desired := forge.DesiredReviewState{
		Project: project, MR: mr,
		Thread:  &forge.DesiredThread{Marker: tm, Body: "finding body"},
		Summary: &forge.DesiredSummary{Marker: sm, Body: "summary body"},
	}

	first, err := forge.Reconcile(c, fixedClock{}, desired, forge.Preconditions{})
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if h.notePosts != 1 {
		t.Fatalf("run 1 must post exactly one replacement summary, posted %d", h.notePosts)
	}

	second, err := forge.Reconcile(c, fixedClock{}, desired, forge.Preconditions{})
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if h.notePosts != 1 {
		t.Fatalf("run 2 must create NOTHING — total note posts %d, want 1", h.notePosts)
	}
	if h.notePuts == 0 {
		t.Fatal("run 2 must edit the healthy summary in place")
	}
	if h.discussionPosts != 0 {
		t.Fatalf("no thread may be created in either run, posted %d", h.discussionPosts)
	}

	if fmt.Sprintf("%+v", first) != fmt.Sprintf("%+v", second) {
		t.Fatalf("double run must be stable:\n run1 = %+v\n run2 = %+v", first, second)
	}
	if len(second.Warnings) != 1 {
		t.Fatalf("the lingering corrupt note must keep warning, got %v", second.Warnings)
	}
}

// TestMalformedBotThreadConvergesWithoutDuplicateRepair — review finding F8.
//
// It is TEMPTING to say a skipped THREAD converges via step-8 duplicate-repair.
// It does not, and this test is the proof rather than the argument: a corrupt
// thread is filtered out of ListBotThreads, so it can never present as a
// VISIBLE duplicate for repair to act on. `repairs` stays EMPTY on every run.
//
// Convergence is real, but it is the same no-new-duplicate/reuse mechanism that
// governs summary notes: run 1 posts one healthy artifact for the slot, and
// every later run finds and reuses THAT one. The corrupt artifact lingers,
// warning, until an operator deletes it.
//
// ADR-0019's amendment and reconciliation-state-table.md must state this
// mechanism, not the repair one — a frozen normative contract that names the
// wrong mechanism misleads the next implementer even though behaviour is fine.
func TestMalformedBotThreadConvergesWithoutDuplicateRepair(t *testing.T) {
	const project, mr = "42", "7"
	tm := threadMarker(project, mr)

	h := &markerHarness{
		project: project, mr: mr,
		// The ONLY bot artifact for this slot is corrupt: nothing healthy to reuse.
		discussions: []markerFixture{{id: "d-corrupt", author: botUser, body: malformedMarkerBody()}},
	}
	c := h.client(t)
	desired := forge.DesiredReviewState{
		Project: project, MR: mr,
		Thread: &forge.DesiredThread{Marker: tm, Body: "finding body"},
	}

	first, err := forge.Reconcile(c, fixedClock{}, desired, forge.Preconditions{})
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if h.discussionPosts != 1 {
		t.Fatalf("run 1 must post exactly one healthy thread, posted %d", h.discussionPosts)
	}
	if len(first.Repairs) != 0 {
		t.Fatalf("run 1: duplicate-repair must NOT fire — the corrupt thread is "+
			"invisible to the listing, so no duplicate is visible; got %+v", first.Repairs)
	}

	second, err := forge.Reconcile(c, fixedClock{}, desired, forge.Preconditions{})
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}
	// THE LOAD-BEARING ASSERTION: convergence is reuse, not repair.
	if h.discussionPosts != 1 {
		t.Fatalf("run 2 must REUSE the healthy thread, not post again — total posts %d, want 1",
			h.discussionPosts)
	}
	if len(second.Repairs) != 0 {
		t.Fatalf("run 2: duplicate-repair still must NOT fire; got %+v", second.Repairs)
	}
	if len(second.Operations) != 1 || second.Operations[0].TargetID != "d-new-1" {
		t.Fatalf("run 2 must target the healthy thread run 1 created, got %+v", second.Operations)
	}
	if len(second.Warnings) != 1 {
		t.Fatalf("the lingering corrupt thread must keep warning, got %v", second.Warnings)
	}
	if fmt.Sprintf("%+v", first) != fmt.Sprintf("%+v", second) {
		t.Fatalf("double run must be stable:\n run1 = %+v\n run2 = %+v", first, second)
	}

	// The evidence the corrected contract text rests on, visible under -v.
	// (That `repairs` CAN be non-empty is pinned separately by the conformance
	// suite's TestConformanceDuplicateRepair — so "empty" here is a finding,
	// not an artefact of repair being unreachable in general.)
	t.Logf("run1: discussionPosts=1 repairs=%v | run2: discussionPosts=%d repairs=%v",
		first.Repairs, h.discussionPosts, second.Repairs)
}

// TestContributorMarkersAreInvisible — REQ-AUD-S12-02 (spoof half), in the
// adapter. The author-identity filter runs BEFORE the marker is parsed, so a
// contributor note is invisible either way. The malformed case additionally
// asserts NO WARNING: warning about a contributor note would leak the fact that
// the filter had reached the marker at all, and would let anyone spam the
// receipt.
func TestContributorMarkersAreInvisible(t *testing.T) {
	const project, mr = "42", "7"
	m := threadMarker(project, mr)

	for _, tc := range []struct{ name, body string }{
		{"well_formed", healthyBody(t, m)},
		{"malformed", malformedMarkerBody()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &markerHarness{
				project: project, mr: mr,
				discussions: []markerFixture{{id: "d-spoof", author: "mallory", body: tc.body}},
			}
			c := h.client(t)

			threads, err := c.ListBotThreads(project, mr)
			if err != nil {
				t.Fatalf("a contributor note must never error reconcile: %v", err)
			}
			if len(threads) != 0 {
				t.Fatalf("contributor note must be invisible, got %+v", threads)
			}
			if got := c.Warnings(); len(got) != 0 {
				t.Fatalf("a contributor note must produce no warning, got %v", got)
			}

			receipt, err := forge.Reconcile(c, fixedClock{}, forge.DesiredReviewState{
				Project: project, MR: mr,
				Thread: &forge.DesiredThread{Marker: m, Body: "finding body"},
			}, forge.Preconditions{})
			if err != nil {
				t.Fatal(err)
			}
			// The spoof occupies no slot: the bot posts its OWN thread.
			if h.discussionPosts != 1 {
				t.Fatalf("the spoofed note must not satisfy the slot, posts = %d", h.discussionPosts)
			}
			if len(receipt.Warnings) != 0 {
				t.Fatalf("contributor notes must never reach the warning channel, got %v", receipt.Warnings)
			}
		})
	}
}

// TestWarningsAreDedupedAndSorted pins the determinism of the warning channel:
// Reconcile rescans (step 9), so the same corrupt note is seen more than once
// per run, and the receipt must not grow. Sorted + deduped keeps a double run
// byte-identical, which `task determinism` checks.
func TestWarningsAreDedupedAndSorted(t *testing.T) {
	c := New("https://gitlab.example", "tok", botUser)
	c.warn("b second")
	c.warn("a first")
	c.warn("b second")

	got := c.Warnings()
	want := []string{"a first", "b second"}
	if len(got) != len(want) {
		t.Fatalf("warnings = %v, want %v (deduped)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("warnings = %v, want %v (sorted)", got, want)
		}
	}
}
