package aggregate

// risk.go is the E2-S06 aggregation tail (ADR-0007 + Amendment 2): author-declared
// rule.points accrue PER FIRING and the binding risk.threshold gates APPROVE as
// decision order #4 — the LAST check, run only after block (#1), unresolved
// challenge (#2), and uncovered/unproven obligations (#3) have been reduced. It
// adds no clock/rand/env/network (ADR-0013 purity holds): integer arithmetic only.

// firingEffectDecision maps a FIRING finding's effect to its decision
// contribution inside the coverage loop. It differs from effectDecision (the
// walking-skeleton Aggregate mapping) in exactly ONE case: `comment` is
// DECISION-NEUTRAL. Per ADR-0007's effect table a comment "Blocks merge? no" — it
// is the SOFT signal channel: a comment firing accrues its author-declared points
// and is escalated to REVIEW ONLY when the summed points exceed the binding
// threshold (order #4 below), never by its own presence. block -> BLOCK and
// challenge/require-review -> REVIEW remain hard, decision-lowering effects.
//
// This is the ADR-0007 "many small oddities add up to human-please-look"
// mechanism; without a decision-neutral soft effect the risk threshold could
// never be the deciding factor (every hard effect already forces REVIEW/BLOCK
// before order #4 is reached).
func firingEffectDecision(e Effect) Decision {
	if e == EffectComment {
		return DecisionApprove // neutral: escalated only via the points threshold
	}
	return effectDecision(e)
}

// riskDecision is aggregation order #4: a points sum that EXCEEDS the binding
// threshold escalates an otherwise-APPROVE decision to REVIEW; sum <= threshold
// contributes APPROVE (neutral). worse() then reduces it against the earlier
// orders, so a BLOCK/REVIEW already decided by #1..#3 is never displaced.
//
// Fail-safe direction (GUIDELINES §2): points OVER-count -> OVER-REVIEW is the
// SAFE error; an under-count that produced a wrong APPROVE is the forbidden one.
// Strict `>` (not `>=`) matches ADR-0007: sum EQUAL to the threshold still
// APPROVEs ("<= threshold => APPROVE").
func riskDecision(sum, threshold int) Decision {
	if sum > threshold {
		return DecisionReview
	}
	return DecisionApprove
}
