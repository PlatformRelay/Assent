package conformance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
	gitlab "github.com/PlatformRelay/assent/internal/forge/gitlab"
)

// backends_test.go wires the two built-in backends into the shared suite. Each is
// a Factory and nothing more — the case bodies live in importable non-test Go, so
// a third adapter (GitHub, E10-S06+) needs a file this size and no copied cases.

// ---- fake backend ----

type fakeBackend struct {
	f  *fake.Forge
	cp *countingPort
}

func (b fakeBackend) MergeAttempts() int        { return b.cp.mergeAttempts }
func (b fakeBackend) MergesPerformed() int      { return b.cp.mergesPerformed }
func (b fakeBackend) Approvals() int            { return b.cp.approvals }
func (b fakeBackend) ThreadsCreated() int       { return b.cp.threadsCreated }
func (b fakeBackend) ThreadsResolved() int      { return b.cp.threadsResolved }
func (b fakeBackend) NotesCreated() int         { return b.f.NoteCreateCalls }
func (b fakeBackend) NotesUpdated() int         { return b.f.NoteUpdateCalls }
func (b fakeBackend) NoteBody(id string) string { return b.f.NoteBody(id) }
func (b fakeBackend) IsResolved(id string) bool { return b.f.IsResolved(id) }
func (b fakeBackend) ThreadCount() int          { return b.f.ThreadCount() }
func (b fakeBackend) BotThreadCount() int       { return b.f.BotThreadCount() }
func (b fakeBackend) OpenBotThreadCount() int   { return b.f.OpenBotThreadCount() }

func (b fakeBackend) SeedThread(id, author string, m forge.Marker, resolved bool) error {
	b.f.SeedThread(id, author, m, resolved)
	return nil
}

func (b fakeBackend) SeedNote(id, author string, m forge.Marker, body string) error {
	b.f.SeedNote(id, author, m, body)
	return nil
}

func (b fakeBackend) MoveTargetHead(sha string) { b.f.CurrentTargetSha = sha }

func (b fakeBackend) Pins() forge.DesiredMerge {
	return forge.DesiredMerge{
		SourceSha:         b.f.CurrentSourceSha,
		TargetSha:         b.f.CurrentTargetSha,
		MergeResultDigest: b.f.CurrentMergeResultDigest,
	}
}

func (b fakeBackend) DriftSourceHeadAfterRead(sha string) {
	b.f.AfterCurrentHeads = func(fk *fake.Forge) { fk.CurrentSourceSha = sha }
}

func fakeFactory(t TB, cfg Config) Backend {
	t.Helper()
	f := fake.New(cfg.BotAuthor, cfg.CurrentSourceSHA, cfg.CurrentTargetSHA, cfg.CurrentMergeResultDigest)
	cp := newCountingPort(f)
	b := fakeBackend{f: f, cp: cp}
	return Backend{Port: cp, Fixture: b, Observer: b}
}

// ---- gitlab backend (httptest) ----

type gitlabBackend struct {
	h  *gitlabHarness
	cp *countingPort
}

func (b gitlabBackend) MergeAttempts() int   { return b.cp.mergeAttempts }
func (b gitlabBackend) MergesPerformed() int { return b.cp.mergesPerformed }
func (b gitlabBackend) Approvals() int       { return b.cp.approvals }
func (b gitlabBackend) ThreadsCreated() int  { return b.cp.threadsCreated }
func (b gitlabBackend) ThreadsResolved() int { return b.cp.threadsResolved }
func (b gitlabBackend) NotesCreated() int    { return b.h.noteCreateCalls }
func (b gitlabBackend) NotesUpdated() int    { return b.h.noteUpdateCalls }

func (b gitlabBackend) NoteBody(id string) string {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "note/"))
	if err != nil {
		return ""
	}
	for _, note := range b.h.notes {
		if note.id == n {
			return note.body
		}
	}
	return ""
}

func (b gitlabBackend) IsResolved(id string) bool {
	for _, d := range b.h.discussions {
		if d.id == id {
			return d.resolved
		}
	}
	return false
}

func (b gitlabBackend) ThreadCount() int { return len(b.h.discussions) }

