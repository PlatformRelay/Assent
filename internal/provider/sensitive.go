package provider

import (
	"time"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// ToAggregateFact maps a host-classified provider Fact onto the EvaluationInput
// envelope (aggregate.Fact) bound into CEL.
//
// Sensitive propagation (REQ-E5-S04-02 / ADR-0012 A-08): Declaration.Sensitive
// becomes Fact.Sensitive on the bound envelope. The host only propagates the
// marker — E8 (forge adapter + presentation/renderer) owns redaction of
// comments, debug sections, traces, logs, and report artifacts. Values remain
// available to CEL predicates; redaction is a presentation concern, not a
// decision-path drop. Non-sensitive declarations leave Sensitive=false.
//
// Load-time maxAge for sensitive declarations is capped at MaxAgeSensitive
// (15m) by ValidateDeclarationMaxAge — longer maxAge is rejected, never clamped
// (REQ-E5-S04-01; provider-contract.md).
func ToAggregateFact(f Fact) aggregate.Fact {
	out := aggregate.Fact{
		State:      f.State,
		Sensitive:  f.Declaration.Sensitive,
		ObservedAt: f.ObservedAt.UTC().Format(time.RFC3339),
		Reason:     f.Reason,
	}
	if f.ExpiresAt != nil {
		out.ExpiresAt = f.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if f.State == StateResolved {
		out.Value = f.Value
	}
	return out
}
