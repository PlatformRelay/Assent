package aggregate

import (
	"encoding/json"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// REQ-E2-S03-01 — an `all` whose first leaf holds and second fails resolves
// false and the finding carries the SECOND leaf's message (per-leaf attribution).
func TestAllShortCircuitsToFailingLeafMessage(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:  "quota-guard",
				Phase: policy.PhaseEnforce,
				Match: policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
				Prove: &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{All: []policy.AssertTree{
					{Leaf: &policy.Leaf{CEL: "new >= old", Message: "partitions may not decrease"}},
					{Leaf: &policy.Leaf{CEL: "new <= facts.quota.max_partitions.value", Message: "over quota"}},
				}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "partition-over-quota"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}
	in := &EvaluationInput{
		ChangeSet: ChangeSet{Changes: []EvalChange{
			// old=10,new=12: first leaf holds (12>=10); second fails (12<=8 false).
			{Subject: "s:1", File: "a.yaml", Path: "/partitions", Kind: "modify", Old: intNum(10), New: intNum(12)},
		}},
		Facts: map[string]map[string]Fact{"quota": {"max_partitions": {State: "resolved", Value: intNum(8)}}},
	}

	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if got.Decision != DecisionBlock {
		t.Errorf("decision = %q, want BLOCK (second leaf fails)", got.Decision)
	}
	if len(got.Findings) != 1 {
		t.Fatalf("want one finding, got %+v", got.Findings)
	}
	if got.Findings[0].Message != "over quota" {
		t.Errorf("finding.Message = %q, want the SECOND (failing) leaf's message %q", got.Findings[0].Message, "over quota")
	}
	if got.Findings[0].Effect != EffectBlock {
		t.Errorf("finding.Effect = %q, want block (clean-false onFailure)", got.Findings[0].Effect)
	}
}

// REQ-E2-S03-02 — `any` with one true + one false leaf is satisfied; `not` over
// a true leaf is false (OR/negation semantics).
func TestAnyAndNotSemantics(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	in := EvaluationInput{}
	ch := EvalChange{File: "f", Path: "/p", Kind: "modify", Old: intNum(5), New: intNum(10)}

	// any: [new > old (true), new < old (false)] -> satisfied.
	anyTree := policy.AssertTree{Any: []policy.AssertTree{
		{Leaf: &policy.Leaf{CEL: "new > old"}},
		{Leaf: &policy.Leaf{CEL: "new < old"}},
	}}
	sat, _, werr := walkAssertTree(env, in, ch, "prod", anyTree)
	if werr != nil {
		t.Fatalf("any walk errored: %v", werr)
	}
	if !sat {
		t.Error("any[true,false] must be satisfied (OR)")
	}
	// order-independent: swap the leaves -> still satisfied.
	anyRev := policy.AssertTree{Any: []policy.AssertTree{
		{Leaf: &policy.Leaf{CEL: "new < old"}},
		{Leaf: &policy.Leaf{CEL: "new > old"}},
	}}
	satRev, _, _ := walkAssertTree(env, in, ch, "prod", anyRev)
	if satRev != sat {
		t.Error("any result must be order-independent")
	}

	// not over a true leaf -> false.
	notTree := policy.AssertTree{Not: &policy.AssertTree{Leaf: &policy.Leaf{CEL: "new > old"}}}
	sat, _, werr = walkAssertTree(env, in, ch, "prod", notTree)
	if werr != nil {
		t.Fatalf("not walk errored: %v", werr)
	}
	if sat {
		t.Error("not(true) must be false (negation)")
	}
	// not over a false leaf -> true.
	notFalse := policy.AssertTree{Not: &policy.AssertTree{Leaf: &policy.Leaf{CEL: "new < old"}}}
	if sat, _, _ = walkAssertTree(env, in, ch, "prod", notFalse); !sat {
		t.Error("not(false) must be true")
	}
}

// REQ-E2-S03-03 — a bare-string `when` and the single-leaf-object `{cel: ...}`
// form produce BYTE-IDENTICAL findings; the bare string is exact shorthand for a
// one-leaf tree. Parsed through the loader (not hand-built) so the shorthand
// equivalence is proven end-to-end, not asserted trivially.
func TestBareStringEqualsSingleLeaf(t *testing.T) {
	mkPolicy := func(when string) *policy.MergePolicy {
		raw := []byte(`{
		  "apiVersion": "assent.dev/v1alpha1",
		  "kind": "MergePolicy",
		  "metadata": {"name": "p"},
		  "spec": {
		    "entries": {},
		    "rules": [
		      {
		        "name": "no-shrink",
		        "phase": "enforce",
		        "match": {"valueChanges": {"pointers": ["/partitions"], "kinds": ["modify"]}},
		        "prove": {"obligation": "non-destructive", "when": ` + when + `},
		        "onFailure": {"effect": "block", "code": "shrunk"}
		      }
		    ]
		  }
		}`)
		mp, err := policy.LoadMergePolicy(raw)
		if err != nil {
			t.Fatalf("LoadMergePolicy(%s): %v", when, err)
		}
		return mp
	}

	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "a.yaml", Path: "/partitions", Kind: "modify", Old: intNum(12), New: intNum(6)},
	}}}

	bare, err := Cover(mkPolicy(`"new >= old"`), bind, in)
	if err != nil {
		t.Fatalf("Cover bare: %v", err)
	}
	obj, err := Cover(mkPolicy(`{"cel": "new >= old"}`), bind, in)
	if err != nil {
		t.Fatalf("Cover object: %v", err)
	}

	bareJSON, _ := json.Marshal(bare)
	objJSON, _ := json.Marshal(obj)
	if string(bareJSON) != string(objJSON) {
		t.Errorf("bare string and single-leaf object diverged:\n bare=%s\n obj =%s", bareJSON, objJSON)
	}
	if bare.Decision != DecisionBlock {
		t.Errorf("shrink must BLOCK, got %q", bare.Decision)
	}
}

