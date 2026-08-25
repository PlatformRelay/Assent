package conformance

import "testing"

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

// TestConformanceRerunIdempotence is REQ-E4-S09-01.
func TestConformanceRerunIdempotence(t *testing.T) {
	runCaseOnAllBackends(t, "p3e5-rerun-idempotence")
}

// TestConformanceDuplicateRepair is REQ-E4-S09-02.
func TestConformanceDuplicateRepair(t *testing.T) {
	runCaseOnAllBackends(t, "p3e5-duplicate-repair")
}

// TestConformanceSpoofedMarkerIgnored is REQ-E4-S09-03.
func TestConformanceSpoofedMarkerIgnored(t *testing.T) {
	runCaseOnAllBackends(t, "p3e5-spoofed-marker-ignored")
}

// TestSHAGuardObservesMergeAttempts is REQ-E10-S01-04's named proof: the extracted
// SHA-guard cases still observe a merge ATTEMPT COUNT and not merely a returned
// error.
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
