package aggregate

// approval.go is the E2-S07 decision-side of require-review (ADR-0017 §3): the
// engine-facing form of the FROZEN approval-evidence.schema.json
// (schemas.ApprovalEvidenceSchema) plus the satisfaction predicate a
// require-review obligation is proven against. The evidence is SEPARATELY
// INJECTED, pre-fetched by the forge tier (E4); this package NEVER fetches it and
// NEVER reaches a network/clock (TestCorePurity).
//
// A require-review obligation is satisfied for a subject IFF ALL hold — else the
// finding STANDS (fail-closed, never auto-merge):
//  1. an ApprovalEvidence is present (nil ⇒ unsatisfied — never satisfied by absence);
//  2. verifyingCapability is a REAL capability (approval-rules-api|codeowners);
//     `none` ⇒ a capability gap (recorded, distinct from a missing approval), never a satisfaction;
//  3. the evidence is not expired (expiry is PRE-COMPUTED by the fetch tier — see Expired);
//  4. pins.sourceSha equals the evaluated sourceSha — STALE evidence (mismatch or
//     absent sha) NEVER satisfies (the highest-value fail-open trap, REQ-07-02);
//  5. approvalsRequired is met by approvedBy[] AFTER excluding the MR author
//     (self-approval) and any approver not in the forge-proven eligible set (bots).
//
// Determinism (ADR-0013): pure over its inputs. The satisfaction order fails
// closed at the FIRST unmet condition, so an ambiguous/partial evidence can only
// stay unsatisfied, never leak an APPROVE.
//
// The evidence enters as an already-validated Go value (spec Non-goals: E2 owns
// "the decision-side satisfaction logic over an already-injected
// ApprovalEvidence"; the FETCH tier E4 validates it against the frozen
// schemas.ApprovalEvidenceSchema at fetch time). This package therefore does not
// re-validate JSON here — and cannot import that schema, which the schemas
// package exposes only to its own tests. The field names below deliberately
// mirror the frozen approval-evidence.schema.json so the Go type is not a fork.

// ApprovalEvidence is the engine-facing form of the frozen
// approval-evidence.schema.json — the ONLY contract a require-review obligation
// may be satisfied against (ADR-0017 §3). Field names mirror the frozen schema
// (verifyingCapability, approvalsRequired, approvedBy, eligibility, pins) so the
// Go type is not a fork; LoadApprovalEvidence validates against the frozen schema
// before decoding into it.
type ApprovalEvidence struct {
	// VerifyingCapability is the forge capability that proved this evidence:
	// "approval-rules-api" | "codeowners" | "none". "none" is a capability gap.
	VerifyingCapability string
	// ApprovalsRequired is the forge rule threshold (approval_rules[].approvals_required).
	ApprovalsRequired int
	// ApprovedBy are the ACTUAL approvers for this rule (approval_state).
	ApprovedBy []Approver
	// Eligibility is the forge-proven eligible-approver id set (eligibleApproverIds).
	// An approver counts toward the threshold only if its id is in this set —
	// this is how a bot / non-eligible principal is excluded (ADR-0017 §3
	// "forge-proven eligible approval"), the frozen schema carrying no bot flag.
	Eligibility []string
	// Pins mirrors the DecisionRecord pins the evidence was observed against; only
	// SourceSha is load-bearing for staleness in this lane.
	Pins ApprovalPins
	// Expired is PRE-COMPUTED by the fetch tier (E4), which holds the clock. Like
	// facts[].state (S05), expiry state is CARRIED, never recomputed against a
	// clock here — internal/core is clock-free (TestCorePurity). The frozen schema
	// carries only an expiresAt timestamp; interpreting it against "now" is a
	// forge-tier (E4) responsibility, reported as a design flag for S07.
	Expired bool
}

// Approver is one actual approver entry (id/username/isAuthor). The frozen schema
// forces isAuthor:false on every approvedBy entry (an adapter honesty invariant),
// so self-approval is caught by identity match to the MR author, with IsAuthor
// checked as defense-in-depth.
type Approver struct {
	ID       string
	Username string
	IsAuthor bool
}

// ApprovalPins is the load-bearing slice of the evidence pins for S07: the source
// SHA the approval was observed against (staleness gate).
type ApprovalPins struct {
	SourceSha string
}

