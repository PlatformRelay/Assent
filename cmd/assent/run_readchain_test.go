package main

import (
	"bytes"
	"strings"
	"testing"
)

// run_readchain_test.go — the one-read-chain check for the Snapshot (E10
// branch-review round 2, finding 1). orchestrate reads the MR twice: GetMR
// (read 1, which pins the record's SHAs) and Snapshot (read 2, which carries
// the merge-result digest and the changed-file list). If the forge heads moved
// between the reads, the record would pin read 1's SHAs while judging read 2's
// data — the run must FAIL CLOSED (error, zero forge writes, no record) rather
// than assemble a decision from two reads.
func TestRunFailsClosedWhenHeadsMoveBetweenMRReadAndSnapshot(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"
	f.secondPRSha = "srcMoved" // the Snapshot's MR read serves a MOVED head

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &errOut, f.factory())
	if code == 0 {
		t.Fatalf("a run whose two MR reads disagree must exit non-zero, got 0\nstdout:\n%s\nstderr:\n%s", out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "forge heads moved between the MR read and the snapshot (pinned srcSHA/tgtTIP, snapshot srcMoved/tgtTIP)") {
		t.Errorf("the refusal must name the pinned vs snapshot heads (re-evaluation required):\nstderr:\n%s", errOut.String())
	}
	if f.approvals != 0 || f.merges != 0 || f.discussionsPosted != 0 || f.notesPosted != 0 {
		t.Errorf("a heads-moved run must write NOTHING to the forge: approvals=%d merges=%d discussions=%d notes=%d",
			f.approvals, f.merges, f.discussionsPosted, f.notesPosted)
	}
	if strings.Contains(out.String(), `"decision"`) {
		t.Errorf("no DecisionRecord may be emitted from inconsistent reads:\n%s", out.String())
	}
}

// TestRunSameHeadsProceeds is the positive control: with the heads identical
// across both MR reads, the run proceeds and the armed path merges — the
// heads-moved refusal above is not satisfied by a run that always fails.
func TestRunSameHeadsProceeds(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (the identical-heads control proceeds)\n%s", code, out.String())
	}
	if f.approvals != 1 || f.merges != 1 {
		t.Errorf("same-heads run must proceed to approve+merge: approvals=%d merges=%d", f.approvals, f.merges)
	}
}
