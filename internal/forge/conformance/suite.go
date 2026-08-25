package conformance

import (
	"errors"
	"sync"

	"github.com/PlatformRelay/assent/internal/forge"
)

// suite.go is the importable entry point for the forge conformance suite
// (E10-S01, REQ-E10-S01-01). Before it, the cases were `_test.go` bodies that no
// other package could reach, so a second adapter's only route to conformance was
// to copy them.
//
// A Factory constructs one backend in a known initial state.

// Factory builds one backend under test from a Config. It returns a
// `forge.Forge` and NOT a `forge.RunPort`: the composite port is E10-S02's
// deliverable and S02 is LGTM-gated, so declaring it here would smuggle an
// LGTM-gated core-contract change into an `[autonomous]` lane. S02 depends on
// S01 precisely so the port change lands against an executable suite; the
// substitution is a one-line change to this type when it does.
type Factory func(t TB, cfg Config) Backend

// Case is one catalogued conformance case. ID is not decorative — it is the join
// key to `catalog.yaml`, and TestCatalogMatchesExecutedCases fails when the two
// sets diverge in either direction.
type Case struct {
	ID  string
	Run func(t TB, f Factory)
}

// Cases returns the suite in a fixed order. This slice IS the dispatch list: the
// catalog gate compares the catalog against what running the suite actually
// executes, so deleting an entry here reds that gate. That is the point — a gate
// keyed on test-function NAMES would stay green for a case that had been
// unhooked from the runner, which is a predicate over text standing in for a
// property that is structural.
func Cases() []Case {
	return []Case{
		{ID: "sha-guard-target-advanced", Run: caseSHAGuardTargetAdvanced},
		{ID: "sha-guard-source-moved", Run: caseSHAGuardSourceMoved},
		{ID: "p3e5-rerun-idempotence", Run: caseRerunIdempotence},
		{ID: "p3e5-duplicate-repair", Run: caseDuplicateRepair},
		{ID: "p3e5-spoofed-marker-ignored", Run: caseSpoofedMarkerIgnored},
	}
}

// RunSuite runs every conformance case against the backend the Factory builds
// and returns the IDs it actually dispatched. This is the whole public surface an
// adapter needs: one call, no copying.
//
// The return value is not decoration — it is the DENOMINATOR for
// REQ-E10-S01-02. The catalog gate checks the catalog against OBSERVED
// EXECUTION, never against a hand-maintained list or a scan for test-function
// names, so a case that stops running cannot stay accounted for. Having one
// runner rather than a separate "and also tell me what ran" entry point is
// deliberate: two paths would let the reported set and the executed set drift,
// which is the failure this gate exists to catch.
func RunSuite(t TB, f Factory) []string {
	t.Helper()
	return runCases(t, f, Cases())
}

func runCases(t TB, f Factory, cases []Case) []string {
	t.Helper()
	var (
		mu       sync.Mutex
		executed []string
	)
	for _, c := range cases {
		t.Run(c.ID, func(t TB) {
			// Recorded on ENTRY, not on pass. A case that runs and fails has still
			// been executed; conflating "did not run" with "ran and failed" would
			// let a red suite look like an incomplete one.
			mu.Lock()
			executed = append(executed, c.ID)
			mu.Unlock()
			c.Run(t, f)
		})
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), executed...)
}

// ---- SHA-guard cases (ADR-0015 §2, REQ-E4-S07-01/02) ----

const (
	pinSource   = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pinTarget   = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	pinDigest   = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	movedTarget = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	movedSource = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
)

func approveState(pins forge.DesiredMerge) forge.DesiredReviewState {
	return forge.DesiredReviewState{
		Project: proj,
		MR:      mrIID,
		Approve: true,
		Merge:   &pins,
	}
}

func armedPre(pins forge.DesiredMerge) forge.Preconditions {
	return forge.Preconditions{
		ArmEligible:       true,
		SourceSha:         pins.SourceSha,
		TargetSha:         pins.TargetSha,
		MergeResultDigest: pins.MergeResultDigest,
	}
}

