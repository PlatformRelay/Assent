package change

import (
	"bytes"
	"testing"
)

// renameCS builds the delete+add ChangeSet the fold operates on, straight from the real differ
// (so the inputs carry genuine positions/renders, not hand-built literals).
func renameCS(t *testing.T, base, head string) ChangeSet {
	t.Helper()
	cs, err := Diff("f.yaml", []byte(base), []byte(head))
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("fixture must be decidable, got opaque: %s", cs.OpaqueReason)
	}
	return cs
}

func countKind(cs ChangeSet, k Kind) int {
	n := 0
	for _, c := range cs.Changes {
		if c.Kind == k {
			n++
		}
	}
	return n
}

// REQ-E1-S02-01 — with the default (RenameRaw), an identical-value delete+add pair is returned
// UNCHANGED: two Changes (delete, add), never a rename. Proves the opt-in default is genuinely
// raw, not detect-by-default. Both RenameRaw and the zero value ("") must behave this way.
func TestRenameFoldDefaultRaw(t *testing.T) {
	cs := renameCS(t, "old_name: 1\nkeep: 9\n", "new_name: 1\nkeep: 9\n")
	if countKind(cs, KindDelete) != 1 || countKind(cs, KindAdd) != 1 {
		t.Fatalf("fixture should be one delete + one add, got %+v", cs.Changes)
	}
	for _, mode := range []RenameMode{RenameRaw, RenameMode("")} {
		got := FoldRenames(cs, mode)
		if countKind(got, KindRename) != 0 {
			t.Errorf("mode %q must not fold (raw default), got a rename: %+v", mode, got.Changes)
		}
		if len(got.Changes) != len(cs.Changes) {
			t.Errorf("mode %q must be identity, changed length %d -> %d", mode, len(cs.Changes), len(got.Changes))
		}
	}
}

// REQ-E1-S02-02 — with RenameDetect, an identical-value delete@A / add@B pair folds into exactly
// one KindRename carrying oldPath:A, newPath:B (Path), and the shared value; positions come from
// each side. Proven by a byte-stable golden that also double-runs.
func TestRenameFoldDetect(t *testing.T) {
	cs := renameCS(t, "old_name: 1\nkeep: 9\n", "new_name: 1\nkeep: 9\n")
	got := FoldRenames(cs, RenameDetect)
	if len(got.Changes) != 1 {
		t.Fatalf("expected the pair to fold to one rename, got %d: %+v", len(got.Changes), got.Changes)
	}
	r := got.Changes[0]
	if r.Kind != KindRename {
		t.Errorf("Kind = %q, want rename", r.Kind)
	}
	if r.OldPath != "/old_name" || r.Path != "/new_name" {
		t.Errorf("oldPath/path = %q/%q, want /old_name//new_name", r.OldPath, r.Path)
	}
	if r.Old != "1" || r.New != "1" {
		t.Errorf("old/new = %q/%q, want the shared value 1/1", r.Old, r.New)
	}
	if r.OldPos == nil || r.NewPos == nil {
		t.Fatalf("a rename must carry positions on both sides, got old=%v new=%v", r.OldPos, r.NewPos)
	}
	const golden = `{"changes":[{"file":"f.yaml","kind":"rename","new":"1","newPos":{"column":11,"line":1},"old":"1","oldPath":"/old_name","oldPos":{"column":11,"line":1},"path":"/new_name"}],"opaque":false}`
	if g := string(canonicalJSON(t, got)); g != golden {
		t.Errorf("golden mismatch:\n got: %s\nwant: %s", g, golden)
	}
	// Double-run byte-stable.
	if !bytes.Equal(canonicalJSON(t, FoldRenames(cs, RenameDetect)), canonicalJSON(t, FoldRenames(cs, RenameDetect))) {
		t.Error("fold double-run diverged")
	}
}

// REQ-E1-S02-03 — the fold can NEVER resolve more leniently than the raw delete. This story folds
// ONLY on byte-identical values, so:
//   - a delete+add pair whose values DIFFER (a crafted near-rename) is NOT folded — it stays the
//     raw, stricter delete+add, so a real value change is never hidden behind a rename;
//   - an exactly-equal pair folds (pure move, no value change).
//
// Because a fold only ever collapses a zero-value-change move, the folded entry carries the same
// deleted value the raw delete would have, and the engine's "stricter of delete/rename effect"
// rule (E2/E3) can never be dodged by folding.
func TestRenameFoldNeverLaxerThanDelete(t *testing.T) {
	// Values differ by one digit — just enough that this is NOT a pure move.
	near := renameCS(t, "old_name: 1\nkeep: 9\n", "new_name: 2\nkeep: 9\n")
	got := FoldRenames(near, RenameDetect)
	if countKind(got, KindRename) != 0 {
		t.Fatalf("a value-changing delete+add must NOT fold (never laxer than delete), got %+v", got.Changes)
	}
	if countKind(got, KindDelete) != 1 || countKind(got, KindAdd) != 1 {
		t.Errorf("the raw, stricter delete+add must survive unfolded, got %+v", got.Changes)
	}

	// Control: the byte-identical pair DOES fold, confirming the difference above is what blocked it.
	exact := renameCS(t, "old_name: 1\nkeep: 9\n", "new_name: 1\nkeep: 9\n")
	if countKind(FoldRenames(exact, RenameDetect), KindRename) != 1 {
		t.Error("an identical-value pair should fold to a rename")
	}

	// Ambiguous: two deletes and two adds all sharing value "1" -> no guessing, left raw.
	ambig := renameCS(t, "a: 1\nb: 1\nkeep: 9\n", "c: 1\nd: 1\nkeep: 9\n")
	amb := FoldRenames(ambig, RenameDetect)
	if countKind(amb, KindRename) != 0 {
		t.Errorf("ambiguous 2:2 pairing must not fold (deterministic, no guessing), got %+v", amb.Changes)
	}
}

// REQ-E1-S02-04 — the fold is order-independent: shuffling the input ChangeSet's entry order
// yields a byte-identical folded result after canonical sort. (Purity of the package is proven by
// TestCorePurity over internal/change/**.)
func TestRenameFoldOrderIndependent(t *testing.T) {
	// A doc with two independent renames + one untouched key, so ordering actually varies.
	cs := renameCS(t,
		"alpha: 1\nbeta: 2\nkeep: 9\n",
		"alpha2: 1\nbeta2: 2\nkeep: 9\n",
	)
	forward := FoldRenames(cs, RenameDetect)

	// Reverse the input change order and fold again.
	rev := ChangeSet{Changes: make([]Change, len(cs.Changes))}
	for i, c := range cs.Changes {
		rev.Changes[len(cs.Changes)-1-i] = c
	}
	reversed := FoldRenames(rev, RenameDetect)

	if !bytes.Equal(canonicalJSON(t, forward), canonicalJSON(t, reversed)) {
		t.Errorf("fold is order-dependent:\n forward:  %s\n reversed: %s",
			canonicalJSON(t, forward), canonicalJSON(t, reversed))
	}
	if countKind(forward, KindRename) != 2 {
		t.Errorf("expected two independent renames to fold, got %+v", forward.Changes)
	}
}
