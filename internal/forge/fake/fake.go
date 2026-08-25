// Package fake is the in-memory forge substrate for the P4-E1 Reconcile tests
// (S06/S08/S07-02). It records threads/approvals/merges and their
// forge-assigned ids, lists existing bot artifacts FILTERED BY AUTHOR IDENTITY
// (a contributor comment carrying a well-formed marker is invisible to
// reconciliation), and implements the compare-and-swap merge that accepts ONLY
// when the current source SHA, target SHA AND merge-result digest all still
// equal the pinned values. There is NO live infra — this fake is the only
// substrate for every test in this lane.
package fake

import (
	"fmt"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/render"
)

// Forge is the in-memory fake. It is deterministic: forge-assigned ids are
// allocated from a monotonic counter, and every mutation is recorded so tests
// can assert EXACT write counts (the fail-closed teeth: zero approvals AND zero
// merges when a write must not occur).
type Forge struct {
	// BotAuthor is the configured bot/service-account identity. ListBotThreads
	// returns only threads whose Author equals this — the author-identity filter
	// (ADR-0019). Threads authored by anyone else are excluded.
	BotAuthor string

	threads []forge.Thread
	notes   []forge.Note

	// Current forge CAS state: the source/target SHA and merge-result digest the
	// forge currently holds. MergeCAS accepts only if the desired pins all match
	// these. Tests move these to simulate a SHA advancing after evaluation.
	CurrentSourceSha         string
	CurrentTargetSha         string
	CurrentMergeResultDigest string

	// Recorded writes — the assertion surface for fail-closed proofs.
	Approvals []string
	Merges    []string

	// Note create-vs-update counters (E10-S01, REQ-E10-S01-04). UpsertComment is
	// ONE port method with two outcomes, so the conformance suite's port-level
	// decorator cannot tell them apart — only the backend can. Every other write
	// counter the suite needs is counted at the port instead (see
	// internal/forge/conformance/portcount.go), so nothing else was added here.
	NoteCreateCalls int
	NoteUpdateCalls int

	seq    int
	merged bool

	// Read-side fixture fields for Snapshot/Resolve (E4-S01). Write-path tests may
	// leave these zero; Snapshot/Resolve tests populate them explicitly.
	MRAuthor     string
	SourceBranch string
	TargetBranch string
	ChangedFiles []string
	Capabilities forge.CapabilityFlags
	ResolveMode  ResolveMode

	// ChangedFilesGap, when non-empty, models a TRUNCATED / unprovable
	// changed-file enumeration (ADR-0020 §6): Snapshot then reports
	// ChangedFilesComplete=false carrying this reason, while still returning
	// whatever partial ChangedFiles list is configured — so a VISIBLE
	// `.assent/**` path still dominates to BLOCK. Empty (the default) means the
	// enumeration is provably complete.
	ChangedFilesGap string

	// ChangedFilesErr, when non-nil, models a diff-endpoint 404/5xx
	// (ADR-0020 §3/§6): Snapshot returns it as a HARD ERROR with no snapshot at
	// all. A missing diff resource is forge anomaly — never evidence of an empty
	// change set, which is exactly the fail-open the 404→empty mapping created.
	ChangedFilesErr error

	// AfterCurrentHeads, when non-nil, is invoked at the END of CurrentHeads —
	// AFTER the current heads are read but BEFORE MergeCAS runs. It is the TOCTOU
	// seam: a test sets it to advance CurrentTargetSha (etc.) so Reconcile's
	// pre-check sees the still-pinned heads (passes), then the head moves in the
	// window, then the atomic MergeCAS guard observes the drift and must reject.
	// Default nil = no race (the ordinary, non-racing path).
	AfterCurrentHeads func(f *Forge)

	// RescanListBotThreads, when non-nil, replaces the bot-thread listing on the
	// first ListBotThreads call after a forge mutation within the same Reconcile
	// (the post-write rescan — P3-E5 step 9). Used to simulate rescan mismatch.
	RescanListBotThreads func(listed []forge.Thread) ([]forge.Thread, error)

	mutationsSinceList int
}

// New returns a fake forge configured with the given bot identity and current
// CAS state (source/target SHA + merge-result digest).
func New(botAuthor, curSource, curTarget, curDigest string) *Forge {
	return &Forge{
		BotAuthor:                botAuthor,
		CurrentSourceSha:         curSource,
		CurrentTargetSha:         curTarget,
		CurrentMergeResultDigest: curDigest,
	}
}

