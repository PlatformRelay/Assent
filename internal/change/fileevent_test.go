package change

import "testing"

// TestFileEventConstructor (REQ-EFE-S02-01) pins that change.FileEvent mints the
// canonical whole-file event shape — File set, Path=="" (the whole-file
// discriminator the S01 matcher keys on), Kind as given — and NOTHING else (no
// Old/New/pos/entryRef/oldPath), so it is the single, drift-free `path==""` minter
// (Judgment call (d)).
func TestFileEventConstructor(t *testing.T) {
	for _, k := range []Kind{KindAdd, KindDelete} {
		got := FileEvent("topics/orders.yaml", k)
		if got.File != "topics/orders.yaml" || got.Kind != k {
			t.Fatalf("FileEvent(%q, %q) = %+v, want File+Kind set", "topics/orders.yaml", k, got)
		}
		if got.Path != "" {
			t.Fatalf("FileEvent must mint the whole-file discriminator Path==\"\", got %q", got.Path)
		}
		// A whole-file event carries no value-level payload: Old/New/positions/
		// entryRef/oldPath/classes/environment must all be zero so it validates as a
		// clean file-event and never drifts from the matcher's `path==""` shape.
		if got.Old != "" || got.New != "" || got.OldPos != nil || got.NewPos != nil ||
			got.EntryRef != "" || got.OldPath != "" || len(got.Classes) != 0 || got.Environment != "" {
			t.Fatalf("FileEvent must carry no value-level payload, got %+v", got)
		}
	}
}

// TestOneSidedLifecyclePresenceSignal pins the EFE-S02/S03 ambiguity invariant:
// presence is nil-vs-non-nil, NEVER len()==0. Empty-but-present ([]byte{}) is
// PRESENT and must not mint a delete; only a true nil side is absent.
func TestOneSidedLifecyclePresenceSignal(t *testing.T) {
	present := []byte("enabled: true\n")
	emptyPresent := make([]byte, 0) // non-nil, len 0 — empty-but-present

	if kind, ok := OneSidedLifecycle(nil, present); !ok || kind != KindAdd {
		t.Fatalf("nil base + present head: got (%q, %v), want (add, true)", kind, ok)
	}
	if kind, ok := OneSidedLifecycle(present, nil); !ok || kind != KindDelete {
		t.Fatalf("present base + nil head: got (%q, %v), want (delete, true)", kind, ok)
	}
	if _, ok := OneSidedLifecycle(present, emptyPresent); ok {
		t.Fatal("empty-but-present head must NOT be a one-sided delete")
	}
	if _, ok := OneSidedLifecycle(emptyPresent, present); ok {
		t.Fatal("empty-but-present base must NOT be a one-sided add")
	}
	if _, ok := OneSidedLifecycle(present, present); ok {
		t.Fatal("both-present must not mint a lifecycle")
	}
	if _, ok := OneSidedLifecycle(nil, nil); ok {
		t.Fatal("both-absent must not mint a lifecycle")
	}
}
