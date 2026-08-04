package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// shrinkPolicy returns a MergePolicy + Binding of the D-016
// `partitions-must-not-shrink` shape: a valueChanges /partitions modify rule that
// proves `non-destructive` via the BARE `new >= old` (no int() coercion — the
// D-016 predicate), blocking on failure with points 10. The binding requires only
// `non-destructive` so the decision is driven solely by the numeric compare.
func shrinkPolicy() (*policy.MergePolicy, *policy.Binding) {
	mp := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:  "partitions-must-not-shrink",
				Phase: policy.PhaseEnforce,
				Match: policy.Match{ValueChanges: &policy.ValueChangesMatch{
					Pointers: []string{"/partitions"},
					Kinds:    []string{"modify"},
				}},
				Prove: &policy.Prove{
					Obligation: "non-destructive",
					When:       policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}},
				},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "partition-count-shrunk"},
				Points:    10,
			}},
		},
	}
	bind := &policy.Binding{
		Class:       "topic-registry",
		Environment: "prod",
		Risk:        policy.Risk{Threshold: 1},
		Require:     []string{"non-destructive"},
	}
	return mp, bind
}

// TestDecodeCanonical proves the pure decoder INVERTS internal/change's canonical
// render into the typed value the engine compares: numeric literals -> json.Number
// (so a `new >= old` is numeric), a JSON-quoted !!str -> a bare Go string (so a
// numeric rule does NOT lexically compare it), and bool/null/absent -> their types.
func TestDecodeCanonical(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want any
	}{
		{"int literal", "12", json.Number("12")},
		{"int shrink target", "6", json.Number("6")},
		{"octal-looking literal kept raw", "016", json.Number("016")},
		{"float literal", "3.5", json.Number("3.5")},
		{"negative literal", "-4", json.Number("-4")},
		{"bool true", "true", true},
		{"bool false", "false", false},
		{"null", "null", nil},
		{"absent add/delete side", "", nil},
		{"quoted string stays a string", `"12"`, "12"},
		{"quoted word stays a string", `"prod"`, "prod"},
		// Documented S02 limitation (NOT this lane's bug): an >int64 literal flows
		// through faithfully as json.Number; toCEL then falls to float64/string.
		{"over-int64 literal kept as json.Number", "18446744073709551617", json.Number("18446744073709551617")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeCanonical(tc.in)
			if got != tc.want {
				t.Fatalf("decodeCanonical(%q) = %#v (%T), want %#v (%T)", tc.in, got, got, tc.want, tc.want)
			}
		})
	}
}

// TestRealDiffNumericShrinkBlocks is THE GATE. S10 fed a pre-typed
// evaluation-input.json, BYPASSING the decoder; this drives the ACTUAL differ. A
// real base/head YAML pair (partitions 12 -> 6, the D-016 shape) runs through
// change.Diff -> the decoder -> end-to-end CoverWithApproval, and MUST BLOCK,
// proving (a) the decoded Old/New are TYPED json.Number, and (b) `new >= old` is a
// NUMERIC compare (6 >= 12 is false), never the lexical "6" >= "12" (true) that
// would APPROVE a shrink. TestStringOldNewFailsOpen is the paired mutation proof.
func TestRealDiffNumericShrinkBlocks(t *testing.T) {
	path := "topics/prod/orders-events.yaml"
	base := readShrinkFixture(t, "base", path)
	head := readShrinkFixture(t, "head", path)

	cs, err := change.Diff(path, base, head)
	if err != nil {
		t.Fatalf("change.Diff: %v", err)
	}
	if len(cs.Changes) != 1 {
		t.Fatalf("changes = %d (%+v), want exactly 1 (/partitions modify)", len(cs.Changes), cs.Changes)
	}
	// The differ's canonical render: an int change carries the RAW literal strings.
	if c := cs.Changes[0]; c.Path != "/partitions" || c.Old != "12" || c.New != "6" {
		t.Fatalf("change = %+v, want /partitions old=\"12\" new=\"6\" (canonical strings)", c)
	}

	in := buildEvaluationInput(cs, aggregate.MR{}, []string{"non-destructive"})

	// (a) The decoded Old/New are TYPED json.Number — not the raw canonical strings.
	got := in.ChangeSet.Changes[0]
	if got.Old != json.Number("12") || got.New != json.Number("6") {
		t.Fatalf("decoded old/new = %#v/%#v, want json.Number(\"12\")/json.Number(\"6\")", got.Old, got.New)
	}

	// (b) End-to-end: numeric 6 >= 12 is false -> `non-destructive` unproven -> the
	// block onFailure fires -> BLOCK. A shrink must never reach APPROVE.
	mp, bind := shrinkPolicy()
	res, err := aggregate.CoverWithApproval(mp, bind, &in, nil)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	if res.Decision != aggregate.DecisionBlock {
		t.Fatalf("decision = %q, want BLOCK (a partition shrink 12->6 must never APPROVE)", res.Decision)
	}
	assertHasFinding(t, res.Findings, aggregate.EffectBlock, "partition-count-shrunk")
}

// TestStringOldNewFailsOpen is the MUTATION proof that the decoder is load-bearing.
// With the SAME rule but Old/New left as the RAW canonical strings "12"/"6" (the
// un-decoded shape), `new >= old` is a LEXICAL compare and "6" >= "12" is TRUE — the
// obligation "proves", nothing fires, and the shrink APPROVEs. That is exactly the
// forbidden outcome decodeCanonical closes; if this ever stops APPROVing, the engine
// changed and the gate above no longer demonstrates the decoder matters.
func TestStringOldNewFailsOpen(t *testing.T) {
	in := aggregate.EvaluationInput{
		ChangeSet: aggregate.ChangeSet{Changes: []aggregate.EvalChange{{
			Subject: "file:topics/prod/orders-events.yaml",
			File:    "topics/prod/orders-events.yaml",
			Path:    "/partitions",
			Kind:    "modify",
			Old:     "12", // RAW canonical string (the fail-open shape the decoder replaces)
			New:     "6",
		}}},
		Facts:   map[string]map[string]aggregate.Fact{},
		Require: []string{"non-destructive"},
	}
	mp, bind := shrinkPolicy()
	res, err := aggregate.CoverWithApproval(mp, bind, &in, nil)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	if res.Decision != aggregate.DecisionApprove {
		t.Fatalf("decision = %q, want APPROVE — this test DOCUMENTS the lexical fail-open (\"6\" >= \"12\" is true) that decodeCanonical closes; a non-APPROVE here means the mutation proof is stale", res.Decision)
	}
}

func readShrinkFixture(t *testing.T, side, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "shrink-diff", side, filepath.FromSlash(path))) //nolint:gosec // fixed test fixture path, not user input.
	if err != nil {
		t.Fatalf("read %s fixture %s: %v", side, path, err)
	}
	return b
}

func assertHasFinding(t *testing.T, findings []aggregate.Finding, effect aggregate.Effect, code string) {
	t.Helper()
	for _, f := range findings {
		if f.Effect == effect && f.Code == code {
			return
		}
	}
	t.Fatalf("no finding with effect=%q code=%q in %+v", effect, code, findings)
}
