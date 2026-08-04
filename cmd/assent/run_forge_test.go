package main

import (
	"bytes"
	"strings"
	"testing"
)

// REQ-E4-S06-01: forge Resolve supplies ApprovalEvidence that satisfies a
// require-review fixture when eligible → APPROVE + forge-probed merge writes.
func TestRunResolveApprovalEvidence(t *testing.T) {
	f := newFakeGitLab(t)
	f.mergePolicy = mergePolicyRequireReviewOnFalse
	f.rulesetBinding = rulesetBindingOwnership
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"
	f.approvalEligible = true

	var out bytes.Buffer
	code := runRun(runArgs(), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), `"decision":"APPROVE"`) {
		t.Fatalf("eligible Resolve evidence must satisfy require-review → APPROVE:\n%s", out.String())
	}
	if f.approvals != 1 || f.merges != 1 {
		t.Errorf("APPROVE with forge-probed arming must write: approvals=%d merges=%d", f.approvals, f.merges)
	}
}

// REQ-E4-S06-02: Snapshot changed files feed classifier when --checkout unset;
// a smuggled `.assent/**` path in Snapshot (not the governed subject alone) → BLOCK.
func TestRunSnapshotChangedFiles(t *testing.T) {
	t.Run("snapshot_only_policy_edit_blocks", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"
		f.changedFiles = []string{f.governedPath, ".assent/packs/topic.yml"}

		var out bytes.Buffer
		code := runRun(runArgs(), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if !strings.Contains(out.String(), `"decision":"BLOCK"`) {
			t.Errorf("Snapshot `.assent/**` must dominate classifier → BLOCK:\n%s", out.String())
		}
		if f.approvals != 0 || f.merges != 0 {
			t.Errorf("BLOCK must not approve/merge: approvals=%d merges=%d", f.approvals, f.merges)
		}
	})

	t.Run("checkout_set_snapshot_ignored", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"
		f.changedFiles = []string{f.governedPath, ".assent/packs/topic.yml"}
		checkout := writeCheckout(t, map[string][2]string{
			f.governedPath:   {"partitions: 12\n", "partitions: 24\n"},
			"docs/readme.yaml": {"version: 1\n", "version: 2\n"},
		})

		var out bytes.Buffer
		code := runRun(runArgs("--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if !strings.Contains(out.String(), `"decision":"APPROVE"`) {
			t.Errorf("checkout-only paths must win over Snapshot extras → APPROVE:\n%s", out.String())
		}
		if f.approvals != 1 || f.merges != 1 {
			t.Errorf("expected forge-probed merge writes: approvals=%d merges=%d", f.approvals, f.merges)
		}
	})
}

// REQ-E4-S06-04: Free-tier Resolve gap → REVIEW, never APPROVE; capability gap
// recorded distinctly (verifyingCapability:none path).
func TestRunTierGapNeverApproves(t *testing.T) {
	f := newFakeGitLab(t)
	f.mergePolicy = mergePolicyOwnership
	f.rulesetBinding = rulesetBindingOwnership
	f.freeTier = true
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if strings.Contains(out.String(), `"decision":"APPROVE"`) {
		t.Fatalf("tier gap must never APPROVE on missing approval evidence:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `"decision":"REVIEW"`) {
		t.Errorf("expected REVIEW on tier gap:\n%s", out.String())
	}
	if f.approvals != 0 || f.merges != 0 {
		t.Errorf("tier gap must not write: approvals=%d merges=%d", f.approvals, f.merges)
	}
}

// REQ-E4-S06-05: forge-probed ArmEligible gates writes — not env self-assertion,
// not --arm alone. Spoofed env arming vars must not arm when forge probe eligible
// is the authority (writes happen from probe, not env).
func TestRunForgeProbedArmingGatesWrites(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	t.Setenv("ASSENT_PIPELINE_CONFIG_PROTECTED", "true")
	t.Setenv("ASSENT_PIPELINE_CONFIG_AUTHOR_EDITABLE", "false")
	t.Setenv("ASSENT_PIPELINE_TOKEN_PRIVILEGED", "false")

	var out bytes.Buffer
	code := runRun(runArgs(), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if f.approvals != 1 || f.merges != 1 {
		t.Errorf("forge-probed eligible APPROVE must write without --arm or env: approvals=%d merges=%d", f.approvals, f.merges)
	}
	if !strings.Contains(out.String(), `"decision":"APPROVE"`) {
		t.Errorf("expected APPROVE record:\n%s", out.String())
	}
}

// REQ-E4-S06-06: forge probe ArmEligible=false → zero writes even with --arm.
func TestRunForgeProbeRefusesArmDespiteFlag(t *testing.T) {
	t.Run("missing_C3_gate", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.projectJSON = fakeForgeIneligibleProjectJSON
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"

		var out bytes.Buffer
		code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (advisory)\n%s", code, out.String())
		}
		if f.approvals != 0 || f.merges != 0 {
			t.Errorf("--arm must not override forge refusal: approvals=%d merges=%d", f.approvals, f.merges)
		}
		if !strings.Contains(out.String(), "advisory-only") {
			t.Errorf("expected ErrArmingRefused summary:\n%s", out.String())
		}
	})

	t.Run("insecure_C17_topology", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.projectJSON = fakeForgeInsecureProjectJSON
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"

		var out bytes.Buffer
		code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if f.approvals != 0 || f.merges != 0 {
			t.Errorf("insecure topology must refuse writes: approvals=%d merges=%d", f.approvals, f.merges)
		}
	})

	t.Run("tier_gap", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.freeTier = true
		f.mergePolicy = mergePolicyChallenge
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"

		var out bytes.Buffer
		code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if f.approvals != 0 || f.merges != 0 {
			t.Errorf("tier gap must refuse arming writes: approvals=%d merges=%d", f.approvals, f.merges)
		}
	})
}

// REQ-E4-S06-07: --checkout set + Snapshot returns extra policy paths → classifier
// uses checkout paths only (Snapshot extras ignored; no union).
func TestRunCheckoutPrecedenceOverSnapshot(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"
	f.changedFiles = []string{f.governedPath, ".assent/merge-policy.yaml"}

	checkout := writeCheckout(t, map[string][2]string{
		f.governedPath:   {"partitions: 12\n", "partitions: 24\n"},
		"docs/readme.yaml": {"version: 1\n", "version: 2\n"},
	})

	var out bytes.Buffer
	code := runRun(runArgs("--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if strings.Contains(out.String(), `"decision":"BLOCK"`) {
		t.Fatalf("Snapshot `.assent/**` extra must be ignored when --checkout is set:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `"decision":"APPROVE"`) {
		t.Errorf("checkout-only enumeration must APPROVE:\n%s", out.String())
	}
	if f.approvals != 1 || f.merges != 1 {
		t.Errorf("expected merge writes: approvals=%d merges=%d", f.approvals, f.merges)
	}
}
