package schemas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// REQ-P3-E4-S03-01: ComparisonRecord carries a closed six-member delta
// taxonomy enum — no "other"/free-text kind. Unknown kinds reject.
func TestComparisonRecordClosedTaxonomy(t *testing.T) {
	t.Run("all six taxonomy members are individually valid", func(t *testing.T) {
		kinds := []string{
			"stricter-intervention-added",
			"destructive-or-authorization-intervention-missed",
			"subject-or-obligation-uncovered",
			"newly-auto-mergeable",
			"score-threshold-change",
			"explanation-only",
		}
		for _, kind := range kinds {
			doc := comparisonRecordDoc(kind)
			if err := validateJSON(ComparisonRecordSchema, doc); err != nil {
				t.Fatalf("kind %q should be valid: %v", kind, err)
			}
		}
	})

	t.Run("adversarial: unknown delta kind is rejected (no other/free-text)", func(t *testing.T) {
		doc := comparisonRecordDoc("other")
		if err := validateJSON(ComparisonRecordSchema, doc); err == nil {
			t.Fatal("expected unknown delta kind \"other\" to be rejected")
		}
	})

	t.Run("adversarial: free-text delta kind is rejected", func(t *testing.T) {
		doc := comparisonRecordDoc("somehow-safer-i-promise")
		if err := validateJSON(ComparisonRecordSchema, doc); err == nil {
			t.Fatal("expected free-text delta kind to be rejected")
		}
	})

	t.Run("schema freezes the closed enum and fail-closed members", func(t *testing.T) {
		raw := readComparisonSchemaFile(t, "comparison/v1alpha1/comparison-record.schema.json")
		for _, needle := range []string{
			`"enum"`,
			"destructive-or-authorization-intervention-missed",
			"explanation-only",
		} {
			if !strings.Contains(raw, needle) {
				t.Errorf("comparison-record.schema.json must contain %q", needle)
			}
		}
	})
}

// REQ-P3-E4-S03-02: wording-only message-template changes classify as
// explanation-only; the schema documents that rendered-message changes never
// enter semantic promotion gates (ADR-0014 amendment).
func TestComparisonRecordMessageWordingIsExplanationOnly(t *testing.T) {
	t.Run("explanation-only delta with per-delta identity is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ComparisonRecord",
			"baselineProfile": "prod-strict@6",
			"candidateProfile": "prod-strict@7",
			"caseId": "topic-delete-refuse",
			"deltas": [
				{
					"kind": "explanation-only",
					"rule": "no-topic-delete",
					"obligation": "non-destructive",
					"subject": "topic-registry:orders.events.v1",
					"baseline": {"decision": "BLOCK", "effect": "block", "present": true},
					"candidate": {"decision": "BLOCK", "effect": "block", "present": true}
				}
			]
		}`
		if err := validateJSON(ComparisonRecordSchema, doc); err != nil {
			t.Fatalf("expected valid explanation-only record, got: %v", err)
		}
	})

	t.Run("schema documents message wording → explanation-only", func(t *testing.T) {
		raw := readComparisonSchemaFile(t, "comparison/v1alpha1/comparison-record.schema.json")
		if !strings.Contains(strings.ToLower(raw), "message") {
			t.Fatal("comparison-record.schema.json must mention message (wording-only → explanation-only)")
		}
	})
}

// REQ-P3-E4-S03-03: PolicyComparisonSuite pins stable caseIds and an
// immutable-corpus invariant (ReplayBundle content hash per caseId).
func TestPolicyComparisonSuiteImmutableCorpus(t *testing.T) {
	t.Run("suite with unique caseIds and digests is valid", func(t *testing.T) {
		doc := validComparisonSuiteDoc()
		if err := validateJSON(ComparisonSuiteSchema, doc); err != nil {
			t.Fatalf("expected valid suite, got: %v", err)
		}
	})

	t.Run("adversarial: duplicate caseId is rejected", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyComparisonSuite",
			"metadata": {"name": "lifecycle-corpus", "version": "1"},
			"spec": {
				"cases": [
					{"caseId": "same", "replayBundleDigest": "sha256:aaa"},
					{"caseId": "same", "replayBundleDigest": "sha256:bbb"}
				],
				"promotionGates": ` + canonicalPromotionGatesJSON + `,
				"acceptedDeltas": []
			}
		}`
		if err := validateJSON(ComparisonSuiteSchema, doc); err == nil {
			t.Fatal("expected duplicate caseId to be rejected")
		}
	})

	t.Run("schema freezes caseId + immutable corpus wording", func(t *testing.T) {
		raw := readComparisonSchemaFile(t, "comparison/v1alpha1/comparison-suite.schema.json")
		if !strings.Contains(raw, `"caseId"`) {
			t.Error(`comparison-suite.schema.json must contain "caseId"`)
		}
		if !strings.Contains(strings.ToLower(raw), "immutable") {
			t.Error("comparison-suite.schema.json must document the immutable corpus invariant")
		}
	})
}

