package conformance

import (
	"errors"
	"fmt"

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
