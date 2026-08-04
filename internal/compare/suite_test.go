package compare

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

func mkTwoCaseSuiteJSON(widenDigest, agreeDigest string) []byte {
	return []byte(`{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "PolicyComparisonSuite",
  "metadata": {"name": "pcs-s06-fixture", "version": "1"},
  "spec": {
    "cases": [
      {"caseId": "partition-shrink-widen", "replayBundleDigest": "` + widenDigest + `"},
      {"caseId": "partition-grow-agree", "replayBundleDigest": "` + agreeDigest + `"}
    ],
    "promotionGates": [
      {"gateId": "zero-missed-destructive", "failOnKinds": ["destructive-or-authorization-intervention-missed"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "zero-missed-authorization-ownership", "failOnKinds": ["destructive-or-authorization-intervention-missed"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "no-unexpected-obligation-removal", "failOnKinds": ["subject-or-obligation-uncovered"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "bounded-auto-merge-widening", "failOnKinds": ["newly-auto-mergeable"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "explicitly-accepted-deltas", "failOnKinds": ["stricter-intervention-added", "score-threshold-change"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"}
    ],
    "acceptedDeltas": []
  }
}`)
}

func suiteProfiles() (Profile, Profile) {
	baseline := mkProfile("prod-strict@6", "new >= old", "", policy.EffectBlock)
	candidate := mkProfile("prod-strict@7", "true", "", policy.EffectBlock)
	return baseline, candidate
}

func twoCaseBundles(t *testing.T) (map[string][]byte, string, string) {
	t.Helper()
	widenDigest, err := ReplayBundleDigest([]byte(bundleJSON))
	if err != nil {
		t.Fatalf("ReplayBundleDigest(widen): %v", err)
	}
	agreeDigest, err := ReplayBundleDigest([]byte(obligationBundleJSON))
	if err != nil {
		t.Fatalf("ReplayBundleDigest(agree): %v", err)
	}
	return map[string][]byte{
		"partition-shrink-widen": []byte(bundleJSON),
		"partition-grow-agree":   []byte(obligationBundleJSON),
	}, widenDigest, agreeDigest
}

// REQ-PCS-S06-01: a suite with ≥2 cases yields one ComparisonRecord and gate
// results per case.
func TestRunSuiteMultiCase(t *testing.T) {
	bundles, widenDigest, agreeDigest := twoCaseBundles(t)
	suite, err := LoadSuite(mkTwoCaseSuiteJSON(widenDigest, agreeDigest))
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}
	baseline, candidate := suiteProfiles()

	got, err := RunSuite(suite, bundles, baseline, candidate)
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	if len(got.Records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(got.Records))
	}
	if got.Records[0].CaseID != "partition-grow-agree" || got.Records[1].CaseID != "partition-shrink-widen" {
		t.Fatalf("records not sorted by caseId: %q, %q", got.Records[0].CaseID, got.Records[1].CaseID)
	}
	if len(got.Records[0].Deltas) != 0 {
		t.Fatalf("agree case deltas = %+v, want none", got.Records[0].Deltas)
	}
	if len(got.Records[1].Deltas) != 1 || got.Records[1].Deltas[0].Kind != KindNewlyAutoMergeable {
		t.Fatalf("widen case delta = %+v, want newly-auto-mergeable", got.Records[1].Deltas)
	}
	if got.Gates.Results[GateBoundedAutoMergeWidening] != VerdictFail {
		t.Fatalf("bounded-auto-merge-widening = %q, want FAIL", got.Gates.Results[GateBoundedAutoMergeWidening])
	}
	if got.Gates.FirstFailure != GateBoundedAutoMergeWidening {
		t.Fatalf("FirstFailure = %q, want %q", got.Gates.FirstFailure, GateBoundedAutoMergeWidening)
	}
	for _, rec := range got.Records {
		if err := rec.Validate(); err != nil {
			t.Fatalf("record %q schema: %v", rec.CaseID, err)
		}
	}
}

// REQ-PCS-S06-02: digest mismatch fails closed before evaluation.
func TestRunSuiteDigestMismatch(t *testing.T) {
	bundles, widenDigest, agreeDigest := twoCaseBundles(t)
	suite, err := LoadSuite(mkTwoCaseSuiteJSON(widenDigest, agreeDigest))
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}
	bundles["partition-shrink-widen"] = []byte(`{"apiVersion":"assent.dev/v1alpha1","kind":"ReplayBundle","pins":{"toolVersion":"x","toolDigest":"sha256:aaaa","policySha":"sha256:bbbb","sourceSha":"c","targetSha":"d","mergeResultDigest":"sha256:eeee","factsResolvedAt":{}},"evaluationInput":{"apiVersion":"assent.dev/v1alpha1","kind":"EvaluationInput","changeSet":{"changes":[]},"facts":{},"mr":{"author":"a","sourceBranch":"s","targetBranch":"main"},"require":[]}}`)
	baseline, candidate := suiteProfiles()
	_, err = RunSuite(suite, bundles, baseline, candidate)
	if err == nil {
		t.Fatal("expected digest mismatch error, got nil")
	}
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("error = %v, want ErrDigestMismatch", err)
	}
}

