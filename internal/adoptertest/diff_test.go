package adoptertest

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// diff_test.go covers the S04 failure UX: the reconstructed ready-to-copy actual
// block (ActualExpectation + MarshalExpectation), the located finding-level diff
// (findingDiff), the assembled report (RenderFailure), and determinism. The pure
// formatting lives in internal/adoptertest; cmd/assent/test.go only wires it in.

// TestDecisionMismatchRendersCopyableActual (REQ-E6-S04-01): a decision mismatch
// renders expected vs actual AND a ready-to-copy actual block that itself
// strict-decodes against the frozen schema and round-trips through the matcher.
func TestDecisionMismatchRendersCopyableActual(t *testing.T) {
	const threshold = 4
	res := result(aggregate.DecisionReview,
		finding("partitions-within-cap", "capped", "challenge", 1),
	)
	out := Outcome{
		Name:         "capped/over-cap",
		Pass:         false,
		Expected:     "APPROVE",
		Actual:       "REVIEW",
		ActualExpect: ActualExpectation(res, threshold),
		Reasons:      []string{"decision: expected APPROVE, got REVIEW"},
	}

	report, err := RenderFailure(Expectation{Decision: "APPROVE"}, out)
	if err != nil {
		t.Fatalf("RenderFailure: %v", err)
	}
	if !strings.Contains(report, "expected APPROVE, got REVIEW") {
		t.Fatalf("report missing decision diff:\n%s", report)
	}

	// The ready-to-copy block must itself strict-decode against the FROZEN schema…
	block, err := MarshalExpectation(out.ActualExpect)
	if err != nil {
		t.Fatalf("MarshalExpectation: %v", err)
	}
	reDecoded, err := LoadExpectation(block)
	if err != nil {
		t.Fatalf("emitted block does not strict-decode against the frozen schema: %v\nblock:\n%s", err, block)
	}
	// …and round-trip: re-decoding it and matching against the SAME Result yields
	// zero reasons (the property S05's --update "re-run passes" relies on).
	reasons, err := Match(reDecoded, res, threshold)
	if err != nil {
		t.Fatalf("Match on round-tripped block: %v", err)
	}
	if len(reasons) != 0 {
		t.Fatalf("round-tripped block did not match its own Result, reasons: %v\nblock:\n%s", reasons, block)
	}
	// The report must embed the same block bytes verbatim (indented).
	if !strings.Contains(report, "partitions-within-cap") {
		t.Fatalf("report missing the copyable actual block:\n%s", report)
	}
}

// TestFindingDiffEnumeratesDeltas (REQ-E6-S04-02): a finding mismatch enumerates
// missing / unexpected / effect-mismatched findings, located to the rule.
func TestFindingDiffEnumeratesDeltas(t *testing.T) {
	const threshold = 10
	// Actual: rule-a fired as comment (author expected block -> effect-mismatch);
	// rule-c fired (author never listed it -> unexpected, since exact). rule-b was
	// expected but never fired -> missing.
	res := result(aggregate.DecisionReview,
		finding("rule-a", "", "comment", 0),
		finding("rule-c", "", "comment", 0),
	)
	expected := Expectation{
		Decision: "BLOCK",
		Exact:    true,
		Findings: []ExpectFinding{
			{Rule: "rule-a", Effect: "block"},
			{Rule: "rule-b", Effect: "challenge"},
		},
	}

	deltas := findingDiff(expected, ActualExpectation(res, threshold))
	joined := strings.Join(deltas, "\n")

	for _, want := range []string{
		"effect-mismatch", "rule-a", // rule-a: expected block, got comment
		"missing", "rule-b", // rule-b never fired
		"unexpected", "rule-c", // rule-c fired but was not listed (exact)
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("finding diff missing %q:\n%s", want, joined)
		}
	}

	// Must-contain default: an extra actual finding is NOT an "unexpected" delta.
	mustContain := Expectation{
		Decision: "REVIEW",
		Findings: []ExpectFinding{{Rule: "rule-a", Effect: "comment"}},
	}
	if d := findingDiff(mustContain, ActualExpectation(res, threshold)); len(d) != 0 {
		t.Fatalf("must-contain diff should ignore extras, got: %v", d)
	}
}

// TestDiffOutputDoubleRunStable (REQ-E6-S04-04): the diff output is deterministic —
// canonical ordering, double-run byte-identical, across the block and the report.
func TestDiffOutputDoubleRunStable(t *testing.T) {
	const threshold = 4
	res := result(aggregate.DecisionReview,
		finding("size-bounded", "bounded", "challenge", 3),
		finding("author-owns-entry", "ownership", "require-review", 0),
	)
	out := Outcome{
		Name:         "pack/case",
		Pass:         false,
		Expected:     "APPROVE",
		Actual:       "REVIEW",
		ActualExpect: ActualExpectation(res, threshold),
	}
	expected := Expectation{Decision: "APPROVE"}

	block1, err := MarshalExpectation(out.ActualExpect)
	if err != nil {
		t.Fatalf("MarshalExpectation: %v", err)
	}
	block2, err := MarshalExpectation(ActualExpectation(res, threshold))
	if err != nil {
		t.Fatalf("MarshalExpectation (2): %v", err)
	}
	if string(block1) != string(block2) {
		t.Fatalf("actual block not byte-stable:\n--- 1 ---\n%s\n--- 2 ---\n%s", block1, block2)
	}

	r1, err := RenderFailure(expected, out)
	if err != nil {
		t.Fatalf("RenderFailure: %v", err)
	}
	r2, err := RenderFailure(expected, out)
	if err != nil {
		t.Fatalf("RenderFailure (2): %v", err)
	}
	if r1 != r2 {
		t.Fatalf("report not byte-stable:\n--- 1 ---\n%s\n--- 2 ---\n%s", r1, r2)
	}
}

// TestScoreOnlyFailureFallsBackToReasons proves a case that fails on score alone
// (decision + findings match) still explains itself via the matcher's Reasons — no
// failure is ever reported without a located explanation.
func TestScoreOnlyFailureFallsBackToReasons(t *testing.T) {
	const threshold = 4
	res := result(aggregate.DecisionReview,
		finding("partitions-within-cap", "capped", "challenge", 1),
	)
	expected := Expectation{
		Decision: "REVIEW",
		Findings: []ExpectFinding{{Rule: "partitions-within-cap", Obligation: "capped", Effect: "challenge"}},
	}
	out := Outcome{
		Name:         "capped/over-cap",
		Pass:         false,
		Expected:     "REVIEW",
		Actual:       "REVIEW",
		ActualExpect: ActualExpectation(res, threshold),
		Reasons:      []string{"score.total: expected 99, got 1"},
	}
	report, err := RenderFailure(expected, out)
	if err != nil {
		t.Fatalf("RenderFailure: %v", err)
	}
	if !strings.Contains(report, "score.total: expected 99, got 1") {
		t.Fatalf("score-only failure not surfaced:\n%s", report)
	}
}
