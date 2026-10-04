package aggregate

import (
	"strings"
	"testing"
)

// costBombWhen is a nested comprehension whose evaluation cost exceeds celCostBudget
// (ADR-0013). Without cel.CostLimit on the leaf evaluator, it completes as true; with
// the limit, evaluation errors and the caller fails safe to REVIEW.
// n=39 triple-nested comprehension exceeds celCostBudget; n=38 stays under (probe in REL-02).
const costBombWhen = `[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39].map(x, [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39].map(y, [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39].map(z, x*y*z))).size() > 0`

// TestEvalLeafEnforcesCostBudget — REL-02 / AUD-02: the live leaf evaluator
// (evalLeaf, the one seam the coverage loop and every rule path uses) applies
// celCostBudget, so an over-budget when-expression errors and fails safe to
// REVIEW rather than completing.
//
// It doubles as the D-131 interaction proof: costBombWhen's expensive node is the
// LEFT OPERAND of `> 0`, so it is exactly the kind of node textOrderGuard wraps
// (cel.CustomDecorator). This asserts the cost observer still charges a wrapped
// operand — keep the `> 0` shape if this expression is ever rewritten.
func TestEvalLeafEnforcesCostBudget(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("newEvalEnv: %v", err)
	}
	in := EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{{
		Subject: "s:1", File: "f.yaml", Path: "/x", Kind: "modify", Old: "1", New: "1",
	}}}, Facts: map[string]map[string]Fact{}}

	_, err = evalLeaf(env, in, in.ChangeSet.Changes[0], "prod", costBombWhen)
	if err == nil {
		t.Fatal("evalLeaf must error when the when-expression exceeds celCostBudget")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "cost") {
		t.Fatalf("want cost-limit error, got: %v", err)
	}
}
