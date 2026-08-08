package aggregate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// overRange numbers fit NEITHER int64 NOR float64, so toCEL's third branch
// (evaluate.go:191 — `return x.String()`) is the one that binds them.
//
//   - "1e400"  — grammatically valid JSON, ParseInt rejects the exponent form and
//     ParseFloat reports value-out-of-range (+Inf), so both typed branches fail.
//   - a 400-digit integer — ParseInt out of range; ParseFloat out of range too.
//
// A 60-digit integer is deliberately NOT here: it overflows int64 but Float64
// succeeds (1.23e+59), so it takes the float64 branch, not the string fallback.
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
// toCEL binds an integral json.Number as int64 and a decimal as float64. A
// number that fits NEITHER falls back to its STRING form (evaluate.go:191). This
// test pins what a predicate then does with it: comparing that string binding
// against an in-range number is a cross-type compare that cel-go rejects
// ("no such overload") -> evalLeaf errors -> the engine fails safe. It must NEVER
// silently coerce the over-range value into a number, and the run must never
// APPROVE through a string/number confusion.
//
// The string fallback is asserted DIRECTLY (type(new) == string) so the line's
// hit count is non-zero in the coverage profile for a stated reason, not as a
// side effect of some other assertion.
func TestToCELOverflowFailsSafe(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	in := EvaluationInput{}

	// First: both over-range literals really do reach the string fallback. If a
	// future toCEL change made either one typed, every case below would stop
	// testing the fallback branch and would start passing for the wrong reason.
	for _, lit := range []string{overRangeExp, overRangeDigits} {
		n := json.Number(lit)
		if _, err := n.Int64(); err == nil {
			t.Fatalf("%s: Int64 must fail for an over-range literal (else the int64 branch binds it)", lit)
		}
		if _, err := n.Float64(); err == nil {
			t.Fatalf("%s: Float64 must fail for an over-range literal (else the float64 branch binds it)", lit)
		}
		bound := toCEL(n)
		s, ok := bound.(string)
		if !ok {
			t.Fatalf("%s: toCEL bound %T, want the string fallback", lit, bound)
		}
		if s != lit {
			t.Fatalf("%s: string fallback lost the literal, got %q", lit, s)
		}
	}

	// The fallback is observable in the activation: the binding IS a CEL string.
	// This is the assertion that pins evaluate.go:191 itself.
	overCh := EvalChange{File: "topics/x.yaml", Path: "/partitions", Kind: "modify",
		Old: json.Number("100"), New: json.Number(overRangeExp)}
	isStr, err := evalLeaf(env, in, overCh, "prod", "type(new) == string")
	if err != nil {
		t.Fatalf("type(new) probe errored: %v", err)
	}
	if !isStr {
		t.Fatal("an over-range json.Number must bind as the string fallback, not as a number")
	}

	// Every predicate shape a real policy uses over that binding must ERROR.
	cases := []struct {
		name string
		ch   EvalChange
		expr string
	}{
		{
			name: "relational_against_in_range_old",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("100"), New: json.Number(overRangeExp)},
			expr: "new >= old",
		},
		{
			name: "relational_against_int_literal",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("100"), New: json.Number(overRangeExp)},
			expr: "new > 100",
		},
		{
			name: "over_range_on_the_old_side",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number(overRangeExp), New: json.Number("6")},
			expr: "new >= old",
		},
		{
			name: "digit_overflow_relational",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("12"), New: json.Number(overRangeDigits)},
			expr: "new >= old",
		},
		{
			name: "explicit_int_conversion",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("1"), New: json.Number(overRangeExp)},
			expr: "int(new) > 0",
		},
		{
			name: "arithmetic_against_in_range",
			ch:   EvalChange{File: "a.yaml", Path: "/n", Kind: "modify", Old: json.Number("1"), New: json.Number(overRangeExp)},
			expr: "new - 1 > 0",
		},
	}
	// Positive control: a table that silently iterated nothing would pass vacuously.
	if len(cases) != 6 {
		t.Fatalf("table lost cases: have %d, want 6", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalLeaf(env, in, tc.ch, "prod", tc.expr)
			if err == nil {
				t.Fatalf("%q over an over-range value returned %v with NO error — "+
					"a string/number confusion silently produced a boolean", tc.expr, got)
			}
			// evalLeaf's contract: an erroring predicate is never reported satisfied.
			if got {
				t.Fatalf("%q errored (%v) but still reported satisfied", tc.expr, err)
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

	// The over-range case: the compare cannot be made, so the obligation is
	// UNPROVEN via predicate.error — not proven, and not the rule's own effect.
	overRange := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "topic:orders", File: "topics/orders.yaml", Path: "/partitions", Kind: "modify",
			Old: json.Number("12"), New: json.Number(overRangeExp)},
	}}}
	got, err := Cover(pol, bind, overRange)
	if err != nil {
		t.Fatalf("Cover (over-range): %v", err)
	}
	if got.Decision == DecisionApprove {
		t.Fatal("an over-range value bound as a string must never APPROVE (string/number confusion)")
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
