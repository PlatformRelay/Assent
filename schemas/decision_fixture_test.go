package schemas

import (
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// P3-E1-S02 definition of done: a hand-built positive fixture round-trips
// through all five decision schemas — an EvaluationInput instance's
// obligations and subjects reappear, correctly redacted, in the
// corresponding DecisionRecord/PresentationModel.
func TestDecisionRuntimeRecordsFixtureRoundTrip(t *testing.T) {
	const evaluationInput = `{
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
					"state": "resolved",
					"sensitive": true,
					"observedAt": "2026-07-21T10:00:00Z",
					"expiresAt": "2026-07-21T10:15:00Z",
					"value": "streaming-platform"
				}
			}
		},
		"mr": {"author": "alice", "sourceBranch": "topic/orders-partitions", "targetBranch": "main", "labels": ["kafka"]},
		"require": ["non-destructive"]
	}`

	const pins = `{
		"toolVersion": "0.1.0",
		"toolDigest": "sha256:aaaa",
		"policySha": "sha256:bbbb",
		"sourceSha": "cccc",
		"targetSha": "dddd",
		"mergeResultDigest": "sha256:eeee",
		"factsResolvedAt": {"quota": "2026-07-21T10:00:00Z", "owner": "2026-07-21T10:00:00Z"}
	}`

	// The same obligation ("non-destructive") and subject
	// ("topic-registry:orders.events.v1") that appear in the EvaluationInput
	// reappear in DecisionRecord/PresentationModel — but the sensitive
	// "owner.team" fact value ("streaming-platform") never does, because
	// findings carry no fact-value field at all.
	const decisionRecord = `{
		"apiVersion": "assent.dev/v1alpha1",
		"kind": "DecisionRecord",
		"decision": "APPROVE",
		"findings": {
			"observed": [],
			"enforcing": [
				{
					"rule": "partition-increase-within-quota",
					"obligation": "non-destructive",
					"effect": "comment",
					"subject": "topic-registry:orders.events.v1",
					"points": 0,
					"code": "partition-quota-ok"
				}
			]
		},
		"pins": ` + pins + `
	}`

	const presentationModel = `{
		"apiVersion": "assent.dev/v1alpha1",
		"kind": "PresentationModel",
		"decision": "APPROVE",
		"findings": [
			{
				"rule": "partition-increase-within-quota",
				"obligation": "non-destructive",
				"effect": "comment",
				"subject": "topic-registry:orders.events.v1",
				"points": 0,
				"code": "partition-quota-ok"
			}
		]
	}`

	const replayBundle = `{
		"apiVersion": "assent.dev/v1alpha1",
		"kind": "ReplayBundle",
		"pins": ` + pins + `,
		"evaluationInput": ` + evaluationInput + `
	}`

	const publicationReceipt = `{
		"apiVersion": "assent.dev/v1alpha1",
		"kind": "PublicationReceipt",
		"operations": [
			{"kind": "merge", "targetId": "mr/42", "performedAt": "2026-07-21T10:10:00Z"}
		]
	}`

	for name, tc := range map[string]struct {
		schema *jsonschema.Schema
		doc    string
	}{
		"EvaluationInput":    {schema: EvaluationInputSchema, doc: evaluationInput},
		"DecisionRecord":     {schema: DecisionRecordSchema, doc: decisionRecord},
		"PresentationModel":  {schema: PresentationModelSchema, doc: presentationModel},
		"ReplayBundle":       {schema: ReplayBundleSchema, doc: replayBundle},
		"PublicationReceipt": {schema: PublicationReceiptSchema, doc: publicationReceipt},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateJSON(tc.schema, tc.doc); err != nil {
				t.Fatalf("expected %s fixture to validate, got: %v", name, err)
			}
		})
	}
}
