package aggregate

import (
	"encoding/json"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// points_test.go is the E2-S06 acceptance suite: author-declared rule.points
// accrue PER FIRING (per matched-and-failed change, ADR-0007 Amendment 2 — the
// bulk-change / salami-slice guard) and the binding risk.threshold gates APPROVE
// as aggregation order #4 (the LAST check, after block #1 / challenge #2 /
// uncovered-or-unproven obligation #3).
//
// comment is the SOFT points channel: a comment firing is DECISION-NEUTRAL
// (ADR-0007 "Blocks merge? no"); it accrues points and is escalated to REVIEW
// ONLY when the summed points exceed the threshold. block/challenge/
// require-review remain decision-lowering (via effectDecision).

// bumpChange builds one modify change on the shared subject "s:1" whose numeric
// value SHRANK (new < old), so a `new >= old` leaf evaluates cleanly FALSE and
// the rule FIRES for it. intNum mirrors LoadEvaluationInput's json.Number typing
// so the CEL compare is numeric, not lexical.
func bumpChange(pointer string) EvalChange {
	return EvalChange{Subject: "s:1", File: "topics/prod/a.yaml", Path: pointer, Kind: "modify", Old: intNum(2), New: intNum(1)}
}

// softBumpRule proves a NON-required obligation ("signal") with a `comment`
// onFailure and an author-declared points weight, matching every /pN pointer. It
// is the soft points channel: each shrunk /pN change is one firing.
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

// fiveBumps returns five shrunk changes on the ONE subject "s:1" — five firings
// of softBumpRule, collapsing to exactly ONE comment finding.
func fiveBumps() []EvalChange {
	return []EvalChange{
		bumpChange("/p0"), bumpChange("/p1"), bumpChange("/p2"), bumpChange("/p3"), bumpChange("/p4"),
	}
}

// TestAuthoredRulePointsReproduceGolden — REQ-E2-S06-01. The D-016 golden's
// points are reproduced from AUTHORED rule.points (lane F added points:10 to
// partitions-must-not-shrink; topic-owner-must-approve authors none). There is NO
// engine effect->points default: a rule authoring no points never acquires a
// nonzero weight from its effect alone.
func TestAuthoredRulePointsReproduceGolden(t *testing.T) {
	mp, bind, in := loadD016(t)
	got, err := Cover(mp, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if got.Decision != DecisionBlock {
		t.Errorf("decision = %q, want BLOCK", got.Decision)
	}

	// Authored points, keyed by the finding code the golden emits.
	wantPoints := map[string]int{
		"partition-count-shrunk":     10, // block rule authored points:10
		"ownership-approval-missing": 0,  // require-review rule authored none
	}
	sawBlock := false
	for _, f := range got.Findings {
		want, ok := wantPoints[f.Code]
		if !ok {
			t.Errorf("unexpected finding code %q", f.Code)
			continue
		}
		if f.Points != want {
			t.Errorf("finding %q points = %d, want authored %d", f.Code, f.Points, want)
		}
		if f.Code == "partition-count-shrunk" {
			sawBlock = true
			// Adversarial: the block finding's weight is the AUTHORED 10, not an
			// effect-derived default — `block` has no inherent weight.
			if f.Effect != EffectBlock {
				t.Errorf("expected block effect on %q, got %q", f.Code, f.Effect)
			}
		}
		// Adversarial: the require-review rule authored NO points -> stays 0; its
		// require-review effect must never manufacture a nonzero weight.
		if f.Code == "ownership-approval-missing" && f.Points != 0 {
			t.Errorf("no-points rule acquired weight %d from its effect", f.Points)
		}
	}
	if !sawBlock {
		t.Fatal("golden block finding (partition-count-shrunk) not emitted")
	}
}

// TestPointsAccruePerFiring — REQ-E2-S06-02, THE DISCRIMINATING TEST. ONE
// subject, ONE points:2 rule matching K=5 shrunk changes -> 5 firings, exactly 1
// comment finding, sum = 5*2 = 10. The threshold boundary is pinned at exactly
// 10: threshold 9 -> REVIEW, threshold 10 -> APPROVE. That boundary PROVES the
// sum is 5*2=10 (per firing), NOT 1*2=2 (per finding) — under a per-finding model
// sum=2 would APPROVE at threshold 9, contradicting the REVIEW assertion. (This
// is why firings(5) > findings(1): a ten-distinct-subjects design would give
// findings==firings and prove nothing.)
func TestPointsAccruePerFiring(t *testing.T) {
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{softBumpRule(2)}}}
	changes := fiveBumps()

	// The single collapsed finding: 5 firings on one subject -> 1 finding carrying
	// the AUTHORED per-firing weight 2 (NOT 10 — the finding-vs-sum asymmetry).
	// threshold just above the sum so the finding count is asserted without the
	// threshold masking it.
	bindHi := &policy.Binding{Require: nil, Environment: "prod", Risk: policy.Risk{Threshold: 10}}
	inHi := &EvaluationInput{ChangeSet: ChangeSet{Changes: changes}}
	got, err := Cover(pol, bindHi, inHi)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if len(got.Findings) != 1 {
		t.Fatalf("5 firings on one subject must collapse to exactly 1 finding, got %d: %+v", len(got.Findings), got.Findings)
	}
	if got.Findings[0].Points != 2 {
		t.Errorf("finding carries authored per-firing weight, points = %d, want 2", got.Findings[0].Points)
	}
	// sum = 5*2 = 10 <= threshold 10 -> APPROVE (comment is decision-neutral).
	if got.Decision != DecisionApprove {
		t.Errorf("sum(10) <= threshold(10) with only a comment firing must APPROVE, got %q", got.Decision)
	}

	// Boundary just below the per-firing sum: threshold 9 < 10 -> REVIEW. If points
	// were counted per-finding (sum=2), 2 <= 9 would APPROVE — so REVIEW here proves
	// the per-firing 5*2 accrual.
	bindLo := &policy.Binding{Require: nil, Environment: "prod", Risk: policy.Risk{Threshold: 9}}
	inLo := &EvaluationInput{ChangeSet: ChangeSet{Changes: changes}}
	rev, err := Cover(pol, bindLo, inLo)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if len(rev.Findings) != 1 {
		t.Fatalf("still exactly 1 finding, got %d", len(rev.Findings))
	}
	if rev.Decision != DecisionReview {
		t.Errorf("sum(5*2=10) > threshold(9) must REVIEW (proving per-firing, not per-finding sum=2), got %q", rev.Decision)
	}
}

