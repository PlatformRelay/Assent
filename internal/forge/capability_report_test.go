package forge

import (
	"testing"
)

// capability_report_test.go covers the CapabilityReport surface the doctor
// and the fake consume (E10-S04): the zero-report substitution, the unknown
// entry helpers, and the deterministic Sorted view.

func TestAllUnknownCapabilityReportRefusesArming(t *testing.T) {
	report := AllUnknownCapabilityReport()
	for _, c := range AllCapabilities() {
		if report.State(c) != CapabilityUnknown {
			t.Fatalf("the all-unknown report must grade every capability unknown, %q = %q", c, report.State(c))
		}
	}
	probe := PreconditionFromReport(report)
	if probe.ArmEligible {
		t.Fatal("an all-unknown report must refuse arming (ADR-0021 §3 — the fake's fail-safe default)")
	}
}

func TestUnknownCapabilityEntryIsUnknown(t *testing.T) {
	entry := UnknownCapabilityEntry("why")
	if entry.State != CapabilityUnknown || entry.Reason != "why" {
		t.Fatalf("UnknownCapabilityEntry = %+v", entry)
	}
}

func TestCapabilityEntryHelpers(t *testing.T) {
	if got := SupportedCapabilityEntry("why").State; got != CapabilitySupported {
		t.Fatalf("SupportedCapabilityEntry.State = %q", got)
	}
	if got := AbsentCapabilityEntry("why").State; got != CapabilityAbsent {
		t.Fatalf("AbsentCapabilityEntry.State = %q", got)
	}
}

func TestCapabilityReportSortedIsDeterministic(t *testing.T) {
	report := mustReport(t, fullEntries(t))
	// Override three rows so the view carries all three states.
	entries := entryMapOf(report)
	entries[CapabilityProtectedPipelineSource] = UnknownCapabilityEntry("b")
	entries[CapabilityMergeResultPinning] = AbsentCapabilityEntry("c")
	report = mustReport(t, entries)
	views := report.Sorted()
	if len(views) != len(AllCapabilities()) {
		t.Fatalf("Sorted carried %d entries, want %d", len(views), len(AllCapabilities()))
	}
	var names []string
	for _, v := range views {
		names = append(names, v.Capability)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("Sorted is not in capability order: %v", names)
		}
	}
	again := report.Sorted()
	for i := range views {
		if views[i] != again[i] {
			t.Fatalf("Sorted is not deterministic: %v vs %v", views, again)
		}
	}
	for _, v := range views {
		if v.Capability == string(CapabilityMergeResultPinning) && (v.State != string(CapabilityAbsent) || v.Reason != "c") {
			t.Fatalf("the absent view lost its entry: %+v", views[0])
		}
	}
}

func TestStateAndReasonFallBackToUnknown(t *testing.T) {
	// A report consulted for a capability it does not carry: unknown + "" —
	// an adapter cannot dodge an answer by omission (the zero report reads
	// every capability as unknown).
	report := CapabilityReport{}
	if report.State(CapabilityEligibleApprovalEvidence) != CapabilityUnknown {
		t.Fatal("a missing entry must read as unknown")
	}
	if report.Reason(CapabilityEligibleApprovalEvidence) != "" {
		t.Fatal("a missing entry carries no reason")
	}
}

// fullEntries builds a complete entry map (every capability supported), the
// base the state-override tests vary.
func fullEntries(t *testing.T) map[Capability]CapabilityEntry {
	t.Helper()
	entries := make(map[Capability]CapabilityEntry, len(AllCapabilities()))
	for _, c := range AllCapabilities() {
		entries[c] = SupportedCapabilityEntry("a")
	}
	return entries
}

func TestEnumerationOpaqueReasonShape(t *testing.T) {
	complete := Snapshot{ChangedFilesComplete: true}
	if got := complete.EnumerationOpaqueReason(); got != "" {
		t.Fatalf("a complete enumeration has no opaque reason, got %q", got)
	}
	incomplete := Snapshot{ChangedFilesComplete: false, ChangedFilesGap: "the reason"}
	if got := incomplete.EnumerationOpaqueReason(); got != EnumerationIncompletePrefix+"the reason" {
		t.Fatalf("EnumerationOpaqueReason = %q", got)
	}
}
