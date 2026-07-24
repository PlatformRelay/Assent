package schemas

import "testing"

// REQ-P3-E1-S01-03: merge-policy.schema.json restricts each rules[] entry's
// match to exactly one of the four matcher-domain shapes (files,
// values.pointers, fileEvents, valueChanges — no generic path overload);
// requires either an obligation-proving rule (prove + onFailure, effect never
// a bare "vouch") or a non-obligation effect rule (comment/challenge/block)
// with no prove; composes assert/when trees via all/any/not over CEL-string
// leaves (a bare string is single-leaf shorthand); and requires entries
// (mode/root/identity.pointer) wherever a class declares governed subjects,
// rejecting unkeyed list mode without identity.pointer.
func TestMergePolicySchema(t *testing.T) {
	t.Run("obligation-proving rule with values.pointers match is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"entries": {
					"kafka-topic": {"mode": "map", "root": "/topics"}
				},
				"rules": [
					{
						"name": "partition-increase-within-quota",
						"phase": "enforce",
						"match": {"values": {"pointers": ["/partitions"]}},
						"prove": {
							"obligation": "non-destructive",
							"when": "new >= old && new <= facts.quota.max_partitions"
						},
						"onFailure": {"effect": "challenge", "code": "partition-quota-exceeded"}
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("non-obligation bare-effect rule with fileEvents match is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "no-topic-deletion",
						"phase": "enforce",
						"match": {"fileEvents": {"paths": ["topics/**"], "kinds": ["delete", "rename"]}},
						"effect": "block",
						"message": "Topic deletion is never auto-mergeable."
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("full-form assert tree with all/any/not combinators is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "cross-field-consistency",
						"phase": "enforce",
						"match": {"values": {"pointers": ["/config/cleanup.policy", "/retentionMs"]}},
						"prove": {
							"obligation": "consistent-retention",
							"when": {
								"any": [
									{"cel": "entry.config[\"cleanup.policy\"] == oldEntry.config[\"cleanup.policy\"]"},
									{
										"cel": "entry.retentionMs == oldEntry.retentionMs",
										"message": "cleanup.policy flip combined with a retention change"
									}
								]
							}
						},
						"onFailure": {"effect": "require-review", "code": "policy-flip-with-retention-change"}
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("entries list mode with identity.pointer is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"entries": {
					"kafka-topic": {"mode": "list", "root": "/topics", "identity": {"pointer": "/name"}}
				},
				"rules": [
					{
						"name": "no-topic-deletion",
						"phase": "enforce",
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

	t.Run("adversarial: match with two matcher domains is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "ambiguous-match",
						"phase": "enforce",
						"match": {"files": {"paths": ["topics/**"]}, "values": {"pointers": ["/partitions"]}},
						"effect": "comment"
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err == nil {
			t.Fatal("expected match with two matcher domains to fail validation")
		}
	})

	t.Run("adversarial: generic path overload is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "legacy-path-match",
						"phase": "enforce",
						"match": {"path": "**/partitions"},
						"effect": "comment"
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err == nil {
			t.Fatal("expected the generic path overload to fail validation")
		}
	})

	t.Run("adversarial: prove rule with bare effect vouch is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "partition-increase-within-quota",
						"phase": "enforce",
						"match": {"values": {"pointers": ["/partitions"]}},
						"prove": {"obligation": "non-destructive", "when": "new >= old"},
						"effect": "vouch"
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err == nil {
			t.Fatal("expected a rule with both prove and effect vouch to fail validation")
		}
	})

	t.Run("adversarial: bare effect vouch with no prove is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "legacy-vouch",
						"phase": "enforce",
						"match": {"values": {"pointers": ["/partitions"]}},
						"effect": "vouch"
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err == nil {
			t.Fatal("expected bare effect: vouch to fail validation")
		}
	})

	t.Run("adversarial: rule with neither prove nor effect is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "no-outcome",
						"phase": "enforce",
						"match": {"values": {"pointers": ["/partitions"]}}
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err == nil {
			t.Fatal("expected a rule with neither prove nor effect to fail validation")
		}
	})

	t.Run("adversarial: onFailure with invalid effect enum is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"rules": [
					{
						"name": "partition-increase-within-quota",
						"phase": "enforce",
						"match": {"values": {"pointers": ["/partitions"]}},
						"prove": {"obligation": "non-destructive", "when": "new >= old"},
						"onFailure": {"effect": "vouch", "code": "x"}
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err == nil {
			t.Fatal("expected onFailure.effect: vouch to fail validation")
		}
	})

	t.Run("adversarial: entries list mode without identity.pointer is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"entries": {
					"kafka-topic": {"mode": "list", "root": "/topics"}
				},
				"rules": [
					{
						"name": "no-topic-deletion",
						"phase": "enforce",
						"match": {"files": {"paths": ["topics/**"]}},
						"effect": "comment"
					}
				]
			}
		}`
		if err := validateJSON(MergePolicySchema, doc); err == nil {
			t.Fatal("expected unkeyed list-mode entries to fail validation")
		}
	})

	t.Run("entries document mode without identity.pointer is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "MergePolicy",
			"metadata": {"name": "topic-safety"},
			"spec": {
				"entries": {
					"kafka-topic": {"mode": "document", "root": ""}
				},
				"rules": [
					{
						"name": "no-topic-deletion",
						"phase": "enforce",
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
}
