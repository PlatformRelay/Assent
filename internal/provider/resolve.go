package provider

import (
	"context"
	"encoding/json"
	"time"

	"github.com/PlatformRelay/assent/schemas"
)

// CallFunc is one transport invocation (HTTP or exec) for a FactQuery.
// S01 uses stub CallFuncs in tests; S03 lands real transports.
type CallFunc func(ctx context.Context) ([]byte, error)

// Result is the host classification of one provider ResolveFacts call.
// Every requested output appears in Facts with an explicit state — never omitted,
// never marked resolved on a failure path (REQ-E5-S01-01).
type Result struct {
	Facts       map[string]Fact
	Negotiation Negotiation
}

// AutoMergeEligible reports whether facts from this resolve may arm auto-merge.
// False on capability gap (major mismatch) and on any host-synthesized failure
// path where negotiation did not accept the provider.
func (r Result) AutoMergeEligible() bool {
	return r.Negotiation.Outcome == OutcomeAccept && r.Negotiation.AutoMergeEligible
}

// ResolveFacts is the host side of the protocol: it calls the provider and
// classifies the outcome into exactly one Fact per requested output. Failure
// paths land in distinct non-resolved states — a controlling fact is therefore
// fail-closed by construction and never a silently absent key.
//
// Classifier (Spike C, preserved):
//   - transport/call error → unavailable
//   - undecodable body → invalid
//   - Negotiate major mismatch → unavailable + AutoMergeEligible=false
//   - schema / queryId / omitted output → invalid
//   - expiresAt <= asOf (now) → expired (value dropped)
//
// `now` is injected by the host (the pinned evaluation instant), never read
// from a wall clock here. Declaration cross-check is opt-in via
// ResolveFactsChecked (REQ-E5-S02-04).
func ResolveFacts(ctx context.Context, call CallFunc, q FactQuery, now time.Time) Result {
	return ResolveFactsChecked(ctx, call, q, now, nil)
}

// ResolveFactsChecked is ResolveFacts plus an optional host declaration
// cross-check: when expected is non-nil, each returned fact's echoed
// declaration must match config on type/cardinality/subject/sensitive/maxAge;
// mismatch → invalid with value dropped (never silently accept).
func ResolveFactsChecked(ctx context.Context, call CallFunc, q FactQuery, now time.Time, expected map[string]Declaration) Result {
	raw, err := call(ctx)
	if err != nil {
		return failClosed(q, StateUnavailable, "provider call failed: "+err.Error(), now, refusedNegotiation(q.Outputs))
	}

	// Soft-decode before schema validation so a mismatched major is refused as a
	// capability gap (unavailable) rather than processed under the host schema
	// (which const-locks apiVersion to v1alpha1 and would otherwise yield invalid).
	var resp FactResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return failClosed(q, StateInvalid, "response undecodable", now, refusedNegotiation(q.Outputs))
	}

	neg := Negotiate(HostMajor, resp.APIVersion, q.Outputs)
	if neg.Outcome != OutcomeAccept {
		return failClosed(q, StateUnavailable, "provider protocol major capability gap", now, neg)
	}

	if err := schemas.ValidateProviderResponse(raw); err != nil {
		return failClosed(q, StateInvalid, "response failed schema validation", now, neg)
	}

	// Re-decode after schema validation (defensive; same bytes).
	if err := json.Unmarshal(raw, &resp); err != nil {
		return failClosed(q, StateInvalid, "response undecodable", now, neg)
	}
	if resp.QueryID != q.QueryID {
		return failClosed(q, StateInvalid, "queryId mismatch", now, neg)
	}

	byName := make(map[string]Fact, len(resp.Facts))
	for _, f := range resp.Facts {
		byName[f.Name] = f
	}

	out := make(map[string]Fact, len(q.Outputs))
	for _, name := range q.Outputs {
		fact, ok := byName[name]
		if !ok {
			out[name] = synthesize(q, name, StateInvalid, "provider omitted a requested output", now)
			continue
		}
		if fact.State == StateResolved && fact.ExpiresAt != nil && !fact.ExpiresAt.After(now) {
			fact.State = StateExpired
			fact.Reason = "expiresAt is not after the evaluation instant"
			fact.Value = nil
		}
		// Never let a non-resolved provider fact carry a value into CEL.
		if fact.State != StateResolved {
			fact.Value = nil
		}
		if expected != nil {
			want, have := expected[name]
			if !have {
				fact = synthesize(q, name, StateInvalid, "host config has no declaration for requested output", now)
			} else if !DeclarationsEqual(want, fact.Declaration) {
				fact = synthesize(q, name, StateInvalid, "provider echoed declaration does not match host config", now)
			}
		}
		out[name] = fact
	}
	return Result{Facts: out, Negotiation: neg}
}

func failClosed(q FactQuery, state, reason string, now time.Time, neg Negotiation) Result {
	return Result{
		Facts:       synthesizeAll(q, state, reason, now),
		Negotiation: neg,
	}
}

// refusedNegotiation is used when the host never accepted a provider major
// (transport failure / undecodable body). Auto-merge must stay disarmed.
func refusedNegotiation(outputs []string) Negotiation {
	return capabilityGap(Negotiation{HostMajor: HostMajor}, outputs, nil)
}

func synthesizeAll(q FactQuery, state, reason string, now time.Time) map[string]Fact {
	out := make(map[string]Fact, len(q.Outputs))
	for _, name := range q.Outputs {
		out[name] = synthesize(q, name, state, reason, now)
	}
	return out
}

func synthesize(q FactQuery, name, state, reason string, now time.Time) Fact {
	return Fact{
		Name:       name,
		State:      state,
		Subject:    q.Subject,
		ObservedAt: now,
		Reason:     reason,
	}
}
