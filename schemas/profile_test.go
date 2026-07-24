package schemas

import "testing"

// REQ-P3-E4-S02-01: PolicyProfile requires an explicit writes boolean; missing
// or non-boolean values are hard schema errors. Single-writer across profiles
// for the same (environment, class) binding is a lint invariant (documented),
// not a per-document schema constraint.
func TestPolicyProfileWritesRequired(t *testing.T) {
	t.Run("profile with writes true is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyProfile",
			"metadata": {"name": "prod-writer"},
			"spec": {
				"writes": true,
				"environments": ["prod"],
				"classes": ["kafka-topic"]
			}
		}`
		if err := validateJSON(ProfileSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("recorder-only profile with writes false is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyProfile",
			"metadata": {"name": "candidate-shadow"},
			"spec": {
				"writes": false,
				"environments": ["*"],
				"classes": ["*"],
				"packs": ["topics", "topics-strict"]
			}
		}`
		if err := validateJSON(ProfileSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("adversarial: missing writes is invalid (no silent default)", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyProfile",
			"metadata": {"name": "undecorated"},
			"spec": {
				"environments": ["prod"],
				"classes": ["kafka-topic"]
			}
		}`
		if err := validateJSON(ProfileSchema, doc); err == nil {
			t.Fatal("expected a profile missing writes to fail validation")
		}
	})

	t.Run("adversarial: non-boolean writes is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyProfile",
			"metadata": {"name": "bad-writes"},
			"spec": {
				"writes": "yes",
				"environments": ["prod"],
				"classes": ["kafka-topic"]
			}
		}`
		if err := validateJSON(ProfileSchema, doc); err == nil {
			t.Fatal("expected non-boolean writes to fail validation")
		}
	})

	t.Run("adversarial: missing scope environments is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyProfile",
			"metadata": {"name": "no-env"},
			"spec": {
				"writes": true,
				"classes": ["kafka-topic"]
			}
		}`
		if err := validateJSON(ProfileSchema, doc); err == nil {
			t.Fatal("expected profile missing environments to fail validation")
		}
	})

	t.Run("adversarial: unknown top-level field is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyProfile",
			"metadata": {"name": "bogus"},
			"spec": {
				"writes": false,
				"environments": ["*"],
				"classes": ["*"]
			},
			"bogus": true
		}`
		if err := validateJSON(ProfileSchema, doc); err == nil {
			t.Fatal("expected unknown top-level field to fail validation")
		}
	})
}

// Config.profiles is the schema-level precedence-table artifact (ordered
// PolicyProfile name refs). Optional so pre-profile configs remain valid.
func TestConfigProfilesPrecedenceArtifact(t *testing.T) {
	t.Run("config with ordered profiles precedence is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "Config",
			"environments": [{"name": "prod", "match": {"paths": ["topics/prod/**"]}}],
			"classes": [{"name": "kafka-topic", "match": {"paths": ["topics/**"]}}],
			"profiles": [
				{"name": "prod-strict"},
				{"name": "default-writer"}
			]
		}`
		if err := validateJSON(ConfigSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("adversarial: duplicate profile name in precedence table is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "Config",
			"environments": [{"name": "dev", "match": {"paths": ["**"]}}],
			"classes": [{"name": "kafka-topic", "match": {"paths": ["topics/**"]}}],
			"profiles": [
				{"name": "default-writer"},
				{"name": "default-writer"}
			]
		}`
		if err := validateJSON(ConfigSchema, doc); err == nil {
			t.Fatal("expected duplicate profile name in Config.profiles to fail validation")
		}
	})
}
