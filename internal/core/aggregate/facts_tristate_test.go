package aggregate

// facts_tristate_test.go locks in the E2-S05 fact tri-state fail-safe: a
// controlling/authorization fact in state unavailable/invalid/expired can never
// yield APPROVE, driven end-to-end through the Cover decision loop (ADR-0007 F6 /
// ADR-0017 §6). Determinism: no clock is introduced — expiry state is
// pre-computed by the provider tier and carried in facts[].state, never
// recomputed against time.Now in internal/core.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// TestExpiredFactStateGuardFailsClosed is the D-016 owner-rule path (REQ-E2-S05-01):
// `when: facts.owner.team.state == 'resolved'` over an EXPIRED owner fact is FALSE
// -> ownership unproven -> the rule's require-review fires per governed subject ->
// the run can never APPROVE. Reuses the canonical frozen fixture.
func TestExpiredFactStateGuardFailsClosed(t *testing.T) {
	mp, bind, in := loadD016(t)

	// Precondition: the fixture's owner fact really is expired (guards the test
	// against a fixture edit silently flipping it to resolved).
	if got := in.Facts["owner"]["team"].State; got != "expired" {
		t.Fatalf("fixture precondition: owner fact state = %q, want expired", got)
	}

	got, err := Cover(mp, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if got.Decision == DecisionApprove {
		t.Fatal("an expired controlling ownership fact must never APPROVE")
	}

	// Every governed subject the ownership rule matched must carry a require-review
	// ownership finding (the state guard fell false, obligation unproven).
	var ownerFindings int
	for _, f := range got.Findings {
		if f.Obligation != "ownership" {
			continue
		}
		ownerFindings++
		if f.Effect != EffectRequireReview {
			t.Errorf("ownership finding %+v: effect = %q, want require-review", f, f.Effect)
		}
		if f.Code != "ownership-approval-missing" {
			t.Errorf("ownership finding %+v: code = %q, want ownership-approval-missing", f, f.Code)
		}
	}
	if ownerFindings == 0 {
		t.Fatal("expired owner fact produced no ownership require-review finding")
	}
}

// TestAbsentFactValueErrorsFailSafe (REQ-E2-S05-02): a `when` reading a
// non-resolved fact's absent `value` evaluates to a tri-state ERROR, fail-safe by
// effect (obligation unproven), never a silent true/false in the permissive
// direction. Adversarial: EACH of unavailable/invalid/expired is proven to fail
// closed — including the hardening case where a malformed non-resolved fact
// carries a STALE in-memory value that must still not bind.
func TestAbsentFactValueErrorsFailSafe(t *testing.T) {
	// A rule that PROVES ownership only when the owner fact's value equals a
	// sentinel. On a non-resolved fact `.value` is absent -> the leaf errors.
	provePolicy := func() *policy.MergePolicy {
		return &policy.MergePolicy{
			Spec: policy.MergePolicySpec{
				Rules: []policy.Rule{{
					Name:      "owner-value-present",
					Phase:     policy.PhaseEnforce,
					Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
					Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "facts.owner.team.value == 1"}}},
					OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "owner-missing"},
				}},
			},
		}
	}
	bind := &policy.Binding{Require: []string{"ownership"}, Environment: "prod"}
	change := EvalChange{Subject: "s:1", File: "a.yaml", Path: "/x", Kind: "modify", Old: intNum(1), New: intNum(2)}

	for _, state := range []string{"unavailable", "invalid", "expired"} {
		t.Run(state, func(t *testing.T) {
			// (a) canonical: a non-resolved fact carries NO value key.
			in := &EvaluationInput{
				ChangeSet: ChangeSet{Changes: []EvalChange{change}},
				Facts:     map[string]map[string]Fact{"owner": {"team": {State: state}}},
			}
			assertPredicateErrorFailSafe(t, provePolicy(), bind, in)

			// (b) adversarial hardening: a MALFORMED non-resolved fact that carries a
			// stale in-memory value must STILL fail closed — the evaluator must not
			// bind a value for any state != resolved. Without the state gate this
			// value would bind, the predicate would be TRUE, ownership would prove,
			// and the run would fail OPEN to APPROVE.
			inStale := &EvaluationInput{
				ChangeSet: ChangeSet{Changes: []EvalChange{change}},
				Facts:     map[string]map[string]Fact{"owner": {"team": {State: state, Value: intNum(1)}}},
			}
			assertPredicateErrorFailSafe(t, provePolicy(), bind, inStale)
		})
	}
}

