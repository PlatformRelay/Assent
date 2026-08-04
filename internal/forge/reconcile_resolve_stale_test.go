package forge_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

// TestReconcileResolvesNoLongerDesired proves REQ-E4-S04-02: when a finding slot
// is absent from DesiredReviewState, the existing open bot thread is resolved so
// no orphan open findings remain.
func TestReconcileResolvesNoLongerDesired(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	f.SeedThread("note/9001", botID, reviewMarker(), false)

	slot := reviewMarker().Slot
	desired := forge.DesiredReviewState{
		Project:   proj,
		MR:        mrIID,
		ClearSlot: &slot,
	}

	before := f.ThreadCount()
	receipt, err := forge.Reconcile(f, testClock(), desired, forge.Preconditions{})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if got := f.ThreadCount() - before; got != 0 {
		t.Fatalf("clear-slot must not create threads, created %d", got)
	}
	if !f.IsResolved("note/9001") {
		t.Fatal("open bot thread for cleared slot must be resolved")
	}
	if got := f.OpenBotThreadCount(); got != 0 {
		t.Fatalf("no open bot threads must remain for the cleared slot, got %d", got)
	}
	if len(receipt.Operations) != 1 {
		t.Fatalf("expected one thread op recording the resolution, got %+v", receipt.Operations)
	}
	if got := receipt.Operations[0].TargetID; got != "note/9001" {
		t.Fatalf("receipt must reference the resolved thread, got %q", got)
	}
}
