package schemadrift_test

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/schemadrift"
)

// toolDigestDescriptionLiteral captures the source text of the toolDigest
// description — the one field D-120 fences — from the raw schema JSON. It is
// DERIVED rather than hardcoded on purpose: once this lane merges, origin/main
// carries the new wording, and a literal anchor would red main the moment the
// baseline moved (silently turning the "description only" and "removing the
// description" cases into vacuous no-op replaces first).
var toolDigestDescriptionLiteral = regexp.MustCompile(
	`(?s)"toolDigest":\s*\{.*?("description": "(?:[^"\\]|\\.)*")`)

// baselineDecisionRecord returns decision-record.schema.json as origin/main has
// it — the same baseline the drift guard compares against — together with the
// exact toolDigest description literal it currently contains.
func baselineDecisionRecord(t *testing.T) (schema, description string) {
	t.Helper()
	cmd := exec.Command("git", "show", "origin/main:schemas/decision/v1alpha1/decision-record.schema.json")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("read origin/main decision-record schema: %v", err)
	}
	m := toolDigestDescriptionLiteral.FindStringSubmatch(string(out))
	if m == nil {
		t.Fatalf("baseline has no toolDigest description literal; update this test")
	}
	return string(out), m[1]
}

// TestAllowedD120ToolDigestDescriptionChange fences the ONE annotation AUD-S04 is
// permitted to touch (D-120). Both polarities matter: the toolDigest description
// must pass, and everything else — including OTHER descriptions in the same file
// — must fail, or the fence degrades into a blanket licence to edit annotations
// on a frozen schema.
func TestAllowedD120ToolDigestDescriptionChange(t *testing.T) {
	baseline, toolDigestDescription := baselineDecisionRecord(t)
	// Any wording that is not the baseline's own — so the "changed" cases below
	// are real changes whatever the baseline currently says.
	const newDescription = `"description": "Deterministic build-content proxy per D-120."`
	if toolDigestDescription == newDescription {
		t.Fatal("test fixture collides with the baseline wording; pick another")
	}

	t.Run("identical passes", func(t *testing.T) {
		if err := schemadrift.AllowedD120ToolDigestDescriptionChange([]byte(baseline), []byte(baseline)); err != nil {
			t.Fatalf("identical documents must pass: %v", err)
		}
	})

	t.Run("toolDigest description only passes", func(t *testing.T) {
		tampered := mustReplace(t, baseline, toolDigestDescription, newDescription)
		if err := schemadrift.AllowedD120ToolDigestDescriptionChange([]byte(baseline), []byte(tampered)); err != nil {
			t.Fatalf("description-only edit must pass: %v", err)
		}
	})

	// THE discriminating case. An implementation that normalises every
	// "description" in the document — rather than the one at
	// $defs.pins.properties.toolDigest — passes every other test here and
	// silently opens the whole frozen schema to annotation edits.
	t.Run("a different description fails", func(t *testing.T) {
		cases := map[string][2]string{
			"policySha gains a description": {
				`"policySha": { "type": "string", "minLength": 1 },`,
				`"policySha": { "type": "string", "minLength": 1, "description": "harmless annotation" },`,
			},
			"toolVersion gains a description": {
				`"toolVersion": { "type": "string", "minLength": 1 },`,
				`"toolVersion": { "type": "string", "minLength": 1, "description": "harmless annotation" },`,
			},
			"capabilityGap description reworded": {
				`"description": "Present only when mergeResultDigest is null;`,
				`"description": "Reworded. Present only when mergeResultDigest is null;`,
			},
			"top-level title description reworded": {
				`"description": "Redacted, stable outcome + evidence digests`,
				`"description": "Reworded. Redacted, stable outcome + evidence digests`,
			},
		}
		for name, pair := range cases {
			tampered := mustReplace(t, baseline, pair[0], pair[1])
			if err := schemadrift.AllowedD120ToolDigestDescriptionChange([]byte(baseline), []byte(tampered)); err == nil {
				t.Errorf("%s: must be rejected — D-120 fences the toolDigest description only", name)
			}
		}
	})

	// Validation keywords are the whole point of "annotation-only": a published
	// v0.1.0 record must stay valid against the edited schema.
	t.Run("validation keyword changes fail", func(t *testing.T) {
		cases := map[string][2]string{
			"toolDigest minLength widened": {
				`"toolDigest": {
          "type": "string",
          "minLength": 1,`,
				`"toolDigest": {
          "type": "string",
          "minLength": 2,`,
			},
			"toolDigest retyped": {
				`"toolDigest": {
          "type": "string",`,
				`"toolDigest": {
          "type": ["string", "null"],`,
			},
			"toolDigest gains a pattern": {
				`"toolDigest": {
          "type": "string",
          "minLength": 1,`,
				`"toolDigest": {
          "type": "string",
          "minLength": 1,
          "pattern": "^sha256:",`,
			},
			"pins gains a required field": {
				`"required": ["toolVersion", "toolDigest", "policySha"`,
				`"required": ["buildId", "toolVersion", "toolDigest", "policySha"`,
			},
			"decision enum narrowed": {
				`"decision": { "enum": ["APPROVE", "REVIEW", "BLOCK"] }`,
				`"decision": { "enum": ["APPROVE", "REVIEW"] }`,
			},
		}
		for name, pair := range cases {
			tampered := mustReplace(t, baseline, pair[0], pair[1])
			if err := schemadrift.AllowedD120ToolDigestDescriptionChange([]byte(baseline), []byte(tampered)); err == nil {
				t.Errorf("%s: must be rejected — D-120 permits an annotation, not validation", name)
			}
		}
	})

	// A co-mingled hunk must not ride along with the permitted annotation.
	t.Run("permitted description plus a smuggled change fails", func(t *testing.T) {
		tampered := mustReplace(t, baseline, toolDigestDescription, newDescription)
		tampered = mustReplace(t, tampered,
			`"policySha": { "type": "string", "minLength": 1 },`,
			`"policySha": { "type": "string" },`)
		if err := schemadrift.AllowedD120ToolDigestDescriptionChange([]byte(baseline), []byte(tampered)); err == nil {
			t.Fatal("a smuggled policySha change must not ride along with the permitted annotation")
		}
	})

	t.Run("removing the description fails", func(t *testing.T) {
		tampered := mustReplace(t, baseline, `"minLength": 1,
          `+toolDigestDescription, `"minLength": 1`)
		if err := schemadrift.AllowedD120ToolDigestDescriptionChange([]byte(baseline), []byte(tampered)); err == nil {
			t.Fatal("removing the annotation must fail — it cannot pass as unchanged")
		}
	})

	t.Run("missing toolDigest node fails", func(t *testing.T) {
		err := schemadrift.AllowedD120ToolDigestDescriptionChange(
			[]byte(`{"$defs":{"pins":{"properties":{"toolDigest":{"description":"x"}}}}}`),
			[]byte(`{"$defs":{"pins":{"properties":{}}}}`),
		)
		if err == nil {
			t.Fatal("a candidate without the toolDigest node must fail")
		}
	})

	t.Run("non-string description fails", func(t *testing.T) {
		err := schemadrift.AllowedD120ToolDigestDescriptionChange(
			[]byte(`{"$defs":{"pins":{"properties":{"toolDigest":{"description":"x"}}}}}`),
			[]byte(`{"$defs":{"pins":{"properties":{"toolDigest":{"description":42}}}}}`),
		)
		if err == nil {
			t.Fatal("a non-string description must fail")
		}
	})

	t.Run("invalid JSON fails", func(t *testing.T) {
		if err := schemadrift.AllowedD120ToolDigestDescriptionChange([]byte(`{`), []byte(baseline)); err == nil {
			t.Fatal("invalid baseline JSON must fail")
		}
		if err := schemadrift.AllowedD120ToolDigestDescriptionChange([]byte(baseline), []byte(`{`)); err == nil {
			t.Fatal("invalid candidate JSON must fail")
		}
	})
}

