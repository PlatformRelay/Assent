package classify

import (
	"regexp"
	"strings"

	"github.com/PlatformRelay/assent/internal/change"
)

// Matcher domains (E1-S06, ADR-0017 §5). This file adds the four decidable matcher-
// domain evaluation primitives a policy pack's routing rule uses to select the changes it governs,
// over a single-file ChangeSet. They are ADDITIVE vocabulary: the reserved-class dominance
// (Classify / ValidateRouting, ADR-0015 §1) is unchanged — a matcher SELECTS changes, it never
// routes, so it cannot weaken the `.assent/**` self-vouch boundary (an `.assent/**` edit still
// classifies to assent-policy regardless of which changes a matcher would select).
//
// The four domains (ADR-0017 §5), each PURE and order-independent over the input ChangeSet:
//   - files:          a path glob over Change.File (the file the change lives in).
//   - values.pointers: a JSON-pointer glob over Change.Path (the field pointer within the file).
//   - valueChanges:   a structural match on Change.Kind (any add/delete/modify/rename), any path.
//   - entryEvents:    a collection-ENTRY event — a change carrying an EntryRef (E1-S05) whose Kind
//                     matches. This matches collection-entry identity churn within one file; it is
//                     deliberately NOT ADR-0003's whole-file `fileEvents` (git-detected whole-file
//                     add/delete/rename), which this epic defers (see the epic Non-goals).
//
// These are the primitives E3's `assent lint` and the declarative policy frontend will call; E1
// ships the evaluation functions, not the authoring surface that invokes them.

// MatchFiles selects the changes whose File matches the path glob. The glob supports `*` (any run
// of non-`/` characters, within a path segment) and `**` (any characters including `/`, spanning
// segments), e.g. `topics/**` matches `topics/orders.yml` and `topics/sub/x.yml` but not
// `catalog/services.json`.
func MatchFiles(cs change.ChangeSet, glob string) []change.Change {
	return selectChanges(cs, func(c change.Change) bool { return globMatch(glob, c.File) })
}

// MatchValuePointers selects the changes whose Path (an RFC-6901 JSON pointer within the file)
// matches the pointer glob — the FIELD pointer, not the file glob (the overload ADR-0017 §5 ends).
// `/partitions` matches exactly `/partitions`; `/services/*/tier` matches a tier under any service.
func MatchValuePointers(cs change.ChangeSet, glob string) []change.Change {
	return selectChanges(cs, func(c change.Change) bool { return globMatch(glob, c.Path) })
}

// MatchValueChanges selects the changes whose Kind is one of kinds — a structural match
// independent of path or entry ("any delete," "any modify"). With no kinds given, selects none.
func MatchValueChanges(cs change.ChangeSet, kinds ...change.Kind) []change.Change {
	want := make(map[change.Kind]struct{}, len(kinds))
	for _, k := range kinds {
		want[k] = struct{}{}
	}
	return selectChanges(cs, func(c change.Change) bool {
		_, ok := want[c.Kind]
		return ok
	})
}

// MatchEntryEvents selects collection-ENTRY events: changes that carry an EntryRef (i.e. belong to
// a collection entry derived in E1-S05's map/list mode) AND whose Kind is one of kinds. A change
// with no EntryRef (a plain document-mode field change) is never an entry event, so it is not
// selected — this is what distinguishes entryEvents from valueChanges.
func MatchEntryEvents(cs change.ChangeSet, kinds ...change.Kind) []change.Change {
	want := make(map[change.Kind]struct{}, len(kinds))
	for _, k := range kinds {
		want[k] = struct{}{}
	}
	return selectChanges(cs, func(c change.Change) bool {
		if c.EntryRef == "" {
			return false
		}
		_, ok := want[c.Kind]
		return ok
	})
}

// selectChanges returns, IN INPUT ORDER, the changes for which keep returns true. The input
// ChangeSet is already canonically sorted by the differ, so the selection is deterministic and
// order-independent (re-sorting the input yields the same selected set in the same order).
func selectChanges(cs change.ChangeSet, keep func(change.Change) bool) []change.Change {
	var out []change.Change
	for _, c := range cs.Changes {
		if keep(c) {
			out = append(out, c)
		}
	}
	return out
}

// globMatch reports whether s matches a glob supporting `*` (non-`/` run) and `**` (any run,
// spanning `/`). It is a PURE function of (glob, s): the glob is translated to an anchored regexp
// with all other characters escaped. A malformed translation (which cannot occur for these two
// wildcards) matches nothing (fail-closed).
func globMatch(glob, s string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*") // ** spans path separators
				i++
			} else {
				b.WriteString("[^/]*") // * stays within one segment
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}
