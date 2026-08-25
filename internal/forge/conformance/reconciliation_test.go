package conformance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
	gitlab "github.com/PlatformRelay/assent/internal/forge/gitlab"
	"github.com/PlatformRelay/assent/internal/render"
)

// ---- GitLab httptest harness (list/create/resolve path for conformance) ----

type gitlabDiscussion struct {
	id       string
	body     string
	resolved bool
	author   string
}

type gitlabNote struct {
	id     int
	body   string
	author string
}

type gitlabHarness struct {
	project         string
	mr              string
	botAuthor       string
	discussions     []gitlabDiscussion
	notes           []gitlabNote
	nextID          int
	createCalls     int
	resolveCalls    int
	noteCreateCalls int
	noteUpdateCalls int

	// E10-S01: MR heads + merge/approve state, so the SHA-guard cases can run
	// against GitLab instead of being fake-only.
	sourceSHA    string
	targetSHA    string
	mrReads      int
	approveCalls int
	mergePUTs    int

	// afterMRRead fires once an MR read has been SERVED — the TOCTOU seam.
	afterMRRead func(h *gitlabHarness)
}

func newGitLabHarness(project, mr string) *gitlabHarness {
	return &gitlabHarness{project: project, mr: mr, botAuthor: botID, nextID: 9000}
}

func (h *gitlabHarness) seed(id, author string, marker forge.Marker, resolved bool) error {
	body, err := render.Envelope(marker, "seed")
	if err != nil {
		return err
	}
	h.discussions = append(h.discussions, gitlabDiscussion{id: id, body: body, resolved: resolved, author: author})
	return nil
}

func (h *gitlabHarness) seedNote(id int, author string, marker forge.Marker, body string) error {
	fullBody, err := render.Envelope(marker, body)
	if err != nil {
		return err
	}
	h.notes = append(h.notes, gitlabNote{id: id, body: fullBody, author: author})
	return nil
}

func (h *gitlabHarness) client(t interface {
	Helper()
	Cleanup(func())
}) *gitlab.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(h.handle))
	t.Cleanup(srv.Close)
	return gitlab.New(srv.URL, "test-token", h.botAuthor,
		gitlab.WithSleeper(func(time.Duration) {}))
}

