// Package schemas compiles and validates the versioned JSON Schemas that are
// assent's public contract surface (ADR-0017 §7: the serialized schemas are
// the API, not the internal Go types). This package is contract-tooling, not
// engine/decision code (D-016) — it neither trips the "no engine code before
// the Phase-3 gate" rule nor the internal/ coverage gate (D-010).
package schemas

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed policy/v1alpha1/config.schema.json
var configSchemaJSON []byte

//go:embed policy/v1alpha1/ruleset-binding.schema.json
var rulesetBindingSchemaJSON []byte

//go:embed policy/v1alpha1/merge-policy.schema.json
var mergePolicySchemaJSON []byte

//go:embed policy/v1alpha1/pack.schema.json
var packSchemaJSON []byte

//go:embed policy/v1alpha1/profile.schema.json
var profileSchemaJSON []byte

//go:embed decision/v1alpha1/evaluation-input.schema.json
var evaluationInputSchemaJSON []byte

//go:embed decision/v1alpha1/decision-record.schema.json
var decisionRecordSchemaJSON []byte

//go:embed decision/v1alpha1/replay-bundle.schema.json
var replayBundleSchemaJSON []byte

//go:embed decision/v1alpha1/presentation-model.schema.json
var presentationModelSchemaJSON []byte

//go:embed decision/v1alpha1/publication-receipt.schema.json
var publicationReceiptSchemaJSON []byte

//go:embed comparison/v1alpha1/comparison-record.schema.json
var comparisonRecordSchemaJSON []byte

//go:embed comparison/v1alpha1/comparison-suite.schema.json
var comparisonSuiteSchemaJSON []byte

var (
	// ConfigSchema validates schemas/policy/v1alpha1/config.schema.json instances.
	ConfigSchema = mustCompile("config.schema.json", configSchemaJSON)
	// RulesetBindingSchema validates schemas/policy/v1alpha1/ruleset-binding.schema.json instances.
	RulesetBindingSchema = mustCompile("ruleset-binding.schema.json", rulesetBindingSchemaJSON)
	// MergePolicySchema validates schemas/policy/v1alpha1/merge-policy.schema.json instances.
	MergePolicySchema = mustCompile("merge-policy.schema.json", mergePolicySchemaJSON)
	// PackSchema validates schemas/policy/v1alpha1/pack.schema.json instances
	// (pack.yaml manifest; phase is a ceiling — P3-E4-S01 / D-017 B2).
	PackSchema = mustCompile("pack.schema.json", packSchemaJSON)
	// ProfileSchema validates schemas/policy/v1alpha1/profile.schema.json
	// instances (named PolicyProfile; required writes boolean — P3-E4-S02).
	ProfileSchema = mustCompile("profile.schema.json", profileSchemaJSON)
)

// decisionSchemaID is the $id of one of the five decision/v1alpha1 runtime
// record schemas, used as the shared compiler's resource key so cross-file
// $ref (e.g. ReplayBundle -> EvaluationInput, PresentationModel ->
// DecisionRecord's finding $def) resolves within one compiler instance
// instead of drifting from a hand-duplicated copy.
const (
	evaluationInputSchemaID    = "https://assent.dev/schemas/decision/v1alpha1/evaluation-input.schema.json"
	decisionRecordSchemaID     = "https://assent.dev/schemas/decision/v1alpha1/decision-record.schema.json"
	replayBundleSchemaID       = "https://assent.dev/schemas/decision/v1alpha1/replay-bundle.schema.json"
	presentationModelSchemaID = "https://assent.dev/schemas/decision/v1alpha1/presentation-model.schema.json"
	publicationReceiptSchemaID = "https://assent.dev/schemas/decision/v1alpha1/publication-receipt.schema.json"
	comparisonRecordSchemaID   = "https://assent.dev/schemas/comparison/v1alpha1/comparison-record.schema.json"
	comparisonSuiteSchemaID    = "https://assent.dev/schemas/comparison/v1alpha1/comparison-suite.schema.json"
)

