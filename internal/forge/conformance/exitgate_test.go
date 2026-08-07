package conformance

// exitgate_test.go is the P5-E7-S08 autonomous exit gate (REQ-E7-S08-01..03):
// verify.yaml wires determinism + sanitization + e2e-vet (D-086); the forge-neutral
// catalog indexes every E4 + E7 L1 case with a green test; schemas/ stays frozen.
// S06 (kind lab) and S07 (live L3) are infra-gated optional per D-081/D-083.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/schemadrift"
)

const (
	verifyWorkflow = "../../../.github/workflows/verify.yaml"
	catalogPath    = "catalog.yaml"
)

// l1CatalogTests is the authoritative L1 surface the exit gate pins (E4-S07..S09,
// E4-S08, E4-S05 catalog-index, E7-S03 new cases). Each row must appear in catalog.yaml
// at level L1 with a matching test name.
var l1CatalogTests = []string{
	"TestConformanceTargetAdvancedRejected",
	"TestConformanceSourceMovedRejected",
	"TestConformanceRerunIdempotence",
	"TestConformanceDuplicateRepair",
	"TestConformanceSpoofedMarkerIgnored",
	"TestRunAssentPolicySelfModificationBlocks",
	"TestRunPolicyFromTargetRefOnly",
	"TestDoctorForgeInsecureCITopology",
	"TestRunForkContextAdvisoryOnly",
	"TestRunExpiredFactBlocksArming",
	// AUD-S01 (ADR-0020, D-119) — REQUIRED changed-file-completeness cases.
	"TestRunEnumerationIncompleteNeverApproves",
	"TestFoldSnapshotPathsIncompleteEnumeration",
	"TestChangedFiles404IsError",
}

type catalogDoc struct {
	Cases []struct {
		ID      string `yaml:"id"`
		Level   string `yaml:"level"`
		Req     string `yaml:"req"`
		Test    string `yaml:"test"`
		Package string `yaml:"package"`
		Forge   string `yaml:"forge"`
	} `yaml:"cases"`
}

// TestE7ExitGateVerifyWiring is REQ-E7-S08-01: verify.yaml (CI superset, D-086) enforces
// S04 determinism + S05 sanitization + P4 e2e-vet without extending task check.
func TestE7ExitGateVerifyWiring(t *testing.T) {
	raw, err := os.ReadFile(verifyWorkflow) //nolint:gosec // fixed in-repo workflow path.
	if err != nil {
		t.Fatalf("read verify workflow: %v", err)
	}
	body := string(raw)
	for _, check := range []struct {
		name  string
		match func(string) bool
	}{
		{"determinism gate step", func(s string) bool {
			return regexp.MustCompile(`(?i)determinism`).MatchString(s)
		}},
		{"sanitization check", func(s string) bool {
			return strings.Contains(s, "check-sanitization")
		}},
		{"e2e compile vet", func(s string) bool {
			return strings.Contains(s, "tags e2e")
		}},
	} {
		if !check.match(body) {
			t.Errorf("verify.yaml missing %s (REQ-E7-S08-01 / D-086)", check.name)
		}
	}
}

// TestE7ExitGateCatalogL1Complete is REQ-E7-S08-02: catalog.yaml lists every E4 + E7
// L1 case with req/test/package fields; L3 and github-deferred rows are excluded.
func TestE7ExitGateCatalogL1Complete(t *testing.T) {
	raw, err := os.ReadFile(catalogPath) //nolint:gosec // co-located catalog fixture.
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var doc catalogDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse catalog: %v", err)
	}
	byTest := map[string]struct{}{}
	var l1Count int
	for _, c := range doc.Cases {
		if c.Level != "L1" {
			continue
		}
		l1Count++
		if c.Test == "" || c.Req == "" || c.Package == "" {
			t.Errorf("L1 case %q missing test/req/package", c.ID)
			continue
		}
		byTest[c.Test] = struct{}{}
	}
	if l1Count < len(l1CatalogTests) {
		t.Errorf("catalog L1 rows = %d, want >= %d", l1Count, len(l1CatalogTests))
	}
	for _, name := range l1CatalogTests {
		if _, ok := byTest[name]; !ok {
			t.Errorf("catalog missing L1 row for %s", name)
		}
	}
}

// TestE7ExitGateSchemasFrozen is E7 epic DoD: no schema drift beyond E8 D-088
// presentation block in config.schema.json.
func TestE7ExitGateSchemasFrozen(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	if err := schemadrift.CheckGitFrozenOrD088PresentationOnly(repoRoot); err != nil {
		t.Fatalf("schema drift: %v", err)
	}
}
