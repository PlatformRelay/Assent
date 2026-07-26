//go:build e2e

// L3 walking-skeleton end-to-end wiring (P4-E1-S09).
//
// This file WIRES the skeleton flow the L3 run (S10) will exercise against the
// Spike-B *testcontainer* GitLab profile — it is deliberately NOT the green run.
// S09's job is that this scaffold COMPILES and VETS clean under
// `go vet -tags e2e ./...` on every PR (REQ-P4-E1-S09-01) so the wiring cannot
// rot, and that it SKIPS (never fails) when no GitLab endpoint is configured, so
// the autonomous PR gate stays green and `task check` never boots a container
// (REQ-P4-E1-S09-02). Booting GitLab and asserting live thread/approval/merge is
// S10 (infra-gated, parked for the operator).
package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// gitlabEndpointEnv is the single env var that arms the L3 e2e run. When it is
// unset (the autonomous session / a plain `task e2e` with no infra), the tests
// skip before touching any live infra or a real forge adapter. S10 sets it to
// the Spike-B testcontainer base URL, e.g. http://localhost:8980 — the default
// external_url that hack/spikes/e2e/boot-testcontainer.sh publishes.
const gitlabEndpointEnv = "ASSENT_E2E_GITLAB"

// Spike-B testcontainer profile references (REQ-P4-E1-S09-01). These are the
// operator entry points S10 will drive; naming them here keeps the wiring
// honest and greppable against hack/spikes/e2e/.
const (
	// bootTestcontainerScript boots GitLab CE as a plain Docker container
	// ("testcontainer" profile — the Spike-B / P2-E2 CI default).
	bootTestcontainerScript = "hack/spikes/e2e/boot-testcontainer.sh"
	// smokeScript is the Spike-B product-surface smoke that exercises the exact
	// forge primitives Reconcile needs (thread → resolve → approve → SHA-pinned
	// merge). S10 asserts the same primitives through the assent binary.
	smokeScript = "hack/spikes/e2e/smoke.sh"
	// testcontainerProfile is the ASSENT_SPIKE_PROFILE value the boot/smoke
	// scripts key on for the containerized (non-kind) GitLab.
	testcontainerProfile = "testcontainer"
)

// seedSampleRelPath is the examples/repos/ seed the skeleton evaluates: a
// one-topic-per-file YAML registry. The L3 MR modifies a single scalar field
// (e.g. partitions / retention_hours) in this document — the modify-only case
// the differ (internal/change) and aggregation (internal/core) are built for.
// smoke.sh already seeds the "topic-registry-smoke" project from this same dir,
// so S10 reuses the identical seed.
var seedSampleRelPath = filepath.Join(
	"examples", "repos", "topic-registry", "topics", "prod", "orders.events.v1.yaml",
)

// repoRoot walks up from this test file (test/e2e/) to the module root so the
// seed and boot-script paths resolve regardless of the test's working dir.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// test/e2e -> repo root is two levels up.
	return filepath.Dir(filepath.Dir(wd))
}

// TestSkeletonE2E sketches the L3 skeleton flow it WOULD run against the Spike-B
// testcontainer GitLab, seeded with one examples/repos/ sample. It SKIPS before
// any live-infra / real-forge-adapter code path when ASSENT_E2E_GITLAB is unset.
//
//	CLI (bin/assent, subprocess) -> differ -> aggregation -> classify -> decision
//	  -> Reconcile (post one resolvable thread OR approve + SHA-pinned merge)
//	  -> emit DecisionRecord.
//
// Only the in-memory fake forge exists today; the real GitLab adapter is a later
// slice (S10+). So the CLI step is black-boxed as a subprocess against the built
// binary rather than by importing internal/* — that is also the most faithful L3
// shape (exercise the real binary against a real forge), and it keeps this file
// decoupled from the engine packages the sibling lanes are still landing.
func TestSkeletonE2E(t *testing.T) {
	endpoint := os.Getenv(gitlabEndpointEnv)
	if endpoint == "" {
		t.Skipf(
			"no GitLab endpoint: set %s to the Spike-B testcontainer base URL "+
				"(boot it with ASSENT_SPIKE_PROFILE=%s %s; default external_url "+
				"http://localhost:8980) — this is the infra-gated S10 green run, "+
				"parked for the operator; the autonomous gate stays green",
			gitlabEndpointEnv, testcontainerProfile, bootTestcontainerScript,
		)
	}

	// --- Everything below is the S10 sketch: it is wired but not yet asserted.
	// It compiles and references the real testcontainer profile + a real
	// examples/repos/ seed, but the green run (booting GitLab, opening an MR,
	// asserting a live thread/approval/merge, replaying the DecisionRecord) is
	// TODO(S10): infra-gated. It only executes once ASSENT_E2E_GITLAB is set.
	root := repoRoot(t)

	seedPath := filepath.Join(root, seedSampleRelPath)
	if _, err := os.Stat(seedPath); err != nil {
		t.Fatalf("seed sample missing: %s: %v", seedPath, err)
	}
	bootScript := filepath.Join(root, bootTestcontainerScript)
	smoke := filepath.Join(root, smokeScript)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// CLI step (subprocess): the pipeline invokes the built `bin/assent` the same
	// way a GitLab CI job would. Referenced here, built by S10 (`task build`);
	// not required to exist at compile time. The one-field YAML change on the
	// seeded MR flows CLI -> differ -> aggregation -> classify -> decision ->
	// Reconcile against the testcontainer endpoint.
	assentBin := filepath.Join(root, "bin", "assent")
	runAssent := exec.CommandContext(ctx, assentBin, "run",
		"--gitlab-endpoint", endpoint,
		"--seed", seedPath,
		"--profile", testcontainerProfile,
	)

	// TODO(P4-E1-S10): boot GitLab via bootScript, seed the sample project (as
	// smoke.sh does), open a one-field-change MR, run the assent binary
	// (runAssent), then assert against the forge API — one resolvable thread OR
	// approve + SHA-pinned merge — and validate the emitted DecisionRecord.
	_ = bootScript
	_ = smoke
	_ = runAssent
	t.Fatalf("L3 green run is S10 (infra-gated) and not implemented in S09; "+
		"endpoint %s was configured but the green-run body is parked", endpoint)
}
