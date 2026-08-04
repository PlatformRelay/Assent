package forge_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

// occStale and occFresh are two distinct occurrence digests for the same slot —
// marker-grammar.md occurrence-supersession adversarial case.
const (
	occStale = "sha256:aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"
	occFresh = "sha256:bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"
)

func supersedeSlot() forge.Slot {
	return forge.Slot{
		Project:  proj,
		MR:       mrIID,
		Rule:     "topic-safety/retention-shrink-challenge",
		Effect:   "challenge",
		EntryRef: "topic-registry:orders.events.v1",
	}
}

func staleMarker() forge.Marker {
	return forge.Marker{
		Slot:       supersedeSlot(),
		Occurrence: occStale,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func freshMarker() forge.Marker {
	return forge.Marker{
		Slot:       supersedeSlot(),
		Occurrence: occFresh,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

// TestReconcileSupersedesStaleOccurrence proves REQ-E4-S04-01: when the slot's
// occurrence changed, Reconcile posts a fresh challenge thread and leaves the old
// occurrence resolved-but-stale — never reusing the stale artifact.
func TestReconcileSupersedesStaleOccurrence(t *testing.T) {
	t.Run("unresolved-stale-superseded", func(t *testing.T) {
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/8001", botID, staleMarker(), false)

		before := f.ThreadCount()
		receipt, err := forge.Reconcile(f, testClock(), desiredThreadFor(freshMarker(), nil), forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}

		if got := f.ThreadCount() - before; got != 1 {
			t.Fatalf("supersede must create exactly one new thread, created %d", got)
		}
		if !f.IsResolved("note/8001") {
			t.Fatal("stale thread note/8001 must be resolved, not reused")
		}
		if f.OpenBotThreadCount() != 1 {
			t.Fatalf("expected one open bot thread (fresh occurrence), got %d", f.OpenBotThreadCount())
		}
		bots, err := f.ListBotThreads(proj, mrIID)
		if err != nil {
			t.Fatal(err)
		}
		var freshID string
		for _, b := range bots {
			if b.Marker.Occurrence == occFresh && !b.Resolved {
				freshID = b.ID
			}
			if b.Marker.Occurrence == occStale && !b.Resolved {
				t.Fatalf("stale occurrence must not remain open, thread %s", b.ID)
			}
		}
		if freshID == "" {
			t.Fatal("expected an open bot thread carrying the fresh occurrence")
		}
		if freshID == "note/8001" {
			t.Fatal("receipt must not reuse the stale thread id")
		}
		if len(receipt.Operations) != 1 || receipt.Operations[0].TargetID != freshID {
			t.Fatalf("receipt must reference the fresh thread, got %+v", receipt.Operations)
		}
	})

	t.Run("resolved-stale-left-untouched", func(t *testing.T) {
		// Reviewer already resolved the old occurrence — it stays resolved-but-stale;
		// a new fresh challenge is still posted for the new occurrence.
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/8001", botID, staleMarker(), true)

		receipt, err := forge.Reconcile(f, testClock(), desiredThreadFor(freshMarker(), nil), forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if !f.IsResolved("note/8001") {
			t.Fatal("reviewer-resolved stale thread must stay resolved")
		}
		if got := f.BotThreadCount(); got != 2 {
			t.Fatalf("expected stale + fresh bot threads, got %d", got)
		}
		if receipt.Operations[0].TargetID == "note/8001" {
			t.Fatal("receipt must reference the newly created fresh thread, not the stale one")
		}
	})
}