func (b gitlabBackend) BotThreadCount() int {
	n := 0
	for _, d := range b.h.discussions {
		if d.author == b.h.botAuthor {
			n++
		}
	}
	return n
}

func (b gitlabBackend) OpenBotThreadCount() int {
	n := 0
	for _, d := range b.h.discussions {
		if d.author == b.h.botAuthor && !d.resolved {
			n++
		}
	}
	return n
}

func (b gitlabBackend) SeedThread(id, author string, m forge.Marker, resolved bool) error {
	return b.h.seed(id, author, m, resolved)
}

func (b gitlabBackend) SeedNote(id, author string, m forge.Marker, body string) error {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "note/"))
	if err != nil {
		return fmt.Errorf("gitlab backend: note id %q is not note/<int>: %w", id, err)
	}
	return b.h.seedNote(n, author, m, body)
}

func (b gitlabBackend) MoveTargetHead(sha string) { b.h.targetSHA = sha }

// Pins reports the pins an evaluation would record against THIS backend. The
// digest is synthesised by the adapter (`gitlab.SyntheticDigest`) because GitLab
// exposes no merge-result digest — which is exactly why a case carrying a literal
// digest could only ever run against the fake. Collapsing the synthetic digest
// onto a real one is E10-S03.
func (b gitlabBackend) Pins() forge.DesiredMerge {
	return forge.DesiredMerge{
		SourceSha:         b.h.sourceSHA,
		TargetSha:         b.h.targetSHA,
		MergeResultDigest: gitlab.SyntheticDigest(b.h.sourceSHA, b.h.targetSHA),
	}
}

// DriftSourceHeadAfterRead moves the source head once the pre-check's MR read has
// been served — the TOCTOU window. Reconcile reads the heads, then MergeCAS reads
// them again; arming on the FIRST read means the second sees the moved head, so
// the atomic CAS is the thing that refuses.
func (b gitlabBackend) DriftSourceHeadAfterRead(sha string) {
	b.h.afterMRRead = func(h *gitlabHarness) { h.sourceSHA = sha }
}

func gitlabFactory(t TB, cfg Config) Backend {
	t.Helper()
	h := newGitLabHarness(cfg.Project, cfg.MR)
	h.botAuthor = cfg.BotAuthor
	h.sourceSHA = cfg.CurrentSourceSHA
	h.targetSHA = cfg.CurrentTargetSHA
	cp := newCountingPort(h.client(t))
	b := gitlabBackend{h: h, cp: cp}
	return Backend{Port: cp, Fixture: b, Observer: b}
}

// ---- MR / approve / merge endpoints (E10-S01) ----
//
// The pre-extraction harness served discussions and notes only, so the two
// SHA-guard cases could never run against GitLab — they were fake-only, while
// their catalog rows said `forge: gitlab`. These three routes close that gap.

func (h *gitlabHarness) serveMR(w http.ResponseWriter, _ *http.Request) {
	h.mrReads++
	_ = json.NewEncoder(w).Encode(map[string]any{
		"iid":           1,
		"project_id":    42,
		"sha":           h.sourceSHA,
		"source_branch": "feature",
		"target_branch": "main",
	})
	// Fire AFTER the response is written, so this read returns the PRE-move value
	// and only the NEXT one sees the drift.
	if h.afterMRRead != nil {
		h.afterMRRead(h)
	}
}

func (h *gitlabHarness) serveBranch(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"commit": map[string]any{"id": h.targetSHA},
	})
}

func (h *gitlabHarness) approve(w http.ResponseWriter, _ *http.Request) {
	h.approveCalls++
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": 1})
}

func (h *gitlabHarness) merge(w http.ResponseWriter, r *http.Request) {
	h.mergePUTs++
	// ?sha= is GitLab's compare-and-swap on the SOURCE head: a moved source is
	// 409, no merge. Modelled faithfully so the case proves the guard, not the fake.
	if got := r.URL.Query().Get("sha"); got != h.sourceSHA {
		http.Error(w, "sha mismatch", http.StatusConflict)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"merge_commit_sha": "deadbeef"})
}
