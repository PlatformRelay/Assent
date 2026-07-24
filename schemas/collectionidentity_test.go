package schemas

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// collectionIdentityFixtureDir holds ADR-0017 §9 unique-ID named-collection
// fixtures (P3-E2-S02): every named collection (authored + report) is a list
// with a mandatory unique ID; unkeyed elements and duplicate IDs (with no
// explicit priority tiebreaker) are rejected — source order never carries
// implicit identity.
const collectionIdentityFixtureDir = "testdata/compat/collection-identity"

type collectionIdentityCase struct {
	schema  *jsonschema.Schema
	fixture string
}

func readCollectionIdentityFixture(t *testing.T, relPath string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(collectionIdentityFixtureDir, relPath)) //nolint:gosec // hardcoded test-fixture literal
	if err != nil {
		t.Fatalf("read fixture %s: %v", relPath, err)
	}
	return string(raw)
}

// TestUniqueIDCollections proves REQ-P3-E2-S02-02/03: unkeyed named
// collections and duplicate-ID collections (no priority) are rejected for
// every named-collection-bearing schema. ReplayBundle has no named
// collection of its own (pins + EvaluationInput only) and is covered by the
// additive suite instead.
func TestUniqueIDCollections(t *testing.T) {
	t.Run("unkeyed", func(t *testing.T) {
		// REQ-P3-E2-S02-02: a bare unkeyed array element (missing the
		// collection's mandatory ID field) is rejected — reordering cannot
		// invent identity because the unkeyed shape is forbidden entirely.
		for name, tc := range map[string]collectionIdentityCase{
			"Config":             {schema: ConfigSchema, fixture: "config/unkeyed.json"},
			"RulesetBinding":     {schema: RulesetBindingSchema, fixture: "ruleset-binding/unkeyed.json"},
			"MergePolicy":        {schema: MergePolicySchema, fixture: "merge-policy/unkeyed.json"},
			"DecisionRecord":     {schema: DecisionRecordSchema, fixture: "decision-record/unkeyed.json"},
			"PresentationModel":  {schema: PresentationModelSchema, fixture: "presentation-model/unkeyed.json"},
			"PublicationReceipt": {schema: PublicationReceiptSchema, fixture: "publication-receipt/unkeyed.json"},
		} {
			t.Run(name, func(t *testing.T) {
				doc := readCollectionIdentityFixture(t, tc.fixture)
				if err := validateJSON(tc.schema, doc); err == nil {
					t.Fatalf("expected %s fixture with an unkeyed collection element to be rejected", name)
				}
			})
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		// REQ-P3-E2-S02-03: two elements sharing an ID with no explicit
		// priority are rejected — source order is not a silent tiebreaker.
		// Adversarial: the duplicate differs only in an unrelated field.
		for name, tc := range map[string]collectionIdentityCase{
			"Config":             {schema: ConfigSchema, fixture: "config/duplicate-id.json"},
			"RulesetBinding":     {schema: RulesetBindingSchema, fixture: "ruleset-binding/duplicate-id.json"},
			"MergePolicy":        {schema: MergePolicySchema, fixture: "merge-policy/duplicate-id.json"},
			"DecisionRecord":     {schema: DecisionRecordSchema, fixture: "decision-record/duplicate-id.json"},
			"PresentationModel":  {schema: PresentationModelSchema, fixture: "presentation-model/duplicate-id.json"},
			"PublicationReceipt": {schema: PublicationReceiptSchema, fixture: "publication-receipt/duplicate-id.json"},
		} {
			t.Run(name, func(t *testing.T) {
				doc := readCollectionIdentityFixture(t, tc.fixture)
				if err := validateJSON(tc.schema, doc); err == nil {
					t.Fatalf("expected %s fixture with a duplicate collection ID (no priority) to be rejected", name)
				}
			})
		}
	})
}
