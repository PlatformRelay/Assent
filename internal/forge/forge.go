// Package forge is the P4-E1 walking-skeleton Reconcile port (ADR-0017 §7):
// it turns a DecisionRecord-derived DesiredReviewState into forge writes
// (one resolvable thread for a REVIEW decision; an approval + a SHA-pinned
// merge for an APPROVE decision) against a forge, and returns a
// PublicationReceipt recording what was actually written.
//
// This package lives OUTSIDE internal/core (the core purity rule does not apply
// here — this is the side-effecting write edge). It is nonetheless kept
// DETERMINISTIC and side-effect-free EXCEPT through the injected Forge: the
// clock is injected (never time.Now inside Reconcile), and every forge mutation
// goes through the Forge interface so a test fake is the only substrate — there
// is no live infra in this lane (S06/S08/S07-02).
//
// FAIL-CLOSED is the invariant this package exists to prove. A write must NEVER
// occur when: arming is unmet (Preconditions.ArmEligible == false), the pinned
// source OR target SHA has moved since evaluation, or the preconditions are
// incomplete. Every such state returns a typed error and performs ZERO writes —
// it never fabricates a "receipt with no operations" (the frozen
// publication-receipt schema requires operations minItems:1, so an empty-ops
// receipt is not even representable) and never fabricates a placeholder
// operation (that would record a write that did not happen — the exact
// silent-widening this lane forbids).
package forge

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Sentinel errors for the fail-closed axes. Callers (and tests) branch on these
// with errors.Is; they are the machine-readable proof that a refusal was a
// deliberate fail-closed decision, not an incidental failure.
var (
	// ErrArmingRefused is returned from the APPROVE path when the injected
	// arming precondition (ArmEligible) is not met. No approve/merge write
	// occurs — the run degrades to advisory/report-only (ADR-0015 §8,
	// REQ-P4-E1-S08-02).
	ErrArmingRefused = errors.New("forge: arming precondition unmet — advisory-only, no approve/merge write")

	// ErrSHAMoved is returned when a pinned source OR target SHA has moved on
	// the forge since evaluation. The compare-and-swap merge fails closed; no
	// merge occurs (ADR-0015 §2, ADR-0017 §1, REQ-P4-E1-S07-02).
	ErrSHAMoved = errors.New("forge: pinned SHA moved since evaluation — SHA-guard rejection, re-evaluation required")

	// ErrIncompletePreconditions is returned when the preconditions required to
	// arm a merge are not fully populated (e.g. a missing source/target SHA or
	// merge-result digest). Undecidable/incomplete state fails closed (no write).
	ErrIncompletePreconditions = errors.New("forge: incomplete merge preconditions — cannot arm a compare-and-swap merge")

	// ErrUnsupportedDecision is returned when a DesiredReviewState carries a
	// decision Reconcile does not handle in this slice. Fail closed rather than
	// guess (an unknown decision must never widen to a write).
	ErrUnsupportedDecision = errors.New("forge: unsupported decision for Reconcile")
)

// Marker is the ADR-0019 correlation marker embedded in a bot-authored forge
// artifact. It carries EXACTLY the four frozen concepts (slot, occurrence,
// decision, artifact) from
// docs/contracts/p3-e5-publication-protocol/marker-grammar.schema.json. It is
// correlation metadata only — never decision input or authorization evidence.
// A thread is idempotent by (Slot, Occurrence).
type Marker struct {
	Slot       Slot
	Occurrence string // sha256:<64hex> content digest (ADR-0019 hash).
	Decision   string // sha256:<64hex> content digest (ADR-0019 hash).
	Artifact   Artifact
}

// Slot is the stable finding identity (ADR-0019). Two markers describe the same
// slot iff every present field is equal — this is what makes thread
// idempotence by (slot, occurrence) well-defined.
type Slot struct {
	Project  string
	MR       string
	Rule     string
	Effect   string // comment | challenge | block | require-review
	EntryRef string // optional governed-subject identity; "" when absent.
}

// Artifact is the marker's artifact descriptor (kind + grammar schema version).
type Artifact struct {
	Kind          string // finding-thread | summary-comment
	SchemaVersion string // v1alpha1
}

// key is the (slot, occurrence) idempotence key for a thread. Two desired
// threads with equal key describe the same slot/occurrence and must upsert in
// place — never duplicate.
type key struct {
	slot       Slot
	occurrence string
}

func (m Marker) key() key { return key{slot: m.Slot, occurrence: m.Occurrence} }

// DesiredThread is a single desired resolvable thread derived from a REVIEW
// decision: the marker that correlates it to its slot/occurrence, and the body
// bytes the thread should carry. In this slice a REVIEW yields exactly one
// desired thread.
type DesiredThread struct {
	Marker Marker
	Body   string
}

