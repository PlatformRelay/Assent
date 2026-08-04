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

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
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
const occCrashComment = "sha256:bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"   // line 25

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

// TestConformanceRerunIdempotence replays the frozen P3-E5 rerun-idempotence and
// crash-then-rerun fixtures (REQ-E4-S09-01). A plain rerun creates zero new bot
// threads; a crash-then-rerun fills exactly the one gap slot without duplicating
// partial work.
func TestConformanceRerunIdempotence(t *testing.T) {
	t.Run("fake/rerun-idempotence", func(t *testing.T) {
		// Pre-state = rerun-idempotence.yaml run2.step2ExistingArtifacts (lines 58-66).
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/9001", botID, rerunChallengeMarker(), true) // reviewer-resolved
		f.SeedThread("note/9002", botID, rerunCommentMarker(), false)

		created, err := replayRerunIdempotence(f)
		if err != nil {
			t.Fatal(err)
		}
		// expected.newArtifactsCreated: 0 (rerun-idempotence.yaml line 82).
		if created != 0 {
			t.Fatalf("rerun must create zero new artifacts, created %d", created)
		}
		if got := f.BotThreadCount(); got != 2 {
			t.Fatalf("rerun must leave exactly 2 bot threads, got %d", got)
		}
		if !f.IsResolved("note/9001") {
			t.Fatal("rerun must preserve reviewer resolution of note/9001")
		}
	})

	t.Run("fake/crash-then-rerun", func(t *testing.T) {
		// Pre-state = crash-then-rerun.yaml step2ExistingArtifacts (lines 55-61).
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/7001", botID, crashChallengeMarker(), false)

		created, err := replayCrashThenRerun(f)
		if err != nil {
			t.Fatal(err)
		}
		// expected.newArtifactsCreated: 1 (crash-then-rerun.yaml line 81).
		if created != 1 {
			t.Fatalf("crash-then-rerun must create exactly one gap artifact, created %d", created)
		}
		if got := f.BotThreadCount(); got != 2 {
			t.Fatalf("expected 2 bot threads after gap-fill, got %d", got)
		}
	})

	t.Run("gitlab/rerun-idempotence", func(t *testing.T) {
		h := newGitLabHarness(proj, mrIID)
		if err := h.seed("note/9001", botID, rerunChallengeMarker(), true); err != nil {
			t.Fatal(err)
		}
		if err := h.seed("note/9002", botID, rerunCommentMarker(), false); err != nil {
			t.Fatal(err)
		}
		c := h.client(t)

		created, err := replayRerunIdempotence(c)
		if err != nil {
			t.Fatal(err)
		}
		if created != 0 {
			t.Fatalf("gitlab rerun must create zero new artifacts, created %d", created)
		}
		if h.createCalls != 0 {
			t.Fatalf("gitlab rerun must not POST new discussions, createCalls=%d", h.createCalls)
		}
	})

	t.Run("gitlab/crash-then-rerun", func(t *testing.T) {
		h := newGitLabHarness("platform/orders-service", "551")
		if err := h.seed("note/7001", botID, crashChallengeMarker(), false); err != nil {
			t.Fatal(err)
		}
		c := h.client(t)

		created, err := replayCrashThenRerun(c)
		if err != nil {
			t.Fatal(err)
		}
		if created != 1 {
			t.Fatalf("gitlab crash-then-rerun must create one gap artifact, created %d", created)
		}
		if h.createCalls != 1 {
			t.Fatalf("gitlab crash-then-rerun must POST exactly one discussion, createCalls=%d", h.createCalls)
		}
	})
}

// TestConformanceDuplicateRepair replays duplicate-repair.yaml (REQ-E4-S09-02):
// lowest forge id is canonical, non-canonical duplicates are resolved, and
// PublicationReceipt.repairs records each repair deterministically.
func TestConformanceDuplicateRepair(t *testing.T) {
	fixtureOrder := []string{"note/8005", "note/8001", "note/8003"}
	reversedOrder := []string{"note/8003", "note/8001", "note/8005"}
	wantRepairs := []forge.Repair{
		{RepairedForgeID: "note/8003", CanonicalForgeID: "note/8001", Action: "resolve"},
		{RepairedForgeID: "note/8005", CanonicalForgeID: "note/8001", Action: "resolve"},
	}

	for _, tc := range []struct {
		name string
		seed []string
	}{
		{"fake/fixture-pagination-order", fixtureOrder},
		{"fake/reversed-scan-order", reversedOrder},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fake.New(botID, "src", "tgt", "sha256:merge")
			for _, id := range tc.seed {
				f.SeedThread(id, botID, dupMarker(), false)
			}

			receipt, err := replayDuplicateRepair(f)
			if err != nil {
				t.Fatal(err)
			}

			// expected.canonicalForgeId: note/8001 (duplicate-repair.yaml line 102).
			if len(receipt.Operations) != 1 || receipt.Operations[0].TargetID != "note/8001" {
				t.Fatalf("canonical must be note/8001, got %+v", receipt.Operations)
			}
			if len(receipt.Repairs) != len(wantRepairs) {
				t.Fatalf("expected %d repairs, got %+v", len(wantRepairs), receipt.Repairs)
			}
			for i, want := range wantRepairs {
				if receipt.Repairs[i] != want {
					t.Fatalf("repair[%d]: got %+v want %+v", i, receipt.Repairs[i], want)
				}
			}
			if got := f.OpenBotThreadCount(); got != 1 {
				t.Fatalf("after repair exactly one open bot thread must remain, got %d", got)
			}
		})
	}

	t.Run("gitlab/fixture-pagination-order", func(t *testing.T) {
		h := newGitLabHarness("platform/orders-service", "612")
		for _, id := range fixtureOrder {
			if err := h.seed(id, botID, dupMarker(), false); err != nil {
				t.Fatal(err)
			}
		}
		c := h.client(t)

		receipt, err := replayDuplicateRepair(c)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.Operations[0].TargetID != "note/8001" {
			t.Fatalf("gitlab canonical must be note/8001, got %q", receipt.Operations[0].TargetID)
		}
		if len(receipt.Repairs) != 2 {
			t.Fatalf("gitlab repair must record 2 repairs, got %+v", receipt.Repairs)
		}
		if h.createCalls != 0 {
			t.Fatalf("duplicate repair must not create threads, createCalls=%d", h.createCalls)
		}
		if h.resolveCalls != 2 {
			t.Fatalf("duplicate repair must resolve 2 duplicates, resolveCalls=%d", h.resolveCalls)
		}
		open := 0
		for _, d := range h.discussions {
			if d.author == botID && !d.resolved {
				open++
			}
		}
		if open != 1 {
			t.Fatalf("after repair exactly one open bot discussion must remain, got %d", open)
		}
	})
}

