package conformance

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// suite_test.go is the ENTRY POINT layer: it runs the shared suite against both
// built-in backends.
//
// The five exported test names are unchanged from before extraction, deliberately.
// E10-S01's definition of done forbids renaming a case, and `exitgate_test.go`'s
// `l1CatalogTests` pins these exact names against `catalog.yaml` — the pattern
// recorded in this repo's own lessons: pin named tests and let `go test` carry the
// semantics, rather than asserting a fix's shape from source text. Renaming them
// would have silently unhooked the E7 exit gate.
//
// What changed is what they DO: each now runs its case against every backend,
// where before the fake and GitLab variants were hand-written subtests with
// different assertions and no one comparing them.

type namedBackend struct {
	name string
	f    Factory
}

// backends is the set every case runs against. Adding an adapter here runs the
// entire suite against it — that is the property E10-S01 exists to create.
func backends() []namedBackend {
	return []namedBackend{
		{"fake", fakeFactory},
		{"gitlab", gitlabFactory},
	}
}

func runCaseOnAllBackends(t *testing.T, id string) {
	t.Helper()
	var found *Case
	for _, c := range Cases() {
		if c.ID == id {
			found = &c
			break
		}
	}
	if found == nil {
		t.Fatalf("no conformance case with id %q — Cases() and the entry points have diverged", id)
	}
	for _, be := range backends() {
		t.Run(be.name, func(sub *testing.T) { found.Run(tbT{sub}, be.f) })
	}
}

// TestConformanceTargetAdvancedRejected is REQ-E4-S07-01.
func TestConformanceTargetAdvancedRejected(t *testing.T) {
	runCaseOnAllBackends(t, "sha-guard-target-advanced")
}

// TestConformanceSourceMovedRejected is REQ-E4-S07-02.
func TestConformanceSourceMovedRejected(t *testing.T) {
	runCaseOnAllBackends(t, "sha-guard-source-moved")
}

// TestConformanceSourceMovedAndRestored is REQ-REV1-S01-04: the CAS merges once a
// moved head is restored to the pin, which is why the read-pin (not the CAS) is
// what protects the judged bytes.
func TestConformanceSourceMovedAndRestored(t *testing.T) {
	runCaseOnAllBackends(t, "sha-guard-source-moved-and-restored")
}

// TestConformanceRerunIdempotence is REQ-E4-S09-01.
func TestConformanceRerunIdempotence(t *testing.T) {
	runCaseOnAllBackends(t, "p3e5-rerun-idempotence")
}

// TestConformanceDuplicateRepair is REQ-E4-S09-02.
func TestConformanceDuplicateRepair(t *testing.T) {
	runCaseOnAllBackends(t, "p3e5-duplicate-repair")
}

// TestConformanceOwnMarkersRecognisedIdentity is ADR-0021 item 7: the marker
// filter matches the AUTHENTICATED identity, not "any bot".
func TestConformanceOwnMarkersRecognisedIdentity(t *testing.T) {
	runCaseOnAllBackends(t, "own-markers-recognised-identity")
}

// TestConformanceUnknownNeverArms is S00 Q2's arming case (REQ-E10-S04-02):
// unknown refuses arming with zero writes, and the mandatory positive control
// (the same backend with arming granted) merges — both live in one case body.
func TestConformanceUnknownNeverArms(t *testing.T) {
	runCaseOnAllBackends(t, "capability-unknown-never-arms")
}

// TestConformanceCapabilityReportExhaustive is S00 Q2's exhaustiveness case
// (REQ-E10-S04-01): every member of the closed enum carries a tri-state and a
// reason on every backend. It is a DIRECT entry test over the two built-in
// backends, deliberately not a Cases() row: the report's shape is a property of
// the PORT's snapshot, which the sabotage fixture does not corrupt, so the
// can-fail gate has no purchase on it — the catalog row carries that
// disposition.
func TestCapabilityReportExhaustiveOnAllBackends(t *testing.T) {
	for _, be := range backends() {
		t.Run(be.name, func(sub *testing.T) {
			caseCapabilityReportExhaustive(tbT{sub}, be.f)
		})
	}
}