func (h *gitlabHarness) handle(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get("PRIVATE-TOKEN"); got != "test-token" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	mrBase := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s",
		url.PathEscape(h.project), url.PathEscape(h.mr))
	branchBase := fmt.Sprintf("/api/v4/projects/%s/repository/branches/",
		url.PathEscape(h.project))
	escapedBase := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/discussions",
		url.PathEscape(h.project), url.PathEscape(h.mr))
	notesBase := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/notes",
		url.PathEscape(h.project), url.PathEscape(h.mr))
	path := r.URL.EscapedPath()
	switch {
	case r.Method == http.MethodGet && path == escapedBase:
		h.serveDiscussions(w, r)
	case r.Method == http.MethodPost && path == escapedBase:
		h.createDiscussion(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(path, escapedBase+"/"):
		h.resolveDiscussion(w, r, path, escapedBase)
	case r.Method == http.MethodGet && path == notesBase:
		h.serveNotes(w, r)
	case r.Method == http.MethodPost && path == notesBase:
		h.createNote(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(path, notesBase+"/"):
		h.updateNote(w, r, path, notesBase)
	case r.Method == http.MethodGet && path == mrBase:
		h.serveMR(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(path, branchBase):
		h.serveBranch(w, r)
	case r.Method == http.MethodPost && path == mrBase+"/approve":
		h.approve(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(path, mrBase+"/merge"):
		h.merge(w, r)
	default:
		http.Error(w, "unexpected "+r.Method+" "+path, http.StatusInternalServerError)
	}
}

func (h *gitlabHarness) serveDiscussions(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	perPage := 100
	start := (page - 1) * perPage
	if start >= len(h.discussions) {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	end := start + perPage
	if end > len(h.discussions) {
		end = len(h.discussions)
	}
	out := make([]map[string]any, 0, end-start)
	for _, d := range h.discussions[start:end] {
		out = append(out, map[string]any{
			"id": d.id,
			"notes": []map[string]any{{
				"body":     d.body,
				"resolved": d.resolved,
				"author":   map[string]any{"username": d.author},
			}},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (h *gitlabHarness) createDiscussion(w http.ResponseWriter, r *http.Request) {
	h.createCalls++
	body, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(body))
	h.nextID++
	id := fmt.Sprintf("note/%d", h.nextID)
	h.discussions = append(h.discussions, gitlabDiscussion{
		id: id, body: form.Get("body"), resolved: false, author: h.botAuthor,
	})
	w.WriteHeader(http.StatusCreated)
	_, _ = io.WriteString(w, fmt.Sprintf(`{"id":%q}`, id))
}

func (h *gitlabHarness) resolveDiscussion(w http.ResponseWriter, r *http.Request, path, escapedBase string) {
	if r.URL.Query().Get("resolved") != "true" {
		http.Error(w, "missing resolved=true", http.StatusBadRequest)
		return
	}
	h.resolveCalls++
	rawID := strings.TrimPrefix(path, escapedBase+"/")
	id, err := url.PathUnescape(rawID)
	if err != nil {
		http.Error(w, "bad discussion id", http.StatusBadRequest)
		return
	}
	for i := range h.discussions {
		if h.discussions[i].id == id {
			h.discussions[i].resolved = true
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	http.NotFound(w, r)
}

func (h *gitlabHarness) serveNotes(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	perPage := 100
	start := (page - 1) * perPage
	if start >= len(h.notes) {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	end := start + perPage
	if end > len(h.notes) {
		end = len(h.notes)
	}
	out := make([]map[string]any, 0, end-start)
	for _, n := range h.notes[start:end] {
		out = append(out, map[string]any{
			"id":     n.id,
			"body":   n.body,
			"author": map[string]any{"username": n.author},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (h *gitlabHarness) createNote(w http.ResponseWriter, r *http.Request) {
	h.noteCreateCalls++
	body, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(body))
	h.nextID++
	id := h.nextID
	h.notes = append(h.notes, gitlabNote{id: id, body: form.Get("body"), author: h.botAuthor})
	w.WriteHeader(http.StatusCreated)
	_, _ = io.WriteString(w, fmt.Sprintf(`{"id":%d}`, id))
}

func (h *gitlabHarness) updateNote(w http.ResponseWriter, r *http.Request, path, notesBase string) {
	h.noteUpdateCalls++
	body, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(body))
	rawID := strings.TrimPrefix(path, notesBase+"/")
	idStr, err := url.PathUnescape(rawID)
	if err != nil {
		http.Error(w, "bad note id", http.StatusBadRequest)
		return
	}
	var id int
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		http.Error(w, "bad note id", http.StatusBadRequest)
		return
	}
	for i := range h.notes {
		if h.notes[i].id == id {
			h.notes[i].body = form.Get("body")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, fmt.Sprintf(`{"id":%d}`, id))
			return
		}
	}
	http.NotFound(w, r)
}

// ---- GitLab httptest harness (list/create/resolve path for conformance) ----
// crash-then-rerun fixtures (REQ-E4-S09-01). A plain rerun creates zero new bot
// threads; a crash-then-rerun fills exactly the one gap slot without duplicating
// partial work.
// seedRaw seeds a discussion with a VERBATIM body — used to plant a corrupted
// marker that h.seed (which renders a valid envelope) could never produce.
func (h *gitlabHarness) seedRaw(id, author, body string) {
	h.discussions = append(h.discussions, gitlabDiscussion{id: id, body: body, author: author})
}

// corruptMarkerBody carries the real marker SENTINEL with a payload the regexp
// extracts but the decoder rejects — the REL-06 corruption.
func corruptMarkerBody() string {
	return "<!-- " + render.MarkerSentinel + ` {"slot": 12345} -->` + "\n\nbody"
}

// TestSpoofedMarkerStillIgnored — REQ-AUD-S12-02 (spoof half). The AUD-S12
// skip-with-warning change must NOT weaken the ADR-0019 author-identity filter.
// The filter runs BEFORE the marker is parsed, so a CONTRIBUTOR note is
// invisible whether its marker is perfect or garbage, and it never reaches the
// warning channel.
//
// The bot sub-case is the POSITIVE CONTROL: with an identical corrupt body but
// a bot author, a warning DOES appear. If someone ever moves parseMarker above
// the author check, the contributor cases start warning and this test reds.
func TestSpoofedMarkerStillIgnored(t *testing.T) {
	m := rerunCommentMarker()

	for _, tc := range []struct {
		name string
		body func(t *testing.T) string
	}{
		{
			name: "contributor/well-formed-marker",
			body: func(t *testing.T) string {
				t.Helper()
				body, err := render.Envelope(m, "spoof")
				if err != nil {
					t.Fatal(err)
				}
				return body
			},
		},
		{
			name: "contributor/malformed-marker",
			body: func(*testing.T) string { return corruptMarkerBody() },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newGitLabHarness(m.Slot.Project, m.Slot.MR)
			h.seedRaw("note/6660", "contributor-mallory", tc.body(t))
			c := h.client(t)

			threads, err := c.ListBotThreads(m.Slot.Project, m.Slot.MR)
			if err != nil {
				t.Fatalf("a contributor note must never fail the listing: %v", err)
			}
			if len(threads) != 0 {
				t.Fatalf("contributor note must be invisible, got %+v", threads)
			}
			if got := c.Warnings(); len(got) != 0 {
				t.Fatalf("a contributor note must never reach the warning channel "+
					"(the author filter runs BEFORE parseMarker), got %v", got)
			}

			receipt, err := forge.Reconcile(c, testClock(), desiredThreadFor(m, nil), forge.Preconditions{})
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			if h.createCalls != 1 {
				t.Fatalf("the spoof must not satisfy the slot; createCalls=%d, want 1", h.createCalls)
			}
			if len(receipt.Warnings) != 0 {
				t.Fatalf("receipt must carry no warnings for a contributor note, got %v", receipt.Warnings)
			}
		})
	}

	t.Run("bot/malformed-marker-does-warn", func(t *testing.T) {
		h := newGitLabHarness(m.Slot.Project, m.Slot.MR)
		h.seedRaw("note/7770", botID, corruptMarkerBody())
		c := h.client(t)

		receipt, err := forge.Reconcile(c, testClock(), desiredThreadFor(m, nil), forge.Preconditions{})
		if err != nil {
			t.Fatalf("a corrupted BOT marker must be skipped, not fatal: %v", err)
		}
		if len(receipt.Warnings) != 1 {
			t.Fatalf("an identical body authored by the BOT must warn — otherwise the "+
				"contributor assertions above are vacuous; got %v", receipt.Warnings)
		}
		if !strings.Contains(receipt.Warnings[0], "note/7770") {
			t.Fatalf("the warning must name the skipped artifact, got %q", receipt.Warnings[0])
		}
	})
}
