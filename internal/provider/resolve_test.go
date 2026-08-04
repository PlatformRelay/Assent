package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
)

// fixedAsOf is the host-pinned evaluation instant — no wall clock in assertions.
var fixedAsOf = time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

func groupsQuery() provider.FactQuery {
	return provider.FactQuery{
		APIVersion: provider.APIVersion,
		Kind:       provider.KindFactQuery,
		QueryID:    "q-e5-s01-1",
		AsOf:       fixedAsOf,
		Subject:    provider.Subject{Kind: "user", ID: "alice"},
		Outputs:    []string{"groups"},
	}
}

func groupsDeclaration() provider.Declaration {
	return provider.Declaration{
		Type:        "string",
		Cardinality: "set",
		Subject:     "user",
		Sensitive:   false,
		MaxAge:      "1h",
	}
}

// resolvedResponseBytes builds a schema-valid FactResponse for the groups query.
func resolvedResponseBytes(t *testing.T, q provider.FactQuery, expiresAt time.Time) []byte {
	t.Helper()
	resp := provider.FactResponse{
		APIVersion: provider.APIVersion,
		Kind:       provider.KindFactResponse,
		QueryID:    q.QueryID,
		Facts: []provider.Fact{{
			Name:        "groups",
			Declaration: groupsDeclaration(),
			State:       provider.StateResolved,
			Subject:     q.Subject,
			ObservedAt:  q.AsOf,
			ExpiresAt:   &expiresAt,
			Value:       []any{"platform-team", "reviewers"},
		}},
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// TestResolveFactsStateMachine — REQ-E5-S01-01: every requested key present with an
// explicit state; never omit; never mark resolved on transport/classifier failure.
func TestResolveFactsStateMachine(t *testing.T) {
	q := groupsQuery()
	now := fixedAsOf

	cases := []struct {
		name      string
		call      provider.CallFunc
		wantState string
	}{
		{
			name: "resolved",
			call: func(context.Context) ([]byte, error) {
				return resolvedResponseBytes(t, q, q.AsOf.Add(time.Hour)), nil
			},
			wantState: provider.StateResolved,
		},
		{
			name: "timeout_unavailable",
			call: func(context.Context) ([]byte, error) {
				return nil, errors.New("context deadline exceeded")
			},
			wantState: provider.StateUnavailable,
		},
		{
			name: "garbage_invalid",
			call: func(context.Context) ([]byte, error) {
				return []byte("{{{ not json"), nil
			},
			wantState: provider.StateInvalid,
		},
		{
			name: "schema_invalid",
			call: func(context.Context) ([]byte, error) {
				// Decodable + matching major, but missing required FactResponse fields.
				return []byte(`{"apiVersion":"provider.assent.dev/v1alpha1","kind":"FactResponse","queryId":"q-e5-s01-1"}`), nil
			},
			wantState: provider.StateInvalid,
		},
		{
			name: "queryId_mismatch_invalid",
			call: func(context.Context) ([]byte, error) {
				raw := resolvedResponseBytes(t, q, q.AsOf.Add(time.Hour))
				var resp provider.FactResponse
				if err := json.Unmarshal(raw, &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				resp.QueryID = "other-query"
				out, err := json.Marshal(resp)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				return out, nil
			},
			wantState: provider.StateInvalid,
		},
		{
			name: "omit_output_invalid",
			call: func(context.Context) ([]byte, error) {
				resp := provider.FactResponse{
					APIVersion: provider.APIVersion,
					Kind:       provider.KindFactResponse,
					QueryID:    q.QueryID,
					// Provider returns a fact, but not the requested "groups".
					Facts: []provider.Fact{{
						Name:        "other",
						Declaration: groupsDeclaration(),
						State:       provider.StateUnavailable,
						Subject:     q.Subject,
						ObservedAt:  q.AsOf,
						Reason:      "not requested",
					}},
				}
				out, err := json.Marshal(resp)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				return out, nil
			},
			wantState: provider.StateInvalid,
		},
		{
			name: "expired",
			call: func(context.Context) ([]byte, error) {
				stale := q.AsOf.Add(-time.Minute)
				return resolvedResponseBytes(t, q, stale), nil
			},
			wantState: provider.StateExpired,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := provider.ResolveFacts(t.Context(), tc.call, q, now)
			fact, ok := got.Facts["groups"]
			if !ok {
				t.Fatal("fact key silently absent — must never happen")
			}
			if fact.State != tc.wantState {
				t.Fatalf("state = %q, want %q (reason: %q)", fact.State, tc.wantState, fact.Reason)
			}
			if tc.wantState != provider.StateResolved && fact.State == provider.StateResolved {
				t.Fatal("controlling fact resolved on a failure path — must fail closed")
			}
			if tc.wantState != provider.StateResolved && fact.Value != nil {
				t.Fatalf("non-resolved fact must drop value; got %#v", fact.Value)
			}
			// Every requested key present — no extras required, but no omissions.
			if len(got.Facts) != len(q.Outputs) {
				t.Fatalf("facts len = %d, want %d (keys=%v)", len(got.Facts), len(q.Outputs), keysOf(got.Facts))
			}
			for _, name := range q.Outputs {
				if _, ok := got.Facts[name]; !ok {
					t.Fatalf("requested output %q omitted", name)
				}
			}
		})
	}

	// Multi-output: both keys present even when the call fails.
	t.Run("multi_output_never_omits", func(t *testing.T) {
		q2 := q
		q2.Outputs = []string{"groups", "isOwner"}
		got := provider.ResolveFacts(t.Context(), func(context.Context) ([]byte, error) {
			return nil, errors.New("context deadline exceeded")
		}, q2, now)
		for _, name := range q2.Outputs {
			f, ok := got.Facts[name]
			if !ok {
				t.Fatalf("requested output %q omitted", name)
			}
			if f.State != provider.StateUnavailable {
				t.Fatalf("%s: state = %q, want unavailable", name, f.State)
			}
			if f.State == provider.StateResolved {
				t.Fatalf("%s: resolved on failure path", name)
			}
		}
	})
}

// TestResolveMajorMismatch — REQ-E5-S01-02: Negotiate mismatch ⇒ capability gap,
// all facts unavailable, AutoMergeEligible=false.
func TestResolveMajorMismatch(t *testing.T) {
	q := groupsQuery()
	q.Outputs = []string{"groups", "isOwner"}
	now := fixedAsOf

	call := func(context.Context) ([]byte, error) {
		expires := q.AsOf.Add(time.Hour)
		resp := provider.FactResponse{
			APIVersion: "provider.assent.dev/v2alpha1",
			Kind:       provider.KindFactResponse,
			QueryID:    q.QueryID,
			Facts: []provider.Fact{
				{
					Name:        "groups",
					Declaration: groupsDeclaration(),
					State:       provider.StateResolved,
					Subject:     q.Subject,
					ObservedAt:  q.AsOf,
					ExpiresAt:   &expires,
					Value:       []any{"platform-team"},
				},
				{
					Name:        "isOwner",
					Declaration: provider.Declaration{Type: "boolean", Cardinality: "single", Subject: "user", Sensitive: false, MaxAge: "1h"},
					State:       provider.StateResolved,
					Subject:     q.Subject,
					ObservedAt:  q.AsOf,
					ExpiresAt:   &expires,
					Value:       true,
				},
			},
		}
		raw, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return raw, nil
	}

	got := provider.ResolveFacts(t.Context(), call, q, now)
	if got.Negotiation.Outcome != provider.OutcomeCapabilityGap {
		t.Fatalf("outcome = %q, want capability_gap", got.Negotiation.Outcome)
	}
	if !got.Negotiation.ProviderRefused {
		t.Fatal("capability gap must refuse the provider")
	}
	if got.Negotiation.AutoMergeEligible {
		t.Fatal("capability-gap facts must never remain auto-merge eligible")
	}
	if got.AutoMergeEligible() {
		t.Fatal("Result.AutoMergeEligible must be false on major mismatch")
	}
	for _, name := range q.Outputs {
		f, ok := got.Facts[name]
		if !ok {
			t.Fatalf("requested output %q omitted", name)
		}
		if f.State != provider.StateUnavailable {
			t.Fatalf("%s: state = %q, want unavailable (must not process mismatched major as resolved)", name, f.State)
		}
		if f.Value != nil {
			t.Fatalf("%s: value must be dropped on capability gap; got %#v", name, f.Value)
		}
	}
}

// goldenView is the byte-stable projection of a ResolveFacts result for goldens.
type goldenView struct {
	AutoMergeEligible bool                     `json:"autoMergeEligible"`
	Negotiation       string                   `json:"negotiation"`
	Facts             map[string]goldenFact    `json:"facts"`
}

type goldenFact struct {
	Name       string          `json:"name"`
	State      string          `json:"state"`
	Subject    provider.Subject `json:"subject"`
	ObservedAt time.Time       `json:"observedAt"`
	ExpiresAt  *time.Time      `json:"expiresAt,omitempty"`
	Value      any             `json:"value,omitempty"`
	Reason     string          `json:"reason,omitempty"`
}

func toGoldenView(r provider.Result) goldenView {
	facts := make(map[string]goldenFact, len(r.Facts))
	for k, f := range r.Facts {
		facts[k] = goldenFact{
			Name:       f.Name,
			State:      f.State,
			Subject:    f.Subject,
			ObservedAt: f.ObservedAt,
			ExpiresAt:  f.ExpiresAt,
			Value:      f.Value,
			Reason:     f.Reason,
		}
	}
	return goldenView{
		AutoMergeEligible: r.AutoMergeEligible(),
		Negotiation:       string(r.Negotiation.Outcome),
		Facts:             facts,
	}
}

func canonicalize(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("remarshal: %v", err)
	}
	return out
}

// TestResolveGoldens — REQ-E5-S01-03: Spike classifier behaviors preserved
// byte-stable in goldens (resolved / unavailable / invalid / expired).
func TestResolveGoldens(t *testing.T) {
	q := groupsQuery()
	now := fixedAsOf

	cases := []struct {
		file string
		call provider.CallFunc
	}{
		{
			file: "resolved.json",
			call: func(context.Context) ([]byte, error) {
				return resolvedResponseBytes(t, q, q.AsOf.Add(time.Hour)), nil
			},
		},
		{
			file: "unavailable-timeout.json",
			call: func(context.Context) ([]byte, error) {
				return nil, errors.New("context deadline exceeded")
			},
		},
		{
			file: "invalid-garbage.json",
			call: func(context.Context) ([]byte, error) {
				return []byte("{{{ not json"), nil
			},
		},
		{
			file: "expired.json",
			call: func(context.Context) ([]byte, error) {
				stale := q.AsOf.Add(-time.Minute)
				return resolvedResponseBytes(t, q, stale), nil
			},
		},
		{
			file: "major-mismatch.json",
			call: func(context.Context) ([]byte, error) {
				expires := q.AsOf.Add(time.Hour)
				resp := provider.FactResponse{
					APIVersion: "provider.assent.dev/v2alpha1",
					Kind:       provider.KindFactResponse,
					QueryID:    q.QueryID,
					Facts: []provider.Fact{{
						Name:        "groups",
						Declaration: groupsDeclaration(),
						State:       provider.StateResolved,
						Subject:     q.Subject,
						ObservedAt:  q.AsOf,
						ExpiresAt:   &expires,
						Value:       []any{"platform-team"},
					}},
				}
				raw, err := json.Marshal(resp)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				return raw, nil
			},
		},
	}

	dir := filepath.Join("testdata", "resolve")
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			got := provider.ResolveFacts(t.Context(), tc.call, q, now)
			gotCanon := canonicalize(t, toGoldenView(got))
			path := filepath.Join(dir, tc.file)
			if os.Getenv("UPDATE_GOLDENS") == "1" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				var pretty bytes.Buffer
				if err := json.Indent(&pretty, gotCanon, "", "  "); err != nil {
					t.Fatalf("indent: %v", err)
				}
				pretty.WriteByte('\n')
				if err := os.WriteFile(path, pretty.Bytes(), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
			}
			wantRaw, err := os.ReadFile(path) //nolint:gosec // test golden path under testdata/
			if err != nil {
				t.Fatalf("read golden %s: %v (run UPDATE_GOLDENS=1 to create)", path, err)
			}
			wantCanon := canonicalize(t, json.RawMessage(wantRaw))
			if !bytes.Equal(gotCanon, wantCanon) {
				t.Fatalf("golden mismatch for %s\n got: %s\nwant: %s", tc.file, gotCanon, wantCanon)
			}
			// Double-run determinism.
			got2 := provider.ResolveFacts(t.Context(), tc.call, q, now)
			got2Canon := canonicalize(t, toGoldenView(got2))
			if !bytes.Equal(gotCanon, got2Canon) {
				t.Fatalf("non-deterministic ResolveFacts for %s", tc.file)
			}
		})
	}
}

func keysOf(m map[string]provider.Fact) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
