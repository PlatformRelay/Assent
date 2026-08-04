package compare

import (
	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// isObligationUncovered reports whether the candidate newly fails to cover a
// required obligation the baseline covered, with a degraded decision. Priority
// slot #2 in the classifier table (D-117): missed intervention > uncovered > …
func isObligationUncovered(baseline, candidate aggregate.Result) bool {
	baseUnc := uncoveredObligations(baseline)
	candUnc := uncoveredObligations(candidate)
	for obl := range candUnc {
		if baseUnc[obl] {
			continue
		}
		if decisionWorsened(baseline.Decision, candidate.Decision) {
			return true
		}
	}
	return false
}

// uncoveredObligations returns obligation names with an aggregate.uncovered
// finding in the result (fail-safe REVIEW from a required obligation with no
// enforce-phase proving rule).
func uncoveredObligations(r aggregate.Result) map[string]bool {
	out := map[string]bool{}
	for _, f := range r.Findings {
		if f.Code != "obligation.uncovered" {
			continue
		}
		if f.Obligation != "" {
			out[f.Obligation] = true
		}
	}
	return out
}

// decisionWorsened reports whether cand is strictly worse than base on the
// APPROVE < REVIEW < BLOCK lattice.
func decisionWorsened(base, cand aggregate.Decision) bool {
	return decisionRank(cand) > decisionRank(base)
}

func decisionRank(d aggregate.Decision) int {
	switch d {
	case aggregate.DecisionApprove:
		return 0
	case aggregate.DecisionReview:
		return 1
	case aggregate.DecisionBlock:
		return 2
	default:
		return 1
	}
}
