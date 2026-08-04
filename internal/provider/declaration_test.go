package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
)

// TestDeclarationCrossCheck — REQ-E5-S02-04: host cross-checks the provider's
// echoed declaration against config (type/cardinality/subject/sensitive/maxAge);
// mismatch → invalid (never silently accept).
func TestDeclarationCrossCheck(t *testing.T) {
	expected := provider.Declaration{
		Type:        "string",
		Cardinality: "set",
		Subject:     "user",
		Sensitive:   false,
		MaxAge:      "1h",
	}
	q := groupsQuery()
	now := fixedAsOf

	t.Run("match_keeps_resolved", func(t *testing.T) {
		call := func(context.Context) ([]byte, error) {
			return resolvedResponseBytes(t, q, q.AsOf.Add(time.Hour)), nil
		}
		got := provider.ResolveFactsChecked(t.Context(), call, q, now, map[string]provider.Declaration{
			"groups": expected,
		})
		f := got.Facts["groups"]
		if f.State != provider.StateResolved {
			t.Fatalf("state = %q, want resolved (reason: %q)", f.State, f.Reason)
		}
		if f.Value == nil {
			t.Fatal("matched declaration must keep resolved value")
		}
	})

	mismatchFields := []struct {
		name   string
		mutate func(*provider.Declaration)
	}{
		{"type", func(d *provider.Declaration) { d.Type = "principal" }},
		{"cardinality", func(d *provider.Declaration) { d.Cardinality = "single" }},
		{"subject", func(d *provider.Declaration) { d.Subject = "group" }},
		{"sensitive", func(d *provider.Declaration) { d.Sensitive = true }},
		{"maxAge", func(d *provider.Declaration) { d.MaxAge = "30m" }},
	}
	for _, tc := range mismatchFields {
		t.Run("mismatch_"+tc.name, func(t *testing.T) {
			call := func(context.Context) ([]byte, error) {
				raw := resolvedResponseBytes(t, q, q.AsOf.Add(time.Hour))
				var resp provider.FactResponse
				if err := json.Unmarshal(raw, &resp); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				d := resp.Facts[0].Declaration
				tc.mutate(&d)
				resp.Facts[0].Declaration = d
				out, err := json.Marshal(resp)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				return out, nil
			}
			got := provider.ResolveFactsChecked(t.Context(), call, q, now, map[string]provider.Declaration{
				"groups": expected,
			})
			f, ok := got.Facts["groups"]
			if !ok {
				t.Fatal("fact key silently absent — must never happen")
			}
			if f.State != provider.StateInvalid {
				t.Fatalf("state = %q, want invalid on declaration mismatch", f.State)
			}
			if f.Value != nil {
				t.Fatalf("mismatched declaration must drop value; got %#v", f.Value)
			}
			if f.Reason == "" {
				t.Fatal("invalid fact must carry a reason")
			}
		})
	}

	// Golden: declaration mismatch → invalid, byte-stable.
	t.Run("golden_mismatch", func(t *testing.T) {
		call := func(context.Context) ([]byte, error) {
			raw := resolvedResponseBytes(t, q, q.AsOf.Add(time.Hour))
			var resp provider.FactResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			resp.Facts[0].Declaration.MaxAge = "30m"
			out, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			return out, nil
		}
		got := provider.ResolveFactsChecked(t.Context(), call, q, now, map[string]provider.Declaration{
			"groups": expected,
		})
		gotCanon := canonicalize(t, toGoldenView(got))
		path := filepath.Join("testdata", "resolve", "declaration-mismatch.json")
		if os.Getenv("UPDATE_GOLDENS") == "1" {
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, gotCanon, "", "  "); err != nil {
				t.Fatalf("indent: %v", err)
			}
			pretty.WriteByte('\n')
			if err := os.WriteFile(path, pretty.Bytes(), 0o600); err != nil {
				t.Fatalf("write golden: %v", err)
			}
		}
		wantRaw, err := os.ReadFile(path) //nolint:gosec // test golden under testdata/
		if err != nil {
			t.Fatalf("read golden %s: %v (run UPDATE_GOLDENS=1 to create)", path, err)
		}
		wantCanon := canonicalize(t, json.RawMessage(wantRaw))
		if !bytes.Equal(gotCanon, wantCanon) {
			t.Fatalf("golden mismatch\n got: %s\nwant: %s", gotCanon, wantCanon)
		}
	})
}

func jsonMarshalCompact(v any) ([]byte, error) {
	return json.Marshal(v)
}
