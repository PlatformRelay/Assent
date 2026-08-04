package adoptertest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// result builds an aggregate.Result whose findings are already in the engine's
// canonical order (the tests hand-order them so Match's determinism is exercised
// over the SAME sorted input the engine emits).
func result(dec aggregate.Decision, fs ...aggregate.Finding) aggregate.Result {
	return aggregate.Result{Decision: dec, Findings: fs}
}

func finding(rule, obligation, effect string, points int) aggregate.Finding {
	return aggregate.Finding{
		Rule:       rule,
		Obligation: obligation,
		Effect:     aggregate.Effect(effect),
		Points:     points,
	}
}

// TestFindingsMustContainDefault (REQ-E6-S03-01): every listed finding must fire;
// unlisted findings that ALSO fire are allowed; a missing listed finding fails,
// naming it.
func TestFindingsMustContainDefault(t *testing.T) {
	res := result(aggregate.DecisionReview,
		finding("author-owns-entry", "ownership", "require-review", 0),
		finding("size-bounded", "bounded", "challenge", 3),
	)

	t.Run("all listed fire, extras allowed", func(t *testing.T) {
		exp := Expectation{
			Decision: "REVIEW",
			Findings: []ExpectFinding{{Rule: "author-owns-entry", Effect: "require-review"}},
		}
		reasons, err := Match(exp, res, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reasons) != 0 {
			t.Fatalf("expected must-contain to pass (extra size-bounded allowed), got reasons: %v", reasons)
		}
	})

	t.Run("a missing listed finding fails naming it", func(t *testing.T) {
		exp := Expectation{
			Decision: "REVIEW",
			Findings: []ExpectFinding{{Rule: "no-such-rule", Effect: "block"}},
		}
		reasons, err := Match(exp, res, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reasons) == 0 {
			t.Fatal("expected a missing-finding reason, got none")
		}
		if !anyContains(reasons, "no-such-rule") {
			t.Fatalf("reason must name the missing rule, got: %v", reasons)
		}
	})

	t.Run("obligation is matched when asserted", func(t *testing.T) {
		exp := Expectation{
			Decision: "REVIEW",
			Findings: []ExpectFinding{{Rule: "size-bounded", Obligation: "wrong-obl", Effect: "challenge"}},
		}
		reasons, _ := Match(exp, res, 10)
		if len(reasons) == 0 {
			t.Fatal("a wrong obligation must fail must-contain")
		}
	})

	t.Run("message~ matches the rendered finding message", func(t *testing.T) {
		withMsg := aggregate.Result{Decision: aggregate.DecisionReview, Findings: []aggregate.Finding{
			{Rule: "size-bounded", Obligation: "bounded", Effect: "challenge", Message: "grew by 40 lines"},
		}}
		ok := Expectation{Decision: "REVIEW", Findings: []ExpectFinding{{Rule: "size-bounded", Effect: "challenge", Message: "grew by [0-9]+ lines"}}}
		if reasons, err := Match(ok, withMsg, 10); err != nil || len(reasons) != 0 {
			t.Fatalf("message~ regex should match: err=%v reasons=%v", err, reasons)
		}
		bad := Expectation{Decision: "REVIEW", Findings: []ExpectFinding{{Rule: "size-bounded", Effect: "challenge", Message: "shrank"}}}
		if reasons, _ := Match(bad, withMsg, 10); len(reasons) == 0 {
			t.Fatal("a non-matching message~ must fail")
		}
	})
}

