package render_test

// exitgate_test.go is the P5-E8-S14 autonomous exit gate (REQ-E8-S14-01..03):
// verify.yaml + task determinism wire render goldens (-count=2); schemas/ stays
// frozen or D-088 presentation-only; backlog marks E8 authoritative. Safety split
// lives in cmd/assent/render_exitgate_test.go (REQ-E8-S14-02).

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/schemadrift"
)

const (
	verifyWorkflow = "../../.github/workflows/verify.yaml"
	taskfilePath   = "../../Taskfile.yml"
	backlogPath    = "../../openspec/specs/backlog.md"
)

// TestE8ExitGateVerifyWiring is REQ-E8-S14-01: verify.yaml and task determinism
// mirror E7-S04 by double-running render finding-thread + summary goldens.
func TestE8ExitGateVerifyWiring(t *testing.T) {
	for _, f := range []struct {
		path string
		name string
	}{
		{verifyWorkflow, "verify.yaml"},
		{taskfilePath, "Taskfile.yml determinism"},
	} {
		raw, err := os.ReadFile(f.path) //nolint:gosec // fixed in-repo paths.
		if err != nil {
			t.Fatalf("read %s: %v", f.path, err)
		}
		body := string(raw)
		if !strings.Contains(body, "-count=2") {
			t.Errorf("%s missing -count=2 determinism flag (REQ-E8-S14-01)", f.name)
		}
		if !strings.Contains(body, "./internal/render/") {
			t.Errorf("%s missing ./internal/render/ determinism target (REQ-E8-S14-01)", f.name)
		}
		for _, needle := range []string{"TestRenderGoldens", "TestRenderSummaryGolden"} {
			if !strings.Contains(body, needle) {
				t.Errorf("%s missing render golden test %q (REQ-E8-S14-01 / E7-S04 pattern)", f.name, needle)
			}
		}
		if f.name == "verify.yaml" {
			if !regexp.MustCompile(`(?i)determinism`).MatchString(body) {
				t.Error("verify.yaml missing determinism gate step name (E7-S04 superset)")
			}
		}
	}
}

// TestE8ExitGateSchemasFrozen is E8 epic DoD: no schema drift beyond E8 D-088
// presentation block in config.schema.json.
func TestE8ExitGateSchemasFrozen(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	if err := schemadrift.CheckGitFrozenOrD088PresentationOnly(repoRoot); err != nil {
		t.Fatalf("schema drift: %v", err)
	}
}

// TestE8ExitGateBacklogAuthoritative is REQ-E8-S14-03: backlog indexes the E8 spec.
func TestE8ExitGateBacklogAuthoritative(t *testing.T) {
	raw, err := os.ReadFile(backlogPath) //nolint:gosec // fixed in-repo path.
	if err != nil {
		t.Fatalf("read backlog: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "p5-e8-renderer/spec.md") {
		t.Fatal("backlog must reference p5-e8-renderer/spec.md (REQ-E8-S14-03)")
	}
	if !strings.Contains(body, "E8 **AUTONOMOUS COMPLETE**") && !strings.Contains(body, "**E8 AUTONOMOUS COMPLETE**") {
		t.Fatal("backlog must mark E8 AUTONOMOUS COMPLETE (REQ-E8-S14-03)")
	}
}
