package aggregate

// asserttree.go is the E2-S03 combinator walker over the frozen assertTree
// (`all`/`any`/`not` combinators over CEL-string leaves, ADR-0013). It composes
// the E2-S02 single-leaf primitive (evalLeaf) into a TRI-STATE result —
// satisfied / clean-false / error — never collapsing an error to false.
//
// Semantics (Kleene three-valued logic; RESULT is order-INDEPENDENT, only the
// attributed message follows declared leaf order — REQ-E2-S03-05):
//
//   - leaf  : evalLeaf; error -> (false, msg, err); false -> (false, msg, nil);
//             true -> (true, "", nil). A bare-string `when` decodes to a single
//             Leaf, so a one-leaf tree is EXACTLY the S02 behaviour (REQ-03-03).
//   - all   : AND. false DOMINATES error (a clean-false sibling makes the whole
//             conjunction cleanly false regardless of a sibling error) — so the
//             tri-state is order-independent. Scan every child: any clean-false ->
//             (false, firstFalseMsg, nil); else any error -> (false, firstErrMsg,
//             err); else -> (true, "", nil). "Short-circuit to the first failing
//             leaf" (REQ-03-01) means WHICH message is attributed (declared order),
//             not stopping evaluation.
//   - any   : OR. true DOMINATES error. Scan every child: any true -> satisfied;
//             else any error -> (false, firstErrMsg, err) [fail-safe: an erroring
//             leaf must NOT let `any` spuriously satisfy]; else all clean-false ->
//             (false, firstFalseMsg, nil).
//   - not   : negation over the child result. error PROPAGATES (fail-safe: an
//             erroring leaf inside `not` must NOT spuriously fire); child-true ->
//             (false, childMsg, nil); child-false -> (true, "", nil).
//
// Fail-safe by effect (ADR-0007 F6 / ADR-0017 §6): whenever the walker returns a
// non-nil error the caller (coverSubject) routes it to predicate.error ->
// require-review; a clean-false routes to the rule's onFailure effect. Neither
// can APPROVE. An empty/malformed tree (none of leaf/all/any/not set, or nesting
// past the depth ceiling) returns an error — never a vacuous true.
//
// Pure: no clock/rand/env/network. Determinism rides on evalLeaf's cost-budgeted,
// side-effect-free cel-go evaluation; this walker only orders leaf calls.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/cel-go/cel"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// maxAssertDepth bounds the assertTree nesting the walker descends (a
// documented depth ceiling, ADR-0013 cost budget). A tree deeper than this
// returns an error -> fail-safe, never an unbounded recursion. Authored trees
// are small (the D-016 fixture is depth 1); this only guards a pathological or
// hand-built input.
const maxAssertDepth = 32

// walkAssertTree evaluates one assertTree over the activation built for the
// handed change, returning (satisfied, attributedMessage, error). The message is
// the failing/erroring leaf's expanded `message` (empty when satisfied or when
// the attributed leaf declared none); the error is non-nil for any tri-state
// error (fail-safe by effect). It NEVER returns (true, _, nil) for an unproven
// or unevaluable tree.
func walkAssertTree(env *cel.Env, in EvaluationInput, ch EvalChange, envLabel string, tree policy.AssertTree) (bool, string, error) {
	return walkAssertTreeDepth(env, in, ch, envLabel, tree, 0)
}

