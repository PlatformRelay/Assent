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
// ("6" >= "12" is lexically true, "12" >= "6" lexically false) — so since D-131
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
	"sort"
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
	// ruleUnmatchedEdit labels the fail-safe REVIEW a value-level change earns when
	// NO enforce-effective prove rule selects it, under a binding that requires an
	// obligation some rule proves (RVW-S02 / C-1): an edit outside every rule's
	// match scope is not positively vouched, so it must never silently APPROVE.
	ruleUnmatchedEdit = "aggregate.unmatchedEdit"
)

// ReservedPolicyClass is the built-in meta-class (ADR-0008/ADR-0015 §1) that an
// MR editing its own .assent/** policy lands in. A subject in this class
// DOMINATES to BLOCK independent of any predicate — an MR cannot vouch itself.
const ReservedPolicyClass = "assent-policy"

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
