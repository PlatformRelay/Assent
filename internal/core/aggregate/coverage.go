package aggregate

// coverage.go is the E2-S04 multi-obligation AND coverage loop over the frozen
// policy model. It grows the walking-skeleton's single-obligation Aggregate into
// full ADR-0017 §2 coverage: a binding's require[] is satisfied only when, for
// EACH required obligation AND EACH governed subject the changeSet touches, an
// enforce-phase `prove.{obligation, when}` rule matched that subject and its
// `when` held cleanly true. AND-only — a require list is a conjunction, never an
// anyOf/alternative-proof composition (ADR-0017 §9 do-not-generalize).
//
// Fail-safe throughout (never APPROVE on ambiguity):
//   - an uncovered required obligation (no enforce rule proves it) -> REVIEW;
//   - a matched change whose `when` errors (absent fact value, type mismatch,
//     cost overrun, non-bool) -> REVIEW (predicate.error);
//   - an all/any/not assert tree (E2-S03, not implemented here) -> REVIEW;
//   - a clean-false `when` -> the rule's onFailure effect, per subject.
//
// Determinism (ADR-0013): pure over its inputs (no clock/rand/env/network), and
// the findings are canonically sorted so shuffling the rule order or the change
// order yields a byte-identical Result.
//
// It consumes the E2-S01 loaded policy types (policy.MergePolicy / policy.Binding)
// and an EvaluationInput, reusing the E2-S02 evalLeaf primitive for every leaf.
// policy does NOT import aggregate, so this import is cycle-free.

import (
	"fmt"

	"github.com/google/cel-go/cel"

	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/glob"
)

// Cover computes the multi-obligation × multi-subject decision over the loaded
// merge policy, binding, and evaluation input. It returns the reduced decision
// plus the canonically sorted findings that justify it. An error is returned only
// for a policy the loader should have rejected (an unsupported/absent match
// domain) or an unbuildable CEL env — the decision itself is always fail-safe.
func Cover(pol *policy.MergePolicy, bind *policy.Binding, in *EvaluationInput) (Result, error) {
	if pol == nil || bind == nil || in == nil {
		return Result{}, fmt.Errorf("coverage: nil policy, binding, or input")
	}

	env, err := newEvalEnv()
	if err != nil {
		return Result{}, fmt.Errorf("coverage: build CEL env: %w", err)
	}

	decision := DecisionApprove
	var findings []Finding

	// An obligation is "covered" when at least one ENFORCE-phase rule names it in
	// prove.obligation — regardless of whether that rule's match selected a
	// subject. (A proving rule that matches no subject means the obligation simply
	// does not apply here; it is NOT uncovered — REQ-S04-02 note.)
	covered := map[string]bool{}

	for i := range pol.Spec.Rules {
		r := pol.Spec.Rules[i]
		// Only enforce-phase rules feed the decision (observe/off are recorded
		// elsewhere, never aggregated — ADR-0018 §1). A missing phase never
		// reaches here (the loader requires it) but would be conservatively skipped.
		if r.Phase != policy.PhaseEnforce {
			continue
		}
		// Only obligation (prove) rules participate in coverage. A non-obligation
		// direct-effect rule (rule.effect, no prove) is a later story, not S04.
		if r.Prove == nil {
			continue
		}
		covered[r.Prove.Obligation] = true

		matched, merr := matchChanges(r.Match, in.ChangeSet.Changes)
		if merr != nil {
			return Result{}, fmt.Errorf("coverage: rule %q: %w", r.Name, merr)
		}
		if len(matched) == 0 {
			continue
		}

		expr, leafErr := leafExpr(r.Prove.When)

		for _, subj := range subjectsOf(matched) {
			f, contributes := coverSubject(env, in, r, expr, bind.Environment, leafErr, subj, matched)
			if !contributes {
				continue
			}
			decision = worse(decision, effectDecision(f.Effect))
			findings = append(findings, f)
		}
	}

	// Uncovered-obligation guard: a required obligation with NO enforce-phase
	// proving rule can never be proven -> fail-safe REVIEW, never a vacuous APPROVE.
	for _, obl := range bind.Require {
		if covered[obl] {
			continue
		}
		decision = worse(decision, DecisionReview)
		// Subject is intentionally EMPTY: an uncovered obligation has no proving
		// rule, so no matched change and no governed subject to attribute it to.
		// NOTE for the REQ-06 re-seat: the DecisionRecord finding schema constrains
		// subject to entryRef minLength:1, so serializing this finding through
		// decision.Build will fail validation until the re-seat picks either a
		// sentinel subject or per-governed-subject uncovered emission (an ADR-0017
		// §5 entry-identity decision deferred with the rest of REQ-06). Cover's
		// output is not serialized in this lane, so this is latent, not live.
		findings = append(findings, Finding{
			Rule:       ruleUncovered,
			Obligation: obl,
			Effect:     EffectRequireReview,
			Points:     0,
			Code:       "obligation.uncovered",
		})
	}

	sortFindings(findings)
	return Result{Decision: decision, Findings: findings}, nil
}

