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
	return cover(pol, bind, in, nil, policy.PhaseEnforce)
}

// CoverWithApproval is the E2-S07 evidence-aware decision entry: it additionally
// takes a separately-injected ApprovalContext (the evaluated sourceSha + per-
// governed-subject pre-fetched ApprovalEvidence) so an authored require-review
// obligation can be SATISFIED by valid, eligible, sha-matching, non-expired,
// non-self/bot approval (ADR-0017 §3). A nil appr is exactly Cover. Evidence is
// injected as a second input, never a field on the frozen EvaluationInput.
func CoverWithApproval(pol *policy.MergePolicy, bind *policy.Binding, in *EvaluationInput, appr *ApprovalContext) (Result, error) {
	return cover(pol, bind, in, appr, policy.PhaseEnforce)
}

// CoverWithPhaseCeiling is the E2-S08 pack-ceiling decision entry: it additionally
// takes a pack-level phase CEILING (ADR-0018 §1) that DOWNGRADES every rule's
// effective phase to min(rule.phase, ceiling) on the off<observe<enforce ordering.
// The ceiling only ever caps toward off — it is never additive:
//   - ceiling enforce ⇒ each rule's own phase stands (no cap) — exactly Cover;
//   - ceiling observe ⇒ every rule caps at observe (an enforce rule inside an
//     observe pack runs as observe → its finding lands in the observed bucket);
//   - ceiling off ⇒ nothing in the pack evaluates.
//
// The ceiling is threaded as a PARAMETER (default PhaseEnforce = no cap) because
// the frozen MergePolicy carries no spec.phase — only a Pack does (spec.phase),
// and Cover works over MergePolicy. A caller that has loaded a Pack passes its
// spec.phase here; a caller with no pack passes enforce (or uses Cover). An empty
// ceiling is normalized to enforce (no cap) so a caller slip never caps everything off.
func CoverWithPhaseCeiling(pol *policy.MergePolicy, bind *policy.Binding, in *EvaluationInput, appr *ApprovalContext, ceiling policy.Phase) (Result, error) {
	return cover(pol, bind, in, appr, ceiling)
}

