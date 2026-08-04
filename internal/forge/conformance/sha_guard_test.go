package conformance

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/forge/fake"
)

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