func shaGuardConfig() Config {
	return Config{
		Project:          proj,
		MR:               mrIID,
		BotAuthor:        botID,
		CurrentSourceSHA: pinSource,
		CurrentTargetSHA: pinTarget,
		// Only the fake can honour a literal digest; GitLab synthesises its own
		// from source+target. Factories are free to ignore this and report the
		// truth through Fixture.Pins, which is what the cases actually read.
		CurrentMergeResultDigest: pinDigest,
	}
}

// caseSHAGuardTargetAdvanced is REQ-E4-S07-01: the target branch tip advanced
// after the evaluation pins were taken, so Reconcile refuses with ErrSHAMoved,
// performs no approve write, and — the strengthened part — never reaches
// MergeCAS at all, because the CurrentHeads pre-check fails closed BEFORE any
// forge mutation. The pre-extraction case asserted only that zero merges were
// PERFORMED, which is equally true of a backend that skipped the pre-check and
// let the atomic CAS do the refusing. Those are different guarantees, and only
// the attempt count separates them.
func caseSHAGuardTargetAdvanced(t TB, f Factory) {
	t.Helper()
	// Twice: the guard must be deterministic, not first-run-only.
	for run := 0; run < 2; run++ {
		b := f(t, shaGuardConfig())
		pins := b.Fixture.Pins() // what an evaluation would have recorded
		b.Fixture.MoveTargetHead(movedTarget)

		_, err := forge.Reconcile(b.Port, testClock(), approveState(pins), armedPre(pins))
		if !errors.Is(err, forge.ErrSHAMoved) {
			t.Fatalf("run %d: want ErrSHAMoved, got %v", run, err)
		}
		if got := b.Observer.MergesPerformed(); got != 0 {
			t.Fatalf("run %d: zero merges expected, got %d", run, got)
		}
		if got := b.Observer.MergeAttempts(); got != 0 {
			t.Fatalf("run %d: pre-check must fail closed BEFORE MergeCAS is called, got %d attempt(s)", run, got)
		}
		if got := b.Observer.Approvals(); got != 0 {
			t.Fatalf("run %d: pre-check rejection must record zero approvals, got %d", run, got)
		}
	}
}

// caseSHAGuardSourceMoved is REQ-E4-S07-02: the MR source head moves inside the
// TOCTOU window between the pre-check read and MergeCAS, so the ATOMIC CAS guard
// refuses (409/406 on GitLab, ErrSHAMoved here), zero merges are performed, and
// at most the one dangling approval the window permits survives. MergeAttempts
// must be exactly 1 — this case is only meaningful if the CAS was actually
// REACHED, and "no merge happened" alone cannot distinguish that from a run that
// never got there, which is the same assertion the target-advanced case makes.
func caseSHAGuardSourceMoved(t TB, f Factory) {
	t.Helper()
	for run := 0; run < 2; run++ {
		b := f(t, shaGuardConfig())
		pins := b.Fixture.Pins()
		b.Fixture.DriftSourceHeadAfterRead(movedSource)

		_, err := forge.Reconcile(b.Port, testClock(), approveState(pins), armedPre(pins))
		if !errors.Is(err, forge.ErrSHAMoved) {
			t.Fatalf("run %d: want ErrSHAMoved, got %v", run, err)
		}
		if got := b.Observer.MergesPerformed(); got != 0 {
			t.Fatalf("run %d: zero merges expected, got %d", run, got)
		}
		if got := b.Observer.MergeAttempts(); got != 1 {
			t.Fatalf("run %d: the CAS guard must be REACHED and refuse, want 1 attempt, got %d", run, got)
		}
		if got := b.Observer.Approvals(); got != 1 {
			t.Fatalf("run %d: MergeCAS rejection may leave one dangling approval, got %d", run, got)
		}
	}
}

// reconcileForObservation drives one SHA-guarded reconcile and discards the
// outcome. The SHA-guard cases already assert on the error; this exists for
// REQ-E10-S01-04's observation proof, which asserts on the surface instead.
func reconcileForObservation(b Backend, pins forge.DesiredMerge) (forge.PublicationReceipt, error) {
	return forge.Reconcile(b.Port, testClock(), approveState(pins), armedPre(pins))
}
