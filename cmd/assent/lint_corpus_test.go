package main

// lint_corpus_test.go is the cmd-tier half of the E3-S08 exit gate: the archetype
// LOAD + EVALUATE gate (REQ-E3-S08-02) and the end-to-end exit-code / double-run
// gate (REQ-E3-S08-05). It reuses the SAME cmd-tier loader `assent catalogue`/
// `assent lint` use (loadCatalogueInput) and the SAME E2 decision engine
// (aggregate.Cover) — no bespoke re-implementation.
//
// # Why this is a per-obligation Cover gate, not a base/head fixture replay
//
// REQ-E3-S08-02 asks that every non-locked archetype pack "loads + evaluates via
// Cover to its expected decision". The conformant packs (service-catalog +
// infra-vars; topic-registry pinned/excluded per D-052) carry `.assent/tests/**`
// base/head/facts/expect fixtures — but those are the E6 ADOPTER test format
// (`assent test`, an explicit E3 non-goal). They do NOT map directly onto an
// aggregate.EvaluationInput, for two concrete reasons discovered here and reported
// rather than fabricated around (per the story's "report which + why" instruction):
//
//   1. FACT SHAPE. The fixtures' facts.yaml use the AUTHORED adopter shape
//      (`author: {groups: [...]}`, `schema: {valid: true}`) — NOT the resolved-fact
//      ENVELOPE (`{state: resolved, value: ...}`) that aggregate.factsToCEL/Fact
//      require. Translating one to the other is the E5 fact-resolution + E6 harness.
//   2. ENTRY-TREE BINDING. aggregate.bindLeafActivation binds `entry`/`oldEntry`
//      to a change's New/Old value (a DOCUMENTED approximation — per-EntryRef
//      entry-tree reconstruction is explicitly deferred). So an entry-scoped
//      `files` rule (ownership: `entry.owner in facts...`) and a `valueChanges`
//      leaf rule (bounded-change: `new >= facts...`) cannot both be satisfied over
//      ONE shared changeset — the entry rule needs New = the entry object, the leaf
//      rule needs New = the typed scalar. Reproducing a whole multi-rule pack's
//      decision over one EvaluationInput is thus blocked until E6 lands the
//      differ→evalinput assembler + entry reconstruction.
//
// So the gate LOADS every conformant pack through the E2 strict loader, then drives
// each LOADED rule through aggregate.Cover (a single-rule policy + a binding
// requiring that obligation) with a faithful proving + failing EvaluationInput,
// asserting the produced Decision reproduces the archetype golden. Every obligation
// (ownership, schema-valid, context-fresh, allowed-fields, non-destructive,
// bounded-change) is exercised on the REAL loaded rule through the REAL engine, and
// every produced decision matches archetype-goldens.md.

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// lintFixturesDir is the hard-error corpus, relative to this package (cmd/assent).
const lintFixturesDir = "../../examples/lint-fixtures"

// rf is a RESOLVED provider fact carrying value v — the only state that exposes a
// `.value` binding (aggregate.factsToCEL). Facts authored in the E6 fixtures are
// translated into this envelope shape (see the file header).
func rf(v any) aggregate.Fact { return aggregate.Fact{State: "resolved", Value: v} }

// archCase drives one LOADED rule through aggregate.Cover and asserts the produced
// decision reproduces the archetype golden (or the documented engine truth).
type archCase struct {
	name       string // sub-test name
	starter    string // examples/packs/<starter>
	rule       string // loaded rule name to evaluate
	obligation string // the obligation the binding requires
	facts      map[string]map[string]aggregate.Fact
	change     aggregate.EvalChange
	want       aggregate.Decision
}

// scFile / ivFile are the governed subject files each pack's rules match.
const (
	scFile = "catalog/prod/core-services.json"
	ivFile = "envs/prod/compute.tfvars"
)

