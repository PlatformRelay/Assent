package compare

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/schemas"
)

// bundleJSON is a minimal ReplayBundle whose pre-built EvaluationInput carries a
// single destructive change: topic partitions SHRINK 12 -> 6. It is schema-valid
// against the frozen replay-bundle contract (pins + evaluationInput), and it is the
// shared, immutable replay input every baseline/candidate profile is evaluated over.
const bundleJSON = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "ReplayBundle",
  "pins": {
    "toolVersion": "0.0.0-test",
    "toolDigest": "sha256:aaaa",
    "policySha": "sha256:bbbb",
    "sourceSha": "cccc",
    "targetSha": "dddd",
    "mergeResultDigest": "sha256:eeee",
    "factsResolvedAt": {}
  },
  "evaluationInput": {
    "apiVersion": "assent.dev/v1alpha1",
    "kind": "EvaluationInput",
    "changeSet": {
      "changes": [
        {
          "subject": "topic-registry:orders.events.v1",
          "file": "topics/prod/orders.events.v1.yaml",
          "path": "/partitions",
          "kind": "modify",
          "old": 12,
          "new": 6
        }
      ]
    },
    "facts": {},
    "mr": {"author": "alice", "sourceBranch": "topic/shrink", "targetBranch": "main"},
    "require": ["non-destructive"]
  }
}`

// mkProfile builds a single-rule profile proving `non-destructive` over the
// /partitions value change. cel is the leaf predicate; msg is the leaf message
// (its only-wording lever for explanation-only); eff is the onFailure effect.
func mkProfile(name, cel, msg string, eff policy.Effect) Profile {
	return Profile{
		Name: name,
		Policy: &policy.MergePolicy{
			Spec: policy.MergePolicySpec{
				Rules: []policy.Rule{{
					Name:      "non-destructive",
					Phase:     policy.PhaseEnforce,
					Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
					Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: cel, Message: msg}}},
					OnFailure: &policy.OnFailure{Effect: eff, Code: "partitions.shrunk"},
				}},
			},
		},
		Bind:    &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod", Class: "kafka-topic"},
		Ceiling: policy.PhaseEnforce,
	}
}

func loadBundle(t *testing.T) *aggregate.EvaluationInput {
	t.Helper()
	in, err := LoadBundle([]byte(bundleJSON))
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	return in
}

// REQ-E6-S09-01: one ReplayBundle under baseline + candidate yields two decisions
// whose difference classifies as exactly ONE closed-taxonomy kind. Here baseline
// BLOCKs the shrink and the candidate relaxes the guard to auto-mergeable APPROVE
// -> newly-auto-mergeable, and the widening gate FAILs.
func TestCompareClassifiesOneDelta(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod-strict@6", "new >= old", "", policy.EffectBlock)
	candidate := mkProfile("prod-strict@7", "true", "", policy.EffectBlock)

	got, err := Compare(in, baseline, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Baseline != aggregate.DecisionBlock {
		t.Fatalf("baseline decision = %q, want BLOCK", got.Baseline)
	}
	if got.Candidate != aggregate.DecisionApprove {
		t.Fatalf("candidate decision = %q, want APPROVE", got.Candidate)
	}
	if got.Kind != KindNewlyAutoMergeable {
		t.Fatalf("kind = %q, want %q", got.Kind, KindNewlyAutoMergeable)
	}
	if got.Gate != GateBoundedAutoMergeWidening {
		t.Fatalf("gate = %q, want %q", got.Gate, GateBoundedAutoMergeWidening)
	}
	if got.Verdict != VerdictFail {
		t.Fatalf("verdict = %q, want FAIL (a newly-auto-mergeable widening trips the gate)", got.Verdict)
	}
}

// REQ-E6-S09-03 (unit side): an explanation-only (wording-only) delta — the two
// profiles are identical except one failing leaf's message — reaches the same
// decision with identical finding identities and NEVER trips the gate.
func TestCompareExplanationOnlyNeverTripsGate(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod@6", "new >= old", "partitions must not shrink below the baseline", policy.EffectBlock)
	candidate := mkProfile("prod@7", "new >= old", "partitions may not be reduced", policy.EffectBlock)

	got, err := Compare(in, baseline, candidate)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Baseline != aggregate.DecisionBlock || got.Candidate != aggregate.DecisionBlock {
		t.Fatalf("both sides must BLOCK, got baseline=%q candidate=%q", got.Baseline, got.Candidate)
	}
	if got.Kind != KindExplanationOnly {
		t.Fatalf("kind = %q, want %q (only the leaf message differs)", got.Kind, KindExplanationOnly)
	}
	if got.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, want PASS (explanation-only never trips a promotion gate)", got.Verdict)
	}
}

// REQ-E6-S09-02: a difference matching none of the seed's classified kinds is a
// HARD classification error (fail-closed) — never an unclassified silent pass.
// Here baseline BLOCKs and the candidate downgrades the effect to require-review,
// so the delta is BLOCK -> REVIEW: a real, more-lenient move that is NOT the
// newly-auto-mergeable widening the seed classifies.
func TestCompareUnclassifiableDeltaFailsClosed(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod@6", "new >= old", "", policy.EffectBlock)
	candidate := mkProfile("prod@7", "new >= old", "", policy.EffectRequireReview)

	got, err := Compare(in, baseline, candidate)
	if err == nil {
		t.Fatalf("expected a fail-closed classification error, got a classified comparison: %+v", got)
	}
	if !errors.Is(err, ErrUnclassifiable) {
		t.Fatalf("error = %v, want it to wrap ErrUnclassifiable", err)
	}
}

// REQ-E6-S09-04: comparison is side-effect-free and double-runs byte-identical
// (no clock/env/net/random anywhere in the path).
func TestCompareDoubleRunStable(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod@6", "new >= old", "", policy.EffectBlock)
	candidate := mkProfile("prod@7", "true", "", policy.EffectBlock)

	first, err := Compare(in, baseline, candidate)
	if err != nil {
		t.Fatalf("Compare (1): %v", err)
	}
	second, err := Compare(loadBundle(t), baseline, candidate)
	if err != nil {
		t.Fatalf("Compare (2): %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("double run diverged:\n first=%+v\nsecond=%+v", first, second)
	}
}

// TestKindConstantsAreFrozenTaxonomy proves every exported Kind constant is
// exactly a member of the frozen comparison-record deltaKind enum (drift guard):
// a ComparisonRecord carrying a delta of that kind validates, and a bogus kind
// is rejected. This keeps the seed's taxonomy grounded in the schema without
// re-authoring it.
func TestKindConstantsAreFrozenTaxonomy(t *testing.T) {
	all := []Kind{
		KindStricterInterventionAdded,
		KindDestructiveOrAuthorizationInterventionMissed,
		KindSubjectOrObligationUncovered,
		KindNewlyAutoMergeable,
		KindScoreThresholdChange,
		KindExplanationOnly,
	}
	for _, k := range all {
		if err := validateComparisonRecordKind(t, string(k)); err != nil {
			t.Errorf("Kind %q rejected by frozen comparison-record schema: %v", k, err)
		}
	}
	if err := validateComparisonRecordKind(t, "other"); err == nil {
		t.Errorf("a non-taxonomy kind must be rejected by the frozen schema, was accepted")
	}
}

// TestGateIsFrozenSuiteConformant proves the seed's single gate — its gateId,
// the failOnKinds member it fires on, defaultVerdict and acceptance — is a valid
// row of the frozen comparison-suite promotionGates table.
func TestGateIsFrozenSuiteConformant(t *testing.T) {
	// The suite schema pins EXACTLY five gates; the seed evaluates only the
	// bounded-auto-merge-widening row, but a schema-valid suite needs all five
	// (authored here as data purely for the conformance check, not the runner).
	suite := map[string]any{
		"apiVersion": "assent.dev/v1alpha1",
		"kind":       "PolicyComparisonSuite",
		"metadata":   map[string]any{"name": "seed-conformance"},
		"spec": map[string]any{
			"cases": []any{map[string]any{"caseId": "c1", "replayBundleDigest": "sha256:aaaa"}},
			"promotionGates": []any{
				map[string]any{"gateId": string(GateBoundedAutoMergeWidening), "failOnKinds": []any{string(KindNewlyAutoMergeable)}, "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
				map[string]any{"gateId": "zero-missed-destructive", "failOnKinds": []any{string(KindDestructiveOrAuthorizationInterventionMissed)}, "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
				map[string]any{"gateId": "zero-missed-authorization-ownership", "failOnKinds": []any{string(KindDestructiveOrAuthorizationInterventionMissed)}, "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
				map[string]any{"gateId": "no-unexpected-obligation-removal", "failOnKinds": []any{string(KindSubjectOrObligationUncovered)}, "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
				map[string]any{"gateId": "explicitly-accepted-deltas", "failOnKinds": []any{string(KindStricterInterventionAdded), string(KindScoreThresholdChange)}, "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
			},
		},
	}
	if err := schemas.ComparisonSuiteSchema.Validate(suite); err != nil {
		t.Fatalf("seed gate row is not a frozen comparison-suite promotionGate: %v", err)
	}
}

func validateComparisonRecordKind(t *testing.T, kind string) error {
	t.Helper()
	rec := map[string]any{
		"apiVersion":       "assent.dev/v1alpha1",
		"kind":             "ComparisonRecord",
		"baselineProfile":  "b@1",
		"candidateProfile": "c@1",
		"caseId":           "c1",
		"deltas": []any{map[string]any{
			"kind":      kind,
			"rule":      "r",
			"subject":   "topic-registry:orders.events.v1",
			"baseline":  map[string]any{"present": true},
			"candidate": map[string]any{"present": false},
		}},
	}
	// Round-trip through JSON so numbers/shape match the validator's any-tree.
	raw, _ := json.Marshal(rec)
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	return schemas.ComparisonRecordSchema.Validate(doc)
}