// assertPredicateErrorFailSafe asserts Cover never APPROVEs and emits exactly the
// one predicate.error require-review finding for the fail-safe path.
func assertPredicateErrorFailSafe(t *testing.T, mp *policy.MergePolicy, bind *policy.Binding, in *EvaluationInput) {
	t.Helper()
	got, err := Cover(mp, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if got.Decision == DecisionApprove {
		t.Fatal("a non-resolved fact's absent .value must never APPROVE (fail-open)")
	}
	if len(got.Findings) != 1 {
		t.Fatalf("want exactly one fail-safe finding, got %+v", got.Findings)
	}
	if got.Findings[0].Effect != EffectRequireReview || got.Findings[0].Code != "predicate.error" {
		t.Fatalf("want a predicate.error require-review finding, got %+v", got.Findings[0])
	}
}

// TestControllingFactMayNotFailOpen (REQ-E2-S05-03): a provider backing a
// controlling/authorization fact — one whose facts feed a require-review
// obligation proof — may not be configured `failure: open` (ADR-0017 §6). The
// check is a load-time policy validation; an advisory provider (referenced only
// by a non-require-review effect) may fail open.
func TestControllingFactMayNotFailOpen(t *testing.T) {
	mp := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{
				{ // authorization rule: onFailure escalates to require-review, reads facts.owner.
					Name:      "topic-owner-must-approve",
					Phase:     policy.PhaseEnforce,
					Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
					Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "facts.owner.team.state == 'resolved'"}}},
					OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "ownership-approval-missing"},
				},
				{ // advisory rule: onFailure is block (not require-review), reads facts.quota.
					Name:      "partitions-bounded",
					Phase:     policy.PhaseEnforce,
					Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
					Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new <= facts.quota.max_partitions"}}},
					OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "over-quota"},
				},
			},
		},
	}

	t.Run("controlling provider failure:open is rejected", func(t *testing.T) {
		cfg := &policy.Config{Providers: map[string]policy.Provider{
			"owner": {Type: "http", Failure: "open"},
			"quota": {Type: "http", Failure: "closed"},
		}}
		err := policy.ValidateProviderPosture(cfg, mp)
		if err == nil {
			t.Fatal("a controlling (require-review-backing) provider configured failure:open must be rejected")
		}
		if !strings.Contains(err.Error(), "owner") {
			t.Errorf("rejection must name the offending provider (owner), got: %v", err)
		}
	})

	t.Run("advisory provider may fail open", func(t *testing.T) {
		// quota is referenced only by a block (non-require-review) rule -> advisory.
		cfg := &policy.Config{Providers: map[string]policy.Provider{
			"owner": {Type: "http", Failure: "closed"},
			"quota": {Type: "http", Failure: "open"},
		}}
		if err := policy.ValidateProviderPosture(cfg, mp); err != nil {
			t.Errorf("an advisory provider (no require-review proof) may fail open, got rejection: %v", err)
		}
	})

	t.Run("all closed is accepted", func(t *testing.T) {
		cfg := &policy.Config{Providers: map[string]policy.Provider{
			"owner": {Type: "http", Failure: "closed"},
			"quota": {Type: "http", Failure: "closed"},
		}}
		if err := policy.ValidateProviderPosture(cfg, mp); err != nil {
			t.Errorf("all-closed providers must be accepted, got: %v", err)
		}
	})
}

// TestResolvedFactProvesNormally (REQ-E2-S05-04): a resolved fact whose value
// satisfies the predicate proves the obligation normally — the fail-safe does not
// over-fire on healthy facts. Uses a D-016-style resolved quota fact.
func TestResolvedFactProvesNormally(t *testing.T) {
	mp := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "under-quota",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
				Prove:     &policy.Prove{Obligation: "bounded", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new <= facts.quota.max_partitions.value"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "over-quota"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"bounded"}, Environment: "prod"}
	in := &EvaluationInput{
		ChangeSet: ChangeSet{Changes: []EvalChange{
			{Subject: "s:1", File: "topics/prod/a.yaml", Path: "/partitions", Kind: "modify", Old: intNum(12), New: intNum(6)},
		}},
		Facts: map[string]map[string]Fact{
			"quota": {"max_partitions": {State: "resolved", Value: intNum(24)}},
		},
	}

	got, err := Cover(mp, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if len(got.Findings) != 0 {
		t.Errorf("a healthy resolved fact proving the obligation must emit no finding, got %+v", got.Findings)
	}
	if got.Decision != DecisionApprove {
		t.Errorf("a proven obligation over a resolved fact must not lower APPROVE, got %q", got.Decision)
	}
}

// TestFactTristateDoubleRunStable (REQ-E2-S05-05): the tri-state fail-safe is
// deterministic — no clock, byte-identical over a double run.
func TestFactTristateDoubleRunStable(t *testing.T) {
	mp := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "owner-state-resolved",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
				Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "facts.owner.team.state == 'resolved'"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "owner-missing"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"ownership"}, Environment: "prod"}
	in := &EvaluationInput{
		ChangeSet: ChangeSet{Changes: []EvalChange{
			{Subject: "s:1", File: "a.yaml", Path: "/x", Kind: "modify", Old: intNum(1), New: intNum(2)},
		}},
		Facts: map[string]map[string]Fact{"owner": {"team": {State: "expired"}}},
	}

	a, err := Cover(mp, bind, in)
	if err != nil {
		t.Fatalf("Cover a: %v", err)
	}
	b, err := Cover(mp, bind, in)
	if err != nil {
		t.Fatalf("Cover b: %v", err)
	}
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	if string(aJSON) != string(bJSON) {
		t.Errorf("tri-state decision not double-run stable:\n a=%s\n b=%s", aJSON, bJSON)
	}
	if a.Decision == DecisionApprove {
		t.Fatal("expired owner fact must never APPROVE")
	}
}
