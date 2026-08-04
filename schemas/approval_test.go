package schemas

import (
	// Blank import: enables the //go:embed directive below to embed the schema bytes.
	_ "embed"
	"testing"
)

// fullPins is a valid DecisionRecord.$defs.pins instance (canonical shape).
const fullPins = `{
	"toolVersion": "0.0.0-dev",
	"toolDigest": "sha256:tool",
	"policySha": "pol1",
	"sourceSha": "abc123",
	"targetSha": "def456",
	"mergeResultDigest": "sha256:aaaa",
	"factsResolvedAt": {}
}`

// REQ-P3-E1-S04-01: dossier §4 chain — approvalsRequired + approvedBy[] +
// eligibility + canonical pins $ref + required isAuthor.
func TestApprovalEvidenceSchema(t *testing.T) {
	t.Run("GitLab dossier §4 multi-approval rule is valid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": false},
			"source": {"rule": "security-review", "ruleType": "regular"},
			"eligibility": {"eligibleApproverIds": ["101", "202", "303"]},
			"approvalsRequired": 2,
			"approvedBy": [
				{"id": "202", "username": "alice", "isAuthor": false},
				{"id": "303", "username": "carol", "isAuthor": false}
			],
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("CODEOWNERS-sourced rule is valid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "303", "username": "bob", "isAuthor": false},
			"source": {"rule": "codeowners:/topics/**", "ruleType": "code_owner"},
			"eligibility": {"eligibleApproverIds": ["303", "404"]},
			"approvalsRequired": 1,
			"approvedBy": [{"id": "303", "username": "bob", "isAuthor": false}],
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"expiresAt": "2026-07-20T11:00:00Z",
			"verifyingCapability": "codeowners"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("adversarial: missing approvalsRequired is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": false},
			"source": {"rule": "security-review", "ruleType": "regular"},
			"eligibility": {"eligibleApproverIds": ["202"]},
			"approvedBy": [{"id": "202", "username": "alice", "isAuthor": false}],
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected missing approvalsRequired to fail validation")
		}
	})

	t.Run("adversarial: missing approvedBy is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": false},
			"source": {"rule": "security-review", "ruleType": "regular"},
			"eligibility": {"eligibleApproverIds": ["202"]},
			"approvalsRequired": 1,
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected missing approvedBy to fail validation")
		}
	})

	t.Run("adversarial: approvalsRequired 0 is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": false},
			"source": {"rule": "security-review", "ruleType": "regular"},
			"eligibility": {"eligibleApproverIds": ["202"]},
			"approvalsRequired": 0,
			"approvedBy": [{"id": "202", "username": "alice", "isAuthor": false}],
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected approvalsRequired: 0 to fail validation")
		}
	})

	t.Run("adversarial: forked subset pins (missing toolVersion) is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": false},
			"source": {"rule": "security-review", "ruleType": "regular"},
			"eligibility": {"eligibleApproverIds": ["202"]},
			"approvalsRequired": 1,
			"approvedBy": [{"id": "202", "username": "alice", "isAuthor": false}],
			"pins": {"sourceSha": "abc123", "targetSha": "def456", "mergeResultDigest": "sha256:aaaa"},
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected forked subset pins to fail — must use DecisionRecord pins shape")
		}
	})

	t.Run("adversarial: empty eligibleApproverIds is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": false},
			"source": {"rule": "security-review", "ruleType": "regular"},
			"eligibility": {"eligibleApproverIds": []},
			"approvalsRequired": 1,
			"approvedBy": [{"id": "202", "username": "alice", "isAuthor": false}],
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected empty eligibleApproverIds to fail validation")
		}
	})

	t.Run("adversarial: omitted principal.isAuthor is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice"},
			"source": {"rule": "security-review", "ruleType": "regular"},
			"eligibility": {"eligibleApproverIds": ["202"]},
			"approvalsRequired": 1,
			"approvedBy": [{"id": "202", "username": "alice", "isAuthor": false}],
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected omitted principal.isAuthor to fail validation (roast P2-C)")
		}
	})

	t.Run("adversarial: self-approval (principal.isAuthor: true) is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": true},
			"source": {"rule": "security-review", "ruleType": "regular"},
			"eligibility": {"eligibleApproverIds": ["202"]},
			"approvalsRequired": 1,
			"approvedBy": [{"id": "202", "username": "alice", "isAuthor": false}],
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected principal.isAuthor: true to fail validation")
		}
	})

	t.Run("adversarial: approvedBy entry missing isAuthor is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": false},
			"source": {"rule": "security-review", "ruleType": "regular"},
			"eligibility": {"eligibleApproverIds": ["202"]},
			"approvalsRequired": 1,
			"approvedBy": [{"id": "202", "username": "alice"}],
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected approvedBy[].isAuthor omission to fail validation")
		}
	})

	t.Run("canonical pins with null mergeResultDigest + capabilityGap is valid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": false},
			"source": {"rule": "security-review", "ruleType": "regular"},
			"eligibility": {"eligibleApproverIds": ["202"]},
			"approvalsRequired": 1,
			"approvedBy": [{"id": "202", "username": "alice", "isAuthor": false}],
			"pins": {
				"toolVersion": "0.0.0-dev",
				"toolDigest": "sha256:tool",
				"policySha": "pol1",
				"sourceSha": "abc123",
				"targetSha": "def456",
				"mergeResultDigest": null,
				"capabilityGap": "no-merge-train",
				"factsResolvedAt": {}
			},
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err != nil {
			t.Fatalf("expected canonical pins capability-gap shape to be valid, got: %v", err)
		}
	})
}

// REQ-P3-E1-S04-02: closed ruleType enum — discussion resolvers cannot populate
// require-review evidence (ADR-0017 §3).
func TestApprovalEvidenceExcludesDiscussion(t *testing.T) {
	t.Run("adversarial: source.ruleType discussion-resolved is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": false},
			"source": {"rule": "resolve-all-threads", "ruleType": "discussion-resolved"},
			"eligibility": {"eligibleApproverIds": ["202"]},
			"approvalsRequired": 1,
			"approvedBy": [{"id": "202", "username": "alice", "isAuthor": false}],
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "approval-rules-api"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected source.ruleType: discussion-resolved to fail validation")
		}
	})
}

// REQ-P3-E1-S04-03: verifyingCapability none → evidence fields structurally absent.
func TestApprovalEvidenceCapabilityGap(t *testing.T) {
	t.Run("capability gap with no evidence fields is valid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "none"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err != nil {
			t.Fatalf("expected valid, got: %v", err)
		}
	})

	t.Run("adversarial: capability none with approvedBy present is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"approvalsRequired": 1,
			"approvedBy": [{"id": "202", "username": "alice", "isAuthor": false}],
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "none"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected verifyingCapability: none with approvedBy to fail validation")
		}
	})

	t.Run("adversarial: capability none with principal present is invalid", func(t *testing.T) {
		doc := `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "ApprovalEvidence",
			"principal": {"id": "202", "username": "alice", "isAuthor": false},
			"pins": ` + fullPins + `,
			"observedAt": "2026-07-20T10:00:00Z",
			"verifyingCapability": "none"
		}`
		if err := validateJSON(ApprovalEvidenceSchema, doc); err == nil {
			t.Fatal("expected verifyingCapability: none with principal to fail validation")
		}
	})
}
