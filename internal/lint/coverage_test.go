package lint

import (
	"strings"
	"testing"
)

// bindingSrc is a schema-valid RulesetBinding requiring the given obligations,
// bound to pack "p". Built inline so each test owns its exact defect surface.
func bindingSrc(require string) Source {
	return Source{
		Path: ".assent/bindings.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: RulesetBinding
bindings:
  - class: kafka-topic
    environment: dev
    packs: [p]
    risk: { threshold: 10 }
    require: [` + require + `]
`),
	}
}

// ruleSrc is a schema-valid MergePolicy under pack "p" proving one obligation.
func ruleSrc(name, obligation string) Source {
	return Source{
		Path: ".assent/packs/p/rules/" + name + ".yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: ` + name + `
spec:
  rules:
    - name: ` + name + `
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

// TestObligationCoverageUncovered — REQ-E3-S01-01: a require[] obligation no
// bound rule proves emits exactly one located obligation-coverage error and the
// report signals a non-zero exit.
func TestObligationCoverageUncovered(t *testing.T) {
	rep := Lint([]Source{
		bindingSrc("ownership, freshness"),
		ruleSrc("owns", "ownership"),
		// A test case for the proven rule so tests-per-rule (E3-S06, now wired into
		// the shared Lint) does not add a second diagnostic — this test isolates the
		// obligation-coverage defect.
		caseDirSrc("p", "ownership"),
	})
	diags := rep.Diagnostics()
	if len(diags) != 1 {
		t.Fatalf("want exactly 1 diagnostic, got %d: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeObligationCoverage {
		t.Errorf("code = %q, want %q", d.Code, CodeObligationCoverage)
	}
	if d.Severity != SeverityError {
		t.Errorf("severity = %q, want %q", d.Severity, SeverityError)
	}
	if !strings.Contains(d.Message, "freshness") {
		t.Errorf("message must name the uncovered obligation %q, got %q", "freshness", d.Message)
	}
	if !strings.Contains(d.Location.Name, "kafka-topic") || !strings.Contains(d.Location.Name, "dev") {
		t.Errorf("location must name the binding (class, environment), got %q", d.Location.Name)
	}
	if d.Location.File != ".assent/bindings.yaml" {
		t.Errorf("location file = %q, want the binding file", d.Location.File)
	}
	if !rep.HasErrors() {
		t.Error("report must signal a non-zero exit when an error diagnostic is present")
	}
}

// TestTolerantIngestionAccumulatesAllDiagnostics — REQ-E3-S01-02 (the critical
// test): a pack with TWO distinct defects — a strict-schema violation (bad
// onFailure.effect enum) AND an uncovered obligation — reports BOTH in one run.
// This proves lint does NOT inherit the strict loader's first-error abort.
func TestTolerantIngestionAccumulatesAllDiagnostics(t *testing.T) {
	// ownership.yaml proves `ownership` but carries a bad onFailure.effect enum:
	// the tolerant decode still yields prove.obligation (so the coverage check
	// runs), while the strict loader rejects the enum (one schema diagnostic).
	badRule := Source{
		Path: ".assent/packs/p/rules/owns.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: owns
spec:
  rules:
    - name: owns
      phase: enforce
      match:
        files:
          paths: ["topics/**/*.yaml"]
      prove:
        obligation: ownership
        when: "entry.owner in facts.author.groups"
      onFailure:
        effect: bogus-effect
        code: ownership.unproven
`),
	}
	rep := Lint([]Source{
		bindingSrc("ownership, freshness"), // freshness uncovered
		badRule,                            // strict-schema violation, proves ownership
	})
	diags := rep.Diagnostics()

	var haveSchema, haveCoverage bool
	for _, d := range diags {
		switch d.Code {
		case CodeSchemaInvalid:
			haveSchema = true
			if d.Location.File != ".assent/packs/p/rules/owns.yaml" {
				t.Errorf("schema diagnostic must be located to the offending doc, got %q", d.Location.File)
			}
		case CodeObligationCoverage:
			haveCoverage = true
			if !strings.Contains(d.Message, "freshness") {
				t.Errorf("coverage diagnostic must name freshness, got %q", d.Message)
			}
		}
	}
	if !haveSchema {
		t.Error("missing the strict-schema-violation diagnostic (fail-many broken: strict loader aborted ingestion)")
	}
	if !haveCoverage {
		t.Error("missing the obligation-coverage diagnostic (fail-many broken: coverage never ran)")
	}
	if !rep.HasErrors() {
		t.Error("report must signal a non-zero exit")
	}
}

