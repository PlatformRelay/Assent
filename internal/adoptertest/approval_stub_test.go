package adoptertest_test

import (
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// loadReviewCase assembles the document-mode require-review case (testdata/reviewpack)
// with the given approval context. The rule's `when` is clean-false for the modify
// change, so onFailure require-review fires; whether it is SATISFIED depends only on
// the injected approval stub.
func loadReviewCase(t *testing.T, appr *adoptertest.Case) adoptertest.Case {
	t.Helper()
	const root = "testdata/reviewpack"
	mp, err := policy.LoadMergePolicy(readFile(t, filepath.Join(root, "pack", "rules", "review.yaml")))
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	rb, err := policy.LoadRulesetBinding(readFile(t, filepath.Join(root, "bindings.yaml")))
	if err != nil {
		t.Fatalf("load binding: %v", err)
	}
	const file = "config.json"
	c := adoptertest.Case{
		Name:   "review",
		Policy: mp,
		Bind:   &rb.Bindings[0],
		File:   file,
		Base:   readFile(t, filepath.Join(root, "case", "base", file)),
		Head:   readFile(t, filepath.Join(root, "case", "head", file)),
	}
	if appr != nil {
		c.Approval = appr.Approval
	}
	return c
}

// TestStubbedApprovalEvidenceSatisfiesRequireReview (REQ-E6-S02-05) proves a stubbed
// sha-matched ApprovalEvidence satisfies a require-review obligation via
// ApprovalContext (no live forge), and that an ABSENT or SHA-MISMATCHED stub leaves
// it unsatisfied → REVIEW. The fail-safe direction is load-bearing: a require-review
// obligation is NEVER satisfied by the absence of evidence.
func TestStubbedApprovalEvidenceSatisfiesRequireReview(t *testing.T) {
	appr, err := adoptertest.MapApproval(readFile(t, "testdata/reviewpack/approval.yaml"))
	if err != nil {
		t.Fatalf("MapApproval: %v", err)
	}

	// Matched sha -> satisfied -> APPROVE.
	matched := loadReviewCase(t, nil)
	matched.Approval = appr
	out, err := adoptertest.RunCase(matched)
	if err != nil {
		t.Fatalf("RunCase (matched): %v", err)
	}
	if out.Actual != "APPROVE" {
		t.Fatalf("sha-matched stub decision = %q, want APPROVE (obligation satisfied)", out.Actual)
	}

	// Absent stub -> unsatisfied -> REVIEW (never satisfied by absence).
	absent := loadReviewCase(t, nil)
	absent.Approval = nil
	out, err = adoptertest.RunCase(absent)
	if err != nil {
		t.Fatalf("RunCase (absent): %v", err)
	}
	if out.Actual != "REVIEW" {
		t.Fatalf("absent-stub decision = %q, want REVIEW (never satisfied by absence)", out.Actual)
	}

	// Sha-mismatched stub -> stale evidence never satisfies -> REVIEW.
	mismatched := loadReviewCase(t, nil)
	stale := *appr
	stale.SourceSha = "sha-different"
	mismatched.Approval = &stale
	out, err = adoptertest.RunCase(mismatched)
	if err != nil {
		t.Fatalf("RunCase (mismatched): %v", err)
	}
	if out.Actual != "REVIEW" {
		t.Fatalf("sha-mismatched stub decision = %q, want REVIEW (stale never satisfies)", out.Actual)
	}
}
