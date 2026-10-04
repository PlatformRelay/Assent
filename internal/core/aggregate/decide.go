package aggregate

import (
	"fmt"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// DecideRequest carries the guard signals and the decision inputs for the single
// guarded engine entry (XREV-S01 / D2). The guard signals are explicit because
// the frozen EvaluationInput wire shape carries neither the subject class nor
// opacity (that would be a contract change): the caller supplies them.
type DecideRequest struct {
	// Subject is the governed-subject entryRef, used to attribute the synthetic
	// reserved-class and undecidable findings.
	Subject string
	// SubjectClass is the classified subject class; ReservedPolicyClass dominates.
	SubjectClass string
	// Opaque marks a changeset the differ could not decide (fail-safe REVIEW).
	Opaque bool
	// Policy/Binding are the loaded policy and the routed binding.
	Policy  *policy.MergePolicy
	Binding *policy.Binding
	// Input is the decoded evaluation input. Required for the non-reserved paths.
	Input *EvaluationInput
	// Approval is the optional injected approval evidence.
	Approval *ApprovalContext
	// Ceiling is the pack phase ceiling (empty normalizes to enforce).
	Ceiling policy.Phase
	// Precedence/Profiles are the profile table for write-authority resolution;
	// empty is the safe default (no write authority).
	Precedence []policy.ProfileRef
	Profiles   []*policy.Profile
}

// Decide is the ONE guarded engine entry (XREV-S01 / D2). It re-asserts the three
// dominating fail-safe guards AROUND the coverage loop, so a caller cannot
// accidentally reach a vacuous APPROVE:
//
//   - reserved-class self-edit (assent-policy) dominates to BLOCK before any
//     predicate — an MR cannot vouch for itself (ADR-0015 §1, D-042).
//   - an opaque OR empty changeset is undecidable -> REVIEW, never a silent
//     APPROVE (the coverage loop treats "no matched change" as an obligation that
//     does not apply).
//   - an empty/absent require[] makes the obligation layer vacuous -> a hard error
//     refusing to arm APPROVE (RVW-S01 / D-184, GUIDELINES §Safety-1).
//
// CAVEAT (stated so this does not overstate itself): the frozen EvaluationInput
// wire shape carries neither the subject class nor opacity, so the reserved-class
// and opaque guards read CALLER-SUPPLIED signals (req.SubjectClass / req.Opaque).
// The engine cannot derive them. Every live caller supplies them correctly — run
// from the classifier and the differ's opaque flag, adoptertest short-circuits its
// undecidable path before Decide, and compare's frozen bundle can represent
// neither — but a future caller that lies about opacity is not structurally
// stopped by this entry. The empty-require and empty-changeset guards ARE derived
// from the input and cannot be bypassed.
//
// Otherwise it runs the profile-aware coverage loop (empty precedence/profiles is
// the safe no-write-authority default), so one entry serves the run path, the
// adopter harness, and the comparison engine.
func Decide(req DecideRequest) (Result, error) {
	if req.SubjectClass == ReservedPolicyClass {
		return reservedClassBlock(req.Subject), nil
	}
	if req.Input == nil {
		return Result{}, fmt.Errorf("decide: nil evaluation input")
	}
	if req.Opaque || len(req.Input.ChangeSet.Changes) == 0 {
		return undecidableReview(req.Subject), nil
	}
	if req.Binding == nil || len(req.Binding.Require) == 0 {
		class, env := "", ""
		if req.Binding != nil {
			class, env = req.Binding.Class, req.Binding.Environment
		}
		return Result{}, fmt.Errorf("ruleset-binding binding (class=%q, environment=%q) declares no required obligations (require is empty) — refusing to arm APPROVE; add at least one obligation to require[] (GUIDELINES §Safety-1, D-184)", class, env)
	}
	return CoverWithProfile(req.Policy, req.Binding, req.Input, req.Approval, req.Ceiling, req.Precedence, req.Profiles)
}

// reservedClassBlock is the reserved-class self-edit BLOCK result (ADR-0015 §1):
// a smuggled `.assent/**` edit can never vouch for itself. It reconstructs exactly
// what the walking skeleton emitted before the guarded entry absorbed it.
func reservedClassBlock(subject string) Result {
	return Result{
		Decision: DecisionBlock,
		Findings: []Finding{{
			Rule:    ReservedPolicyClass,
			Effect:  EffectBlock,
			Subject: subject,
			Points:  0,
			Code:    "assent-policy.self-edit",
		}},
	}
}

// undecidableReview is the fail-safe REVIEW result for an opaque/empty changeset,
// so the outcome is auditable (never a silent APPROVE).
func undecidableReview(subject string) Result {
	return Result{
		Decision: DecisionReview,
		Findings: []Finding{{
			Rule:    ruleUndecidable,
			Effect:  EffectRequireReview,
			Subject: subject,
			Points:  0,
			Code:    "changeset.undecidable",
		}},
	}
}
