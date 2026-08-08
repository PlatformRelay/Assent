package aggregate

// relational_string_test.go is the regression suite for the D-131 fail-open:
// CEL's relational operators (< <= > >=) are DEFINED over strings and compare
// them LEXICALLY, so a change value that binds as a Go string (a YAML !!str the
// decoder deliberately keeps a string, or a numeric literal too large to
// represent) turned an ordering rule into a silently wrong boolean instead of an
// error. Both polarities are wrong: lexically "6" >= "12" is TRUE (a shrink
// APPROVEs) and "12" >= "6" is FALSE (a grow BLOCKs).
//
// The tests below pin BOTH directions of the fix: the string-operand compares
// must now ERROR (-> predicate.error -> REVIEW, the fail-safe path), and every
// legitimate compare — numeric, explicitly coerced, equality, string membership,
// short-circuited-away — must still evaluate exactly as before.

import (
	"encoding/json"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// shrinkRulePolicy is the D-016 `partitions-must-not-shrink` shape: a
// valueChanges /partitions modify rule proving `non-destructive` with the BARE
// `new >= old` (no int() coercion), blocking on failure. The binding requires
// only that obligation, so the decision is driven solely by the compare.
func shrinkRulePolicy() (*policy.MergePolicy, *policy.Binding) {
	mp := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "partitions-must-not-shrink",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
				Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "partition-count-shrunk"},
				Points:    10,
			}},
		},
	}
	bind := &policy.Binding{Class: "topic-registry", Environment: "prod", Risk: policy.Risk{Threshold: 1}, Require: []string{"non-destructive"}}
	return mp, bind
}

func stringScalarInput(oldVal, newVal string) *EvaluationInput {
	return &EvaluationInput{
		ChangeSet: ChangeSet{Changes: []EvalChange{{
			Subject: "topic-registry:orders.events.v1",
			File:    "topics/prod/orders-events.yaml",
			Path:    "/partitions",
			Kind:    "modify",
			Old:     oldVal,
			New:     newVal,
		}}},
		Facts:   map[string]map[string]Fact{},
		Require: []string{"non-destructive"},
	}
}

func onlyPredicateError(t *testing.T, res Result) {
	t.Helper()
	if res.Decision != DecisionReview {
		t.Fatalf("decision = %q, want REVIEW (a relational compare over text must fail SAFE)", res.Decision)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %+v, want exactly one predicate.error", res.Findings)
	}
	if f := res.Findings[0]; f.Code != "predicate.error" || f.Effect != EffectRequireReview {
		t.Fatalf("finding = %+v, want code=predicate.error effect=require-review", f)
	}
}

// TestQuotedNumericShrinkMustNotApproveThroughCover is THE reproduction, driven
// through the production entry point aggregate.Cover (not evalLeaf): a
// `partitions: "12"` -> `"6"` change — quoted in the adopter's YAML, so the
// decoder KEEPS it a Go string by design — used to make `new >= old` the lexical
// "6" >= "12" = TRUE, proving `non-destructive`, firing nothing, and returning
// APPROVE with zero findings. A BLOCK -> APPROVE flip. It must now fail safe.
func TestQuotedNumericShrinkMustNotApproveThroughCover(t *testing.T) {
	mp, bind := shrinkRulePolicy()
	res, err := Cover(mp, bind, stringScalarInput("12", "6"))
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if res.Decision == DecisionApprove {
		t.Fatalf("decision = APPROVE with findings %+v — the lexical fail-open is OPEN: \"6\" >= \"12\" is lexically true", res.Findings)
	}
	onlyPredicateError(t, res)
}

// TestQuotedNumericGrowMustNotBlockLexically is the OTHER polarity of the same
// defect and proves the guard is not merely "erroring in the APPROVE direction":
// `partitions: "6"` -> `"12"` is a legitimate GROW, but lexically "12" >= "6" is
// FALSE, so the rule fired and BLOCKed a change that satisfies the policy. A
// wrong answer either way — the compare must error, never decide.
func TestQuotedNumericGrowMustNotBlockLexically(t *testing.T) {
	mp, bind := shrinkRulePolicy()
	res, err := Cover(mp, bind, stringScalarInput("6", "12"))
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if res.Decision == DecisionBlock {
		t.Fatalf("decision = BLOCK with findings %+v — a lexical compare judged a legitimate grow 6->12 destructive", res.Findings)
	}
	onlyPredicateError(t, res)
}

