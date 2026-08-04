package provider_test

import (
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
)

// TestMaxAgeOmitRejected — REQ-E5-S02-03: omitting maxAge is a load-time error
// (not a silent fill-in and not a silent "no limit") per provider-contract.md.
func TestMaxAgeOmitRejected(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{
			name: "field_absent",
			raw: `{
				"name": "no-maxage",
				"requests": {"values": {"pointers": []}},
				"outputs": {
					"groups": {
						"type": "principal",
						"cardinality": "set",
						"subject": "user",
						"sensitive": false
					}
				}
			}`,
		},
		{
			name: "empty_string",
			raw: `{
				"name": "empty-maxage",
				"requests": {"values": {"pointers": []}},
				"outputs": {
					"groups": {
						"type": "principal",
						"cardinality": "set",
						"subject": "user",
						"sensitive": false,
						"maxAge": ""
					}
				}
			}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := provider.LoadProviderConfig([]byte(tc.raw))
			if err == nil {
				t.Fatal("omitted maxAge must be a load-time error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "maxage") {
				t.Fatalf("error should mention maxAge; got: %v", err)
			}
		})
	}
}

// TestMaxAgeExceedRejected — REQ-E5-S02-03: exceeding the type default / sensitive
// 15m / 24h global cap is rejected at load (never clamped). Table pinned to
// docs/planning/provider-contract.md.
func TestMaxAgeExceedRejected(t *testing.T) {
	// Ceiling table from provider-contract.md (host validation ceilings).
	ceilings := []struct {
		typ       string
		sensitive bool
		ceiling   time.Duration
	}{
		{typ: "principal", sensitive: false, ceiling: time.Hour},
		{typ: "boolean", sensitive: false, ceiling: time.Hour},
		{typ: "string", sensitive: false, ceiling: 24 * time.Hour},
		{typ: "integer", sensitive: false, ceiling: 24 * time.Hour},
		{typ: "string", sensitive: true, ceiling: 15 * time.Minute},
		{typ: "principal", sensitive: true, ceiling: 15 * time.Minute},
	}

	for _, row := range ceilings {
		t.Run("ceiling_"+row.typ+sensitiveSuffix(row.sensitive), func(t *testing.T) {
			got := provider.MaxAgeCeiling(provider.Declaration{
				Type:      row.typ,
				Sensitive: row.sensitive,
			})
			if got != row.ceiling {
				t.Fatalf("MaxAgeCeiling(%s,sensitive=%v) = %v, want %v (provider-contract.md)",
					row.typ, row.sensitive, got, row.ceiling)
			}
		})
	}

	// Global cap is 24h — even registry types cannot exceed it.
	if provider.MaxAgeGlobalCap != 24*time.Hour {
		t.Fatalf("MaxAgeGlobalCap = %v, want 24h", provider.MaxAgeGlobalCap)
	}

	exceedCases := []struct {
		name string
		raw  string
	}{
		{
			name: "principal_over_1h",
			raw: `{
				"name": "long-principal",
				"requests": {"values": {"pointers": []}},
				"outputs": {
					"groups": {
						"type": "principal",
						"cardinality": "set",
						"subject": "user",
						"sensitive": false,
						"maxAge": "2h"
					}
				}
			}`,
		},
		{
			name: "boolean_over_1h",
			raw: `{
				"name": "long-bool",
				"requests": {"values": {"pointers": []}},
				"outputs": {
					"isOwner": {
						"type": "boolean",
						"cardinality": "single",
						"subject": "user",
						"sensitive": false,
						"maxAge": "90m"
					}
				}
			}`,
		},
		{
			name: "registry_over_24h",
			raw: `{
				"name": "long-registry",
				"requests": {"values": {"pointers": []}},
				"outputs": {
					"costCenter": {
						"type": "string",
						"cardinality": "single",
						"subject": "entry",
						"sensitive": false,
						"maxAge": "48h"
					}
				}
			}`,
		},
		{
			name: "sensitive_over_15m",
			raw: `{
				"name": "long-sensitive",
				"requests": {"values": {"pointers": []}},
				"outputs": {
					"secretMeta": {
						"type": "string",
						"cardinality": "single",
						"subject": "entry",
						"sensitive": true,
						"maxAge": "1h"
					}
				}
			}`,
		},
	}
	for _, tc := range exceedCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := provider.LoadProviderConfig([]byte(tc.raw))
			if err == nil {
				t.Fatal("exceeding maxAge ceiling must be rejected at load (never clamped)")
			}
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "maxage") && !strings.Contains(msg, "max age") {
				t.Fatalf("error should mention maxAge; got: %v", err)
			}
		})
	}

	// Shortening below the ceiling is accepted (never fill-in; explicit value required).
	t.Run("shorten_accepted", func(t *testing.T) {
		cfg, err := provider.LoadProviderConfig([]byte(`{
			"name": "short-principal",
			"requests": {"values": {"pointers": []}},
			"outputs": {
				"groups": {
					"type": "principal",
					"cardinality": "set",
					"subject": "user",
					"sensitive": false,
					"maxAge": "30m"
				}
			}
		}`))
		if err != nil {
			t.Fatalf("shortening below ceiling must be accepted: %v", err)
		}
		if cfg.Outputs["groups"].MaxAge != "30m" {
			t.Fatalf("maxAge = %q, want 30m (must not clamp/fill)", cfg.Outputs["groups"].MaxAge)
		}
	})

	// Exact ceiling is accepted.
	t.Run("exact_ceiling_accepted", func(t *testing.T) {
		_, err := provider.LoadProviderConfig([]byte(`{
			"name": "exact",
			"requests": {"values": {"pointers": []}},
			"outputs": {
				"groups": {
					"type": "principal",
					"cardinality": "set",
					"subject": "user",
					"sensitive": false,
					"maxAge": "1h"
				}
			}
		}`))
		if err != nil {
			t.Fatalf("exact ceiling must be accepted: %v", err)
		}
	})
}

func sensitiveSuffix(s bool) string {
	if s {
		return "_sensitive"
	}
	return ""
}
