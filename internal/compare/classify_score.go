package compare

import (
	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// isScoreThresholdChange reports whether the only delta between baseline and
// candidate is risk-threshold or rule.points arithmetic: identical intervention
// finding identities (rule/obligation/effect/subject/code — points excluded) but
// a different aggregate decision. Priority slot #4 (D-117).
func isScoreThresholdChange(baseline, candidate aggregate.Result) bool {
	if baseline.Decision == candidate.Decision {
		return false
	}
	return equalStrings(
		interventionKeys(baseline.Findings),
		interventionKeys(candidate.Findings),
	)
}
