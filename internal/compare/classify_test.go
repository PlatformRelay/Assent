package compare

import (
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// REQ-PCS-S02-01: baseline BLOCK intervention on an identity, candidate lacks that
// finding and does not reach APPROVE -> destructive-or-authorization-intervention-missed.
func TestClassifyMissedIntervention(t *testing.T) {
	subject := "topic-registry:orders.events.v1"
	baseline := aggregate.Result{
		Decision: aggregate.DecisionBlock,
		Findings: []aggregate.Finding{{
			Rule:       "non-destructive",
			Obligation: "non-destructive",
			Effect:     aggregate.EffectBlock,
			Subject:    subject,
			Code:       "partitions.shrunk",
		}},
	}
	candidate := aggregate.Result{
		Decision: aggregate.DecisionReview,
		Findings: []aggregate.Finding{{
			Rule:       "aggregate.uncovered",
			Obligation: "non-destructive",
			Effect:     aggregate.EffectRequireReview,
			Code:       "obligation.uncovered",
		}},
	}

	got, err := classify(baseline, candidate)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got != KindDestructiveOrAuthorizationInterventionMissed {
		t.Fatalf("kind = %q, want %q", got, KindDestructiveOrAuthorizationInterventionMissed)
	}
}

// REQ-PCS-S02-02: baseline APPROVE with no intervention on an identity, candidate
// adds BLOCK/require-review/challenge on that identity -> stricter-intervention-added.
func TestClassifyStricterInterventionAdded(t *testing.T) {
	subject := "topic-registry:orders.events.v1"
	baseline := aggregate.Result{
		Decision: aggregate.DecisionApprove,
		Findings: nil,
	}
	candidate := aggregate.Result{
		Decision: aggregate.DecisionBlock,
		Findings: []aggregate.Finding{{
			Rule:       "non-destructive",
			Obligation: "non-destructive",
			Effect:     aggregate.EffectBlock,
			Subject:    subject,
			Code:       "partitions.shrunk",
		}},
	}

	got, err := classify(baseline, candidate)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got != KindStricterInterventionAdded {
		t.Fatalf("kind = %q, want %q", got, KindStricterInterventionAdded)
	}
}

// Integration: removing a baseline block rule so the candidate reaches REVIEW without
// the baseline's intervention finding classifies as missed (not fail-closed).
func TestCompareMissedInterventionViaProfiles(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod-strict@6", "new >= old", "", policy.EffectBlock)
	candidate := Profile{
		Name:    "prod-permissive@7",
		Policy:  &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: nil}},
		Bind:    baseline.Bind,
		Ceiling: policy.PhaseEnforce,
	}

	got, err := Compare(in, baseline, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Baseline != aggregate.DecisionBlock {
		t.Fatalf("baseline decision = %q, want BLOCK", got.Baseline)
	}
	if got.Candidate != aggregate.DecisionReview {
		t.Fatalf("candidate decision = %q, want REVIEW", got.Candidate)
	}
	if got.Kind != KindDestructiveOrAuthorizationInterventionMissed {
		t.Fatalf("kind = %q, want %q", got, KindDestructiveOrAuthorizationInterventionMissed)
	}
	if got.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, want PASS (seed gate only trips on widening)", got.Verdict)
	}
}

// Integration: candidate adds a block intervention the baseline APPROVE side lacked.
func TestCompareStricterInterventionAddedViaProfiles(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod-permissive@6", "true", "", policy.EffectBlock)
	candidate := mkProfile("prod-strict@7", "new >= old", "", policy.EffectBlock)

	got, err := Compare(in, baseline, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Baseline != aggregate.DecisionApprove {
		t.Fatalf("baseline decision = %q, want APPROVE", got.Baseline)
	}
	if got.Candidate != aggregate.DecisionBlock {
		t.Fatalf("candidate decision = %q, want BLOCK", got.Candidate)
	}
	if got.Kind != KindStricterInterventionAdded {
		t.Fatalf("kind = %q, want %q", got, KindStricterInterventionAdded)
	}
	if got.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, want PASS (seed gate only trips on widening)", got.Verdict)
	}
}

// BLOCK -> REVIEW with a weakened effect on the same identity stays fail-closed.
func TestClassifyBlockToReviewWeakenedEffectUnclassifiable(t *testing.T) {
	subject := "topic-registry:orders.events.v1"
	baseline := aggregate.Result{
		Decision: aggregate.DecisionBlock,
		Findings: []aggregate.Finding{{
			Rule:       "non-destructive",
			Obligation: "non-destructive",
			Effect:     aggregate.EffectBlock,
			Subject:    subject,
			Code:       "partitions.shrunk",
		}},
	}
	candidate := aggregate.Result{
		Decision: aggregate.DecisionReview,
		Findings: []aggregate.Finding{{
			Rule:       "non-destructive",
			Obligation: "non-destructive",
			Effect:     aggregate.EffectRequireReview,
			Subject:    subject,
			Code:       "partitions.shrunk",
		}},
	}
	if _, err := classify(baseline, candidate); !errors.Is(err, ErrUnclassifiable) {
		t.Fatalf("weakened intervention on same identity must fail closed, got %v", err)
	}
}
