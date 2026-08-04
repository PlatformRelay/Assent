package compare

import "testing"

func TestExitCodeForGateADRTable(t *testing.T) {
	tests := []struct {
		gate GateID
		want int
	}{
		{GateZeroMissedDestructive, 1},
		{GateZeroMissedAuthorizationOwnership, 2},
		{GateNoUnexpectedObligationRemoval, 3},
		{GateBoundedAutoMergeWidening, 4},
		{GateExplicitlyAcceptedDeltas, 5},
	}
	for _, tc := range tests {
		if got := ExitCodeForGate(tc.gate); got != tc.want {
			t.Errorf("ExitCodeForGate(%q) = %d, want %d", tc.gate, got, tc.want)
		}
	}
}

func TestExitCodeForGateUnknown(t *testing.T) {
	if got := ExitCodeForGate(GateID("unknown-gate")); got != ExitCodeFailClosed {
		t.Fatalf("unknown gate exit = %d, want %d", got, ExitCodeFailClosed)
	}
}

func TestExitCodeForSuiteRun(t *testing.T) {
	if got := ExitCodeForSuiteRun(GateEvaluation{FirstFailure: ""}); got != 0 {
		t.Fatalf("all-pass exit = %d, want 0", got)
	}
	ev := GateEvaluation{FirstFailure: GateBoundedAutoMergeWidening}
	if got := ExitCodeForSuiteRun(ev); got != 4 {
		t.Fatalf("widening fail exit = %d, want 4", got)
	}
}