// DesiredMerge is the SHA-pinned merge derived from an APPROVE decision. It
// carries ALL THREE compare-and-swap values (ADR-0017 §1, ADR-0015 §2): a
// source-only pin (`merge?sha=` alone) is INSUFFICIENT, so SourceSha, TargetSha
// AND MergeResultDigest are all required and all checked at merge time.
type DesiredMerge struct {
	SourceSha         string
	TargetSha         string
	MergeResultDigest string
}

// DesiredReviewState is the derived intent Reconcile publishes (ADR-0017 §7).
// It is derived from the DecisionRecord upstream: a REVIEW decision populates
// exactly one Thread; an APPROVE decision populates Approve+Merge. Exactly one
// of {Thread} or {Approve && Merge} is set per this one-slot slice.
type DesiredReviewState struct {
	// Project/MR identify the merge request the writes target on the forge.
	Project string
	MR      string

	// Thread is the single desired thread for a REVIEW decision; nil otherwise.
	Thread *DesiredThread

	// Approve is true for an APPROVE decision (arm the approval).
	Approve bool
	// Merge is the SHA-pinned merge for an APPROVE decision; nil otherwise.
	Merge *DesiredMerge
}

// Preconditions carry the out-of-band arming decision plus the pinned SHAs and
// merge-result digest the compare-and-swap merge honours (ADR-0017 §1, ADR-0015
// §2). ArmEligible is INJECTED TEST DATA in this lane — the S05
// PreconditionReport.ArmEligible bool passed straight in.
//
// D-034 SEAM (arming gate): ArmEligible here is consumed as injected data
// against the in-memory fake; this path NEVER calls cmd/assent's
// readPipelineDescription (the INSECURE-PLACEHOLDER env reader) and there is no
// real forge write behind it. Before ArmEligible is ever allowed to gate a REAL
// merge against a live forge, that INSECURE-PLACEHOLDER reader MUST be replaced
// by real protected-source verification (D-034) — a later slice. If a caller
// needs the real reader here, STOP: this lane must not wire it.
type Preconditions struct {
	// ArmEligible is the S05 arming decision, injected. When false the APPROVE
	// path performs no approve/merge write (ErrArmingRefused).
	ArmEligible bool

	// SourceSha, TargetSha, MergeResultDigest are the evaluated pins the merge
	// compare-and-swap honours. All three are required to arm a merge; any
	// missing value is an incomplete precondition and fails closed.
	SourceSha         string
	TargetSha         string
	MergeResultDigest string
}

// completeForMerge reports whether every pin needed for a compare-and-swap
// merge is present. An incomplete precondition fails closed — a merge is never
// attempted against a partially-pinned target.
func (p Preconditions) completeForMerge() bool {
	return p.SourceSha != "" && p.TargetSha != "" && p.MergeResultDigest != ""
}

// Clock is the injected time source for performedAt timestamps. Reconcile never
// calls time.Now — a test clock makes receipts byte-stable and goldens
// reproducible (ADR-0013 double-run gate).
type Clock interface {
	Now() time.Time
}

// Forge is the side-effecting port Reconcile writes through (ADR-0011/ADR-0017
// §7). The in-memory fake is the only implementation in this lane; a real
// GitLab adapter is a later (infra-gated) slice. Every method that lists
// existing bot artifacts filters by AUTHOR IDENTITY (ADR-0019): a non-bot
// (contributor) artifact is INVISIBLE to reconciliation regardless of what
// marker it carries.
type Forge interface {
	// ListBotThreads returns the threads authored by the configured bot/service
	// account on the given MR. It MUST filter by author identity: a contributor
	// comment carrying a well-formed marker is excluded and has zero effect on
	// reconciliation (ADR-0019 / P3-E5-S01-04).
	ListBotThreads(project, mr string) ([]Thread, error)

	// CurrentHeads returns the forge's CURRENT source SHA, target SHA and
	// merge-result digest for the MR. Reconcile reads these BEFORE any write so
	// that a SHA drift fails closed with ZERO writes — the approval must not be
	// recorded when the merge would be SHA-rejected (P0: no write when SHA moved).
	// MergeCAS still re-checks atomically at merge time as the final guard.
	CurrentHeads(project, mr string) (source, target, digest string, err error)

	// CreateThread posts a new resolvable thread carrying the marker and body,
	// authored by the bot, and returns its forge-assigned id.
	CreateThread(project, mr string, marker Marker, body string) (Thread, error)

	// Approve records an approval on the MR and returns its forge-assigned id.
	Approve(project, mr string) (string, error)

	// MergeCAS performs a compare-and-swap merge: it merges ONLY if the current
	// source SHA, target SHA AND merge-result digest on the forge all still
	// equal the pinned values (all three — source-only is insufficient). If any
	// has moved it returns ErrSHAMoved and performs NO merge. On success it
	// returns the forge-assigned merge id.
	MergeCAS(project, mr string, m DesiredMerge) (string, error)
}

