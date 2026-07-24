package schemas

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// contractSchemasByKind maps a document's (apiVersion, kind) to the schema
// that owns it (P3-E1-S06 REQ-03b: "validates every fixture … against its
// matching schema by apiVersion/kind"). Only the domains compiled as
// package-level (non-test-only) vars are listed here — approval/testfixture
// instances are still compile+fixture checked by their own _test.go files in
// this package, exercised by the same `go test ./schemas/...` the CI job
// runs.
var contractSchemasByKind = map[string]*jsonschema.Schema{
	"Config":             ConfigSchema,
	"RulesetBinding":     RulesetBindingSchema,
	"MergePolicy":        MergePolicySchema,
	"Pack":               PackSchema,
	"PolicyProfile":      ProfileSchema,
	"EvaluationInput":    EvaluationInputSchema,
	"DecisionRecord":     DecisionRecordSchema,
	"ReplayBundle":       ReplayBundleSchema,
	"PresentationModel":  PresentationModelSchema,
	"PublicationReceipt": PublicationReceiptSchema,
	// ApprovalEvidence is registered so a standalone ApprovalEvidence fixture
	// (the named-consumer-compat evidence instance, P3-E1-S07 REQ-03) is swept
	// against its owning schema too, instead of tripping the !known hard-error
	// branch (decide-and-log, P3-E1-S07: strictly widens sweep coverage).
	"ApprovalEvidence": ApprovalEvidenceSchema,
	// Comparison kinds (P3-E4-S03) — registered so future examples/contracts
	// fixtures validate against the closed taxonomy / suite schemas.
	"ComparisonRecord":       ComparisonRecordSchema,
	"PolicyComparisonSuite":  ComparisonSuiteSchema,
}

const contractAPIVersion = "assent.dev/v1alpha1"

// TestExampleContractsFixturesValidate is the CI-facing fixture-validation
// step REQ-P3-E1-S06-03 describes (schemas.yml's job runs this via
// `go test ./schemas/...`). It walks examples/contracts/** — this epic's own
// fixture directory (the P3-E1-S07 exit-gate and named-consumer-compat
// fixtures land there) — and validates every document declaring a
// recognized apiVersion/kind pair against its matching schema, failing hard
// on drift.
//
// Scope note (decide-and-log, P3-E1-S06): this walk is intentionally scoped
// to examples/contracts/**, not all of examples/**. examples/archetypes/**
// and examples/policies/** predate the ADR-0017 schema freeze and either
// carry an explicit "# DRAFT" marker (ADR-0017 consequences: "Examples
// migrate to prove/onFailure when the schemas land … until then they carry
// DRAFT markers") or, in one already-migrated-looking archetype, have
// pre-existing drift (a rule missing the now-required `match`) that is out
// of this lane's owned paths to fix. Migrating those examples is a separate,
// follow-up concern — see agent-context/INBOX.md.
func TestExampleContractsFixturesValidate(t *testing.T) {
	root := filepath.Join("..", "examples", "contracts")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		t.Skip("examples/contracts/ does not exist yet (lands with P3-E1-S07) — gate armed, not applicable")
	}
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}
	if len(entries) == 0 {
		t.Skip("examples/contracts/ is empty — gate armed, not applicable")
	}

	checked := 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".yaml" && ext != ".yml" {
			return nil
		}

		raw, readErr := os.ReadFile(path) //nolint:gosec // fixed test-fixture tree, not user input
		if readErr != nil {
			t.Errorf("%s: read: %v", path, readErr)
			return nil
		}

		doc, decodeErr := decodeContractDoc(ext, raw)
		if decodeErr != nil {
			t.Errorf("%s: decode: %v", path, decodeErr)
			return nil
		}

		obj, ok := doc.(map[string]any)
		if !ok {
			return nil // not an apiVersion/kind-shaped document (e.g. a bare array)
		}
		apiVersion, _ := obj["apiVersion"].(string)
		kind, _ := obj["kind"].(string)
		if apiVersion != contractAPIVersion || kind == "" {
			return nil // no matching contract kind to validate against
		}
		schema, known := contractSchemasByKind[kind]
		if !known {
			t.Errorf("%s: kind %q has no known schemas/**/v1alpha1 schema — add it to contractSchemasByKind or fix the kind", path, kind)
			return nil
		}

		if err := schema.Validate(doc); err != nil {
			t.Errorf("%s: fails %s schema validation: %v", path, kind, err)
			return nil
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if checked == 0 {
		t.Skip("no apiVersion/kind-bearing fixtures found under examples/contracts/ — gate armed, not applicable")
	}
	t.Logf("validated %d fixture(s) under %s", checked, root)
}

// decodeContractDoc parses raw JSON/YAML into the any-tree shape
// jsonschema.Schema.Validate expects (json.Number for numbers, not float64 —
// matching jsonschema.UnmarshalJSON's own decoding in compiler.go).
func decodeContractDoc(ext string, raw []byte) (any, error) {
	if ext == ".json" {
		return jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	}
	var yamlDoc any
	if err := yaml.Unmarshal(raw, &yamlDoc); err != nil {
		return nil, err
	}
	// Round-trip through encoding/json so numeric/map types match the shape
	// jsonschema.UnmarshalJSON produces (yaml.v3 already yields
	// map[string]any with string keys, but json.Number vs. float64 differs).
	jsonBytes, err := json.Marshal(yamlDoc)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(strings.NewReader(string(jsonBytes)))
}
