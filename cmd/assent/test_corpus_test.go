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
// TOPIC-REGISTRY / D-052: topic-registry is EXCLUDED from the green corpus. It is
// `mode: document` and its non-destructive rule needs the E1-DEFERRED fileEvents domain,
// so the strict loader HARD-REJECTS it (asserted by TestExamplesPacksKnownBlockers). It
// stays the pinned known-blocker — the tracked corpus gap, NOT deleted.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
)

// greenExamplePacks are the non-locked example packs that gate themselves green under
// `assent test`. topic-registry is excluded (D-052, see file header); rego is locked
// (D-012) and carries no .assent/tests/** at all.
var greenExamplePacks = []string{"service-catalog", "infra-vars"}

// brokenPackDir is the DELIBERATELY-broken fixture (a valid pack whose expect.yaml pins
// the wrong decision). It lives under testdata, never examples/packs, so the shipped
// corpus stays green while the failure path is still proven.
const brokenPackDir = "testdata/broken-pack"

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

// TestExampleCorpusReconcilesArchetypeManifest is REQ-E6-S08-04: the examples/packs/**
// corpus decisions reconcile with the docs/planning/archetype-goldens.md seed manifest.
// The manifest is transcribed here (its Manifest version 1 table); each mapped
// obligation's proving + negative expect.yaml decision MUST equal the manifest's, so a
// drift on either side is caught. The ONE logged divergence (D-061) is documented and
// asserted so it can never rot silently: service-catalog's non-destructive is
// ENTRY-removal (require-review) whereas the manifest's no-destruction is FILE-level
// deletion (block) — the deferred fileEvents domain (D-052 / topic-registry), a
// different, stricter obligation.
func TestExampleCorpusReconcilesArchetypeManifest(t *testing.T) {
	// Manifest (archetype-goldens.md v1): archetype obligation -> {prove, negative}.
	type gold struct{ prove, negative string }
	manifest := map[string]gold{
		"ownership":           {"APPROVE", "REVIEW"},
		"schema-validity":     {"APPROVE", "BLOCK"},
		"allow-listed-fields": {"APPROVE", "REVIEW"},
		"bounded-change":      {"APPROVE", "REVIEW"},
		"freshness":           {"APPROVE", "REVIEW"},
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

	decisionOf := func(t *testing.T, pack, rel string) string {
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

	for _, r := range refs {
		r := r
		t.Run(r.pack+"/"+r.archetype, func(t *testing.T) {
			g, ok := manifest[r.archetype]
			if !ok {
				t.Fatalf("archetype %q not in transcribed manifest", r.archetype)
			}
			if got := decisionOf(t, r.pack, r.caseDir); got != g.prove {
				t.Errorf("%s proving decision = %s, manifest %s = %s", r.caseDir, got, r.archetype, g.prove)
			}
			if got := decisionOf(t, r.pack, r.caseDir+"/negative"); got != g.negative {
				t.Errorf("%s/negative decision = %s, manifest %s negative = %s", r.caseDir, got, r.archetype, g.negative)
			}
		})
	}

	// The ONE logged divergence (D-061): non-destructive (entry-removal, REVIEW) vs the
	// manifest's no-destruction (file-deletion, BLOCK). Asserted so a future change to
	// either side surfaces here rather than silently diverging further.
	t.Run("non-destructive divergence is logged", func(t *testing.T) {
		if got := decisionOf(t, "service-catalog", "catalog/non-destructive/negative"); got != "REVIEW" {
			t.Errorf("non-destructive/negative = %s, want REVIEW (entry-removal require-review; the manifest's file-level no-destruction is BLOCK — deferred fileEvents, D-052)", got)
		}
	})
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
