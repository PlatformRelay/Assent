package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

// examples_packs_lint_test.go is the E3 lane-C corpus guard: it pins the
// authored starter packs under examples/packs/** to the two conformance
// guarantees lane C establishes, so a future edit that regresses either is
// caught here (this is the durable seed the E3-S08 archetype gate builds on).
//
// Two halves are asserted per pack, both over the SAME strict authorities the
// engine and `assent lint` use (no bespoke re-implementation):
//
//  1. LOADS — loadCatalogueInput strict-loads every .assent/** document via the
//     E2 policy.Load{MergePolicy,Pack,RulesetBinding} loaders (the same walk
//     `assent catalogue` uses). A missing `phase` or an unknown field aborts it.
//  2. LINTS CLEAN — runLint (the `assent lint <dir>` entry point) exercises all
//     six hard-error checks (obligation-coverage, facts-syntax, facts-shape,
//     structural, predicate-scope, config-posture, tests-per-rule) and must exit
//     0 with no diagnostics.
//
// examplesPacksDir is repo-relative to this package (cmd/assent).
const examplesPacksDir = "../../examples/packs"

// TestExamplesPacksLoadAndLintClean is the positive corpus guard: every
// conformant pack tree strict-loads AND lints clean. A regression (a dropped
// phase, a facts reference reverted off `.value`, an uncovered obligation) fails
// here by name.
func TestExamplesPacksLoadAndLintClean(t *testing.T) {
	// All non-locked starter packs: service-catalog + infra-vars (E3 lane C) and
	// topic-registry (EFE-S04 — D-052 pin fully retired; load+lint+evaluate green).
	conformant := []string{"service-catalog", "infra-vars", "topic-registry"}

	for _, name := range conformant {
		name := name
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(examplesPacksDir, name)

			// Half 1: strict loader accepts every .assent/** document.
			if _, err := loadCatalogueInput(dir); err != nil {
				t.Fatalf("%s: strict loader rejected a document: %v", name, err)
			}

			// Half 2: `assent lint` is clean (exit 0, no diagnostics).
			var stdout, stderr bytes.Buffer
			if code := runLint([]string{dir}, &stdout, &stderr); code != 0 {
				t.Fatalf("%s: assent lint exit = %d, want 0; diagnostics:\n%s", name, code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("%s: a clean pack must emit no lint diagnostics, got:\n%s", name, stderr.String())
			}
		})
	}
}
