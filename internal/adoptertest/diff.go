package adoptertest

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// diff.go is the PURE failure UX (P5-E6-S04). On a case FAILURE it renders a
// deterministic, located report: the expected-vs-actual decision, a finding-level
// diff (missing / effect-mismatched / unexpected), and a ready-to-copy expect.yaml
// actual block. A PASSING case is never rendered (the caller — cmd/assent/test.go —
// gates on !Outcome.Pass). All formatting lives here; the command shell only wires it.
//
// The serialization (ActualExpectation + MarshalExpectation) is factored as reusable,
// exported functions: S05's --update reuses the SAME two-step model→bytes path to
// WRITE the block into expect.yaml, so the copyable block a human hand-copies here is
// byte-for-byte what --update would write.
//
// Determinism (ADR-0014 golden L0): every function joins the engine's PRE-SORTED
// Result.Findings and the expectation's DECLARED order; nothing ranges a Go map to
// build output, so a double run is byte-identical.

// ActualExpectation reconstructs the expect.yaml #/$defs/expectation block that the
// produced Result would satisfy: the decision, the findings that actually fired (rule
// + obligation-when-present + effect, in the engine's canonical order), and the score
// (Σ points over the enforcing findings + the binding threshold). It deliberately
// emits NO findings[].path (D-054 — unsupported; a golden must never carry a path the
// matcher errors on) and NO message~ (a discouraged presentation assertion, never a
// golden). Must-contain is left as the default (no exact: true) — matching every
// shipped expect.yaml. threshold is the binding's approve threshold (aggregate.Result
// exposes none, so the caller threads it).
func ActualExpectation(res aggregate.Result, threshold int) Expectation {
	exp := Expectation{Decision: string(res.Decision)}
	total := 0
	for i := range res.Findings {
		af := res.Findings[i]
		exp.Findings = append(exp.Findings, ExpectFinding{
			Rule:       af.Rule,
			Obligation: af.Obligation,
			Effect:     string(af.Effect),
		})
		total += af.Points
	}
	// Score pins the arithmetic the SAME way Match recomputes it (Σ points over the
	// enforcing findings), so the emitted block round-trips against its own Result.
	exp.Score = &ExpectScore{Total: total, Threshold: threshold}
	return exp
}

// MarshalExpectation serializes an Expectation into the expect.yaml bytes a human (or
// S05's --update) can copy. FAIL-CLOSED: the emitted bytes MUST themselves
// strict-decode against the FROZEN test-expectation schema (LoadExpectation both
// validates and re-decodes) — else we would hand an author an invalid golden, or
// S05 would write one. A round-trip failure is an error, never silent bytes.
func MarshalExpectation(exp Expectation) ([]byte, error) {
	raw, err := yaml.Marshal(exp)
	if err != nil {
		return nil, fmt.Errorf("marshal expectation: %w", err)
	}
	if _, err := LoadExpectation(raw); err != nil {
		return nil, fmt.Errorf("serialized expectation does not round-trip against the frozen schema: %w", err)
	}
	return raw, nil
}

// RenderFailure assembles the deterministic S04 failure report for one case from the
// author's `expected` expectation and the evaluated `out` (whose ActualExpect carries
// the reconstructed actual block). It renders three parts: (1) expected-vs-actual
// decision; (2) a located finding-level diff; (3) the ready-to-copy actual block.
// When the structured finding diff is empty yet the case still failed on score/message
// alone (decision + findings matched), it falls back to the matcher's located Reasons
// so no failure is ever reported without an explanation. Pure and deterministic.
func RenderFailure(expected Expectation, out Outcome) (string, error) {
	block, err := MarshalExpectation(out.ActualExpect)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "FAIL %s\n", out.Name)
	fmt.Fprintf(&b, "  decision: expected %s, got %s\n", out.Expected, out.Actual)

	deltas := findingDiff(expected, out.ActualExpect)
	switch {
	case len(deltas) > 0:
		b.WriteString("  findings:\n")
		for _, d := range deltas {
			fmt.Fprintf(&b, "    %s\n", d)
		}
	case out.Expected == out.Actual:
		// Decision + findings matched: the case failed on score/message~ alone.
		// Surface the matcher's located Reasons so the failure is never unexplained.
		for _, r := range out.Reasons {
			fmt.Fprintf(&b, "  %s\n", r)
		}
	}

	b.WriteString("  actual (ready to copy into expect.yaml):\n")
	for _, line := range strings.Split(strings.TrimRight(string(block), "\n"), "\n") {
		fmt.Fprintf(&b, "    %s\n", line)
	}
	return b.String(), nil
}

// findingDiff computes the located finding-level delta between the author's `expected`
// expectation and the reconstructed `actual` one, joining on rule name (plus the
// obligation when the expectation asserts one). It classifies:
//   - missing:         an expected rule that did not fire at all;
//   - effect-mismatch: an expected rule that fired with a DIFFERENT effect (the case
//     the coarse must-contain matcher would report only as "missing", hiding that the
//     rule fired with the wrong effect);
//   - unexpected:      an actual rule the author did not list — emitted ONLY under
//     exact (under must-contain extras are allowed, so listing them would be a false
//     positive; they are already visible in the copyable block).
//
// Both inputs are declared/pre-sorted slices, so the output order is deterministic.
func findingDiff(expected, actual Expectation) []string {
	var deltas []string

	// missing / effect-mismatch: walk the author's declared order.
	for _, ef := range expected.Findings {
		af := findActualByRule(ef, actual.Findings)
		switch {
		case af == nil:
			deltas = append(deltas, fmt.Sprintf("missing: rule=%q effect=%q%s", ef.Rule, ef.Effect, obligationSuffix(ef.Obligation)))
		case ef.Effect != af.Effect:
			deltas = append(deltas, fmt.Sprintf("effect-mismatch: rule=%q expected effect=%q, got %q", ef.Rule, ef.Effect, af.Effect))
		}
	}

	// unexpected: only under exact — a fired rule the author never listed.
	if expected.Exact {
		for i := range actual.Findings {
			af := actual.Findings[i]
			if !expectedNamesRule(af, expected.Findings) {
				deltas = append(deltas, fmt.Sprintf("unexpected: rule=%q effect=%q", af.Rule, af.Effect))
			}
		}
	}

	return deltas
}

// findActualByRule returns the first actual finding sharing ef's rule (and, when ef
// asserts an obligation, its obligation), or nil — the rule-level join effect-mismatch
// needs (distinct from match.go's rule+effect identity join).
func findActualByRule(ef ExpectFinding, actual []ExpectFinding) *ExpectFinding {
	for i := range actual {
		if actual[i].Rule != ef.Rule {
			continue
		}
		if ef.Obligation != "" && ef.Obligation != actual[i].Obligation {
			continue
		}
		return &actual[i]
	}
	return nil
}

// expectedNamesRule reports whether some expected finding names af's rule (and, when
// it asserts an obligation, matches it) — the exact closed-list membership test at
// rule granularity (an effect difference is reported as effect-mismatch, not as an
// unexpected extra).
func expectedNamesRule(af ExpectFinding, expected []ExpectFinding) bool {
	for _, ef := range expected {
		if ef.Rule != af.Rule {
			continue
		}
		if ef.Obligation != "" && ef.Obligation != af.Obligation {
			continue
		}
		return true
	}
	return false
}
