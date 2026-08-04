package change

// OneSidedLifecycle detects an UNAMBIGUOUS whole-file lifecycle from one-sided
// presence (EFE-S02/S03): exactly one of base/head is ABSENT. An absent base +
// present head is a file-ADD; a present base + absent head is a file-DELETE. It
// returns (kind, true) ONLY for that unambiguous case; every other shape falls
// through to Diff (both present, both absent, or an empty-but-PRESENT side).
//
// THE AMBIGUITY INVARIANT: the presence signal is nil-vs-non-nil, NEVER
// len()==0. An empty-but-present document ({} / []byte{}) is non-nil and MUST
// NOT be mistaken for a delete — treating it as one is a fail-OPEN. Callers
// (adoptertest assemble, cmd/assent live checkout) share this helper so the
// harness and the live path cannot drift.
func OneSidedLifecycle(base, head []byte) (Kind, bool) {
	baseAbsent := base == nil
	headAbsent := head == nil
	switch {
	case baseAbsent && !headAbsent:
		return KindAdd, true
	case !baseAbsent && headAbsent:
		return KindDelete, true
	default:
		return "", false
	}
}