// REQ-E2-S03-04 — a leaf that errors nested inside all/any/not propagates as a
// tri-state error (fail-safe), never silently false. Adversarial: any:[erroring,
// false] must NOT resolve satisfied, and not:[erroring] must NOT fire.
func TestErroringLeafInTreeFailsSafe(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	in := EvaluationInput{} // no facts -> facts.quota.* errors
	ch := EvalChange{File: "f", Path: "/p", Kind: "modify", Old: intNum(5), New: intNum(10)}

	errLeaf := policy.AssertTree{Leaf: &policy.Leaf{CEL: "new <= facts.quota.max.value"}} // absent fact -> error
	falseLeaf := policy.AssertTree{Leaf: &policy.Leaf{CEL: "new < old"}}                  // 10 < 5 -> clean false
	trueLeaf := policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}                  // 10 >= 5 -> clean true

	// any:[erroring, false] -> no true, one error -> NOT satisfied, error (fail-safe).
	sat, _, werr := walkAssertTree(env, in, ch, "prod", policy.AssertTree{Any: []policy.AssertTree{errLeaf, falseLeaf}})
	if sat {
		t.Error("any[erroring,false] must NOT resolve satisfied (fail-safe)")
	}
	if werr == nil {
		t.Error("any[erroring,false] must propagate the leaf error (tri-state), not silent false")
	}

	// not:[erroring] -> error propagates -> NOT satisfied (must not spuriously fire).
	sat, _, werr = walkAssertTree(env, in, ch, "prod", policy.AssertTree{Not: &errLeaf})
	if sat {
		t.Error("not[erroring] must NOT fire (fail-safe)")
	}
	if werr == nil {
		t.Error("not[erroring] must propagate the leaf error")
	}

	// all:[true, erroring] -> no clean false, one error -> NOT satisfied, error.
	sat, _, werr = walkAssertTree(env, in, ch, "prod", policy.AssertTree{All: []policy.AssertTree{trueLeaf, errLeaf}})
	if sat {
		t.Error("all[true,erroring] must NOT be satisfied")
	}
	if werr == nil {
		t.Error("all[true,erroring] must propagate the leaf error")
	}
}

// REQ-E2-S03-05 — the walker is order-independent for the all/any RESULT (only
// the attributed message follows declared order) and double-runs byte-identical.
func TestAssertTreeDoubleRunStable(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	in := EvaluationInput{}
	ch := EvalChange{File: "f", Path: "/p", Kind: "modify", Old: intNum(5), New: intNum(10)}

	errLeaf := policy.AssertTree{Leaf: &policy.Leaf{CEL: "new <= facts.quota.max.value"}}
	falseLeaf := policy.AssertTree{Leaf: &policy.Leaf{CEL: "new < old"}}

	// Kleene-AND: `all` with a clean-false and an erroring leaf must yield the SAME
	// tri-state regardless of leaf order — false DOMINATES the error (order-
	// independent result; a plain double-run would never surface this).
	satA, _, errA := walkAssertTree(env, in, ch, "prod", policy.AssertTree{All: []policy.AssertTree{falseLeaf, errLeaf}})
	satB, _, errB := walkAssertTree(env, in, ch, "prod", policy.AssertTree{All: []policy.AssertTree{errLeaf, falseLeaf}})
	if satA != satB || (errA == nil) != (errB == nil) {
		t.Errorf("all tri-state is order-dependent: [false,err]=(%v,%v) [err,false]=(%v,%v)", satA, errA, satB, errB)
	}
	if satA {
		t.Error("all containing a clean-false leaf must be unsatisfied")
	}
	if errA != nil {
		t.Error("all with a clean-false leaf must NOT surface the sibling error (false dominates)")
	}

	// Double-run through Cover is byte-identical.
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{{
		Name:  "any-guard",
		Phase: policy.PhaseEnforce,
		Match: policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
		Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Any: []policy.AssertTree{
			{Leaf: &policy.Leaf{CEL: "new < old", Message: "a"}},
			{Leaf: &policy.Leaf{CEL: "new == old", Message: "b"}},
		}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "owner"},
	}}}}
	bind := &policy.Binding{Require: []string{"ownership"}, Environment: "prod"}
	cin := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "a.yaml", Path: "/p", Kind: "modify", Old: intNum(5), New: intNum(10)},
	}}}
	r1, _ := Cover(pol, bind, cin)
	r2, _ := Cover(pol, bind, cin)
	j1, _ := json.Marshal(r1)
	j2, _ := json.Marshal(r2)
	if string(j1) != string(j2) {
		t.Errorf("double-run diverged:\n a=%s\n b=%s", j1, j2)
	}
}

