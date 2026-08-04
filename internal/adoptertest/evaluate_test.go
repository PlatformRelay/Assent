package adoptertest_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
)

// TestEvaluateOpaqueDiffFailsSafeReview proves an undecidable (opaque) base/↔head/
// diff maps to the fail-safe REVIEW decision, never a silent APPROVE — the differ's
// in-band opaque signal must dominate. A non-object JSON root is opaque per ADR-0011.
func TestEvaluateOpaqueDiffFailsSafeReview(t *testing.T) {
	c := loadCase(t, "within-cap") // a valid pack + binding
	c.Base = []byte("[1, 2]\n")    // non-object root -> the differ returns opaque
	c.Head = []byte("[1, 3]\n")

	res, err := adoptertest.Evaluate(c)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if string(res.Decision) != "REVIEW" {
		t.Fatalf("opaque diff decision = %q, want REVIEW (fail-safe)", res.Decision)
	}
}

// TestEvaluateNoOpChangesetFailsSafeReview proves a no-op case (head == base -> a
// 0-change, NON-opaque changeset) maps to the fail-safe REVIEW, never a vacuous
// APPROVE. Cover treats "no matched change" as an obligation that does not apply and
// reduces to APPROVE; a harness that reported that as a green test would give false
// confidence a policy was exercised when nothing changed. Mirrors run.go's undecidable
// guard (cs.Opaque || len(cs.Changes) == 0).
func TestEvaluateNoOpChangesetFailsSafeReview(t *testing.T) {
	c := loadCase(t, "within-cap") // its head normally APPROVEs
	c.Head = c.Base                // no-op: nothing changed -> empty changeset

	res, err := adoptertest.Evaluate(c)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if string(res.Decision) != "REVIEW" {
		t.Fatalf("no-op changeset decision = %q, want REVIEW (a vacuous APPROVE would be a fail-open)", res.Decision)
	}
}

// TestRunCaseSurfacesEvaluationError proves a case that cannot be evaluated (here a
// nil binding, which aggregate.Cover rejects) surfaces an error rather than a
// silent pass — an unevaluable case is never a green case.
func TestRunCaseSurfacesEvaluationError(t *testing.T) {
	c := loadCase(t, "within-cap")
	c.Bind = nil // Cover fails closed on a nil binding

	if _, err := adoptertest.RunCase(c); err == nil {
		t.Fatal("expected RunCase to surface an evaluation error for a nil binding")
	}
}
