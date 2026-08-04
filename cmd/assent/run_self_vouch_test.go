package main

import (
	"bytes"
	"strings"
	"testing"
)

// mergePolicySmuggledSource is adversarial source-branch policy bytes: empty rules
// would APPROVE anything if wrongly loaded. orchestrate must ignore it (ADR-0015 §1).
const mergePolicySmuggledSource = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "MergePolicy",
  "metadata": { "name": "smuggled-permissive" },
  "spec": { "entries": {}, "rules": [] }
}`

// REQ-E4-S08-01 (D-042 F1): Snapshot changed-files includes `.assent/**` →
// assent-policy meta-class → BLOCK with ZERO forge writes (no thread/approve/merge).
func TestRunAssentPolicySelfModificationBlocks(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n" // governed alone + --arm => APPROVE
	f.changedFiles = []string{f.governedPath, ".assent/merge-policy.yaml"}

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), `"decision":"BLOCK"`) {
		t.Fatalf("`.assent/**` self-edit must BLOCK:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `"code":"assent-policy.self-edit"`) {
		t.Fatalf("expected assent-policy.self-edit finding:\n%s", out.String())
	}
	if f.discussionsPosted != 0 || f.approvals != 0 || f.merges != 0 {
		t.Errorf("self-edit BLOCK must produce ZERO forge writes: threads=%d approvals=%d merges=%d",
			f.discussionsPosted, f.approvals, f.merges)
	}
	if !strings.Contains(out.String(), "no forge writes") {
		t.Errorf("expected self-edit no-write summary:\n%s", out.String())
	}
}

// REQ-E4-S08-02: policy documents load from the TARGET ref only — the source
// branch may carry smuggled bytes but PolicySha and evaluation use target policy.
func TestRunPolicyFromTargetRefOnly(t *testing.T) {
	f := newFakeGitLab(t)
	f.mergePolicy = mergePolicyChallenge
	f.sourceMergePolicy = mergePolicySmuggledSource
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"
	f.changedFiles = []string{f.governedPath, ".assent/merge-policy.yaml"}

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	wantPolicySha := `"policySha":"` + sha256Prefix + sha256Hex([]byte(f.mergePolicy)) + `"`
	if !strings.Contains(out.String(), wantPolicySha) {
		t.Fatalf("PolicySha must digest TARGET-ref merge-policy, not source smuggle:\n got output missing %s\n%s", wantPolicySha, out.String())
	}
	smuggledSha := sha256Prefix + sha256Hex([]byte(f.sourceMergePolicy))
	if strings.Contains(out.String(), smuggledSha) {
		t.Fatalf("record must NOT carry source-branch policy digest %q", smuggledSha)
	}
	for _, load := range f.policyLoads {
		if load.ref != f.target {
			t.Errorf("policy load from ref %q, want target ref %q only (path %q)", load.ref, f.target, load.path)
		}
	}
	if len(f.policyLoads) < 2 {
		t.Fatalf("expected merge-policy + ruleset-binding loads, got %d: %+v", len(f.policyLoads), f.policyLoads)
	}
}
