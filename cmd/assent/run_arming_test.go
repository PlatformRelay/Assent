package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// REQ-E7-S03-04 (ADR-0017 §4): a controlling authorization fact past max_age at arming
// time blocks approve/merge writes even when evaluation at t₀ would APPROVE.
func TestRunExpiredFactBlocksArming(t *testing.T) {
	evalAt := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	prov := fakeOwnerProvider(t, evalAt)

	f := newFakeGitLab(t)
	f.mergePolicy = mergePolicyOwnership
	f.rulesetBinding = rulesetBindingOwnership
	f.config = configOwnerHTTP(prov.URL)
	f.providerDecls = map[string]string{"owner": ownerDeclarationJSON}
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	// First clock() → evaluation instant (fact fresh). Second → after 1h maxAge expires.
	clock := advancingClock(evalAt, evalAt.Add(2*time.Hour))

	var out bytes.Buffer
	code := runRun(runArgs("--arm", "--config", ".assent/config.yaml"), env("tok"), clock, &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (advisory)\n%s", code, out.String())
	}
	body := out.String()
	if !strings.Contains(body, `"decision":"APPROVE"`) {
		t.Fatalf("evaluation at t₀ with fresh fact must APPROVE:\n%s", body)
	}
	if f.approvals != 0 || f.merges != 0 {
		t.Errorf("expired controlling fact must block arming writes: approvals=%d merges=%d", f.approvals, f.merges)
	}
	if !strings.Contains(body, "advisory-only") {
		t.Errorf("expected arming refusal summary:\n%s", body)
	}
}

// D-134 / audit finding ARM-04: `--arm` is ADVISORY-ONLY. Since c05cde0 (E4-S06)
// the forge-probed PreconditionProbe gates approve/merge, so the flag's ONLY
// remaining observable effect is the `arm=<bool>` token in the run summary — and
// that token had zero discriminating coverage: hardcoding
// `summarize(result.Decision, true, …)` in orchestrate left the whole suite green,
// which is why the flag rotted into a documented-but-false safety gate unnoticed.
//
// This test pins BOTH polarities of the token, so either constant substitution
// reds exactly one subtest, AND pins the advisory semantics the docs now promise:
// the write outcome is IDENTICAL with and without the flag.
//
// It is deliberately NOT a red-first test — cfg.arm is already threaded to
// summarize, so there is no implementation to add. Its proof obligation is
// mutation-kill, verified by hand at authoring time:
//
//	summarize(result.Decision, true, …)  → reds without_flag_reports_arm_false
//	summarize(result.Decision, false, …) → reds with_flag_reports_arm_true
func TestRunArmFlagIsAdvisoryOnly(t *testing.T) {
	// A forge-probed-eligible APPROVE fixture: the arming preconditions ARE met,
	// so writes happen and the flag is the only variable under test.
	runFixture := func(t *testing.T, extra ...string) (summary string, approvals, merges int) {
		t.Helper()
		f := newFakeGitLab(t)
		f.baseFile = "partitions: 12\n"
		f.headFile = "partitions: 24\n"

		var out bytes.Buffer
		if code := runRun(runArgs(extra...), env("tok"), fixedClock(), &out, &out, f.factory()); code != 0 {
			t.Fatalf("runRun%v exit = %d, want 0\n%s", extra, code, out.String())
		}
		body := out.String()
		if !strings.Contains(body, `"decision":"APPROVE"`) {
			t.Fatalf("fixture must APPROVE for the arming path to be exercised:\n%s", body)
		}
		lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
		return lines[len(lines)-1], f.approvals, f.merges
	}

	t.Run("without_flag_reports_arm_false", func(t *testing.T) {
		summary, approvals, merges := runFixture(t)
		if !strings.Contains(summary, "arm=false") {
			t.Errorf("summary must carry arm=false when --arm is absent, got %q", summary)
		}
		if strings.Contains(summary, "arm=true") {
			t.Errorf("summary must not claim arm=true when --arm is absent, got %q", summary)
		}
		// The documented consequence: absent --arm is NOT advisory. The forge-probed
		// precondition is met, so the writes happen anyway (REQ-E4-S06-05).
		if approvals != 1 || merges != 1 {
			t.Errorf("forge-probed arming is the gate, not --arm: approvals=%d merges=%d, want 1/1", approvals, merges)
		}
	})

	t.Run("with_flag_reports_arm_true", func(t *testing.T) {
		summary, approvals, merges := runFixture(t, "--arm")
		if !strings.Contains(summary, "arm=true") {
			t.Errorf("summary must carry arm=true when --arm is passed, got %q", summary)
		}
		if strings.Contains(summary, "arm=false") {
			t.Errorf("summary must not claim arm=false when --arm is passed, got %q", summary)
		}
		if approvals != 1 || merges != 1 {
			t.Errorf("--arm must not change the write outcome: approvals=%d merges=%d, want 1/1", approvals, merges)
		}
	})

	// The flag is cosmetic: everything in the summary EXCEPT the arm token, and the
	// whole write outcome, is byte-identical across the two runs. This is the claim
	// docs/usage/cli.md now makes, pinned rather than asserted in prose.
	t.Run("flag_changes_nothing_but_the_token", func(t *testing.T) {
		off, offApprovals, offMerges := runFixture(t)
		on, onApprovals, onMerges := runFixture(t, "--arm")
		if got, want := strings.Replace(on, "arm=true", "arm=false", 1), off; got != want {
			t.Errorf("--arm changed the summary beyond its own token:\n with --arm (normalised) %q\n without --arm          %q", got, want)
		}
		if offApprovals != onApprovals || offMerges != onMerges {
			t.Errorf("--arm changed the write outcome: without=%d/%d with=%d/%d",
				offApprovals, offMerges, onApprovals, onMerges)
		}
	})
}

func advancingClock(first, second time.Time) runClock {
	calls := 0
	return func() time.Time {
		calls++
		if calls == 1 {
			return first
		}
		return second
	}
}