// REQ-PCS-S06-03: double-run of RunSuite is byte-identical (determinism).
func TestRunSuiteDeterministic(t *testing.T) {
	bundles, widenDigest, agreeDigest := twoCaseBundles(t)
	raw := mkTwoCaseSuiteJSON(widenDigest, agreeDigest)
	suite, err := LoadSuite(raw)
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}
	baseline, candidate := suiteProfiles()

	first, err := RunSuite(suite, bundles, baseline, candidate)
	if err != nil {
		t.Fatalf("RunSuite (1): %v", err)
	}
	suite2, err := LoadSuite(raw)
	if err != nil {
		t.Fatalf("LoadSuite (2): %v", err)
	}
	second, err := RunSuite(suite2, bundles, baseline, candidate)
	if err != nil {
		t.Fatalf("RunSuite (2): %v", err)
	}
	a, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	b, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("double run diverged:\n first=%s\nsecond=%s", string(a), string(b))
	}
}

func TestLoadSuiteRejectsUnknownAcceptedCaseID(t *testing.T) {
	_, widenDigest, agreeDigest := twoCaseBundles(t)
	raw := []byte(`{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "PolicyComparisonSuite",
  "metadata": {"name": "bad-allowlist", "version": "1"},
  "spec": {
    "cases": [
      {"caseId": "partition-shrink-widen", "replayBundleDigest": "` + widenDigest + `"},
      {"caseId": "partition-grow-agree", "replayBundleDigest": "` + agreeDigest + `"}
    ],
    "promotionGates": [
      {"gateId": "zero-missed-destructive", "failOnKinds": ["destructive-or-authorization-intervention-missed"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "zero-missed-authorization-ownership", "failOnKinds": ["destructive-or-authorization-intervention-missed"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "no-unexpected-obligation-removal", "failOnKinds": ["subject-or-obligation-uncovered"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "bounded-auto-merge-widening", "failOnKinds": ["newly-auto-mergeable"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "explicitly-accepted-deltas", "failOnKinds": ["stricter-intervention-added", "score-threshold-change"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"}
    ],
    "acceptedDeltas": [
      {"caseId": "no-such-case", "kind": "newly-auto-mergeable", "rule": "r", "subject": "s", "rationale": "orphan"}
    ]
  }
}`)
	_, err := LoadSuite(raw)
	if err == nil {
		t.Fatal("expected unknown acceptedDeltas caseId to fail closed")
	}
}

func TestRunSuiteMissingCaseBundle(t *testing.T) {
	_, widenDigest, agreeDigest := twoCaseBundles(t)
	suite, err := LoadSuite(mkTwoCaseSuiteJSON(widenDigest, agreeDigest))
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}
	baseline, candidate := suiteProfiles()
	_, err = RunSuite(suite, map[string][]byte{
		"partition-shrink-widen": []byte(bundleJSON),
	}, baseline, candidate)
	if err == nil {
		t.Fatal("expected missing case bundle error")
	}
}

func TestLoadSuiteRejectsInvalidSchema(t *testing.T) {
	_, err := LoadSuite([]byte(`{"kind":"not-a-suite"}`))
	if err == nil {
		t.Fatal("expected schema validation error")
	}
}

func TestReplayBundleDigestRejectsInvalidJSON(t *testing.T) {
	_, err := ReplayBundleDigest([]byte("not json"))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRunSuiteFailsOnUnclassifiableDelta(t *testing.T) {
	widenDigest, err := ReplayBundleDigest([]byte(bundleJSON))
	if err != nil {
		t.Fatalf("ReplayBundleDigest: %v", err)
	}
	raw := []byte(`{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "PolicyComparisonSuite",
  "metadata": {"name": "unclassifiable", "version": "1"},
  "spec": {
    "cases": [{"caseId": "block-to-review", "replayBundleDigest": "` + widenDigest + `"}],
    "promotionGates": [
      {"gateId": "zero-missed-destructive", "failOnKinds": ["destructive-or-authorization-intervention-missed"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "zero-missed-authorization-ownership", "failOnKinds": ["destructive-or-authorization-intervention-missed"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "no-unexpected-obligation-removal", "failOnKinds": ["subject-or-obligation-uncovered"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "bounded-auto-merge-widening", "failOnKinds": ["newly-auto-mergeable"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"},
      {"gateId": "explicitly-accepted-deltas", "failOnKinds": ["stricter-intervention-added", "score-threshold-change"], "defaultVerdict": "fail", "acceptance": "per-delta-identity"}
    ]
  }
}`)
	suite, err := LoadSuite(raw)
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}
	baseline := mkProfile("prod@6", "new >= old", "", policy.EffectBlock)
	candidate := mkProfile("prod@7", "new >= old", "", policy.EffectRequireReview)
	_, err = RunSuite(suite, map[string][]byte{"block-to-review": []byte(bundleJSON)}, baseline, candidate)
	if err == nil {
		t.Fatal("expected unclassifiable delta error")
	}
	if !errors.Is(err, ErrUnclassifiable) {
		t.Fatalf("error = %v, want ErrUnclassifiable", err)
	}
}

func TestSuiteRunResultMarshalEmptyRecords(t *testing.T) {
	raw, err := json.Marshal(SuiteRunResult{Gates: GateEvaluation{Results: map[GateID]Verdict{
		GateBoundedAutoMergeWidening: VerdictPass,
	}}})
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"records":[]`)) {
		t.Fatalf("expected empty records array, got %s", string(raw))
	}
}
