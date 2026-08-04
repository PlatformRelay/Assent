package main

import (
	"bytes"
	"strings"
	"testing"
)

// REQ-E7-S03-03 (ADR-0015 §8): fork/untrusted contributor MR context → advisory-only,
// zero forge writes (no thread, approve, or merge) even when decision APPROVE and
// forge probe would arm.
func TestRunForkContextAdvisoryOnly(t *testing.T) {
	f := newFakeGitLab(t)
	f.forkMR = true
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (advisory report)\n%s", code, out.String())
	}
	body := out.String()
	if !strings.Contains(body, `"decision":"APPROVE"`) {
		t.Fatalf("fork MR may still evaluate → APPROVE record; got:\n%s", body)
	}
	if f.discussionsPosted != 0 || f.approvals != 0 || f.merges != 0 {
		t.Errorf("fork/untrusted context must produce ZERO forge writes: threads=%d approvals=%d merges=%d",
			f.discussionsPosted, f.approvals, f.merges)
	}
	if !strings.Contains(body, "advisory-only") {
		t.Errorf("expected fork advisory-only summary:\n%s", body)
	}
	if !strings.Contains(body, "fork") && !strings.Contains(body, "untrusted") {
		t.Errorf("summary should name fork/untrusted context:\n%s", body)
	}
}
