package adoptertest_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// repoRoot is the minimal single-rule fixture repo (a JSON /partitions cap governed
// by a resolved fact) this package's directory-case tests evaluate against.
const repoRoot = "testdata/repo/.assent"

// loadCase reads one directory case (tests/capped/<name>/) plus the pack it belongs
// to off disk and assembles an adoptertest.Case. Reading the files is the test's I/O
// boundary; the library under test stays pure (takes bytes). It fails the test on any
// load error — a case that will not load is not a meaningful pass/fail signal.
func loadCase(t *testing.T, name string) adoptertest.Case {
	t.Helper()

	mp, err := policy.LoadMergePolicy(readFile(t, filepath.Join(repoRoot, "packs", "capped", "rules", "capped.yaml")))
	if err != nil {
		t.Fatalf("load pack policy: %v", err)
	}
	rb, err := policy.LoadRulesetBinding(readFile(t, filepath.Join(repoRoot, "bindings.yaml")))
	if err != nil {
		t.Fatalf("load binding: %v", err)
	}
	if len(rb.Bindings) == 0 {
		t.Fatal("fixture binding declares no bindings")
	}

	caseDir := filepath.Join(repoRoot, "tests", "capped", name)
	expect, err := adoptertest.LoadExpectation(readFile(t, filepath.Join(caseDir, "expect.yaml")))
	if err != nil {
		t.Fatalf("load expect.yaml: %v", err)
	}
	facts, err := adoptertest.MapFacts(readFile(t, filepath.Join(caseDir, "facts.yaml")))
	if err != nil {
		t.Fatalf("map facts.yaml: %v", err)
	}

	const file = "config.json"
	return adoptertest.Case{
		Name:   name,
		Policy: mp,
		Bind:   &rb.Bindings[0],
		File:   file,
		Base:   readFile(t, filepath.Join(caseDir, "base", file)),
		Head:   readFile(t, filepath.Join(caseDir, "head", file)),
		Facts:  facts,
		Expect: expect,
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // fixed in-repo test fixture path, not user input.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// TestDirectoryCaseEvaluatesToExpectedDecision (REQ-E6-S01-01) drives a single-rule
// directory case end-to-end: base/↔head/ through the production change.Diff +
// evaldecode.BuildEvaluationInput, the authored facts.yaml lifted into the resolved
// envelope, and aggregate.Cover — asserting the produced Decision equals expect.yaml.
// The proving head APPROVEs; the over-cap head REVIEWs; the unresolved-fact case
// REVIEWs (never APPROVEs on a fact that was never resolved).
func TestDirectoryCaseEvaluatesToExpectedDecision(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"within-cap", "APPROVE"},
		{"over-cap", "REVIEW"},
		{"unresolved-fact", "REVIEW"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := adoptertest.RunCase(loadCase(t, tc.name))
			if err != nil {
				t.Fatalf("RunCase: %v", err)
			}
			if out.Actual != tc.want {
				t.Fatalf("decision = %q, want %q", out.Actual, tc.want)
			}
			if !out.Pass {
				t.Fatalf("case did not pass: expected %q, got %q", out.Expected, out.Actual)
			}
		})
	}
}

// TestCaseDoubleRunStable (REQ-E6-S01-05) proves determinism: the same case evaluated
// twice produces a byte-identical aggregate.Result (no clock/env/net/random in the
// decision path).
func TestCaseDoubleRunStable(t *testing.T) {
	for _, name := range []string{"within-cap", "over-cap", "unresolved-fact"} {
		t.Run(name, func(t *testing.T) {
			c := loadCase(t, name)
			first, err := adoptertest.Evaluate(c)
			if err != nil {
				t.Fatalf("Evaluate #1: %v", err)
			}
			second, err := adoptertest.Evaluate(c)
			if err != nil {
				t.Fatalf("Evaluate #2: %v", err)
			}
			if !bytes.Equal(mustJSON(t, first), mustJSON(t, second)) {
				t.Fatalf("double run not byte-identical:\n#1 %s\n#2 %s", mustJSON(t, first), mustJSON(t, second))
			}
		})
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
