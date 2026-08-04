package comparison_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/compare"
	"github.com/PlatformRelay/assent/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// corpusGateCoverage maps each promotion gate ID (plus explanation-only) to a
// committed caseId that adversarially exercises it (REQ-PCS-S08-02).
var corpusGateCoverage = map[string]string{
	string(compare.GateZeroMissedDestructive):           "partition-grow-agree",
	string(compare.GateZeroMissedAuthorizationOwnership): "partition-grow-agree",
	string(compare.GateNoUnexpectedObligationRemoval):    "partition-grow-agree",
	string(compare.GateBoundedAutoMergeWidening):         "partition-shrink-widen-accepted",
	string(compare.GateExplicitlyAcceptedDeltas):           "score-threshold-change-accepted",
	"explanation-only": "partition-shrink-wording-only",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Dir(filepath.Dir(wd))
}

func buildAssent(t *testing.T, root string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "assent")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/assent") // #nosec G204 -- test-controlled build
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build assent: %v\n%s", err, out)
	}
	return bin
}

func decodeSuiteDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // fixed corpus tree
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	ext := strings.ToLower(filepath.Ext(path))
	var doc any
	switch ext {
	case ".json":
		doc, err = jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	case ".yaml", ".yml":
		if err = yaml.Unmarshal(raw, &doc); err == nil {
			jsonBytes, mErr := json.Marshal(doc)
			if mErr != nil {
				t.Fatalf("marshal yaml doc: %v", mErr)
			}
			doc, err = jsonschema.UnmarshalJSON(strings.NewReader(string(jsonBytes)))
		}
	default:
		t.Fatalf("unsupported suite ext %q", ext)
	}
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("%s: want object document", path)
	}
	return obj
}

// REQ-PCS-S08-01: every examples/comparison/**/suite.yaml validates against ComparisonSuiteSchema.
func TestComparisonSuitesValidate(t *testing.T) {
	root := repoRoot(t)
	corpusRoot := filepath.Join(root, "examples", "comparison")
	var checked int
	err := filepath.WalkDir(corpusRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base != "suite.yaml" && base != "suite.yml" && base != "suite.json" {
			return nil
		}
		doc := decodeSuiteDoc(t, path)
		if err := schemas.ComparisonSuiteSchema.Validate(doc); err != nil {
			t.Errorf("%s: ComparisonSuiteSchema: %v", path, err)
			return nil
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walk corpus: %v", err)
	}
	if checked == 0 {
		t.Fatal("no suite.yaml found under examples/comparison/**")
	}
}

// REQ-PCS-S08-02: corpus covers all five promotion gate IDs (plus explanation-only).
func TestCorpusCoversAllGates(t *testing.T) {
	root := repoRoot(t)
	corpusRoot := filepath.Join(root, "examples", "comparison")

	foundCases := make(map[string]struct{})
	err := filepath.WalkDir(corpusRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		base := filepath.Base(path)
		if base != "suite.yaml" && base != "suite.yml" && base != "suite.json" {
			return nil
		}
		doc := decodeSuiteDoc(t, path)
		spec, _ := doc["spec"].(map[string]any)
		if spec == nil {
			return nil
		}
		cases, _ := spec["cases"].([]any)
		for _, c := range cases {
			cm, _ := c.(map[string]any)
			if cm == nil {
				continue
			}
			id, _ := cm["caseId"].(string)
			if id != "" {
				foundCases[id] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk corpus: %v", err)
	}

	for gate, caseID := range corpusGateCoverage {
		if _, ok := foundCases[caseID]; !ok {
			t.Errorf("gate %q maps to missing caseId %q", gate, caseID)
		}
	}
}

// REQ-PCS-S08-03 (positive): committed corpus runs green under assent compare --suite.
func TestCompareCorpusRunsGreen(t *testing.T) {
	root := repoRoot(t)
	bin := buildAssent(t, root)
	suites := []string{
		filepath.Join(root, "examples", "comparison", "promotion-gates"),
		filepath.Join(root, "examples", "comparison", "wording-only"),
	}
	for _, suite := range suites {
		t.Run(filepath.Base(suite), func(t *testing.T) {
			cmd := exec.Command(bin, "compare", "--suite", suite) // #nosec G204 -- test-built binary
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("assent compare --suite %s: %v\n%s", suite, err, out)
			}
			if !strings.Contains(string(out), "gate bounded-auto-merge-widening=PASS") {
				t.Fatalf("stdout = %q, want bounded-auto-merge-widening=PASS", out)
			}
		})
	}
}

// REQ-PCS-S08-03 (negative): a broken gate fixture must fail compare (exit 4 widening).
func TestCompareCorpusGateFailurePath(t *testing.T) {
	root := repoRoot(t)
	bin := buildAssent(t, root)

	src := filepath.Join(root, "examples", "comparison", "promotion-gates")
	dir := t.TempDir()
	copyDir(t, src, dir)

	suitePath := filepath.Join(dir, "suite.yaml")
	doc := decodeSuiteDoc(t, suitePath)
	spec := doc["spec"].(map[string]any)
	spec["acceptedDeltas"] = []any{}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal suite: %v", err)
	}
	if err := os.WriteFile(suitePath, raw, 0o600); err != nil {
		t.Fatalf("write suite: %v", err)
	}

	cmd := exec.Command(bin, "compare", "--suite", dir) // #nosec G204 -- test-built binary
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for broken widening fixture, stdout=%q", out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != compare.ExitCodeForGate(compare.GateBoundedAutoMergeWidening) {
		t.Fatalf("compare broken fixture: err=%v stdout=%q, want exit %d", err, out, compare.ExitCodeForGate(compare.GateBoundedAutoMergeWidening))
	}
	if !strings.Contains(string(out), "bounded-auto-merge-widening=FAIL") {
		t.Fatalf("stdout = %q, want bounded-auto-merge-widening=FAIL", out)
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(path) //nolint:gosec // test fixture copy
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600) // #nosec G703 -- test temp dir copy from fixed corpus tree
	})
	if err != nil {
		t.Fatalf("copyDir %s -> %s: %v", src, dst, err)
	}
}