// Thread is a forge thread as recorded by the forge: its forge-assigned id, the
// marker it carries, and its author identity (so listing can filter to the bot).
type Thread struct {
	ID       string
	Marker   Marker
	Author   string
	Resolved bool
}

// Operation is one recorded write in the PublicationReceipt (thread | approval |
// merge), keyed unique by TargetID.
type Operation struct {
	Kind        string `json:"kind"`
	TargetID    string `json:"targetId"`
	PerformedAt string `json:"performedAt"`
}

// PublicationReceipt records what was actually written to the forge (ADR-0017
// §7). It validates against
// schemas/decision/v1alpha1/publication-receipt.schema.json — operations[] each
// {kind, targetId, performedAt}, keyed unique by targetId, minItems:1 (a
// zero-write reconciliation returns a typed error, never an empty receipt).
// Top-level additionalProperties:true, so a later slice (S12) may ADD a
// top-level `repairs` property without a schema change — this lane emits none.
type PublicationReceipt struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Operations []Operation `json:"operations"`
}

const (
	receiptAPIVersion = "assent.dev/v1alpha1"
	receiptKind       = "PublicationReceipt"

	kindThread   = "thread"
	kindApproval = "approval"
	kindMerge    = "merge"
)

// Reconcile publishes the DesiredReviewState to the forge and returns a
// PublicationReceipt of what was written (ADR-0017 §7). It is the single write
// entry point and enforces every fail-closed axis:
//
//   - REVIEW (Thread set): idempotent by (slot, occurrence). If a bot thread
//     already exists for that key, ZERO new threads are created and the
//     existing thread's id is reported; else exactly one thread is created.
//   - APPROVE (Approve + Merge set): gated on ArmEligible AND a complete set of
//     pins AND a compare-and-swap that honours source+target+mergeResultDigest.
//     Any refusal returns a typed error with ZERO writes.
func Reconcile(f Forge, clock Clock, desired DesiredReviewState, pre Preconditions) (PublicationReceipt, error) {
	switch {
	case desired.Thread != nil:
		return reconcileThread(f, clock, desired)
	case desired.Approve && desired.Merge != nil:
		return reconcileApproveMerge(f, clock, desired, pre)
	default:
		return PublicationReceipt{}, ErrUnsupportedDecision
	}
}

// reconcileThread posts exactly one resolvable thread for the REVIEW slot, or
// reuses the existing bot thread with the same (slot, occurrence) — idempotent
// by the ADR-0019 marker. The bot-thread listing is author-identity filtered by
// the fake, so a contributor comment carrying a well-formed marker is never
// seen here (REQ-P4-E1-S06-02 adversarial case).
func reconcileThread(f Forge, clock Clock, desired DesiredReviewState) (PublicationReceipt, error) {
	existing, err := f.ListBotThreads(desired.Project, desired.MR)
	if err != nil {
		return PublicationReceipt{}, fmt.Errorf("forge: list bot threads: %w", err)
	}

	want := desired.Thread.Marker.key()
	for _, t := range existing {
		if t.Marker.key() == want {
			// Idempotent: the slot/occurrence already has a bot thread. Create
			// nothing; report the existing id so the receipt still records the
			// thread op (validates), while the fake proves zero new writes.
			return receiptOf(Operation{
				Kind:        kindThread,
				TargetID:    t.ID,
				PerformedAt: rfc3339(clock),
			}), nil
		}
	}

	created, err := f.CreateThread(desired.Project, desired.MR, desired.Thread.Marker, desired.Thread.Body)
	if err != nil {
		return PublicationReceipt{}, fmt.Errorf("forge: create thread: %w", err)
	}
	return receiptOf(Operation{
		Kind:        kindThread,
		TargetID:    created.ID,
		PerformedAt: rfc3339(clock),
	}), nil
}

