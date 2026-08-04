package builtin_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
	"github.com/PlatformRelay/assent/internal/provider/builtin"
)

func resourceOwnerQuery(resourceID string) provider.FactQuery {
	return provider.FactQuery{
		APIVersion: provider.APIVersion,
		Kind:       provider.KindFactQuery,
		QueryID:    "q-e5-s08-1",
		AsOf:       fixedAsOf,
		Subject:    provider.Subject{Kind: "entry", ID: resourceID},
		Outputs:    []string{builtin.OutputOwner},
	}
}

// TestResourceOwnerResolves — REQ-E5-S08-01: known resource → owner resolved.
func TestResourceOwnerResolves(t *testing.T) {
	fake := &builtin.FakeResourceOwner{
		Owners: map[string]string{
			"kafka-topic:orders.events.v1": "team-orders",
		},
	}
	q := resourceOwnerQuery("kafka-topic:orders.events.v1")

	result := builtin.ResolveResourceOwner(context.Background(), fake, q)
	fact, ok := result.Facts[builtin.OutputOwner]
	if !ok {
		t.Fatal("owner fact must be present (never omit a requested key)")
	}
	if fact.State != provider.StateResolved {
		t.Fatalf("owner.state = %q (%s), want resolved", fact.State, fact.Reason)
	}
	if fact.Value != "team-orders" {
		t.Fatalf("owner.value = %#v, want team-orders", fact.Value)
	}
	if fact.ExpiresAt == nil || !fact.ExpiresAt.Equal(fixedAsOf.Add(24*time.Hour)) {
		t.Fatalf("expiresAt = %v, want asOf+24h", fact.ExpiresAt)
	}
	if fact.Declaration.Type != "string" || fact.Declaration.Cardinality != "single" {
		t.Fatalf("declaration = %+v, want string/single (registry lookup)", fact.Declaration)
	}
	if fact.Declaration.Subject != "entry" {
		t.Fatalf("declaration.subject = %q, want entry", fact.Declaration.Subject)
	}

	if !builtin.IsResourceOwnerType(builtin.TypeResourceOwner) {
		t.Fatalf("IsResourceOwnerType(%q) = false, want true", builtin.TypeResourceOwner)
	}
	if builtin.IsResourceOwnerType(builtin.TypeRepoFile) {
		t.Fatal("repo-file must not register as resource-owner")
	}

	// Fixture map loaded from testdata resolves the same owner.
	t.Run("fixture_map", func(t *testing.T) {
		fsys := os.DirFS(filepath.Join("testdata", "resource-owner"))
		client, err := builtin.LoadResourceOwnerMap(fsys, "owners.yaml")
		if err != nil {
			t.Fatal(err)
		}
		q := resourceOwnerQuery("kafka-topic:payments.settled.v2")
		result := builtin.ResolveResourceOwner(context.Background(), client, q)
		fact := result.Facts[builtin.OutputOwner]
		if fact.State != provider.StateResolved {
			t.Fatalf("fixture owner.state = %q (%s), want resolved", fact.State, fact.Reason)
		}
		if fact.Value != "team-payments" {
			t.Fatalf("fixture owner.value = %#v, want team-payments", fact.Value)
		}
	})

	// Determinism: double-run yields identical states + values.
	again := builtin.ResolveResourceOwner(context.Background(), fake, q)
	a, b := result.Facts[builtin.OutputOwner], again.Facts[builtin.OutputOwner]
	if a.State != b.State || a.Value != b.Value {
		t.Fatalf("double-run drift: %+v vs %+v", a, b)
	}
}

// TestResourceOwnerUnknownFailClosed — REQ-E5-S08-02: unknown resource → unavailable,
// never resolved with empty owner that would satisfy ownership.
func TestResourceOwnerUnknownFailClosed(t *testing.T) {
	t.Run("unknown_resource_unavailable", func(t *testing.T) {
		fake := &builtin.FakeResourceOwner{
			Owners: map[string]string{
				// orders topic absent — unknown ownership, not "empty owner"
			},
		}
		result := builtin.ResolveResourceOwner(context.Background(), fake, resourceOwnerQuery("kafka-topic:orders.events.v1"))
		fact, ok := result.Facts[builtin.OutputOwner]
		if !ok {
			t.Fatal("owner fact must be present")
		}
		if fact.State == provider.StateResolved {
			t.Fatalf("unknown ownership must not resolve (got resolved value %#v) — empty-resolved would allow fail-open paths", fact.Value)
		}
		if fact.State != provider.StateUnavailable {
			t.Fatalf("owner.state = %q, want unavailable", fact.State)
		}
		if fact.Value != nil {
			t.Fatalf("non-resolved fact must drop value, got %#v", fact.Value)
		}
	})

	t.Run("fixture_map_unknown", func(t *testing.T) {
		fsys := os.DirFS(filepath.Join("testdata", "resource-owner"))
		client, err := builtin.LoadResourceOwnerMap(fsys, "owners.yaml")
		if err != nil {
			t.Fatal(err)
		}
		result := builtin.ResolveResourceOwner(context.Background(), client, resourceOwnerQuery("kafka-topic:unknown.topic.v1"))
		fact := result.Facts[builtin.OutputOwner]
		if fact.State == provider.StateResolved {
			t.Fatalf("unknown resource must not resolve empty owner; value=%#v", fact.Value)
		}
		if fact.State != provider.StateUnavailable {
			t.Fatalf("owner.state = %q, want unavailable", fact.State)
		}
		if fact.Value != nil {
			t.Fatalf("non-resolved must drop value, got %#v", fact.Value)
		}
	})

	t.Run("known_empty_is_resolved_empty", func(t *testing.T) {
		// Present key with empty owner is known registry data (zero owner), distinct
		// from unknown/missing — only the latter must stay non-resolved.
		fake := &builtin.FakeResourceOwner{
			Owners: map[string]string{"kafka-topic:unowned.v1": ""},
		}
		result := builtin.ResolveResourceOwner(context.Background(), fake, resourceOwnerQuery("kafka-topic:unowned.v1"))
		fact := result.Facts[builtin.OutputOwner]
		if fact.State != provider.StateResolved {
			t.Fatalf("known-empty owner state = %q (%s), want resolved", fact.State, fact.Reason)
		}
		if fact.Value != "" {
			t.Fatalf("known-empty value = %#v, want empty string", fact.Value)
		}
	})
}