// TestRiskThresholdGatesApprove — REQ-E2-S06-03. Covered obligations (a satisfied
// required `ownership`), no block, no challenge, and comment points firings: the
// aggregation-order #4 risk check gates the decision. sum <= threshold -> APPROVE;
// same inputs, sum > threshold -> REVIEW.
func TestRiskThresholdGatesApprove(t *testing.T) {
	// A satisfied required obligation (new >= old holds) proves coverage; it emits
	// no finding and does not lower APPROVE.
	ownerOK := policy.Rule{
		Name:      "owner-ok",
		Phase:     policy.PhaseEnforce,
		Match:     policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"/owner"}}},
		Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "owner-missing"},
	}
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{ownerOK, softBumpRule(2)}}}
	// /owner grows (satisfied), /p0../p4 shrink (5 comment firings, sum = 10).
	changes := append([]EvalChange{
		{Subject: "s:1", File: "topics/prod/a.yaml", Path: "/owner", Kind: "modify", Old: intNum(1), New: intNum(2)},
	}, fiveBumps()...)
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: changes}}

	t.Run("sum <= threshold -> APPROVE", func(t *testing.T) {
		bind := &policy.Binding{Require: []string{"ownership"}, Environment: "prod", Risk: policy.Risk{Threshold: 10}}
		got, err := Cover(pol, bind, in)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionApprove {
			t.Errorf("covered + no block/challenge + sum(10) <= threshold(10) -> APPROVE, got %q: %+v", got.Decision, got.Findings)
		}
	})

	t.Run("sum > threshold -> REVIEW", func(t *testing.T) {
		bind := &policy.Binding{Require: []string{"ownership"}, Environment: "prod", Risk: policy.Risk{Threshold: 4}}
		got, err := Cover(pol, bind, in)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionReview {
			t.Errorf("same inputs but sum(10) > threshold(4) -> REVIEW, got %q", got.Decision)
		}
	})
}