// TestValidateSchemaPathDriftD120 — the path fence admits decision-record.schema.json
// and nothing else new.
func TestValidateSchemaPathDriftD120(t *testing.T) {
	t.Run("decision-record path allowed", func(t *testing.T) {
		if err := schemadrift.ValidateSchemaPathDrift([]string{
			"schemas/decision/v1alpha1/decision-record.schema.json",
		}); err != nil {
			t.Fatalf("D-120 fenced path must be allowed: %v", err)
		}
	})

	t.Run("both fenced paths allowed together", func(t *testing.T) {
		if err := schemadrift.ValidateSchemaPathDrift([]string{
			"schemas/policy/v1alpha1/config.schema.json",
			"schemas/decision/v1alpha1/decision-record.schema.json",
		}); err != nil {
			t.Fatalf("both fenced paths must be allowed: %v", err)
		}
	})

	t.Run("sibling decision schemas still rejected", func(t *testing.T) {
		for _, path := range []string{
			"schemas/decision/v1alpha1/replay-bundle.schema.json",
			"schemas/decision/v1alpha1/evaluation-input.schema.json",
			"schemas/decision/v1alpha1/presentation-model.schema.json",
		} {
			err := schemadrift.ValidateSchemaPathDrift([]string{
				"schemas/decision/v1alpha1/decision-record.schema.json",
				path,
			})
			if err == nil {
				t.Errorf("%s must still be frozen", path)
			} else if !strings.Contains(err.Error(), path) {
				t.Errorf("error must name %s, got: %v", path, err)
			}
		}
	})
}

// mustReplace performs a single substitution and FAILS if it was a no-op. Every
// tampered fixture in this file is built by string replacement against a baseline
// that moves when the schema is edited; a silently-missed anchor would turn an
// adversarial case into a comparison of the baseline with itself, i.e. a test
// that asserts nothing while staying green.
func mustReplace(t *testing.T, s, old, replacement string) string {
	t.Helper()
	out := strings.Replace(s, old, replacement, 1)
	if out == s {
		t.Fatalf("anchor not found in baseline (update this test): %q", old)
	}
	return out
}
