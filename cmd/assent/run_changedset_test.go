package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCheckout materialises a fixture checkout directory with base/ and head/
// subtrees. files maps a relative path to a [base, head] content pair; an empty
// string means the file is ABSENT on that side (a whole-file add/delete). The
// returned root is what `--checkout` points at — the LOCAL-checkout double the
// E1-S08 enumeration reads (never a forge API). Deterministic: no clock, no net.
func writeCheckout(t *testing.T, files map[string][2]string) string {
	t.Helper()
	root := t.TempDir()
	write := func(side, rel, content string) {
		if content == "" {
			return // absent on this side
		}
		p := filepath.Join(root, side, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	for rel, bh := range files {
		write("base", rel, bh[0])
		write("head", rel, bh[1])
	}
	return root
}

// REQ-E1-S08-01: an MR whose governed file would otherwise APPROVE (partitions
// 12 -> 24, --arm) but which ALSO smuggles a `.assent/**` policy edit is caught
// on the live-adapter path: the enumerated changed-file set routes the union to
// assent-policy -> BLOCK, never approve/merge. The governed file alone would have
// merged; the smuggled `.assent` edit is what blocks — the golden's control/real
// contrast, now proven on cmd/assent.
func TestRunEnumeratesChangedFileSetAndBlocksOnPolicyEdit(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n" // governed alone => APPROVE

	checkout := writeCheckout(t, map[string][2]string{
		"topics/orders.yaml":      {"partitions: 12\n", "partitions: 24\n"},
		".assent/packs/topic.yml": {"threshold: 10\n", "threshold: 1\n"},
	})

	var out bytes.Buffer
	code := runRun(runArgs("--arm", "--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), `"decision":"BLOCK"`) {
		t.Errorf("expected BLOCK (assent-policy dominates the smuggled edit):\n%s", out.String())
	}
	if f.approvals != 0 || f.merges != 0 {
		t.Errorf("smuggled `.assent` edit must NOT approve/merge: approvals=%d merges=%d", f.approvals, f.merges)
	}
	if f.discussionsPosted != 1 {
		t.Errorf("BLOCK must post exactly one thread, got %d", f.discussionsPosted)
	}
}

// REQ-E1-S08-02 (additive proof): a checkout containing ONLY non-policy files
// behaves exactly as the shipped single-governed-file path — the governed
// predicate still APPROVEs and, with --arm, merges. Enumerating extra non-policy
// files does not regress the approve path. (The unmodified run_test.go suite is
// the primary REQ-02 evidence; this pins the checkout-mode non-policy case.)
func TestRunChangedFileSetNonPolicyApprovesLikeSingleFile(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	checkout := writeCheckout(t, map[string][2]string{
		"topics/orders.yaml": {"partitions: 12\n", "partitions: 24\n"},
		"docs/readme.yaml":   {"version: 1\n", "version: 2\n"}, // unrelated, non-policy
	})

	var out bytes.Buffer
	code := runRun(runArgs("--arm", "--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), `"decision":"APPROVE"`) {
		t.Errorf("non-policy checkout must decide like single-file (APPROVE):\n%s", out.String())
	}
	if f.approvals != 1 || f.merges != 1 {
		t.Errorf("armed non-policy APPROVE must approve+merge: approvals=%d merges=%d", f.approvals, f.merges)
	}
}

// REQ-E1-S08-03: an opaque changed file among several must fail the WHOLE run
// safe — it is never silently dropped so the clean files approve on their own.
func TestRunOpaqueFileAmongManyFailsSafe(t *testing.T) {
	// Case A: an opaque NON-policy file among a clean governed file. The union is
	// opaque -> REVIEW; --arm cannot merge.
	t.Run("opaque-non-policy", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"

		checkout := writeCheckout(t, map[string][2]string{
			"topics/orders.yaml": {"partitions: 12\n", "partitions: 24\n"},
			// Unparseable head => Diff is opaque (fail-safe), not silently dropped.
			"config/app.yaml": {"a: 1\n", "\tthis: is: not: [valid"},
		})

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if f.approvals != 0 || f.merges != 0 {
			t.Errorf("opaque-among-many must NOT approve on the clean file: approvals=%d merges=%d", f.approvals, f.merges)
		}
		if !strings.Contains(out.String(), `"decision":"REVIEW"`) {
			t.Errorf("opaque non-policy file must fail safe to REVIEW:\n%s", out.String())
		}
	})

	// Case B (adversarial): an opaque `.assent/**` file plus a clean governed
	// file. The path-scan still routes to assent-policy -> BLOCK even though the
	// opaque diff carries no changes — a policy edit cannot hide behind its own
	// opacity. Either way the run fails safe (never APPROVE).
	t.Run("opaque-policy-adversarial", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"

		checkout := writeCheckout(t, map[string][2]string{
			"topics/orders.yaml":      {"partitions: 12\n", "partitions: 24\n"},
			".assent/packs/topic.yml": {"threshold: 10\n", "\tnot: [valid: yaml"},
		})

		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if f.approvals != 0 || f.merges != 0 {
			t.Errorf("opaque `.assent` edit must NOT approve on the clean file: approvals=%d merges=%d", f.approvals, f.merges)
		}
		if strings.Contains(out.String(), `"decision":"APPROVE"`) {
			t.Errorf("opaque `.assent` edit must never APPROVE:\n%s", out.String())
		}
		if !strings.Contains(out.String(), `"decision":"BLOCK"`) {
			t.Errorf("opaque `.assent` edit should route to assent-policy BLOCK (path dominates opacity):\n%s", out.String())
		}
	})
}

// REQ-E1-S08-04: the changed-file-set enumeration is proven against a fixture
// checkout only (no live network) and the decision path double-runs
// byte-identical.
func TestRunChangedFileSetDoubleRunStable(t *testing.T) {
	files := map[string][2]string{
		"topics/orders.yaml":      {"partitions: 12\n", "partitions: 24\n"},
		".assent/packs/topic.yml": {"threshold: 10\n", "threshold: 1\n"},
	}
	run := func() string {
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"
		checkout := writeCheckout(t, files)
		var out bytes.Buffer
		code := runRun(runArgs("--arm", "--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		return out.String()
	}
	first := run()
	second := run()
	if first != second {
		t.Fatalf("changed-file-set decision path is not byte-identical across runs:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	if !strings.Contains(first, `"decision":"BLOCK"`) {
		t.Fatalf("expected the smuggled-policy fixture to BLOCK:\n%s", first)
	}
}
