package adoptertest_test

import (
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// TestMrYamlPopulatesMRWithDefaults (REQ-E6-S02-06) proves an mr.yaml populates
// aggregate.MR (author/labels/target) and threads into the `mr` predicate scope,
// and that an ABSENT mr.yaml applies the ADR-0014 default (empty MR). The pack's
// obligation is proven only when `"urgent" in mr.labels`: with the label present it
// APPROVEs; with the default (no labels) it fires → REVIEW.
func TestMrYamlPopulatesMRWithDefaults(t *testing.T) {
	const root = "testdata/mrpack"
	mp, err := policy.LoadMergePolicy(readFile(t, filepath.Join(root, "pack", "rules", "labeled.yaml")))
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	rb, err := policy.LoadRulesetBinding(readFile(t, filepath.Join(root, "bindings.yaml")))
	if err != nil {
		t.Fatalf("load binding: %v", err)
	}
	const file = "config.json"
	base := func() adoptertest.Case {
		return adoptertest.Case{
			Name: "mr", Policy: mp, Bind: &rb.Bindings[0], File: file,
			Base: readFile(t, filepath.Join(root, "case", "base", file)),
			Head: readFile(t, filepath.Join(root, "case", "head", file)),
		}
	}

	// mr.yaml present, labels: [urgent] -> obligation proven -> APPROVE.
	mr, err := adoptertest.MapMR(readFile(t, filepath.Join(root, "mr.yaml")))
	if err != nil {
		t.Fatalf("MapMR: %v", err)
	}
	if mr.Author != "alice" || mr.TargetBranch != "main" || len(mr.Labels) != 1 || mr.Labels[0] != "urgent" {
		t.Fatalf("mr.yaml not lifted faithfully: %+v", mr)
	}
	withMR := base()
	withMR.MR = mr
	out, err := adoptertest.RunCase(withMR)
	if err != nil {
		t.Fatalf("RunCase (with mr): %v", err)
	}
	if out.Actual != "APPROVE" {
		t.Fatalf("with mr.labels=[urgent] decision = %q, want APPROVE", out.Actual)
	}

	// Absent mr.yaml -> the ADR-0014 default (zero MR, no labels) -> the label
	// obligation fires -> REVIEW.
	def, err := adoptertest.MapMR(nil)
	if err != nil {
		t.Fatalf("MapMR(nil): %v", err)
	}
	if def.Author != "" || def.SourceBranch != "" || def.TargetBranch != "" || len(def.Labels) != 0 {
		t.Fatalf("absent mr.yaml must map to the zero MR default, got %+v", def)
	}
	withoutMR := base() // Case.MR defaults to the zero MR
	out, err = adoptertest.RunCase(withoutMR)
	if err != nil {
		t.Fatalf("RunCase (no mr): %v", err)
	}
	if out.Actual != "REVIEW" {
		t.Fatalf("absent mr.yaml decision = %q, want REVIEW (default has no labels)", out.Actual)
	}
}

// TestMapMRRejectsUnknownField proves mr.yaml is strict-decoded: an unknown field is
// a located rejection, never silently ignored.
func TestMapMRRejectsUnknownField(t *testing.T) {
	if _, err := adoptertest.MapMR([]byte("author: a\nbogus: x\n")); err == nil {
		t.Fatal("expected an unknown-field rejection for mr.yaml")
	}
}

// TestMapApprovalRejectsUnknownFieldAndMissingSubject proves approval.yaml is
// strict-decoded and that an evidence entry with no subject is rejected (a stub that
// keys nothing could never satisfy the right subject).
func TestMapApprovalRejectsUnknownFieldAndMissingSubject(t *testing.T) {
	if _, err := adoptertest.MapApproval([]byte("sourceSha: s\nbogus: 1\n")); err == nil {
		t.Fatal("expected an unknown-field rejection for approval.yaml")
	}
	if _, err := adoptertest.MapApproval([]byte("sourceSha: s\nevidence:\n  - approvalsRequired: 1\n")); err == nil {
		t.Fatal("expected a missing-subject rejection for approval.yaml evidence")
	}
}
