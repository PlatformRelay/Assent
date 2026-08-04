package change

// FileEvent mints the canonical WHOLE-FILE lifecycle event (EFE-S02, Judgment
// call (d)): a Change whose Path is "" — the whole-file discriminator the S01
// fileEvents matcher (internal/core/aggregate.matchChanges) and its E6 mirror key
// on — carrying only File and Kind, and NO value-level payload (Old/New/positions/
// entryRef). It is the SINGLE place a `path==""` Change is minted: the minter, the
// matcher, and the mirror must agree on this shape, so a whole-file add/delete is
// never hand-rolled elsewhere (the evaldecode "one canonical decoder or the
// fail-open reopens" lesson).
//
// Callers pass a lifecycle Kind — KindAdd (a file present only on head) or
// KindDelete (a file present only on base). The loader accepts fileEvents kinds ⊆
// {add, delete} and rejects modify/rename at load (S01, Judgment call (b)), so a
// FileEvent minted with any other Kind can never be selected by a loaded rule.
//
// PURE (no clock/rand/env/network): a deterministic function of its inputs, so it
// is TestCorePurity-safe alongside the rest of internal/change.
func FileEvent(file string, kind Kind) Change {
	return Change{File: file, Path: "", Kind: kind}
}
