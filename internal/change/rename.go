package change

// RenameMode is the opt-in, per-class switch controlling whether a delete+add pair with an
// identical value is folded into a single KindRename move (ADR-0003 amendment). The default is
// RenameRaw: the fold NEVER happens unless a class explicitly asks for RenameDetect, so a repo
// that has not opted in keeps seeing the raw, stricter delete+add pair.
//
// E1 ships this fold FUNCTION and its config type only. Wiring `renames:` into an authored policy
// envelope's `classes[]` schema (the surface that decides, per class, which changes are in scope
// for a given mode) is E3's declarative policy frontend — this package stays free of the envelope
// parser. A caller (the future engine) scopes a class's changes and calls FoldRenames on them.
type RenameMode string

const (
	// RenameRaw is the default: never fold; a delete+add pair stays two separate Changes.
	RenameRaw RenameMode = "raw"
	// RenameDetect folds an unambiguous, IDENTICAL-value delete+add pair into one KindRename.
	RenameDetect RenameMode = "detect"
)

// FoldRenames is a PURE post-process over one file's ChangeSet. Under RenameDetect it collapses
// each UNAMBIGUOUS (exactly one delete, exactly one add) pair sharing an IDENTICAL rendered value
// into a single KindRename Change — Path = the new (head) path, OldPath = the old (base) path,
// Old == New = the shared value, with positions carried from each side. Any other mode, an opaque
// ChangeSet, or a change list with nothing to fold is returned unchanged (identity).
//
// SAFETY — "never laxer than delete" (ADR-0003 amendment, the attacker-tunable-downgrade guard):
// folding is restricted to pairs whose value is BYTE-IDENTICAL, so a fold only ever collapses a
// pure move where nothing about the value changed. A pair whose values differ AT ALL (a crafted
// "near-rename" sitting just above any similarity threshold) does NOT meet the equality bar and is
// left as the raw, stricter delete+add — so the folded result can never resolve more leniently
// than the unfolded delete would have. The engine (E2/E3) still applies the STRICTER of a class's
// configured delete-effect and rename-effect to a KindRename; E1 guarantees structurally that a
// fold never hides a real value change, which is the property that makes that engine rule safe.
// Ambiguous pairings (a value appearing in more than one delete, or more than one add) are also
// left raw — the fold never guesses which delete paired with which add, keeping it deterministic.
//
// PURE: no clock/env/network/random; order-independent (the result is canonically re-sorted, and
// candidate selection depends only on value identity and 1:1 multiplicity, not input order).
func FoldRenames(cs ChangeSet, mode RenameMode) ChangeSet {
	if mode != RenameDetect || cs.Opaque || len(cs.Changes) == 0 {
		return cs
	}

	// Index delete/add positions by their rendered value. Only a value that appears in EXACTLY
	// one delete and EXACTLY one add is an unambiguous fold candidate.
	delByVal := map[string][]int{}
	addByVal := map[string][]int{}
	for i, c := range cs.Changes {
		switch c.Kind {
		case KindDelete:
			delByVal[c.Old] = append(delByVal[c.Old], i)
		case KindAdd:
			addByVal[c.New] = append(addByVal[c.New], i)
		}
	}

	folded := make(map[int]bool)
	var renames []Change
	for val, dIdx := range delByVal {
		aIdx, ok := addByVal[val]
		if !ok || len(dIdx) != 1 || len(aIdx) != 1 {
			continue // absent on the add side, or ambiguous on either side -> leave raw
		}
		d := cs.Changes[dIdx[0]]
		a := cs.Changes[aIdx[0]]
		if d.Path == a.Path {
			continue // same path is modify territory, not a move
		}
		folded[dIdx[0]] = true
		folded[aIdx[0]] = true
		renames = append(renames, Change{
			File:    d.File,
			Path:    a.Path, // new (head) path
			OldPath: d.Path, // old (base) path
			Kind:    KindRename,
			Old:     d.Old, // == a.New == val (byte-identical, checked via the value index)
			New:     a.New,
			OldPos:  d.OldPos,
			NewPos:  a.NewPos,
		})
	}
	if len(renames) == 0 {
		return cs
	}

	out := make([]Change, 0, len(cs.Changes))
	for i, c := range cs.Changes {
		if !folded[i] {
			out = append(out, c)
		}
	}
	out = append(out, renames...)
	sortChanges(out)
	return ChangeSet{Changes: out}
}