// TestExactClosedListVsMustContainDefault (REQ-E6-S03-02): exact:true closes the
// list (an unlisted firing finding fails); an omitted exact stays must-contain.
func TestExactClosedListVsMustContainDefault(t *testing.T) {
	res := result(aggregate.DecisionReview,
		finding("author-owns-entry", "ownership", "require-review", 0),
		finding("size-bounded", "bounded", "challenge", 3),
	)
	listOne := []ExpectFinding{{Rule: "author-owns-entry", Effect: "require-review"}}

	t.Run("omitted exact is must-contain (unlisted finding allowed)", func(t *testing.T) {
		exp := Expectation{Decision: "REVIEW", Findings: listOne}
		if exp.Exact {
			t.Fatal("zero-value Expectation must default to must-contain (Exact=false)")
		}
		reasons, err := Match(exp, res, 10)
		if err != nil || len(reasons) != 0 {
			t.Fatalf("must-contain should allow the unlisted size-bounded: err=%v reasons=%v", err, reasons)
		}
	})

	t.Run("exact:true fails on an unlisted firing finding", func(t *testing.T) {
		exp := Expectation{Decision: "REVIEW", Exact: true, Findings: listOne}
		reasons, err := Match(exp, res, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !anyContains(reasons, "size-bounded") {
			t.Fatalf("exact:true must fail naming the unlisted size-bounded, got: %v", reasons)
		}
	})

	t.Run("exact:true passes when the list is complete", func(t *testing.T) {
		exp := Expectation{Decision: "REVIEW", Exact: true, Findings: []ExpectFinding{
			{Rule: "author-owns-entry", Effect: "require-review"},
			{Rule: "size-bounded", Effect: "challenge"},
		}}
		if reasons, err := Match(exp, res, 10); err != nil || len(reasons) != 0 {
			t.Fatalf("a complete exact list should pass: err=%v reasons=%v", err, reasons)
		}
	})
}

// TestAbsentAndScoreAssertions (REQ-E6-S03-03): absent fails when a named rule
// fires; score pins total (Σ finding.Points) and threshold, failing on mismatch
// reporting expected vs actual.
func TestAbsentAndScoreAssertions(t *testing.T) {
	res := result(aggregate.DecisionReview,
		finding("size-bounded", "bounded", "challenge", 3),
		finding("risky-field", "safety", "comment", 4),
	)

	t.Run("absent fails when a named rule fired", func(t *testing.T) {
		exp := Expectation{Decision: "REVIEW", Absent: []string{"size-bounded"}}
		reasons, _ := Match(exp, res, 10)
		if !anyContains(reasons, "size-bounded") {
			t.Fatalf("absent must fail naming the fired rule, got: %v", reasons)
		}
	})

	t.Run("absent passes when the named rule did not fire", func(t *testing.T) {
		exp := Expectation{Decision: "REVIEW", Absent: []string{"never-fires"}}
		if reasons, _ := Match(exp, res, 10); len(reasons) != 0 {
			t.Fatalf("absent of a non-firing rule should pass, got: %v", reasons)
		}
	})

	t.Run("score matches total and threshold", func(t *testing.T) {
		// Σ points = 3 + 4 = 7; threshold passed as 10.
		exp := Expectation{Decision: "REVIEW", Score: &ExpectScore{Total: 7, Threshold: 10}}
		if reasons, err := Match(exp, res, 10); err != nil || len(reasons) != 0 {
			t.Fatalf("score should match: err=%v reasons=%v", err, reasons)
		}
	})

	t.Run("score total mismatch fails reporting expected vs actual", func(t *testing.T) {
		exp := Expectation{Decision: "REVIEW", Score: &ExpectScore{Total: 99, Threshold: 10}}
		reasons, _ := Match(exp, res, 10)
		if !anyContains(reasons, "99") || !anyContains(reasons, "7") {
			t.Fatalf("score mismatch must report expected(99) and actual(7), got: %v", reasons)
		}
	})

	t.Run("score threshold mismatch fails", func(t *testing.T) {
		exp := Expectation{Decision: "REVIEW", Score: &ExpectScore{Total: 7, Threshold: 3}}
		reasons, _ := Match(exp, res, 10)
		if !anyContains(reasons, "threshold") {
			t.Fatalf("score threshold mismatch must be reported, got: %v", reasons)
		}
	})
}

// TestUnevaluableAssertionFailsClosed (REQ-E6-S03-04): a findings[].path assertion
// the matcher cannot evaluate (D-054: aggregate.Finding has no Path) ERRORS the
// case (fail-closed), never a silent pass.
func TestUnevaluableAssertionFailsClosed(t *testing.T) {
	res := result(aggregate.DecisionReview,
		finding("author-owns-entry", "ownership", "require-review", 0),
	)
	exp := Expectation{
		Decision: "REVIEW",
		Findings: []ExpectFinding{{Rule: "author-owns-entry", Effect: "require-review", Path: "/owner"}},
	}
	reasons, err := Match(exp, res, 10)
	if err == nil {
		t.Fatalf("a path assertion must ERROR (fail-closed), got reasons=%v err=nil", reasons)
	}
	if !strings.Contains(err.Error(), "path") {
		t.Fatalf("the error must name the unsupported path assertion, got: %v", err)
	}
	if len(reasons) != 0 {
		t.Fatalf("fail-closed must not also return mismatch reasons, got: %v", reasons)
	}

	t.Run("an invalid message~ pattern errors (fail-closed)", func(t *testing.T) {
		withMsg := aggregate.Result{Decision: aggregate.DecisionReview, Findings: []aggregate.Finding{
			{Rule: "size-bounded", Effect: "challenge", Message: "grew"},
		}}
		bad := Expectation{Decision: "REVIEW", Findings: []ExpectFinding{
			{Rule: "size-bounded", Effect: "challenge", Message: "("}, // unbalanced group
		}}
		if _, err := Match(bad, withMsg, 10); err == nil {
			t.Fatal("an uncompilable message~ pattern must error (fail-closed), not silently pass")
		}
	})
}

// TestMatcherDoubleRunStable (REQ-E6-S03-05): Match is pure — canonical assertion
// ordering, byte-identical over a double run.
func TestMatcherDoubleRunStable(t *testing.T) {
	res := result(aggregate.DecisionReview,
		finding("author-owns-entry", "ownership", "require-review", 0),
		finding("size-bounded", "bounded", "challenge", 3),
		finding("risky-field", "safety", "comment", 4),
	)
	exp := Expectation{
		Decision: "REVIEW",
		Exact:    true,
		Findings: []ExpectFinding{{Rule: "author-owns-entry", Effect: "require-review"}},
		Absent:   []string{"size-bounded", "risky-field"},
		Score:    &ExpectScore{Total: 999, Threshold: 1},
	}
	first, err1 := Match(exp, res, 10)
	second, err2 := Match(exp, res, 10)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v / %v", err1, err2)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("double run diverged:\n first: %v\nsecond: %v", first, second)
	}
	if len(first) == 0 {
		t.Fatal("expected several mismatch reasons to order-check, got none")
	}
}

func anyContains(reasons []string, sub string) bool {
	for _, r := range reasons {
		if strings.Contains(r, sub) {
			return true
		}
	}
	return false
}
