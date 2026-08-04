package conformance

import (
	"errors"
	"fmt"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

const (
	pinSource   = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pinTarget   = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pinDigest   = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	movedTarget = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	movedSource = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
)

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

func armedPre() forge.Preconditions {
	return forge.Preconditions{
		ArmEligible:       true,
		SourceSha:         pinSource,
		TargetSha:         pinTarget,
		MergeResultDigest: pinDigest,
	}
}

// shaGuardOutcome captures the fail-closed result of a SHA-guarded reconcile.
type shaGuardOutcome struct {
	Err       error
	Approvals int
	Merges    int
}

func runSHAGuard(f *fake.Forge) shaGuardOutcome {
	_, err := forge.Reconcile(f, testClock(), approveState(), armedPre())
	return shaGuardOutcome{
		Err:       err,
		Approvals: len(f.Approvals),
		Merges:    len(f.Merges),
	}
}

func assertSHAMoved(out shaGuardOutcome) error {
	if !errors.Is(out.Err, forge.ErrSHAMoved) {
		return fmt.Errorf("want ErrSHAMoved, got %v", out.Err)
	}
	if out.Merges != 0 {
		return fmt.Errorf("zero merges expected, got %d", out.Merges)
	}
	return nil
}

// TestConformanceTargetAdvancedRejected is REQ-E4-S07-01: the target branch tip
// advanced after evaluation pins were taken → Reconcile refuses with the typed
// ErrSHAMoved summary, records zero merges, and performs no approve write (the
// CurrentHeads pre-check fails closed before any forge mutation).
func TestConformanceTargetAdvancedRejected(t *testing.T) {
	for run := 0; run < 2; run++ {
		// Evaluated pins: source+target+digest. Forge CURRENT state: target moved.
		f := fake.New(botID, pinSource, movedTarget, pinDigest)

		out := runSHAGuard(f)
		if err := assertSHAMoved(out); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if out.Approvals != 0 {
			t.Fatalf("run %d: pre-check rejection must record zero approvals, got %d", run, out.Approvals)
		}
	}
}

// TestConformanceSourceMovedRejected is REQ-E4-S07-02: the MR source head moved
// after evaluation → the atomic MergeCAS guard refuses (409/406 mapping on the
// GitLab adapter; ErrSHAMoved here), records zero merges, and leaves at most the
// dangling approval the TOCTOU window permits. The AfterCurrentHeads drift hook
// models the head advancing between the pre-check read and MergeCAS.
func TestConformanceSourceMovedRejected(t *testing.T) {
	for run := 0; run < 2; run++ {
		f := fake.New(botID, pinSource, pinTarget, pinDigest)
		f.AfterCurrentHeads = func(fk *fake.Forge) {
			fk.CurrentSourceSha = movedSource
		}

		out := runSHAGuard(f)
		if err := assertSHAMoved(out); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if out.Approvals != 1 {
			t.Fatalf("run %d: MergeCAS rejection may leave one dangling approval, got %d", run, out.Approvals)
		}
	}
}
