// Package aggregate is the PURE, order-independent obligations aggregator for
// the P4-E1 walking skeleton (P4-E1-S03, ADR-0017 §2/§6). It evaluates one
// assert/CEL rule (`prove: {obligation, when}` with `onFailure: {effect, code}`)
// over the S02 change.ChangeSet and reduces it to a decision APPROVE/REVIEW/BLOCK.
//
// Safety model (the fail-open class this package closes):
//
//   - APPROVE is modelled as COVERAGE, never absence-of-failure: the required
//     obligation is APPROVE only if a rule proving exactly that obligation
//     evaluated cleanly to a boolean true. A required obligation with no proving
//     rule, or whose rule errored, is NOT approved — it degrades to REVIEW/BLOCK.
//     This is the ADR-0017 §2 invariant ("every required obligation satisfied to
//     arm"); computing APPROVE as "no rule returned false" would silently approve
//     an unproven obligation, the exact forbidden outcome.
//   - An opaque ChangeSet (the differ could not decide), an EMPTY change list
//     (schema minItems:1 would be violated downstream), a predicate that fails to
//     COMPILE, a predicate that ERRORS at eval (incl. every numeric-coercion
//     failure — see the CEL binding note below), and a predicate whose result is
//     not a boolean ALL fail SAFE to REVIEW, never APPROVE.
//
// CEL numeric coercion (constraint c). change.Change.Old/New are the differ's
// CANONICAL, TAG-DISCRIMINATING STRINGS ("12" for int 12, "\"12\"" for the
// string "12", "016" kept literal). internal/evaldecode INVERTS that render
// before the engine sees it, so a numeric literal arrives as a json.Number and
// toCEL binds it as int64/float64 — `new >= old` is a NUMERIC compare and needs
// no int() in the expression. A non-numeric input to an explicit int()/double()
// still ERRORS (empirically verified in cel-go: the error surfaces via BOTH the
// Eval error slot AND a types.Err result value — this package checks both), which
// the tri-state routes to REVIEW. We deliberately do NOT strconv-coerce Go-side
// with a 0/false/"" default: that would fail OPEN to APPROVE on a parse failure.
// A value that is GENUINELY text (a YAML !!str, e.g. `partitions: "12"`) still
// binds as a string, and a lexical compare over it is wrong in both directions
// ("6" >= "12" is lexically true, "12" >= "6" lexically false) — so since D-129
// evalLeaf's textOrderGuard makes an ordering operator over a text operand an
// EVALUATION ERROR (-> predicate.error -> REVIEW), never an answer. Ordering
// quoted numerics deliberately means coercing first: int(new) >= int(old).
//
// Change-ness signal (constraint d). The PRESENCE of an entry in the ChangeSet is
// the "this field changed" signal. Old==New string-equal can still be a real
// (tag-only) change the differ emitted; this package NEVER re-derives change-ness
// from Old vs New. It only binds them for the predicate to read.
//
// Purity (GUIDELINES §5, ADR-0013): this package reads no clock, randomness,
// environment, or network. cel-go evaluation over fixed inputs is deterministic;
// the reduction sorts findings by a total key so shuffled input yields a
// byte-identical decision + findings slice.
package aggregate

import (
	"fmt"
	"sort"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"

	"github.com/PlatformRelay/assent/internal/change"
)

// Effect is a rule's onFailure effect (ADR-0017 §2, the DecisionRecord finding
// schema enum). This thin slice maps an UNSATISFIED effect to a decision:
// block -> BLOCK; comment/challenge/require-review -> REVIEW. A satisfied
// obligation contributes no finding and does not lower APPROVE. (require-review
// is authorization that this provider-less slice cannot forge-prove, so an
// unsatisfied one stays REVIEW, never APPROVE — ADR-0017 §3.)
type Effect string

const (
	// EffectComment is a non-blocking informational effect (ADR-0017 §2).
	EffectComment Effect = "comment"
	// EffectChallenge is a resolvable acknowledgement (ADR-0017 §3).
	EffectChallenge Effect = "challenge"
	// EffectBlock is a hard block (ADR-0017 §2).
	EffectBlock Effect = "block"
	// EffectRequireReview needs forge-proven eligible approval (ADR-0017 §3).
	EffectRequireReview Effect = "require-review"
)

