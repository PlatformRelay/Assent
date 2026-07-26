package forge_test

import (
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

// TestMergeFailsClosedOnMovedSHA is the S07-02 SHA-guard golden (REQ-P4-E1-S07-02):
// a decision pinned to sourceSha/targetSha; a write attempted AFTER the target
// (or source) SHA has moved FAILS CLOSED (SHA-guard rejection / re-evaluation
// required), and NO merge occurs. ADVERSARIAL: advancing the target after
// evaluation yields REJECTION, not a silent merge of an unevaluated result.
//
// Both directions are proven — moving ONLY the target (with the source still
// pinned) and moving ONLY the source (with the target still pinned) — so a
// source-only CAS (`merge?sha=` alone) is demonstrably insufficient.
func TestMergeFailsClosedOnMovedSHA(t *testing.T) {
	cases := []struct {
		name                        string
		curSource, curTarget, curDg string
	}{
		{
			// The MR head is still the evaluated source, but the TARGET branch
			// advanced after evaluation. A source-only pin would merge here — the
			// SHA-guard must reject.
			name:      "target-moved",
			curSource: pinSource,
			curTarget: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			curDg:     pinDigest,
		},
		{
			// The source (MR head) advanced after evaluation; the target is
			// unchanged. Must also reject — merging would publish an unevaluated head.
			name:      "source-moved",
			curSource: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			curTarget: pinTarget,
			curDg:     pinDigest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arm and pin to the EVALUATED values; the fake's CURRENT state has
			// moved (per the case) since evaluation.
			f := fake.New(botID, tc.curSource, tc.curTarget, tc.curDg)

			_, err := forge.Reconcile(f, testClock(), approveState(), armedPre())
			if !errors.Is(err, forge.ErrSHAMoved) {
				t.Fatalf("expected SHA-guard rejection (ErrSHAMoved) when %s, got %v", tc.name, err)
			}
			// The teeth: NO write occurred — assert on the fake's recorded state,
			// not merely the error. A silent merge of an unevaluated result is the
			// exact failure this golden guards against. Crucially the APPROVAL (a
			// write) must ALSO be zero: the SHA-guard runs BEFORE the approval, so
			// SHA-moved leaves no "approved-but-couldn't-merge" limbo (P0: no write
			// when SHA moved).
			if len(f.Merges) != 0 {
				t.Fatalf("%s: no merge must occur on SHA-guard rejection, got %d merges", tc.name, len(f.Merges))
			}
			if len(f.Approvals) != 0 {
				t.Fatalf("%s: no approval must occur on SHA-guard rejection (approval is a write), got %d", tc.name, len(f.Approvals))
			}
		})
	}
}

// TestMergeFailsClosedOnTOCTOURace proves the ATOMIC MergeCAS guard (not just
// the CurrentHeads pre-check) fails closed. It drives an APPROVE through a fake
// whose head advances in the TOCTOU window BETWEEN the pre-check read and the
// MergeCAS call: the pre-check sees the still-pinned heads and passes, then the
// target moves, then MergeCAS observes the drift and must reject with ErrSHAMoved
// and record ZERO merges.
//
// This is the safety net the pre-check would otherwise mask: if a future real
// adapter weakened MergeCAS to a source-only CAS, THIS test (and only this test)
// would catch it, because it is the only path that reaches MergeCAS with the
// pre-check already satisfied. It exercises the atomic-guard error branch in
// reconcileApproveMerge that every non-racing test short-circuits.
func TestMergeFailsClosedOnTOCTOURace(t *testing.T) {
	// The fake starts pinned to the evaluated heads — the CurrentHeads pre-check
	// will pass. The hook then advances the TARGET after the read, so the atomic
	// MergeCAS sees a moved target.
	f := fake.New(botID, pinSource, pinTarget, pinDigest)
	f.AfterCurrentHeads = func(fk *fake.Forge) {
		fk.CurrentTargetSha = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	}

	_, err := forge.Reconcile(f, testClock(), approveState(), armedPre())
	if !errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("atomic MergeCAS guard must reject a head that moved in the TOCTOU window, got %v", err)
	}
	// The teeth: the atomic guard let NO merge through of the unevaluated head.
	if len(f.Merges) != 0 {
		t.Fatalf("no merge must occur when MergeCAS rejects the raced head, got %d merges", len(f.Merges))
	}
	// The approval WAS recorded (pre-check passed, so Approve ran) — the
	// deliberate dangling-approval case the reconcileApproveMerge docstring now
	// documents. The SAFETY invariant this test guards (no merge of a moved SHA)
	// holds; the dangling approval is an S10/adapter concern.
	if len(f.Approvals) != 1 {
		t.Fatalf("expected exactly one dangling approval from the raced path, got %d", len(f.Approvals))
	}
}

// TestMergeFailsClosedDoubleRun proves the SHA-guard rejection is deterministic:
// two runs against the same moved-SHA state both reject with ErrSHAMoved and
// both record zero merges (double-run stability, ADR-0013).
func TestMergeFailsClosedDoubleRun(t *testing.T) {
	movedTarget := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	for i := 0; i < 2; i++ {
		f := fake.New(botID, pinSource, movedTarget, pinDigest)
		_, err := forge.Reconcile(f, testClock(), approveState(), armedPre())
		if !errors.Is(err, forge.ErrSHAMoved) {
			t.Fatalf("run %d: expected ErrSHAMoved, got %v", i, err)
		}
		if len(f.Merges) != 0 {
			t.Fatalf("run %d: zero merges expected, got %d", i, len(f.Merges))
		}
		if len(f.Approvals) != 0 {
			t.Fatalf("run %d: zero approvals expected, got %d", i, len(f.Approvals))
		}
	}
}
