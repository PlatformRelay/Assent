package forge_test

import (
	"encoding/json"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

// TestReconcilePostsOneThread proves REQ-P4-E1-S06-01: a REVIEW DecisionRecord
// → Reconcile posts EXACTLY ONE resolvable thread carrying the (slot,
// occurrence) marker, and the returned PublicationReceipt records a kind:thread
// op that VALIDATES against publication-receipt.schema.json.
func TestReconcilePostsOneThread(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")

	receipt, err := forge.Reconcile(f, testClock(), reviewState(), forge.Preconditions{})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Exactly one thread created on the forge, carrying the marker.
	if got := f.ThreadCount(); got != 1 {
		t.Fatalf("expected exactly one thread created, got %d", got)
	}
	if got := f.BotThreadCount(); got != 1 {
		t.Fatalf("expected exactly one bot thread, got %d", got)
	}

	// The created bot thread carries the (slot, occurrence) marker verbatim.
	bots, err := f.ListBotThreads(proj, mrIID)
	if err != nil {
		t.Fatal(err)
	}
	if len(bots) != 1 || bots[0].Marker != reviewMarker() {
		t.Fatalf("created thread must carry the review marker, got %+v", bots)
	}

	// Receipt records exactly one kind:thread op with a non-empty targetId.
	if len(receipt.Operations) != 1 {
		t.Fatalf("expected one operation, got %d: %+v", len(receipt.Operations), receipt.Operations)
	}
	op := receipt.Operations[0]
	if op.Kind != "thread" {
		t.Errorf("expected kind:thread, got %q", op.Kind)
	}
	if op.TargetID == "" {
		t.Error("thread op must carry a non-empty targetId")
	}
	if op.PerformedAt != "2026-07-26T10:00:00Z" {
		t.Errorf("performedAt must come from the injected clock, got %q", op.PerformedAt)
	}

	// Receipt validates against the frozen schema.
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateReceipt(t, raw); err != nil {
		t.Fatalf("PublicationReceipt does not validate: %v\n%s", err, raw)
	}
}

// TestReconcileThreadIdempotent proves REQ-P4-E1-S06-02: with the fake already
// holding the thread (same slot/occurrence) from a prior run, Reconcile creates
// ZERO new threads and leaves the existing untouched. ADVERSARIAL: a contributor
// (non-bot) comment carrying a well-formed, schema-valid marker is EXCLUDED by
// the author-identity filter and has zero effect.
func TestReconcileThreadIdempotent(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")

	// Prior-run bot thread with the SAME slot/occurrence marker.
	f.SeedThread("note/9002", botID, reviewMarker(), false)

	// ADVERSARIAL: a contributor (non-bot) comment carrying a SYNTACTICALLY
	// PERFECT, schema-valid marker for the SAME slot/occurrence. It must be
	// invisible to the author-identity-filtered listing and have zero effect.
	f.SeedThread("note/6660", "contributor-mallory", reviewMarker(), false)

	before := f.ThreadCount()
	receipt, err := forge.Reconcile(f, testClock(), reviewState(), forge.Preconditions{})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// ZERO new threads created — the seeded bot thread already occupies the slot.
	if after := f.ThreadCount(); after != before {
		t.Fatalf("rerun must create zero new threads: before=%d after=%d", before, after)
	}
	// Exactly one BOT thread remains (the contributor comment is not counted as
	// occupying the slot for reconciliation purposes).
	if got := f.BotThreadCount(); got != 1 {
		t.Fatalf("expected exactly one bot thread after rerun, got %d", got)
	}

	// The receipt reports the EXISTING bot thread id (note/9002), never the
	// contributor's (note/6660) — proving the contributor comment had zero
	// effect and the reused artifact is the bot's own.
	if len(receipt.Operations) != 1 {
		t.Fatalf("expected one operation, got %d", len(receipt.Operations))
	}
	if got := receipt.Operations[0].TargetID; got != "note/9002" {
		t.Fatalf("receipt must report the existing bot thread id, got %q (contributor id would be note/6660)", got)
	}

	raw, _ := json.Marshal(receipt)
	if err := validateReceipt(t, raw); err != nil {
		t.Fatalf("idempotent receipt must validate: %v\n%s", err, raw)
	}
}

// TestReconcileContributorMarkerAloneCreatesThread proves the author-identity
// filter is what excludes the contributor comment — NOT marker well-formedness.
// With ONLY a contributor comment carrying the matching marker (no bot thread),
// Reconcile still creates a new BOT thread: the contributor artifact is
// invisible, so the slot reads as unoccupied.
func TestReconcileContributorMarkerAloneCreatesThread(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	f.SeedThread("note/6660", "contributor-mallory", reviewMarker(), false)

	_, err := forge.Reconcile(f, testClock(), reviewState(), forge.Preconditions{})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// A NEW bot thread was created despite the matching contributor marker —
	// the contributor comment did not satisfy the slot (author-identity filter).
	if got := f.BotThreadCount(); got != 1 {
		t.Fatalf("expected one bot thread created (contributor marker is invisible), got %d", got)
	}
	if got := f.ThreadCount(); got != 2 {
		t.Fatalf("expected 2 total threads (contributor + new bot), got %d", got)
	}
}