// SeedThread records a pre-existing thread (e.g. from a prior run, or an
// adversarial contributor comment) with a chosen author identity. It is the
// test hook for the idempotence + author-identity cases; the author governs
// whether ListBotThreads will return it.
func (f *Forge) SeedThread(id, author string, marker forge.Marker, resolved bool) {
	f.threads = append(f.threads, forge.Thread{
		ID:       id,
		Marker:   marker,
		Author:   author,
		Resolved: resolved,
	})
}

// SeedNote records a pre-existing bot or contributor MR note for summary-port
// tests. Author identity governs whether ListBotNotes returns it.
func (f *Forge) SeedNote(id, author string, marker forge.Marker, body string) {
	f.notes = append(f.notes, forge.Note{
		ID:     id,
		Marker: marker,
		Author: author,
		Body:   body,
	})
}

func (f *Forge) noteMutation() { f.mutationsSinceList++ }

// ListBotThreads returns only threads authored by the configured bot — the
// author-identity filter (ADR-0019). A contributor (non-bot) comment carrying a
// syntactically perfect, schema-valid marker is EXCLUDED here and therefore has
// zero effect on reconciliation. Filtering is by AUTHOR IDENTITY, never by
// marker well-formedness. When RescanListBotThreads is set, the first list after
// a mutation within one Reconcile simulates the step-9 rescan listing.
func (f *Forge) ListBotThreads(_, _ string) ([]forge.Thread, error) {
	var out []forge.Thread
	for _, t := range f.threads {
		if t.Author == f.BotAuthor {
			out = append(out, t)
		}
	}
	if f.mutationsSinceList > 0 && f.RescanListBotThreads != nil {
		f.mutationsSinceList = 0
		return f.RescanListBotThreads(out)
	}
	f.mutationsSinceList = 0
	return out, nil
}

// ListBotNotes returns only notes authored by the configured bot — the same
// author-identity filter as ListBotThreads (ADR-0019).
func (f *Forge) ListBotNotes(_, _ string) ([]forge.Note, error) {
	var out []forge.Note
	for _, n := range f.notes {
		if n.Author == f.BotAuthor {
			out = append(out, n)
		}
	}
	return out, nil
}

// UpsertComment creates or edits-in-place exactly one summary-comment note per
// MR. When a bot note with artifact.kind summary-comment already exists, its
// body is updated and the same forge id is returned — never a second note.
func (f *Forge) UpsertComment(_, _ string, marker forge.Marker, body string) (forge.Note, error) {
	if marker.Artifact.Kind != "summary-comment" {
		return forge.Note{}, forge.ErrInvalidSummaryMarker
	}
	fullBody, err := render.Envelope(marker, body)
	if err != nil {
		return forge.Note{}, err
	}
	for i := range f.notes {
		if f.notes[i].Author != f.BotAuthor {
			continue
		}
		if f.notes[i].Marker.Artifact.Kind != marker.Artifact.Kind {
			continue
		}
		if marker.Artifact.Kind == "summary-comment" {
			f.NoteUpdateCalls++
			f.notes[i].Marker = marker
			f.notes[i].Body = fullBody
			f.noteMutation()
			return f.notes[i], nil
		}
	}
	f.NoteCreateCalls++
	f.seq++
	n := forge.Note{
		ID:     fmt.Sprintf("note/%d", 8000+f.seq),
		Marker: marker,
		Author: f.BotAuthor,
		Body:   fullBody,
	}
	f.notes = append(f.notes, n)
	f.noteMutation()
	return n, nil
}

// CurrentHeads returns the forge's current source SHA, target SHA and
// merge-result digest. Reconcile reads these before any write so a SHA drift
// fails closed with zero writes; tests move these to simulate a SHA advancing
// after evaluation.
func (f *Forge) CurrentHeads(_, _ string) (source, target, digest string, err error) {
	src, tgt, dg := f.CurrentSourceSha, f.CurrentTargetSha, f.CurrentMergeResultDigest
	// TOCTOU seam: fire AFTER capturing the values Reconcile's pre-check sees, so
	// the hook can move the heads in the window before MergeCAS. The returned
	// values are the PRE-move snapshot (the pre-check passes); MergeCAS then reads
	// the POST-move state and rejects. This models a real head advancing between
	// the two forge round-trips.
	if f.AfterCurrentHeads != nil {
		f.AfterCurrentHeads(f)
	}
	return src, tgt, dg, nil
}

