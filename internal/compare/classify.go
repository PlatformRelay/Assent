package compare

import (
	"fmt"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// classify places the baseline->candidate delta into one taxonomy Kind, returns a
// zero Kind ("") when the two Results fully agree (no delta), or a wrapped
// ErrUnclassifiable when the delta is real but matches none of the classified kinds
// (fail-closed). Per-delta kinds are resolved at finding identity and the highest-
// priority kind wins (D-117): missed intervention > uncovered > newly-auto-mergeable
// > score-threshold > stricter-added > explanation-only.
func classify(baseline, candidate aggregate.Result) (Kind, error) {
	if baseline.Decision == candidate.Decision {
		return classifyEqualDecision(baseline, candidate)
	}

	if detectMissedIntervention(baseline, candidate) {
		return KindDestructiveOrAuthorizationInterventionMissed, nil
	}

	if isObligationUncovered(baseline, candidate) {
		return KindSubjectOrObligationUncovered, nil
	}

	interventionsEqual := equalStrings(
		interventionKeys(baseline.Findings),
		interventionKeys(candidate.Findings),
	)

	if candidate.Decision == aggregate.DecisionApprove && baseline.Decision != aggregate.DecisionApprove {
		if interventionsEqual {
			return KindScoreThresholdChange, nil
		}
		return KindNewlyAutoMergeable, nil
	}

	if isScoreThresholdChange(baseline, candidate) {
		return KindScoreThresholdChange, nil
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
// a real finding-level semantic delta -> fail-closed.
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
