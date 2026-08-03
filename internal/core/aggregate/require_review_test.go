package aggregate

// require_review_test.go is the E2-S07 merge-gate security lane: a require-review
// obligation is satisfied ONLY by a separately-injected, forge-proven, eligible,
// sha-matching, non-expired ApprovalEvidence (ADR-0017 §3). The tests below are
// TDD-first and each closes a documented fail-open trap:
//   - absence (REQ-07-01): nil evidence never satisfies;
//   - stale sha (REQ-07-02): a sha mismatch never satisfies — the highest-value trap;
//   - self/bot (REQ-07-03): the MR author and any non-eligible (bot) approver do
//     not count toward approvalsRequired;
//   - none-capability (REQ-07-04): verifyingCapability:none records a capability
//     gap (distinct from a missing approval) and can never auto-merge;
//   - satisfaction path (REQ-07-05): a fully-valid evidence suppresses the finding.
// Plus two implicit-trap tests the REQs do not enumerate but the lane implies:
// evidence must NEVER suppress a co-occurring block, and must NEVER rescue a
// predicate.error require-review (structural gating, not Effect-based filtering).

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// ownershipRequireReviewPolicy is a minimal single-rule policy whose ownership
// obligation fires require-review on a clean-false `when` — the authored
// require-review obligation ApprovalEvidence may satisfy (D-016 owner-rule shape).
func ownershipRequireReviewPolicy() *policy.MergePolicy {
	return &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "topic-owner-must-approve",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
				Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "false"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "ownership-approval-missing"},
			}},
		},
	}
}

func ownershipBinding() *policy.Binding {
	// A generous threshold so the ONLY thing preventing APPROVE is the
	// require-review obligation — satisfying it must reach APPROVE (REQ-07-05).
	return &policy.Binding{Require: []string{"ownership"}, Environment: "prod", Risk: policy.Risk{Threshold: 100}}
}

func oneSubjectInput(subject string) *EvaluationInput {
	return &EvaluationInput{
		MR: MR{Author: "mona-author"},
		ChangeSet: ChangeSet{Changes: []EvalChange{
			{Subject: subject, File: "topics/a.yaml", Path: "/x", Kind: "modify", Old: intNum(1), New: intNum(2)},
		}},
	}
}

// validEvidence is a fully-eligible, sha-matching, non-expired evidence: one
// eligible non-author approver meeting approvalsRequired: 1.
func validEvidence(sourceSha string) *ApprovalEvidence {
	return &ApprovalEvidence{
		VerifyingCapability: "approval-rules-api",
		ApprovalsRequired:   1,
		Eligibility:         []string{"reviewer-id"},
		ApprovedBy:          []Approver{{ID: "reviewer-id", Username: "rita-reviewer", IsAuthor: false}},
		Pins:                ApprovalPins{SourceSha: sourceSha},
		Expired:             false,
	}
}

// TestRequireReviewUnsatisfiedWithoutEvidence — REQ-07-01: nil evidence (and the
// no-approval Cover entry) never satisfy a require-review obligation.
func TestRequireReviewUnsatisfiedWithoutEvidence(t *testing.T) {
	subj := "topic-registry:orders.events.v1"
	pol, bind, in := ownershipRequireReviewPolicy(), ownershipBinding(), oneSubjectInput(subj)

	// (a) the plain no-approval entry.
	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	assertRequireReviewStands(t, got, subj)

	// (b) an explicit ApprovalContext with a nil evidence map behaves identically.
	got2, err := CoverWithApproval(pol, bind, in, &ApprovalContext{SourceSha: "abc123"})
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	assertRequireReviewStands(t, got2, subj)
	if len(got2.CapabilityGaps) != 0 {
		t.Errorf("a plain missing approval must not record a capability gap, got %v", got2.CapabilityGaps)
	}
}