// ResolveThread marks the bot thread with the given forge id resolved in place —
// the duplicate-repair action (S12-03). It creates nothing and records nothing
// new; it only flips Resolved on the existing thread. Resolving is idempotent
// (an already-resolved or unknown id is a no-op that returns no error), so a
// re-run of the repair produces zero new writes.
func (f *Forge) ResolveThread(_, _, id string) error {
	for i := range f.threads {
		if f.threads[i].ID == id {
			if !f.threads[i].Resolved {
				f.threads[i].Resolved = true
				f.noteMutation()
			}
			return nil
		}
	}
	return nil
}

// CreateThread records a new bot-authored thread with a fresh forge id.
func (f *Forge) CreateThread(_, _ string, marker forge.Marker, body string) (forge.Thread, error) {
	if _, err := render.Envelope(marker, body); err != nil {
		return forge.Thread{}, err
	}
	f.seq++
	t := forge.Thread{
		ID:     fmt.Sprintf("note/%d", 9000+f.seq),
		Marker: marker,
		Author: f.BotAuthor,
	}
	f.threads = append(f.threads, t)
	f.noteMutation()
	return t, nil
}

// Approve records an approval with a fresh forge id.
func (f *Forge) Approve(_, _ string) (string, error) {
	f.seq++
	id := fmt.Sprintf("approval/%d", 8000+f.seq)
	f.Approvals = append(f.Approvals, id)
	return id, nil
}

// MergeCAS performs the compare-and-swap merge. It accepts ONLY if the current
// source SHA, target SHA AND merge-result digest all still equal the pinned
// values — all three (source-only is insufficient, ADR-0017 §1). If ANY has
// moved it returns forge.ErrSHAMoved and performs NO merge (no state change, no
// recorded merge). A second merge is rejected (idempotent guard).
func (f *Forge) MergeCAS(_, _ string, m forge.DesiredMerge) (string, error) {
	if f.merged {
		return "", fmt.Errorf("fake: already merged")
	}
	if m.SourceSha != f.CurrentSourceSha ||
		m.TargetSha != f.CurrentTargetSha ||
		m.MergeResultDigest != f.CurrentMergeResultDigest {
		return "", forge.ErrSHAMoved
	}
	f.seq++
	id := fmt.Sprintf("merge/%d", 7000+f.seq)
	f.Merges = append(f.Merges, id)
	f.merged = true
	return id, nil
}

// ThreadCount returns the total number of recorded threads (bot AND non-bot) —
// the assertion surface proving a rerun created zero NEW threads.
func (f *Forge) ThreadCount() int { return len(f.threads) }

// BotThreadCount returns the number of bot-authored threads — proves exactly one
// bot thread exists after an idempotent rerun, independent of any contributor
// comment.
func (f *Forge) BotThreadCount() int {
	n := 0
	for _, t := range f.threads {
		if t.Author == f.BotAuthor {
			n++
		}
	}
	return n
}

// OpenBotThreadCount returns the number of UNRESOLVED bot-authored threads — the
// assertion surface proving duplicate-repair left exactly one OPEN occupant per
// slot (the canonical), the resolved duplicates no longer occupying it.
func (f *Forge) OpenBotThreadCount() int {
	n := 0
	for _, t := range f.threads {
		if t.Author == f.BotAuthor && !t.Resolved {
			n++
		}
	}
	return n
}

// IsResolved reports whether the thread with the given forge id is resolved —
// lets a test assert the exact set of repaired (resolved) vs. canonical (open)
// duplicates after repair.
func (f *Forge) IsResolved(id string) bool {
	for _, t := range f.threads {
		if t.ID == id {
			return t.Resolved
		}
	}
	return false
}

// SummaryNoteCount returns bot-authored notes with artifact.kind summary-comment.
func (f *Forge) SummaryNoteCount() int {
	n := 0
	for _, note := range f.notes {
		if note.Author == f.BotAuthor && note.Marker.Artifact.Kind == "summary-comment" {
			n++
		}
	}
	return n
}

// NoteBody returns the stored body for a note id (test assertion helper).
func (f *Forge) NoteBody(id string) string {
	for _, n := range f.notes {
		if n.ID == id {
			return n.Body
		}
	}
	return ""
}

// static assertion that the fake implements the port.
var _ forge.Forge = (*Forge)(nil)
