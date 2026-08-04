package provider_test

import (
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
)

// TestBuildQueryMinimized — REQ-E5-S02-01: only declared pointers appear in FactQuery;
// undeclared change content (incl. secret refs) never enters the serialized request.
func TestBuildQueryMinimized(t *testing.T) {
	cfg, err := provider.LoadProviderConfig([]byte(`{
		"name": "toy-groups",
		"requests": {"values": {"pointers": ["/owner"]}},
		"outputs": {
			"groups": {
				"type": "string",
				"cardinality": "set",
				"subject": "user",
				"sensitive": false,
				"maxAge": "1h"
			}
		}
	}`))
	if err != nil {
		t.Fatalf("config load: %v", err)
	}

	change := map[string]provider.ValueChange{
		"/owner":     {Old: "team-a", New: "team-b"},
		"/secretRef": {Old: "vault/old", New: "vault/new"},
	}
	asOf := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	q := provider.BuildQuery(cfg, "q-min-1", asOf, provider.Subject{Kind: "user", ID: "alice"}, []string{"groups"}, change)

	if len(q.Projections.Values) != 1 || q.Projections.Values[0].Pointer != "/owner" {
		t.Fatalf("projections = %+v, want exactly the declared /owner", q.Projections.Values)
	}
	if q.Projections.Values[0].Old != "team-a" || q.Projections.Values[0].New != "team-b" {
		t.Fatalf("projection values = %+v, want old=team-a new=team-b", q.Projections.Values[0])
	}

	raw, err := jsonMarshalCompact(q)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, undeclared := range []string{"/secretRef", "vault/old", "vault/new"} {
		if strings.Contains(string(raw), undeclared) {
			t.Errorf("undeclared change content %q leaked into the request:\n%s", undeclared, raw)
		}
	}
	// Untouched declared pointers must not invent projections.
	q2 := provider.BuildQuery(cfg, "q-min-2", asOf, provider.Subject{Kind: "user", ID: "alice"}, []string{"groups"}, map[string]provider.ValueChange{
		"/secretRef": {Old: "a", New: "b"},
	})
	if len(q2.Projections.Values) != 0 {
		t.Fatalf("untouched declared pointer must not project; got %+v", q2.Projections.Values)
	}
}

// TestFullContentCapabilityGate — REQ-E5-S02-02: fullContent without
// trusted-full-content is refused at load; with the capability it loads.
func TestFullContentCapabilityGate(t *testing.T) {
	t.Run("refused_without_capability", func(t *testing.T) {
		_, err := provider.LoadProviderConfig([]byte(`{
			"name": "greedy",
			"requests": {"fullContent": true},
			"outputs": {
				"groups": {
					"type": "string",
					"cardinality": "set",
					"subject": "user",
					"sensitive": false,
					"maxAge": "1h"
				}
			}
		}`))
		if err == nil {
			t.Fatal("config load accepted fullContent without trusted-full-content — must refuse")
		}
		if !strings.Contains(err.Error(), provider.CapabilityFullContent) {
			t.Fatalf("error should name the capability; got: %v", err)
		}
	})

	t.Run("allowed_with_capability", func(t *testing.T) {
		_, err := provider.LoadProviderConfig([]byte(`{
			"name": "trusted",
			"requests": {"fullContent": true},
			"capabilities": ["trusted-full-content"],
			"outputs": {
				"groups": {
					"type": "string",
					"cardinality": "set",
					"subject": "user",
					"sensitive": false,
					"maxAge": "1h"
				}
			}
		}`))
		if err != nil {
			t.Fatalf("config load refused despite explicit capability: %v", err)
		}
	})
}