// TestStaleApprovalEvidenceDoesNotSatisfy — REQ-07-02, the highest-value trap: an
// otherwise fully-eligible, capability-proven evidence whose pins.sourceSha does
// NOT match the evaluated sourceSha must fail to satisfy (the finding stands).
func TestStaleApprovalEvidenceDoesNotSatisfy(t *testing.T) {
	subj := "topic-registry:orders.events.v1"
	pol, bind, in := ownershipRequireReviewPolicy(), ownershipBinding(), oneSubjectInput(subj)

	appr := &ApprovalContext{
		SourceSha: "current-sha-AAAA",
		Evidence:  map[string]*ApprovalEvidence{subj: validEvidence("superseded-sha-BBBB")},
	}
	got, err := CoverWithApproval(pol, bind, in, appr)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	assertRequireReviewStands(t, got, subj)
	if len(got.CapabilityGaps) != 0 {
		t.Errorf("stale evidence is a missing approval, not a capability gap: %v", got.CapabilityGaps)
	}
}

// TestExpiredApprovalEvidenceDoesNotSatisfy — REQ-07-05 ("non-expired") and the
// ADR-0017 §4 arming precondition: an otherwise fully-valid, sha-matching,
// eligible evidence whose PRE-COMPUTED Expired flag is set must NOT satisfy (an
// expired authorization that auto-merges is the arming fail-open this lane
// closes). Expiry is carried, never recomputed against a clock (TestCorePurity).
func TestExpiredApprovalEvidenceDoesNotSatisfy(t *testing.T) {
	subj := "topic-registry:orders.events.v1"
	pol, bind, in := ownershipRequireReviewPolicy(), ownershipBinding(), oneSubjectInput(subj)

	ev := validEvidence("sha-1")
	ev.Expired = true // pre-computed by the fetch tier (E4), carried into the engine
	appr := &ApprovalContext{SourceSha: "sha-1", Evidence: map[string]*ApprovalEvidence{subj: ev}}
	got, err := CoverWithApproval(pol, bind, in, appr)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	assertRequireReviewStands(t, got, subj)
	if len(got.CapabilityGaps) != 0 {
		t.Errorf("expired evidence is a missing approval, not a capability gap: %v", got.CapabilityGaps)
	}
}

// TestFailClosedEvidenceGuards — belt-and-suspenders fail-closed branches: an
// unknown verifyingCapability (not one of the three enum values) and a
// non-positive approvalsRequired must both leave the obligation unsatisfied.
func TestFailClosedEvidenceGuards(t *testing.T) {
	subj := "topic-registry:orders.events.v1"
	pol, bind := ownershipRequireReviewPolicy(), ownershipBinding()

	cases := map[string]*ApprovalEvidence{
		"unknown verifyingCapability": func() *ApprovalEvidence {
			ev := validEvidence("sha-1")
			ev.VerifyingCapability = "totally-made-up"
			return ev
		}(),
		"non-positive approvalsRequired": func() *ApprovalEvidence {
			ev := validEvidence("sha-1")
			ev.ApprovalsRequired = 0
			return ev
		}(),
	}
	for name, ev := range cases {
		t.Run(name, func(t *testing.T) {
			in := oneSubjectInput(subj)
			appr := &ApprovalContext{SourceSha: "sha-1", Evidence: map[string]*ApprovalEvidence{subj: ev}}
			got, err := CoverWithApproval(pol, bind, in, appr)
			if err != nil {
				t.Fatalf("CoverWithApproval: %v", err)
			}
			assertRequireReviewStands(t, got, subj)
		})
	}
}

