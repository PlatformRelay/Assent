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
	sides := make(map[string][2]*string, len(files))
	for rel, bh := range files {
		var base, head *string
		if bh[0] != "" {
			s := bh[0]
			base = &s
		}
		if bh[1] != "" {
			s := bh[1]
			head = &s
		}
		sides[rel] = [2]*string{base, head}
	}
	return writeCheckoutPresence(t, sides)
}

// writeCheckoutPresence is writeCheckout with an explicit presence signal: a nil
// *string means ABSENT; a non-nil *string (including pointing at "") means
// PRESENT — so an empty-but-present file (EFE-S03 fail-safe) can be fixture'd
// without colliding with writeCheckout's ""=absent convention.
func writeCheckoutPresence(t *testing.T, files map[string][2]*string) string {
	t.Helper()
	root := t.TempDir()
	write := func(side, rel string, content *string) {
		if content == nil {
			return // absent on this side
		}
		p := filepath.Join(root, side, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(*content), 0o600); err != nil {
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

// mergePolicyFileEvents is a document-mode pack whose sole rule governs a
// whole-file add/delete over topics/*.yaml (EFE-S03): prove.when kind != "delete"
// is proven by an ADD and failed by a DELETE — the live-checkout mirror of the
// S02 harness fileEventsPack.
const mergePolicyFileEvents = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "MergePolicy",
  "metadata": { "name": "topic-lifecycle" },
  "spec": {
    "entries": { "topic-registry": { "mode": "document", "root": "", "identity": { "pointer": "/metadata/name" } } },
    "rules": [
      {
        "name": "topic-lifecycle",
        "phase": "enforce",
        "match": { "fileEvents": { "paths": ["topics/*.yaml"], "kinds": ["add", "delete"] } },
        "prove": { "obligation": "reviewed-deletion", "when": "kind != \"delete\"" },
        "onFailure": { "effect": "require-review", "code": "topic.deletion-needs-review" }
      }
    ]
  }
}`

const rulesetBindingReviewedDeletion = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "RulesetBinding",
  "bindings": [
    { "class": "topic-registry", "environment": "prod", "packs": ["topic-lifecycle"], "risk": { "threshold": 10 }, "require": ["reviewed-deletion"] }
  ]
}`

// TestLiveCheckoutMintsFileEvent (REQ-EFE-S03-01) — a dirCheckout whose governed
// file is present on exactly one side mints a clean whole-file event (via
// change.FileEvent) that a fileEvents rule selects end-to-end: head-only = ADD
// (proving polarity → APPROVE); base-only = DELETE (failing polarity → REVIEW).
// Presence is nil-vs-non-nil from the local tree, never empty bytes.
func TestLiveCheckoutMintsFileEvent(t *testing.T) {
	present := "enabled: true\n"

	t.Run("present only on head -> file-ADD (proving polarity)", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.mergePolicy = mergePolicyFileEvents
		f.rulesetBinding = rulesetBindingReviewedDeletion
		// FileAtRef still answers, but --checkout is the presence authority for
		// the governed subject (EFE-S03); one-sided lifecycle lives in the tree.
		f.baseFile = present
		f.headFile = present

		checkout := writeCheckout(t, map[string][2]string{
			"topics/orders.yaml": {"", present}, // absent base, present head
		})

		var out bytes.Buffer
		code := runRun(runArgs("--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if !strings.Contains(out.String(), `"decision":"APPROVE"`) {
			t.Fatalf("governed file-ADD must prove reviewed-deletion -> APPROVE:\n%s", out.String())
		}
	})

	t.Run("present only on base -> file-DELETE (failing polarity)", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.mergePolicy = mergePolicyFileEvents
		f.rulesetBinding = rulesetBindingReviewedDeletion
		f.baseFile = present
		f.headFile = present

		checkout := writeCheckout(t, map[string][2]string{
			"topics/orders.yaml": {present, ""}, // present base, absent head
		})

		var out bytes.Buffer
		code := runRun(runArgs("--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if !strings.Contains(out.String(), `"decision":"REVIEW"`) {
			t.Fatalf("governed file-DELETE must fail reviewed-deletion -> REVIEW:\n%s", out.String())
		}
		if f.approvals != 0 || f.merges != 0 {
			t.Errorf("file-DELETE REVIEW must not approve/merge: approvals=%d merges=%d", f.approvals, f.merges)
		}
	})
}

// TestLiveCheckoutAmbiguousStaysOpaque (REQ-EFE-S03-02, fail-safe) — a clean
// file-event is minted ONLY on unambiguous one-sided presence. An empty-but-present
// side (`{}` / empty bytes) is NOT a delete; presence-unknowable stays opaque →
// REVIEW. No path=="" event from empty bytes (a wrongly-minted delete is fail-OPEN).
func TestLiveCheckoutAmbiguousStaysOpaque(t *testing.T) {
	t.Run("empty-but-present head is NOT a whole-file delete", func(t *testing.T) {
		f := newFakeGitLab(t)
		f.mergePolicy = mergePolicyFileEvents
		f.rulesetBinding = rulesetBindingReviewedDeletion
		f.baseFile = "enabled: true\n"
		f.headFile = ""

		present := "enabled: true\n"
		empty := "" // non-nil pointer below → PRESENT empty bytes, not absent
		checkout := writeCheckoutPresence(t, map[string][2]*string{
			"topics/orders.yaml": {&present, &empty},
		})

		var out bytes.Buffer
		code := runRun(runArgs("--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
		if code != 0 {
			t.Fatalf("exit = %d, want 0\n%s", code, out.String())
		}
		if !strings.Contains(out.String(), `"decision":"REVIEW"`) {
			t.Fatalf("empty-but-present must fail safe to REVIEW (opaque, not a file-delete):\n%s", out.String())
		}
		// The fileEvents onFailure code must NOT fire — that would mean a whole-file
		// DELETE was minted from empty bytes (fail-OPEN).
		if strings.Contains(out.String(), "topic.deletion-needs-review") {
			t.Fatalf("empty-but-present must NOT mint a file-DELETE matched by fileEvents:\n%s", out.String())
		}
	})

	t.Run("presence-unknowable fold stays opaque (no lifecycle mint)", func(t *testing.T) {
		// A checkout that cannot report nil-absent presence (both sides non-nil
		// empty bytes, no distinct presence signal) must NOT clear opacity via a
		// fabricated file-event — fold stays opaque → fail-safe.
		fold, err := foldCheckout(unknowablePresenceCheckout{})
		if err != nil {
			t.Fatalf("foldCheckout: %v", err)
		}
		if !fold.opaque {
			t.Fatalf("presence-unknowable checkout must stay fold.opaque, got %+v", fold)
		}
	})
}

// unknowablePresenceCheckout is a localCheckout whose FileContents returns
// non-nil empty bytes on both sides — content without a nil-absent presence
// signal. OneSidedLifecycle must refuse; Diff is opaque; fold must stay opaque.
type unknowablePresenceCheckout struct{}

func (unknowablePresenceCheckout) ChangedFiles() ([]string, error) {
	return []string{"topics/orders.yaml"}, nil
}

func (unknowablePresenceCheckout) FileContents(string) ([]byte, []byte, error) {
	return []byte{}, []byte{}, nil
}

// TestLiveCheckoutFileEventDoubleRun (REQ-EFE-S03-03) — the live checkout
// file-event population double-runs byte-identical for both polarities.
func TestLiveCheckoutFileEventDoubleRun(t *testing.T) {
	present := "enabled: true\n"
	cases := []struct {
		name  string
		files map[string][2]string
		want  string
	}{
		{"add", map[string][2]string{"topics/orders.yaml": {"", present}}, `"decision":"APPROVE"`},
		{"delete", map[string][2]string{"topics/orders.yaml": {present, ""}}, `"decision":"REVIEW"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := func() string {
				f := newFakeGitLab(t)
				f.mergePolicy = mergePolicyFileEvents
				f.rulesetBinding = rulesetBindingReviewedDeletion
				f.baseFile = present
				f.headFile = present
				checkout := writeCheckout(t, tc.files)
				var out bytes.Buffer
				code := runRun(runArgs("--checkout", checkout), env("tok"), fixedClock(), &out, &out, f.factory())
				if code != 0 {
					t.Fatalf("exit = %d, want 0\n%s", code, out.String())
				}
				return out.String()
			}
			first := run()
			second := run()
			if first != second {
				t.Fatalf("live file-event path not byte-identical:\n--- first ---\n%s\n--- second ---\n%s", first, second)
			}
			if !strings.Contains(first, tc.want) {
				t.Fatalf("expected %s:\n%s", tc.want, first)
			}
		})
	}
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
