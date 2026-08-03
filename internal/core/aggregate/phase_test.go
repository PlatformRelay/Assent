package aggregate

import (
	"encoding/json"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// blockProvingRule builds a block-producing obligation rule at the given phase:
// its `when` is cleanly false (`false`), so its OnFailure block effect fires for
// every matched change. Identical across phases except for Phase — the paired
// REQ-08-01 fixture.
func blockProvingRule(phase policy.Phase) policy.Rule {
	return policy.Rule{
		Name:      "would-block",
		Phase:     phase,
		Points:    7,
		Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
		Prove:     &policy.Prove{Obligation: "signal", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "false"}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "would-block"},
	}
}

// singleEvalInput is a single governed change the block rule matches.
func singleEvalInput() *EvaluationInput {
	return &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "topic-registry:orders.events.v1", File: "topics/prod/orders.yaml", Path: "/x", Kind: "modify", Old: intNum(1), New: intNum(2)},
	}}}
}

// TestObserveExcludedEnforceIncluded — REQ-E2-S08-01. Two identical block-
// producing rules differing ONLY in phase: enforce routes its finding to the
// enforcing bucket and yields BLOCK; observe routes the identical finding to the
// observed bucket and leaves the decision UNCHANGED — the observe finding is
// STRUCTURALLY excluded from aggregation (decision AND points).
func TestObserveExcludedEnforceIncluded(t *testing.T) {
	// signal is NOT required and threshold is 0, so if an observe finding (or its
	// 7 points) leaked into aggregation the decision would flip to BLOCK/REVIEW.
	bind := &policy.Binding{Require: nil, Environment: "prod", Risk: policy.Risk{Threshold: 0}}
	in := singleEvalInput()

	enforcePol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{blockProvingRule(policy.PhaseEnforce)}}}
	enf, err := Cover(enforcePol, bind, in)
	if err != nil {
		t.Fatalf("Cover enforce: %v", err)
	}
	if enf.Decision != DecisionBlock {
		t.Errorf("enforce decision = %q, want BLOCK", enf.Decision)
	}
	if len(enf.Findings) != 1 || enf.Findings[0].Code != "would-block" {
		t.Fatalf("enforce: want one enforcing would-block finding, got %+v", enf.Findings)
	}
	if len(enf.Observed) != 0 {
		t.Errorf("enforce: observed bucket must be empty, got %+v", enf.Observed)
	}

	observePol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{blockProvingRule(policy.PhaseObserve)}}}
	obs, err := Cover(observePol, bind, in)
	if err != nil {
		t.Fatalf("Cover observe: %v", err)
	}
	// Decision UNCHANGED: the observe finding never enters the reduction, and its
	// 7 points never enter the sum (threshold 0 would else escalate to REVIEW).
	if obs.Decision != DecisionApprove {
		t.Errorf("observe decision = %q, want APPROVE (finding + points structurally excluded)", obs.Decision)
	}
	if len(obs.Findings) != 0 {
		t.Errorf("observe: enforcing bucket must be empty, got %+v", obs.Findings)
	}
	if len(obs.Observed) != 1 || obs.Observed[0].Code != "would-block" {
		t.Fatalf("observe: want one observed would-block finding, got %+v", obs.Observed)
	}
	// The observed finding is the SAME finding the enforce phase would have
	// enforced — only its routing (bucket) differs.
	if obs.Observed[0] != enf.Findings[0] {
		t.Errorf("observed finding %+v must equal the enforce finding %+v (only phase routing differs)", obs.Observed[0], enf.Findings[0])
	}
}

// TestOffRuleNeverEvaluated — REQ-E2-S08-02. A phase:off rule is NEVER evaluated:
// no finding in observed OR enforcing. Adversarial: its `when` references an
// undeclared identifier that WOULD raise a compile error if evaluated — off must
// short-circuit BEFORE any CEL compile/eval, so no predicate.error surfaces.
func TestOffRuleNeverEvaluated(t *testing.T) {
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{{
		Name:  "parked",
		Phase: policy.PhaseOff,
		Match: policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
		// undeclared identifier -> a COMPILE error IF this rule were ever evaluated.
		Prove:     &policy.Prove{Obligation: "signal", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "undeclared_ident_xyz >= 1"}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "would-block"},
	}}}}
	bind := &policy.Binding{Require: nil, Environment: "prod"}
	in := singleEvalInput()

	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover must not error on an off rule (it is never evaluated): %v", err)
	}
	if got.Decision != DecisionApprove {
		t.Errorf("decision = %q, want APPROVE (off rule never feeds the decision)", got.Decision)
	}
	if len(got.Findings) != 0 {
		t.Errorf("off rule must produce no enforcing finding, got %+v", got.Findings)
	}
	if len(got.Observed) != 0 {
		t.Errorf("off rule must produce no observed finding (never evaluated), got %+v", got.Observed)
	}
}