// TestSelfAndBotApprovalExcluded — REQ-07-03: an approver who is the MR author
// (self-approval) or not forge-proven eligible (a bot) does not count toward
// approvalsRequired, so the obligation stays unsatisfied.
func TestSelfAndBotApprovalExcluded(t *testing.T) {
	subj := "topic-registry:orders.events.v1"
	pol, bind := ownershipRequireReviewPolicy(), ownershipBinding()

	cases := map[string]*ApprovalEvidence{
		"self-approval (approver username == mr.author)": {
			VerifyingCapability: "approval-rules-api",
			ApprovalsRequired:   1,
			Eligibility:         []string{"mona-id"},
			ApprovedBy:          []Approver{{ID: "mona-id", Username: "mona-author", IsAuthor: false}},
			Pins:                ApprovalPins{SourceSha: "sha-1"},
		},
		"bot approver not in forge-proven eligible set": {
			VerifyingCapability: "approval-rules-api",
			ApprovalsRequired:   1,
			Eligibility:         []string{"reviewer-id"},
			ApprovedBy:          []Approver{{ID: "assent-bot-id", Username: "assent-bot", IsAuthor: false}},
			Pins:                ApprovalPins{SourceSha: "sha-1"},
		},
	}
	for name, ev := range cases {
		t.Run(name, func(t *testing.T) {
			in := oneSubjectInput(subj)
			appr := &ApprovalContext{SourceSha: "sha-1", Evidence: map[string]*ApprovalEvidence{subj: ev}}
			got, err := CoverWithApproval(pol, bind, in, appr)
			if err != nil {
				t.Fatalf("CoverWithApproval: %v", err)
			}
			assertRequireReviewStands(t, got, subj)
			if len(got.CapabilityGaps) != 0 {
				t.Errorf("an excluded approver is a missing approval, not a capability gap: %v", got.CapabilityGaps)
			}
		})
	}
}

// TestNoneCapabilityIsGapNeverAutoMerge — REQ-07-04: verifyingCapability:none is
// a capability gap. It never satisfies (finding stands, never APPROVE) AND is
// recorded distinctly (Result.CapabilityGaps) so it stays distinguishable from a
// plain missing approval (locking the d016_missing_approval invariant).
func TestNoneCapabilityIsGapNeverAutoMerge(t *testing.T) {
	subj := "topic-registry:orders.events.v1"
	pol, bind, in := ownershipRequireReviewPolicy(), ownershipBinding(), oneSubjectInput(subj)

	ev := &ApprovalEvidence{VerifyingCapability: "none", Pins: ApprovalPins{SourceSha: "sha-1"}}
	appr := &ApprovalContext{SourceSha: "sha-1", Evidence: map[string]*ApprovalEvidence{subj: ev}}
	got, err := CoverWithApproval(pol, bind, in, appr)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	assertRequireReviewStands(t, got, subj)
	if got.CapabilityGaps[subj] == "" {
		t.Errorf("verifyingCapability:none must record a capability gap for %q, got %v", subj, got.CapabilityGaps)
	}
}

// TestValidEligibleEvidenceSatisfies — REQ-07-05: a fully-valid evidence
// suppresses the require-review finding for that subject and, when it is the only
// bar to APPROVE, the decision is APPROVE — proving the path is not permanently closed.
func TestValidEligibleEvidenceSatisfies(t *testing.T) {
	subj := "topic-registry:orders.events.v1"
	pol, bind, in := ownershipRequireReviewPolicy(), ownershipBinding(), oneSubjectInput(subj)

	appr := &ApprovalContext{
		SourceSha: "current-sha-AAAA",
		Evidence:  map[string]*ApprovalEvidence{subj: validEvidence("current-sha-AAAA")},
	}
	got, err := CoverWithApproval(pol, bind, in, appr)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	for _, f := range got.Findings {
		if f.Effect == EffectRequireReview && f.Subject == subj {
			t.Errorf("valid evidence must suppress the require-review finding, got %+v", f)
		}
	}
	if got.Decision != DecisionApprove {
		t.Errorf("with the only obligation satisfied, decision = %q, want APPROVE", got.Decision)
	}
	if len(got.CapabilityGaps) != 0 {
		t.Errorf("a satisfied obligation records no capability gap, got %v", got.CapabilityGaps)
	}
}

