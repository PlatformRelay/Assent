package aggregate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// overRange numbers fit NEITHER int64 NOR float64, so toCEL's third branch — the
// one that REFUSES to bind them at all (D-131 / ADR-0013 Amendment 1) — is what
// handles them.
//
//   - "1e400"  — grammatically valid JSON, ParseInt rejects the exponent form and
//     ParseFloat reports value-out-of-range (+Inf), so both typed branches fail.
//   - a 400-digit integer — ParseInt out of range; ParseFloat out of range too.
//
// A 60-digit integer is deliberately NOT here: it overflows int64 but Float64
// succeeds (1.23e+59), so it takes the float64 branch and still compares (lossily)
// — the ADR-0013 residual #1, which D-131 deliberately left alone.
const (
	overRangeExp    = "1e400"
	overRangeDigits = "1" + // a 400-digit integer: beyond float64's ~1.8e308 too.
		"0000000000000000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000"
)

// TestToCELOverflowFailsSafe — REQ-AUD-S13-01 (TEST-02).
//
// toCEL binds an integral json.Number as int64 and a decimal as float64. A number
// that fits NEITHER is REFUSED: it binds a CEL ERROR value. It used to fall back
// to the literal's STRING form, which was a silent demotion to text — `9e399 >
// 1e400` became the lexical "9e399" > "1e400" = true, the numerically wrong answer
// with no error at all. D-131 closed that; this test pins the replacement contract
// with the rigour the fallback version had, and pins the arms that D-131's own
// suite (relational_string_test.go) does not:
//
//   - the 400-digit NON-exponent literal. That suite only exercises 1e400/9e399/
//     -1e400, and "does ParseFloat reject it" is a different question for a plain
//     digit string than for an exponent form.
//   - WHY these literals reach the refusing branch — Int64 AND Float64 must BOTH
//     fail. Nothing over there asserts that, so a future change binding 1e400 as
//     +Inf would leave that suite green while this test quietly stopped testing
//     the branch it names.
//   - operators that are NOT relational: `type(new)`, `int(new)`, `string(new)`,
//     `new - 1`. Every assertion over there reaches the refusal through a
//     relational or `==` operator, where D-131's textOrderGuard is a second line
//     of defence; here nothing but toCEL's own return value can produce the error.
//     `int(...)` matters most: it is the BLESSED escape hatch for ordering quoted
//     numerics (TestLegitimateComparesStillEvaluate depends on `int(new) >=
//     int(old)` still working), so this pins that the hatch does not launder an
//     unrepresentable literal back into something comparable.
//
// MUTATION that reds this test and leaves relational_string_test.go's whole
// suite green (verified): refuse only the exponent forms — `types.NewErr(...)`
// when the literal contains e/E, `return x.String()` otherwise. Every
// unrepresentable literal that suite uses is an exponent form, so the 400-digit
// arm below is the only thing that catches a half-closed fix.
// MUTATIONS that red this test too (not differential, but it must still catch
// them): restore `return x.String()` outright, or return a coercible sentinel
// (0, +Inf, math.MaxInt64) — `int(new) > 0` would then answer cleanly again.
func TestToCELOverflowFailsSafe(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	in := EvaluationInput{}

	// First: both over-range literals really do reach the refusing branch. If a
	// future toCEL change made either one typed, every case below would stop
	// testing that branch and would start passing for the wrong reason.
	for _, lit := range []string{overRangeExp, overRangeDigits} {
		n := json.Number(lit)
		if _, err := n.Int64(); err == nil {
			t.Fatalf("%s: Int64 must fail for an over-range literal (else the int64 branch binds it)", lit)
		}
		if _, err := n.Float64(); err == nil {
			t.Fatalf("%s: Float64 must fail for an over-range literal (else the float64 branch binds it)", lit)
		}
		bound := toCEL(n)
		if s, isString := bound.(string); isString {
			t.Fatalf("%s: toCEL returned the STRING form %q — the D-131 lexical demotion is back", lit, s)
		}
		rv, ok := bound.(ref.Val)
		if !ok {
			t.Fatalf("%s: toCEL bound %T (%v), want a CEL error value", lit, bound, bound)
		}
		if !types.IsError(rv) {
			t.Fatalf("%s: toCEL bound a %s value (%v), want a CEL error", lit, rv.Type().TypeName(), rv.Value())
		}
		// The equivalent of the old "the fallback kept the literal" assertion: a
		// refusal an adopter cannot act on is not much better than a wrong answer,
		// so the error must NAME the number it refused. (The exact sentence is
		// goldened once, at the Cover/message seam, in relational_string_test.go's
		// TestUnrepresentableNumericInterpolatesItsRefusal — not duplicated here.)
		cause, isErr := rv.Value().(error)
		if !isErr {
			t.Fatalf("%s: the CEL error value carries %T, not an error", lit, rv.Value())
		}
		if !strings.Contains(cause.Error(), lit) {
			t.Fatalf("%s: the refusal does not name the number it refused: %q", lit, cause.Error())
		}
	}

	// The refusal is observable in the ACTIVATION, not merely in toCEL's return
	// value. This is the assertion that isolates toCEL's contract from D-131's
	// textOrderGuard: `type(new) == string` contains no relational operator, so
	// the guard provably is not what makes it fail — the bound value itself is.
	// Under the string fallback this predicate was a clean `true`; it must now be
	// unanswerable, because a value the engine refused to bind must not be
	// inspectable as if it had been bound.
	overCh := EvalChange{File: "topics/x.yaml", Path: "/partitions", Kind: "modify",
		Old: json.Number("100"), New: json.Number(overRangeExp)}
	isStr, err := evalLeaf(env, in, overCh, "prod", "type(new) == string")
	if err == nil {
		t.Fatalf("`type(new) == string` returned %v with NO error — an unrepresentable literal is answering predicates again", isStr)
	}
	if isStr {
		t.Fatal("`type(new) == string` errored but still reported satisfied")
	}

	// Every predicate shape a real policy uses over that binding must ERROR — and
	// the error must be attributable to the refused literal, not to some unrelated
	// overload failure that would happen to look the same.
	cases := []struct {
		name string
		ch   EvalChange
		expr string
		lit  string // the over-range literal whose refusal must be the cause
	}{
		{
			name: "relational_against_in_range_old",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("100"), New: json.Number(overRangeExp)},
			expr: "new >= old",
			lit:  overRangeExp,
		},
		{
			name: "relational_against_int_literal",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("100"), New: json.Number(overRangeExp)},
			expr: "new > 100",
			lit:  overRangeExp,
		},
		{
			name: "over_range_on_the_old_side",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number(overRangeExp), New: json.Number("6")},
			expr: "new >= old",
			lit:  overRangeExp,
		},
		{
			name: "digit_overflow_relational",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("12"), New: json.Number(overRangeDigits)},
			expr: "new >= old",
			lit:  overRangeDigits,
		},
		{
			// int() is the documented escape hatch for ordering a quoted numeric.
			// It must NOT double as a way to make an unrepresentable literal
			// comparable again.
			name: "explicit_int_conversion",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("1"), New: json.Number(overRangeExp)},
			expr: "int(new) > 0",
			lit:  overRangeExp,
		},
		{
			// Asking explicitly for the very value the old fallback handed over
			// must not resurrect it.
			name: "explicit_string_conversion",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("1"), New: json.Number(overRangeExp)},
			expr: `string(new) == "` + overRangeExp + `"`,
			lit:  overRangeExp,
		},
		{
			// Arithmetic, not comparison: the refusal propagates through operators
			// the D-131 guard never watches.
			name: "arithmetic_against_in_range",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("1"), New: json.Number(overRangeExp)},
			expr: "new - 1 > 0",
			lit:  overRangeExp,
		},
	}
	// Positive control: a table that silently iterated nothing would pass vacuously.
	if len(cases) != 7 {
		t.Fatalf("table lost cases: have %d, want 7", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalLeaf(env, in, tc.ch, "prod", tc.expr)
			if err == nil {
				t.Fatalf("%q over an over-range value returned %v with NO error — "+
					"a value assent refused to bind silently produced a boolean", tc.expr, got)
			}
			// evalLeaf's contract: an erroring predicate is never reported satisfied.
			if got {
				t.Fatalf("%q errored (%v) but still reported satisfied", tc.expr, err)
			}
			if !strings.Contains(err.Error(), tc.lit) {
				t.Fatalf("%q errored with %q, which does not name the refused literal — "+
					"the failure is not attributable to toCEL's refusal", tc.expr, err)
			}
		})
	}
}