// REQ-P3-E4-S03-04 (schema half): promotion gates are a schema-level
// pass/fail table keyed by delta kind; acceptedDeltas allowlist is keyed by
// caseId + delta identity (never by kind alone).
func TestPolicyComparisonSuitePromotionGates(t *testing.T) {
	t.Run("acceptedDeltas require caseId + identity (not kind alone)", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyComparisonSuite",
			"metadata": {"name": "lifecycle-corpus", "version": "1"},
			"spec": {
				"cases": [
					{"caseId": "c1", "replayBundleDigest": "sha256:aaa"}
				],
				"promotionGates": ` + canonicalPromotionGatesJSON + `,
				"acceptedDeltas": [
					{
						"caseId": "c1",
						"kind": "newly-auto-mergeable",
						"rule": "partition-increase-within-quota",
						"subject": "topic-registry:orders.events.v1",
						"rationale": "widening accepted for this case after review"
					}
				]
			}
		}`
		if err := validateJSON(ComparisonSuiteSchema, doc); err != nil {
			t.Fatalf("expected valid acceptedDeltas entry, got: %v", err)
		}
	})

	t.Run("adversarial: acceptedDeltas entry missing identity is rejected", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyComparisonSuite",
			"metadata": {"name": "lifecycle-corpus", "version": "1"},
			"spec": {
				"cases": [
					{"caseId": "c1", "replayBundleDigest": "sha256:aaa"}
				],
				"promotionGates": ` + canonicalPromotionGatesJSON + `,
				"acceptedDeltas": [
					{
						"caseId": "c1",
						"kind": "destructive-or-authorization-intervention-missed",
						"rationale": "accept all of this kind — footgun"
					}
				]
			}
		}`
		if err := validateJSON(ComparisonSuiteSchema, doc); err == nil {
			t.Fatal("expected acceptedDeltas without rule/subject identity to be rejected (never by kind alone)")
		}
	})

	t.Run("destructive-or-authorization-intervention-missed is a failOnKinds member", func(t *testing.T) {
		raw := readComparisonSchemaFile(t, "comparison/v1alpha1/comparison-suite.schema.json")
		if !strings.Contains(raw, "destructive-or-authorization-intervention-missed") {
			t.Fatal("promotion gates table must key off destructive-or-authorization-intervention-missed")
		}
		if !strings.Contains(raw, "acceptedDeltas") {
			t.Fatal("suite schema must define acceptedDeltas")
		}
	})
}

// REQ-P3-E4-S03-04 (doc half): planning doc names promotion gates + acceptedDeltas.
func TestPromotionGatesPlanningDoc(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "docs", "planning", "policy-lifecycle-promotion-gates.md")) //nolint:gosec // fixed planning doc path
	if err != nil {
		t.Fatalf("read promotion-gates planning doc: %v", err)
	}
	body := strings.ToLower(string(raw))
	if !strings.Contains(body, "promotion gate") {
		t.Error("docs/planning/policy-lifecycle-promotion-gates.md must mention promotion gate")
	}
	if !strings.Contains(body, "accepteddeltas") {
		t.Error("docs/planning/policy-lifecycle-promotion-gates.md must mention acceptedDeltas")
	}
}

