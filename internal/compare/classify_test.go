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

// obligationBundleJSON is a ReplayBundle whose change grows partitions (6→12),
// satisfying a non-destructive guard when present. Used for obligation-coverage
// classifier goldens.
const obligationBundleJSON = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "ReplayBundle",
  "pins": {
    "toolVersion": "0.0.0-test",
    "toolDigest": "sha256:aaaa",
    "policySha": "sha256:bbbb",
    "sourceSha": "cccc",
    "targetSha": "dddd",
    "mergeResultDigest": "sha256:eeee",
    "factsResolvedAt": {}
  },
  "evaluationInput": {
    "apiVersion": "assent.dev/v1alpha1",
    "kind": "EvaluationInput",
    "changeSet": {
      "changes": [
        {
          "subject": "topic-registry:orders.events.v1",
          "file": "topics/prod/orders.events.v1.yaml",
          "path": "/partitions",
          "kind": "modify",
          "old": 6,
          "new": 12
        }
      ]
    },
    "facts": {},
    "mr": {"author": "alice", "sourceBranch": "topic/grow", "targetBranch": "main"},
    "require": ["ownership", "non-destructive"]
  }
}`

// scoreBundleJSON carries five soft-bump firings (sum=10) on one subject for
// threshold arithmetic classifier goldens.
const scoreBundleJSON = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "ReplayBundle",
  "pins": {
    "toolVersion": "0.0.0-test",
    "toolDigest": "sha256:aaaa",
    "policySha": "sha256:bbbb",
    "sourceSha": "cccc",
    "targetSha": "dddd",
    "mergeResultDigest": "sha256:eeee",
    "factsResolvedAt": {}
  },
  "evaluationInput": {
    "apiVersion": "assent.dev/v1alpha1",
    "kind": "EvaluationInput",
    "changeSet": {
      "changes": [
        {"subject": "topic-registry:orders.events.v1", "file": "topics/prod/a.yaml", "path": "/p0", "kind": "modify", "old": 2, "new": 1},
        {"subject": "topic-registry:orders.events.v1", "file": "topics/prod/a.yaml", "path": "/p1", "kind": "modify", "old": 2, "new": 1},
        {"subject": "topic-registry:orders.events.v1", "file": "topics/prod/a.yaml", "path": "/p2", "kind": "modify", "old": 2, "new": 1},
        {"subject": "topic-registry:orders.events.v1", "file": "topics/prod/a.yaml", "path": "/p3", "kind": "modify", "old": 2, "new": 1},
        {"subject": "topic-registry:orders.events.v1", "file": "topics/prod/a.yaml", "path": "/p4", "kind": "modify", "old": 2, "new": 1}
      ]
    },
    "facts": {},
    "mr": {"author": "alice", "sourceBranch": "topic/bump", "targetBranch": "main"},
    "require": ["ownership"]
  }
}`

func ownershipRule() policy.Rule {
	return policy.Rule{
		Name:      "owner-ok",
		Phase:     policy.PhaseEnforce,
		Match:     policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"/owner"}}},
		Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "true"}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "owner-missing"},
	}
}

func nonDestructiveRule() policy.Rule {
	return policy.Rule{
		Name:      "non-destructive",
		Phase:     policy.PhaseEnforce,
		Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
		Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "partitions.shrunk"},
	}
}

func softBumpRule(points int) policy.Rule {
	return policy.Rule{
		Name:      "soft-bump",
		Phase:     policy.PhaseEnforce,
		Points:    points,
		Match:     policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"/p0", "/p1", "/p2", "/p3", "/p4"}}},
		Prove:     &policy.Prove{Obligation: "signal", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectComment, Code: "value-bumped"},
	}
}

func mkObligationProfile(name string, rules []policy.Rule) Profile {
	return Profile{
		Name: name,
		Policy: &policy.MergePolicy{
			Spec: policy.MergePolicySpec{Rules: rules},
		},
		Bind: &policy.Binding{
			Require:     []string{"ownership", "non-destructive"},
			Environment: "prod",
			Class:       "kafka-topic",
		},
		Ceiling: policy.PhaseEnforce,
	}
}

func mkScoreProfile(name string, threshold int) Profile {
	return mkScoreProfileWithPoints(name, threshold, 2)
}

