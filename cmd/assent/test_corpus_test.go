package main

// test_corpus_test.go is the P5-E6-S08 EXIT GATE. It wires the S01–S07 `assent test`
// machinery into a dogfood gate over the SHIPPED example packs: every non-locked
// examples/packs/** pack gates itself GREEN under the real runTest entry point (REQ-01)
// and passes --coverage both-polarity (REQ-02); a deliberately-broken pack FAILS with
// the S04 expected/actual diff UX and a non-zero exit (REQ-03); the pack decisions
// reconcile with the archetype-goldens.md seed manifest (REQ-04); and the whole gate is
// deterministic with the correct exit codes over clean + broken corpora (REQ-05).
//
// It drives runTest (the SAME function `assent test` dispatches) so `go test ./...` — a
// job CI already runs — IS the example-pack dogfood job: the packs gate themselves in CI
// with no bespoke runner (ADR-0006 dogfooding). examplesPacksDir is defined in
// examples_packs_lint_test.go (the E3 lane-C corpus guard this gate builds on).
//
// CORPUS RECONCILIATION (Judgment call (d), D-061): the gate's discovery root is the
// ADOPTER `.assent/tests/**` format (expect.yaml), NOT the examples/archetypes/**
// expected.yaml seed manifest — the two are the known filename/root split. The manifest
// stays the P3-E3 golden seed; REQ-04 cross-checks the pack decisions against it.
//
// TOPIC-REGISTRY / D-052 (EFE-S04): topic-registry is IN the green corpus. Its
// non-destructive rule is re-authored to provable fileEvents{kinds:[add,delete]} with
// prove.when `kind != "delete"`; file-ADD proves and file-DELETE fails under assent test.
//
// E-FILEEVENTS EXIT GATE (EFE-S05): TestFileEventsCreateAndDeleteFixtures pins
// service-catalog CREATE+DELETE beyond topic-registry; TestFileEventsCorpusBothPolarityCoverage
// pins topic-registry + corpus-wide --coverage; TestFileEventsGateDoubleRun pins
// determinism + schemas/ frozen or D-088 presentation-only (E8-S02).

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/adoptertest"
	"github.com/PlatformRelay/assent/internal/schemadrift"
)

// greenExamplePacks are the non-locked example packs that gate themselves green under
// `assent test`. rego is locked (D-012) and carries no .assent/tests/** at all.
// Keep Taskfile.yml dogfood-examples + .github/workflows/verify.yaml dogfood step in
// sync (TestDogfoodScriptsIncludeGreenExamplePacks).
var greenExamplePacks = []string{"service-catalog", "infra-vars", "topic-registry"}

// brokenPackDir is the DELIBERATELY-broken fixture (a valid pack whose expect.yaml pins
// the wrong decision). It lives under testdata, never examples/packs, so the shipped
// corpus stays green while the failure path is still proven.
const brokenPackDir = "testdata/broken-pack"

// TestDogfoodScriptsIncludeGreenExamplePacks pins the CLI dogfood loops
// (Taskfile dogfood-examples + verify.yaml) to the same pack set as greenExamplePacks
// so unpinning a pack (EFE-S04 / topic-registry) cannot leave the shell loops stale.
func TestDogfoodScriptsIncludeGreenExamplePacks(t *testing.T) {
	files := []string{
		filepath.Join("..", "..", "Taskfile.yml"),
		filepath.Join("..", "..", ".github", "workflows", "verify.yaml"),
	}
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // fixed in-repo path relative to cmd/assent.
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		body := string(raw)
		for _, pack := range greenExamplePacks {
			// Match the shell `for pack in …` token list, not incidental prose.
			if !strings.Contains(body, pack) {
				t.Errorf("%s: dogfood loop missing green pack %q", f, pack)
			}
		}
	}
}

