package compare

import (
	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// detectMissedIntervention reports whether the baseline had a block or require-review
// intervention on an identity the candidate lacks while failing to reach APPROVE.
func detectMissedIntervention(baseline, candidate aggregate.Result) bool {
	if candidate.Decision == aggregate.DecisionApprove {
		return false
	}
	candByID := findingsByIdentity(candidate.Findings)
	for _, f := range baseline.Findings {
		if !isMissedInterventionEffect(f.Effect) {
			continue
		}
		if _, ok := candByID[deltaIdentity(f)]; !ok {
			return true
		}
	}
	return false
}

// detectStricterInterventionAdded reports whether the candidate adds a block,
// require-review, or challenge intervention on an identity the baseline APPROVE side
// lacked (same decision or lenient relative to the new intervention).
func detectStricterInterventionAdded(baseline, candidate aggregate.Result) bool {
	if baseline.Decision != aggregate.DecisionApprove {
		return false
	}
	baseByID := interventionFindingsByIdentity(baseline.Findings)
	for _, f := range candidate.Findings {
		if !isStricterInterventionEffect(f.Effect) {
			continue
		}
		if _, ok := baseByID[deltaIdentity(f)]; !ok {
			return true
		}
	}
	return false
}

// deltaIdentity is the per-delta identity key (rule, optional obligation, subject)
// used by ComparisonRecord and acceptedDeltas allowlists.
func deltaIdentity(f aggregate.Finding) string {
	return f.Rule + "|" + f.Obligation + "|" + f.Subject
}

func findingsByIdentity(findings []aggregate.Finding) map[string]aggregate.Finding {
	out := make(map[string]aggregate.Finding, len(findings))
	for _, f := range findings {
		out[deltaIdentity(f)] = f
	}
	return out
}

func interventionFindingsByIdentity(findings []aggregate.Finding) map[string]aggregate.Finding {
	out := make(map[string]aggregate.Finding, len(findings))
	for _, f := range findings {
		if !isStricterInterventionEffect(f.Effect) {
			continue
		}
		out[deltaIdentity(f)] = f
	}
	return out
}

func isMissedInterventionEffect(e aggregate.Effect) bool {
	return e == aggregate.EffectBlock || e == aggregate.EffectRequireReview
}

func isStricterInterventionEffect(e aggregate.Effect) bool {
	return e == aggregate.EffectBlock || e == aggregate.EffectRequireReview || e == aggregate.EffectChallenge
}
