package adoptertest

import (
	"fmt"
	"regexp"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// match.go is the PURE expectation matcher (P5-E6-S03). Over the engine's produced
// aggregate.Result it checks the four structural assertions the frozen
// #/$defs/expectation carries — findings (must-contain by default / exact closed
// list), absent, score — plus the discouraged message~ presentation match. It runs
// AFTER the coarse decision assertion (RunCase) so a case pins WHICH rule fired with
// WHICH effect, not just the decision.
//
// FAIL-CLOSED: an assertion the matcher cannot faithfully evaluate ERRORS the case
// (an error return), never a silent pass — a silent pass is a silent-approve of a
// wrong decision. Today the only such assertion is findings[].path (D-054):
// aggregate.Finding carries no Path field, so Match cannot check it; threading Path
// onto the emitted Finding is a decision-path change reserved to its own
// fail-safety-reviewed engine lane, NOT a matcher line-item.
//
// A genuine mismatch (a missing/unexpected finding, a fired absent rule, a wrong
// score) is a `reasons` entry, NOT an error — the two channels are distinct so
// RunCase can fail the case on a mismatch yet propagate a fail-closed error.
//
// Determinism: Match iterates the expectation's DECLARED order (Findings, Absent)
// and the engine's PRE-SORTED Result.Findings; it never ranges a Go map to build
// output, so a double run is byte-identical (ADR-0014 golden L0).

// Match checks exp's findings/absent/score/message assertions against res. threshold
// is the binding's risk threshold (aggregate.Result exposes no threshold, so the
// caller threads it from policy.Binding.Risk.Threshold). It returns the mismatch
// reasons (empty ⇒ matched) OR a fail-closed error for an unevaluable assertion.
func Match(exp Expectation, res aggregate.Result, threshold int) ([]string, error) {
	// Fail-closed FIRST: a path assertion cannot be evaluated (D-054). Erroring
	// before any mismatch accounting keeps the outcome order-independent — a path
	// assertion always errors, never a silent pass and never a partial mismatch set.
	for _, ef := range exp.Findings {
		if ef.Path != "" {
			return nil, fmt.Errorf("finding rule %q: path assertion unsupported (D-054: aggregate.Finding has no path field); remove findings[].path until the engine lane lands", ef.Rule)
		}
	}

	var reasons []string

	// 1. must-contain: every listed finding must fire (extras allowed by default).
	for _, ef := range exp.Findings {
		af := findActual(ef, res.Findings)
		if af == nil {
			reasons = append(reasons, fmt.Sprintf("missing expected finding: rule=%q effect=%q%s", ef.Rule, ef.Effect, obligationSuffix(ef.Obligation)))
			continue
		}
		if ef.Message != "" {
			matched, err := regexp.MatchString(ef.Message, af.Message)
			if err != nil {
				return nil, fmt.Errorf("finding rule %q: invalid message~ pattern %q: %w", ef.Rule, ef.Message, err)
			}
			if !matched {
				reasons = append(reasons, fmt.Sprintf("finding rule %q: message %q does not match /%s/", ef.Rule, af.Message, ef.Message))
			}
		}
	}

	// 2. exact: the list is CLOSED — an actual finding matched by no listed
	// expectation fails. Iterate the engine's pre-sorted Findings for stable order.
	if exp.Exact {
		for i := range res.Findings {
			af := res.Findings[i]
			if !coveredByExpected(af, exp.Findings) {
				reasons = append(reasons, fmt.Sprintf("unexpected finding (exact): rule=%q effect=%q", af.Rule, af.Effect))
			}
		}
	}

	// 3. absent: a named rule that fired fails, in declared order.
	for _, name := range exp.Absent {
		if ruleFired(name, res.Findings) {
			reasons = append(reasons, fmt.Sprintf("rule %q fired but was asserted absent", name))
		}
	}

	// 4. score: pins the arithmetic. Total is Σ finding.Points over the enforcing
	// findings; threshold is the binding's approve threshold. The sum is a LOWER
	// BOUND on the engine's per-firing pointsSum (a K-firing finding carries the
	// per-firing weight, not K*weight — the documented aggregate asymmetry), so a
	// multi-firing divergence can only UNDERcount → a mismatch (the SAFE direction:
	// it fails, never spuriously passes on higher real risk). Faithful
	// firing-multiplied totals need an engine-exposed sum (logged OQ, parallel to
	// the path lane); the fixtures Match gates are single-firing, so total is exact.
	if exp.Score != nil {
		total := 0
		for i := range res.Findings {
			total += res.Findings[i].Points
		}
		if total != exp.Score.Total {
			reasons = append(reasons, fmt.Sprintf("score.total: expected %d, got %d", exp.Score.Total, total))
		}
		if threshold != exp.Score.Threshold {
			reasons = append(reasons, fmt.Sprintf("score.threshold: expected %d, got %d", exp.Score.Threshold, threshold))
		}
	}

	return reasons, nil
}

// findActual returns the first actual finding identity-matching ef (rule + effect,
// plus obligation when ef asserts one), or nil. Path/Message are refinements handled
// by the caller, not identity.
func findActual(ef ExpectFinding, actual []aggregate.Finding) *aggregate.Finding {
	for i := range actual {
		if findingMatches(ef, actual[i]) {
			return &actual[i]
		}
	}
	return nil
}

// coveredByExpected reports whether an actual finding is named by some expected
// finding (rule + effect, plus obligation when asserted) — the exact closed-list
// membership test.
func coveredByExpected(af aggregate.Finding, expected []ExpectFinding) bool {
	for _, ef := range expected {
		if findingMatches(ef, af) {
			return true
		}
	}
	return false
}

// findingMatches is the identity predicate shared by must-contain and exact: the
// rule and effect match, and — only when ef asserts an obligation — the obligation
// matches too (an omitted obligation does not constrain).
func findingMatches(ef ExpectFinding, af aggregate.Finding) bool {
	if ef.Rule != af.Rule || ef.Effect != string(af.Effect) {
		return false
	}
	if ef.Obligation != "" && ef.Obligation != af.Obligation {
		return false
	}
	return true
}

// ruleFired reports whether any actual finding carries the named rule.
func ruleFired(name string, actual []aggregate.Finding) bool {
	for i := range actual {
		if actual[i].Rule == name {
			return true
		}
	}
	return false
}

// obligationSuffix renders an optional " obligation=..." tail for a missing-finding
// reason, so the located error names the full asserted identity.
func obligationSuffix(obligation string) string {
	if obligation == "" {
		return ""
	}
	return fmt.Sprintf(" obligation=%q", obligation)
}
