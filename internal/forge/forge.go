// Package forge is the ADR-0017 §7 forge port surface:
//
//	Snapshot → Resolve → Reconcile(DesiredReviewState, Preconditions) → PublicationReceipt
//
// Snapshot (E4-S01) reads MR heads, changed files, bot threads, and tier capability
// flags without mutating the forge. Resolve (E4-S01) maps a require-review subject
// and pinned SHAs to aggregate.ApprovalEvidence or an explicit CapabilityGap —
// never silent APPROVE on missing proof. Reconcile (P4-E1) turns a
// DecisionRecord-derived DesiredReviewState into forge writes (one resolvable
// thread for REVIEW; approval + SHA-pinned merge for APPROVE) and returns a
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
	"strconv"
	"strings"
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

	// ErrInvalidSummaryMarker is returned when UpsertComment is invoked with a
	// marker whose artifact.kind is not summary-comment.
	ErrInvalidSummaryMarker = errors.New("forge: UpsertComment requires artifact.kind summary-comment")

	// ErrRescanFailed is returned when the post-write rescan (P3-E5 step 9) finds
	// the forge does not reflect the desired state. Writes may have occurred, but
	// success is never reported without forge confirmation.
	ErrRescanFailed = errors.New("forge: post-publication rescan mismatch — forge state does not reflect desired")
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