// reconcileApproveMerge approves and SHA-pinned-merges the MR — but ONLY when
// every gate passes, in fail-closed order:
//
//  1. arming (injected ArmEligible) — else ErrArmingRefused, zero writes;
//  2. complete pins (source+target+mergeResultDigest all present) — else
//     ErrIncompletePreconditions, zero writes;
//  3. the merge desire must itself carry the same three pins (defence in depth
//     against a desired/precondition mismatch) — else ErrIncompletePreconditions;
//  4. the SHA-guard: read the forge's CURRENT heads and reject with ErrSHAMoved
//     BEFORE any write if the source, target OR merge-result digest has drifted;
//  5. approve, then compare-and-swap merge honouring all three pins — MergeCAS
//     re-checks atomically as the final guard.
//
// The pre-write refusals (arming, incomplete pins, pin mismatch, and the
// CurrentHeads SHA-guard at step 4) are ALL checked BEFORE the approval, so each
// of them records ZERO approvals AND zero merges.
//
// The one exception is the atomic MergeCAS guard at step 5: if the head moves in
// the TOCTOU window BETWEEN the CurrentHeads pre-check and MergeCAS, the approval
// has already been recorded, so this path can leave a DANGLING approval with NO
// merge. That is deliberate and matches real GitLab (approve-then-merge?sha=,
// no rollback); the dangling approval is cleared by the forge's "remove approvals
// on new push" setting — an S10/adapter concern, not this engine slice.
//
// The P0 SAFETY invariant that ALWAYS holds is the one that matters: NO merge of
// a moved/unevaluated SHA (MergeCAS re-checks all three pins atomically and fails
// closed), and NO write of any kind when arming is unmet. A dangling approval is
// not a merge and does not widen what was merged — the fail-closed direction is
// intact.
func reconcileApproveMerge(f Forge, clock Clock, desired DesiredReviewState, pre Preconditions) (PublicationReceipt, error) {
	// D-034 arming gate: ArmEligible is injected data (S05 report), never the
	// INSECURE-PLACEHOLDER env reader. Fail closed when unset.
	if !pre.ArmEligible {
		return PublicationReceipt{}, ErrArmingRefused
	}
	if !pre.completeForMerge() {
		return PublicationReceipt{}, ErrIncompletePreconditions
	}
	// The desired merge must be pinned to the SAME three values the
	// preconditions carry; a mismatch is an incomplete/inconsistent pin and
	// fails closed rather than merging an unevaluated result.
	m := *desired.Merge
	if m.SourceSha != pre.SourceSha || m.TargetSha != pre.TargetSha || m.MergeResultDigest != pre.MergeResultDigest {
		return PublicationReceipt{}, ErrIncompletePreconditions
	}

	// SHA-guard BEFORE any write (P0: no write when SHA moved). Read the forge's
	// current heads and reject with ErrSHAMoved — recording ZERO approvals and
	// zero merges — if the source, target OR merge-result digest has drifted
	// since evaluation. Without this pre-check the approval (a write) would be
	// recorded before MergeCAS's atomic guard could reject the merge, leaving an
	// "approved-but-couldn't-merge" limbo the fail-closed invariant forbids.
	// MergeCAS re-checks atomically below as the final guard against a TOCTOU
	// race, so the two together are belt-and-braces, not redundant.
	curSource, curTarget, curDigest, err := f.CurrentHeads(desired.Project, desired.MR)
	if err != nil {
		return PublicationReceipt{}, fmt.Errorf("forge: read current heads: %w", err)
	}
	if curSource != m.SourceSha || curTarget != m.TargetSha || curDigest != m.MergeResultDigest {
		return PublicationReceipt{}, ErrSHAMoved
	}

	approvalID, err := f.Approve(desired.Project, desired.MR)
	if err != nil {
		return PublicationReceipt{}, fmt.Errorf("forge: approve: %w", err)
	}

	mergeID, err := f.MergeCAS(desired.Project, desired.MR, m)
	if err != nil {
		// SHA moved (or any CAS failure): fail closed. The approval was recorded
		// but no merge occurred; the caller must re-evaluate. Surface the error
		// so no receipt claims a merge that did not happen.
		return PublicationReceipt{}, err
	}

	return receiptOf(
		Operation{Kind: kindApproval, TargetID: approvalID, PerformedAt: rfc3339(clock)},
		Operation{Kind: kindMerge, TargetID: mergeID, PerformedAt: rfc3339(clock)},
	), nil
}

// receiptOf builds a PublicationReceipt from the given operations, sorted by a
// total key (targetId) so the receipt is deterministic and its operations are
// keyed-unique-by-targetId order-independent (ADR-0017 §9).
func receiptOf(ops ...Operation) PublicationReceipt {
	sorted := make([]Operation, len(ops))
	copy(sorted, ops)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TargetID < sorted[j].TargetID })
	return PublicationReceipt{
		APIVersion: receiptAPIVersion,
		Kind:       receiptKind,
		Operations: sorted,
	}
}

// rfc3339 formats the injected clock as an RFC3339 UTC timestamp for
// performedAt. UTC + a fixed layout keeps goldens byte-stable.
func rfc3339(c Clock) string {
	return c.Now().UTC().Format(time.RFC3339)
}