// TestToCELOverflowNeverApprovesThroughCover — REQ-AUD-S13-01, production entry
// point. The unit assertions above prove evalLeaf errors; this proves the error
// actually reaches the decision as a fail-safe REVIEW through Cover, the real
// coverage loop. A `partitions-must-not-shrink`-shaped rule whose subject carries
// an over-range new value must surface predicate.error/require-review — it must
// not prove the obligation and it must never APPROVE.
//
// relational_string_test.go also drives an unrepresentable literal through Cover
// (TestUnrepresentableNumericInterpolatesItsRefusal, which goldens the rendered
// message). What that test cannot show, because every input it hands Cover
// errors, is that this policy DECIDES anything at all: the control below is the
// difference between "the refusal produced REVIEW" and "this fixture produces
// REVIEW no matter what you feed it".
func TestToCELOverflowNeverApprovesThroughCover(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "partitions-must-not-shrink",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
				Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "partition-count-shrunk"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}

	// Control: an ordinary in-range shrink proves the rule FAILS normally (BLOCK
	// via the rule's own onFailure code) — so the over-range case below is
	// distinguishable from "this policy blocks everything".
	inRange := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "topic:orders", File: "topics/orders.yaml", Path: "/partitions", Kind: "modify",
			Old: json.Number("12"), New: json.Number("6")},
	}}}
	ctrl, err := Cover(pol, bind, inRange)
	if err != nil {
		t.Fatalf("Cover (control): %v", err)
	}
	if len(ctrl.Findings) != 1 || ctrl.Findings[0].Code != "partition-count-shrunk" {
		t.Fatalf("control must produce the rule's own onFailure finding, got %+v", ctrl.Findings)
	}

	// The over-range case: the value was refused at binding time, so the compare
	// cannot be made and the obligation is UNPROVEN via predicate.error — not
	// proven, and not the rule's own effect.
	overRange := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "topic:orders", File: "topics/orders.yaml", Path: "/partitions", Kind: "modify",
			Old: json.Number("12"), New: json.Number(overRangeExp)},
	}}}
	got, err := Cover(pol, bind, overRange)
	if err != nil {
		t.Fatalf("Cover (over-range): %v", err)
	}
	if got.Decision == DecisionApprove {
		t.Fatal("an over-range value assent refused to bind must never APPROVE")
	}
	if len(got.Findings) != 1 {
		t.Fatalf("want exactly one finding, got %+v", got.Findings)
	}
	f := got.Findings[0]
	if f.Code != "predicate.error" || f.Effect != EffectRequireReview {
		t.Fatalf("want predicate.error/require-review (fail-safe), got code=%q effect=%q", f.Code, f.Effect)
	}
	if !strings.Contains(string(got.Decision), "REVIEW") {
		t.Fatalf("decision = %q, want the fail-safe REVIEW", got.Decision)
	}
}