// TestAssertTreeMessageTemplateExpansion — a leaf message with {{ old }}/{{ new }}
// and {{ env }} placeholders expands over the SAME activation model as the CEL
// leaf (ADR-0013 residual #5). Locks the shared activation contract.
func TestAssertTreeMessageTemplateExpansion(t *testing.T) {
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{{
		Name:  "shrink",
		Phase: policy.PhaseEnforce,
		Match: policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
		Prove: &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{
			CEL:     "new >= old",
			Message: "partitions {{ old }} -> {{ new }} in env {{ env }}",
		}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "shrunk"},
	}}}}
	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "a.yaml", Path: "/partitions", Kind: "modify", Old: intNum(12), New: intNum(6)},
	}}}
	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if len(got.Findings) != 1 {
		t.Fatalf("want one finding, got %+v", got.Findings)
	}
	want := "partitions 12 -> 6 in env prod"
	if got.Findings[0].Message != want {
		t.Errorf("expanded message = %q, want %q", got.Findings[0].Message, want)
	}
}

// TestMessageTemplateFactsPath — a {{ facts.<provider>.<name>.<field> }} placeholder
// resolves over the shared activation (the same nested facts map the CEL leaf
// navigates), locking the facts template case ADR-0013 residual #5 calls out.
func TestMessageTemplateFactsPath(t *testing.T) {
	in := loadD016Input(t) // owner fact is expired
	part := changeAtPath(t, in, "topics/prod/orders-events.yaml", "/partitions")

	got := expandMessage("owner is {{ facts.owner.team.state }}", *in, part, "prod")
	if got != "owner is expired" {
		t.Errorf("facts template = %q, want %q", got, "owner is expired")
	}
	// A path descending into a map (not a scalar) is left LITERAL, never panicking.
	lit := expandMessage("obj {{ facts.owner.team }}", *in, part, "prod")
	if lit != "obj {{ facts.owner.team }}" {
		t.Errorf("non-scalar path must stay literal, got %q", lit)
	}
}

// TestPerSubjectMessageOrderIndependent — when one subject has MULTIPLE matched
// changes that fail with DIFFERENT expanded messages, the attributed Message must
// be a canonical function of the failing set, NOT the input order. Reversing the
// change list must yield a byte-identical Result (determinism hard-rule / REQ-05).
func TestPerSubjectMessageOrderIndependent(t *testing.T) {
	mkPol := func() *policy.MergePolicy {
		return &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{{
			Name:  "no-shrink",
			Phase: policy.PhaseEnforce,
			Match: policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
			Prove: &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{
				CEL:     "new >= old",
				Message: "{{ path }} {{ old }}->{{ new }}",
			}}},
			OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "shrunk"},
		}}}}
	}
	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}
	c1 := EvalChange{Subject: "s:1", File: "a.yaml", Path: "/p1", Kind: "modify", Old: intNum(5), New: intNum(3)}
	c2 := EvalChange{Subject: "s:1", File: "a.yaml", Path: "/p2", Kind: "modify", Old: intNum(9), New: intNum(4)}

	fwd, err := Cover(mkPol(), bind, &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{c1, c2}}})
	if err != nil {
		t.Fatalf("Cover fwd: %v", err)
	}
	rev, err := Cover(mkPol(), bind, &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{c2, c1}}})
	if err != nil {
		t.Fatalf("Cover rev: %v", err)
	}
	fj, _ := json.Marshal(fwd)
	rj, _ := json.Marshal(rev)
	if string(fj) != string(rj) {
		t.Errorf("per-subject message capture is order-dependent:\n fwd=%s\n rev=%s", fj, rj)
	}
}
