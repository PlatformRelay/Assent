package schemas

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// reportToleranceFixtureDir holds ADR-0017 §9 additive-tolerant report
// fixtures (P3-E2-S02): report kinds must decode when a newer producer adds
// an unknown top-level field — the opposite of S01's strict-decode rule for
// authored safety-bearing resources.
const reportToleranceFixtureDir = "testdata/compat/report-tolerance"

// TestReportAdditiveTolerant proves REQ-P3-E2-S02-01: DecisionRecord,
// ReplayBundle, PresentationModel, and PublicationReceipt fixtures carrying
// an extra top-level field unknown to schema-version-N decode successfully,
// and the unknown field is preserved on the parsed document (not stripped,
// not a hard failure). No generated rule catalogue schema exists yet — that
// kind is skipped until it lands (spec: "if present").
func TestReportAdditiveTolerant(t *testing.T) {
	cases := map[string]struct {
		schema  *jsonschema.Schema
		fixture string
	}{
		"DecisionRecord":     {schema: DecisionRecordSchema, fixture: "decision-record/additive-field.json"},
		"ReplayBundle":       {schema: ReplayBundleSchema, fixture: "replay-bundle/additive-field.json"},
		"PresentationModel":  {schema: PresentationModelSchema, fixture: "presentation-model/additive-field.json"},
		"PublicationReceipt": {schema: PublicationReceiptSchema, fixture: "publication-receipt/additive-field.json"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(reportToleranceFixtureDir, tc.fixture)) //nolint:gosec // hardcoded test-fixture literal
			if err != nil {
				t.Fatalf("read fixture %s: %v", tc.fixture, err)
			}
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("parse fixture %s: %v", tc.fixture, err)
			}
			obj, ok := doc.(map[string]any)
			if !ok {
				t.Fatalf("fixture %s: expected object document", tc.fixture)
			}
			if _, has := obj["futureExtension"]; !has {
				t.Fatalf("fixture %s must plant futureExtension before validate", tc.fixture)
			}
			if err := tc.schema.Validate(doc); err != nil {
				t.Fatalf("expected additive-tolerant decode of %s to succeed, got: %v", name, err)
			}
			// Preserve: validation must not strip the unknown top-level field.
			if _, has := obj["futureExtension"]; !has {
				t.Fatalf("expected futureExtension to be preserved on %s after validate", name)
			}
		})
	}
}