// TestPackPhaseCeiling — REQ-E2-S08-03. A pack spec.phase ceiling DOWNGRADES: an
// enforce rule under an observe ceiling runs as observe (finding -> observed, not
// enforcing); under an off ceiling it evaluates not at all. The ceiling only ever
// caps toward off — min(rule.phase, pack.phase).
func TestPackPhaseCeiling(t *testing.T) {
	bind := &policy.Binding{Require: nil, Environment: "prod", Risk: policy.Risk{Threshold: 0}}
	in := singleEvalInput()
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{blockProvingRule(policy.PhaseEnforce)}}}

	// Observe ceiling caps the enforce rule to observe.
	capped, err := CoverWithPhaseCeiling(pol, bind, in, nil, policy.PhaseObserve)
	if err != nil {
		t.Fatalf("CoverWithPhaseCeiling(observe): %v", err)
	}
	if capped.Decision != DecisionApprove {
		t.Errorf("observe-ceiling decision = %q, want APPROVE (enforce rule capped to observe)", capped.Decision)
	}
	if len(capped.Findings) != 0 {
		t.Errorf("observe ceiling: enforcing bucket must be empty, got %+v", capped.Findings)
	}
	if len(capped.Observed) != 1 || capped.Observed[0].Code != "would-block" {
		t.Fatalf("observe ceiling: want the enforce rule's finding routed to observed, got %+v", capped.Observed)
	}

	// Off ceiling: the inner enforce rule evaluates not at all.
	off, err := CoverWithPhaseCeiling(pol, bind, in, nil, policy.PhaseOff)
	if err != nil {
		t.Fatalf("CoverWithPhaseCeiling(off): %v", err)
	}
	if off.Decision != DecisionApprove {
		t.Errorf("off-ceiling decision = %q, want APPROVE", off.Decision)
	}
	if len(off.Findings) != 0 || len(off.Observed) != 0 {
		t.Errorf("off ceiling: no rule evaluates, both buckets must be empty; enforcing=%+v observed=%+v", off.Findings, off.Observed)
	}

	// Enforce ceiling: each rule's own phase stands (no cap) — the enforce rule
	// enforces, exactly as plain Cover.
	full, err := CoverWithPhaseCeiling(pol, bind, in, nil, policy.PhaseEnforce)
	if err != nil {
		t.Fatalf("CoverWithPhaseCeiling(enforce): %v", err)
	}
	if full.Decision != DecisionBlock || len(full.Findings) != 1 {
		t.Errorf("enforce ceiling must not cap: decision=%q findings=%+v", full.Decision, full.Findings)
	}
}

// TestMissingPhaseRejected — REQ-E2-S08-04. A MergePolicy rule or a Pack with a
// missing `phase` is rejected at load (no-implicit-enforce, ADR-0018 §1). The
// frozen schemas mark phase required and the loaders validate pre-decode, so a
// missing phase can never silently default to off/observe/enforce.
func TestMissingPhaseRejected(t *testing.T) {
	ruleMissingPhase := []byte(`{
	  "apiVersion": "assent.dev/v1alpha1",
	  "kind": "MergePolicy",
	  "metadata": {"name": "topic-safety"},
	  "spec": {
	    "entries": {},
	    "rules": [
	      {"name": "undecorated", "match": {"files": {"paths": ["topics/**"]}}, "effect": "block"}
	    ]
	  }
	}`)
	if _, err := policy.LoadMergePolicy(ruleMissingPhase); err == nil {
		t.Error("a MergePolicy rule missing phase must be rejected at load (no-implicit-enforce)")
	}

	packMissingPhase := []byte(`{
	  "apiVersion": "assent.dev/v1alpha1",
	  "kind": "Pack",
	  "metadata": {"name": "topics"},
	  "spec": {"version": "1.0.0"}
	}`)
	if _, err := policy.LoadPack(packMissingPhase); err == nil {
		t.Error("a Pack missing spec.phase must be rejected at load (no-implicit-enforce)")
	}

	// Control: the same document WITH a phase loads cleanly — the rejection is the
	// missing field, not a generic parse failure.
	if _, err := policy.LoadPack([]byte(`{
	  "apiVersion": "assent.dev/v1alpha1",
	  "kind": "Pack",
	  "metadata": {"name": "topics"},
	  "spec": {"phase": "observe", "version": "1.0.0"}
	}`)); err != nil {
		t.Errorf("a Pack WITH phase must load: %v", err)
	}
}

// TestObservePhaseDoubleRunStable — REQ-E2-S08-05 (determinism). A policy mixing
// enforce and observe rules produces a byte-identical Result across a double-run
// and under a shuffled rule order — the observed bucket is canonically sorted
// like the enforcing one.
func TestObservePhaseDoubleRunStable(t *testing.T) {
	rules := []policy.Rule{
		blockProvingRule(policy.PhaseObserve),
		{
			Name:      "enforce-owner",
			Phase:     policy.PhaseEnforce,
			Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
			Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "false"}}},
			OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "owner-missing"},
		},
	}
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: rules}}
	bind := &policy.Binding{Require: []string{"ownership"}, Environment: "prod"}
	in := singleEvalInput()

	base, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover base: %v", err)
	}
	if len(base.Observed) != 1 {
		t.Fatalf("want one observed finding, got %+v", base.Observed)
	}
	baseJSON, _ := json.Marshal(base)

	again, _ := Cover(pol, bind, in)
	againJSON, _ := json.Marshal(again)
	if string(baseJSON) != string(againJSON) {
		t.Errorf("phase routing not double-run stable:\n a=%s\n b=%s", baseJSON, againJSON)
	}

	shuffled := *pol
	shuffled.Spec.Rules = reversedRules(rules)
	got, err := Cover(&shuffled, bind, in)
	if err != nil {
		t.Fatalf("Cover shuffled: %v", err)
	}
	gotJSON, _ := json.Marshal(got)
	if string(baseJSON) != string(gotJSON) {
		t.Errorf("phase routing not order-independent:\n base=%s\n got =%s", baseJSON, gotJSON)
	}
}
