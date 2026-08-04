package compare

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

func canonicalPromotionGates() []PromotionGate {
	return []PromotionGate{
		{GateID: GateZeroMissedDestructive, FailOnKinds: []Kind{KindDestructiveOrAuthorizationInterventionMissed}, DefaultVerdict: "fail", Acceptance: "per-delta-identity"},
		{GateID: GateZeroMissedAuthorizationOwnership, FailOnKinds: []Kind{KindDestructiveOrAuthorizationInterventionMissed}, DefaultVerdict: "fail", Acceptance: "per-delta-identity"},
		{GateID: GateNoUnexpectedObligationRemoval, FailOnKinds: []Kind{KindSubjectOrObligationUncovered}, DefaultVerdict: "fail", Acceptance: "per-delta-identity"},
		{GateID: GateBoundedAutoMergeWidening, FailOnKinds: []Kind{KindNewlyAutoMergeable}, DefaultVerdict: "fail", Acceptance: "per-delta-identity"},
		{GateID: GateExplicitlyAcceptedDeltas, FailOnKinds: []Kind{KindStricterInterventionAdded, KindScoreThresholdChange}, DefaultVerdict: "fail", Acceptance: "per-delta-identity"},
	}
}

func wideningRecord(t *testing.T) ComparisonRecord {
	t.Helper()
	in := loadBundle(t)
	baseline := mkProfile("prod-strict@6", "new >= old", "", policy.EffectBlock)
	candidate := mkProfile("prod-strict@7", "true", "", policy.EffectBlock)
	rec, err := BuildComparisonRecord("widen-case", baseline.Name, candidate.Name,
		evaluateMust(t, in, baseline), evaluateMust(t, in, candidate), []string{testBundleSubject})
	if err != nil {
		t.Fatalf("BuildComparisonRecord: %v", err)
	}
	return rec
}

func explanationOnlyRecord(t *testing.T) ComparisonRecord {
	t.Helper()
	in := loadBundle(t)
	baseline := mkProfile("prod@6", "new >= old", "partitions must not shrink below the baseline", policy.EffectBlock)
	candidate := mkProfile("prod@7", "new >= old", "partitions may not be reduced", policy.EffectBlock)
	rec, err := BuildComparisonRecord("wording-only", baseline.Name, candidate.Name,
		evaluateMust(t, in, baseline), evaluateMust(t, in, candidate), []string{testBundleSubject})
	if err != nil {
		t.Fatalf("BuildComparisonRecord: %v", err)
	}
	return rec
}

// REQ-PCS-S05-01: a newly-auto-mergeable delta not in acceptedDeltas FAILs
// bounded-auto-merge-widening; other gates PASS for that delta kind.
func TestGateBoundedAutoMergeWidening(t *testing.T) {
	rec := wideningRecord(t)
	ev := EvaluateGates(PolicyComparisonSuiteSpec{PromotionGates: canonicalPromotionGates()}, []ComparisonRecord{rec})

	if ev.Results[GateBoundedAutoMergeWidening] != VerdictFail {
		t.Fatalf("bounded-auto-merge-widening = %q, want FAIL", ev.Results[GateBoundedAutoMergeWidening])
	}
	for _, g := range []GateID{
		GateZeroMissedDestructive,
		GateZeroMissedAuthorizationOwnership,
		GateNoUnexpectedObligationRemoval,
		GateExplicitlyAcceptedDeltas,
	} {
		if ev.Results[g] != VerdictPass {
			t.Fatalf("%s = %q, want PASS (widening-only record)", g, ev.Results[g])
		}
	}
	if ev.FirstFailure != GateBoundedAutoMergeWidening {
		t.Fatalf("FirstFailure = %q, want %q", ev.FirstFailure, GateBoundedAutoMergeWidening)
	}
}

// REQ-PCS-S05-02: an acceptedDeltas entry for the exact caseId+identity+kind
// prevents that delta from failing its gate.
func TestAcceptedDeltaAllowsSpecificIdentity(t *testing.T) {
	rec := wideningRecord(t)
	d := rec.Deltas[0]
	accepted := []AcceptedDelta{{
		CaseID:    rec.CaseID,
		Kind:      d.Kind,
		Rule:      d.Rule,
		Subject:   d.Subject,
		Rationale: "reviewed widening for quota pilot",
	}}
	ev := EvaluateGates(PolicyComparisonSuiteSpec{
		PromotionGates: canonicalPromotionGates(),
		AcceptedDeltas: accepted,
	}, []ComparisonRecord{rec})

	if ev.Results[GateBoundedAutoMergeWidening] != VerdictPass {
		t.Fatalf("bounded-auto-merge-widening = %q, want PASS when delta is allowlisted", ev.Results[GateBoundedAutoMergeWidening])
	}
	if ev.FirstFailure != "" {
		t.Fatalf("FirstFailure = %q, want empty when all gates pass", ev.FirstFailure)
	}

	// Negative: kind-only partial identity must NOT allow (wrong rule).
	wrongRule := []AcceptedDelta{{
		CaseID:    rec.CaseID,
		Kind:      d.Kind,
		Rule:      "other-rule",
		Subject:   d.Subject,
		Rationale: "kind-only footgun attempt",
	}}
	ev2 := EvaluateGates(PolicyComparisonSuiteSpec{
		PromotionGates: canonicalPromotionGates(),
		AcceptedDeltas: wrongRule,
	}, []ComparisonRecord{rec})
	if ev2.Results[GateBoundedAutoMergeWidening] != VerdictFail {
		t.Fatalf("bounded-auto-merge-widening = %q, want FAIL when rule identity mismatches", ev2.Results[GateBoundedAutoMergeWidening])
	}
}