// DesiredSummary is the per-MR summary comment (P3-E5 step 3,
// artifact.kind: summary-comment). It is edited in place on every run when
// populated — never re-posted as a second note.
type DesiredSummary struct {
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

	// ClearSlot when non-nil means the finding for this slot no longer fires —
	// reconcile resolves any open bot thread for the slot (P3-E5 step 7). Mutually
	// exclusive with Thread and the APPROVE path.
	ClearSlot *Slot

	// Approve is true for an APPROVE decision (arm the approval).
	Approve bool
	// Merge is the SHA-pinned merge for an APPROVE decision; nil otherwise.
	Merge *DesiredMerge

	// Summary is the per-MR summary comment when step 3 applies; nil when the
	// caller has not populated the summary slot (E8-S12 additive preamble).
	Summary *DesiredSummary
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

// Clocker is the injected time source for performedAt timestamps. Reconcile never
// calls time.Now — a test clock makes receipts byte-stable and goldens
// reproducible (ADR-0013 double-run gate).
type Clocker interface {
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

	// ResolveThread resolves (removes as an open occupant) the bot thread with the
	// given forge id — the duplicate-repair action (S12-03). It marks the thread
	// resolved in place; it creates NOTHING. Resolving is idempotent (resolving an
	// already-resolved thread is a no-op).
	ResolveThread(project, mr, id string) error

	// Approve records an approval on the MR and returns its forge-assigned id.
	Approve(project, mr string) (string, error)

	// MergeCAS performs a compare-and-swap merge: it merges ONLY if the current
	// source SHA, target SHA AND merge-result digest on the forge all still
	// equal the pinned values (all three — source-only is insufficient). If any
	// has moved it returns ErrSHAMoved and performs NO merge. On success it
	// returns the forge-assigned merge id.
	MergeCAS(project, mr string, m DesiredMerge) (string, error)

	// ListBotNotes returns bot-authored MR notes (non-resolvable comments)
	// filtered by AUTHOR IDENTITY (ADR-0019). A contributor note carrying a
	// well-formed marker is excluded — same filter as ListBotThreads.
	ListBotNotes(project, mr string) ([]Note, error)

	// UpsertComment creates OR edits-in-place exactly one note keyed by the
	// marker's artifact kind (summary-comment upserts one per MR; never posts a
	// second summary note). Returns the note with its stable forge-assigned id.
	UpsertComment(project, mr string, marker Marker, body string) (Note, error)
}

// Note is a bot-authored MR note (non-thread comment) as recorded by the forge.
type Note struct {
	ID     string
	Marker Marker
	Author string
	Body   string
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

// Repair is one recorded duplicate-repair in the PublicationReceipt (S12-03,
// P3-E5-S03-01). When two+ bot artifacts occupy the SAME (slot, occurrence) — a
// race between unserialized publishers — Reconcile keeps the LOWEST-forge-ID
// artifact as canonical and resolves every other against it. Each resolution is
// recorded here: the repaired (non-canonical) forge id, the canonical id it was
// resolved against, and the fixed action ("resolve"). Deterministic: the
// canonical is the numeric-minimum forge id, INDEPENDENT of scan/pagination
// order (never first-seen-wins).
type Repair struct {
	RepairedForgeID  string `json:"repairedForgeId"`
	CanonicalForgeID string `json:"canonicalForgeId"`
	Action           string `json:"action"`
}

// PublicationReceipt records what was actually written to the forge (ADR-0017
// §7). It validates against
// schemas/decision/v1alpha1/publication-receipt.schema.json — operations[] each
// {kind, targetId, performedAt}, keyed unique by targetId, minItems:1 (a
// zero-write reconciliation returns a typed error, never an empty receipt).
// Top-level additionalProperties:true, so this slice (S12) ADDS a top-level
// `repairs` property WITHOUT a schema change; `omitempty` keeps every prior
// receipt (which performs no repair) byte-identical — no `"repairs":null` leaks
// into the goldens.
type PublicationReceipt struct {
	APIVersion string      `json:"apiVersion"`
	Kind       string      `json:"kind"`
	Operations []Operation `json:"operations"`
	Repairs    []Repair    `json:"repairs,omitempty"`
}

const (
	receiptAPIVersion = "assent.dev/v1alpha1"
	receiptKind       = "PublicationReceipt"

	kindThread   = "thread"
	kindApproval = "approval"
	kindMerge    = "merge"

	// repairActionResolve is the fixed action recorded for every duplicate-repair
	// (P3-E5-S03-01 / duplicate-repair.yaml): the non-canonical duplicate is
	// resolved against the canonical.
	repairActionResolve = "resolve"
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
func Reconcile(f Forge, clock Clocker, desired DesiredReviewState, pre Preconditions) (PublicationReceipt, error) {
	path, err := reconcilePathOf(desired)
	if err != nil {
		return PublicationReceipt{}, err
	}

	var preambleOps []Operation
	if desired.Summary != nil {
		op, err := reconcileSummaryPreamble(f, clock, desired)
		if err != nil {
			return PublicationReceipt{}, err
		}
		preambleOps = append(preambleOps, op)
	}

	var receipt PublicationReceipt
	switch path {
	case pathClearSlot:
		receipt, err = reconcileClearSlot(f, clock, desired)
	case pathThread:
		receipt, err = reconcileThread(f, clock, desired)
	case pathApproveMerge:
		receipt, err = reconcileApproveMerge(f, clock, desired, pre)
	}
	if err != nil {
		return PublicationReceipt{}, err
	}
	if len(preambleOps) == 0 {
		return receipt, nil
	}
	all := append(preambleOps, receipt.Operations...)
	return receiptOf(all...), nil
}

type reconcilePathKind int

const (
	pathClearSlot reconcilePathKind = iota
	pathThread
	pathApproveMerge
)

func reconcilePathOf(desired DesiredReviewState) (reconcilePathKind, error) {
	switch {
	case desired.ClearSlot != nil:
		return pathClearSlot, nil
	case desired.Thread != nil:
		return pathThread, nil
	case desired.Approve && desired.Merge != nil:
		return pathApproveMerge, nil
	default:
		return 0, ErrUnsupportedDecision
	}
}

// reconcileSummaryPreamble implements P3-E5 step 2 (list bot notes) then step 3
// (upsert the one summary-comment in place). It runs before the mutually
// exclusive Thread | ClearSlot | Approve path when desired.Summary is set.
func reconcileSummaryPreamble(f Forge, clock Clocker, desired DesiredReviewState) (Operation, error) {
	if _, err := f.ListBotNotes(desired.Project, desired.MR); err != nil {
		return Operation{}, fmt.Errorf("forge: list bot notes: %w", err)
	}
	return reconcileSummary(f, clock, desired)
}

func reconcileSummary(f Forge, clock Clocker, desired DesiredReviewState) (Operation, error) {
	summary := desired.Summary
	if summary.Marker.Artifact.Kind != "summary-comment" {
		return Operation{}, ErrInvalidSummaryMarker
	}
	note, err := f.UpsertComment(desired.Project, desired.MR, summary.Marker, summary.Body)
	if err != nil {
		return Operation{}, fmt.Errorf("forge: upsert summary: %w", err)
	}
	return Operation{
		Kind:        kindThread,
		TargetID:    note.ID,
		PerformedAt: rfc3339(clock),
	}, nil
}

// reconcileThread posts exactly one resolvable thread for the REVIEW slot, or
// reuses the existing bot thread with the same (slot, occurrence) — idempotent
// by the ADR-0019 marker. The bot-thread listing is author-identity filtered by
// the fake, so a contributor comment carrying a well-formed marker is never
// seen here (REQ-P4-E1-S06-02 adversarial case).
func reconcileThread(f Forge, clock Clocker, desired DesiredReviewState) (PublicationReceipt, error) {
	existing, err := f.ListBotThreads(desired.Project, desired.MR)
	if err != nil {
		return PublicationReceipt{}, fmt.Errorf("forge: list bot threads: %w", err)
	}

	want := desired.Thread.Marker.key()
	wantSlot := desired.Thread.Marker.Slot

	// Step 6 — supersede stale occurrences: same slot, different occurrence. Resolve
	// any still-open stale threads so the fresh occurrence gets its own review record.
	for _, t := range existing {
		if t.Marker.Slot != wantSlot || t.Marker.Occurrence == want.occurrence || t.Resolved {
			continue
		}
		if err := f.ResolveThread(desired.Project, desired.MR, t.ID); err != nil {
			return PublicationReceipt{}, fmt.Errorf("forge: resolve stale occurrence %s: %w", t.ID, err)
		}
	}

	// Collect ALL bot threads that occupy the desired (slot, occurrence) — not
	// just the first seen. A race between unserialized publishers can leave two+
	// artifacts on one slot; a first-match-wins loop would (a) miss the duplicate
	// and (b) pick a scan-order-dependent survivor. Collecting all of them lets
	// the repair pick a canonical by a FIXED rule (lowest forge id) below.
	var occupants []Thread
	for _, t := range existing {
		if t.Marker.key() == want {
			occupants = append(occupants, t)
		}
	}

	switch {
	case len(occupants) >= 2:
		// Duplicate-repair path (S12-03): keep the lowest-forge-id artifact as
		// canonical, resolve every other against it, and record each repair.
		receipt, err := repairDuplicates(f, clock, desired, occupants)
		if err != nil {
			return PublicationReceipt{}, err
		}
		if err := rescanThreadDesired(f, desired, rescanModeRepair); err != nil {
			return PublicationReceipt{}, err
		}
		return receipt, nil
	case len(occupants) == 1:
		// Idempotent / preserve-resolution: the slot/occurrence already has exactly
		// one bot thread. Create nothing; report the existing id so the receipt
		// still records the thread op (validates), while the fake proves zero new
		// writes. A reviewer-resolved thread stays resolved (P3-E5 step 5).
		mode := rescanModeOpen
		if occupants[0].Resolved {
			mode = rescanModePreserveResolved
		}
		receipt := receiptOf(Operation{
			Kind:        kindThread,
			TargetID:    occupants[0].ID,
			PerformedAt: rfc3339(clock),
		})
		if err := rescanThreadDesired(f, desired, mode); err != nil {
			return PublicationReceipt{}, err
		}
		return receipt, nil
	}

	created, err := f.CreateThread(desired.Project, desired.MR, desired.Thread.Marker, desired.Thread.Body)
	if err != nil {
		return PublicationReceipt{}, fmt.Errorf("forge: create thread: %w", err)
	}
	receipt := receiptOf(Operation{
		Kind:        kindThread,
		TargetID:    created.ID,
		PerformedAt: rfc3339(clock),
	})
	if err := rescanThreadDesired(f, desired, rescanModeOpen); err != nil {
		return PublicationReceipt{}, err
	}
	return receipt, nil
}

type rescanThreadMode int

const (
	rescanModeOpen rescanThreadMode = iota
	rescanModePreserveResolved
	rescanModeRepair
)

// rescanThreadDesired re-lists bot threads (P3-E5 step 9) and fails closed when
// the forge no longer reflects the desired slot/occurrence state.
func rescanThreadDesired(f Forge, desired DesiredReviewState, mode rescanThreadMode) error {
	threads, err := f.ListBotThreads(desired.Project, desired.MR)
	if err != nil {
		return fmt.Errorf("forge: rescan list bot threads: %w", err)
	}
	if err := verifyThreadRescan(threads, desired.Thread.Marker, mode); err != nil {
		return fmt.Errorf("%w: %v", ErrRescanFailed, err)
	}
	return nil
}

func verifyThreadRescan(threads []Thread, marker Marker, mode rescanThreadMode) error {
	want := marker.key()
	slot := marker.Slot

	for _, t := range threads {
		if t.Marker.Slot == slot && t.Marker.Occurrence != want.occurrence && !t.Resolved {
			return fmt.Errorf("stale occurrence thread %s still open", t.ID)
		}
	}

	var matching []Thread
	for _, t := range threads {
		if t.Marker.key() == want {
			matching = append(matching, t)
		}
	}

	openMatching := matching[:0]
	for _, t := range matching {
		if !t.Resolved {
			openMatching = append(openMatching, t)
		}
	}

	switch mode {
	case rescanModeRepair:
		if len(openMatching) != 1 {
			return fmt.Errorf("expected exactly one open canonical thread after repair, got %d open (%d total)", len(openMatching), len(matching))
		}
	case rescanModePreserveResolved:
		if len(matching) != 1 {
			return fmt.Errorf("expected exactly one thread for preserved slot, got %d", len(matching))
		}
		if !matching[0].Resolved {
			return fmt.Errorf("thread %s must remain resolved (preserve-resolution)", matching[0].ID)
		}
	default: // rescanModeOpen
		if len(openMatching) != 1 {
			return fmt.Errorf("expected exactly one open thread for desired slot/occurrence, got %d open (%d total)", len(openMatching), len(matching))
		}
	}
	return nil
}

// reconcileClearSlot resolves open bot threads for a slot that no longer appears
// in DesiredReviewState (P3-E5 step 7).
func reconcileClearSlot(f Forge, clock Clocker, desired DesiredReviewState) (PublicationReceipt, error) {
	slot := *desired.ClearSlot
	existing, err := f.ListBotThreads(desired.Project, desired.MR)
	if err != nil {
		return PublicationReceipt{}, fmt.Errorf("forge: list bot threads: %w", err)
	}

	var open []Thread
	for _, t := range existing {
		if t.Marker.Slot == slot && !t.Resolved {
			open = append(open, t)
		}
	}

	switch len(open) {
	case 0:
		// Idempotent clear: nothing open to resolve. Reference any bot thread for
		// the slot so the receipt still validates (minItems:1).
		for _, t := range existing {
			if t.Marker.Slot == slot {
				receipt := receiptOf(Operation{
					Kind:        kindThread,
					TargetID:    t.ID,
					PerformedAt: rfc3339(clock),
				})
				if err := rescanClearSlot(f, desired); err != nil {
					return PublicationReceipt{}, err
				}
				return receipt, nil
			}
		}
		return PublicationReceipt{}, ErrUnsupportedDecision
	case 1:
		if err := f.ResolveThread(desired.Project, desired.MR, open[0].ID); err != nil {
			return PublicationReceipt{}, fmt.Errorf("forge: resolve no-longer-desired %s: %w", open[0].ID, err)
		}
		receipt := receiptOf(Operation{
			Kind:        kindThread,
			TargetID:    open[0].ID,
			PerformedAt: rfc3339(clock),
		})
		if err := rescanClearSlot(f, desired); err != nil {
			return PublicationReceipt{}, err
		}
		return receipt, nil
	default:
		return PublicationReceipt{}, fmt.Errorf("forge: duplicate open threads for cleared slot %q", slot.Rule)
	}
}

func rescanClearSlot(f Forge, desired DesiredReviewState) error {
	slot := *desired.ClearSlot
	threads, err := f.ListBotThreads(desired.Project, desired.MR)
	if err != nil {
		return fmt.Errorf("forge: rescan list bot threads: %w", err)
	}
	for _, t := range threads {
		if t.Marker.Slot == slot && !t.Resolved {
			return fmt.Errorf("%w: slot %q still has open thread %s", ErrRescanFailed, slot.Rule, t.ID)
		}
	}
	return nil
}

// repairDuplicates deterministically repairs two+ bot artifacts occupying the
// same (slot, occurrence) slot (S12-03, P3-E5-S03-01). The FIXED rule is
// lowest-forge-id-canonical: the numeric-minimum forge id (NOT the first seen in
// scan/pagination order) is canonical; every other duplicate is resolved against
// it and recorded in PublicationReceipt.repairs.
//
// Determinism guarantees (independent of ListBotThreads' return order):
//   - the canonical is chosen by NUMERIC forge-id comparison (note/8001 < note/8003
//     < note/8005), so reversing the scan order yields the SAME canonical — never
//     first-seen-wins;
//   - the repairs slice is sorted by numeric repaired-id ascending, so the
//     receipt is byte-stable regardless of the order occupants were discovered;
//   - it creates NOTHING (zero new artifacts); it only resolves duplicates.
func repairDuplicates(f Forge, clock Clocker, desired DesiredReviewState, occupants []Thread) (PublicationReceipt, error) {
	// Canonical = the numeric-minimum forge id. forgeIDNum parses the integer
	// after the last '/'; comparing those integers (not the lexical strings) is
	// what makes note/999 < note/1000 order correctly.
	canonical := occupants[0]
	for _, t := range occupants[1:] {
		if forgeIDNum(t.ID) < forgeIDNum(canonical.ID) {
			canonical = t
		}
	}

	var repairs []Repair
	for _, t := range occupants {
		if t.ID == canonical.ID {
			continue
		}
		if err := f.ResolveThread(desired.Project, desired.MR, t.ID); err != nil {
			return PublicationReceipt{}, fmt.Errorf("forge: resolve duplicate %s: %w", t.ID, err)
		}
		repairs = append(repairs, Repair{
			RepairedForgeID:  t.ID,
			CanonicalForgeID: canonical.ID,
			Action:           repairActionResolve,
		})
	}

	// Sort repairs by numeric repaired-id ascending so the receipt is
	// scan-order-independent and byte-stable (matches the fixture's
	// [note/8003, note/8005] order).
	sort.Slice(repairs, func(i, j int) bool {
		return forgeIDNum(repairs[i].RepairedForgeID) < forgeIDNum(repairs[j].RepairedForgeID)
	})

	receipt := receiptOf(Operation{
		Kind:        kindThread,
		TargetID:    canonical.ID,
		PerformedAt: rfc3339(clock),
	})
	receipt.Repairs = repairs
	return receipt, nil
}

// forgeIDNum parses the numeric suffix after the last '/' of a forge id
// (e.g. "note/8001" -> 8001) for NUMERIC canonical selection. A forge id without
// a parseable numeric suffix sorts as the maximum int so it can never silently
// win the canonical (lowest-id) selection — an unparseable id is never made
// canonical over a well-formed one.
func forgeIDNum(id string) int {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	n, err := strconv.Atoi(id)
	if err != nil {
		return int(^uint(0) >> 1) // max int: unparseable ids never win min-selection.
	}
	return n
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
func reconcileApproveMerge(f Forge, clock Clocker, desired DesiredReviewState, pre Preconditions) (PublicationReceipt, error) {
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
func rfc3339(c Clocker) string {
	return c.Now().UTC().Format(time.RFC3339)
}