func comparisonRecordDoc(kind string) string {
	return `{
		"apiVersion": "assent.dev/v1alpha1",
		"kind": "ComparisonRecord",
		"baselineProfile": "prod-strict@6",
		"candidateProfile": "prod-strict@7",
		"caseId": "case-a",
		"deltas": [
			{
				"kind": "` + kind + `",
				"rule": "no-topic-delete",
				"subject": "topic-registry:orders.events.v1",
				"baseline": {"decision": "BLOCK", "effect": "block", "present": true},
				"candidate": {"decision": "APPROVE", "present": false}
			}
		]
	}`
}

func validComparisonSuiteDoc() string {
	return `{
		"apiVersion": "assent.dev/v1alpha1",
		"kind": "PolicyComparisonSuite",
		"metadata": {"name": "lifecycle-corpus", "version": "1"},
		"spec": {
			"baselineProfile": "prod-strict",
			"candidateProfile": "prod-strict-candidate",
			"cases": [
				{
					"caseId": "topic-delete-refuse",
					"replayBundleDigest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
					"replayBundleRef": "fixtures/topic-delete-refuse/replay-bundle.json"
				},
				{
					"caseId": "partition-widen",
					"replayBundleDigest": "sha256:2222222222222222222222222222222222222222222222222222222222222222"
				}
			],
			"promotionGates": ` + canonicalPromotionGatesJSON + `,
			"acceptedDeltas": []
		}
	}`
}

// canonicalPromotionGatesJSON is the five-gate pass/fail table required by
// every PolicyComparisonSuite (P3-E4-S03-04). Kept as a shared fixture string
// so tests and the planning doc stay aligned.
const canonicalPromotionGatesJSON = `[
	{
		"gateId": "zero-missed-destructive",
		"failOnKinds": ["destructive-or-authorization-intervention-missed"],
		"defaultVerdict": "fail",
		"acceptance": "per-delta-identity"
	},
	{
		"gateId": "zero-missed-authorization-ownership",
		"failOnKinds": ["destructive-or-authorization-intervention-missed"],
		"defaultVerdict": "fail",
		"acceptance": "per-delta-identity"
	},
	{
		"gateId": "no-unexpected-obligation-removal",
		"failOnKinds": ["subject-or-obligation-uncovered"],
		"defaultVerdict": "fail",
		"acceptance": "per-delta-identity"
	},
	{
		"gateId": "bounded-auto-merge-widening",
		"failOnKinds": ["newly-auto-mergeable"],
		"defaultVerdict": "fail",
		"acceptance": "per-delta-identity"
	},
	{
		"gateId": "explicitly-accepted-deltas",
		"failOnKinds": ["stricter-intervention-added", "score-threshold-change"],
		"defaultVerdict": "fail",
		"acceptance": "per-delta-identity"
	}
]`

// REQ-PCS-S05-03: acceptedDeltas without rule/subject identity is rejected at
// suite load (never accept by kind alone).
func TestComparisonSuiteAcceptedDeltaRequiresIdentity(t *testing.T) {
	doc := `{
		"apiVersion": "assent.dev/v1alpha1",
		"kind": "PolicyComparisonSuite",
		"metadata": {"name": "lifecycle-corpus", "version": "1"},
		"spec": {
			"cases": [
				{"caseId": "c1", "replayBundleDigest": "sha256:aaa"}
			],
			"promotionGates": ` + canonicalPromotionGatesJSON + `,
			"acceptedDeltas": [
				{
					"caseId": "c1",
					"kind": "destructive-or-authorization-intervention-missed",
					"rationale": "accept all of this kind — footgun"
				}
			]
		}
	}`
	if err := validateJSON(ComparisonSuiteSchema, doc); err == nil {
		t.Fatal("expected acceptedDeltas without rule/subject identity to be rejected (never by kind alone)")
	}
}

func readComparisonSchemaFile(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(rel)) //nolint:gosec // test reads fixed schema tree
	if err != nil {
		t.Fatalf("read schema %s: %v", rel, err)
	}
	return string(raw)
}