// TestRelationalOverStringOperandsErrors pins the seam itself: EVERY relational
// operator over two string-bound operands errors, in both argument orders, and
// never returns a boolean.
func TestRelationalOverStringOperandsErrors(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("newEvalEnv: %v", err)
	}
	in := *stringScalarInput("12", "6")
	ch := in.ChangeSet.Changes[0]
	for _, expr := range []string{
		"new >= old", "new > old", "new <= old", "new < old",
		"old >= new", "old > new", "old <= new", "old < new",
		`new >= "12"`, `"12" <= new`, // a string LITERAL operand is no exemption
		"entry >= oldEntry",          // the scalar-fallback entry binding
		"string(new) >= string(old)", // an explicit coercion TO string is still text ordering
		"mr.author < path",           // two unrelated string-typed scope fields
	} {
		got, err := evalLeaf(env, in, ch, "prod", expr)
		if err == nil {
			t.Errorf("evalLeaf(%q) = (%v, nil) — a relational compare over text must ERROR, never answer", expr, got)
		}
	}
}

// TestLegitimateComparesStillEvaluate is the required opposite polarity: a fix
// that errors on everything is not a fix. Numeric ordering, explicit coercion
// (the repo's `int(new)` idiom — the supported way to order quoted numerics),
// equality, string predicates and membership must all still evaluate cleanly.
func TestLegitimateComparesStillEvaluate(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("newEvalEnv: %v", err)
	}
	numeric := EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{{
		Subject: "s:1", File: "f.yaml", Path: "/partitions", Kind: "modify",
		Old: json.Number("12"), New: json.Number("6"),
	}}}, Facts: map[string]map[string]Fact{}, MR: MR{Author: "alice"}}
	quoted := *stringScalarInput("12", "6")

	cases := []struct {
		in   EvaluationInput
		expr string
		want bool
	}{
		{numeric, "new >= old", false},
		{numeric, "new < old", true},
		{numeric, "new >= 6", true},
		{numeric, "new <= 5.5", false},
		// The supported way to order a QUOTED numeric: coerce, then compare.
		{quoted, "int(new) >= int(old)", false},
		{quoted, "int(new) < int(old)", true},
		{quoted, "double(new) < double(old)", true},
		// Equality, string predicates and membership are untouched by the guard.
		{quoted, `new == "6"`, true},
		{quoted, `new != old`, true},
		{quoted, `new.startsWith("6")`, true},
		{quoted, `path == "/partitions"`, true},
		{quoted, `kind in ["modify", "add"]`, true},
		{quoted, `size(new) < 3`, true}, // size() is an int — an int compare, not text
	}
	for _, tc := range cases {
		ch := tc.in.ChangeSet.Changes[0]
		got, err := evalLeaf(env, tc.in, ch, "prod", tc.expr)
		if err != nil {
			t.Errorf("evalLeaf(%q) errored: %v — a legitimate compare must still evaluate", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("evalLeaf(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

// TestShortCircuitedStringRelationalStillEvaluates proves the guard inspects what
// was ACTUALLY evaluated, not what the expression mentions: a string relational
// behind a false conjunct never runs, so the leaf stays a clean false. Erroring
// here would flip working policies to REVIEW for a compare that never happened.
func TestShortCircuitedStringRelationalStillEvaluates(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("newEvalEnv: %v", err)
	}
	in := *stringScalarInput("12", "6")
	ch := in.ChangeSet.Changes[0]
	got, err := evalLeaf(env, in, ch, "prod", `kind == "add" && new >= old`)
	if err != nil {
		t.Fatalf("evalLeaf on a short-circuited string relational errored: %v", err)
	}
	if got {
		t.Fatalf("got true, want false (kind is modify)")
	}
}

// TestComprehensionTextCompareCaughtEveryIteration pins the guard's per-iteration
// reach. A comprehension body is evaluated once per element, but the interpreter
// keeps only ONE recorded value per AST node, so reading state after the fact
// would see the LAST iteration only: `changes.all(c, c.new > c.old)` over a text
// change followed by a numeric one would come back a clean `true` with the text
// compare invisible. The guard watches the operands as they evaluate, so every
// iteration is seen — and a compare that short-circuits away is still not judged.
func TestComprehensionTextCompareCaughtEveryIteration(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("newEvalEnv: %v", err)
	}
	textChange := EvalChange{Subject: "s:1", File: "f.yaml", Path: "/a", Kind: "modify", Old: "12", New: "6"}
	numChange := EvalChange{Subject: "s:2", File: "f.yaml", Path: "/b", Kind: "modify", Old: json.Number("1"), New: json.Number("9")}

	// all() visits every element: the text compare is the FIRST iteration and the
	// numeric one overwrites the recorded state — it must still be caught.
	textFirst := EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{textChange, numChange}}, Facts: map[string]map[string]Fact{}}
	if got, err := evalLeaf(env, textFirst, textChange, "prod", "changes.all(c, c.new > c.old)"); err == nil {
		t.Errorf("all() over a text-then-number changeset = (%v, nil) — the first iteration ordered text lexically", got)
	}
	numFirst := EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{numChange, textChange}}, Facts: map[string]map[string]Fact{}}
	if got, err := evalLeaf(env, numFirst, numChange, "prod", "changes.all(c, c.new > c.old)"); err == nil {
		t.Errorf("all() over a number-then-text changeset = (%v, nil) — the second iteration ordered text lexically", got)
	}

	// exists() short-circuits on the first true, so the text element is never
	// compared at all — nothing was ordered, and the clean answer stands.
	got, err := evalLeaf(env, numFirst, numChange, "prod", "changes.exists(c, c.new > c.old)")
	if err != nil {
		t.Fatalf("exists() short-circuiting before the text element must not error: %v", err)
	}
	if !got {
		t.Fatalf("exists() = false, want true (9 > 1 on the first element)")
	}
}

