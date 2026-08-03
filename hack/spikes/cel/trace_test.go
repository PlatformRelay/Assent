package celspike_test

import (
	"testing"

	celspike "github.com/PlatformRelay/assent/hack/spikes/cel"
	"github.com/google/cel-go/cel"
)

func TestTrace(t *testing.T) {
	env, err := celspike.NewEnv()
	if err != nil {
		t.Fatalf("NewEnv: %v", err)
	}

	t.Run("failing-vs-erroring-distinguishable", func(t *testing.T) {
		act := map[string]any{
			"old":   int64(12),
			"new":   "16",
			"facts": celspike.Facts{Quota: celspike.QuotaFacts{MaxPartitions: 24}},
		}
		failRoot := celspike.Leaf{ID: "bounds", CEL: "new >= old", Message: "non-decreasing"}
		failAct := map[string]any{
			"old":   int64(16),
			"new":   int64(12),
			"facts": celspike.Facts{Quota: celspike.QuotaFacts{MaxPartitions: 24}},
		}
		failRes := celspike.Walk(env, failRoot, failAct)
		if len(failRes.Traces) != 1 {
			t.Fatalf("traces=%d", len(failRes.Traces))
		}
		if failRes.Traces[0].State != celspike.TriFail {
			t.Fatalf("want fail, got %s err=%v", failRes.Traces[0].State, failRes.Traces[0].Err)
		}
		if failRes.Traces[0].ErrorClass != celspike.ClassNone {
			t.Fatalf("failing leaf must not carry error class, got %s", failRes.Traces[0].ErrorClass)
		}

		errRoot := celspike.Leaf{ID: "bad-type", CEL: "new >= old", Message: "type error"}
		errRes := celspike.Walk(env, errRoot, act)
		if len(errRes.Traces) != 1 {
			t.Fatalf("traces=%d", len(errRes.Traces))
		}
		if errRes.Traces[0].State != celspike.TriError {
			t.Fatalf("want error, got %s", errRes.Traces[0].State)
		}
		if errRes.Traces[0].ErrorClass != celspike.ClassTypeMismatch {
			t.Fatalf("want type_mismatch, got %s (%v)", errRes.Traces[0].ErrorClass, errRes.Traces[0].Err)
		}

		combo := celspike.All{Children: []celspike.Noder{
			celspike.Leaf{ID: "failing", CEL: "new >= old", Message: "fail"},
			// string→int conversion errors (dyn == would silently false — see report).
			celspike.Leaf{ID: "erroring", CEL: "int(entry.owner) > 0", Message: "type"},
		}}
		comboAct := map[string]any{
			"old":   int64(16),
			"new":   int64(12),
			"entry": map[string]any{"owner": "orders-team"},
			"facts": celspike.Facts{},
		}
		got := celspike.Walk(env, combo, comboAct)
		if len(got.Traces) < 2 {
			t.Fatalf("want >=2 traces, got %v", celspike.FormatTraceSummary(got.Traces))
		}
		var sawFail, sawErr bool
		for _, tr := range got.Traces {
			switch tr.State {
			case celspike.TriFail:
				sawFail = true
				if tr.ID != "failing" {
					t.Fatalf("fail leaf id=%s", tr.ID)
				}
			case celspike.TriError:
				sawErr = true
				if tr.ErrorClass == celspike.ClassNone {
					t.Fatalf("erroring leaf %s missing class: %v", tr.ID, tr.Err)
				}
			}
		}
		if !sawFail || !sawErr {
			t.Fatalf("need both fail and error in trace: %s", celspike.FormatTraceSummary(got.Traces))
		}
	})

	t.Run("error-classes-distinguishable", func(t *testing.T) {
		cases := []struct {
			name  string
			leaf  celspike.Leaf
			act   map[string]any
			class celspike.ErrorClass
			limit uint64 // 0 = default budget
		}{
			{
				name:  "unknown_field",
				leaf:  celspike.Leaf{ID: "uf", CEL: "facts.qota.max_partitions > 0"},
				act:   map[string]any{"old": int64(1), "new": int64(1), "facts": celspike.Facts{}},
				class: celspike.ClassUnknownField,
			},
			{
				name:  "type_mismatch",
				leaf:  celspike.Leaf{ID: "tm", CEL: "new >= old"},
				act:   map[string]any{"old": int64(1), "new": "x", "facts": celspike.Facts{}},
				class: celspike.ClassTypeMismatch,
			},
			{
				name: "missing_fact_key",
				leaf: celspike.Leaf{ID: "mf", CEL: "entry.missing_key == true"},
				act: map[string]any{
					"old":   int64(1),
					"new":   int64(1),
					"entry": map[string]any{},
					"facts": celspike.Facts{},
				},
				class: celspike.ClassMissingFact,
			},
			{
				name:  "cost_limit",
				leaf:  celspike.Leaf{ID: "cl", CEL: `[1,2,3,4,5,6,7,8,9,10].map(x, [1,2,3,4,5,6,7,8,9,10].map(y, x*y)).size() > 0`},
				act:   map[string]any{"old": int64(1), "new": int64(1), "facts": celspike.Facts{}},
				class: celspike.ClassCostLimit,
				limit: 5,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.limit > 0 {
					tr := evalWithLimit(t, env, tc.leaf, tc.act, tc.limit)
					if tr.State != celspike.TriError {
						t.Fatalf("state=%s err=%v", tr.State, tr.Err)
					}
					if tr.ErrorClass != tc.class {
						t.Fatalf("class=%s want %s (err=%v)", tr.ErrorClass, tc.class, tr.Err)
					}
					return
				}
				res := celspike.Walk(env, tc.leaf, tc.act)
				if len(res.Traces) != 1 {
					t.Fatalf("traces=%d", len(res.Traces))
				}
				tr := res.Traces[0]
				if tr.State != celspike.TriError {
					t.Fatalf("state=%s err=%v", tr.State, tr.Err)
				}
				if tr.ErrorClass != tc.class {
					t.Fatalf("class=%s want %s (err=%v)", tr.ErrorClass, tc.class, tr.Err)
				}
			})
		}
	})
}

func evalWithLimit(t *testing.T, env *cel.Env, leaf celspike.Leaf, act map[string]any, limit uint64) celspike.LeafTrace {
	t.Helper()
	return classifyViaEval(t, env, leaf, act, limit)
}

func classifyViaEval(t *testing.T, env *cel.Env, leaf celspike.Leaf, act map[string]any, limit uint64) celspike.LeafTrace {
	t.Helper()
	ast, iss := env.Compile(leaf.CEL)
	tr := celspike.LeafTrace{ID: leaf.ID}
	if iss.Err() != nil {
		tr.State = celspike.TriError
		tr.Err = iss.Err()
		tr.ErrorClass = celspike.ClassifyError(iss.Err())
		return tr
	}
	prg, err := env.Program(ast, cel.CostLimit(limit), cel.CostTracking(nil))
	if err != nil {
		t.Fatalf("program: %v", err)
	}
	_, _, err = prg.Eval(act)
	if err == nil {
		t.Fatal("expected cost limit error")
	}
	tr.State = celspike.TriError
	tr.Err = err
	tr.ErrorClass = celspike.ClassifyError(err)
	return tr
}