// TestRequiredObligationCommentFiringNeverApproves — fail-safe guard on the
// comment-neutral soft channel (REQ-E2-S06-03 adversarial; ADR-0017 §2 supersedes
// ADR-0007's coverage line). comment is decision-neutral ONLY for a NON-required
// signal obligation. A REQUIRED obligation proved by a comment-onFailure rule that
// FIRES is unproven -> REVIEW, never APPROVE — even when the points sum is well
// under threshold. Without the required-obligation guard, comment-neutrality would
// fail OPEN, approving an unproven required obligation (the exact §2 forbidden
// outcome; "never under-count to a wrong APPROVE").
func TestRequiredObligationCommentFiringNeverApproves(t *testing.T) {
	rule := policy.Rule{
		Name:      "ownership-soft",
		Phase:     policy.PhaseEnforce,
		Points:    1,
		Match:     policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"/p0"}}},
		Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectComment, Code: "owner-soft"},
	}
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{rule}}}
	// ownership is REQUIRED; the single change shrinks -> the comment rule FIRES,
	// leaving the required obligation unproven. Threshold 100 >> sum(1), so the
	// threshold alone would APPROVE — only the §2 required-obligation rule forces
	// REVIEW.
	bind := &policy.Binding{Require: []string{"ownership"}, Environment: "prod", Risk: policy.Risk{Threshold: 100}}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{bumpChange("/p0")}}}
	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if got.Decision == DecisionApprove {
		t.Fatalf("an unproven REQUIRED obligation must never APPROVE (comment neutral only for non-required signals), got %q: %+v", got.Decision, got.Findings)
	}
	if got.Decision != DecisionReview {
		t.Errorf("decision = %q, want REVIEW", got.Decision)
	}
}

// TestBlockDominatesPointsTotal — REQ-E2-S06-04. A block firing dominates (order
// #1) regardless of the points sum (order #4): points never rescue a block (when
// the sum is under threshold) and the threshold's REVIEW never displaces a BLOCK.
func TestBlockDominatesPointsTotal(t *testing.T) {
	blockRule := policy.Rule{
		Name:      "must-not-shrink",
		Phase:     policy.PhaseEnforce,
		Points:    5,
		Match:     policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"/parts"}}},
		Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "parts-shrunk"},
	}
	// A block firing plus five comment firings (sum from comments = 10, +5 block = 15).
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{blockRule, softBumpRule(2)}}}
	changes := append([]EvalChange{
		{Subject: "s:1", File: "topics/prod/a.yaml", Path: "/parts", Kind: "modify", Old: intNum(6), New: intNum(3)},
	}, fiveBumps()...)
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: changes}}

	for _, tc := range []struct {
		name      string
		threshold int
	}{
		{"points under threshold cannot rescue a block", 100},
		{"points over threshold cannot worsen past block", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod", Risk: policy.Risk{Threshold: tc.threshold}}
			got, err := Cover(pol, bind, in)
			if err != nil {
				t.Fatalf("Cover: %v", err)
			}
			if got.Decision != DecisionBlock {
				t.Errorf("block must dominate the points total, got %q", got.Decision)
			}
		})
	}
}

// TestPointsDoubleRunStable — REQ-E2-S06-05. The points sum is order-independent
// over the firing set, and a golden double-run is byte-identical. (TestCorePurity
// — no clock/rand in internal/core — lives in purity_test.go and stays green:
// this story adds only pure integer arithmetic.)
func TestPointsDoubleRunStable(t *testing.T) {
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{softBumpRule(2)}}}
	bind := &policy.Binding{Require: nil, Environment: "prod", Risk: policy.Risk{Threshold: 9}}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: fiveBumps()}}

	base, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover base: %v", err)
	}
	baseJSON, _ := json.Marshal(base)
	if base.Decision != DecisionReview {
		t.Fatalf("expected REVIEW (sum 10 > threshold 9), got %q", base.Decision)
	}

	// Double-run: same input twice is byte-identical.
	again, _ := Cover(pol, bind, in)
	againJSON, _ := json.Marshal(again)
	if string(baseJSON) != string(againJSON) {
		t.Errorf("points not double-run stable:\n a=%s\n b=%s", baseJSON, againJSON)
	}

	// Order-independent: reversing the firing set yields the same sum + decision.
	shIn := &EvaluationInput{ChangeSet: ChangeSet{Changes: reversedChanges(fiveBumps())}}
	shuf, _ := Cover(pol, bind, shIn)
	shufJSON, _ := json.Marshal(shuf)
	if string(baseJSON) != string(shufJSON) {
		t.Errorf("points sum not order-independent:\n base=%s\n shuf=%s", baseJSON, shufJSON)
	}
}
