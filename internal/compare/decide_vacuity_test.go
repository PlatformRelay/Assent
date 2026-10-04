package compare_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/compare"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// TestCompareEmptyRequireFailsClosed — REQ-XREV-S01-01 / D2. `compare` was the
// live engine consumer with NO trust-boundary guard: a profile binding with an
// empty require[] produced a vacuous APPROVE-based comparison. Routing it through
// aggregate.Decide makes the empty-require case a hard error, never a silent pass.
// Reverting the empty-require branch in Decide reddens this test.
func TestCompareEmptyRequireFailsClosed(t *testing.T) {
	mp := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "r",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"topics/**"}}},
				Prove:     &policy.Prove{Obligation: "ok", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "true"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "c"},
			}},
		},
	}
	bind := &policy.Binding{Require: nil} // the vacuity: no required obligations
	in := &aggregate.EvaluationInput{
		ChangeSet: aggregate.ChangeSet{Changes: []aggregate.EvalChange{{
			Subject: "topics/orders.yaml", File: "topics/orders.yaml", Path: "/partitions", Kind: "modify", Old: "1", New: "2",
		}}},
		Facts: map[string]map[string]aggregate.Fact{},
	}

	_, err := compare.Compare(in,
		compare.Profile{Name: "base", Policy: mp, Bind: bind},
		compare.Profile{Name: "cand", Policy: mp, Bind: bind},
	)
	if err == nil {
		t.Fatal("compare with an empty require[] returned nil error — the vacuous APPROVE was not closed")
	}
}