// TestCapabilityAllSupportedArmsForgeLevel is the all-supported positive
// control pinned at the FORGE package (REQ-E10-S04-02): the all-supported
// report arms, so the never-arms refusal above cannot be satisfied by a
// consult function that refuses everything.
func TestAllSupportedDoesArmForgeLevel(t *testing.T) {
	entries := map[forge.Capability]forge.CapabilityEntry{}
	for _, c := range forge.AllCapabilities() {
		entries[c] = forge.SupportedCapabilityEntry("fixture: probe proven")
	}
	report, err := forge.NewCapabilityReport(entries)
	if err != nil {
		t.Fatalf("capability report: %v", err)
	}
	probe := forge.PreconditionFromReport(report)
	if !probe.ArmEligible {
		t.Fatalf("the all-supported report MUST arm — without this the refusal tests are vacuous; refusals=%+v", probe.Refusals)
	}
}

// TestConformanceForkHeadUnchangedFileNoLifecycle is S00 Q1's case: a fork MR
// whose governed file is unchanged yields NO lifecycle event — the
// fabricated-DELETE defect the two-argument port would mint on every fork.
func TestConformanceForkHeadUnchangedFileNoLifecycle(t *testing.T) {
	runCaseOnAllBackends(t, "fork-head-unchanged-file-no-lifecycle")
}

// TestConformanceForkHeadGenuineDeleteDetected is the mandatory positive
// control: a fork MR that really deletes the governed file still mints
// KindDelete.
func TestConformanceForkHeadGenuineDeleteDetected(t *testing.T) {
	runCaseOnAllBackends(t, "fork-head-genuine-delete-detected")
}

// TestConformanceForbiddenNotAbsent is S00 Q4: a permission-refused read never
// renders as absence.
func TestConformanceForbiddenNotAbsent(t *testing.T) {
	runCaseOnAllBackends(t, "forbidden-never-renders-as-absent")
}

// TestConformanceAbsentFileIsAbsent is the positive control for the case above.
func TestConformanceAbsentFileIsAbsent(t *testing.T) {
	runCaseOnAllBackends(t, "absent-file-still-renders-as-absent")
}

// TestSHAGuardObservesMergeAttempts is REQ-E10-S01-04's named proof: the extracted
//
// It is written as a property over the observation surface rather than a re-run of
// the cases, because the weakening it guards against is not "the case fails" — it
// is "the case stops looking". The two SHA-guard scenarios must disagree about
// MergeAttempts (0 when the pre-check refuses, 1 when the CAS refuses) while
// agreeing that no merge was performed. If someone downgrades the surface to a
// single "did a merge happen" boolean to accommodate a backend, the two scenarios
// collapse to the same observation and this test reds.
func TestSHAGuardObservesMergeAttempts(t *testing.T) {
	for _, be := range backends() {
		t.Run(be.name, func(sub *testing.T) {
			t := tbT{sub}
			preCheck := be.f(t, shaGuardConfig())
			pins := preCheck.Fixture.Pins()
			preCheck.Fixture.MoveTargetHead(movedTarget)
			_, _ = reconcileForObservation(preCheck, pins)

			cas := be.f(t, shaGuardConfig())
			casPins := cas.Fixture.Pins()
			cas.Fixture.DriftSourceHeadAfterRead(movedSource)
			_, _ = reconcileForObservation(cas, casPins)

			if preCheck.Observer.MergeAttempts() != 0 {
				t.Fatalf("target-advanced must not reach MergeCAS, got %d attempt(s)",
					preCheck.Observer.MergeAttempts())
			}
			if cas.Observer.MergeAttempts() != 1 {
				t.Fatalf("source-moved must reach MergeCAS exactly once, got %d",
					cas.Observer.MergeAttempts())
			}
			if preCheck.Observer.MergeAttempts() == cas.Observer.MergeAttempts() {
				t.Fatal("the two SHA-guard scenarios became indistinguishable — " +
					"the observation surface has been downgraded (REQ-E10-S01-04)")
			}
			for _, c := range []struct {
				name string
				o    Observer
			}{{"target-advanced", preCheck.Observer}, {"source-moved", cas.Observer}} {
				if got := c.o.MergesPerformed(); got != 0 {
					t.Fatalf("%s: no merge may be performed, got %d", c.name, got)
				}
			}
		})
	}
}

// TestConformanceSpoofedMarkerIgnored is REQ-E4-S09-03.
func TestConformanceSpoofedMarkerIgnored(t *testing.T) {
	runCaseOnAllBackends(t, "p3e5-spoofed-marker-ignored")
}
