package evaldecode_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/evaldecode"
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
		// go-yaml resolves the capitalized spellings as !!bool and the differ emits
		// the raw literal — these MUST decode to a Go bool, not json.Number/string.
		{"bool True", "True", true},
		{"bool TRUE", "TRUE", true},
		{"bool False", "False", false},
		{"bool FALSE", "FALSE", false},
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
			got := evaldecode.DecodeCanonical(tc.in)
			if got != tc.want {
				t.Fatalf("evaldecode.DecodeCanonical(%q) = %#v (%T), want %#v (%T)", tc.in, got, got, tc.want, tc.want)
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

	in := evaldecode.BuildEvaluationInput(cs, aggregate.MR{}, []string{"non-destructive"})

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

// TestUndecodedStringOldNewFailsSafe is the MUTATION proof that the decoder is
// load-bearing — rewritten for D-129. It used to assert that the un-decoded shape
// APPROVEs a shrink (documenting the lexical fail-open as the thing the decoder
// closes). That fail-open is now closed a SECOND time, at the engine seam: a
// relational compare over two string-bound operands errors instead of answering
// lexically. So the mutation no longer flips the decision to APPROVE — it degrades
// it to the fail-safe REVIEW.
//
// The proof stays DISCRIMINATING by asserting the pair, not merely "not APPROVE":
// decoded (json.Number) -> BLOCK + partition-count-shrunk (the policy's real
// answer); un-decoded (raw canonical strings) -> REVIEW + predicate.error. Delete
// the decoder and the first arm fails; delete the engine guard and the second arm
// goes back to APPROVE. Neither layer can rot silently.
func TestUndecodedStringOldNewFailsSafe(t *testing.T) {
	mkInput := func(oldVal, newVal any) aggregate.EvaluationInput {
		return aggregate.EvaluationInput{
			ChangeSet: aggregate.ChangeSet{Changes: []aggregate.EvalChange{{
				Subject: "file:topics/prod/orders-events.yaml",
				File:    "topics/prod/orders-events.yaml",
				Path:    "/partitions",
				Kind:    "modify",
				Old:     oldVal,
				New:     newVal,
			}}},
			Facts:   map[string]map[string]aggregate.Fact{},
			Require: []string{"non-destructive"},
		}
	}
	mp, bind := shrinkPolicy()

	decoded := mkInput(json.Number("12"), json.Number("6"))
	resDecoded, err := aggregate.CoverWithApproval(mp, bind, &decoded, nil)
	if err != nil {
		t.Fatalf("CoverWithApproval (decoded): %v", err)
	}
	if resDecoded.Decision != aggregate.DecisionBlock {
		t.Fatalf("decoded decision = %q, want BLOCK — the typed numeric compare 6 >= 12 is false", resDecoded.Decision)
	}
	assertHasFinding(t, resDecoded.Findings, aggregate.EffectBlock, "partition-count-shrunk")

	undecoded := mkInput("12", "6") // RAW canonical strings: the un-decoded shape
	resStr, err := aggregate.CoverWithApproval(mp, bind, &undecoded, nil)
	if err != nil {
		t.Fatalf("CoverWithApproval (un-decoded): %v", err)
	}
	if resStr.Decision == aggregate.DecisionApprove {
		t.Fatalf("un-decoded decision = APPROVE — the lexical fail-open (\"6\" >= \"12\" is true) is OPEN again")
	}
	if resStr.Decision != aggregate.DecisionReview {
		t.Fatalf("un-decoded decision = %q, want REVIEW (the relational-over-text guard fails safe)", resStr.Decision)
	}
	assertHasFinding(t, resStr.Findings, aggregate.EffectRequireReview, "predicate.error")
}

// TestQuotedNumericShrinkFailsSafeEndToEnd is the REACHABLE half of D-129, driven
// through the whole production chain (change.Diff -> DecodeCanonical -> Cover) on
// a real base/head YAML pair whose `partitions` is QUOTED — routine in adopter
// YAML. The decoder keeps a !!str a Go string BY DESIGN (a quoted "12" is not the
// number 12), so before the fix the D-016 rule's bare `new >= old` became the
// lexical "6" >= "12" = TRUE: `non-destructive` "proved", nothing fired, and a
// partition shrink came out APPROVE with zero findings. The compare must error.
func TestQuotedNumericShrinkFailsSafeEndToEnd(t *testing.T) {
	path := "topics/prod/orders-events.yaml"
	base := readFixture(t, "shrink-diff-quoted", "base", path)
	head := readFixture(t, "shrink-diff-quoted", "head", path)

	cs, err := change.Diff(path, base, head)
	if err != nil {
		t.Fatalf("change.Diff: %v", err)
	}
	if len(cs.Changes) != 1 {
		t.Fatalf("changes = %d (%+v), want exactly 1 (/partitions modify)", len(cs.Changes), cs.Changes)
	}
	// The differ's canonical render JSON-QUOTES a !!str, discriminating it from the
	// !!int literal — that tag discrimination is what makes the value a Go string.
	if c := cs.Changes[0]; c.Path != "/partitions" || c.Old != `"12"` || c.New != `"6"` {
		t.Fatalf("change = %+v, want /partitions old=%q new=%q (JSON-quoted !!str render)", c, `"12"`, `"6"`)
	}

	in := evaldecode.BuildEvaluationInput(cs, aggregate.MR{}, []string{"non-destructive"})
	got := in.ChangeSet.Changes[0]
	if got.Old != "12" || got.New != "6" {
		t.Fatalf("decoded old/new = %#v/%#v, want the bare Go strings \"12\"/\"6\" (a !!str stays a string)", got.Old, got.New)
	}

	mp, bind := shrinkPolicy()
	res, err := aggregate.CoverWithApproval(mp, bind, &in, nil)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	if res.Decision == aggregate.DecisionApprove {
		t.Fatalf("decision = APPROVE with findings %+v — a quoted-numeric partition shrink 12->6 reached APPROVE through the real differ", res.Findings)
	}
	if res.Decision != aggregate.DecisionReview {
		t.Fatalf("decision = %q, want REVIEW (predicate error over text operands)", res.Decision)
	}
	assertHasFinding(t, res.Findings, aggregate.EffectRequireReview, "predicate.error")
}

// TestCapitalizedBoolDecodesAsBool is the F1 proof: a !!bool field rendered
// `False`/`True` by the differ decodes to a Go bool, so a rule sees a real bool —
// NOT the pre-fix json.Number("True")->toCEL.String()->Go string that would make a
// predicate lexically string-compare a boolean field.
//
// Note an ordering rule (`new >= old`) CANNOT discriminate here: for booleans the
// lexical spelling order ("False" < "True") happens to coincide with the logical
// order (false < true), so a string binding and a bool binding both answer the same
// — the fail-open hides. A TYPE-SENSITIVE predicate (`new == true`) exposes it: it
// proves only when `new` is a real bool; a string "True" makes `new == true` a
// cross-type comparison that never proves (fail-safe, not APPROVE).
func TestCapitalizedBoolDecodesAsBool(t *testing.T) {
	cs, err := change.Diff("t.yaml", []byte("enabled: False\n"), []byte("enabled: True\n"))
	if err != nil {
		t.Fatalf("change.Diff: %v", err)
	}
	if len(cs.Changes) != 1 {
		t.Fatalf("changes = %d (%+v), want 1 (/enabled modify)", len(cs.Changes), cs.Changes)
	}
	if c := cs.Changes[0]; c.Old != "False" || c.New != "True" {
		t.Fatalf("change = %+v, want old=\"False\" new=\"True\" (raw !!bool literals)", c)
	}

	in := evaldecode.BuildEvaluationInput(cs, aggregate.MR{}, []string{"non-destructive"})
	got := in.ChangeSet.Changes[0]
	if got.Old != false || got.New != true {
		t.Fatalf("decoded old/new = %#v/%#v, want Go bools false/true (not json.Number/string)", got.Old, got.New)
	}

	// A type-sensitive predicate over the /enabled bool: `new == true` proves ONLY
	// when new is bound as a real bool.
	mp, bind := shrinkPolicy()
	mp.Spec.Rules[0].Match.ValueChanges.Pointers = []string{"/enabled"}
	mp.Spec.Rules[0].Prove.When = policy.AssertTree{Leaf: &policy.Leaf{CEL: "new == true"}}
	res, err := aggregate.CoverWithApproval(mp, bind, &in, nil)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	if res.Decision != aggregate.DecisionApprove {
		t.Fatalf("decision = %q, want APPROVE — new must bind as a BOOL so `new == true` proves; a pre-fix string binding would never prove", res.Decision)
	}

	// Mutation control: the pre-fix RAW-STRING binding does NOT prove the same
	// bool predicate — proving the decoder's bool binding is what makes it correct.
	inStr := in
	inStr.ChangeSet = aggregate.ChangeSet{Changes: []aggregate.EvalChange{{
		Subject: got.Subject, File: got.File, Path: got.Path, Kind: got.Kind,
		Old: "False", New: "True", // raw canonical strings (the un-decoded shape)
	}}}
	resStr, err := aggregate.CoverWithApproval(mp, bind, &inStr, nil)
	if err != nil {
		t.Fatalf("CoverWithApproval (string mutation): %v", err)
	}
	if resStr.Decision == aggregate.DecisionApprove {
		t.Fatalf("a string-bound bool must NOT prove `new == true` (cross-type) — got APPROVE; the decoder's bool binding is what closes this")
	}
}

// TestSubjectOf proves the governed-subject derivation: a collection EntryRef is
// preferred; a document-mode change (no EntryRef) falls back to file:<path>. A
// non-empty subject is required (the DecisionRecord finding schema pins it).
func TestSubjectOf(t *testing.T) {
	if got := evaldecode.SubjectOf(change.Change{EntryRef: "topic-registry:orders.events.v1", File: "topics/x.yaml"}); got != "topic-registry:orders.events.v1" {
		t.Errorf("SubjectOf(entryRef) = %q, want the EntryRef", got)
	}
	if got := evaldecode.SubjectOf(change.Change{File: "topics/x.yaml"}); got != "file:topics/x.yaml" {
		t.Errorf("SubjectOf(no entryRef) = %q, want the file:<path> fallback", got)
	}
}

func readShrinkFixture(t *testing.T, side, path string) []byte {
	t.Helper()
	return readFixture(t, "shrink-diff", side, path)
}

func readFixture(t *testing.T, dir, side, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", dir, side, filepath.FromSlash(path))) //nolint:gosec // fixed test fixture path, not user input.
	if err != nil {
		t.Fatalf("read %s/%s fixture %s: %v", dir, side, path, err)
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
