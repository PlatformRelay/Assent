package forge

import (
	"strings"
	"testing"
)

// precondition_test.go pins the neutral capability model's arming behaviour
// (E10-S04 / REQ-E10-S04-01/02): the enum is closed, `unknown` refuses arming
// identically to `absent` but reads differently in prose, and the all-supported
// report arms — the mandatory positive control that keeps every refusal test
// from being vacuously satisfied by a consult function that never arms.

func supportedReport() (CapabilityReport, error) {
	return NewCapabilityReport(map[Capability]CapabilityEntry{
		CapabilityResolvableThreads:           SupportedCapabilityEntry("licensed by p3e5-* cases"),
		CapabilityThreadsBlockMerge:           SupportedCapabilityEntry("probe true"),
		CapabilityBlockingReview:              SupportedCapabilityEntry("required reviews >= 1"),
		CapabilityReviewDismissalRestrictions: SupportedCapabilityEntry("dismissal restricted"),
		CapabilitySHAGuardedMerge:             SupportedCapabilityEntry("sha-guard-* cases"),
		CapabilityDeferredMergeArming:         SupportedCapabilityEntry("auto-merge allowed"),
		CapabilityArmingRevokedOnPush:         SupportedCapabilityEntry("stale dismissal + required check"),
		CapabilityMergeResultPinning:          SupportedCapabilityEntry("merge ref readable"),
		CapabilityEligibleApprovalEvidence:    SupportedCapabilityEntry("approval rules"),
		CapabilityApprovalResetOnPush:         SupportedCapabilityEntry("stale dismissal on"),
		CapabilityProtectedPipelineSource:     SupportedCapabilityEntry("workflows from target branch"),
	})
}

func mustReport(t *testing.T, entries map[Capability]CapabilityEntry) CapabilityReport {
	t.Helper()
	report, err := NewCapabilityReport(entries)
	if err != nil {
		t.Fatalf("build report: %v", err)
	}
	return report
}

func entryMapOf(report CapabilityReport) map[Capability]CapabilityEntry {
	entries := make(map[Capability]CapabilityEntry, len(AllCapabilities()))
	for _, c := range AllCapabilities() {
		entries[c] = CapabilityEntry{State: report.State(c), Reason: report.Reason(c)}
	}
	return entries
}

func hasRefusal(probe PreconditionProbe, code PreconditionRefusalCode) bool {
	for _, r := range probe.Refusals {
		if r.Code == code {
			return true
		}
	}
	return false
}

// The arming consultation set, stated explicitly (S00 Q2's scope rule): the
// three gates precondition has always consulted, and nothing else.
var armingSet = []Capability{
	CapabilityProtectedPipelineSource,
	CapabilityThreadsBlockMerge,
	CapabilityEligibleApprovalEvidence,
}

// TestPreconditionFromReportAllSupportedArms is the mandatory POSITIVE CONTROL
// (REQ-E10-S04-02): the all-capabilities-supported report arms. Without it, a
// consult function that never arms would satisfy every refusal test vacuously.
func TestPreconditionFromReportAllSupportedArms(t *testing.T) {
	report, err := supportedReport()
	if err != nil {
		t.Fatal(err)
	}
	probe := PreconditionFromReport(report)

	if !probe.ArmEligible {
		t.Fatalf("all-capabilities-supported must arm; refusals=%+v", probe.Refusals)
	}
	if !probe.AutoMergeEligible {
		t.Error("AutoMergeEligible must be true when eligible approval evidence is supported")
	}
	if !probe.ProtectedConfigVerified {
		t.Error("ProtectedConfigVerified must be true when protected-pipeline-source is supported")
	}
	if len(probe.Refusals) != 0 {
		t.Errorf("eligible probe must carry no refusals; got %+v", probe.Refusals)
	}
	if len(probe.CapabilityGaps) != 0 {
		t.Errorf("eligible probe must carry no capability gaps; got %+v", probe.CapabilityGaps)
	}
	if probe.DuplicatePrevention != DuplicatePreventionBestEffort {
		t.Errorf("DuplicatePrevention = %q, want safe default %q",
			probe.DuplicatePrevention, DuplicatePreventionBestEffort)
	}
}

// TestUnknownDoesNotArm is REQ-E10-S04-02's table: for every consultation
// point, `absent` and `unknown` produce the identical non-arming outcome, with
// a DISTINGUISHABLE detail string for `unknown`.
func TestUnknownDoesNotArm(t *testing.T) {
	for _, state := range []CapabilityState{CapabilityAbsent, CapabilityUnknown} {
		state := state
		for _, consulted := range armingSet {
			t.Run(string(consulted)+"/"+string(state), func(t *testing.T) {
				report, err := supportedReport()
				if err != nil {
					t.Fatal(err)
				}
				entries := entryMapOf(report)
				entries[consulted] = CapabilityEntry{State: state, Reason: "why it is " + string(state)}
				report = mustReport(t, entries)

				probe := PreconditionFromReport(report)
				if probe.ArmEligible {
					t.Fatalf("capability %q state %q must refuse arming; refusals=%+v", consulted, state, probe.Refusals)
				}
				if state == CapabilityUnknown {
					if !strings.Contains(anyRefusalDetail(probe), "UNPROBED/UNKNOWN") {
						t.Errorf("an unknown capability must be distinguishable in prose, refusals=%+v", probe.Refusals)
					}
				}
			})
		}
	}
}

