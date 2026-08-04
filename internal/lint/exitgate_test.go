package lint_test

// exitgate_test.go is one half of the E3-S08 exit gate (spec §E3-S08): the
// hard-error fixture-corpus table (REQ-E3-S08-01) plus the pre-conformance
// fails-to-load proof (REQ-E3-S08-04) and the corpus determinism guard
// (REQ-E3-S08-05, internal/lint half). The archetype LOAD+EVALUATE gate
// (REQ-E3-S08-02) and the end-to-end exit-code/double-run gate live in
// cmd/assent/lint_corpus_test.go (they need the cmd-tier loader + aggregate
// engine); the catalogue-generation gate (REQ-E3-S08-03) lives in
// internal/catalogue.
//
// The corpus under examples/lint-fixtures/<code>/{good,bad} is the durable seed:
// for EACH hard error a POSITIVE (clean) tree and a NEGATIVE (violating) tree,
// each a minimal `.assent/**` pack. The table below asserts `assent lint` (the
// pure lint.Lint over the walked tree) emits EXACTLY the expected diagnostic
// code(s) on each negative and is CLEAN on each positive — so a future regression
// in any of the checks (or the tolerant-ingestion bridge) fails here by name.
//
// external package (lint_test): only the exported surface (lint.Lint /
// lint.Source / lint.Diagnostic) is used, so the corpus gate is decoupled from
// check internals.

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/lint"
)

// fixturesRoot is the hard-error corpus, relative to this package (internal/lint).
const fixturesRoot = "../../examples/lint-fixtures"

// hardErrorFixture is one row of the corpus table: the hard error's diagnostic
// code (also the fixture directory name) and the EXACT set of codes its negative
// tree must emit. Most negatives isolate a single code; unkeyed-list additionally
// trips the strict loader (a mode:list entry with no identity.pointer is refused
// by merge-policy.schema.json too), so its negative also carries the tolerant
// schema-invalid bridge diagnostic — asserted explicitly rather than hidden.
type hardErrorFixture struct {
	code    string
	wantBad []string
}

// hardErrorCorpus is the authoritative list of the E3 hard errors the exit gate
// pins. Adding a hard error to the lint pipeline means adding a fixture pair here.
var hardErrorCorpus = []hardErrorFixture{
	{code: "obligation-coverage", wantBad: []string{"obligation-coverage"}},
	{code: "facts-reference-syntax", wantBad: []string{"facts-reference-syntax"}},
	{code: "facts-reference-shape", wantBad: []string{"facts-reference-shape"}},
	{code: "reserved-class", wantBad: []string{"reserved-class"}},
	{code: "no-implicit-enforce-phase", wantBad: []string{"no-implicit-enforce-phase"}},
	// unkeyed-list ALSO fails the strict loader (the schema requires
	// identity.pointer for mode:list), captured tolerantly as one schema-invalid
	// diagnostic — the actionable unkeyed-list is what an author reads.
	{code: "unkeyed-list", wantBad: []string{"schema-invalid", "unkeyed-list"}},
	{code: "undeclared-predicate-scope", wantBad: []string{"undeclared-predicate-scope"}},
	{code: "message-template-scope", wantBad: []string{"message-template-scope"}},
	{code: "fail-open", wantBad: []string{"fail-open"}},
	{code: "single-writer-profile", wantBad: []string{"single-writer-profile"}},
	{code: "tests-per-rule", wantBad: []string{"tests-per-rule"}},
}

