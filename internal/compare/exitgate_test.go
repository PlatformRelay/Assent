package compare_test

// exitgate_test.go is the P5-PCS-S09 autonomous exit gate (REQ-PCS-S09-01..03):
// hack/compare/exitgate_test.sh runs full suite + E6 seed dir + schemas/ drift guard;
// backlog/later-phases mark PCS authoritative; D-057 deferred scope closed via D-118.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	exitgateScript   = "../../hack/compare/exitgate_test.sh"
	taskfilePath     = "../../Taskfile.yml"
	backlogPath      = "../../openspec/specs/backlog.md"
	laterPhasesPath  = "../../openspec/specs/later-phases.md"
	decisionsPath    = "../../docs/decisions/decisions.md"
	e6SeedFixtureDir = "../../examples/comparison/e6-seed"
)

// TestPCSExitGateScriptPresent is REQ-PCS-S09-01: the exit gate harness exists and is wired in task check.
func TestPCSExitGateScriptPresent(t *testing.T) {
	raw, err := os.ReadFile(exitgateScript) //nolint:gosec // fixed in-repo path.
	if err != nil {
		t.Fatalf("read exit gate script: %v", err)
	}
	body := string(raw)
	for _, needle := range []string{
		"promotion-gates",
		"e6-seed",
		"git diff schemas/",
		"bounded-auto-merge-widening=PASS",
		"verdict=PASS",
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("exitgate_test.sh missing %q (REQ-PCS-S09-01)", needle)
		}
	}

	taskRaw, err := os.ReadFile(taskfilePath) //nolint:gosec // fixed in-repo path.
	if err != nil {
		t.Fatalf("read Taskfile: %v", err)
	}
	taskBody := string(taskRaw)
	if !strings.Contains(taskBody, "compare-exitgate-test") {
		t.Fatal("Taskfile.yml must define compare-exitgate-test (REQ-PCS-S09-01)")
	}
	if !strings.Contains(taskBody, "hack/compare/exitgate_test.sh") {
		t.Fatal("Taskfile check must invoke hack/compare/exitgate_test.sh (REQ-PCS-S09-01)")
	}
}

// TestPCSExitGateE6SeedFixture is REQ-PCS-S09-01: committed single-dir seed layout for E6-S09 regression.
func TestPCSExitGateE6SeedFixture(t *testing.T) {
	for _, name := range []string{"bundle.json", "binding.yaml", "baseline.yaml", "candidate.yaml"} {
		path := filepath.Join(e6SeedFixtureDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("E6 seed fixture missing %s: %v", name, err)
		}
	}
}

// TestPCSExitGateBacklogAuthoritative is REQ-PCS-S09-03: backlog marks PCS closed and cites D-057/D-118.
func TestPCSExitGateBacklogAuthoritative(t *testing.T) {
	raw, err := os.ReadFile(backlogPath) //nolint:gosec // fixed in-repo path.
	if err != nil {
		t.Fatalf("read backlog: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "p5-pcs-policy-comparison/spec.md") {
		t.Fatal("backlog must reference p5-pcs-policy-comparison/spec.md (REQ-PCS-S09-03)")
	}
	if !strings.Contains(body, "PCS **AUTONOMOUS COMPLETE**") && !strings.Contains(body, "**PCS AUTONOMOUS COMPLETE**") {
		t.Fatal("backlog must mark PCS AUTONOMOUS COMPLETE (REQ-PCS-S09-03)")
	}
	if !strings.Contains(body, "D-057") || !strings.Contains(body, "D-118") {
		t.Fatal("backlog must cite D-057 closed and D-118 (REQ-PCS-S09-03)")
	}
}

// TestPCSExitGateLaterPhasesAuthoritative is REQ-PCS-S09-03: later-phases marks PCS closed.
func TestPCSExitGateLaterPhasesAuthoritative(t *testing.T) {
	raw, err := os.ReadFile(laterPhasesPath) //nolint:gosec // fixed in-repo path.
	if err != nil {
		t.Fatalf("read later-phases: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "p5-pcs-policy-comparison/spec.md") {
		t.Fatal("later-phases must reference p5-pcs-policy-comparison/spec.md (REQ-PCS-S09-03)")
	}
	if !strings.Contains(body, "PCS") || !strings.Contains(body, "AUTONOMOUS COMPLETE") {
		t.Fatal("later-phases must mark PCS AUTONOMOUS COMPLETE (REQ-PCS-S09-03)")
	}
	if !strings.Contains(body, "D-118") {
		t.Fatal("later-phases must cite D-118 (REQ-PCS-S09-03)")
	}
}

// TestPCSExitGateD057CrossRef is REQ-PCS-S09-03: decisions cross-reference closes D-057 deferred scope.
func TestPCSExitGateD057CrossRef(t *testing.T) {
	raw, err := os.ReadFile(decisionsPath) //nolint:gosec // fixed in-repo path.
	if err != nil {
		t.Fatalf("read decisions: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "D-118") {
		t.Fatal("decisions.md must contain D-118 (REQ-PCS-S09-03)")
	}
	if !strings.Contains(body, "D-057") {
		t.Fatal("decisions.md must still reference D-057 (REQ-PCS-S09-03 cross-ref)")
	}
	if !strings.Contains(body, "deferred scope") || !strings.Contains(strings.ToLower(body), "closed") {
		t.Fatal("decisions must record D-057 deferred scope closed (REQ-PCS-S09-03)")
	}
}
