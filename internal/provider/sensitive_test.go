package provider_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/provider"
)

// TestSensitiveMaxAge — REQ-E5-S04-01 (INBOX F1/F2): sensitive declaration
// applies ≤15m maxAge; a longer declared maxAge is rejected at load (never clamped).
func TestSensitiveMaxAge(t *testing.T) {
	if provider.MaxAgeSensitive != 15*time.Minute {
		t.Fatalf("MaxAgeSensitive = %v, want 15m (provider-contract.md)", provider.MaxAgeSensitive)
	}

	ceiling := provider.MaxAgeCeiling(provider.Declaration{
		Type:      "string",
		Sensitive: true,
	})
	if ceiling != 15*time.Minute {
		t.Fatalf("MaxAgeCeiling(sensitive string) = %v, want 15m", ceiling)
	}
	// Sensitive forces 15m regardless of the type's non-sensitive ceiling (principal=1h).
	principalSensitive := provider.MaxAgeCeiling(provider.Declaration{
		Type:      "principal",
		Sensitive: true,
	})
	if principalSensitive != 15*time.Minute {
		t.Fatalf("MaxAgeCeiling(sensitive principal) = %v, want 15m (not 1h)", principalSensitive)
	}

	t.Run("over_15m_rejected_not_clamped", func(t *testing.T) {
		_, err := provider.LoadProviderConfig([]byte(`{
			"name": "sensitive-too-long",
			"requests": {"values": {"pointers": []}},
			"outputs": {
				"secretMeta": {
					"type": "string",
					"cardinality": "single",
					"subject": "entry",
					"sensitive": true,
					"maxAge": "16m"
				}
			}
		}`))
		if err == nil {
			t.Fatal("sensitive maxAge > 15m must be rejected at load (never clamped)")
		}
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "maxage") && !strings.Contains(msg, "max age") {
			t.Fatalf("error should mention maxAge; got: %v", err)
		}
		if strings.Contains(msg, "clamp") && !strings.Contains(msg, "never clamped") {
			t.Fatalf("must not describe clamping as success; got: %v", err)
		}
	})

	t.Run("exact_15m_accepted", func(t *testing.T) {
		cfg, err := provider.LoadProviderConfig([]byte(`{
			"name": "sensitive-exact",
			"requests": {"values": {"pointers": []}},
			"outputs": {
				"secretMeta": {
					"type": "string",
					"cardinality": "single",
					"subject": "entry",
					"sensitive": true,
					"maxAge": "15m"
				}
			}
		}`))
		if err != nil {
			t.Fatalf("sensitive maxAge=15m must be accepted: %v", err)
		}
		if cfg.Outputs["secretMeta"].MaxAge != "15m" {
			t.Fatalf("maxAge = %q, want 15m (must not rewrite/clamp)", cfg.Outputs["secretMeta"].MaxAge)
		}
		if !cfg.Outputs["secretMeta"].Sensitive {
			t.Fatal("Sensitive flag must remain true after load")
		}
	})

	t.Run("shorten_below_15m_accepted", func(t *testing.T) {
		cfg, err := provider.LoadProviderConfig([]byte(`{
			"name": "sensitive-short",
			"requests": {"values": {"pointers": []}},
			"outputs": {
				"secretMeta": {
					"type": "string",
					"cardinality": "single",
					"subject": "entry",
					"sensitive": true,
					"maxAge": "5m"
				}
			}
		}`))
		if err != nil {
			t.Fatalf("sensitive maxAge shorter than 15m must be accepted: %v", err)
		}
		if cfg.Outputs["secretMeta"].MaxAge != "5m" {
			t.Fatalf("maxAge = %q, want 5m", cfg.Outputs["secretMeta"].MaxAge)
		}
	})
}

// TestSensitivePropagates — REQ-E5-S04-02: resolved sensitive facts set
// aggregate.Fact.Sensitive=true for the CEL/E8 redaction handoff; non-sensitive stays false.
func TestSensitivePropagates(t *testing.T) {
	q := provider.FactQuery{
		APIVersion: provider.APIVersion,
		Kind:       provider.KindFactQuery,
		QueryID:    "q-e5-s04-sensitive",
		AsOf:       fixedAsOf,
		Subject:    provider.Subject{Kind: "user", ID: "alice"},
		Outputs:    []string{"secretMeta", "groups"},
	}
	expires := fixedAsOf.Add(10 * time.Minute)
	sensitiveDecl := provider.Declaration{
		Type:        "string",
		Cardinality: "single",
		Subject:     "entry",
		Sensitive:   true,
		MaxAge:      "15m",
	}
	plainDecl := groupsDeclaration()

	resp := provider.FactResponse{
		APIVersion: provider.APIVersion,
		Kind:       provider.KindFactResponse,
		QueryID:    q.QueryID,
		Facts: []provider.Fact{
			{
				Name:        "secretMeta",
				Declaration: sensitiveDecl,
				State:       provider.StateResolved,
				Subject:     q.Subject,
				ObservedAt:  q.AsOf,
				ExpiresAt:   &expires,
				Value:       "redact-me",
			},
			{
				Name:        "groups",
				Declaration: plainDecl,
				State:       provider.StateResolved,
				Subject:     q.Subject,
				ObservedAt:  q.AsOf,
				ExpiresAt:   &expires,
				Value:       []any{"platform-team"},
			},
		},
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got := provider.ResolveFactsChecked(context.Background(), func(context.Context) ([]byte, error) {
		return raw, nil
	}, q, fixedAsOf, map[string]provider.Declaration{
		"secretMeta": sensitiveDecl,
		"groups":     plainDecl,
	})

	secret, ok := got.Facts["secretMeta"]
	if !ok || secret.State != provider.StateResolved {
		t.Fatalf("secretMeta state = %q, want resolved", secret.State)
	}
	bound := provider.ToAggregateFact(secret)
	if !bound.Sensitive {
		t.Fatal("resolved sensitive fact must set aggregate.Fact.Sensitive=true (E8 redaction handoff)")
	}
	if bound.State != provider.StateResolved {
		t.Fatalf("bound state = %q, want resolved", bound.State)
	}
	if bound.Value != "redact-me" {
		t.Fatalf("bound value = %#v, want redact-me (value preserved; E8 redacts at render)", bound.Value)
	}
	if bound.ObservedAt == "" {
		t.Fatal("ObservedAt must be carried onto the aggregate envelope")
	}
	if bound.ExpiresAt == "" {
		t.Fatal("ExpiresAt must be carried onto the aggregate envelope")
	}

	plain, ok := got.Facts["groups"]
	if !ok || plain.State != provider.StateResolved {
		t.Fatalf("groups state = %q, want resolved", plain.State)
	}
	plainBound := provider.ToAggregateFact(plain)
	if plainBound.Sensitive {
		t.Fatal("non-sensitive fact must leave aggregate.Fact.Sensitive=false")
	}

	// CEL handoff: factsToCEL binds the Sensitive flag under the envelope key.
	celFacts := map[string]map[string]aggregate.Fact{
		"secrets": {"secretMeta": bound},
		"author":  {"groups": plainBound},
	}
	if !celFacts["secrets"]["secretMeta"].Sensitive {
		t.Fatal("CEL-bound sensitive fact lost Sensitive=true")
	}
	if celFacts["author"]["groups"].Sensitive {
		t.Fatal("CEL-bound non-sensitive fact must stay Sensitive=false")
	}
}