// TestAllExamplePacksGreenUnderAssentTest is REQ-E6-S08-01: every non-locked
// examples/packs/**/.assent/tests/** case evaluates via the whole-pack replay to its
// expect.yaml decision + findings (exit 0, every case PASS, no FAIL, clean stderr).
func TestAllExamplePacksGreenUnderAssentTest(t *testing.T) {
	for _, pack := range greenExamplePacks {
		pack := pack
		t.Run(pack, func(t *testing.T) {
			dir := filepath.Join(examplesPacksDir, pack)
			var so, se bytes.Buffer
			if code := runTest([]string{dir}, &so, &se); code != 0 {
				t.Fatalf("assent test %s: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", pack, code, so.String(), se.String())
			}
			if se.Len() != 0 {
				t.Errorf("%s: a green pack must emit no stderr, got:\n%s", pack, se.String())
			}
			if strings.Contains(so.String(), "FAIL") {
				t.Errorf("%s: a green pack must print no FAIL, got:\n%s", pack, so.String())
			}
		})
	}
}

// TestExampleCorpusBothPolarityCoverage is REQ-E6-S08-02: `--coverage` both-polarity
// passes across the whole example corpus (every enforce obligation rule proven-silent
// in >=1 case AND driven to fire in >=1 case).
func TestExampleCorpusBothPolarityCoverage(t *testing.T) {
	for _, pack := range greenExamplePacks {
		pack := pack
		t.Run(pack, func(t *testing.T) {
			dir := filepath.Join(examplesPacksDir, pack)
			var so, se bytes.Buffer
			if code := runTest([]string{"--coverage", dir}, &so, &se); code != 0 {
				t.Fatalf("assent test --coverage %s: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", pack, code, so.String(), se.String())
			}
			if !strings.Contains(so.String(), "coverage: OK") {
				t.Errorf("%s: --coverage must report OK, got:\n%s", pack, so.String())
			}
		})
	}
}

// TestDeliberatelyBrokenPackFailsWithDiff is REQ-E6-S08-03: a deliberately-broken pack
// fails with the S04 expected/actual diff UX AND a non-zero exit — the negative-path
// proof that the gate actually catches a regression (mirrors E3's negative fixture).
func TestDeliberatelyBrokenPackFailsWithDiff(t *testing.T) {
	var so, se bytes.Buffer
	code := runTest([]string{brokenPackDir}, &so, &se)
	if code == 0 {
		t.Fatalf("broken pack: exit = 0, want non-zero\nstdout:\n%s", so.String())
	}
	out := so.String()
	for _, want := range []string{
		"FAIL capped/within-cap",
		"decision: expected BLOCK, got APPROVE",
		"actual (ready to copy into expect.yaml)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("broken pack: diff UX missing %q; got:\n%s", want, out)
		}
	}
}

// TestTopicRegistryFileEventsGreen is REQ-EFE-S04-01 (closes D-052): topic-registry's
// re-authored non-destructive (kinds:[add,delete], when: kind != "delete") evaluates
// its file-DELETE case to REVIEW and its file-ADD proving case to APPROVE (satisfied/
// silent) under assent test. File-lifecycle fixtures use inline cases.yaml (null side).
func TestTopicRegistryFileEventsGreen(t *testing.T) {
	dir := filepath.Join(examplesPacksDir, "topic-registry")
	var so, se bytes.Buffer
	if code := runTest([]string{dir}, &so, &se); code != 0 {
		t.Fatalf("assent test topic-registry: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, so.String(), se.String())
	}
	out := so.String()
	for _, want := range []string{
		"PASS topics/non-destructive (",
		"PASS topics/non-destructive-delete (",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("topic-registry fileEvents green missing %q; got:\n%s", want, out)
		}
	}
	// Pin polarities from the authored inline expects so a silent flip cannot rot.
	if got := decisionOfInlineCase(t, "topic-registry", "non-destructive"); got != "APPROVE" {
		t.Errorf("file-ADD proving decision = %s, want APPROVE", got)
	}
	if got := decisionOfInlineCase(t, "topic-registry", "non-destructive-delete"); got != "REVIEW" {
		t.Errorf("file-DELETE failing decision = %s, want REVIEW", got)
	}
}