// TestUnrepresentableNumericFailsSafe covers the ADR-0013 residual #1 arm: a
// numeric literal that fits neither int64 nor float64 used to fall back to its
// STRING form, so `9e399 > 1e400` was the lexical "9e399" > "1e400" = true — a
// numerically FALSE compare answered true, with no error. It must fail safe.
func TestUnrepresentableNumericFailsSafe(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("newEvalEnv: %v", err)
	}
	in := EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{{
		Subject: "s:1", File: "f.yaml", Path: "/x", Kind: "modify",
		Old: json.Number("1e400"), New: json.Number("9e399"),
	}}}, Facts: map[string]map[string]Fact{}}
	ch := in.ChangeSet.Changes[0]
	for _, expr := range []string{"new > old", "new >= old", `new == "9e399"`} {
		if got, err := evalLeaf(env, in, ch, "prod", expr); err == nil {
			t.Errorf("evalLeaf(%q) = (%v, nil) — an unrepresentable numeric literal must never bind to a comparable value", expr, got)
		}
	}
	// A large-but-representable literal still binds (lossily, as float64) — the
	// documented residual, unchanged by this fix.
	big := EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{{
		Subject: "s:1", File: "f.yaml", Path: "/x", Kind: "modify",
		Old: json.Number("18446744073709551617"), New: json.Number("18446744073709551618"),
	}}}, Facts: map[string]map[string]Fact{}}
	if _, err := evalLeaf(env, big, big.ChangeSet.Changes[0], "prod", "new >= old"); err != nil {
		t.Errorf("an over-int64 but float64-representable literal must still compare: %v", err)
	}
}

// TestLegacyAggregatePathAlsoRefusesTextOrdering covers the OTHER evaluation
// seam. The walking-skeleton Aggregate/evalRule path declares old/new as CEL
// StringType and binds the differ's RAW canonical strings, so a bare `new >= old`
// there is ALWAYS lexical — it relied on authors writing int()/double(), with
// nothing enforcing it. One unguarded evaluator is how this fail-open returns, so
// the guard is applied to both. Coerced compares on that path still evaluate.
func TestLegacyAggregatePathAlsoRefusesTextOrdering(t *testing.T) {
	env, err := newCELEnv()
	if err != nil {
		t.Fatalf("newCELEnv: %v", err)
	}
	act := map[string]any{"old": "12", "new": "6", "changes": []map[string]string{}}

	for _, expr := range []string{"new >= old", "new < old", "old > new"} {
		if got, err := evalRule(env, act, expr); err == nil {
			t.Errorf("evalRule(%q) = (%v, nil) — the walking-skeleton path ordered raw canonical text", expr, got)
		}
	}
	// Both polarities: the coerced forms this path always mandated still work.
	for expr, want := range map[string]bool{
		"int(new) >= int(old)": false,
		"int(new) < int(old)":  true,
		`new == "6"`:           true,
		"old == new":           false,
	} {
		got, err := evalRule(env, act, expr)
		if err != nil {
			t.Errorf("evalRule(%q) errored: %v", expr, err)
			continue
		}
		if got != want {
			t.Errorf("evalRule(%q) = %v, want %v", expr, got, want)
		}
	}
}

// TestToCELNeverYieldsAStringForANumericLiteral pins the unit-level invariant the
// evaldecode package doc asserts: toCEL never converts a numeric literal into its
// string form (the silent lexical demotion).
func TestToCELNeverYieldsAStringForANumericLiteral(t *testing.T) {
	for _, lit := range []string{"12", "-4", "3.5", "18446744073709551617", "9e399", "1e400", "-1e400"} {
		if got := toCEL(json.Number(lit)); got == lit {
			t.Errorf("toCEL(json.Number(%q)) returned the STRING form — a numeric literal must never demote to text", lit)
		}
	}
}
