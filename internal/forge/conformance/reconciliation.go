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

	"github.com/PlatformRelay/assent/internal/forge"
	gitlab "github.com/PlatformRelay/assent/internal/forge/gitlab"
)

const decHex = "sha256:1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaaa"

// ---- rerun-idempotence.yaml (docs/contracts/p3-e5-publication-protocol/fixtures/rerun-idempotence.yaml) ----

const occChallenge = "sha256:c6957a516c95532386bed08f56441dfbb8d18efda24f5abdab1e48437aa3357d" // line 23
const occComment = "sha256:1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaaa"   // line 25

func rerunChallengeMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  proj,
			MR:       mrIID,
			Rule:     "topic-safety/retention-shrink-challenge",
			Effect:   "challenge",
			EntryRef: "topic-registry:orders.events.v1",
		},
		Occurrence: occChallenge,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func rerunCommentMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  proj,
			MR:       mrIID,
			Rule:     "ownership/entry-owner-required",
			Effect:   "comment",
			EntryRef: "topic-registry:orders.events.v1",
		},
		Occurrence: occComment,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

// ---- crash-then-rerun.yaml ----

const occCrashChallenge = "sha256:aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111" // line 23
const occCrashComment = "sha256:bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"     // line 25

func crashChallengeMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "platform/orders-service",
			MR:       "551",
			Rule:     "topic-safety/retention-shrink-challenge",
			Effect:   "challenge",
			EntryRef: "topic-registry:payments.events.v1",
		},
		Occurrence: occCrashChallenge,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func crashCommentMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "platform/orders-service",
			MR:       "551",
			Rule:     "ownership/entry-owner-required",
			Effect:   "comment",
			EntryRef: "topic-registry:payments.events.v1",
		},
		Occurrence: occCrashComment,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

// ---- duplicate-repair.yaml ----

const occDup = "sha256:dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444" // line 38

func dupMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "platform/orders-service",
			MR:       "612",
			Rule:     "topic-safety/retention-shrink-challenge",
			Effect:   "challenge",
			EntryRef: "topic-registry:orders.events.v1",
		},
		Occurrence: occDup,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func desiredThreadFor(m forge.Marker) forge.DesiredReviewState {
	return forge.DesiredReviewState{
		Project: m.Slot.Project,
		MR:      m.Slot.MR,
		Thread:  &forge.DesiredThread{Marker: m, Body: "obligation not proven"},
	}
}

func dupDesired() forge.DesiredReviewState {
	return desiredThreadFor(dupMarker())
}

// replayRerunIdempotence exercises rerun-idempotence.yaml run2 against f.
func replayRerunIdempotence(f forge.Forge) (newArtifacts int, err error) {
	before, err := botThreadCount(f, proj, mrIID)
	if err != nil {
		return 0, err
	}
	for _, m := range []forge.Marker{rerunChallengeMarker(), rerunCommentMarker()} {
		r, err := forge.Reconcile(f, testClock(), desiredThreadFor(m), forge.Preconditions{})
		if err != nil {
			return 0, fmt.Errorf("Reconcile(%s): %w", m.Slot.Rule, err)
		}
		if len(r.Repairs) != 0 {
			return 0, fmt.Errorf("rerun must record no repairs, got %+v", r.Repairs)
		}
	}
	after, err := botThreadCount(f, proj, mrIID)
	if err != nil {
		return 0, err
	}
	return after - before, nil
}

// replayCrashThenRerun exercises crash-then-rerun.yaml run2 against f.
func replayCrashThenRerun(f forge.Forge) (newArtifacts int, err error) {
	project := crashChallengeMarker().Slot.Project
	mr := crashChallengeMarker().Slot.MR
	before, err := botThreadCount(f, project, mr)
	if err != nil {
		return 0, err
	}
	for _, m := range []forge.Marker{crashChallengeMarker(), crashCommentMarker()} {
		r, err := forge.Reconcile(f, testClock(), desiredThreadFor(m), forge.Preconditions{})
		if err != nil {
			return 0, fmt.Errorf("Reconcile(%s): %w", m.Slot.Rule, err)
		}
		if len(r.Repairs) != 0 {
			return 0, fmt.Errorf("crash-then-rerun must record no repairs, got %+v", r.Repairs)
		}
	}
	after, err := botThreadCount(f, project, mr)
	if err != nil {
		return 0, err
	}
	return after - before, nil
}

