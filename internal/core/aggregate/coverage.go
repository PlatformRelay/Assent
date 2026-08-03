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
// merge policy, binding, and evaluation input, with NO injected approval evidence
// — every require-review obligation stays unsatisfied (the D-016 golden path).
// It is the stable 3-arg entry preserved byte-identical for existing callers.
func Cover(pol *policy.MergePolicy, bind *policy.Binding, in *EvaluationInput) (Result, error) {
	return cover(pol, bind, in, nil)
}

// CoverWithApproval is the E2-S07 evidence-aware decision entry: it additionally
// takes a separately-injected ApprovalContext (the evaluated sourceSha + per-
// governed-subject pre-fetched ApprovalEvidence) so an authored require-review
// obligation can be SATISFIED by valid, eligible, sha-matching, non-expired,
// non-self/bot approval (ADR-0017 §3). A nil appr is exactly Cover. Evidence is
// injected as a second input, never a field on the frozen EvaluationInput.
func CoverWithApproval(pol *policy.MergePolicy, bind *policy.Binding, in *EvaluationInput, appr *ApprovalContext) (Result, error) {
	return cover(pol, bind, in, appr)
}

// cover is the shared implementation. It returns the reduced decision plus the
// canonically sorted findings that justify it. An error is returned only for a
// policy the loader should have rejected (an unsupported/absent match domain) or
// an unbuildable CEL env — the decision itself is always fail-safe.
func cover(pol *policy.MergePolicy, bind *policy.Binding, in *EvaluationInput, appr *ApprovalContext) (Result, error) {
	if pol == nil || bind == nil || in == nil {
		return Result{}, fmt.Errorf("coverage: nil policy, binding, or input")
	}

	env, err := newEvalEnv()
	if err != nil {
		return Result{}, fmt.Errorf("coverage: build CEL env: %w", err)
	}

	decision := DecisionApprove
	var findings []Finding
	// capabilityGaps records verifyingCapability:none gaps discovered while
	// satisfying require-review, keyed by subject (nil until the first gap, so the
	// no-evidence Result stays byte-identical). It never satisfies — the finding
	// still stands — but is surfaced distinctly (S10 threads it into pins.capabilityGap).
	var capabilityGaps map[string]string
	// mrAuthor and sourceSha are the S07 satisfaction inputs, shared across rules.
	mrAuthor := in.MR.Author
	sourceSha := appr.sourceSha()
	// pointsSum accrues author-declared rule.points PER FIRING (per matched-and-
	// failed change), NOT per finding — ADR-0007 Amendment 2, the bulk-change /
	// salami-slice guard. It gates APPROVE at order #4 (risk check) below.
	pointsSum := 0

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
		ctx := coverCtx{
			env: env, in: in, r: r, expr: expr, envLabel: bind.Environment, leafErr: leafErr,
			appr: appr, sourceSha: sourceSha, mrAuthor: mrAuthor,
		}

		for _, subj := range subjectsOf(matched) {
			f, firings, contributes, gap := coverSubject(ctx, subj, matched)
			// A verifyingCapability:none gap is recorded distinctly (never satisfies;
			// the finding still stands below) so a capability gap stays separable
			// from a plain missing approval (d016_missing_approval invariant).
			if gap {
				if capabilityGaps == nil {
					capabilityGaps = map[string]string{}
				}
				capabilityGaps[subj] = capabilityGapNone
			}
			if !contributes {
				continue
			}
			// Points accrue PER FIRING: the finding carries the AUTHORED per-firing
			// weight r.Points (e.g. 10), but the SUM adds firings*r.Points — a
			// subject with K failed changes of a points:w rule contributes K*w. The
			// finding-vs-sum asymmetry (one finding shows w; the sum counts K*w) is
			// INTENTIONAL (ADR-0007 Amendment 2); do NOT "fix" the finding to K*w.
			pointsSum += firings * r.Points
			// Decision mapping. comment is the decision-NEUTRAL soft points channel
			// (ADR-0007) — but ONLY for a NON-required (signal) obligation. A
			// REQUIRED obligation is satisfied-to-arm (ADR-0017 §2, which SUPERSEDES
			// ADR-0007's coverage line): an unproven required obligation lowers the
			// decision regardless of effect, so comment does NOT go neutral there.
			// Without this guard a required obligation proved by a comment-onFailure
			// rule that FIRED would fail OPEN to APPROVE on an unproven obligation.
			eff := firingEffectDecision(f.Effect)
			if contains(bind.Require, r.Prove.Obligation) {
				eff = effectDecision(f.Effect) // required: comment still lowers (§2)
			}
			decision = worse(decision, eff)
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

	// Aggregation order #4 (ADR-0007, the LAST check): with block (#1), unresolved
	// challenge (#2), and uncovered/unproven obligations (#3) already reduced, a
	// points sum over the binding threshold escalates an otherwise-APPROVE decision
	// to REVIEW. worse() keeps any earlier BLOCK/REVIEW intact (a block dominates a
	// points total; the threshold never displaces it).
	decision = worse(decision, riskDecision(pointsSum, bind.Risk.Threshold))

	sortFindings(findings)
	return Result{Decision: decision, Findings: findings, CapabilityGaps: capabilityGaps}, nil
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
//
// The returned int is the FIRING COUNT K: how many of this subject's matched
// changes did NOT cleanly pass (evaluated `when`-false, or errored). The caller
// multiplies K by the rule's authored points for the per-firing risk sum
// (ADR-0007 Amendment 2). K is 0 (and contributes==false) only when every matched
// change cleanly proved the obligation. Over-counting K (e.g. an errored change)
// is the SAFE direction — it can only raise the sum toward REVIEW, never toward a
// wrong APPROVE.
//
// The final bool is `gap`: true only when a verifyingCapability:none
// ApprovalEvidence was consulted for an authored require-review firing (recorded
// distinctly by the caller; the finding still stands).
//
// S07 satisfaction is gated STRUCTURALLY: injected evidence is consulted ONLY in
// the clean-false authored-onFailure `require-review` branch. A predicate error,
// an assert-tree (S03) leafErr, a nil-OnFailure shape error, and a block effect
// therefore can NEVER be suppressed by evidence — evidence satisfies an authored
// require-review obligation, never a fail-safe error or a block.
func coverSubject(c coverCtx, subj string, matched []EvalChange) (Finding, int, bool, bool) {
	r := c.r
	// An all/any/not tree is E2-S03; treat it as unproven here (fail-safe). Every
	// matched change for this subject is a fail-safe firing (over-count is SAFE).
	if c.leafErr != nil {
		return Finding{
			Rule:       r.Name,
			Obligation: r.Prove.Obligation,
			Effect:     EffectRequireReview,
			Subject:    subj,
			Points:     r.Points,
			Code:       "predicate.error",
		}, subjectMatchCount(subj, matched), true, false
	}

	anyErr, anyFalse := false, false
	firings := 0
	for _, ch := range matched {
		if ch.Subject != subj {
			continue
		}
		ok, evalErr := evalLeaf(c.env, *c.in, ch, c.envLabel, c.expr)
		if evalErr != nil {
			anyErr = true
			firings++ // a predicate-error firing (fail-safe; over-count is SAFE)
			continue
		}
		if !ok {
			anyFalse = true
			firings++ // a clean-false firing -> the rule's onFailure effect
		}
		// a clean-true change proves the obligation for this change: NOT a firing.
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
		}, firings, true, false
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
			}, firings, true, false
		}
		eff := Effect(r.OnFailure.Effect)
		// E2-S07: an authored require-review obligation may be SATISFIED by valid
		// injected approval evidence for this subject. This is the ONLY branch that
		// consults evidence — a block/comment/challenge onFailure is untouched.
		if eff == EffectRequireReview {
			satisfied, gap := approvalSatisfies(c.appr.evidenceFor(subj), c.sourceSha, c.mrAuthor)
			if satisfied {
				return Finding{}, 0, false, false // obligation satisfied -> silent, no firing
			}
			return Finding{
				Rule:       r.Name,
				Obligation: r.Prove.Obligation,
				Effect:     EffectRequireReview,
				Subject:    subj,
				Points:     r.Points,
				Code:       r.OnFailure.Code,
			}, firings, true, gap
		}
		return Finding{
			Rule:       r.Name,
			Obligation: r.Prove.Obligation,
			Effect:     eff,
			Subject:    subj,
			Points:     r.Points,
			Code:       r.OnFailure.Code,
		}, firings, true, false
	default:
		return Finding{}, 0, false, false // satisfied for this subject -> silent, no firing
	}
}

// subjectMatchCount counts the matched changes attributed to subj — the firing
// count used when the whole predicate is unevaluable (an S03 tree `when`), where
// every matched change is treated as a fail-safe firing.
func subjectMatchCount(subj string, matched []EvalChange) int {
	n := 0
	for _, ch := range matched {
		if ch.Subject == subj {
			n++
		}
	}
	return n
}

// coverCtx bundles the per-rule evaluation context handed to coverSubject —
// grouping what were eight positional parameters into one struct so the helper
// stays within the linter's parameter bound (go:S107) without a behavioural
// change. env/in/envLabel are shared across subjects of a rule; r/expr/leafErr
// are the rule's decoded proof; appr/sourceSha/mrAuthor are the shared E2-S07
// require-review satisfaction inputs (appr may be nil ⇒ no injected evidence).
type coverCtx struct {
	env       *cel.Env
	in        *EvaluationInput
	r         policy.Rule
	expr      string
	envLabel  string
	leafErr   error
	appr      *ApprovalContext
	sourceSha string
	mrAuthor  string
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