// TestArchetypePacksEvaluateToExpectedDecision is the REQ-E3-S08-02 gate.
func TestArchetypePacksEvaluateToExpectedDecision(t *testing.T) {
	cases := []archCase{
		// -------- service-catalog (pack catalog) --------
		{
			name: "ownership/prove", starter: "service-catalog", rule: "author-owns-entry", obligation: "ownership",
			facts:  map[string]map[string]aggregate.Fact{"author": {"groups": rf([]any{"orders-team"})}},
			change: aggregate.EvalChange{Subject: "catalog-service:orders-api", File: scFile, Path: "/services/orders-api/owner", Kind: "modify", New: map[string]any{"owner": "orders-team"}, Old: map[string]any{"owner": "orders-team"}},
			want:   aggregate.DecisionApprove,
		},
		{
			name: "ownership/reject", starter: "service-catalog", rule: "author-owns-entry", obligation: "ownership",
			facts:  map[string]map[string]aggregate.Fact{"author": {"groups": rf([]any{"orders-team"})}},
			change: aggregate.EvalChange{Subject: "catalog-service:storefront-web", File: scFile, Path: "/services/storefront-web/owner", Kind: "modify", New: map[string]any{"owner": "storefront-team"}, Old: map[string]any{"owner": "orders-team"}},
			want:   aggregate.DecisionReview,
		},
		{
			name: "schema-valid/prove", starter: "service-catalog", rule: "catalog-v1", obligation: "schema-valid",
			facts:  map[string]map[string]aggregate.Fact{"schema": {"valid": rf(true)}},
			change: aggregate.EvalChange{Subject: "catalog-service:orders-api", File: scFile, Path: "/services/orders-api/tier", Kind: "modify", New: int64(1), Old: int64(1)},
			want:   aggregate.DecisionApprove,
		},
		{
			name: "schema-valid/block", starter: "service-catalog", rule: "catalog-v1", obligation: "schema-valid",
			facts:  map[string]map[string]aggregate.Fact{"schema": {"valid": rf(false)}},
			change: aggregate.EvalChange{Subject: "catalog-service:orders-api", File: scFile, Path: "/services/orders-api/name", Kind: "modify", New: "renamed", Old: "orders-api"},
			want:   aggregate.DecisionBlock,
		},
		{
			name: "context-fresh/prove", starter: "service-catalog", rule: "oncall-exists", obligation: "context-fresh",
			facts:  map[string]map[string]aggregate.Fact{"oncall": {"orders_rotation": {State: "resolved"}}},
			change: aggregate.EvalChange{Subject: "catalog-service:orders-api", File: scFile, Path: "/services/orders-api/oncall", Kind: "modify", New: "orders-rotation", Old: "orders-rotation"},
			want:   aggregate.DecisionApprove,
		},
		{
			// An expired CONTROLLING context fact fails to ARM the change →
			// require-review (ADR-0017 §3/§4), matching archetype-goldens.md:27.
			name: "context-fresh/review", starter: "service-catalog", rule: "oncall-exists", obligation: "context-fresh",
			facts:  map[string]map[string]aggregate.Fact{"oncall": {"orders_rotation": {State: "expired"}}},
			change: aggregate.EvalChange{Subject: "catalog-service:orders-api", File: scFile, Path: "/services/orders-api/oncall", Kind: "modify", New: "orders-rotation", Old: "orders-rotation"},
			want:   aggregate.DecisionReview,
		},
		{
			// allowed-fields positive: only a safe field (endpoints) changed — the
			// /owner,/tier valueChanges rule does not match, so nothing fires.
			name: "allowed-fields/safe-field", starter: "service-catalog", rule: "only-safe-fields", obligation: "allowed-fields",
			change: aggregate.EvalChange{Subject: "catalog-service:orders-api", File: scFile, Path: "/services/orders-api/endpoints", Kind: "modify", New: "b", Old: "a"},
			want:   aggregate.DecisionApprove,
		},
		{
			name: "allowed-fields/sensitive", starter: "service-catalog", rule: "only-safe-fields", obligation: "allowed-fields",
			change: aggregate.EvalChange{Subject: "catalog-service:orders-api", File: scFile, Path: "/owner", Kind: "modify", New: "payments-team", Old: "orders-team"},
			want:   aggregate.DecisionReview,
		},
		{
			name: "non-destructive/reorder", starter: "service-catalog", rule: "no-entry-removal", obligation: "non-destructive",
			change: aggregate.EvalChange{Subject: "catalog-service:services", File: scFile, Path: "/services", Kind: "modify", New: []any{int64(1), int64(2), int64(3)}, Old: []any{int64(3), int64(2), int64(1)}},
			want:   aggregate.DecisionApprove,
		},
		{
			name: "non-destructive/removal", starter: "service-catalog", rule: "no-entry-removal", obligation: "non-destructive",
			change: aggregate.EvalChange{Subject: "catalog-service:services", File: scFile, Path: "/services", Kind: "modify", New: []any{int64(1), int64(2)}, Old: []any{int64(1), int64(2), int64(3)}},
			want:   aggregate.DecisionReview,
		},
		// -------- infra-vars (pack vars) --------
		{
			name: "vars-ownership/prove", starter: "infra-vars", rule: "author-owns-entry", obligation: "ownership",
			facts:  map[string]map[string]aggregate.Fact{"author": {"groups": rf([]any{"orders-team"})}},
			change: aggregate.EvalChange{Subject: "workload:orders-api", File: ivFile, Path: "/workloads/orders-api/owner", Kind: "modify", New: map[string]any{"owner": "orders-team"}, Old: map[string]any{"owner": "orders-team"}},
			want:   aggregate.DecisionApprove,
		},
		{
			name: "vars-ownership/reject", starter: "infra-vars", rule: "author-owns-entry", obligation: "ownership",
			facts:  map[string]map[string]aggregate.Fact{"author": {"groups": rf([]any{"payments-team"})}},
			change: aggregate.EvalChange{Subject: "workload:orders-api", File: ivFile, Path: "/workloads/orders-api/owner", Kind: "modify", New: map[string]any{"owner": "orders-team"}, Old: map[string]any{"owner": "orders-team"}},
			want:   aggregate.DecisionReview,
		},
		{
			name: "bounded-change/in-band", starter: "infra-vars", rule: "memory-mb-bounds", obligation: "bounded-change",
			facts:  map[string]map[string]aggregate.Fact{"band": {"memory_mb": rf(map[string]any{"min": int64(512), "max": int64(4096)})}},
			change: aggregate.EvalChange{Subject: "workload:orders-api", File: ivFile, Path: "/memory_mb", Kind: "modify", New: int64(3072), Old: int64(2048)},
			want:   aggregate.DecisionApprove,
		},
		{
			name: "bounded-change/over-band", starter: "infra-vars", rule: "memory-mb-bounds", obligation: "bounded-change",
			facts:  map[string]map[string]aggregate.Fact{"band": {"memory_mb": rf(map[string]any{"min": int64(512), "max": int64(4096)})}},
			change: aggregate.EvalChange{Subject: "workload:orders-api", File: ivFile, Path: "/memory_mb", Kind: "modify", New: int64(65536), Old: int64(2048)},
			want:   aggregate.DecisionReview,
		},
	}

	// Load each conformant pack once via the E2 strict loader (proves the "loads"
	// half of REQ-E3-S08-02), indexing its rules + prod binding.
	loaded := map[string]loadedStarter{}
	for _, starter := range []string{"service-catalog", "infra-vars"} {
		loaded[starter] = loadStarter(t, starter)
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			ls := loaded[c.starter]
			rule, ok := ls.rules[c.rule]
			if !ok {
				t.Fatalf("rule %q not found in loaded pack %q", c.rule, c.starter)
			}
			// A single-rule policy over the LOADED rule + a binding requiring exactly
			// this obligation (so an unrelated required obligation does not add a
			// fail-safe REVIEW), driven through the real E2 engine.
			pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{rule}}}
			bind := &policy.Binding{
				Class:       ls.prod.Class,
				Environment: "prod",
				Packs:       ls.prod.Packs,
				Risk:        ls.prod.Risk,
				Require:     []string{c.obligation},
			}
			in := &aggregate.EvaluationInput{
				ChangeSet: aggregate.ChangeSet{Changes: []aggregate.EvalChange{c.change}},
				Facts:     c.facts,
			}
			res, err := aggregate.Cover(pol, bind, in)
			if err != nil {
				t.Fatalf("Cover: %v", err)
			}
			if res.Decision != c.want {
				t.Errorf("rule %q: Cover decision = %s, want %s\n findings=%+v", c.rule, res.Decision, c.want, res.Findings)
			}
			// Determinism: the same decision twice, byte-identical.
			again, _ := aggregate.Cover(pol, bind, in)
			if again.Decision != res.Decision {
				t.Errorf("rule %q: Cover not double-run stable: %s vs %s", c.rule, res.Decision, again.Decision)
			}
		})
	}
}

