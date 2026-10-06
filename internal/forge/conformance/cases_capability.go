package conformance

import (
	"errors"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge"
)

// cases_capability.go holds the capability-model cases S00 minted (Q2's
// arming rows): the report is exhaustive, and unknown never arms — with the
// all-supported positive control carried by the SAME case's armed leg, so the
// refusal cannot be satisfied by an adapter that never arms at all. They
// consult the PORT's capability report (Backend.Port.Snapshot) and the port's
// arming evaluator (forge.PreconditionFromReport) — the arming decision is a
// PORT property, not an adapter's. Both built-in adapters' HONEST v1 reports
// mark protected-pipeline-source unknown (the retired SEC-04 heuristic;
// OQ-33), so both built-in factories refuse arming through the port evaluator.

// capabilityConfig is the Config the capability cases build their backend with.
func capabilityConfig() Config {
	return Config{
		Project:                  proj,
		MR:                       mrIID,
		BotAuthor:                botID,
		CurrentSourceSHA:         pinSource,
		CurrentTargetSHA:         pinTarget,
		CurrentMergeResultDigest: pinDigest,
	}
}

// caseCapabilityReportExhaustive is
// TestConformanceCapabilityReportExhaustive (S00 Q2, epic REQ-E10-S04-01):
// every member of the closed enum carries an entry with a tri-state and a
// non-empty, adapter-supplied reason on every backend. An adapter that does not
// probe a capability reports UNKNOWN — never silently supported (the SEC-04
// shape), and never by omission. It is a direct entry test (not a Cases() row):
// the report's shape is a property of the PORT's snapshot, which the sabotage
// fixture does not corrupt, so the can-fail gate has no purchase on it — its
// catalog row carries that disposition.
func caseCapabilityReportExhaustive(t TB, f Factory) {
	t.Helper()
	b := f(t, capabilityConfig())
	snapshot, err := b.Port.Snapshot(proj, mrIID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	report := snapshot.Capabilities
	for _, c := range forge.AllCapabilities() {
		state := report.State(c)
		if state != forge.CapabilitySupported && state != forge.CapabilityAbsent && state != forge.CapabilityUnknown {
			t.Fatalf("capability %q state %q is outside the tri-state", c, state)
		}
		if strings.TrimSpace(report.Reason(c)) == "" {
			t.Fatalf("capability %q carries no reason — the reason is what doctor renders; every state must say why it is what it is", c)
		}
	}
}

// caseCapabilityUnknownNeverArms is S00 Q2's arming case (epic REQ-E10-S04-02),
// in two legs on the SAME backend:
//
//	leg 1 (the refusal): unknown at a consultation point refuses arming through
//	    the PORT, and the refused reconcile performs zero forge writes;
//	leg 2 (the mandatory positive control): the SAME backend with arming GRANTED
//	    (the injected decision the SHA-guard cases use) merges — merges == 1.
//
// Leg 2 is not optional: without it the refusal leg is satisfiable by an
// adapter that never arms under any conditions — the documented
// tests-that-cannot-fail class.
func caseCapabilityUnknownNeverArms(t TB, f Factory) {
	t.Helper()
	b := f(t, capabilityConfig())
	snapshot, err := b.Port.Snapshot(proj, mrIID)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	report := snapshot.Capabilities
	if got := report.State(forge.CapabilityProtectedPipelineSource); got != forge.CapabilityUnknown {
		t.Fatalf("a v1 adapter that probes no protected-pipeline-source predicate must report UNKNOWN, got %q (reason %q)",
			got, report.Reason(forge.CapabilityProtectedPipelineSource))
	}
	probe := forge.PreconditionFromReport(report)
	if probe.ArmEligible {
		t.Fatalf("an honest report with an unprobed protected-pipeline-source must refuse arming; refusals=%+v", probe.Refusals)
	}
	pins := b.Fixture.Pins()

	// Leg 1: the port's refused arming decision gates the write.
	desired := approveState(pins)
	pre := forge.Preconditions{
		ArmEligible:       probe.ArmEligible,
		SourceSha:         pins.SourceSha,
		TargetSha:         pins.TargetSha,
		MergeResultDigest: pins.MergeResultDigest,
	}
	_, recErr := forge.Reconcile(b.Port, testClock(), desired, pre)
	if !errors.Is(recErr, forge.ErrArmingRefused) {
		t.Fatalf("an unknown capability must refuse arming with forge.ErrArmingRefused, got %v", recErr)
	}
	if got := b.Observer.MergeAttempts() + b.Observer.MergesPerformed() + b.Observer.Approvals(); got != 0 {
		t.Fatalf("a capability-refused run must perform zero forge writes, got %d", got)
	}

	// Leg 2 (positive control): a fresh backend of the same factory with arming
	// GRANTED merges — the consult point, not the adapter, is what refused.
	b2 := f(t, capabilityConfig())
	pins2 := b2.Fixture.Pins()
	if _, err := forge.Reconcile(b2.Port, testClock(), approveState(pins2), armedPre(pins2)); err != nil {
		t.Fatalf("the armed leg must merge (the consult point, not the adapter, is what refused), got %v", err)
	}
	if got := b2.Observer.MergesPerformed(); got != 1 {
		t.Fatalf("the armed leg must merge exactly once, got %d", got)
	}
}