// TestConformanceSpoofedMarkerIgnored proves contributor marker spoofing has zero
// reconciliation effect (REQ-E4-S09-03 / P3-E5-S01-04): the author-identity
// filter on ListBotThreads excludes non-bot markers.
func TestConformanceSpoofedMarkerIgnored(t *testing.T) {
	m := rerunCommentMarker()

	t.Run("fake/contributor-marker-invisible-on-rerun", func(t *testing.T) {
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/9002", botID, m, false)
		f.SeedThread("note/6660", "contributor-mallory", m, false)

		before := f.ThreadCount()
		receipt, err := forge.Reconcile(f, testClock(), desiredThreadFor(m), forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if after := f.ThreadCount(); after != before {
			t.Fatalf("spoofed marker must not create threads on rerun: before=%d after=%d", before, after)
		}
		if got := receipt.Operations[0].TargetID; got != "note/9002" {
			t.Fatalf("receipt must target bot thread note/9002, got %q (contributor would be note/6660)", got)
		}
	})

	t.Run("fake/contributor-only-creates-bot-thread", func(t *testing.T) {
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/6660", "contributor-mallory", m, false)

		_, err := forge.Reconcile(f, testClock(), desiredThreadFor(m), forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if got := f.BotThreadCount(); got != 1 {
			t.Fatalf("contributor marker alone must not satisfy slot; expected 1 bot thread, got %d", got)
		}
	})

	t.Run("gitlab/contributor-marker-excluded", func(t *testing.T) {
		h := newGitLabHarness(m.Slot.Project, m.Slot.MR)
		if err := h.seed("note/9002", botID, m, false); err != nil {
			t.Fatal(err)
		}
		if err := h.seed("note/6660", "contributor-mallory", m, false); err != nil {
			t.Fatal(err)
		}
		c := h.client(t)

		before, err := botThreadCount(c, m.Slot.Project, m.Slot.MR)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := forge.Reconcile(c, testClock(), desiredThreadFor(m), forge.Preconditions{})
		if err != nil {
			t.Fatal(err)
		}
		after, err := botThreadCount(c, m.Slot.Project, m.Slot.MR)
		if err != nil {
			t.Fatal(err)
		}
		if after-before != 0 {
			t.Fatalf("gitlab spoof rerun must create zero artifacts, created %d", after-before)
		}
		if h.createCalls != 0 {
			t.Fatalf("contributor marker must not prevent idempotent reuse; createCalls=%d", h.createCalls)
		}
		if got := receipt.Operations[0].TargetID; got != "note/9002" {
			t.Fatalf("receipt must target bot thread note/9002, got %q", got)
		}
		threads, err := c.ListBotThreads(m.Slot.Project, m.Slot.MR)
		if err != nil {
			t.Fatal(err)
		}
		if len(threads) != 1 || threads[0].ID != "note/9002" {
			t.Fatalf("ListBotThreads must return only bot thread, got %+v", threads)
		}
	})

	t.Run("gitlab/contributor-only-creates-bot-thread", func(t *testing.T) {
		h := newGitLabHarness(m.Slot.Project, m.Slot.MR)
		if err := h.seed("note/6660", "contributor-mallory", m, false); err != nil {
			t.Fatal(err)
		}
		c := h.client(t)

		_, err := forge.Reconcile(c, testClock(), desiredThreadFor(m), forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if h.createCalls != 1 {
			t.Fatalf("contributor marker invisible — must create one bot thread, createCalls=%d", h.createCalls)
		}
		threads, err := c.ListBotThreads(m.Slot.Project, m.Slot.MR)
		if err != nil {
			t.Fatal(err)
		}
		if len(threads) != 1 || threads[0].Author != botID {
			t.Fatalf("only bot-authored thread counts, got %+v", threads)
		}
	})
}