// TestSchemaInvalidMessageIsEnvironmentIndependent — REQ-E3-S01-05 (diagnostic
// model determinism): the strict-loader error the schema-invalid diagnostic
// embeds must carry no absolute file:// schema URI (which the jsonschema
// validator derives from the process CWD at runtime) — else lint output is
// machine-dependent and S08's cross-environment golden corpus breaks. The
// actionable "at '<path>': <detail>" remainder is retained.
//
// The vehicle is a bad onFailure.effect enum (NOT a missing phase): E3-S02's
// no-implicit-enforce-phase now owns the missing-phase defect and DEDUPES the
// phase-only schema-invalid, so a missing-phase fixture would no longer produce a
// schema-invalid at all. The bad-enum violation exercises the identical
// URI-stripping path while keeping a schema-invalid alive to assert over.
func TestSchemaInvalidMessageIsEnvironmentIndependent(t *testing.T) {
	badEnum := Source{
		Path: ".assent/packs/p/rules/owns.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: owns
spec:
  rules:
    - name: owns
      phase: enforce
      match:
        files:
          paths: ["topics/**/*.yaml"]
      prove:
        obligation: ownership
        when: "entry.owner in facts.author.groups"
      onFailure:
        effect: bogus-effect
        code: ownership.unproven
`),
	}
	var found bool
	for _, d := range Lint([]Source{badEnum}).Diagnostics() {
		if d.Code != CodeSchemaInvalid {
			continue
		}
		found = true
		if strings.Contains(d.Message, "file://") {
			t.Errorf("schema-invalid message must not embed an absolute file:// URI (CWD-dependent), got: %q", d.Message)
		}
		if !strings.Contains(d.Message, "onFailure/effect") {
			t.Errorf("schema-invalid message must retain the actionable detail (the offending path), got: %q", d.Message)
		}
	}
	if !found {
		t.Fatal("a rule with a bad onFailure.effect enum must produce a schema-invalid diagnostic")
	}
}

// TestFullyCoveredPackLintsClean — REQ-E3-S01-03: every required obligation is
// proven by a bound rule → no diagnostic, clean exit (no over-fire).
func TestFullyCoveredPackLintsClean(t *testing.T) {
	rep := Lint([]Source{
		bindingSrc("ownership, freshness"),
		ruleSrc("owns", "ownership"),
		ruleSrc("fresh", "freshness"),
		// A test case per rule so tests-per-rule (E3-S06, wired into the shared Lint)
		// does not fire — a truly clean pack has both coverage AND tests.
		caseDirSrc("p", "ownership"),
		caseDirSrc("p", "freshness"),
	})
	if diags := rep.Diagnostics(); len(diags) != 0 {
		t.Fatalf("fully-covered pack must lint clean, got %d diagnostics: %#v", len(diags), diags)
	}
	if rep.HasErrors() {
		t.Error("fully-covered pack must exit clean")
	}
}

// TestLintReportDoubleRunStable — REQ-E3-S01-05: canonically sorted, and a
// double run over identical inputs renders byte-identically (determinism/purity).
func TestLintReportDoubleRunStable(t *testing.T) {
	// A multi-defect input (several bindings + strict errors) so any map-iteration
	// order leak would surface as a rendering difference between runs.
	sources := []Source{
		multiBindingSrc(),
		ruleSrc("owns", "ownership"),
		Source{Path: ".assent/config.yaml", Bytes: []byte("not: a valid config\n")},
	}
	first := Lint(sources).Render()
	second := Lint(sources).Render()
	if first != second {
		t.Fatalf("double run not byte-identical:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	// Canonical ordering: rendered diagnostics are non-decreasing by sort key.
	diags := Lint(sources).Diagnostics()
	for i := 1; i < len(diags); i++ {
		if sortKey(diags[i-1]) > sortKey(diags[i]) {
			t.Errorf("diagnostics not canonically sorted at %d: %q then %q", i, sortKey(diags[i-1]), sortKey(diags[i]))
		}
	}
}

// multiBindingSrc is a RulesetBinding with two bindings, each requiring an
// uncovered obligation, so coverage emits two diagnostics whose relative order
// must be canonical (not binding-list / map order).
func multiBindingSrc() Source {
	return Source{
		Path: ".assent/bindings.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: RulesetBinding
bindings:
  - class: kafka-topic
    environment: prod
    packs: [p]
    risk: { threshold: 4 }
    require: [zeta, alpha]
  - class: kafka-topic
    environment: dev
    packs: [p]
    risk: { threshold: 10 }
    require: [alpha]
`),
	}
}

// TestIngestSkipsUnknownAndMalformedDocs exercises the tolerant-ingestion
// branches the REQ fixtures don't: an unrelated kind (skipped, a test fixture),
// an unparseable doc (one parse diagnostic), and a binding whose pack is absent
// (all its require[] obligations report uncovered).
func TestIngestSkipsUnknownAndMalformedDocs(t *testing.T) {
	rep := Lint([]Source{
		// unrelated kind — a test fixture, skipped silently.
		{Path: ".assent/tests/x/expect.yaml", Bytes: []byte("kind: TestExpectation\ndecision: approve\n")},
		// unparseable YAML — one parse diagnostic located to the file.
		{Path: ".assent/broken.yaml", Bytes: []byte("key: [unterminated\n")},
		// binding referencing a pack with no rule files present.
		bindingSrc("ownership"),
	})
	var haveParse, haveCoverage bool
	for _, d := range rep.Diagnostics() {
		switch d.Code {
		case CodeParseError:
			haveParse = true
			if d.Location.File != ".assent/broken.yaml" {
				t.Errorf("parse diagnostic mislocated: %q", d.Location.File)
			}
		case CodeObligationCoverage:
			haveCoverage = true
		}
	}
	if !haveParse {
		t.Error("an unparseable doc must yield a parse diagnostic")
	}
	if !haveCoverage {
		t.Error("a binding whose pack has no proving rule must report obligation-coverage")
	}
}