// TestExampleCorpusReconcilesArchetypeManifest is REQ-E6-S08-04 / REQ-EFE-S04-02:
// the examples/packs/** corpus decisions reconcile with archetype-goldens.md.
// D-061 is CLOSED by expressing BOTH shapes: entry-removal → REVIEW (service-catalog
// non-destructive) AND file-delete → BLOCK (manifest no-destruction), asserted so
// neither side can rot silently.
func TestExampleCorpusReconcilesArchetypeManifest(t *testing.T) {
	// Manifest (archetype-goldens.md v1): archetype obligation -> {prove, negative}.
	type gold struct{ prove, negative string }
	manifest := map[string]gold{
		"ownership":           {"APPROVE", "REVIEW"},
		"schema-validity":     {"APPROVE", "BLOCK"},
		"allow-listed-fields": {"APPROVE", "REVIEW"},
		"bounded-change":      {"APPROVE", "REVIEW"},
		"freshness":           {"APPROVE", "REVIEW"},
		"no-destruction":      {"APPROVE", "BLOCK"},
	}

	// Each pack case dir -> the archetype obligation it reconciles against.
	type ref struct {
		pack, caseDir, archetype string
	}
	refs := []ref{
		{"service-catalog", "catalog/ownership", "ownership"},
		{"service-catalog", "catalog/schema-valid", "schema-validity"},
		{"service-catalog", "catalog/allowed-fields", "allow-listed-fields"},
		{"service-catalog", "catalog/context-fresh", "freshness"},
		{"infra-vars", "vars/ownership", "ownership"},
		{"infra-vars", "vars/bounded-change", "bounded-change"},
	}

	for _, r := range refs {
		r := r
		t.Run(r.pack+"/"+r.archetype, func(t *testing.T) {
			g, ok := manifest[r.archetype]
			if !ok {
				t.Fatalf("archetype %q not in transcribed manifest", r.archetype)
			}
			if got := decisionOfPackCase(t, r.pack, r.caseDir); got != g.prove {
				t.Errorf("%s proving decision = %s, manifest %s = %s", r.caseDir, got, r.archetype, g.prove)
			}
			if got := decisionOfPackCase(t, r.pack, r.caseDir+"/negative"); got != g.negative {
				t.Errorf("%s/negative decision = %s, manifest %s negative = %s", r.caseDir, got, r.archetype, g.negative)
			}
		})
	}

	// D-061 reconciled (REQ-EFE-S04-02): TWO shapes, separately asserted.
	// (1) Entry-removal stays REVIEW (valueChanges require-review) — not the manifest's
	// file-delete BLOCK. (2) File-delete BLOCK matches archetype no-destruction/delete.
	t.Run("non-destructive entry-removal stays REVIEW", func(t *testing.T) {
		if got := decisionOfPackCase(t, "service-catalog", "catalog/non-destructive/negative"); got != "REVIEW" {
			t.Errorf("entry-removal negative = %s, want REVIEW (valueChanges require-review; distinct from file-delete BLOCK)", got)
		}
	})
	t.Run("non-destructive file-delete is BLOCK per manifest", func(t *testing.T) {
		g := manifest["no-destruction"]
		if got := decisionOfInlineCase(t, "service-catalog", "file-non-destructive"); got != g.prove {
			t.Errorf("file-non-destructive proving = %s, manifest no-destruction = %s", got, g.prove)
		}
		if got := decisionOfInlineCase(t, "service-catalog", "file-non-destructive-delete"); got != g.negative {
			t.Errorf("file-non-destructive-delete = %s, manifest no-destruction negative = %s (D-061)", got, g.negative)
		}
	})
}

// decisionOfPackCase reads an examples/packs/<pack>/.assent/tests/<rel>/expect.yaml
// decision (directory-form cases).
func decisionOfPackCase(t *testing.T, pack, rel string) string {
	t.Helper()
	p := filepath.Join(examplesPacksDir, pack, ".assent", "tests", rel, "expect.yaml")
	raw, err := os.ReadFile(p) //nolint:gosec // fixed in-repo example-pack fixture path.
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	exp, err := adoptertest.LoadExpectation(raw)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return exp.Decision
}