// REQ-PCS-S05-04: explanation-only deltas never flip any gate to FAIL.
func TestExplanationOnlyNeverFailsGate(t *testing.T) {
	rec := explanationOnlyRecord(t)
	ev := EvaluateGates(PolicyComparisonSuiteSpec{PromotionGates: canonicalPromotionGates()}, []ComparisonRecord{rec})

	for _, gate := range canonicalPromotionGates() {
		if ev.Results[gate.GateID] != VerdictPass {
			t.Fatalf("%s = %q, want PASS (explanation-only never fails gates)", gate.GateID, ev.Results[gate.GateID])
		}
	}
	if ev.FirstFailure != "" {
		t.Fatalf("FirstFailure = %q, want empty", ev.FirstFailure)
	}
}

// Exercise all five gate IDs with the delta kind each gate owns.
func TestEvaluateAllFiveGateIDs(t *testing.T) {
	gates := canonicalPromotionGates()

	t.Run("zero-missed-destructive", func(t *testing.T) {
		in := loadBundle(t)
		baseline := mkProfile("prod-strict@6", "new >= old", "", policy.EffectBlock)
		candidate := Profile{
			Name:    "prod-permissive@7",
			Policy:  &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: nil}},
			Bind:    baseline.Bind,
			Ceiling: policy.PhaseEnforce,
		}
		rec, err := BuildComparisonRecord("missed-intervention", baseline.Name, candidate.Name,
			evaluateMust(t, in, baseline), evaluateMust(t, in, candidate), []string{testBundleSubject})
		if err != nil {
			t.Fatalf("BuildComparisonRecord: %v", err)
		}
		ev := EvaluateGates(PolicyComparisonSuiteSpec{PromotionGates: gates}, []ComparisonRecord{rec})
		if ev.Results[GateZeroMissedDestructive] != VerdictFail {
			t.Fatalf("zero-missed-destructive = %q, want FAIL", ev.Results[GateZeroMissedDestructive])
		}
		if ev.Results[GateZeroMissedAuthorizationOwnership] != VerdictFail {
			t.Fatalf("zero-missed-authorization-ownership = %q, want FAIL", ev.Results[GateZeroMissedAuthorizationOwnership])
		}
		if ev.FirstFailure != GateZeroMissedDestructive {
			t.Fatalf("FirstFailure = %q, want %q", ev.FirstFailure, GateZeroMissedDestructive)
		}
	})

	t.Run("no-unexpected-obligation-removal", func(t *testing.T) {
		in := loadBundleJSON(t, obligationBundleJSON)
		baseline := mkObligationProfile("prod-covered@6", []policy.Rule{ownershipRule(), nonDestructiveRule()})
		candidate := mkObligationProfile("prod-uncovered@7", []policy.Rule{nonDestructiveRule()})
		rec, err := BuildComparisonRecord("obligation-uncovered", baseline.Name, candidate.Name,
			evaluateMust(t, in, baseline), evaluateMust(t, in, candidate), []string{testBundleSubject})
		if err != nil {
			t.Fatalf("BuildComparisonRecord: %v", err)
		}
		ev := EvaluateGates(PolicyComparisonSuiteSpec{PromotionGates: gates}, []ComparisonRecord{rec})
		if ev.Results[GateNoUnexpectedObligationRemoval] != VerdictFail {
			t.Fatalf("no-unexpected-obligation-removal = %q, want FAIL", ev.Results[GateNoUnexpectedObligationRemoval])
		}
	})

	t.Run("explicitly-accepted-deltas stricter", func(t *testing.T) {
		in := loadBundle(t)
		baseline := mkProfile("prod-permissive@6", "true", "", policy.EffectBlock)
		candidate := mkProfile("prod-strict@7", "new >= old", "", policy.EffectBlock)
		rec, err := BuildComparisonRecord("stricter-added", baseline.Name, candidate.Name,
			evaluateMust(t, in, baseline), evaluateMust(t, in, candidate), []string{testBundleSubject})
		if err != nil {
			t.Fatalf("BuildComparisonRecord: %v", err)
		}
		ev := EvaluateGates(PolicyComparisonSuiteSpec{PromotionGates: gates}, []ComparisonRecord{rec})
		if ev.Results[GateExplicitlyAcceptedDeltas] != VerdictFail {
			t.Fatalf("explicitly-accepted-deltas = %q, want FAIL", ev.Results[GateExplicitlyAcceptedDeltas])
		}
	})

	t.Run("explicitly-accepted-deltas score-threshold", func(t *testing.T) {
		in := loadBundleJSON(t, scoreBundleJSON)
		baseline := mkScoreProfile("prod-threshold@6", 4)
		candidate := mkScoreProfile("prod-threshold@7", 10)
		rec, err := BuildComparisonRecord("score-threshold", baseline.Name, candidate.Name,
			evaluateMust(t, in, baseline), evaluateMust(t, in, candidate), []string{testBundleSubject})
		if err != nil {
			t.Fatalf("BuildComparisonRecord: %v", err)
		}
		ev := EvaluateGates(PolicyComparisonSuiteSpec{PromotionGates: gates}, []ComparisonRecord{rec})
		if ev.Results[GateExplicitlyAcceptedDeltas] != VerdictFail {
			t.Fatalf("explicitly-accepted-deltas = %q, want FAIL", ev.Results[GateExplicitlyAcceptedDeltas])
		}
	})
}
