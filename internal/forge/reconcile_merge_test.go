package forge_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

const (
	pinSource = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pinTarget = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pinDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

// approveState is the DesiredReviewState for an APPROVE decision: approve + a
// SHA-pinned merge honouring all three pins.
func approveState() forge.DesiredReviewState {
	return forge.DesiredReviewState{
		Project: proj,
		MR:      mrIID,
		Approve: true,
		Merge: &forge.DesiredMerge{
			SourceSha:         pinSource,
			TargetSha:         pinTarget,
			MergeResultDigest: pinDigest,
		},
	}
}

// armedPre is the arming precondition MET, pinned to the evaluated SHAs.
func armedPre() forge.Preconditions {
	return forge.Preconditions{
		ArmEligible:       true,
		SourceSha:         pinSource,
		TargetSha:         pinTarget,
		MergeResultDigest: pinDigest,
	}
}

// TestReconcileApprovesAndPinnedMerges proves REQ-P4-E1-S08-01: an APPROVE
// DecisionRecord WITH the arming precondition met → the fake records ONE
// approval + ONE merge whose precondition honours source+target+mergeResultDigest
// (not source-only), and the receipt records kind:approval + kind:merge ops that
// validate against the schema.
func TestReconcileApprovesAndPinnedMerges(t *testing.T) {
	// Fake's CURRENT CAS state matches the pins — nothing has moved.
	f := fake.New(botID, pinSource, pinTarget, pinDigest)

	receipt, err := forge.Reconcile(f, testClock(), approveState(), armedPre())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if len(f.Approvals) != 1 {
		t.Fatalf("expected exactly one approval, got %d", len(f.Approvals))
	}
	if len(f.Merges) != 1 {
		t.Fatalf("expected exactly one merge, got %d", len(f.Merges))
	}

	// Receipt records two ops: approval + merge, distinct targetIds.
	kinds := map[string]string{}
	for _, op := range receipt.Operations {
		if op.TargetID == "" {
			t.Errorf("op %q must carry a non-empty targetId", op.Kind)
		}
		kinds[op.Kind] = op.TargetID
	}
	if _, ok := kinds["approval"]; !ok {
		t.Error("receipt must record a kind:approval op")
	}
	if _, ok := kinds["merge"]; !ok {
		t.Error("receipt must record a kind:merge op")
	}
	if kinds["approval"] == kinds["merge"] {
		t.Error("approval and merge ops must have distinct targetIds (schema keys unique by targetId)")
	}

	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateReceipt(t, raw); err != nil {
		t.Fatalf("PublicationReceipt does not validate: %v\n%s", err, raw)
	}
}

// TestMergeHonoursAllThreePins proves the CAS honours ALL THREE pins, not just
// source: with source+target matching but the merge-result DIGEST moved, the
// merge fails closed — source-only pinning (`merge?sha=` alone) is insufficient
// (ADR-0017 §1).
func TestMergeHonoursAllThreePins(t *testing.T) {
	// Source and target still match the pins; only the merge-result digest moved.
	f := fake.New(botID, pinSource, pinTarget, "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")

	_, err := forge.Reconcile(f, testClock(), approveState(), armedPre())
	if !errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("expected ErrSHAMoved when merge-result digest moved (source-only pin is insufficient), got %v", err)
	}
	if len(f.Merges) != 0 {
		t.Fatalf("no merge must occur when the merge-result digest moved, got %d", len(f.Merges))
	}
	if len(f.Approvals) != 0 {
		t.Fatalf("no approval must occur when the merge-result digest moved (approval is a write), got %d", len(f.Approvals))
	}
}

// TestNoMergeWhenArmingRefused proves REQ-P4-E1-S08-02 (fail-safe): an APPROVE
// decision but arming precondition UNMET → NO approve/merge write occurs; the
// run degrades to advisory/report-only; the fake records ZERO write operations.
func TestNoMergeWhenArmingRefused(t *testing.T) {
	f := fake.New(botID, pinSource, pinTarget, pinDigest)

	pre := armedPre()
	pre.ArmEligible = false // arming REFUSED (unprotected pipeline).

	_, err := forge.Reconcile(f, testClock(), approveState(), pre)
	if !errors.Is(err, forge.ErrArmingRefused) {
		t.Fatalf("expected ErrArmingRefused when arming is unmet, got %v", err)
	}

	// The teeth: ZERO approvals AND zero merges recorded on the fake — no
	// "approved but couldn't merge" limbo. Assert on the fake's recorded state,
	// not only the returned error.
	if len(f.Approvals) != 0 {
		t.Fatalf("arming refused must record zero approvals, got %d", len(f.Approvals))
	}
	if len(f.Merges) != 0 {
		t.Fatalf("arming refused must record zero merges, got %d", len(f.Merges))
	}
}

// TestNoMergeWhenPreconditionsIncomplete proves an incomplete pin set fails
// closed: an armed run whose preconditions are missing the merge-result digest
// performs no write (undecidable/incomplete → no merge).
func TestNoMergeWhenPreconditionsIncomplete(t *testing.T) {
	f := fake.New(botID, pinSource, pinTarget, pinDigest)

	pre := armedPre()
	pre.MergeResultDigest = "" // incomplete: no merge-result digest.

	_, err := forge.Reconcile(f, testClock(), approveState(), pre)
	if !errors.Is(err, forge.ErrIncompletePreconditions) {
		t.Fatalf("expected ErrIncompletePreconditions, got %v", err)
	}
	if len(f.Approvals) != 0 || len(f.Merges) != 0 {
		t.Fatalf("incomplete preconditions must record zero writes: approvals=%d merges=%d", len(f.Approvals), len(f.Merges))
	}
}

// TestMergeRejectsDesiredPinMismatch proves defence-in-depth: even when arming
// is met and preconditions are complete, a desired merge pinned to DIFFERENT
// values than the preconditions fails closed rather than merging an unevaluated
// result.
func TestMergeRejectsDesiredPinMismatch(t *testing.T) {
	f := fake.New(botID, pinSource, pinTarget, pinDigest)

	ds := approveState()
	ds.Merge.TargetSha = "sha256:9999999999999999999999999999999999999999999999999999999999999999"

	_, err := forge.Reconcile(f, testClock(), ds, armedPre())
	if !errors.Is(err, forge.ErrIncompletePreconditions) {
		t.Fatalf("expected ErrIncompletePreconditions on desired/precondition pin mismatch, got %v", err)
	}
	if len(f.Approvals) != 0 || len(f.Merges) != 0 {
		t.Fatalf("pin mismatch must record zero writes: approvals=%d merges=%d", len(f.Approvals), len(f.Merges))
	}
}

// TestReconcileUnsupportedDecision proves an unpopulated DesiredReviewState (no
// thread, no approve+merge) fails closed rather than silently widening to a
// write.
func TestReconcileUnsupportedDecision(t *testing.T) {
	f := fake.New(botID, pinSource, pinTarget, pinDigest)
	_, err := forge.Reconcile(f, testClock(), forge.DesiredReviewState{Project: proj, MR: mrIID}, forge.Preconditions{})
	if !errors.Is(err, forge.ErrUnsupportedDecision) {
		t.Fatalf("expected ErrUnsupportedDecision for an empty desired state, got %v", err)
	}
	if len(f.Approvals) != 0 || len(f.Merges) != 0 || f.ThreadCount() != 0 {
		t.Fatal("an unsupported decision must perform zero writes")
	}
}