// cover is the shared implementation. It returns the reduced decision plus the
// canonically sorted findings that justify it. An error is returned only for a
// policy the loader should have rejected (an unsupported/absent match domain) or
// an unbuildable CEL env — the decision itself is always fail-safe.
func cover(pol *policy.MergePolicy, bind *policy.Binding, in *EvaluationInput, appr *ApprovalContext, ceiling policy.Phase) (Result, error) {
	if pol == nil || bind == nil || in == nil {
		return Result{}, fmt.Errorf("coverage: nil policy, binding, or input")
	}
	// An empty ceiling means "no cap" — normalize to enforce so a caller slip (an
	// unset pack phase reaching here) never silently caps every rule off. A missing
	// phase on the pack itself is rejected at LOAD (E2-S01 strict decode); this is
	// only the in-memory-caller defensive default.
	if ceiling == "" {
		ceiling = policy.PhaseEnforce
	}

	env, err := newEvalEnv()
	if err != nil {
		return Result{}, fmt.Errorf("coverage: build CEL env: %w", err)
	}

	decision := DecisionApprove
	var findings []Finding
	// observed carries OBSERVE-phase findings — structurally excluded from the
	// decision/points/capabilityGaps below; recorded and canonically sorted, then
	// threaded into DecisionRecord findings.observed by record.go (E2-S08).
	var observed []Finding
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
		// Effective phase is the pack CEILING applied to the rule's own phase:
		// min(rule.phase, ceiling) on off<observe<enforce (ADR-0018 §1). The ceiling
		// only ever DOWNGRADES toward off; an enforce rule is never silently capped
		// EXCEPT by this explicit ceiling (a downgrade bug = a real block becomes
		// non-enforcing = fail-OPEN, so the cap must be this explicit min, nothing
		// implicit). A missing rule phase is rejected at LOAD (E2-S01); a hand-built
		// rule with an unset phase ranks as off here and is conservatively skipped.
		phase := effectivePhase(r.Phase, ceiling)
		// off (or any unknown/empty phase — fail-safe) ⇒ the rule is NEVER
		// evaluated: short-circuit BEFORE prove-nil, matchChanges, the tree walk, and any
		// CEL compile/eval, so a rule whose `when` would error still never evaluates
		// when off (REQ-E2-S08-02). Only the two known evaluated phases survive; an
		// unrecognized phase falls to the fail-safe default (never enforce, never
		// silently feed the decision). A missing phase is rejected at LOAD (E2-S01);
		// this default only guards a hand-built in-memory rule.
		switch phase {
		case policy.PhaseEnforce, policy.PhaseObserve:
			// evaluated below.
		default:
			continue
		}
		// Only obligation (prove) rules participate in coverage. A non-obligation
		// direct-effect rule (rule.effect, no prove) is a later story, not S04.
		if r.Prove == nil {
			continue
		}
		// Only ENFORCE marks the obligation covered — an observe(d) rule (or an
		// enforce rule capped to observe by the pack ceiling) does NOT feed the
		// decision, so it can NEVER satisfy a required obligation. Marking covered
		// from an observe rule would fail OPEN: a required obligation "proven" only
		// in observe would go vacuously satisfied instead of the fail-safe REVIEW.
		if phase == policy.PhaseEnforce {
			covered[r.Prove.Obligation] = true
		}

		matched, merr := matchChanges(r.Match, in.ChangeSet.Changes)
		if merr != nil {
			return Result{}, fmt.Errorf("coverage: rule %q: %w", r.Name, merr)
		}
		if len(matched) == 0 {
			continue
		}

		ctx := coverCtx{
			env: env, in: in, r: r, when: r.Prove.When, envLabel: bind.Environment,
			appr: appr, sourceSha: sourceSha, mrAuthor: mrAuthor,
		}

		for _, subj := range subjectsOf(matched) {
			f, firings, contributes, gap := coverSubject(ctx, subj, matched)
			// OBSERVE routing (ADR-0018 §1): the rule IS evaluated (so operators see
			// what it WOULD do), but its finding is routed to the observed bucket and
			// STRUCTURALLY EXCLUDED from aggregation — it never touches the decision,
			// the points sum, or the capability-gap set. Excluded at the point of
			// production (a `continue` before all of them), not filtered post-hoc.
			if phase == policy.PhaseObserve {
				if contributes {
					observed = append(observed, f)
				}
				continue
			}
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

	// Unmatched-whole-file-DELETE fail-safe escalation (EFE-S02, Judgment call (a) /
	// D-063, D-064). A whole-file DELETE event (path=="", kind==delete) that NO
	// EVALUATED fileEvents rule governs would otherwise leave the decision APPROVE — a
	// fail-OPEN on a destructive op: a file whose CONTENTS are governed by
	// values/valueChanges (which by S01's path!="" disjointness can never select a
	// path=="" delete) gets ZERO protection when deleted wholesale. So an ungoverned
	// whole-file delete escalates to at-least-REVIEW (never APPROVE). ADDITIVE and
	// fail-safe: it only ever raises the decision toward REVIEW via worse(), never
	// relaxes it, and it is a no-op on every changeset the differ mints today (all
	// path!=""), so every existing evaluation is byte-identical. Add is NOT escalated
	// (adding isn't destructive, D-063); only DELETE.
	for _, ch := range in.ChangeSet.Changes {
		if ch.Path != "" || ch.Kind != kindDelete {
			continue // only a WHOLE-FILE (path=="") delete event
		}
		if fileDeleteGoverned(pol, ch, ceiling) {
			continue // a fileEvents rule the loop evaluated selects it -> already governed
		}
		decision = worse(decision, DecisionReview)
		findings = append(findings, Finding{
			Rule:    ruleUnmatchedDelete,
			Effect:  EffectRequireReview,
			Subject: ch.Subject,
			Points:  0,
			Code:    "fileEvent.unmatchedDelete",
		})
	}

	// Aggregation order #4 (ADR-0007, the LAST check): with block (#1), unresolved
	// challenge (#2), and uncovered/unproven obligations (#3) already reduced, a
	// points sum over the binding threshold escalates an otherwise-APPROVE decision
	// to REVIEW. worse() keeps any earlier BLOCK/REVIEW intact (a block dominates a
	// points total; the threshold never displaces it).
	decision = worse(decision, riskDecision(pointsSum, bind.Risk.Threshold))

	sortFindings(findings)
	sortFindings(observed)
	return Result{Decision: decision, Findings: findings, Observed: observed, CapabilityGaps: capabilityGaps}, nil
}

// effectivePhase applies the pack CEILING to a rule's own phase: min(rule,
// ceiling) on the off<observe<enforce ordering (ADR-0018 §1). The ceiling only
// ever DOWNGRADES toward off — it is never additive. A ceiling of enforce (the
// no-pack default) returns the rule's phase unchanged.
func effectivePhase(rule, ceiling policy.Phase) policy.Phase {
	if phaseRank(ceiling) < phaseRank(rule) {
		return ceiling
	}
	return rule
}

// phaseRank totally orders the phases off<observe<enforce for the ceiling min().
// An unknown/empty phase ranks as off (0) — the fail-safe direction: an
// unrecognized phase is NEVER evaluated/enforced. A missing rule/pack phase is
// rejected at LOAD (E2-S01 strict decode); this default only guards a hand-built
// in-memory rule that never passed through the loader.
func phaseRank(p policy.Phase) int {
	switch p {
	case policy.PhaseObserve:
		return 1
	case policy.PhaseEnforce:
		return 2
	case policy.PhaseOff:
		return 0
	default:
		return 0
	}
}

// coverSubject evaluates one (rule, subject) pair over that subject's matched
// changes and returns the finding it contributes (if any). The boolean is false
// when the obligation is SATISFIED for this subject (all matched changes clean
// true) — a satisfied obligation is silent and does not lower APPROVE.
//
// Precedence when a subject's matched changes disagree: any evaluation error
// (a leaf error, or an all/any/not tri-state error surfaced by walkAssertTree)
// makes the obligation UNPROVEN for this subject -> require-review
// (predicate.error); else a clean-false `when` -> the rule's onFailure effect.
// Both never APPROVE; the exact tri-state ordering when a single subject mixes a
// clean-false with an error is refined in E2-S05 (no S04 input exercises the mix,
// and the never-APPROVE invariant holds either way).
//
// The `when` is walked as the full assertTree (E2-S03): a bare-string/single-leaf
// `when` walks to exactly the S02 evalLeaf result (byte-identical findings), and
// an all/any/not composes leaves in Kleene tri-state. The failing leaf's expanded
// `message` is attributed onto the emitted finding (per-leaf attribution).
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
// an all/any/not tri-state error, a nil-OnFailure shape error, and a block effect
// therefore can NEVER be suppressed by evidence — evidence satisfies an authored
// require-review obligation, never a fail-safe error or a block.
func coverSubject(c coverCtx, subj string, matched []EvalChange) (Finding, int, bool, bool) {
	r := c.r

	anyErr, anyFalse := false, false
	firings := 0
	// errMsg/falseMsg attribute the failing leaf's expanded message. Because a
	// subject can have several matched changes that fail with DIFFERENT expanded
	// messages (same template, different {{ old }}/{{ new }}/{{ path }} values), a
	// "first in input order" capture would make the finding depend on change order
	// — a determinism regression, since Message is the only per-change-varying
	// Finding field. We instead keep the LEXICOGRAPHICALLY SMALLEST message, a
	// canonical function of the failing set (order-independent under any shuffle,
	// REQ-E2-S03-05). errMsg feeds the predicate.error branch, falseMsg the
	// clean-false onFailure branch.
	var errMsg, falseMsg string
	for _, ch := range matched {
		if ch.Subject != subj {
			continue
		}
		ok, msg, walkErr := walkAssertTree(c.env, *c.in, ch, c.envLabel, c.when)
		if walkErr != nil {
			firings++ // a predicate-error firing (fail-safe; over-count is SAFE)
			if !anyErr || msg < errMsg {
				errMsg = msg
			}
			anyErr = true
			continue
		}
		if !ok {
			firings++ // a clean-false firing -> the rule's onFailure effect
			if !anyFalse || msg < falseMsg {
				falseMsg = msg
			}
			anyFalse = true
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
			Message:    errMsg,
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
				Message:    falseMsg,
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
				Message:    falseMsg,
			}, firings, true, gap
		}
		return Finding{
			Rule:       r.Name,
			Obligation: r.Prove.Obligation,
			Effect:     eff,
			Subject:    subj,
			Points:     r.Points,
			Code:       r.OnFailure.Code,
			Message:    falseMsg,
		}, firings, true, false
	default:
		return Finding{}, 0, false, false // satisfied for this subject -> silent, no firing
	}
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
	when      policy.AssertTree
	envLabel  string
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

// matchChanges returns, in input order, the changes the rule's match domain
// selects. An absent match domain is an error — the loader should have rejected
// it, so reaching here is a policy defect, not a fail-open "matches nothing".
//
// The domains are DISJOINT on the whole-file discriminator ch.Path (== "" for a
// whole-file event, != "" for a value-level change; evaluation-input.schema.json
// $defs.change.path). fileEvents selects ONLY whole-file (path=="") events; the
// value-level domains (files/values/valueChanges) select ONLY value-level
// (path!="") changes. This disjointness is a fail-safety invariant enforced in
// BOTH directions: a value glob can never select a whole-file event, and
// fileEvents can never select a value-level add/delete. It is a no-op on every
// change the differ mints today (all have path!="").
func matchChanges(m policy.Match, changes []EvalChange) ([]EvalChange, error) {
	switch {
	case m.FileEvents != nil:
		fe := m.FileEvents
		return selectMatched(changes, func(ch EvalChange) bool {
			// Whole-file discriminator: a fileEvents rule selects ONLY a whole-file
			// event (path==""), never a value-level change (disjointness direction 1).
			if ch.Path != "" {
				return false
			}
			if !contains(fe.Kinds, ch.Kind) {
				return false
			}
			return matchesAnyGlob(fe.Paths, ch.File)
		}), nil
	case m.Files != nil:
		return selectMatched(changes, func(ch EvalChange) bool {
			// Value-level domain: never selects a whole-file event (path==""), so a
			// file glob cannot poach a fileEvents change (disjointness direction 2).
			if ch.Path == "" {
				return false
			}
			return matchesAnyGlob(m.Files.Paths, ch.File)
		}), nil
	case m.ValueChanges != nil:
		vc := m.ValueChanges
		return selectMatched(changes, func(ch EvalChange) bool {
			if ch.Path == "" { // value-level only (disjointness direction 2)
				return false
			}
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
			if ch.Path == "" { // value-level only (disjointness direction 2)
				return false
			}
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
		return nil, fmt.Errorf("rule match declares no supported domain (files, values, valueChanges, or fileEvents)")
	}
}

// kindModify is the change.Kind string the Values domain implies. Declared
// locally to avoid importing internal/change here for a single literal.
const kindModify = "modify"

// kindDelete is the change.Kind string of a whole-file DELETE event — the
// destructive lifecycle the unmatched-delete fail-safe escalation guards (EFE-S02).
const kindDelete = "delete"

// fileDeleteGoverned reports whether some EVALUATED fileEvents prove rule selects
// the whole-file delete ch — i.e. the delete is actually run through a rule. It
// mirrors the main cover loop's participation gate EXACTLY (a prove rule whose
// effective phase, after the pack ceiling, is enforce or observe), so "governed"
// means "the loop would evaluate a rule against it". A disabled (off) rule, a
// non-fileEvents domain (by S01 path=="" disjointness it can never select a
// whole-file event anyway), and a direct-effect rule the loop skips all count as
// NOT governing — the fail-safe direction: an ungoverned delete escalates to
// REVIEW. It only ever consults fileEvents rules, whose matchChanges branch never
// errors, so it needs no error plumbing.
func fileDeleteGoverned(pol *policy.MergePolicy, ch EvalChange, ceiling policy.Phase) bool {
	for i := range pol.Spec.Rules {
		r := pol.Spec.Rules[i]
		if r.Prove == nil || r.Match.FileEvents == nil {
			continue
		}
		switch effectivePhase(r.Phase, ceiling) {
		case policy.PhaseEnforce, policy.PhaseObserve:
		default:
			continue // off / unknown: not evaluated -> does not govern
		}
		if m, err := matchChanges(r.Match, []EvalChange{ch}); err == nil && len(m) > 0 {
			return true
		}
	}
	return false
}

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
