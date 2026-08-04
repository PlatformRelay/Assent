package lint

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mpSrc is a schema-valid MergePolicy under the given pack, declaring one prove
// rule (name + obligation). Mirrors coverage_test.go's ruleSrc but lets the test
// pin the pack directory (so the tests/<pack>/ mapping is exercised).
func mpSrc(pack, ruleName, obligation string) Source {
	return Source{
		Path: ".assent/packs/" + pack + "/rules/" + ruleName + ".yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: ` + ruleName + `
spec:
  rules:
    - name: ` + ruleName + `
      phase: enforce
      match:
        files:
          paths: ["topics/**/*.yaml"]
      prove:
        obligation: ` + obligation + `
        when: "entry.owner in facts.author.groups"
      onFailure:
        effect: require-review
        code: ` + obligation + `.unproven
`),
	}
}

// caseDirSrc is one directory-form test case fixture: an expect.yaml at
// `.assent/tests/<pack>/<caseName>/expect.yaml`. Only its PATH is load-bearing for
// the presence check (never its contents), but it is valid YAML so ingest skips it
// cleanly (no `kind` → not a policy doc).
func caseDirSrc(pack, caseName string) Source {
	return Source{
		Path:  ".assent/tests/" + pack + "/" + caseName + "/expect.yaml",
		Bytes: []byte("decision: APPROVE\nfindings: []\n"),
	}
}

// testsPerRuleDiags runs the full lint and returns only the tests-per-rule
// diagnostics, so a fixture may carry other (unrelated) diagnostics without
// masking the assertion under test.
func testsPerRuleDiags(sources []Source) []Diagnostic {
	var out []Diagnostic
	for _, d := range Lint(sources).Diagnostics() {
		if d.Code == CodeTestsPerRule {
			out = append(out, d)
		}
	}
	return out
}

// TestRuleWithoutTestRejected — REQ-E3-S06-01: a rule with no matching case
// directory/inline case emits one tests-per-rule naming the rule; adding a
// matching case clears it.
func TestRuleWithoutTestRejected(t *testing.T) {
	// No case directory at all → the rule is flagged.
	diags := testsPerRuleDiags([]Source{mpSrc("p", "author-owns-entry", "ownership")})
	if len(diags) != 1 {
		t.Fatalf("want exactly 1 tests-per-rule diagnostic, got %d: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeTestsPerRule {
		t.Errorf("code = %q, want %q", diags[0].Code, CodeTestsPerRule)
	}
	if diags[0].Severity != SeverityError {
		t.Errorf("severity = %q, want %q", diags[0].Severity, SeverityError)
	}
	if !strings.Contains(diags[0].Location.Name, "author-owns-entry") {
		t.Errorf("location must name the untested rule, got %q", diags[0].Location.Name)
	}

	// A case directory named for the rule's obligation clears it (the observed
	// convention: dirs are named after the obligation).
	clean := testsPerRuleDiags([]Source{
		mpSrc("p", "author-owns-entry", "ownership"),
		caseDirSrc("p", "ownership"),
	})
	if len(clean) != 0 {
		t.Fatalf("a rule with a matching case dir must lint clean, got %#v", clean)
	}
}

// TestFullyTestedPackLintsClean — REQ-E3-S06-02: the REAL topic-registry pack,
// each rule of which has a `.assent/tests/topics/<obligation>/` directory, emits
// NO tests-per-rule. Reads the on-disk layout, not an invented glob.
func TestFullyTestedPackLintsClean(t *testing.T) {
	// Pack-dir-relative Paths (start `.assent/`), exactly as cmd/assent's
	// discoverAssentTree produces them — so packName() derives "topics" (the inner
	// packs/<name> segment) and not "topic-registry" (the outer examples/packs one).
	dir := filepath.FromSlash("../../examples/packs/topic-registry")
	sources := loadRealAssentTree(t, dir)
	// Sanity: prove we fed relative Paths (guards the packName trap).
	sawRel := false
	for _, s := range sources {
		if s.Path == ".assent/packs/topics/rules/ownership.yaml" {
			sawRel = true
		}
	}
	if !sawRel {
		t.Fatalf("expected a pack-relative source path .assent/packs/topics/rules/ownership.yaml; got e.g. %q", sources[0].Path)
	}

	diags := testsPerRuleDiags(sources)
	if len(diags) != 0 {
		t.Fatalf("the real topic-registry pack must emit no tests-per-rule (every rule has a tests/topics/<obligation>/ dir), got %#v", diags)
	}
}

// TestTestMustReferenceTheRule — REQ-E3-S06-03: a rule whose only on-disk case
// references a DIFFERENT rule is still flagged; presence must be for THAT rule.
func TestTestMustReferenceTheRule(t *testing.T) {
	// Two rules with disjoint token sets. Only rule-b's case dir ("beta") exists.
	// rule-a (tokens {rule-a, alpha}) has no matching case → flagged; rule-b clean.
	diags := testsPerRuleDiags([]Source{
		mpSrc("p", "rule-a", "alpha"),
		mpSrc("p", "rule-b", "beta"),
		caseDirSrc("p", "beta"),
	})
	if len(diags) != 1 {
		t.Fatalf("only rule-a (no case for it) must be flagged, got %d: %#v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Location.Name, "rule-a") {
		t.Errorf("the flagged rule must be rule-a (its only-existing case references a different rule), got %q", diags[0].Location.Name)
	}
}

// TestTestsPerRuleDoubleRunStable — REQ-E3-S06-04: determinism — a mixed corpus
// double-runs byte-identical (presence-only, no case executed, no map order leak).
func TestTestsPerRuleDoubleRunStable(t *testing.T) {
	sources := []Source{
		mpSrc("p", "rule-a", "alpha"),
		mpSrc("p", "rule-b", "beta"),
		mpSrc("q", "rule-c", "gamma"),
		caseDirSrc("p", "beta"),
		caseDirSrc("q", "gamma"),
	}
	first := Lint(sources).Render()
	second := Lint(sources).Render()
	if first != second {
		t.Fatalf("lint render must be byte-identical across runs:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	// And the tests-per-rule half must actually fire (alpha uncovered), so the
	// stability guard is meaningful, not vacuous.
	if len(testsPerRuleDiags(sources)) != 1 {
		t.Fatalf("expected exactly one tests-per-rule (rule-a/alpha uncovered)")
	}
}

// TestTestsPerRuleInlineCases — coverage of the inline `cases.yaml` branch: a case
// whose name matches the rule's obligation clears the rule; a rule no inline case
// names is still flagged.
func TestTestsPerRuleInlineCases(t *testing.T) {
	inline := Source{
		Path: ".assent/tests/p/cases.yaml",
		Bytes: []byte(`cases:
  - name: ownership
    expect: { decision: APPROVE }
`),
	}
	clean := testsPerRuleDiags([]Source{mpSrc("p", "author-owns-entry", "ownership"), inline})
	if len(clean) != 0 {
		t.Fatalf("an inline case named for the obligation must clear the rule, got %#v", clean)
	}
	flagged := testsPerRuleDiags([]Source{mpSrc("p", "author-owns-entry", "freshness"), inline})
	if len(flagged) != 1 {
		t.Fatalf("a rule no inline case names must still be flagged, got %#v", flagged)
	}
}

// TestTestsPerRuleNoIdentityTokenFailSafe — a rule with neither a name nor a prove
// obligation cannot be mapped to any case and is flagged (fail-safe: unmappable →
// flagged, never silently passed).
func TestTestsPerRuleNoIdentityTokenFailSafe(t *testing.T) {
	src := Source{
		Path: ".assent/packs/p/rules/plain.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: plain
spec:
  rules:
    - phase: enforce
      match:
        files:
          paths: ["topics/**/*.yaml"]
      effect: block
      code: some.finding
`),
	}
	// Even with a case dir present, an identity-less rule cannot claim it.
	diags := testsPerRuleDiags([]Source{src, caseDirSrc("p", "anything")})
	if len(diags) != 1 {
		t.Fatalf("an identity-less rule must be flagged fail-safe, got %d: %#v", len(diags), diags)
	}
}

// loadRealAssentTree walks <dir>/.assent and returns every YAML doc as a Source
// with a <dir>-relative, slash path — the same shape cmd/assent's discoverAssentTree
// produces, so packName() and the tests/<pack>/ parser agree on the pack segment.
func loadRealAssentTree(t *testing.T, dir string) []Source {
	t.Helper()
	root := filepath.Join(dir, ".assent")
	var sources []Source
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
			return nil
		}
		raw, rerr := os.ReadFile(path) // #nosec G304,G122 -- test fixture path from walking a fixed in-repo examples tree; no symlink surface.
		if rerr != nil {
			return rerr
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			rel = path
		}
		sources = append(sources, Source{Path: filepath.ToSlash(rel), Bytes: raw})
		return nil
	})
	if err != nil {
		t.Fatalf("walk real .assent tree: %v", err)
	}
	if len(sources) == 0 {
		t.Fatalf("no sources discovered under %q — fixture path wrong?", root)
	}
	return sources
}
