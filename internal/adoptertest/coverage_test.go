package adoptertest_test

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
)

// cappedEnforceObl is the capped fixture pack's enforce obligation rule universe
// (its single rule). The command derives this from the catalogue (enforce phase +
// non-empty obligation); the pure tests pass it directly.
var cappedEnforceObl = map[string]bool{"partitions-within-cap": true}

// witnessOf runs one fixture directory case through the coverage path and returns
// its polarity witness, failing the test unless the case's own assertions held (a
// case that does not pass is not valid coverage evidence).
func witnessOf(t *testing.T, name string) adoptertest.CaseWitness {
	t.Helper()
	out, w, err := adoptertest.RunCaseCoverage(loadCase(t, name), cappedEnforceObl)
	if err != nil {
		t.Fatalf("RunCaseCoverage %s: %v", name, err)
	}
	if !out.Pass {
		t.Fatalf("case %s did not pass (expected %q, got %q; reasons %v)", name, out.Expected, out.Actual, out.Reasons)
	}
	return w
}

// creditsOf converts a case witness into pack-tagged polarity credits (what the
// command accumulates from a passing case).
func creditsOf(pack string, w adoptertest.CaseWitness) []adoptertest.CoverageCredit {
	var cs []adoptertest.CoverageCredit
	for _, r := range w.Failing {
		cs = append(cs, adoptertest.CoverageCredit{Pack: pack, Rule: r, Failing: true})
	}
	for _, r := range w.Proving {
		cs = append(cs, adoptertest.CoverageCredit{Pack: pack, Rule: r, Proving: true})
	}
	return cs
}

func hasStr(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// TestBothPolarityCoverageRequired (REQ-E6-S07-01) proves the both-polarity gate: a
// rule exercised by BOTH a proving case (matched + silent) and a failing case (it
// fires) is COVERED; a rule exercised only in the proving polarity is NOT covered
// and the report NAMES the missing failing case.
func TestBothPolarityCoverageRequired(t *testing.T) {
	universe := []adoptertest.CoverageRule{{Pack: "capped", Rule: "partitions-within-cap"}}

	provingW := witnessOf(t, "within-cap") // APPROVE: rule matched + silent -> proving
	failingW := witnessOf(t, "over-cap")   // REVIEW: findings pin the rule -> failing

	// The witnesses attribute the polarities as expected.
	if !hasStr(provingW.Proving, "partitions-within-cap") || len(provingW.Failing) != 0 {
		t.Fatalf("within-cap should prove the rule silent only, got %+v", provingW)
	}
	if !hasStr(failingW.Failing, "partitions-within-cap") || len(failingW.Proving) != 0 {
		t.Fatalf("over-cap should pin the rule failing only, got %+v", failingW)
	}

	t.Run("both polarities -> covered, exit gate satisfied", func(t *testing.T) {
		credits := append(creditsOf("capped", provingW), creditsOf("capped", failingW)...)
		rep := adoptertest.BuildCoverageReport(universe, credits)
		if !rep.Complete() {
			t.Fatalf("a both-polarity pack must be complete:\n%s", adoptertest.RenderCoverage(rep))
		}
	})

	t.Run("only proving -> not covered, names the missing failing case", func(t *testing.T) {
		rep := adoptertest.BuildCoverageReport(universe, creditsOf("capped", provingW))
		if rep.Complete() {
			t.Fatalf("a proving-only pack must NOT be complete")
		}
		out := adoptertest.RenderCoverage(rep)
		if !strings.Contains(out, "capped/partitions-within-cap") || !strings.Contains(out, "no failing case") {
			t.Fatalf("report must name the rule and its missing failing polarity:\n%s", out)
		}
	})
}

// TestCoverageCountsSafetyAssertionsOnly (REQ-E6-S07-02) proves `--coverage` counts
// only STRUCTURED safety assertions (rule + effect): a finding asserted only via
// `message~` (no structured effect) does NOT establish failing-polarity coverage,
// while the same finding with a structured effect does — the message text never
// being the coverage signal.
func TestCoverageCountsSafetyAssertionsOnly(t *testing.T) {
	base := loadCase(t, "over-cap") // a case where the rule genuinely fires

	t.Run("a message~-only finding does not count toward coverage", func(t *testing.T) {
		c := base
		c.Expect = adoptertest.Expectation{
			Decision: "REVIEW",
			Findings: []adoptertest.ExpectFinding{
				{Rule: "partitions-within-cap", Message: "over the cap"}, // presentation only, no effect
			},
		}
		_, w, err := adoptertest.RunCaseCoverage(c, cappedEnforceObl)
		if err != nil {
			t.Fatalf("RunCaseCoverage: %v", err)
		}
		if len(w.Failing) != 0 {
			t.Fatalf("a message~-only finding must not count as failing coverage, got %+v", w.Failing)
		}
	})

	t.Run("the same finding with a structured effect does count", func(t *testing.T) {
		c := base
		c.Expect = adoptertest.Expectation{
			Decision: "REVIEW",
			Findings: []adoptertest.ExpectFinding{
				{Rule: "partitions-within-cap", Effect: "challenge", Message: "over the cap"},
			},
		}
		_, w, err := adoptertest.RunCaseCoverage(c, cappedEnforceObl)
		if err != nil {
			t.Fatalf("RunCaseCoverage: %v", err)
		}
		if !hasStr(w.Failing, "partitions-within-cap") {
			t.Fatalf("a structured rule+effect finding must count as failing coverage, got %+v", w.Failing)
		}
	})
}

// TestCoverageReportDoubleRunStable (REQ-E6-S07-04) proves the report is
// deterministic: canonical rule (ID) ordering and a double render byte-identical.
func TestCoverageReportDoubleRunStable(t *testing.T) {
	universe := []adoptertest.CoverageRule{
		{Pack: "zeta", Rule: "r1"},
		{Pack: "alpha", Rule: "r2"},
		{Pack: "alpha", Rule: "r1"},
	}
	credits := []adoptertest.CoverageCredit{
		{Pack: "zeta", Rule: "r1", Proving: true},
		{Pack: "alpha", Rule: "r1", Proving: true},
		{Pack: "alpha", Rule: "r1", Failing: true},
	}

	rep1 := adoptertest.BuildCoverageReport(universe, credits)
	rep2 := adoptertest.BuildCoverageReport(universe, credits)
	out1 := adoptertest.RenderCoverage(rep1)
	out2 := adoptertest.RenderCoverage(rep2)
	if out1 != out2 {
		t.Fatalf("coverage report not byte-identical across renders:\n#1 %s\n#2 %s", out1, out2)
	}

	// Canonical ID ordering: alpha/r1 < alpha/r2 < zeta/r1.
	iA := strings.Index(out1, "alpha/r1")
	iB := strings.Index(out1, "alpha/r2")
	iC := strings.Index(out1, "zeta/r1")
	if iA < 0 || iB < 0 || iC < 0 || iA >= iB || iB >= iC {
		t.Fatalf("rules not rendered in canonical ID order:\n%s", out1)
	}
}
