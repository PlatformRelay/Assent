package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/compare"
)

const suitePromotionGatesJSON = `[
  {"gateId": "zero-missed-destructive", "failOnKinds": ["destructive-or-authorization-intervention-missed"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
  {"gateId": "zero-missed-authorization-ownership", "failOnKinds": ["destructive-or-authorization-intervention-missed"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
  {"gateId": "no-unexpected-obligation-removal", "failOnKinds": ["subject-or-obligation-uncovered"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
  {"gateId": "bounded-auto-merge-widening", "failOnKinds": ["newly-auto-mergeable"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
  {"gateId": "explicitly-accepted-deltas", "failOnKinds": ["stricter-intervention-added", "score-threshold-change"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"}
]`

// suiteAgreeBundleJSON is a ReplayBundle whose change grows partitions (6→12) so
// identical strict policies agree with no delta.
const suiteAgreeBundleJSON = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "ReplayBundle",
  "pins": {
    "toolVersion": "0.0.0-test",
    "toolDigest": "sha256:aaaa",
    "policySha": "sha256:bbbb",
    "sourceSha": "cccc",
    "targetSha": "dddd",
    "mergeResultDigest": "sha256:eeee",
    "factsResolvedAt": {}
  },
  "evaluationInput": {
    "apiVersion": "assent.dev/v1alpha1",
    "kind": "EvaluationInput",
    "changeSet": {"changes": [
      {"subject": "topic-registry:orders.events.v1", "file": "topics/prod/orders.events.v1.yaml", "path": "/partitions", "kind": "modify", "old": 6, "new": 12}
    ]},
    "facts": {},
    "mr": {"author": "alice", "sourceBranch": "topic/grow", "targetBranch": "main"},
    "require": ["non-destructive"]
  }
}`

func writeSuiteDir(t *testing.T, suiteJSON string, cases map[string]string, baseline, candidate string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("suite.json", suiteJSON)
	write("binding.yaml", compareBindingYAML)
	write("baseline.yaml", baseline)
	write("candidate.yaml", candidate)
	for caseID, bundle := range cases {
		write(filepath.Join("cases", caseID, "bundle.json"), bundle)
	}
	return dir
}

func mkSuiteJSON(cases []struct{ id, digest string }) string {
	body := `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "PolicyComparisonSuite",
  "metadata": {"name": "pcs-s07-fixture", "version": "1"},
  "spec": {
    "cases": [`
	for i, c := range cases {
		if i > 0 {
			body += ","
		}
		body += `{"caseId": "` + c.id + `", "replayBundleDigest": "` + c.digest + `"}`
	}
	body += `],
    "promotionGates": ` + suitePromotionGatesJSON + `,
    "acceptedDeltas": []
  }
}`
	return body
}

func suiteDigest(t *testing.T, bundle string) string {
	t.Helper()
	d, err := compare.ReplayBundleDigest([]byte(bundle))
	if err != nil {
		t.Fatalf("ReplayBundleDigest: %v", err)
	}
	return d
}

// REQ-PCS-S07-01: suite mode exits 0 when all gates pass.
func TestCompareSuiteAllPass(t *testing.T) {
	agreeDigest := suiteDigest(t, suiteAgreeBundleJSON)
	dir := writeSuiteDir(t,
		mkSuiteJSON([]struct{ id, digest string }{{"partition-grow-agree", agreeDigest}}),
		map[string]string{"partition-grow-agree": suiteAgreeBundleJSON},
		mergePolicyYAML("prod-strict-6", "new >= old", "must not shrink"),
		mergePolicyYAML("prod-strict-7", "new >= old", "must not shrink"),
	)
	var out, errb bytes.Buffer
	code := runCompare([]string{"--suite", dir}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (all gates pass); stdout=%q stderr=%q", code, out.String(), errb.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("bounded-auto-merge-widening=PASS")) {
		t.Errorf("stdout = %q, want gate results listing bounded-auto-merge-widening=PASS", out.String())
	}
}

// REQ-PCS-S07-02: bounded-auto-merge-widening failure exits 4 (ADR-0018).
func TestCompareSuiteExitBoundedWidening(t *testing.T) {
	widenDigest := suiteDigest(t, compareBundleJSON)
	dir := writeSuiteDir(t,
		mkSuiteJSON([]struct{ id, digest string }{{"partition-shrink-widen", widenDigest}}),
		map[string]string{"partition-shrink-widen": compareBundleJSON},
		mergePolicyYAML("prod-strict-6", "new >= old", "must not shrink"),
		mergePolicyYAML("prod-strict-7", "true", "must not shrink"),
	)
	var out, errb bytes.Buffer
	code := runCompare([]string{"--suite", dir}, &out, &errb)
	if code != 4 {
		t.Fatalf("exit code = %d, want 4 (bounded-auto-merge-widening); stdout=%q stderr=%q", code, out.String(), errb.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("bounded-auto-merge-widening=FAIL")) {
		t.Errorf("stdout = %q, want bounded-auto-merge-widening=FAIL", out.String())
	}
}

// REQ-PCS-S07-03: fail-closed classification exits 6, not a gate code.
func TestCompareSuiteExitUnclassified(t *testing.T) {
	widenDigest := suiteDigest(t, compareBundleJSON)
	dir := writeSuiteDir(t,
		mkSuiteJSON([]struct{ id, digest string }{{"block-to-review", widenDigest}}),
		map[string]string{"block-to-review": compareBundleJSON},
		mergePolicyYAML("prod-6", "new >= old", "must not shrink"),
		mergePolicyYAMLEffect("prod-7", "new >= old", "must not shrink", "require-review"),
	)
	var out, errb bytes.Buffer
	code := runCompare([]string{"--suite", dir}, &out, &errb)
	if code != compare.ExitCodeFailClosed {
		t.Fatalf("exit code = %d, want %d (fail-closed); stdout=%q stderr=%q", code, compare.ExitCodeFailClosed, out.String(), errb.String())
	}
}

func TestCompareSuiteRecordFlag(t *testing.T) {
	agreeDigest := suiteDigest(t, suiteAgreeBundleJSON)
	dir := writeSuiteDir(t,
		mkSuiteJSON([]struct{ id, digest string }{{"partition-grow-agree", agreeDigest}}),
		map[string]string{"partition-grow-agree": suiteAgreeBundleJSON},
		mergePolicyYAML("prod-strict-6", "new >= old", "must not shrink"),
		mergePolicyYAML("prod-strict-7", "new >= old", "must not shrink"),
	)
	recordDir := filepath.Join(dir, "records")
	var out, errb bytes.Buffer
	code := runCompare([]string{"--suite", dir, "--record", recordDir}, &out, &errb)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errb.String())
	}
	raw, err := os.ReadFile(filepath.Join(recordDir, "partition-grow-agree.json")) // #nosec G304 -- test temp dir under t.TempDir().
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var rec compare.ComparisonRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	if rec.CaseID != "partition-grow-agree" {
		t.Fatalf("record caseId = %q, want partition-grow-agree", rec.CaseID)
	}
	if err := rec.Validate(); err != nil {
		t.Fatalf("record schema: %v", err)
	}
}