// loadedStarter is a conformant pack loaded via the E2 strict loader: its rules by
// name and its prod binding.
type loadedStarter struct {
	rules map[string]policy.Rule
	prod  policy.Binding
}

// loadStarter loads examples/packs/<starter> via loadCatalogueInput (the same
// cmd-tier strict loader `assent catalogue` uses) and indexes its rules + prod
// binding. A load failure fails the test — proving the pack is lane-C conformant.
func loadStarter(t *testing.T, starter string) loadedStarter {
	t.Helper()
	in, err := loadCatalogueInput(filepath.Join(examplesPacksDir, starter))
	if err != nil {
		t.Fatalf("%s: strict loader rejected a document: %v", starter, err)
	}
	rules := map[string]policy.Rule{}
	for _, pk := range in.Packs {
		for _, mp := range pk.Policies {
			for _, r := range mp.Spec.Rules {
				rules[r.Name] = r
			}
		}
	}
	if len(rules) == 0 {
		t.Fatalf("%s: loaded no rules", starter)
	}
	var prod policy.Binding
	found := false
	for _, rb := range in.Bindings {
		for _, b := range rb.Bindings {
			if b.Environment == "prod" {
				prod = b
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("%s: no prod binding found", starter)
	}
	return loadedStarter{rules: rules, prod: prod}
}

// TestArchetypeCorpusExitCodesAndDoubleRun is the end-to-end half of
// REQ-E3-S08-05: `assent lint` returns the correct exit code over the corpus (0 on
// a clean tree, 1 on a violating tree) and its diagnostics are byte-identical
// across two runs; `assent catalogue` exits 0 over a conformant pack and its report
// is byte-identical across two runs.
func TestArchetypeCorpusExitCodesAndDoubleRun(t *testing.T) {
	// A representative clean tree exits 0; a violating tree exits 1.
	good := filepath.Join(lintFixturesDir, "obligation-coverage", "good")
	bad := filepath.Join(lintFixturesDir, "obligation-coverage", "bad")

	var so, se bytes.Buffer
	if code := runLint([]string{good}, &so, &se); code != 0 {
		t.Errorf("clean fixture: runLint exit = %d, want 0; stderr=%s", code, se.String())
	}

	// Violating tree: exit 1 AND byte-identical stderr across two runs.
	var so1, se1, so2, se2 bytes.Buffer
	code1 := runLint([]string{bad}, &so1, &se1)
	code2 := runLint([]string{bad}, &so2, &se2)
	if code1 != 1 || code2 != 1 {
		t.Errorf("violating fixture: runLint exits = %d,%d, want 1,1", code1, code2)
	}
	if se1.String() != se2.String() {
		t.Errorf("lint diagnostics not double-run stable:\n a=%q\n b=%q", se1.String(), se2.String())
	}

	// Catalogue over a conformant pack exits 0 and is byte-identical across runs.
	pack := filepath.Join(examplesPacksDir, "service-catalog")
	var c1o, c1e, c2o, c2e bytes.Buffer
	if code := runCatalogue([]string{pack}, &c1o, &c1e); code != 0 {
		t.Errorf("catalogue: runCatalogue exit = %d, want 0; stderr=%s", code, c1e.String())
	}
	_ = runCatalogue([]string{pack}, &c2o, &c2e)
	if c1o.String() != c2o.String() {
		t.Errorf("catalogue report not double-run stable across two generations")
	}
}
