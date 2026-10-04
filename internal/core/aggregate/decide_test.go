package aggregate_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// approvePolicyBinding is a minimal satisfiable policy: one enforce rule proving
// obligation "ok" cleanly true over the change built below.
func approvePolicyBinding() (*policy.MergePolicy, *policy.Binding) {
	mp := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "r",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"topics/**"}}},
				Prove:     &policy.Prove{Obligation: "ok", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: `new == "1"`}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "c"},
			}},
		},
	}
	return mp, &policy.Binding{Require: []string{"ok"}}
}

func oneChange() *aggregate.EvaluationInput {
	return &aggregate.EvaluationInput{
		ChangeSet: aggregate.ChangeSet{Changes: []aggregate.EvalChange{{
			Subject: "topics/orders.yaml", File: "topics/orders.yaml", Path: "/partitions", Kind: "modify", Old: "1", New: "1",
		}}},
		Facts:   map[string]map[string]aggregate.Fact{},
		Require: []string{"ok"},
	}
}

// TestDecideEmptyRequireRefusesVacuity — REQ-XREV-S01-01: a binding with an empty
// require[] must ERROR, never APPROVE (the obligation layer would be vacuous).
func TestDecideEmptyRequireRefusesVacuity(t *testing.T) {
	mp, _ := approvePolicyBinding()
	bind := &policy.Binding{Require: nil}
	_, err := aggregate.Decide(aggregate.DecideRequest{
		Subject: "file:topics/orders.yaml",
		Policy:  mp,
		Binding: bind,
		Input:   oneChange(),
	})
	if err == nil {
		t.Fatal("Decide with an empty require[] returned nil error — a vacuous APPROVE was armed")
	}
	// A nil binding has no require[] either: same refusal, and no nil dereference.
	if _, err := aggregate.Decide(aggregate.DecideRequest{
		Subject: "file:topics/orders.yaml",
		Policy:  mp,
		Input:   oneChange(),
	}); err == nil {
		t.Fatal("Decide with a nil binding returned nil error — a vacuous APPROVE was armed")
	}
}

// TestDecideOpaqueOrEmptyReviews — REQ-XREV-S01-02: an opaque or empty changeset
// must REVIEW, carrying the aggregate.changeset finding.
func TestDecideOpaqueOrEmptyReviews(t *testing.T) {
	mp, bind := approvePolicyBinding()
	for _, tc := range []struct {
		name string
		req  aggregate.DecideRequest
	}{
		{"opaque", aggregate.DecideRequest{Subject: "file:x", Opaque: true, Policy: mp, Binding: bind, Input: oneChange()}},
		{"empty", aggregate.DecideRequest{Subject: "file:x", Policy: mp, Binding: bind, Input: &aggregate.EvaluationInput{ChangeSet: aggregate.ChangeSet{}, Facts: map[string]map[string]aggregate.Fact{}, Require: []string{"ok"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := aggregate.Decide(tc.req)
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			if res.Decision != aggregate.DecisionReview {
				t.Fatalf("decision = %s, want REVIEW", res.Decision)
			}
			if len(res.Findings) != 1 || res.Findings[0].Rule != "aggregate.changeset" || res.Findings[0].Subject != "file:x" {
				t.Fatalf("findings = %#v, want one aggregate.changeset finding with subject file:x", res.Findings)
			}
		})
	}
}

// TestDecideReservedClassBlocks — REQ-XREV-S01-03: the reserved class dominates to
// BLOCK before any predicate, even a satisfiable one.
func TestDecideReservedClassBlocks(t *testing.T) {
	mp, bind := approvePolicyBinding()
	res, err := aggregate.Decide(aggregate.DecideRequest{
		Subject:      "file:.assent/config.yml",
		SubjectClass: aggregate.ReservedPolicyClass,
		Policy:       mp,
		Binding:      bind,
		Input:        oneChange(),
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if res.Decision != aggregate.DecisionBlock {
		t.Fatalf("decision = %s, want BLOCK", res.Decision)
	}
}

// TestDecideSatisfiableApproves is the control: the SAME policy/change without the
// reserved class APPROVEs, so the BLOCK above is the guard's doing, not a policy
// that blocks everything.
func TestDecideSatisfiableApproves(t *testing.T) {
	mp, bind := approvePolicyBinding()
	res, err := aggregate.Decide(aggregate.DecideRequest{
		Subject: "file:topics/orders.yaml",
		Policy:  mp,
		Binding: bind,
		Input:   oneChange(),
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if res.Decision != aggregate.DecisionApprove {
		t.Fatalf("decision = %s, want APPROVE (findings %#v)", res.Decision, res.Findings)
	}
}
