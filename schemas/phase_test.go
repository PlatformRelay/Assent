package schemas

import "testing"

// REQ-P3-E4-S01-01: MergePolicy rules require an explicit phase enum
// off|observe|enforce — no default; missing or unknown values are hard errors.
func TestMergePolicyPhaseRequired(t *testing.T) {
	t.Run("rule with phase enforce is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "no-topic-deletion",
						"phase": "enforce",
						"match": {"fileEvents": {"paths": ["topics/**"], "kinds": ["delete"]}},
						"effect": "block"
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("rule with phase observe is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "shadow-block",
						"phase": "observe",
						"match": {"files": {"paths": ["topics/**"]}},
						"effect": "block"
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("rule with phase off is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "parked-rule",
						"phase": "off",
						"match": {"files": {"paths": ["topics/**"]}},
						"effect": "comment"
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("adversarial: missing phase is invalid (no silent default)", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "undecorated",
						"match": {"files": {"paths": ["topics/**"]}},
						"effect": "block"
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err == nil {
			t.Fatal("expected a rule missing phase to fail validation")
		}
	})

	t.Run("adversarial: unknown phase enum is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "bad-phase",
						"phase": "shadow",
						"match": {"files": {"paths": ["topics/**"]}},
						"effect": "block"
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err == nil {
			t.Fatal("expected unknown phase enum to fail validation")
		}
	})
}

// REQ-P3-E4-S01-02: DecisionRecord findings split into observed + enforcing;
// only enforcing findings are aggregation inputs (schema-level structural split).
func TestDecisionRecordFindingsObservedEnforcingSplit(t *testing.T) {
	const pins = `{
		"toolVersion": "0.1.0",
		"toolDigest": "sha256:aaaa",
		"policySha": "sha256:bbbb",
		"sourceSha": "cccc",
		"targetSha": "dddd",
		"mergeResultDigest": "sha256:eeee",
		"factsResolvedAt": {}
	}`

	t.Run("findings with observed and enforcing arrays is valid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "APPROVE",
			"findings": {
				"observed": [
					{"rule": "shadow-block", "effect": "block", "subject": "topic-registry:orders.events.v1", "points": 10, "code": "would-block"}
				],
				"enforcing": [
					{"rule": "partition-ok", "effect": "comment", "subject": "topic-registry:orders.events.v1", "points": 0, "code": "ok"}
				]
			},
			"pins": ` + pins + `
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("empty observed and enforcing arrays is valid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "APPROVE",
			"findings": {"observed": [], "enforcing": []},
			"pins": ` + pins + `
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("adversarial: legacy flat findings array is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "APPROVE",
			"findings": [
				{"rule": "x", "effect": "comment", "subject": "file:x", "points": 0}
			],
			"pins": ` + pins + `
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err == nil {
			t.Fatal("expected flat findings array to fail validation after observed/enforcing split")
		}
	})

	t.Run("adversarial: findings missing observed is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "APPROVE",
			"findings": {"enforcing": []},
			"pins": ` + pins + `
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err == nil {
			t.Fatal("expected findings missing observed to fail validation")
		}
	})

	t.Run("adversarial: findings missing enforcing is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "APPROVE",
			"findings": {"observed": []},
			"pins": ` + pins + `
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err == nil {
			t.Fatal("expected findings missing enforcing to fail validation")
		}
	})
}

// REQ-P3-E4-S01-05: pack.schema.json requires phase as a ceiling enum
// off|observe|enforce — no default.
func TestPackPhaseCeiling(t *testing.T) {
	t.Run("pack with phase observe ceiling is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "Pack",
			"metadata": {"name": "topics"},
			"spec": {
				"phase": "observe",
				"version": "1.0.0",
				"description": "Kafka topic safety pack"
			}
		}`
		if err := validateJSON(PackSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("pack with each ceiling phase is valid", func(t *testing.T) {
		for _, phase := range []string{"off", "observe", "enforce"} {
			doc := `{
				"apiVersion": "assent.dev/v1alpha1",
				"kind": "Pack",
				"metadata": {"name": "topics"},
				"spec": {"phase": "` + phase + `"}
			}`
			if err := validateJSON(PackSchema, doc); err != nil {
				t.Fatalf("expected phase %q valid, got: %v", phase, err)
			}
		}
	})

	t.Run("adversarial: missing pack phase is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "Pack",
			"metadata": {"name": "topics"},
			"spec": {"version": "1.0.0"}
		}`
		if err := validateJSON(PackSchema, doc); err == nil {
			t.Fatal("expected pack missing phase to fail validation")
		}
	})

	t.Run("adversarial: unknown pack phase is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "Pack",
			"metadata": {"name": "topics"},
			"spec": {"phase": "shadow"}
		}`
		if err := validateJSON(PackSchema, doc); err == nil {
			t.Fatal("expected unknown pack phase to fail validation")
		}
	})
}
