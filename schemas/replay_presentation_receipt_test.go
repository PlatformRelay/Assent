package schemas

import "testing"

// REQ-P3-E1-S02-03: replay-bundle.schema.json requires the full unredacted
// EvaluationInput plus every resolved fact value (hermetic replay);
// presentation-model.schema.json mirrors DecisionRecord's findings without
// any raw fact value; rendered markdown is not a declared property (renderers
// ignore unknown top-level fields under ADR-0017 §9 additive tolerance);
// publication-receipt.schema.json records forge operations performed (kind,
// target ids, timestamps) and nested operation shapes carry no secrets, no
// raw policy expressions, and no user-controlled Markdown.
func TestReplayPresentationReceiptSchemas(t *testing.T) {
	const validEvaluationInput = `{
		"apiVersion": "assent.dev/v1alpha1",
		"kind": "EvaluationInput",
		"changeSet": {
			"changes": [
				{
					"subject": "topic-registry:orders.events.v1",
					"file": "topics/prod/orders.events.v1.yaml",
					"path": "/partitions",
					"kind": "modify",
					"old": 6,
					"new": 12
				}
			]
		},
		"facts": {
			"quota": {
				"max_partitions": {
					"state": "resolved",
					"sensitive": false,
					"observedAt": "2026-07-21T10:00:00Z",
					"expiresAt": "2026-07-21T11:00:00Z",
					"value": 24
				}
			}
		},
		"mr": {"author": "alice", "sourceBranch": "topic/orders-partitions", "targetBranch": "main"},
		"require": ["non-destructive"]
	}`

	const validPins = `{
		"toolVersion": "0.1.0",
		"toolDigest": "sha256:aaaa",
		"policySha": "sha256:bbbb",
		"sourceSha": "cccc",
		"targetSha": "dddd",
		"mergeResultDigest": "sha256:eeee",
		"factsResolvedAt": {"quota": "2026-07-21T10:00:00Z"}
	}`

	t.Run("ReplayBundle", func(t *testing.T) {
		t.Run("full unredacted EvaluationInput plus pins is valid", func(t *testing.T) {
			doc := `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "ReplayBundle",
				"pins": ` + validPins + `,
				"evaluationInput": ` + validEvaluationInput + `
			}`
			if err := validateJSON(ReplayBundleSchema, doc); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})

		t.Run("adversarial: missing evaluationInput is invalid", func(t *testing.T) {
			doc := `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "ReplayBundle",
				"pins": ` + validPins + `
			}`
			if err := validateJSON(ReplayBundleSchema, doc); err == nil {
				t.Fatal("expected missing evaluationInput to fail validation")
			}
		})

		t.Run("adversarial: an evaluationInput without a resolved fact's value is invalid (not hermetic)", func(t *testing.T) {
			doc := `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "ReplayBundle",
				"pins": ` + validPins + `,
				"evaluationInput": {
					"apiVersion": "assent.dev/v1alpha1",
					"kind": "EvaluationInput",
					"changeSet": {"changes": [{"subject": "file:x", "file": "x", "path": "", "kind": "add"}]},
					"facts": {"quota": {"max_partitions": {"state": "resolved", "sensitive": false, "observedAt": "2026-07-21T10:00:00Z", "expiresAt": "2026-07-21T11:00:00Z"}}},
					"mr": {"author": "alice", "sourceBranch": "x", "targetBranch": "main"},
					"require": []
				}
			}`
			if err := validateJSON(ReplayBundleSchema, doc); err == nil {
				t.Fatal("expected a resolved fact missing its value to fail validation")
			}
		})
	})

	t.Run("PresentationModel", func(t *testing.T) {
		t.Run("mirrors DecisionRecord findings is valid", func(t *testing.T) {
			const doc = `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "PresentationModel",
				"decision": "REVIEW",
				"findings": [
					{"rule": "partition-increase-within-quota", "obligation": "non-destructive", "effect": "comment", "subject": "topic-registry:orders.events.v1", "points": 0, "code": "partition-quota-ok"}
				]
			}`
			if err := validateJSON(PresentationModelSchema, doc); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})

		t.Run("adversarial: finding carrying a raw fact value field is invalid", func(t *testing.T) {
			const doc = `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "PresentationModel",
				"decision": "BLOCK",
				"findings": [
					{"rule": "x", "effect": "block", "subject": "file:x", "points": 10, "value": 24}
				]
			}`
			if err := validateJSON(PresentationModelSchema, doc); err == nil {
				t.Fatal("expected a finding carrying a raw value field to fail validation")
			}
		})

		t.Run("top-level unknown field is additive-tolerant (ADR-0017 §9)", func(t *testing.T) {
			// Rendered markdown is not a declared property (ADR-0016 §4);
			// under additive-tolerant reports an unknown top-level field is
			// preserved rather than rejected — renderers must ignore it.
			const doc = `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "PresentationModel",
				"decision": "BLOCK",
				"findings": [],
				"markdown": "**bold**"
			}`
			if err := validateJSON(PresentationModelSchema, doc); err != nil {
				t.Fatalf("expected unknown top-level field to be additive-tolerant, got: %v", err)
			}
		})

		t.Run("adversarial: invalid decision enum is invalid", func(t *testing.T) {
			const doc = `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "PresentationModel",
				"decision": "AUTO_MERGE",
				"findings": []
			}`
			if err := validateJSON(PresentationModelSchema, doc); err == nil {
				t.Fatal("expected invalid decision enum to fail validation")
			}
		})
	})

	t.Run("PublicationReceipt", func(t *testing.T) {
		t.Run("thread/approval/merge operations are valid", func(t *testing.T) {
			const doc = `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "PublicationReceipt",
				"operations": [
					{"kind": "thread", "targetId": "note/123", "performedAt": "2026-07-21T10:00:00Z"},
					{"kind": "approval", "targetId": "approval/456", "performedAt": "2026-07-21T10:05:00Z"},
					{"kind": "merge", "targetId": "mr/789", "performedAt": "2026-07-21T10:06:00Z"}
				]
			}`
			if err := validateJSON(PublicationReceiptSchema, doc); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})

		t.Run("adversarial: empty operations is invalid", func(t *testing.T) {
			const doc = `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "PublicationReceipt",
				"operations": []
			}`
			if err := validateJSON(PublicationReceiptSchema, doc); err == nil {
				t.Fatal("expected empty operations[] to fail validation")
			}
		})

		t.Run("adversarial: invalid operation kind is invalid", func(t *testing.T) {
			const doc = `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "PublicationReceipt",
				"operations": [{"kind": "webhook", "targetId": "x", "performedAt": "2026-07-21T10:00:00Z"}]
			}`
			if err := validateJSON(PublicationReceiptSchema, doc); err == nil {
				t.Fatal("expected an invalid operation kind to fail validation")
			}
		})

		t.Run("adversarial: a secret field on an operation is invalid", func(t *testing.T) {
			const doc = `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "PublicationReceipt",
				"operations": [{"kind": "approval", "targetId": "x", "performedAt": "2026-07-21T10:00:00Z", "token": "shh"}]
			}`
			if err := validateJSON(PublicationReceiptSchema, doc); err == nil {
				t.Fatal("expected a secret-shaped field on an operation to fail validation")
			}
		})
	})
}
