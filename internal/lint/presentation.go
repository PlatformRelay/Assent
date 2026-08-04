package lint

// presentation.go is the E8-S11 layer: extends E3-S04 predicate-scope lint to
// docs.summary and debug: CEL message lines (D-095), and rejects tier-1 template
// overrides under .assent/templates/ with an explicit deferred error (ADR-0016
// tier 1 — no loader in this epic).
//
// Purity: pure — string prefix scan over already-read Source paths; scope checks
// reuse aggregate.CompileCheck via scope.go's checkMessageScope.

import (
	"fmt"
	"strings"
)

// CodeTier1Deferred reports .assent/templates/ is present but tier-1 slot
// overrides are not implemented (ADR-0016 tier 1 — designed seam only).
const CodeTier1Deferred = "tier-1-deferred"

const tier1TemplatesPrefix = ".assent/templates/"

// checkPresentationLint runs E8-S11 checks: tier-1 template directory rejection.
// docs.summary / debug: scope lint lives in scope.go (checkPredicateScope).
func checkPresentationLint(sources []Source, rep *Report) {
	checkTier1Templates(sources, rep)
}

// checkTier1Templates emits one tier-1-deferred error when any ingested source
// path lies under .assent/templates/ — never silently ignored.
func checkTier1Templates(sources []Source, rep *Report) {
	for _, s := range sources {
		if !strings.HasPrefix(s.Path, tier1TemplatesPrefix) {
			continue
		}
		rep.addError(CodeTier1Deferred, Location{File: tier1TemplatesPrefix}, fmt.Sprintf(
			"presentation tier-1 template overrides under %q are deferred (ADR-0016 tier 1 — slot overrides are not implemented); remove %q or wait for a future release",
			tier1TemplatesPrefix, tier1TemplatesPrefix))
		return
	}
}