// TestValidEvidenceDoesNotSuppressBlock — implicit trap: evidence satisfying a
// require-review obligation must NEVER suppress a co-occurring block on the same
// subject. Structural gating (only the require-review firing consults evidence)
// guarantees this; the test blocks a regression to Effect-based filtering.
func TestValidEvidenceDoesNotSuppressBlock(t *testing.T) {
	subj := "topic-registry:orders.events.v1"
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{
				{
					Name:      "topic-owner-must-approve",
					Phase:     policy.PhaseEnforce,
					Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
					Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "false"}}},
					OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "ownership-approval-missing"},
				},
				{
					Name:      "partitions-must-not-shrink",
					Phase:     policy.PhaseEnforce,
					Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
					Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
					OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "partition-count-shrunk"},
				},
			},
		},
	}
	bind := &policy.Binding{Require: []string{"ownership", "non-destructive"}, Environment: "prod", Risk: policy.Risk{Threshold: 100}}
	in := &EvaluationInput{
		MR: MR{Author: "mona-author"},
		ChangeSet: ChangeSet{Changes: []EvalChange{
			{Subject: subj, File: "topics/a.yaml", Path: "/partitions", Kind: "modify", Old: intNum(12), New: intNum(6)},
		}},
	}
	appr := &ApprovalContext{SourceSha: "sha-1", Evidence: map[string]*ApprovalEvidence{subj: validEvidence("sha-1")}}

	got, err := CoverWithApproval(pol, bind, in, appr)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	if got.Decision != DecisionBlock {
		t.Errorf("a co-occurring block must stand despite satisfied require-review, decision = %q", got.Decision)
	}
	sawBlock, sawReqReview := false, false
	for _, f := range got.Findings {
		if f.Effect == EffectBlock {
			sawBlock = true
		}
		if f.Effect == EffectRequireReview {
			sawReqReview = true
		}
	}
	if !sawBlock {
		t.Error("the block finding must stand")
	}
	if sawReqReview {
		t.Error("the satisfied require-review finding must be suppressed")
	}
}

// TestEvidenceDoesNotRescuePredicateError — implicit trap: a require-review that
// arises from a predicate ERROR (fail-safe, not a clean-false authored
// obligation) must NEVER be suppressed by evidence — evidence satisfies only the
// authored onFailure require-review, not an unevaluable predicate.
func TestEvidenceDoesNotRescuePredicateError(t *testing.T) {
	subj := "topic-registry:orders.events.v1"
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:  "topic-owner-must-approve",
				Phase: policy.PhaseEnforce,
				Match: policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
				// references an absent fact value -> evalLeaf errors -> predicate.error.
				Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "int(new) <= facts.quota.max.value"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "ownership-approval-missing"},
			}},
		},
	}
	bind, in := ownershipBinding(), oneSubjectInput(subj)
	appr := &ApprovalContext{SourceSha: "sha-1", Evidence: map[string]*ApprovalEvidence{subj: validEvidence("sha-1")}}

	got, err := CoverWithApproval(pol, bind, in, appr)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	if got.Decision == DecisionApprove {
		t.Fatal("a predicate error must never be rescued to APPROVE by evidence")
	}
	if len(got.Findings) != 1 || got.Findings[0].Code != "predicate.error" {
		t.Fatalf("want one standing predicate.error finding, got %+v", got.Findings)
	}
}

// assertRequireReviewStands asserts an unsatisfied require-review finding is
// present for subj and the decision never APPROVEs.
func assertRequireReviewStands(t *testing.T, got Result, subj string) {
	t.Helper()
	if got.Decision == DecisionApprove {
		t.Fatalf("an unsatisfied require-review must never APPROVE, got %q", got.Decision)
	}
	found := false
	for _, f := range got.Findings {
		if f.Effect == EffectRequireReview && f.Subject == subj {
			found = true
		}
	}
	if !found {
		t.Fatalf("want a standing require-review finding for %q, got %+v", subj, got.Findings)
	}
}
