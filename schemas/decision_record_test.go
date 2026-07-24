package schemas

import "testing"

// REQ-P3-E1-S02-02: decision-record.schema.json requires decision: enum
// [APPROVE, REVIEW, BLOCK], findings.{observed,enforcing} (P3-E4-S01 phase
// split; each entry: rule, obligation or none, effect, subject as EntryRef,
// points), and a pins object requiring
// toolVersion, toolDigest, policySha, sourceSha, targetSha,
// mergeResultDigest (nullable only when the forge capability is absent —
// ADR-0017 §1), and per-provider factsResolvedAt. No field may carry a raw
// fact value for a sensitive fact (there is no such field at all).
// Only findings.enforcing feeds aggregation; observed is structurally excluded.
func TestDecisionRecordSchema(t *testing.T) {
	t.Run("APPROVE with pinned mergeResultDigest is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "APPROVE",
			"findings": {
				"observed": [],
				"enforcing": [
					{"rule": "partition-increase-within-quota", "obligation": "non-destructive", "effect": "comment", "subject": "topic-registry:orders.events.v1", "points": 0, "code": "partition-quota-ok"}
				]
			},
			"pins": {
				"toolVersion": "0.1.0",
				"toolDigest": "sha256:aaaa",
				"policySha": "sha256:bbbb",
				"sourceSha": "cccc",
				"targetSha": "dddd",
				"mergeResultDigest": "sha256:eeee",
				"factsResolvedAt": {"quota": "2026-07-21T10:00:00Z"}
			}
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("REVIEW with a declared capability gap and null mergeResultDigest is valid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "REVIEW",
			"findings": {
				"observed": [],
				"enforcing": [
					{"rule": "no-approval-evidence", "obligation": "reviewed", "effect": "require-review", "subject": "topic-registry:orders.events.v1", "points": 5}
				]
			},
			"pins": {
				"toolVersion": "0.1.0",
				"toolDigest": "sha256:aaaa",
				"policySha": "sha256:bbbb",
				"sourceSha": "cccc",
				"targetSha": "dddd",
				"mergeResultDigest": null,
				"capabilityGap": "no merge-train/queue capability on this forge tier (ADR-0017 §1)",
				"factsResolvedAt": {"quota": "2026-07-21T10:00:00Z"}
			}
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("adversarial: APPROVE with null mergeResultDigest and no capabilityGap is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "APPROVE",
			"findings": {"observed": [], "enforcing": []},
			"pins": {
				"toolVersion": "0.1.0",
				"toolDigest": "sha256:aaaa",
				"policySha": "sha256:bbbb",
				"sourceSha": "cccc",
				"targetSha": "dddd",
				"mergeResultDigest": null,
				"factsResolvedAt": {}
			}
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err == nil {
			t.Fatal("expected APPROVE with null mergeResultDigest and no capabilityGap to fail validation")
		}
	})

	t.Run("adversarial: REVIEW with null mergeResultDigest and no capabilityGap is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "REVIEW",
			"findings": {"observed": [], "enforcing": []},
			"pins": {
				"toolVersion": "0.1.0",
				"toolDigest": "sha256:aaaa",
				"policySha": "sha256:bbbb",
				"sourceSha": "cccc",
				"targetSha": "dddd",
				"mergeResultDigest": null,
				"factsResolvedAt": {}
			}
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err == nil {
			t.Fatal("expected null mergeResultDigest without a capabilityGap to fail validation regardless of decision")
		}
	})

	t.Run("adversarial: fabricated capabilityGap alongside a present mergeResultDigest is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "APPROVE",
			"findings": {"observed": [], "enforcing": []},
			"pins": {
				"toolVersion": "0.1.0",
				"toolDigest": "sha256:aaaa",
				"policySha": "sha256:bbbb",
				"sourceSha": "cccc",
				"targetSha": "dddd",
				"mergeResultDigest": "sha256:eeee",
				"capabilityGap": "not actually missing",
				"factsResolvedAt": {}
			}
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err == nil {
			t.Fatal("expected a capabilityGap fabricated alongside a present mergeResultDigest to fail validation")
		}
	})

	t.Run("adversarial: invalid decision enum is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "AUTO_MERGE",
			"findings": {"observed": [], "enforcing": []},
			"pins": {
				"toolVersion": "0.1.0", "toolDigest": "sha256:aaaa", "policySha": "sha256:bbbb",
				"sourceSha": "cccc", "targetSha": "dddd", "mergeResultDigest": "sha256:eeee", "factsResolvedAt": {}
			}
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err == nil {
			t.Fatal("expected invalid decision enum to fail validation")
		}
	})

	t.Run("adversarial: pins missing toolDigest is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "BLOCK",
			"findings": {"observed": [], "enforcing": []},
			"pins": {
				"toolVersion": "0.1.0", "policySha": "sha256:bbbb",
				"sourceSha": "cccc", "targetSha": "dddd", "mergeResultDigest": "sha256:eeee", "factsResolvedAt": {}
			}
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err == nil {
			t.Fatal("expected pins missing toolDigest to fail validation")
		}
	})

	t.Run("adversarial: finding carrying a raw fact value field is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "BLOCK",
			"findings": {
				"observed": [],
				"enforcing": [
					{"rule": "x", "effect": "block", "subject": "file:x", "points": 10, "value": "raw-sensitive-value"}
				]
			},
			"pins": {
				"toolVersion": "0.1.0", "toolDigest": "sha256:aaaa", "policySha": "sha256:bbbb",
				"sourceSha": "cccc", "targetSha": "dddd", "mergeResultDigest": "sha256:eeee", "factsResolvedAt": {}
			}
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err == nil {
			t.Fatal("expected a finding carrying a raw value field to fail validation")
		}
	})

	t.Run("adversarial: missing pins is invalid", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "DecisionRecord",
			"decision": "BLOCK",
			"findings": {"observed": [], "enforcing": []}
		}`
		if err := validateJSON(DecisionRecordSchema, doc); err == nil {
			t.Fatal("expected missing pins to fail validation")
		}
	})
}