// replayDuplicateRepair exercises duplicate-repair.yaml against f. Returns the
// receipt for repair assertions.
func replayDuplicateRepair(f forge.Forge) (forge.PublicationReceipt, error) {
	before, err := botThreadCount(f, "platform/orders-service", "612")
	if err != nil {
		return forge.PublicationReceipt{}, err
	}
	receipt, err := forge.Reconcile(f, testClock(), dupDesired(), forge.Preconditions{})
	if err != nil {
		return forge.PublicationReceipt{}, err
	}
	after, err := botThreadCount(f, "platform/orders-service", "612")
	if err != nil {
		return forge.PublicationReceipt{}, err
	}
	if newArtifacts := after - before; newArtifacts != 0 {
		return forge.PublicationReceipt{}, fmt.Errorf("repair must create zero artifacts, created %d", newArtifacts)
	}
	return receipt, nil
}

func botThreadCount(f forge.Forge, project, mr string) (int, error) {
	threads, err := f.ListBotThreads(project, mr)
	if err != nil {
		return 0, err
	}
	return len(threads), nil
}

// ---- GitLab httptest harness (list/create/resolve path for conformance) ----

type gitlabDiscussion struct {
	id       string
	body     string
	resolved bool
	author   string
}

type gitlabHarness struct {
	project      string
	mr           string
	botAuthor    string
	discussions  []gitlabDiscussion
	nextID       int
	createCalls  int
	resolveCalls int
}

func newGitLabHarness(project, mr string) *gitlabHarness {
	return &gitlabHarness{project: project, mr: mr, botAuthor: botID, nextID: 9000}
}

func (h *gitlabHarness) seed(id, author string, marker forge.Marker, resolved bool) error {
	body, err := renderMarkerBody(marker, "seed")
	if err != nil {
		return err
	}
	h.discussions = append(h.discussions, gitlabDiscussion{id: id, body: body, resolved: resolved, author: author})
	return nil
}

func (h *gitlabHarness) client(t interface {
	Helper()
	Cleanup(func())
}) *gitlab.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(h.handle))
	t.Cleanup(srv.Close)
	return gitlab.New(srv.URL, "test-token", h.botAuthor)
}

func (h *gitlabHarness) handle(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get("PRIVATE-TOKEN"); got != "test-token" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	escapedBase := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/discussions",
		url.PathEscape(h.project), url.PathEscape(h.mr))
	path := r.URL.EscapedPath()
	switch {
	case r.Method == http.MethodGet && path == escapedBase:
		h.serveDiscussions(w, r)
	case r.Method == http.MethodPost && path == escapedBase:
		h.createDiscussion(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(path, escapedBase+"/"):
		h.resolveDiscussion(w, r, path, escapedBase)
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

// renderMarkerBody builds a GitLab note body carrying the ADR-0019 hidden-HTML
// marker (same wire shape as internal/forge/gitlab/marker.go).
func renderMarkerBody(m forge.Marker, human string) (string, error) {
	type slotJSON struct {
		Project  string `json:"project"`
		MR       string `json:"mr"`
		Rule     string `json:"rule"`
		EntryRef string `json:"entryRef,omitempty"`
		Effect   string `json:"effect"`
	}
	type artifactJSON struct {
		Kind          string `json:"kind"`
		SchemaVersion string `json:"schemaVersion"`
	}
	payload, err := json.Marshal(struct {
		Slot       slotJSON     `json:"slot"`
		Occurrence string       `json:"occurrence"`
		Decision   string       `json:"decision"`
		Artifact   artifactJSON `json:"artifact"`
	}{
		Slot: slotJSON{
			Project:  m.Slot.Project,
			MR:       m.Slot.MR,
			Rule:     m.Slot.Rule,
			EntryRef: m.Slot.EntryRef,
			Effect:   m.Slot.Effect,
		},
		Occurrence: m.Occurrence,
		Decision:   m.Decision,
		Artifact: artifactJSON{
			Kind:          m.Artifact.Kind,
			SchemaVersion: m.Artifact.SchemaVersion,
		},
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("<!-- assent:marker %s -->\n\n%s", payload, human), nil
}
