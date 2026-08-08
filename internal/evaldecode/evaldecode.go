// Package evaldecode is the E2-S04 REQ-06 boundary that turns the LIVE differ's
// output into the engine's typed EvaluationInput. It lives OUTSIDE internal/core,
// so it may import internal/change and internal/core/aggregate while internal/core
// stays forge/change-free (TestCorePurity / TestEvaluationIsProviderless).
//
// It was extracted from cmd/assent (P5-E6-S01) so both the `assent` CLI shell AND
// the pure internal/adoptertest test-harness library reuse ONE canonical decoder —
// the load-bearing fail-open fix below must never be duplicated (drift would
// reopen it). cmd/assent keeps thin lowercase wrappers delegating here.
//
// THE FAIL-OPEN THIS CLOSES. internal/change emits change.Change.Old/New as
// CANONICAL, TAG-DISCRIMINATING STRINGS (see internal/change/diff.go render):
//
//	!!int / !!float  -> the RAW literal            ("12", "6", "016")
//	!!bool           -> the raw literal            ("true", "false")
//	!!null           -> "null"
//	!!str            -> JSON-QUOTED                 (the string "12" -> "\"12\"")
//	absent side      -> ""                          (add has no old; delete no new)
//
// The E2 engine (aggregate.evalLeaf / toCEL) does TYPED compares: the D-016
// `partitions-must-not-shrink` rule is the bare `new >= old`, no int() coercion.
// If the raw canonical string were fed straight into EvalChange.Old/New, cel-go
// would bind two STRINGS and `new >= old` would become a LEXICAL compare — and
// lexically "6" >= "12" is TRUE, so a partition shrink 12->6 would be judged
// non-destructive and APPROVE. That is the exact forbidden outcome. DecodeCanonical
// INVERTS the render so a numeric literal becomes a json.Number, which toCEL binds
// as int64/float64 (a numeric compare), restoring the correct 6 >= 12 -> false.
//
// Fail-safe direction (GUIDELINES §2): a value that cannot be decoded must NEVER
// silently become a lexical string. A numeric literal always becomes a json.Number
// (typed); an absent/undecodable value becomes nil (a numeric/relational compare
// over nil ERRORS in cel-go -> the engine's tri-state fail-safe -> REVIEW), never a
// permissive string.
//
// WHAT THIS PACKAGE DOES NOT CLOSE (and where that is closed). Decoding cannot be
// the whole answer, because a value that is GENUINELY text stays text: a YAML
// `partitions: "12"` is a !!str and this package keeps it the Go string "12" by
// design (the differ deliberately distinguishes it from the number 12). CEL then
// binds a string, and CEL's < <= > >= are DEFINED over strings as a lexical
// compare — so before D-131 the D-016 `new >= old` over a quoted shrink 12->6
// answered the lexical "6" >= "12" = TRUE and the shrink APPROVEd. Decoding could
// not prevent that; only the evaluator can. Since D-131 the engine seam refuses
// it: aggregate's textOrderGuard makes any relational compare whose operand
// actually evaluates to text an evaluation ERROR -> predicate.error -> REVIEW.
// The two layers are complementary and BOTH load-bearing — this package makes a
// numeric literal compare numerically (BLOCK, the right answer); the guard makes
// a genuinely-textual operand fail safe (REVIEW) instead of answering wrongly.
package evaldecode

import (
	"encoding/json"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// DecodeCanonical inverts internal/change's canonical render back to the typed
// value the engine compares. It is PURE (no clock/env/network/random).
//
//   - "" / "null"     -> nil (an absent add/delete side, or an explicit null).
//   - a !!bool literal -> the bool. go-yaml resolves true/True/TRUE and
//     false/False/FALSE as !!bool (YAML-1.2 core, resolve.go), and the differ's
//     render emits the RAW literal — so all six spellings are matched. Without the
//     capitalized spellings a bool-field ordering rule (`new >= old`) would decode
//     "True"/"False" to json.Number, fall to a lexical STRING compare, and mis-order.
//   - `"..."`         -> the unquoted string (a !!str value; stays a STRING so a
//     numeric rule over it does NOT numeric-compare it — the differ deliberately
//     kept a string "12" distinct from the number 12. An ORDERING rule over that
//     string does not compare it lexically either: since D-131 the evaluator
//     refuses to order text and fails safe to REVIEW).
//   - anything else   -> json.Number(literal) (a numeric literal, bound typed by
//     toCEL). Every bool (all six emitted spellings), JSON-quoted string, and null
//     is handled ABOVE, so the fallthrough is exactly a numeric literal — never a
//     capitalized bool and never a raw string.
//
// LIMITATION (the ADR-0013 residual #1 the evaluator owns, not the decoder): a
// numeric literal larger than int64 flows through as json.Number and toCEL binds
// it as float64 — a LOSSY compare, which is still live: two distinct literals
// beyond 2^53 can compare equal. What is no longer live is the second half of the
// old wording, "or its string form": since D-131 a literal representable as
// NEITHER int64 nor float64 (beyond ~1.8e308) binds a CEL error value, so it can
// never be ordered as text; it fails safe instead. DecodeCanonical itself is
// unchanged — it preserves the literal faithfully and leaves the edge to toCEL.
func DecodeCanonical(s string) any {
	switch s {
	case "", "null":
		return nil
	case "true", "True", "TRUE":
		return true
	case "false", "False", "FALSE":
		return false
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal([]byte(s), &str); err == nil {
			return str
		}
		// Unreachable from the differ (render JSON-marshals every string, so it
		// always round-trips). Defensive: an undecodable quoted literal fails SAFE
		// to nil (comparison errors -> REVIEW), never a raw lexical string.
		return nil
	}
	return json.Number(s)
}

// SubjectOf derives a change's governed-subject entryRef: its collection EntryRef
// when the collection-mode differ set one (ADR-0017 §5), else the file-derived
// "file:<path>" fallback (the document-mode differ sets no EntryRef). A non-empty
// subject is required — the DecisionRecord finding schema constrains subject to
// entryRef minLength:1.
func SubjectOf(c change.Change) string {
	if c.EntryRef != "" {
		return c.EntryRef
	}
	return "file:" + c.File
}

// BuildEvaluationInput assembles the engine's aggregate.EvaluationInput from the
// LIVE differ ChangeSet, decoding each change's canonical Old/New into typed
// values (DecodeCanonical) so the engine's `new >= old` is a NUMERIC compare.
// Facts is an empty (fail-safe) map: a fact-referencing obligation can never prove,
// so it never APPROVEs on a fact it never resolved; a caller that has stubbed or
// resolved facts assigns them onto the returned value's Facts field. MR and Require
// are threaded from the live MR metadata and the routed binding.
func BuildEvaluationInput(cs change.ChangeSet, mr aggregate.MR, require []string) aggregate.EvaluationInput {
	changes := make([]aggregate.EvalChange, len(cs.Changes))
	for i, c := range cs.Changes {
		changes[i] = aggregate.EvalChange{
			Subject: SubjectOf(c),
			File:    c.File,
			Path:    c.Path,
			Kind:    string(c.Kind),
			Old:     DecodeCanonical(c.Old),
			New:     DecodeCanonical(c.New),
		}
	}
	return aggregate.EvaluationInput{
		ChangeSet: aggregate.ChangeSet{Changes: changes},
		Facts:     map[string]map[string]aggregate.Fact{}, // empty = fail-safe; caller may override.
		MR:        mr,
		Require:   require,
	}
}
