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
