package builtin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
	"github.com/PlatformRelay/assent/internal/provider/builtin"
)

// fixedAsOf is the host-pinned evaluation instant — no wall clock in assertions.
var fixedAsOf = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

func groupsQuery(user string) provider.FactQuery {
	return provider.FactQuery{
		APIVersion: provider.APIVersion,
		Kind:       provider.KindFactQuery,
		QueryID:    "q-e5-s06-1",
		AsOf:       fixedAsOf,
		Subject:    provider.Subject{Kind: "user", ID: user},
		Outputs:    []string{"groups"},
	}
}

// stringList coerces a JSON-decoded fact value ([]any of strings, or []string)
// into a plain []string for assertions.
func stringList(t *testing.T, v any) []string {
	t.Helper()
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, len(x))
		for i, el := range x {
			s, ok := el.(string)
			if !ok {
				t.Fatalf("list[%d] type = %T, want string", i, el)
			}
			out[i] = s
		}
		return out
	case nil:
		return nil
	default:
		t.Fatalf("value type = %T, want string list", v)
		return nil
	}
}

// TestBuiltinGitlabGroups — REQ-E5-S06-01: hermetic fake forge → author.groups resolved.
func TestBuiltinGitlabGroups(t *testing.T) {
	fake := &builtin.FakeForgeGroups{
		Membership: map[string][]string{
			"alice": {"reviewers", "platform-team"}, // unsorted on purpose
		},
	}
	q := groupsQuery("alice")

	result := builtin.ResolveGitlabGroups(context.Background(), fake, q)
	fact, ok := result.Facts["groups"]
	if !ok {
		t.Fatal("groups fact must be present (never omit a requested key)")
	}
	if fact.State != provider.StateResolved {
		t.Fatalf("groups.state = %q (%s), want resolved", fact.State, fact.Reason)
	}
	got := stringList(t, fact.Value)
	want := []string{"platform-team", "reviewers"} // sorted for determinism
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("groups.value = %#v, want %#v", got, want)
	}
	if fact.ExpiresAt == nil || !fact.ExpiresAt.Equal(fixedAsOf.Add(time.Hour)) {
		t.Fatalf("expiresAt = %v, want asOf+1h", fact.ExpiresAt)
	}
	if fact.Declaration.Type != "principal" || fact.Declaration.Cardinality != "set" {
		t.Fatalf("declaration = %+v, want principal/set (membership)", fact.Declaration)
	}

	// Example config type strings wire through the builtin registry.
	for _, typ := range []string{builtin.TypeGitlabGroups, builtin.TypeForgeGroups} {
		if !builtin.IsForgeGroupsType(typ) {
			t.Fatalf("IsForgeGroupsType(%q) = false, want true", typ)
		}
	}
	if builtin.IsForgeGroupsType("builtin/repo-file") {
		t.Fatal("repo-file must not register as forge-groups")
	}

	// Determinism: double-run yields identical states + values.
	again := builtin.ResolveGitlabGroups(context.Background(), fake, q)
	a, b := result.Facts["groups"], again.Facts["groups"]
	if a.State != b.State {
		t.Fatalf("double-run state drift: %q vs %q", a.State, b.State)
	}
	av, bv := stringList(t, a.Value), stringList(t, b.Value)
	if len(av) != len(bv) || (len(av) == 2 && (av[0] != bv[0] || av[1] != bv[1])) {
		t.Fatalf("double-run value drift: %#v vs %#v", av, bv)
	}
}

// TestBuiltinGitlabGroupsFailClosed — REQ-E5-S06-02: unknown/missing membership
// → non-resolved (never empty-resolved allow).
func TestBuiltinGitlabGroupsFailClosed(t *testing.T) {
	t.Run("unknown_user_unavailable", func(t *testing.T) {
		fake := &builtin.FakeForgeGroups{
			Membership: map[string][]string{
				// alice absent — unknown membership, not "empty groups"
			},
		}
		result := builtin.ResolveGitlabGroups(context.Background(), fake, groupsQuery("alice"))
		fact, ok := result.Facts["groups"]
		if !ok {
			t.Fatal("groups fact must be present")
		}
		if fact.State == provider.StateResolved {
			t.Fatalf("unknown membership must not resolve (got resolved value %#v) — empty-resolved would allow fail-open paths", fact.Value)
		}
		if fact.State != provider.StateUnavailable {
			t.Fatalf("groups.state = %q, want unavailable", fact.State)
		}
		if fact.Value != nil {
			t.Fatalf("non-resolved fact must drop value, got %#v", fact.Value)
		}
	})

	t.Run("client_error_unavailable", func(t *testing.T) {
		fake := &builtin.FakeForgeGroups{
			Membership: map[string][]string{"alice": {"platform-team"}},
			Err:        errors.New("forge unreachable"),
		}
		result := builtin.ResolveGitlabGroups(context.Background(), fake, groupsQuery("alice"))
		fact := result.Facts["groups"]
		if fact.State != provider.StateUnavailable {
			t.Fatalf("transport error state = %q, want unavailable", fact.State)
		}
		if fact.Value != nil {
			t.Fatalf("non-resolved must drop value, got %#v", fact.Value)
		}
	})

	t.Run("known_empty_is_resolved_empty", func(t *testing.T) {
		// Present key with empty set is known membership (zero groups), distinct
		// from unknown/missing — only the latter must stay non-resolved.
		fake := &builtin.FakeForgeGroups{
			Membership: map[string][]string{"bob": {}},
		}
		result := builtin.ResolveGitlabGroups(context.Background(), fake, groupsQuery("bob"))
		fact := result.Facts["groups"]
		if fact.State != provider.StateResolved {
			t.Fatalf("known-empty membership state = %q (%s), want resolved", fact.State, fact.Reason)
		}
		got := stringList(t, fact.Value)
		if len(got) != 0 {
			t.Fatalf("known-empty value = %#v, want empty list", got)
		}
	})
}