// TestEveryHardErrorFixtureCaught is the REQ-E3-S08-01 gate: each negative fixture
// triggers exactly its expected code(s); each positive fixture lints clean.
func TestEveryHardErrorFixtureCaught(t *testing.T) {
	for _, f := range hardErrorCorpus {
		f := f
		t.Run(f.code, func(t *testing.T) {
			// Positive: the clean tree emits NO diagnostics.
			if got := lintCodes(t, filepath.Join(fixturesRoot, f.code, "good")); len(got) != 0 {
				t.Errorf("positive fixture %s/good must lint clean, got codes %v", f.code, got)
			}
			// Negative: the violating tree emits EXACTLY the expected code set.
			want := append([]string(nil), f.wantBad...)
			sort.Strings(want)
			got := lintCodes(t, filepath.Join(fixturesRoot, f.code, "bad"))
			if !reflect.DeepEqual(got, want) {
				t.Errorf("negative fixture %s/bad: got codes %v, want exactly %v", f.code, got, want)
			}
		})
	}
}

// TestHardErrorCorpusDoubleRunStable is the internal/lint half of REQ-E3-S08-05:
// linting every fixture twice yields byte-identical canonical output (no
// clock/rand/env/map-order leak into diagnostics).
func TestHardErrorCorpusDoubleRunStable(t *testing.T) {
	for _, f := range hardErrorCorpus {
		for _, side := range []string{"good", "bad"} {
			dir := filepath.Join(fixturesRoot, f.code, side)
			sources := walkAssent(t, dir)
			first := lint.Lint(sources).Render()
			second := lint.Lint(sources).Render()
			if first != second {
				t.Errorf("%s/%s: lint output not double-run stable:\n first=%q\n second=%q", f.code, side, first, second)
			}
		}
	}
}

// TestPreConformancePackFailsToLoad is the REQ-E3-S08-04 gate (mirrors E2's
// lane-F-required proof): a pre-lane-C pack shape — a rule with NO rollout phase —
// is REFUSED by the E2 strict loader with a missing-phase reason, proving lane C's
// phase-conformance corrections are REQUIRED for the archetype packs to load. The
// live corpus counterpart (topic-registry pinned as a known blocker) is proven by
// cmd/assent's TestExamplesPacksKnownBlockers; this asserts the load-time refusal
// directly on the exact pre-conformance defect.
func TestPreConformancePackFailsToLoad(t *testing.T) {
	// A well-formed MergePolicy in every respect EXCEPT the required rollout phase
	// (the defect lane C fixes across examples/packs/**).
	noPhase := []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: pre-conformance
spec:
  rules:
    - name: needs-phase
      match:
        files:
          paths: ["**/*.yaml"]
      prove:
        obligation: safe
        when: 'kind == "modify"'
      onFailure:
        effect: require-review
        code: needs.review
`)
	_, err := policy.LoadMergePolicy(noPhase)
	if err == nil {
		t.Fatal("a MergePolicy rule with no phase must be refused by the strict loader (no-implicit-enforce-phase); lane C's phase conformance would then be unnecessary")
	}
	if !strings.Contains(err.Error(), "phase") {
		t.Errorf("pre-conformance load error must name the missing phase, got: %v", err)
	}
}

// lintCodes lints the `.assent/**` tree under dir and returns the sorted, de-duped
// set of diagnostic codes it emits.
func lintCodes(t *testing.T, dir string) []string {
	t.Helper()
	rep := lint.Lint(walkAssent(t, dir))
	seen := map[string]bool{}
	for _, d := range rep.Diagnostics() {
		seen[d.Code] = true
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// walkAssent walks dir/.assent and returns every YAML document as a lint.Source
// with a repo-relative, slash-separated Path (the same shape cmd/assent's
// discoverAssentTree produces — replicated here so the pure-package test needs no
// cmd import).
func walkAssent(t *testing.T, dir string) []lint.Source {
	t.Helper()
	root := filepath.Join(dir, ".assent")
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		t.Fatalf("no .assent tree under %q: %v", dir, err)
	}
	var sources []lint.Source
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}
		raw, rerr := os.ReadFile(path) //nolint:gosec // in-repo fixture tree, read-only.
		if rerr != nil {
			return rerr
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			rel = path
		}
		sources = append(sources, lint.Source{Path: filepath.ToSlash(rel), Bytes: raw})
		return nil
	})
	if err != nil {
		t.Fatalf("walk %q: %v", root, err)
	}
	return sources
}
