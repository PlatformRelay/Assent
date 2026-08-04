package forge_test

import (
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

// TestReconcileRescanBeforeSuccess proves REQ-E4-S04-03: Reconcile re-lists bot
// threads after writes and fails closed when the forge no longer reflects the
// desired state — never returning a success receipt on a rescan mismatch.
func TestReconcileRescanBeforeSuccess(t *testing.T) {
	t.Run("create-path-mismatch", func(t *testing.T) {
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.RescanListBotThreads = func(_ []forge.Thread) ([]forge.Thread, error) {
			return nil, nil // rescan sees no thread though one was just created
		}

		receipt, err := forge.Reconcile(f, testClock(), reviewState(), forge.Preconditions{})
		if !errors.Is(err, forge.ErrRescanFailed) {
			t.Fatalf("expected ErrRescanFailed, got receipt=%+v err=%v", receipt, err)
		}
		if len(receipt.Operations) != 0 {
			t.Fatalf("must not return a success receipt on rescan mismatch, got %+v", receipt)
		}
	})

	t.Run("clear-slot-mismatch", func(t *testing.T) {
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/9001", botID, reviewMarker(), false)
		slot := reviewMarker().Slot
		desired := forge.DesiredReviewState{
			Project:   proj,
			MR:        mrIID,
			ClearSlot: &slot,
		}
		f.RescanListBotThreads = func(listed []forge.Thread) ([]forge.Thread, error) {
			// Forge still reports the thread open after ResolveThread.
			out := append([]forge.Thread(nil), listed...)
			for i := range out {
				if out[i].ID == "note/9001" {
					out[i].Resolved = false
				}
			}
			return out, nil
		}

		receipt, err := forge.Reconcile(f, testClock(), desired, forge.Preconditions{})
		if !errors.Is(err, forge.ErrRescanFailed) {
			t.Fatalf("expected ErrRescanFailed, got receipt=%+v err=%v", receipt, err)
		}
	})
}
