package main

import (
	"bytes"
	"strings"
	"testing"
)

// run_fork_addressing_test.go pins the E10-S02 trust-boundary half of the
// governed-subject migration (REQ-E10-S02-05(ii)): the two MR-relative
// accessors serve the GOVERNED SUBJECT only — the six policy/decision-input
// loads stay ref-addressed at the TARGET project's pinned target SHA, even for
// a fork MR. A "consistency" refactor of the policy loads onto an MR-relative
// accessor would let a fork's head reach the policy load (the D-130
// who-may-approve escalation shape), and this test reds on exactly that.

// TestPolicyLoadsFromTargetRefOnForkMR runs the orchestration against a fork MR
// whose source project carries POISONED policy bytes, and asserts every policy
// load resolved the pinned target SHA of the TARGET project — the fork's head
// can never reach them.
func TestPolicyLoadsFromTargetRefOnForkMR(t *testing.T) {
	f := newFakeGitLab(t)
	f.forkMR = true
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"
	// Poison on the source side: if any policy document were read at the fork's
	// head (an MR-relative accessor reused for policy), the run would judge it.
	f.sourceMergePolicy = "WRONG: smuggled-policy-from-fork-head\n"
	f.sourceRulesetBinding = "WRONG: smuggled-binding-from-fork-head\n"

	var out bytes.Buffer
	code := runRun(runArgs(), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	body := out.String()
	// The policy digest must be the TARGET-ref document's — never the fork's.
	wantPolicySha := `"policySha":"` + sha256Prefix + sha256Hex([]byte(f.mergePolicy)) + `"`
	if !strings.Contains(body, wantPolicySha) {
		t.Fatalf("policy must load from the TARGET ref even on a fork MR:\n got output missing %s\n%s", wantPolicySha, body)
	}
	if strings.Contains(body, sha256Hex([]byte(f.sourceMergePolicy))) {
		t.Fatal("a fork's head must never reach a policy load (ADR-0015 §1 / REQ-E10-S02-05(ii))")
	}
	for _, load := range f.policyLoads {
		if load.ref != f.targetTip {
			t.Errorf("policy load from ref %q, want pinned target SHA %q only (path %q)", load.ref, f.targetTip, load.path)
		}
	}
	if len(f.policyLoads) < 2 {
		t.Fatalf("expected merge-policy + ruleset-binding loads, got %d: %+v", len(f.policyLoads), f.policyLoads)
	}
}