var decisionSchemas = mustCompileCrossReferenced(map[string][]byte{
	evaluationInputSchemaID:    evaluationInputSchemaJSON,
	decisionRecordSchemaID:     decisionRecordSchemaJSON,
	replayBundleSchemaID:       replayBundleSchemaJSON,
	presentationModelSchemaID: presentationModelSchemaJSON,
	publicationReceiptSchemaID: publicationReceiptSchemaJSON,
})

var comparisonSchemas = mustCompileCrossReferenced(map[string][]byte{
	comparisonRecordSchemaID: comparisonRecordSchemaJSON,
	comparisonSuiteSchemaID:  comparisonSuiteSchemaJSON,
})

var (
	// EvaluationInputSchema validates schemas/decision/v1alpha1/evaluation-input.schema.json instances.
	EvaluationInputSchema = decisionSchemas[evaluationInputSchemaID]
	// DecisionRecordSchema validates schemas/decision/v1alpha1/decision-record.schema.json instances.
	DecisionRecordSchema = decisionSchemas[decisionRecordSchemaID]
	// ReplayBundleSchema validates schemas/decision/v1alpha1/replay-bundle.schema.json instances.
	ReplayBundleSchema = decisionSchemas[replayBundleSchemaID]
	// PresentationModelSchema validates schemas/decision/v1alpha1/presentation-model.schema.json instances.
	PresentationModelSchema = decisionSchemas[presentationModelSchemaID]
	// PublicationReceiptSchema validates schemas/decision/v1alpha1/publication-receipt.schema.json instances.
	PublicationReceiptSchema = decisionSchemas[publicationReceiptSchemaID]
	// ComparisonRecordSchema validates schemas/comparison/v1alpha1/comparison-record.schema.json instances.
	ComparisonRecordSchema = comparisonSchemas[comparisonRecordSchemaID]
	// ComparisonSuiteSchema validates schemas/comparison/v1alpha1/comparison-suite.schema.json instances
	// (PolicyComparisonSuite — closed promotion-gate table + immutable corpus).
	ComparisonSuiteSchema = comparisonSchemas[comparisonSuiteSchemaID]
)

// newCompiler returns a compiler with this package's vendor vocabularies
// (x-uniqueKeys) registered, so every schema in this package resolves the
// same keyword semantics.
func newCompiler() *jsonschema.Compiler {
	c := jsonschema.NewCompiler()
	c.RegisterVocabulary(uniqueKeysVocabulary())
	// x-uniqueKeys is a vendor keyword with no $vocabulary declaration in the
	// schema files (keeping them portable to generic draft 2020-12
	// validators, which simply ignore the unknown keyword) — force this
	// compiler to still assert it.
	c.AssertVocabs()
	return c
}

func compile(name string, raw []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	c := newCompiler()
	if err := c.AddResource(name, doc); err != nil {
		return nil, fmt.Errorf("add resource %s: %w", name, err)
	}
	return c.Compile(name)
}

func mustCompile(name string, raw []byte) *jsonschema.Schema {
	sch, err := compile(name, raw)
	if err != nil {
		panic(fmt.Sprintf("compile %s: %v", name, err))
	}
	return sch
}

// mustCompileCrossReferenced loads every schema into one compiler so
// absolute $id / $ref URIs across files resolve (ReplayBundle → EvaluationInput,
// PresentationModel → DecisionRecord $defs, etc.).
func mustCompileCrossReferenced(resources map[string][]byte) map[string]*jsonschema.Schema {
	c := newCompiler()
	for id, raw := range resources {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			panic(fmt.Sprintf("parse %s: %v", id, err))
		}
		if err := c.AddResource(id, doc); err != nil {
			panic(fmt.Sprintf("add resource %s: %v", id, err))
		}
	}
	out := make(map[string]*jsonschema.Schema, len(resources))
	for id := range resources {
		sch, err := c.Compile(id)
		if err != nil {
			panic(fmt.Sprintf("compile %s: %v", id, err))
		}
		out[id] = sch
	}
	return out
}

// validateJSON parses raw as JSON and validates it against sch.
func validateJSON(sch *jsonschema.Schema, raw string) error {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(raw))
	if err != nil {
		return fmt.Errorf("not JSON: %w", err)
	}
	return sch.Validate(doc)
}