func walkAssertTreeDepth(env *cel.Env, in EvaluationInput, ch EvalChange, envLabel string, tree policy.AssertTree, depth int) (bool, string, error) {
	if depth > maxAssertDepth {
		return false, "", fmt.Errorf("assert tree exceeds max nesting depth %d", maxAssertDepth)
	}
	switch {
	case tree.Leaf != nil:
		ok, err := evalLeaf(env, in, ch, envLabel, tree.Leaf.CEL)
		if err != nil {
			return false, expandMessage(tree.Leaf.Message, in, ch, envLabel), err
		}
		if !ok {
			return false, expandMessage(tree.Leaf.Message, in, ch, envLabel), nil
		}
		return true, "", nil

	case tree.All != nil:
		// AND: false dominates error (order-independent tri-state).
		var firstFalseMsg, firstErrMsg string
		var haveFalse bool
		var firstErr error
		for i := range tree.All {
			s, m, err := walkAssertTreeDepth(env, in, ch, envLabel, tree.All[i], depth+1)
			switch {
			case err != nil:
				if firstErr == nil {
					firstErr, firstErrMsg = err, m
				}
			case !s:
				if !haveFalse {
					haveFalse, firstFalseMsg = true, m
				}
			}
		}
		if haveFalse {
			return false, firstFalseMsg, nil // clean-false dominates a sibling error
		}
		if firstErr != nil {
			return false, firstErrMsg, firstErr
		}
		return true, "", nil

	case tree.Any != nil:
		// OR: true dominates error; an error dominates all-false (fail-safe).
		var firstFalseMsg, firstErrMsg string
		var haveFalse bool
		var firstErr error
		for i := range tree.Any {
			s, m, err := walkAssertTreeDepth(env, in, ch, envLabel, tree.Any[i], depth+1)
			switch {
			case err != nil:
				if firstErr == nil {
					firstErr, firstErrMsg = err, m
				}
			case s:
				return true, "", nil // a satisfied disjunct proves the whole `any`
			default:
				if !haveFalse {
					haveFalse, firstFalseMsg = true, m
				}
			}
		}
		if firstErr != nil {
			return false, firstErrMsg, firstErr // unknown disjunct -> fail-safe error
		}
		return false, firstFalseMsg, nil

	case tree.Not != nil:
		s, m, err := walkAssertTreeDepth(env, in, ch, envLabel, *tree.Not, depth+1)
		if err != nil {
			return false, m, err // error propagates (not must not spuriously fire)
		}
		if s {
			return false, m, nil // negation of a satisfied child is unsatisfied
		}
		return true, "", nil

	default:
		// No branch set: a malformed/empty tree the loader should have rejected.
		// Fail safe rather than treat an empty condition as vacuously true.
		return false, "", fmt.Errorf("assert tree has no leaf/all/any/not branch")
	}
}

// tmplPlaceholder matches a `{{ ... }}` message placeholder (non-greedy, no
// nested braces). The captured body is a dotted path into the SAME activation
// the CEL leaf evaluated over (ADR-0013 residual #5: one activation model).
var tmplPlaceholder = regexp.MustCompile(`\{\{[^{}]*\}\}`)

// expandMessage substitutes `{{ old }}` / `{{ new }}` / `{{ facts.owner.team.state }}`
// style placeholders in a leaf message with values resolved from the change's
// activation (the identical map bindLeafActivation feeds evalLeaf). A placeholder
// that does not resolve to a scalar is left LITERAL (never a panic, never a silent
// blank) — the authoring-time in-scope check is E3's lint, not this expander's job.
// A message with no placeholder is returned unchanged (fast path).
func expandMessage(msg string, in EvaluationInput, ch EvalChange, envLabel string) string {
	if !strings.Contains(msg, "{{") {
		return msg
	}
	act := bindLeafActivation(in, ch, envLabel)
	return tmplPlaceholder.ReplaceAllStringFunc(msg, func(token string) string {
		path := strings.TrimSpace(token[2 : len(token)-2])
		if path == "" {
			return token
		}
		v, ok := resolveActivationPath(act, path)
		if !ok {
			return token // unresolved -> leave the placeholder literal
		}
		return fmt.Sprintf("%v", v)
	})
}

// resolveActivationPath walks a dotted path (e.g. "facts.owner.team.state")
// through the activation's nested maps, returning the leaf value if every segment
// resolves to a map key ending at a scalar/printable value. Any missing segment,
// or a descent into a non-map, returns (nil, false) so the caller leaves the
// placeholder literal — comma-ok throughout, never a panic in the pure engine.
func resolveActivationPath(act map[string]any, dotted string) (any, bool) {
	segs := strings.Split(dotted, ".")
	var cur any = act
	for _, seg := range segs {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[seg]
		if !ok {
			return nil, false
		}
		cur = next
	}
	// Only substitute a scalar-ish value; a map/slice leaf stays literal.
	switch cur.(type) {
	case map[string]any, []any, nil:
		return nil, false
	default:
		return cur, true
	}
}