func mkScoreProfileWithPoints(name string, threshold, points int) Profile {
	return Profile{
		Name: name,
		Policy: &policy.MergePolicy{
			Spec: policy.MergePolicySpec{Rules: []policy.Rule{ownershipRule(), softBumpRule(points)}},
		},
		Bind: &policy.Binding{
			Require:     []string{"ownership"},
			Environment: "prod",
			Class:       "kafka-topic",
			Risk:        policy.Risk{Threshold: threshold},
		},
		Ceiling: policy.PhaseEnforce,
	}
}

func loadBundleJSON(t *testing.T, raw string) *aggregate.EvaluationInput {
	t.Helper()
	in, err := LoadBundle([]byte(raw))
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	return in
}

// REQ-PCS-S03-01: baseline proves obligation O on subject S; candidate omits O
// from coverage while decision degrades -> subject-or-obligation-uncovered.
func TestClassifyObligationUncovered(t *testing.T) {
	in := loadBundleJSON(t, obligationBundleJSON)
	baseline := mkObligationProfile("prod@6", []policy.Rule{ownershipRule(), nonDestructiveRule()})
	candidate := mkObligationProfile("prod@7", []policy.Rule{nonDestructiveRule()})

	got, err := Compare(in, baseline, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Baseline != aggregate.DecisionApprove {
		t.Fatalf("baseline decision = %q, want APPROVE (both obligations proven)", got.Baseline)
	}
	if got.Candidate != aggregate.DecisionReview {
		t.Fatalf("candidate decision = %q, want REVIEW (ownership uncovered)", got.Candidate)
	}
	if got.Kind != KindSubjectOrObligationUncovered {
		t.Fatalf("kind = %q, want %q", got.Kind, KindSubjectOrObligationUncovered)
	}
	if got.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, want PASS (seed gate ignores this kind)", got.Verdict)
	}
}

// REQ-PCS-S03-02: policies identical except threshold/points flip REVIEW→APPROVE
// with no new/removed finding identities -> score-threshold-change.
func TestClassifyScoreThresholdChange(t *testing.T) {
	in := loadBundleJSON(t, scoreBundleJSON)
	baseline := mkScoreProfile("prod@6", 4)
	candidate := mkScoreProfile("prod@7", 10)

	got, err := Compare(in, baseline, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Baseline != aggregate.DecisionReview {
		t.Fatalf("baseline decision = %q, want REVIEW (sum 10 > threshold 4)", got.Baseline)
	}
	if got.Candidate != aggregate.DecisionApprove {
		t.Fatalf("candidate decision = %q, want APPROVE (sum 10 <= threshold 10)", got.Candidate)
	}
	if got.Kind != KindScoreThresholdChange {
		t.Fatalf("kind = %q, want %q", got.Kind, KindScoreThresholdChange)
	}
	if got.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, want PASS (seed gate ignores this kind)", got.Verdict)
	}
}

// REQ-PCS-S03-02 (points-only): rule.points change alone (2→1) with an unchanged
// binding threshold flips REVIEW→APPROVE -> score-threshold-change, not widening.
func TestClassifyScoreThresholdChangePointsOnly(t *testing.T) {
	in := loadBundleJSON(t, scoreBundleJSON)
	baseline := mkScoreProfileWithPoints("prod@6", 5, 2)
	candidate := mkScoreProfileWithPoints("prod@7", 5, 1)

	got, err := Compare(in, baseline, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Baseline != aggregate.DecisionReview {
		t.Fatalf("baseline decision = %q, want REVIEW (sum 10 > threshold 5)", got.Baseline)
	}
	if got.Candidate != aggregate.DecisionApprove {
		t.Fatalf("candidate decision = %q, want APPROVE (sum 5 <= threshold 5)", got.Candidate)
	}
	if got.Kind != KindScoreThresholdChange {
		t.Fatalf("kind = %q, want %q (points-only arithmetic, not widening)", got.Kind, KindScoreThresholdChange)
	}
	if got.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, want PASS (score-threshold never trips the seed gate)", got.Verdict)
	}
}

// REQ-PCS-S03-03: a decision change matching none of the six kinds is fail-closed.
func TestClassifyFailClosed(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod@6", "new >= old", "", policy.EffectBlock)
	candidate := mkProfile("prod@7", "new >= old", "", policy.EffectRequireReview)

	if _, err := classify(evaluateMust(t, in, baseline), evaluateMust(t, in, candidate)); !errors.Is(err, ErrUnclassifiable) {
		t.Fatalf("BLOCK->REVIEW effect downgrade must fail closed, got %v", err)
	}
}

func evaluateMust(t *testing.T, in *aggregate.EvaluationInput, p Profile) aggregate.Result {
	t.Helper()
	res, err := evaluate(in, p)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return res
}
