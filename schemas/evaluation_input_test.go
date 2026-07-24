package schemas

import (
	"os"
	"strings"
	"testing"
)

// REQ-P3-E1-S02-01: evaluation-input.schema.json carries the canonical
// ChangeSet (entries with EntryRef subjects, old/new values, positions), the
// resolved facts map (each fact typed resolved|unavailable|invalid|expired),
// mr metadata, and the binding's require: obligation list — with no field
// naming a predicate backend (cel, rego, or similar) anywhere in the schema.
func TestEvaluationInputSchema(t *testing.T) {
	t.Run("ADR-0017 shape with a resolved and an unavailable fact is valid", func(t *testing.T) {
		const doc = `{
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
						"new": 12,
						"position": {"startLine": 4, "startColumn": 3, "endLine": 4, "endColumn": 10}
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
				},
				"owner": {
					"team": {
						"state": "unavailable",
						"sensitive": true,
						"observedAt": "2026-07-21T10:00:00Z",
						"reason": "provider timeout"
					}
				}
			},
			"mr": {
				"author": "alice",
				"sourceBranch": "topic/orders-partitions",
				"targetBranch": "main",
				"labels": ["kafka"]
			},
			"require": ["non-destructive"]
		}`
		if err := validateJSON(EvaluationInputSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("empty require list is valid (no obligation-only pack)", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "EvaluationInput",
			"changeSet": {"changes": [{"subject": "file:topics/prod/orders.events.v1.yaml", "file": "topics/prod/orders.events.v1.yaml", "path": "", "kind": "modify"}]},
			"facts": {},
			"mr": {"author": "alice", "sourceBranch": "x", "targetBranch": "main"},
			"require": []
		}`
		if err := validateJSON(EvaluationInputSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("wrong kind is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "EvalInput",
			"changeSet": {"changes": [{"subject": "file:x", "file": "x", "path": "", "kind": "add"}]},
			"facts": {},
			"mr": {"author": "alice", "sourceBranch": "x", "targetBranch": "main"},
			"require": []
		}`
		if err := validateJSON(EvaluationInputSchema, doc); err == nil {
			t.Fatal("expected wrong kind to fail validation")
		}
	})

	t.Run("adversarial: resolved fact without value is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "EvaluationInput",
			"changeSet": {"changes": [{"subject": "file:x", "file": "x", "path": "", "kind": "add"}]},
			"facts": {
				"quota": {"max_partitions": {"state": "resolved", "sensitive": false, "observedAt": "2026-07-21T10:00:00Z", "expiresAt": "2026-07-21T11:00:00Z"}}
			},
			"mr": {"author": "alice", "sourceBranch": "x", "targetBranch": "main"},
			"require": []
		}`
		if err := validateJSON(EvaluationInputSchema, doc); err == nil {
			t.Fatal("expected resolved fact without value to fail validation")
		}
	})

	t.Run("adversarial: non-resolved fact carrying a value is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "EvaluationInput",
			"changeSet": {"changes": [{"subject": "file:x", "file": "x", "path": "", "kind": "add"}]},
			"facts": {
				"quota": {"max_partitions": {"state": "expired", "sensitive": false, "observedAt": "2026-07-21T10:00:00Z", "value": 24, "reason": "stale"}}
			},
			"mr": {"author": "alice", "sourceBranch": "x", "targetBranch": "main"},
			"require": []
		}`
		if err := validateJSON(EvaluationInputSchema, doc); err == nil {
			t.Fatal("expected a non-resolved fact carrying a value to fail validation")
		}
	})

	t.Run("adversarial: change entry with a cel field is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "EvaluationInput",
			"changeSet": {"changes": [{"subject": "file:x", "file": "x", "path": "", "kind": "add", "cel": "true"}]},
			"facts": {},
			"mr": {"author": "alice", "sourceBranch": "x", "targetBranch": "main"},
			"require": []
		}`
		if err := validateJSON(EvaluationInputSchema, doc); err == nil {
			t.Fatal("expected a change entry naming a predicate backend field (cel) to fail validation")
		}
	})

	t.Run("adversarial: missing require is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "EvaluationInput",
			"changeSet": {"changes": [{"subject": "file:x", "file": "x", "path": "", "kind": "add"}]},
			"facts": {},
			"mr": {"author": "alice", "sourceBranch": "x", "targetBranch": "main"}
		}`
		if err := validateJSON(EvaluationInputSchema, doc); err == nil {
			t.Fatal("expected missing require to fail validation")
		}
	})

	t.Run("REQ-P3-E1-S02-01: schema text names no predicate backend (backend neutrality)", func(t *testing.T) {
		raw, err := os.ReadFile("decision/v1alpha1/evaluation-input.schema.json")
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ToLower(string(raw))
		for _, backend := range []string{`"cel"`, `"rego"`} {
			if strings.Contains(text, backend) {
				t.Fatalf("evaluation-input.schema.json must not name a predicate backend field, found %s", backend)
			}
		}
	})

	// REQ-P3-E4-S05-03: phase + profile are typed EvaluationInput fields
	// (optional so pre-lifecycle fixtures remain valid; never prose-inferred).
	t.Run("REQ-P3-E4-S05-03: phase and profile typed fields are valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "EvaluationInput",
			"phase": "enforce",
			"profile": {"name": "prod-strict", "revision": 7},
			"changeSet": {"changes": [{"subject": "file:x", "file": "x", "path": "", "kind": "add"}]},
			"facts": {},
			"mr": {"author": "alice", "sourceBranch": "x", "targetBranch": "main"},
			"require": []
		}`
		if err := validateJSON(EvaluationInputSchema, doc); err != nil {
			t.Fatalf("expected phase+profile EvaluationInput valid, got: %v", err)
		}
	})

	t.Run("adversarial: unknown phase enum on EvaluationInput is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "EvaluationInput",
			"phase": "shadow",
			"profile": {"name": "prod-strict"},
			"changeSet": {"changes": [{"subject": "file:x", "file": "x", "path": "", "kind": "add"}]},
			"facts": {},
			"mr": {"author": "alice", "sourceBranch": "x", "targetBranch": "main"},
			"require": []
		}`
		if err := validateJSON(EvaluationInputSchema, doc); err == nil {
			t.Fatal("expected unknown phase enum to fail validation")
		}
	})

	t.Run("adversarial: profile missing name is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "EvaluationInput",
			"phase": "observe",
			"profile": {"revision": 1},
			"changeSet": {"changes": [{"subject": "file:x", "file": "x", "path": "", "kind": "add"}]},
			"facts": {},
			"mr": {"author": "alice", "sourceBranch": "x", "targetBranch": "main"},
			"require": []
		}`
		if err := validateJSON(EvaluationInputSchema, doc); err == nil {
			t.Fatal("expected profile without name to fail validation")
		}
	})
}