// OnFailure is a rule's failure declaration (ADR-0017 §2): the effect applied
// and the stable finding code echoed into the DecisionRecord/PresentationModel.
type OnFailure struct {
	Effect Effect
	Code   string
}

// Rule is one assert/CEL rule that proves exactly one obligation (ADR-0017 §2).
// This is the MINIMAL rule type the walking skeleton needs — multi-obligation
// composition, points/scoring, and require-review authorization are E2.
type Rule struct {
	// Name identifies the rule (finding.rule); part of the canonical sort key.
	Name string
	// Obligation is the single obligation this rule proves (prove.obligation).
	Obligation string
	// When is the CEL assert expression; it may reference old, new, and changes
	// (see bindActivation). It MUST evaluate to a boolean; a non-bool result
	// fails safe to REVIEW.
	When string
	// OnFailure is applied when When is cleanly false.
	OnFailure OnFailure
}

// Binding is the minimal ADR-0017 §2 binding: one require list plus the rules
// that prove those obligations. AND-only composition (every required obligation
// must be proven to arm); this slice carries exactly one required obligation.
type Binding struct {
	// Require is the list of obligation names that must each be proven for APPROVE.
	Require []string
	// Rules are the assert rules; each proves one obligation.
	Rules []Rule
	// Subject is the governed-subject entryRef this binding evaluates over
	// (finding.subject). In the walking skeleton it is the single file subject.
	Subject string
}

// Decision is the reduced outcome (ADR-0017 §2). BLOCK dominates REVIEW
// dominates APPROVE (denies are a union, §2).
type Decision string

const (
	// DecisionApprove means every required obligation was proven cleanly true.
	DecisionApprove Decision = "APPROVE"
	// DecisionReview is the fail-safe outcome: an undecidable/errored/unproven
	// obligation, an opaque or empty ChangeSet, or a non-block unsatisfied effect.
	DecisionReview Decision = "REVIEW"
	// DecisionBlock is an unsatisfied block effect (or the reserved assent-policy class).
	DecisionBlock Decision = "BLOCK"
)

// severity orders decisions for the max-severity reduction (BLOCK > REVIEW > APPROVE).
func (d Decision) severity() int {
	switch d {
	case DecisionBlock:
		return 2
	case DecisionReview:
		return 1
	default:
		return 0
	}
}

// worse returns the more severe of two decisions (the union of denies, §2).
func worse(a, b Decision) Decision {
	if b.severity() > a.severity() {
		return b
	}
	return a
}

// Finding is one emitted obligation-proving-rule outcome, shaped after the
// DecisionRecord #/$defs/finding object (S04 serializes the full record; this
// package produces the finding set and the decision). A finding is emitted for
// an UNSATISFIED or UNDECIDABLE obligation only; a satisfied obligation is silent.
type Finding struct {
	Rule       string `json:"rule"`
	Obligation string `json:"obligation,omitempty"`
	Effect     Effect `json:"effect"`
	Subject    string `json:"subject"`
	Points     int    `json:"points"`
	Code       string `json:"code,omitempty"`
	// Message is the failing leaf's expanded per-leaf message (ADR-0013 E2-S03):
	// when an all/any/not (or single-leaf) `when` is unsatisfied, the attributed
	// leaf's `message` — with {{ old }}/{{ new }}/{{ facts.* }} template expansion
	// over the SAME activation model the CEL leaf saw — names WHICH conjunct failed.
	// omitempty keeps every pre-S03 finding (bare-string/no-message leaves, incl.
	// the D-016 golden) byte-identical, and record.go does not project it into the
	// serialized DecisionRecord finding (the frozen schema has no message field).
	Message string `json:"message,omitempty"`
}