func anyRefusalDetail(probe PreconditionProbe) string {
	joined := ""
	for _, r := range probe.Refusals {
		joined += r.Detail + "\n"
	}
	return joined
}

// The consultation-point negative controls: each consulted capability at
// `absent` refuses with its OWN typed code (the C17/C3/C6-C7 precedent).
func TestPreconditionFromReportRefusalCodes(t *testing.T) {
	base, err := supportedReport()
	if err != nil {
		t.Fatal(err)
	}

	entries := entryMapOf(base)
	entries[CapabilityProtectedPipelineSource] = AbsentCapabilityEntry("probe false")
	probe := PreconditionFromReport(mustReport(t, entries))
	if probe.ArmEligible || !hasRefusal(probe, RefusalInsecureTopology) {
		t.Errorf("protected-pipeline-source not proven must refuse with %q; got %+v",
			RefusalInsecureTopology, probe.Refusals)
	}
	if probe.ProtectedConfigVerified {
		t.Error("ProtectedConfigVerified must be false when the capability is not proven")
	}

	entries = entryMapOf(base)
	entries[CapabilityThreadsBlockMerge] = AbsentCapabilityEntry("probe false")
	probe = PreconditionFromReport(mustReport(t, entries))
	if probe.ArmEligible || !hasRefusal(probe, RefusalDiscussionsGateMissing) {
		t.Errorf("threads not blocking merge must refuse with %q; got %+v",
			RefusalDiscussionsGateMissing, probe.Refusals)
	}

	entries = entryMapOf(base)
	entries[CapabilityEligibleApprovalEvidence] = CapabilityEntry{State: CapabilityUnknown, Reason: "unprobed eligibility"}
	probe = PreconditionFromReport(mustReport(t, entries))
	if probe.ArmEligible || probe.AutoMergeEligible {
		t.Fatal("unprovable eligibility must refuse arming (require-review unsatisfiable)")
	}
	if !hasRefusal(probe, RefusalTierCapabilityGap) {
		t.Errorf("must refuse with %q; got %+v", RefusalTierCapabilityGap, probe.Refusals)
	}
	if len(probe.CapabilityGaps) != 1 || probe.CapabilityGaps[0] != GapFreeTierRequireReview {
		t.Errorf("CapabilityGaps = %v, want [%q]", probe.CapabilityGaps, GapFreeTierRequireReview)
	}
}

// TestCapabilityEnumClosed is REQ-E10-S04-01: exactly eleven named flags, and
// decoding an unknown capability name is an error, not a skip.
func TestCapabilityEnumClosed(t *testing.T) {
	if got := len(AllCapabilities()); got != 11 {
		t.Fatalf("the capability enum carries %d flags, want the dossier §4 eleven", got)
	}
	if _, err := ParseCapability("resolvable-threads"); err != nil {
		t.Fatalf("a known capability must decode: %v", err)
	}
	if _, err := ParseCapability("merge-trains"); err == nil {
		t.Fatal("an unknown capability name must be an error, not a skip")
	}
	if _, err := NewCapabilityReport(map[Capability]CapabilityEntry{
		"made-up-capability": SupportedCapabilityEntry("x"),
	}); err == nil {
		t.Fatal("a report naming an unknown capability must fail to build")
	}
}

// TestCapabilityReportStrictDecode: a report missing an entry is rejected — an
// adapter cannot dodge an answer by omission — and a zero-value entry
// normalizes to unknown, never to a silent supported.
func TestCapabilityReportStrictDecode(t *testing.T) {
	if _, err := NewCapabilityReport(map[Capability]CapabilityEntry{}); err == nil {
		t.Fatal("an empty report must be rejected — every enum member needs an answer")
	}
	partial := map[Capability]CapabilityEntry{CapabilityResolvableThreads: SupportedCapabilityEntry("x")}
	if _, err := NewCapabilityReport(partial); err == nil {
		t.Fatal("a partial report must be rejected — ten missing entries are ten unanswered questions")
	}
	// A zero-value entry (no state) inside a COMPLETE report normalizes to
	// unknown, never to a silent supported.
	entries := map[Capability]CapabilityEntry{}
	for _, c := range AllCapabilities() {
		entries[c] = SupportedCapabilityEntry("fixture probe")
	}
	entries[CapabilityProtectedPipelineSource] = CapabilityEntry{}
	report, err := NewCapabilityReport(entries)
	if err != nil {
		t.Fatalf("capability report: %v", err)
	}
	if report.State(CapabilityProtectedPipelineSource) != CapabilityUnknown {
		t.Fatal("an entry with no state must normalize to unknown — never silently supported")
	}
}
