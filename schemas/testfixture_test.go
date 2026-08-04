package schemas

import (
	"encoding/json"
	"testing"
)

// TestExpectationSchema and its embedded bytes (testExpectationSchemaJSON) are the
// PRODUCTION schema vars in compiler.go (E6-S01 moved them there so the pure
// internal/adoptertest harness can reuse the frozen contract); these tests exercise
// that same production var, same package.

// REQ-P3-E1-S05-01: test-expectation.schema.json requires decision: enum
// [APPROVE, REVIEW, BLOCK], an optional findings[] (each entry's effect
// drawn from the same closed vocabulary MergePolicy/DecisionRecord freeze —
// comment/challenge/block/require-review, never a bare vouch), an optional
// absent[], and an optional score: {total, threshold}.
func TestExpectationSchema_Validates(t *testing.T) {
	t.Run("ADR-0014 expect.yaml example (prove/onFailure vocabulary) is valid", func(t *testing.T) {
		const doc = `{
			"decision": "REVIEW",
			"findings": [
				{"rule": "topics/retention-shrink-challenge", "effect": "challenge", "path": "/retentionMs", "message~": "data loss"}
			],
			"absent": ["topics/no-topic-deletion"],
			"score": {"total": 3, "threshold": 4}
		}`
		if err := validateJSON(TestExpectationSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("a require-review finding (forge-proven ApprovalEvidence obligation) is valid", func(t *testing.T) {
		const doc = `{
			"decision": "REVIEW",
			"findings": [
				{"rule": "security-review-required", "obligation": "security-review", "effect": "require-review", "path": "/topics/0"}
			]
		}`
		if err := validateJSON(TestExpectationSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("exact: true for a closed findings list is valid", func(t *testing.T) {
		const doc = `{
			"decision": "BLOCK",
			"exact": true,
			"findings": [
				{"rule": "no-topic-deletion", "effect": "block"}
			]
		}`
		if err := validateJSON(TestExpectationSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("decision-only (no findings/absent/score) is valid", func(t *testing.T) {
		const doc = `{"decision": "APPROVE"}`
		if err := validateJSON(TestExpectationSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("adversarial: findings[].effect: vouch is invalid (retired, never legal)", func(t *testing.T) {
		const doc = `{
			"decision": "APPROVE",
			"findings": [
				{"rule": "topics/retention-shrink-challenge", "effect": "vouch"}
			]
		}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected effect: vouch to fail validation")
		}
	})

	t.Run("adversarial: missing decision is invalid", func(t *testing.T) {
		const doc = `{"findings": []}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected missing decision to fail validation")
		}
	})

	t.Run("adversarial: invalid decision enum is invalid", func(t *testing.T) {
		const doc = `{"decision": "AUTO_MERGE"}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected invalid decision enum to fail validation")
		}
	})

	t.Run("adversarial: unknown top-level field is invalid", func(t *testing.T) {
		const doc = `{"decision": "APPROVE", "bogus": true}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected unknown top-level field to fail validation")
		}
	})

	t.Run("adversarial: findings[] entry missing rule is invalid", func(t *testing.T) {
		const doc = `{"decision": "APPROVE", "findings": [{"effect": "comment"}]}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected findings[] entry missing rule to fail validation")
		}
	})

	t.Run("adversarial: findings[] entry missing effect is invalid", func(t *testing.T) {
		const doc = `{"decision": "APPROVE", "findings": [{"rule": "x"}]}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected findings[] entry missing effect to fail validation")
		}
	})
}

// REQ-P3-E1-S05-01 freezes must-contain-by-default semantics: an absent `exact`
// means "findings[] must all fire, others may too". The schema's `exact.default`
// annotation must therefore be false — a `default: true` silently flips an
// adopter's omitted-`exact` test from permissive must-contain to a closed list
// (roast P1-2). This guards the frozen annotation against re-inversion.
func TestExactDefaultIsMustContain(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(testExpectationSchemaJSON, &schema); err != nil {
		t.Fatalf("unmarshal test-expectation schema: %v", err)
	}
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("schema has no $defs object")
	}
	expectation, ok := defs["expectation"].(map[string]any)
	if !ok {
		t.Fatal("$defs has no expectation object")
	}
	props, ok := expectation["properties"].(map[string]any)
	if !ok {
		t.Fatal("expectation has no properties object")
	}
	exact, ok := props["exact"].(map[string]any)
	if !ok {
		t.Fatal("expectation.properties has no exact object")
	}
	if got, ok := exact["default"].(bool); !ok || got != false {
		t.Fatalf("exact.default must be false (must-contain by default, REQ-P3-E1-S05-01); got %v", exact["default"])
	}
}

// REQ-P3-E1-S05-02: .assent/tests/<pack>/cases.yaml's cases[] entries require
// name, file, base, head, and an expect object that reuses the exact same
// expect.yaml schema — the inline and directory forms are one contract, not
// two.
func TestInlineCasesReuseExpectSchema(t *testing.T) {
	t.Run("ADR-0014 inline cases.yaml example is valid", func(t *testing.T) {
		const doc = `{
			"cases": [
				{
					"name": "partition-increase-ok",
					"file": "topics/prod/orders.yaml",
					"base": {"name": "orders", "owner": "team-a", "partitions": 12},
					"head": {"name": "orders", "owner": "team-a", "partitions": 24},
					"facts": {"quota": {"max_partitions": 24}, "author": {"groups": ["team-a"]}},
					"expect": {"decision": "APPROVE"}
				}
			]
		}`
		if err := validateJSON(TestExpectationSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("a case without facts (optional) is valid", func(t *testing.T) {
		const doc = `{
			"cases": [
				{
					"name": "new-file-ok",
					"file": "topics/prod/new.yaml",
					"base": null,
					"head": {"name": "new", "owner": "team-a", "partitions": 6},
					"expect": {"decision": "APPROVE"}
				}
			]
		}`
		if err := validateJSON(TestExpectationSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("adversarial: case missing head is invalid", func(t *testing.T) {
		const doc = `{
			"cases": [
				{
					"name": "partition-increase-ok",
					"file": "topics/prod/orders.yaml",
					"base": {"name": "orders", "partitions": 12},
					"expect": {"decision": "APPROVE"}
				}
			]
		}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected case missing head to fail validation")
		}
	})

	t.Run("adversarial: case missing name is invalid", func(t *testing.T) {
		const doc = `{
			"cases": [
				{
					"file": "topics/prod/orders.yaml",
					"base": {"partitions": 12},
					"head": {"partitions": 24},
					"expect": {"decision": "APPROVE"}
				}
			]
		}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected case missing name to fail validation")
		}
	})

	t.Run("adversarial: case missing expect is invalid", func(t *testing.T) {
		const doc = `{
			"cases": [
				{
					"name": "partition-increase-ok",
					"file": "topics/prod/orders.yaml",
					"base": {"partitions": 12},
					"head": {"partitions": 24}
				}
			]
		}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected case missing expect to fail validation")
		}
	})

	t.Run("adversarial: case's expect reusing the shared schema still rejects effect: vouch", func(t *testing.T) {
		const doc = `{
			"cases": [
				{
					"name": "partition-increase-ok",
					"file": "topics/prod/orders.yaml",
					"base": {"partitions": 12},
					"head": {"partitions": 24},
					"expect": {
						"decision": "APPROVE",
						"findings": [{"rule": "x", "effect": "vouch"}]
					}
				}
			]
		}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected the shared expect schema to reject effect: vouch inside an inline case too")
		}
	})

	t.Run("adversarial: empty cases[] is invalid", func(t *testing.T) {
		const doc = `{"cases": []}`
		if err := validateJSON(TestExpectationSchema, doc); err == nil {
			t.Fatal("expected empty cases[] to fail validation")
		}
	})
}

// REQ-P3-E1-S05-03: the message~ field's schema description documents that
// assent test --coverage counts only structured safety assertions, and that
// rendered-output goldens belong to the assent render / PresentationModel
// layer (ADR-0016 §4) — a documentation note, not a validation rule.
func TestExpectationSchema_DocumentsPresentationSeparation(t *testing.T) {
	if err := validateJSON(TestExpectationSchema, `{
		"decision": "REVIEW",
		"findings": [{"rule": "x", "effect": "comment", "message~": "informational only"}]
	}`); err != nil {
		t.Fatalf("expected message~ to remain schema-legal (documentation, not a validation rule), got: %v", err)
	}
}