// coverSubject evaluates one (rule, subject) pair over that subject's matched
// changes and returns the finding it contributes (if any). The boolean is false
// when the obligation is SATISFIED for this subject (all matched changes clean
// true) — a satisfied obligation is silent and does not lower APPROVE.
//
// Precedence when a subject's matched changes disagree: an assert tree (S03) or
// any evaluation error makes the obligation UNPROVEN for this subject ->
// require-review (predicate.error); else a clean-false `when` -> the rule's
// onFailure effect. Both never APPROVE; the exact tri-state ordering when a
// single subject mixes a clean-false with an error is refined in E2-S05 (no S04
// input exercises the mix, and the never-APPROVE invariant holds either way).
func coverSubject(env *cel.Env, in *EvaluationInput, r policy.Rule, expr, envLabel string, leafErr error, subj string, matched []EvalChange) (Finding, bool) {
	// An all/any/not tree is E2-S03; treat it as unproven here (fail-safe).
	if leafErr != nil {
		return Finding{
			Rule:       r.Name,
			Obligation: r.Prove.Obligation,
			Effect:     EffectRequireReview,
			Subject:    subj,
			Points:     r.Points,
			Code:       "predicate.error",
		}, true
	}

	anyErr, anyFalse := false, false
	for _, ch := range matched {
		if ch.Subject != subj {
			continue
		}
		ok, evalErr := evalLeaf(env, *in, ch, envLabel, expr)
		if evalErr != nil {
			anyErr = true
			continue
		}
		if !ok {
			anyFalse = true
		}
	}

	switch {
	case anyErr:
		return Finding{
			Rule:       r.Name,
			Obligation: r.Prove.Obligation,
			Effect:     EffectRequireReview,
			Subject:    subj,
			Points:     r.Points,
			Code:       "predicate.error",
		}, true
	case anyFalse:
		// A prove rule with a nil OnFailure is a malformed policy the schema's
		// oneOf normally rejects — but Cover also accepts hand-built policies, so
		// guard it: a clean-false with no declared effect is a shape error that
		// fails SAFE to require-review, never a nil-deref panic or a silent APPROVE.
		if r.OnFailure == nil {
			return Finding{
				Rule:       r.Name,
				Obligation: r.Prove.Obligation,
				Effect:     EffectRequireReview,
				Subject:    subj,
				Points:     r.Points,
				Code:       "policy.shape-error",
			}, true
		}
		return Finding{
			Rule:       r.Name,
			Obligation: r.Prove.Obligation,
			Effect:     Effect(r.OnFailure.Effect),
			Subject:    subj,
			Points:     r.Points,
			Code:       r.OnFailure.Code,
		}, true
	default:
		return Finding{}, false // satisfied for this subject -> silent
	}
}

