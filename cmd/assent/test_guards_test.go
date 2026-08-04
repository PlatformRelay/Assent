package main

// test_guards_test.go covers the two D-060 FAIL-CLOSED guards the whole-pack replay
// depends on for safety — combinePolicies' conflicting-entries error and
// selectBindingForTest's "bindings differ beyond environment+threshold" error. The
// happy-path corpus (test_corpus_test.go) exercises only the success branches; these
// tests pin the ERROR returns directly, so deleting a guard (turning a masking/
// permissive collapse silent) fails here — mirroring run.go's own fail-closed
// selectBinding test (policy_test.go).

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// listEntry is the entry config every service-catalog rule doc authors identically;
// combinePolicies MERGES them only when they AGREE.
func listEntry(root, ptr string) policy.Entry {
	return policy.Entry{Mode: "list", Root: root, Identity: policy.Identity{Pointer: ptr}}
}

func mpWithEntry(label string, e policy.Entry) *policy.MergePolicy {
	mp := &policy.MergePolicy{}
	mp.Spec.Entries = map[string]policy.Entry{label: e}
	return mp
}

// TestCombinePoliciesFailsClosedOnConflictingEntries pins the fail-closed branch:
// two docs assigning the SAME entry label DIFFERENT configs is a pack defect that
// ERRORS (never a silent pick of one, which could diff a file under the wrong
// collection root). The agree-path is asserted alongside so the guard is not merely
// "always errors".
func TestCombinePoliciesFailsClosedOnConflictingEntries(t *testing.T) {
	t.Run("conflicting label configs error (fail-closed)", func(t *testing.T) {
		docs := []*policy.MergePolicy{
			mpWithEntry("catalog-service", listEntry("/services", "/name")),
			mpWithEntry("catalog-service", listEntry("/workloads", "/name")), // same label, different root
		}
		if _, err := combinePolicies(docs); err == nil {
			t.Fatal("expected combinePolicies to fail closed on conflicting entry configs, got nil")
		}
	})

	t.Run("identical label configs merge cleanly (agree path)", func(t *testing.T) {
		docs := []*policy.MergePolicy{
			mpWithEntry("catalog-service", listEntry("/services", "/name")),
			mpWithEntry("catalog-service", listEntry("/services", "/name")),
		}
		out, err := combinePolicies(docs)
		if err != nil {
			t.Fatalf("identical entry configs must merge, got %v", err)
		}
		if out == nil || len(out.Spec.Entries) != 1 {
			t.Fatalf("expected one merged entry, got %+v", out)
		}
	})

	t.Run("all-nil docs yield a nil policy (caller skips)", func(t *testing.T) {
		out, err := combinePolicies([]*policy.MergePolicy{nil, nil})
		if err != nil {
			t.Fatalf("all-nil combine must not error, got %v", err)
		}
		if out != nil {
			t.Fatalf("all-nil combine must yield nil policy, got %+v", out)
		}
	})
}

// TestSelectBindingForTestFailsClosedBeyondThreshold pins the fail-closed branch: a
// multi-binding document is collapsible ONLY when the bindings differ solely in
// environment + risk.threshold; a difference in class, packs, or require[] could FLIP
// a decision, so it must ERROR (never a silent permissive collapse). The neutral-split
// success + strictest-threshold selection are asserted so the guard is exercised in
// both directions.
func TestSelectBindingForTestFailsClosedBeyondThreshold(t *testing.T) {
	base := func() policy.Binding {
		return policy.Binding{Class: "c", Environment: "dev", Packs: []string{"p"}, Risk: policy.Risk{Threshold: 10}, Require: []string{"a", "b"}}
	}

	failClosed := map[string]policy.Binding{
		"require differs": func() policy.Binding { b := base(); b.Environment = "prod"; b.Require = []string{"a"}; return b }(),
		"class differs":   func() policy.Binding { b := base(); b.Environment = "prod"; b.Class = "other"; return b }(),
		"packs differ":    func() policy.Binding { b := base(); b.Environment = "prod"; b.Packs = []string{"q"}; return b }(),
	}
	for name, second := range failClosed {
		second := second
		t.Run("fail-closed: "+name, func(t *testing.T) {
			rb := &policy.RulesetBinding{Bindings: []policy.Binding{base(), second}}
			if _, err := selectBindingForTest(rb); err == nil {
				t.Fatalf("%s: expected fail-closed error, got nil (a permissive/masking collapse)", name)
			}
		})
	}

	t.Run("neutral split collapses to the STRICTEST (lowest-threshold) binding", func(t *testing.T) {
		dev := base() // threshold 10
		prod := base()
		prod.Environment = "prod"
		prod.Risk.Threshold = 4 // strictest
		rb := &policy.RulesetBinding{Bindings: []policy.Binding{dev, prod}}
		got, err := selectBindingForTest(rb)
		if err != nil {
			t.Fatalf("environment+threshold-only split must collapse, got %v", err)
		}
		if got.Risk.Threshold != 4 || got.Environment != "prod" {
			t.Fatalf("expected the strictest (threshold 4, prod) binding, got threshold=%d env=%s", got.Risk.Threshold, got.Environment)
		}
	})

	t.Run("single binding is used directly", func(t *testing.T) {
		rb := &policy.RulesetBinding{Bindings: []policy.Binding{base()}}
		got, err := selectBindingForTest(rb)
		if err != nil || got == nil {
			t.Fatalf("single binding must be used directly, got err=%v", err)
		}
	})

	t.Run("zero bindings fail closed", func(t *testing.T) {
		if _, err := selectBindingForTest(&policy.RulesetBinding{}); err == nil {
			t.Fatal("zero bindings must fail closed, got nil")
		}
	})
}
