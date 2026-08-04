package compare

// ExitCodeFailClosed is the process exit for load/schema/digest/classification
// errors distinct from every promotion-gate code (ADR-0018 / D-115).
const ExitCodeFailClosed = 6

// ExitCodeForGate maps a failing promotion gate to its ADR-0018 exit code (1–5).
// Unknown gate IDs map to ExitCodeFailClosed.
func ExitCodeForGate(id GateID) int {
	switch id {
	case GateZeroMissedDestructive:
		return 1
	case GateZeroMissedAuthorizationOwnership:
		return 2
	case GateNoUnexpectedObligationRemoval:
		return 3
	case GateBoundedAutoMergeWidening:
		return 4
	case GateExplicitlyAcceptedDeltas:
		return 5
	default:
		return ExitCodeFailClosed
	}
}

// ExitCodeForSuiteRun returns 0 when all gates pass, otherwise the ADR exit code
// for the first failing gate.
func ExitCodeForSuiteRun(gates GateEvaluation) int {
	if gates.FirstFailure == "" {
		return 0
	}
	return ExitCodeForGate(gates.FirstFailure)
}
