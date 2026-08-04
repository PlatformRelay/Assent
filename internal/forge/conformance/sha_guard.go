package conformance

import (
	"errors"
	"fmt"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

const (
	botID       = "assent-bot"
	proj        = "platform/orders-service"
	mrIID       = "482"
	pinSource   = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pinTarget   = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pinDigest   = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	movedTarget = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	movedSource = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func testClock() fixedClock {
	return fixedClock{t: time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)}
}

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
