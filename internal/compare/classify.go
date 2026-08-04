package compare

import (
	"fmt"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// classify places the baseline->candidate delta into one taxonomy Kind, returns a
// zero Kind ("") when the two Results fully agree (no delta), or a wrapped
// ErrUnclassifiable when the delta is real but matches none of the classified kinds
// (fail-closed). Per-delta kinds are resolved at finding identity and the highest-
// priority kind wins (D-117): missed intervention > uncovered (S03) >
// newly-auto-mergeable > score-threshold (S03) > stricter-added > explanation-only.
func classify(baseline, candidate aggregate.Result) (Kind, error) {
	if baseline.Decision == candidate.Decision {
		return classifyEqualDecision(baseline, candidate)
	}

	if detectMissedIntervention(baseline, candidate) {
		return KindDestructiveOrAuthorizationInterventionMissed, nil
	}

	if candidate.Decision == aggregate.DecisionApprove && baseline.Decision != aggregate.DecisionApprove {
		return KindNewlyAutoMergeable, nil
	}

	if detectStricterInterventionAdded(baseline, candidate) {
		return KindStricterInterventionAdded, nil
	}

	return "", fmt.Errorf("%w: baseline=%s candidate=%s",
		ErrUnclassifiable, baseline.Decision, candidate.Decision)
}

// classifyEqualDecision resolves a same-decision pair: identical finding
// identities with a differing message is explanation-only (wording-only, never
// gate-tripping); byte-identical is no delta (""); differing finding identities is
// a real finding-level semantic delta -> fail-closed until S03 classifiers land.
func classifyEqualDecision(baseline, candidate aggregate.Result) (Kind, error) {
	idBase := findingKeys(baseline.Findings, false)
	idCand := findingKeys(candidate.Findings, false)
	if !equalStrings(idBase, idCand) {
		return "", fmt.Errorf("%w: identical decision %s but finding identities differ", ErrUnclassifiable, baseline.Decision)
	}
	fullBase := findingKeys(baseline.Findings, true)
	fullCand := findingKeys(candidate.Findings, true)
	if equalStrings(fullBase, fullCand) {
		return "", nil // fully agree: no delta
	}
	return KindExplanationOnly, nil
}

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
