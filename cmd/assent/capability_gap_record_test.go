package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// capability_gap_record_test.go pins S00 Q3's conformance cases at the record
// surface (ADR-0021 item 8 / change p5-e10-github-forge-execution):
//
//	capability-gap-string-is-merge-result-only — N extra absent/unknown
//	    capabilities leave pins.capabilityGap byte-identical to a run with none
//	    of them; the doctor report carries all N. This is the case that reddens
//	    on the comma-join adapter.
//	capability-gap-positive-control            — the merge-result gap itself IS
//	    still recorded, so the case above is not satisfiable by an empty string.
//
// There is no gap-selection problem to solve (S00 Q3): pins.capabilityGap is
// reserved for merge-result pinning; the other ten capabilities report to
// doctor. This pair is what reddens on a future adapter that tries to
// comma-join the report into the record.

// recordCapabilityGap runs the armed fixture once and returns the
// pins.capabilityGap value the emitted record carried.
func recordCapabilityGap(f *fakeGitLab, t *testing.T) string {
	t.Helper()
	var out bytes.Buffer
	if code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory()); code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	var parsed struct {
		Pins struct {
			CapabilityGap string `json:"capabilityGap"`
		} `json:"pins"`
	}
	if err := json.Unmarshal([]byte(firstJSONLine(t, out.String())), &parsed); err != nil {
		t.Fatalf("the emitted record must be JSON: %v\n%s", err, out.String())
	}
	return parsed.Pins.CapabilityGap
}

// firstJSONLine returns the DecisionRecord line of a run's stdout (the record
// precedes the summary line on stdout).
func firstJSONLine(t *testing.T, raw string) string {
	t.Helper()
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			return line
		}
	}
	t.Fatal("no DecisionRecord line on stdout")
	return ""
}

// TestConformanceCapabilityGapCarriesOnlyMergeResult is S00 Q3's named case:
// a run whose capability report carries N extra absent/unknown entries emits a
// pins.capabilityGap byte-identical to a run with none — the record's gap
// string is merge-result-only, exactly what the frozen schema's pins shape
// models.
func TestConformanceCapabilityGapCarriesOnlyMergeResult(t *testing.T) {
	// Both runs share every fixture byte; only the capability report differs.
	// Run A: all-supported EXCEPT merge-result-pinning (the known GitLab gap:
	// plain merge exposes no merge-result digest).
	runA := newFakeGitLab(t)
	reportA := *runA.capReportOverride
	entriesA := entryMapFrom(t, reportA)
	entriesA[forge.CapabilityMergeResultPinning] = forge.AbsentCapabilityEntry("probe: merge_trains_enabled is false")
	runA.capReportOverride = capReport(t, entriesA)

	// Run B: the same report plus N extra absent/unknown capabilities. The
	// record's gap string must not change; the DOCTOR report carries all N.
	runB := newFakeGitLab(t)
	entriesB := entryMapFrom(t, *runB.capReportOverride)
	entriesB[forge.CapabilityMergeResultPinning] = forge.AbsentCapabilityEntry("probe: merge_trains_enabled is false")
	entriesB[forge.CapabilityBlockingReview] = forge.AbsentCapabilityEntry("probe: required reviews is 0")
	entriesB[forge.CapabilityReviewDismissalRestrictions] = forge.AbsentCapabilityEntry("probe: no dismissal restrictions")
	entriesB[forge.CapabilityApprovalResetOnPush] = forge.UnknownCapabilityEntry("unprobed by this fixture")
	entriesB[forge.CapabilityArmingRevokedOnPush] = forge.AbsentCapabilityEntry("probe: stale dismissal disabled")
	runB.capReportOverride = capReport(t, entriesB)

	gapA := recordCapabilityGap(runA, t)
	gapB := recordCapabilityGap(runB, t)
	if gapA == "" {
		t.Fatal("the emitted record must carry pins.capabilityGap when the merge result is not forge-pinned")
	}
	if gapA != gapB {
		t.Fatalf("N extra absent/unknown capabilities changed the record's capabilityGap:\n  A=%q\n  B=%q — the record surface models exactly ONE capability (merge-result pinning); the rest live in the doctor report (S00 Q3)", gapA, gapB)
	}

	// The doctor surface carries every entry: N entries in, N entries out, with
	// the states the adapter reported — the multi-capability report's home.
	doctor := DoctorFromForgeProbe(forge.PreconditionFromReport(*runB.capReportOverride), *runB.capReportOverride)
	if got := len(doctor.Capabilities.Forge); got != len(forge.AllCapabilities()) {
		t.Fatalf("doctor's typed capability report carries %d entries, want the closed enum's %d", got, len(forge.AllCapabilities()))
	}
	for _, view := range doctor.Capabilities.Forge {
		if view.Capability == string(forge.CapabilityBlockingReview) && view.State != string(forge.CapabilityAbsent) {
			t.Errorf("doctor must carry the extra absent capability verbatim, got %+v", view)
		}
	}
}

// TestConformanceCapabilityGapStillRecordsMergeResult is the mandatory
// positive control: the merge-result gap itself IS recorded (never an empty
// string), so the byte-identical test above cannot be satisfied by wiping the
// field.
func TestConformanceCapabilityGapStillRecordsMergeResult(t *testing.T) {
	runA := newFakeGitLab(t)
	gapA := recordCapabilityGap(runA, t)
	if !strings.HasPrefix(gapA, "gitlab plain-merge exposes no merge-result digest") {
		t.Fatalf("the merge-result gap must still be recorded, got %q", gapA)
	}
}

// entryMapFrom copies a report into an entry map for mutation.
func entryMapFrom(t *testing.T, report forge.CapabilityReport) map[forge.Capability]forge.CapabilityEntry {
	t.Helper()
	entries := make(map[forge.Capability]forge.CapabilityEntry, len(forge.AllCapabilities()))
	for _, c := range forge.AllCapabilities() {
		entries[c] = forge.CapabilityEntry{State: report.State(c), Reason: report.Reason(c)}
	}
	return entries
}

// capReport builds a strict report from an entry map.
func capReport(t *testing.T, entries map[forge.Capability]forge.CapabilityEntry) *forge.CapabilityReport {
	t.Helper()
	report, err := forge.NewCapabilityReport(entries)
	if err != nil {
		t.Fatalf("capability report: %v", err)
	}
	return &report
}
