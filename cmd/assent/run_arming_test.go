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
