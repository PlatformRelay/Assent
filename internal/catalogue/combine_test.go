package catalogue_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/catalogue"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

func listEntry(root, ptr string) policy.Entry {
	return policy.Entry{Mode: "list", Root: root, Identity: policy.Identity{Pointer: ptr}}
}

func mpWithEntry(label string, e policy.Entry) *policy.MergePolicy {
	mp := &policy.MergePolicy{}
	mp.Spec.Entries = map[string]policy.Entry{label: e}
	return mp
}

func TestCombinePoliciesFailsClosedOnConflictingEntries(t *testing.T) {
	t.Run("conflicting label configs error (fail-closed)", func(t *testing.T) {
		docs := []*policy.MergePolicy{
			mpWithEntry("catalog-service", listEntry("/services", "/name")),
			mpWithEntry("catalog-service", listEntry("/workloads", "/name")),
		}
		if _, err := catalogue.CombinePolicies(docs); err == nil {
			t.Fatal("expected CombinePolicies to fail closed on conflicting entry configs")
		}
	})

	t.Run("identical label configs merge cleanly", func(t *testing.T) {
		docs := []*policy.MergePolicy{
			mpWithEntry("catalog-service", listEntry("/services", "/name")),
			mpWithEntry("catalog-service", listEntry("/services", "/name")),
		}
		out, err := catalogue.CombinePolicies(docs)
		if err != nil {
			t.Fatalf("identical entry configs must merge: %v", err)
		}
		if out == nil || len(out.Spec.Entries) != 1 {
			t.Fatalf("expected one merged entry, got %+v", out)
		}
	})

	t.Run("all-nil docs yield nil policy", func(t *testing.T) {
		out, err := catalogue.CombinePolicies([]*policy.MergePolicy{nil, nil})
		if err != nil {
			t.Fatalf("all-nil combine must not error: %v", err)
		}
		if out != nil {
			t.Fatalf("all-nil combine must yield nil policy, got %+v", out)
		}
	})
}
