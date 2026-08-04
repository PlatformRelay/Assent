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

	// Current forge CAS state: the source/target SHA and merge-result digest the
	// forge currently holds. MergeCAS accepts only if the desired pins all match
	// these. Tests move these to simulate a SHA advancing after evaluation.
	CurrentSourceSha         string
	CurrentTargetSha         string
	CurrentMergeResultDigest string

	// Recorded writes — the assertion surface for fail-closed proofs.
	Approvals []string
	Merges    []string

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

	// AfterCurrentHeads, when non-nil, is invoked at the END of CurrentHeads —
	// AFTER the current heads are read but BEFORE MergeCAS runs. It is the TOCTOU
	// seam: a test sets it to advance CurrentTargetSha (etc.) so Reconcile's
	// pre-check sees the still-pinned heads (passes), then the head moves in the
	// window, then the atomic MergeCAS guard observes the drift and must reject.
	// Default nil = no race (the ordinary, non-racing path).
	AfterCurrentHeads func(f *Forge)
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

// ListBotThreads returns only threads authored by the configured bot — the
// author-identity filter (ADR-0019). A contributor (non-bot) comment carrying a
// syntactically perfect, schema-valid marker is EXCLUDED here and therefore has
// zero effect on reconciliation. Filtering is by AUTHOR IDENTITY, never by
// marker well-formedness.
func (f *Forge) ListBotThreads(_, _ string) ([]forge.Thread, error) {
	var out []forge.Thread
	for _, t := range f.threads {
		if t.Author == f.BotAuthor {
			out = append(out, t)
		}
	}
	return out, nil
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
			f.threads[i].Resolved = true
			return nil
		}
	}
	return nil
}

// CreateThread records a new bot-authored thread with a fresh forge id.
func (f *Forge) CreateThread(_, _ string, marker forge.Marker, _ string) (forge.Thread, error) {
	f.seq++
	t := forge.Thread{
		ID:     fmt.Sprintf("note/%d", 9000+f.seq),
		Marker: marker,
		Author: f.BotAuthor,
	}
	f.threads = append(f.threads, t)
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

// static assertion that the fake implements the port.
var _ forge.Forge = (*Forge)(nil)