// Result is the aggregator output: the reduced decision and the canonically
// sorted findings that justify it. Findings are sorted by a TOTAL key so a
// shuffled rule input yields a byte-identical Result (REQ-P4-E1-S03-03).
type Result struct {
	Decision Decision  `json:"decision"`
	Findings []Finding `json:"findings"`
	// Observed carries the findings produced by OBSERVE-phase rules (E2-S08,
	// ADR-0018 §1). They are evaluated and recorded but STRUCTURALLY EXCLUDED from
	// aggregation — they never enter the decision reduction, the points sum, or the
	// capability-gap set (Findings is the enforcing bucket that does). Routed here
	// at the point of production, not filtered post-hoc. Canonically sorted like
	// Findings. omitempty keeps the no-observe Result (the D-016 golden) byte-
	// identical, and record.go threads it into DecisionRecord findings.observed
	// (was hardcoded []).
	Observed []Finding `json:"observed,omitempty"`
	// CapabilityGaps records, per governed subject, a forge capability gap
	// discovered while satisfying a require-review obligation (E2-S07): an
	// injected ApprovalEvidence with verifyingCapability:none. It is the
	// aggregate-layer precursor to DecisionRecord pins.capabilityGap (S10 threads
	// it there); recorded here so a capability gap stays DISTINCT from a plain
	// missing approval (a require-review finding with no gap) — the
	// d016_missing_approval invariant. omitempty keeps the no-evidence Result
	// (D-016 golden) byte-identical. A gap NEVER satisfies, so the require-review
	// finding still stands and the run can never auto-merge.
	CapabilityGaps map[string]string `json:"capabilityGaps,omitempty"`
	// Profile is the resolved covering profile's identity (E2-S09, ADR-0018 §2),
	// stamped by WithProfile. Empty when no profile covers the binding (or none
	// were declared — the D-016 case). Surfaced at the engine layer only; E4
	// threads it into the DecisionRecord once the frozen schema carries the field.
	Profile string `json:"profile,omitempty"`
	// WriteAllowed is whether the resolved profile holds forge write authority
	// (spec.writes) for this binding (E2-S09). It is the SAFE value false unless a
	// single covering writes:true profile resolved — a recorder-only (writes:false)
	// profile never sets it, and an uncovered/undeclared binding defaults to false.
	// A downstream forge step reads it to know whether this run may arm/merge.
	// omitempty keeps the no-profile Result (the D-016 golden) byte-identical.
	WriteAllowed bool `json:"writeAllowed,omitempty"`
}

// Synthetic finding.rule names for outcomes not attributable to a single
// authored rule. The DecisionRecord #/$defs/finding schema requires a non-empty
// rule (minLength:1), so a fail-safe finding must never carry an empty rule.
// S04 serializes these findings into the DecisionRecord; the aggregate Finding
// shape mirrors that schema's finding object, and S04 owns points provenance.
const (
	// ruleUndecidable labels a REVIEW from an opaque/empty ChangeSet or an env
	// failure (no authored rule was reached).
	ruleUndecidable = "aggregate.changeset"
	// ruleUncovered labels a REVIEW from a required obligation with no proving rule.
	ruleUncovered = "aggregate.uncovered"
	// ruleUnmatchedDelete labels the fail-safe REVIEW an ungoverned whole-file DELETE
	// event earns (EFE-S02, Judgment call (a) / D-063): a delete no evaluated
	// fileEvents rule covers must never silently APPROVE.
	ruleUnmatchedDelete = "aggregate.unmatchedDelete"
)

// ReservedPolicyClass is the built-in meta-class (ADR-0008/ADR-0015 §1) that an
// MR editing its own .assent/** policy lands in. A subject in this class
// DOMINATES to BLOCK independent of any predicate — an MR cannot vouch itself.
const ReservedPolicyClass = "assent-policy"