// subjectsOf returns the DISTINCT subjects of the matched changes in
// first-appearance order. The final Result is canonically sorted, so this order
// only affects the transient build, keeping Cover order-independent.
func subjectsOf(matched []EvalChange) []string {
	seen := map[string]bool{}
	var out []string
	for _, ch := range matched {
		if seen[ch.Subject] {
			continue
		}
		seen[ch.Subject] = true
		out = append(out, ch.Subject)
	}
	return out
}

// leafExpr extracts the single CEL leaf string from a prove.when. An all/any/not
// combinator is E2-S03 (the tree walker), NOT this story — it returns an error so
// the caller fails safe rather than silently ignoring the composed condition.
func leafExpr(when policy.AssertTree) (string, error) {
	if when.Leaf != nil {
		return when.Leaf.CEL, nil
	}
	return "", fmt.Errorf("assert tree (all/any/not) is E2-S03, not supported by the S04 coverage loop")
}

// matchChanges returns, in input order, the changes the rule's match domain
// selects. An unsupported (fileEvents) or absent match domain is an error — the
// loader should have rejected it, so reaching here is a policy defect, not a
// fail-open "matches nothing".
func matchChanges(m policy.Match, changes []EvalChange) ([]EvalChange, error) {
	switch {
	case m.FileEvents != nil:
		return nil, fmt.Errorf("match.fileEvents is deferred (E1 fast-follow); use files, values, or valueChanges")
	case m.Files != nil:
		return selectMatched(changes, func(ch EvalChange) bool {
			return matchesAnyGlob(m.Files.Paths, ch.File)
		}), nil
	case m.ValueChanges != nil:
		vc := m.ValueChanges
		return selectMatched(changes, func(ch EvalChange) bool {
			// pointers are GLOBS over the field pointer (ch.Path), consistent with
			// E1's classify.MatchValuePointers and internal/glob — an authored
			// pointer like "/config/*/enabled" is valid (the schema imposes no
			// pattern), so exact membership would fail-open on a wildcard rule.
			if !matchesAnyGlob(vc.Pointers, ch.Path) {
				return false
			}
			if len(vc.Kinds) > 0 && !contains(vc.Kinds, ch.Kind) {
				return false
			}
			if len(vc.Paths) > 0 && !matchesAnyGlob(vc.Paths, ch.File) {
				return false
			}
			return true
		}), nil
	case m.Values != nil:
		v := m.Values
		// Values is the implicit-modify value domain, matched like valueChanges
		// with an implied kind:modify: pointers (globs) AND-narrow over ch.Path,
		// paths (file globs) AND-narrow over ch.File. At least one selector must be
		// present, else the domain matches nothing (fail-closed, never a wildcard).
		if len(v.Pointers) == 0 && len(v.Paths) == 0 {
			return nil, fmt.Errorf("match.values declares neither pointers nor paths")
		}
		return selectMatched(changes, func(ch EvalChange) bool {
			if ch.Kind != kindModify {
				return false
			}
			if len(v.Pointers) > 0 && !matchesAnyGlob(v.Pointers, ch.Path) {
				return false
			}
			if len(v.Paths) > 0 && !matchesAnyGlob(v.Paths, ch.File) {
				return false
			}
			return true
		}), nil
	default:
		return nil, fmt.Errorf("rule match declares no supported domain (files, values, or valueChanges)")
	}
}

// kindModify is the change.Kind string the Values domain implies. Declared
// locally to avoid importing internal/change here for a single literal.
const kindModify = "modify"

// selectMatched returns, IN INPUT ORDER, the changes for which keep is true.
func selectMatched(changes []EvalChange, keep func(EvalChange) bool) []EvalChange {
	var out []EvalChange
	for _, ch := range changes {
		if keep(ch) {
			out = append(out, ch)
		}
	}
	return out
}

// matchesAnyGlob reports whether s matches any of the glob patterns.
func matchesAnyGlob(patterns []string, s string) bool {
	for _, p := range patterns {
		if glob.Match(p, s) {
			return true
		}
	}
	return false
}

// contains reports set membership (exact string equality).
func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