// decisionOfInlineCase reads the expect.decision for a named case inside
// examples/packs/<pack>/.assent/tests/<packSegment>/cases.yaml (E6-S06 shorthand).
// packSegment is the first path segment under tests/ (topics / catalog / vars).
func decisionOfInlineCase(t *testing.T, pack, caseName string) string {
	t.Helper()
	segment := map[string]string{
		"topic-registry":  "topics",
		"service-catalog": "catalog",
		"infra-vars":      "vars",
	}[pack]
	if segment == "" {
		t.Fatalf("no tests segment mapping for pack %q", pack)
	}
	p := filepath.Join(examplesPacksDir, pack, ".assent", "tests", segment, "cases.yaml")
	raw, err := os.ReadFile(p) //nolint:gosec // fixed in-repo example-pack fixture path.
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	var doc struct {
		Cases []struct {
			Name   string `yaml:"name"`
			Expect struct {
				Decision string `yaml:"decision"`
			} `yaml:"expect"`
		} `yaml:"cases"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	for _, c := range doc.Cases {
		if c.Name == caseName {
			return c.Expect.Decision
		}
	}
	t.Fatalf("%s: no case named %q", p, caseName)
	return ""
}

// TestFileEventsCreateAndDeleteFixtures is REQ-EFE-S05-01: a whole-file CREATE and
// a whole-file DELETE fixture (service-catalog — beyond topic-registry alone) each
// evaluate to the pinned decision under whole-pack replay. topic-registry's pair is
// pinned by TestTopicRegistryFileEventsGreen (EFE-S04); this gate pins the second
// pack so the domain cannot rot to a single-pack proof.
func TestFileEventsCreateAndDeleteFixtures(t *testing.T) {
	dir := filepath.Join(examplesPacksDir, "service-catalog")
	var so, se bytes.Buffer
	if code := runTest([]string{dir}, &so, &se); code != 0 {
		t.Fatalf("assent test service-catalog: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, so.String(), se.String())
	}
	out := so.String()
	for _, want := range []string{
		"PASS catalog/file-non-destructive (",
		"PASS catalog/file-non-destructive-delete (",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("fileEvents create/delete fixtures missing %q; got:\n%s", want, out)
		}
	}
	if got := decisionOfInlineCase(t, "service-catalog", "file-non-destructive"); got != "APPROVE" {
		t.Errorf("CREATE (file-ADD) proving decision = %s, want APPROVE", got)
	}
	if got := decisionOfInlineCase(t, "service-catalog", "file-non-destructive-delete"); got != "BLOCK" {
		t.Errorf("DELETE (file-DELETE) failing decision = %s, want BLOCK", got)
	}
}

// TestFileEventsCorpusBothPolarityCoverage is REQ-EFE-S05-02: topic-registry passes
// `assent test --coverage` both-polarity, and the whole green corpus does too
// (file-add proves + file-delete fails for non-destructive). Hardens the E6-S08
// corpus coverage gate with an explicit topic-registry pin after D-052 close.
func TestFileEventsCorpusBothPolarityCoverage(t *testing.T) {
	t.Run("topic-registry", func(t *testing.T) {
		dir := filepath.Join(examplesPacksDir, "topic-registry")
		var so, se bytes.Buffer
		if code := runTest([]string{"--coverage", dir}, &so, &se); code != 0 {
			t.Fatalf("assent test --coverage topic-registry: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, so.String(), se.String())
		}
		out := so.String()
		if !strings.Contains(out, "coverage: OK") {
			t.Errorf("topic-registry --coverage must report OK, got:\n%s", out)
		}
		// Both polarities of non-destructive must PASS under the coverage run.
		for _, want := range []string{
			"PASS topics/non-destructive (",
			"PASS topics/non-destructive-delete (",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("topic-registry coverage missing polarity %q; got:\n%s", want, out)
			}
		}
	})
	for _, pack := range greenExamplePacks {
		pack := pack
		t.Run("corpus/"+pack, func(t *testing.T) {
			dir := filepath.Join(examplesPacksDir, pack)
			var so, se bytes.Buffer
			if code := runTest([]string{"--coverage", dir}, &so, &se); code != 0 {
				t.Fatalf("assent test --coverage %s: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s", pack, code, so.String(), se.String())
			}
			if !strings.Contains(so.String(), "coverage: OK") {
				t.Errorf("%s: --coverage must report OK, got:\n%s", pack, so.String())
			}
		})
	}
}

// TestFileEventsGateDoubleRun is REQ-EFE-S05-03: the whole fileEvents gate
// double-runs byte-identical; schemas/ is unchanged (git diff schemas/ == 0).
func TestFileEventsGateDoubleRun(t *testing.T) {
	run := func(args []string) string {
		var so, se bytes.Buffer
		runTest(args, &so, &se)
		return so.String() + "\x00" + se.String()
	}
	// FileEvents-bearing packs (CREATE+DELETE fixtures): service-catalog + topic-registry.
	fileEventsPacks := []string{"service-catalog", "topic-registry"}
	for _, pack := range fileEventsPacks {
		dir := filepath.Join(examplesPacksDir, pack)
		if a, b := run([]string{dir}), run([]string{dir}); a != b {
			t.Errorf("%s: assent test not double-run stable", pack)
		}
		if a, b := run([]string{"--coverage", dir}), run([]string{"--coverage", dir}); a != b {
			t.Errorf("%s: assent test --coverage not double-run stable", pack)
		}
	}
	// Epic DoD: schemas/ unchanged except D-088 presentation block (E8-S02).
	repoRoot := filepath.Join("..", "..")
	if err := schemadrift.CheckGitFrozenOrD088PresentationOnly(repoRoot); err != nil {
		t.Fatalf("schema drift: %v", err)
	}
}

// TestAssentTestGateDoubleRun is half of REQ-E6-S08-05: the whole gate double-runs
// byte-identical (ADR-0014 determinism; no clock/env/net/random in the decision path).
func TestAssentTestGateDoubleRun(t *testing.T) {
	run := func(args []string) string {
		var so, se bytes.Buffer
		runTest(args, &so, &se)
		return so.String() + "\x00" + se.String()
	}
	for _, pack := range greenExamplePacks {
		dir := filepath.Join(examplesPacksDir, pack)
		if a, b := run([]string{dir}), run([]string{dir}); a != b {
			t.Errorf("%s: assent test not double-run stable", pack)
		}
		if a, b := run([]string{"--coverage", dir}), run([]string{"--coverage", dir}); a != b {
			t.Errorf("%s: assent test --coverage not double-run stable", pack)
		}
	}
	if a, b := run([]string{brokenPackDir}), run([]string{brokenPackDir}); a != b {
		t.Errorf("broken pack: diff UX not double-run stable")
	}
}

// TestAssentTestExitCodes is half of REQ-E6-S08-05: exit codes are correct end-to-end
// over clean + broken corpora — 0 for a green pack, non-zero (1) for the broken pack,
// 2 for a usage error (no directory argument).
func TestAssentTestExitCodes(t *testing.T) {
	for _, pack := range greenExamplePacks {
		var so, se bytes.Buffer
		if code := runTest([]string{filepath.Join(examplesPacksDir, pack)}, &so, &se); code != 0 {
			t.Errorf("%s: exit = %d, want 0", pack, code)
		}
	}
	var so, se bytes.Buffer
	if code := runTest([]string{brokenPackDir}, &so, &se); code != 1 {
		t.Errorf("broken pack: exit = %d, want 1", code)
	}
	so.Reset()
	se.Reset()
	if code := runTest(nil, &so, &se); code != 2 {
		t.Errorf("no-arg usage: exit = %d, want 2", code)
	}
}
