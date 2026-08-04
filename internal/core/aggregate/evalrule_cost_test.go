package aggregate

import (
	"strings"
	"testing"
)

// costBombWhen is a nested comprehension whose evaluation cost exceeds celCostBudget
// (ADR-0013). Without cel.CostLimit on evalRule, it completes as true; with the
// limit, evaluation errors and the caller fails safe to REVIEW.
// n=39 triple-nested comprehension exceeds celCostBudget; n=38 stays under (probe in REL-02).
const costBombWhen = `[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39].map(x, [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39].map(y, [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39].map(z, x*y*z))).size() > 0`

// TestEvalRuleEnforcesCostBudget — REL-02 / AUD-02: production evalRule must apply
// the same celCostBudget as evalLeaf/evalscalar/message-template paths.
func TestEvalRuleEnforcesCostBudget(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("newEvalEnv: %v", err)
	}
	act := map[string]any{"old": int64(1), "new": int64(1)}

	_, err = evalRule(env, act, costBombWhen)
	if err == nil {
		t.Fatal("evalRule must error when when-expression exceeds celCostBudget")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "cost") {
		t.Fatalf("want cost-limit error, got: %v", err)
	}
}