// Aggregate evaluates the binding's single-obligation rules over the S02
// ChangeSet and reduces to a decision, fail-safe throughout.
//
// S07-01 SEAM: subjectClass is the per-subject class signal a later serialized
// edit (internal/core/classify, another lane) computes. When it equals
// ReservedPolicyClass the aggregator SHORT-CIRCUITS to BLOCK *before any
// predicate evaluation* — the reserved-class meta-block dominates even a
// satisfied assert (ADR-0008 amendment). This lane does NOT build the classifier;
// it only leaves this dominating hook so S07-01 wires in cleanly. Pass "" when
// no class is known.
func Aggregate(b Binding, cs change.ChangeSet, subjectClass string) (Result, error) {
	// 1. Reserved-class meta-block dominates before any predicate (S07-01 seam).
	if subjectClass == ReservedPolicyClass {
		return Result{
			Decision: DecisionBlock,
			Findings: []Finding{{
				Rule:    ReservedPolicyClass,
				Effect:  EffectBlock,
				Subject: b.Subject,
				Points:  0,
				Code:    "assent-policy.self-edit",
			}},
		}, nil
	}

	// 2. An opaque ChangeSet is undecidable -> REVIEW (never a silent APPROVE).
	// 3. An empty change list would violate the schema minItems:1 downstream and
	//    means the differ saw no change to prove an obligation over -> REVIEW.
	if cs.Opaque || len(cs.Changes) == 0 {
		return failSafe(b), nil
	}

	env, err := newCELEnv()
	if err != nil {
		// An environment that cannot be built is a program bug, not an input;
		// still fail safe rather than proceed.
		return failSafe(b), err
	}
	activation := bindActivation(cs)

	// Index rules by the obligation they prove, so coverage is computed as
	// "the required obligation is proven by a cleanly-true rule", not
	// "no rule returned false" (fail-open avoidance).
	rulesByObligation := map[string][]Rule{}
	for _, r := range b.Rules {
		rulesByObligation[r.Obligation] = append(rulesByObligation[r.Obligation], r)
	}

	decision := DecisionApprove
	var findings []Finding

	for _, obligation := range b.Require {
		rules := rulesByObligation[obligation]
		if len(rules) == 0 {
			// A required obligation with NO proving rule can never be proven ->
			// REVIEW (coverage gap, never APPROVE).
			decision = worse(decision, DecisionReview)
			findings = append(findings, Finding{
				Rule:       ruleUncovered,
				Obligation: obligation,
				Effect:     EffectRequireReview,
				Subject:    b.Subject,
				Points:     0,
				Code:       "obligation.uncovered",
			})
			continue
		}
		for _, r := range rules {
			satisfied, evalErr := evalRule(env, activation, r.When)
			switch {
			case evalErr != nil:
				// Predicate compile/eval/coercion error or non-bool result ->
				// REVIEW (tri-state fail-safe, ADR-0017 §6, REQ-S03-02).
				decision = worse(decision, DecisionReview)
				findings = append(findings, Finding{
					Rule:       r.Name,
					Obligation: r.Obligation,
					Effect:     EffectRequireReview,
					Subject:    b.Subject,
					Points:     0,
					Code:       "predicate.error",
				})
			case satisfied:
				// Obligation proven by this rule; contributes no finding.
			default:
				// Cleanly false -> the rule's onFailure effect (never APPROVE).
				decision = worse(decision, effectDecision(r.OnFailure.Effect))
				findings = append(findings, Finding{
					Rule:       r.Name,
					Obligation: r.Obligation,
					Effect:     r.OnFailure.Effect,
					Subject:    b.Subject,
					Points:     0,
					Code:       r.OnFailure.Code,
				})
			}
		}
	}

	sortFindings(findings)
	return Result{Decision: decision, Findings: findings}, nil
}

// effectDecision maps an unsatisfied onFailure effect to a decision: block ->
// BLOCK, everything else -> REVIEW. Confirmed against ADR-0017 §2 (every
// obligation must be satisfied to arm; denies are a union) and §3 (require-review
// never degrades to an author-resolvable pass; if unproven, not armed). An
// unsatisfied effect can NEVER yield APPROVE.
func effectDecision(e Effect) Decision {
	if e == EffectBlock {
		return DecisionBlock
	}
	return DecisionReview
}

// failSafe builds the REVIEW result for an undecidable ChangeSet (opaque or
// empty), attaching one finding so the outcome is auditable.
func failSafe(b Binding) Result {
	return Result{
		Decision: DecisionReview,
		Findings: []Finding{{
			Rule:    ruleUndecidable,
			Effect:  EffectRequireReview,
			Subject: b.Subject,
			Points:  0,
			Code:    "changeset.undecidable",
		}},
	}
}

// sortFindings orders findings by a TOTAL key (subject, rule, obligation, code,
// effect) so a shuffled rule input yields a byte-identical Findings slice
// (REQ-P4-E1-S03-03 order-independence).
func sortFindings(fs []Finding) {
	sort.Slice(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Subject != b.Subject {
			return a.Subject < b.Subject
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Obligation != b.Obligation {
			return a.Obligation < b.Obligation
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Effect < b.Effect
	})
}

// newCELEnv builds the CEL environment. old/new are bound as strings (the
// differ's canonical forms); changes is the list of per-entry maps. Numeric
// comparison is the responsibility of the `when` expression via int()/double().
func newCELEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("old", cel.StringType),
		cel.Variable("new", cel.StringType),
		cel.Variable("changes", cel.ListType(cel.MapType(cel.StringType, cel.StringType))),
	)
}

