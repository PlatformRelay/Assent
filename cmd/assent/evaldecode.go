package main

import (
	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/evaldecode"
)

// evaldecode.go keeps the package-main names this CLI's run/test paths call, now
// thin wrappers over the extracted internal/evaldecode boundary (P5-E6-S01). The
// canonical decoder — and the load-bearing "6" >= "12" lexical fail-open fix it
// closes — lives in internal/evaldecode so the `assent` shell and the pure
// internal/adoptertest harness share ONE implementation (drift would reopen it).

// subjectOf derives a change's governed-subject entryRef. See
// internal/evaldecode.SubjectOf.
func subjectOf(c change.Change) string { return evaldecode.SubjectOf(c) }

// buildEvaluationInput assembles the engine's aggregate.EvaluationInput from the
// live differ ChangeSet. See internal/evaldecode.BuildEvaluationInput.
func buildEvaluationInput(cs change.ChangeSet, mr aggregate.MR, require []string) aggregate.EvaluationInput {
	return evaldecode.BuildEvaluationInput(cs, mr, require)
}
