package compare

// PromotionGate is one row of a PolicyComparisonSuite promotionGates table.
type PromotionGate struct {
	GateID         GateID
	FailOnKinds    []Kind
	DefaultVerdict string
	Acceptance     string
}

// AcceptedDelta is one explicitly allowlisted delta identity from suite spec.
type AcceptedDelta struct {
	CaseID     string
	Kind       Kind
	Rule       string
	Subject    string
	Obligation string
	Rationale  string
}

// PolicyComparisonSuiteSpec holds the gate table and acceptedDeltas allowlist
// consumed by EvaluateGates. Full suite loading (PCS-S06) strict-decodes bytes
// into this shape; S05 evaluates an in-memory spec against ComparisonRecords.
type PolicyComparisonSuiteSpec struct {
	PromotionGates []PromotionGate
	AcceptedDeltas []AcceptedDelta
}

// GateEvaluation is the per-gate PASS/FAIL outcome for a suite run.
type GateEvaluation struct {
	// Results maps each gateId from the input table to PASS or FAIL.
	Results map[GateID]Verdict
	// FirstFailure is the first gate (table order) that FAILed, or "" when all pass.
	FirstFailure GateID
}

const (
	// GateZeroMissedDestructive fails on destructive-or-authorization-intervention-missed.
	GateZeroMissedDestructive GateID = "zero-missed-destructive"
	// GateZeroMissedAuthorizationOwnership fails on the same missed-intervention kind
	// under the authorization/ownership reporting axis (ADR-0018).
	GateZeroMissedAuthorizationOwnership GateID = "zero-missed-authorization-ownership"
	// GateNoUnexpectedObligationRemoval fails on subject-or-obligation-uncovered deltas.
	GateNoUnexpectedObligationRemoval GateID = "no-unexpected-obligation-removal"
	// GateExplicitlyAcceptedDeltas fails on stricter-intervention-added and
	// score-threshold-change unless individually allowlisted.
	GateExplicitlyAcceptedDeltas GateID = "explicitly-accepted-deltas"
)

// EvaluateGates applies the suite promotionGates table to the supplied
// ComparisonRecords. For each gate, any case delta whose kind is listed in
// failOnKinds trips the gate unless acceptance is per-delta-identity and the
// delta matches an acceptedDeltas entry keyed by caseId+kind+rule+subject
// (+obligation when present). explanation-only never fails any gate. Pure.
func EvaluateGates(spec PolicyComparisonSuiteSpec, records []ComparisonRecord) GateEvaluation {
	results := make(map[GateID]Verdict, len(spec.PromotionGates))
	var firstFailure GateID

	for _, gate := range spec.PromotionGates {
		verdict := evaluateGate(gate, spec.AcceptedDeltas, records)
		results[gate.GateID] = verdict
		if verdict == VerdictFail && firstFailure == "" {
			firstFailure = gate.GateID
		}
	}
	return GateEvaluation{Results: results, FirstFailure: firstFailure}
}

func evaluateGate(gate PromotionGate, accepted []AcceptedDelta, records []ComparisonRecord) Verdict {
	for _, rec := range records {
		for _, delta := range rec.Deltas {
			if delta.Kind == KindExplanationOnly {
				continue
			}
			if !kindInFailOn(delta.Kind, gate.FailOnKinds) {
				continue
			}
			if gate.Acceptance == "per-delta-identity" && isAcceptedDelta(accepted, rec.CaseID, delta) {
				continue
			}
			if gate.DefaultVerdict == "pass" {
				continue
			}
			return VerdictFail
		}
	}
	return VerdictPass
}

func kindInFailOn(k Kind, kinds []Kind) bool {
	for _, fk := range kinds {
		if fk == k {
			return true
		}
	}
	return false
}

func isAcceptedDelta(accepted []AcceptedDelta, caseID string, delta Delta) bool {
	for _, ad := range accepted {
		if matchesAcceptedDelta(ad, caseID, delta) {
			return true
		}
	}
	return false
}

func matchesAcceptedDelta(ad AcceptedDelta, caseID string, delta Delta) bool {
	if ad.CaseID != caseID || ad.Kind != delta.Kind || ad.Rule != delta.Rule || ad.Subject != delta.Subject {
		return false
	}
	if ad.Obligation != "" {
		return ad.Obligation == delta.Obligation
	}
	return true
}