// bindActivation builds the CEL activation from the ChangeSet.
//
// changes is ALWAYS bound to the full list of entries (each a {subject, file,
// path, kind, old, new} string map) — an entry's PRESENCE is the change signal
// (constraint d), so a rule reads `changes` to reason over what changed.
//
// old/new are the convenience scalar bindings for the single-entry case (the
// walking-skeleton shape). They are bound ONLY when there is exactly one entry.
// With 0 or >1 entries the old/new keys are OMITTED from the activation
// ENTIRELY (not bound to a "" default): they stay DECLARED in newCELEnv so
// Compile still succeeds, but a predicate that references them then hits an Eval
// error `no such attribute(s): old` -> the evalRule error path -> REVIEW, for
// EVERY predicate type. Binding "" would fail-OPEN for a non-numeric predicate:
// a real 2-field diff with `when: "old == new"` would evaluate `"" == ""` ->
// true -> APPROVE on an unproven obligation that inspected none of the real
// changes. Omission makes the fail-safe UNCONDITIONAL on predicate type
// (GUIDELINES §2). A rule that needs multi-entry reasoning must iterate `changes`
// (always bound), not the scalar convenience bindings.
func bindActivation(cs change.ChangeSet) map[string]any {
	changesList := make([]map[string]string, len(cs.Changes))
	for i, c := range cs.Changes {
		changesList[i] = map[string]string{
			"subject": "file:" + c.File,
			"file":    c.File,
			"path":    c.Path,
			"kind":    string(c.Kind),
			"old":     c.Old,
			"new":     c.New,
		}
	}
	act := map[string]any{"changes": changesList}
	// Bind scalar old/new ONLY for the single-entry case; otherwise leave them
	// unbound so any reference errors -> REVIEW (fail-safe on every predicate type).
	if len(cs.Changes) == 1 {
		act["old"] = cs.Changes[0].Old
		act["new"] = cs.Changes[0].New
	}
	return act
}

// evalRule compiles and evaluates one `when` expression. It returns (satisfied,
// nil) ONLY when the predicate compiled, evaluated without error, produced a
// boolean, and ordered nothing lexically. Every other outcome — a compile error
// (undecidable `when`), an eval error (incl. numeric-coercion failure, surfaced
// via the error slot OR a types.Err value), a non-boolean result, or an ordering
// operator over a text operand — returns a non-nil error so the caller fails safe
// to REVIEW. It NEVER returns (true, nil) for a malformed rule.
//
// The D-129 textOrderGuard is applied here too, not only in evalLeaf. This
// walking-skeleton env declares old/new as StringType and binds the differ's RAW
// canonical strings, so EVERY bare relational here is a lexical compare — the
// path mandated int()/double() by convention alone, with nothing enforcing it.
// Guarding both evaluators is deliberate: one evaluation seam left unguarded is
// how this class of fail-open comes back (the same drift argument that pulled the
// canonical decoder into internal/evaldecode, D-055c).
func evalRule(env *cel.Env, activation map[string]any, when string) (bool, error) {
	ast, iss := env.Compile(when)
	if iss != nil && iss.Err() != nil {
		return false, fmt.Errorf("compile when %q: %w", when, iss.Err())
	}
	guard := newTextOrderGuard(ast)
	prg, err := env.Program(ast, cel.CostLimit(celCostBudget), cel.CustomDecorator(guard.decorate))
	if err != nil {
		return false, fmt.Errorf("program when %q: %w", when, err)
	}
	out, _, evalErr := prg.Eval(activation)
	if evalErr != nil {
		return false, fmt.Errorf("eval when %q: %w", when, evalErr)
	}
	// A types.Err result carries the error in the value slot (cel-go can surface
	// a coercion failure either way — verified empirically); reject both.
	if out == nil || types.IsError(out) {
		return false, fmt.Errorf("eval when %q produced an error value", when)
	}
	if err := guard.err(); err != nil {
		return false, fmt.Errorf("eval when %q: %w", when, err)
	}
	b, ok := out.Value().(bool)
	if !ok {
		// A non-boolean `when` is malformed; it must NOT be read as true/false.
		return false, fmt.Errorf("when %q result is %s, not bool", when, out.Type().TypeName())
	}
	return b, nil
}