// ApprovalContext is the E2-S07 injection into the decision entry (CoverWithApproval):
// the evaluated sourceSha and the per-governed-subject pre-fetched evidence. It is
// a SEPARATE injected input, never a field on the frozen (closed) EvaluationInput
// (spec judgment call). A nil ApprovalContext or a subject absent from Evidence ⇒
// no evidence for that subject ⇒ unsatisfied.
//
// Scoping fence: one ApprovalEvidence per subject satisfies ANY authored
// require-review firing for that subject in this lane; per-obligation matching via
// source.rule (a subject carrying two distinct require-review obligations) is NOT
// modelled here — a documented S07 boundary, not a silent fail-open, because a
// missing evidence still fails closed.
type ApprovalContext struct {
	// SourceSha is the evaluated source SHA (DecisionRecord pins.sourceSha) the
	// decision is being made against. Empty ⇒ every evidence is treated stale
	// (fail-closed): a caller that injects evidence but forgets the sha never satisfies.
	SourceSha string
	// Evidence maps a governed subject entryRef -> its injected ApprovalEvidence.
	Evidence map[string]*ApprovalEvidence
}

// capabilityGapNone is the recorded reason for a verifyingCapability:none gap.
const capabilityGapNone = "approval-capability-none"

// evidenceFor returns the injected evidence for subj, or nil.
func (a *ApprovalContext) evidenceFor(subj string) *ApprovalEvidence {
	if a == nil || a.Evidence == nil {
		return nil
	}
	return a.Evidence[subj]
}

// sourceSha returns the evaluated source SHA, or "" when no context is injected.
func (a *ApprovalContext) sourceSha() string {
	if a == nil {
		return ""
	}
	return a.SourceSha
}

// approvalSatisfies reports whether ev satisfies a require-review obligation for
// the evaluated sourceSha and MR author, and whether ev is a capability gap
// (verifyingCapability:none). A gap NEVER satisfies but is recorded distinctly.
// Every negative branch fails CLOSED — the caller keeps the require-review finding.
func approvalSatisfies(ev *ApprovalEvidence, sourceSha, mrAuthor string) (satisfied, capabilityGap bool) {
	if ev == nil {
		return false, false // absence never satisfies (REQ-07-01)
	}
	switch ev.VerifyingCapability {
	case "approval-rules-api", "codeowners":
		// a real forge capability — continue checking.
	case "none":
		return false, true // capability gap (REQ-07-04): never auto-merge, recorded distinct
	default:
		return false, false // unknown/absent capability -> fail closed
	}
	if ev.Expired {
		return false, false // pre-computed expiry (REQ-07-05: non-expired)
	}
	if sourceSha == "" || ev.Pins.SourceSha == "" || ev.Pins.SourceSha != sourceSha {
		return false, false // stale/absent sha NEVER satisfies (REQ-07-02, the top trap)
	}
	if ev.ApprovalsRequired < 1 {
		return false, false // a non-positive threshold is unprovable -> fail closed
	}
	eligible := 0
	for i := range ev.ApprovedBy {
		if countsAsApprover(ev.ApprovedBy[i], ev.Eligibility, mrAuthor) {
			eligible++
		}
	}
	if eligible < ev.ApprovalsRequired {
		return false, false // threshold unmet by eligible non-author approvers (REQ-07-03)
	}
	return true, false // fully valid, eligible, sha-matching, non-expired (REQ-07-05)
}

// countsAsApprover reports whether an approver counts toward approvalsRequired:
// forge-proven eligible (id in the eligible set) AND not the MR author (self-
// approval excluded) AND not flagged isAuthor. A bot / non-eligible principal is
// excluded by the eligibility gate (ADR-0017 §3), the schema carrying no bot flag.
func countsAsApprover(a Approver, eligible []string, mrAuthor string) bool {
	if a.IsAuthor {
		return false // defense-in-depth (schema forces isAuthor:false already)
	}
	if mrAuthor != "" && a.Username == mrAuthor {
		return false // self-approval (REQ-07-03)
	}
	return contains(eligible, a.ID) // forge-proven eligible only (excludes bots)
}
