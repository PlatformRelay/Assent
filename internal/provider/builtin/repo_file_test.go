package builtin_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
	"github.com/PlatformRelay/assent/internal/provider/builtin"
)

// fixedAsOf is the host-pinned evaluation instant — no wall clock in assertions.
var fixedAsOf = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

func quotaDecl() provider.Declaration {
	return provider.Declaration{
		Type:        "integer",
		Cardinality: "single",
		Subject:     "repo",
		Sensitive:   false,
		MaxAge:      "24h",
	}
}

func regionsDecl() provider.Declaration {
	return provider.Declaration{
		Type:        "string",
		Cardinality: "set",
		Subject:     "repo",
		Sensitive:   false,
		MaxAge:      "24h",
	}
}

func repoFileQuery(id string, outputs []string) provider.FactQuery {
	return provider.FactQuery{
		APIVersion: provider.APIVersion,
		Kind:       provider.KindFactQuery,
		QueryID:    id,
		AsOf:       fixedAsOf,
		Subject:    provider.Subject{Kind: "repo", ID: "fixture"},
		Outputs:    outputs,
	}
}

func callRepoFile(t *testing.T, opts builtin.RepoFileOpts, q provider.FactQuery) provider.CallFunc {
	t.Helper()
	return func(ctx context.Context) ([]byte, error) {
		return builtin.CallRepoFile(ctx, opts, q)
	}
}

// TestBuiltinRepoFileMostSpecific — REQ-E5-S07-01 (closes REF-GAP-2):
// most-specific-first path resolution over a fixture tree.
func TestBuiltinRepoFileMostSpecific(t *testing.T) {
	fsys := os.DirFS(filepath.Join("testdata", "repo-file"))

	cases := []struct {
		name       string
		file       string
		anchor     string
		roots      []string
		output     string
		decl       provider.Declaration
		wantValue  any
		wantSource string // documentation: which candidate won
	}{
		{
			name:       "prod_orders_hits_prod_quota",
			file:       "quota.yaml",
			anchor:     "topics/prod/orders.yaml",
			output:     "max_partitions",
			decl:       quotaDecl(),
			wantValue:  6,
			wantSource: "topics/prod/quota.yaml",
		},
		{
			name:       "dev_orders_walks_up_to_topics_quota",
			file:       "quota.yaml",
			anchor:     "topics/dev/orders.yaml",
			output:     "max_partitions",
			decl:       quotaDecl(),
			wantValue:  24,
			wantSource: "topics/quota.yaml",
		},
		{
			name:       "unrelated_path_falls_back_to_repo_root",
			file:       "quota.yaml",
			anchor:     "services/billing/app.yaml",
			output:     "max_partitions",
			decl:       quotaDecl(),
			wantValue:  12,
			wantSource: "quota.yaml",
		},
		{
			name:       "placement_eu_most_specific",
			file:       "allow.yaml",
			anchor:     "placement/eu/topics/orders.yaml",
			output:     "regions",
			decl:       regionsDecl(),
			wantValue:  []any{"eu-west-1", "eu-central-1"},
			wantSource: "placement/eu/allow.yaml",
		},
		{
			name:       "placement_default_when_no_eu_file_on_path",
			file:       "allow.yaml",
			anchor:     "placement/us/topics/orders.yaml",
			output:     "regions",
			decl:       regionsDecl(),
			wantValue:  []any{"us-east-1"},
			wantSource: "placement/allow.yaml",
		},
		{
			name:       "declared_root_clips_walk_above_topics",
			file:       "quota.yaml",
			anchor:     "topics/dev/orders.yaml",
			roots:      []string{"topics"},
			output:     "max_partitions",
			decl:       quotaDecl(),
			wantValue:  24,
			wantSource: "topics/quota.yaml",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := repoFileQuery("q-e5-s07-"+tc.name, []string{tc.output})
			opts := builtin.RepoFileOpts{
				FS:           fsys,
				File:         tc.file,
				Anchor:       tc.anchor,
				Roots:        tc.roots,
				Declarations: map[string]provider.Declaration{tc.output: tc.decl},
			}

			result := provider.ResolveFacts(context.Background(), callRepoFile(t, opts, q), q, fixedAsOf)
			fact, ok := result.Facts[tc.output]
			if !ok {
				t.Fatalf("requested output %q omitted from result", tc.output)
			}
			if fact.State != provider.StateResolved {
				t.Fatalf("state=%q reason=%q want resolved (source %s)", fact.State, fact.Reason, tc.wantSource)
			}
			if fact.Value == nil {
				t.Fatal("resolved fact must carry a value — nil pretends presence")
			}

			got, err := json.Marshal(fact.Value)
			if err != nil {
				t.Fatalf("marshal got: %v", err)
			}
			want, err := json.Marshal(tc.wantValue)
			if err != nil {
				t.Fatalf("marshal want: %v", err)
			}
			if string(got) != string(want) {
				t.Fatalf("value=%s want %s (expected source %s)", got, want, tc.wantSource)
			}
		})
	}
}

// TestBuiltinRepoFileAbsentUnavailable — REQ-E5-S07-02 (fail-safe):
// absent file → unavailable, never resolved with nil/empty pretending presence.
func TestBuiltinRepoFileAbsentUnavailable(t *testing.T) {
	fsys := os.DirFS(filepath.Join("testdata", "repo-file"))
	q := repoFileQuery("q-e5-s07-absent", []string{"max_partitions"})
	opts := builtin.RepoFileOpts{
		FS:           fsys,
		File:         "does-not-exist.yaml",
		Anchor:       "topics/prod/orders.yaml",
		Declarations: map[string]provider.Declaration{"max_partitions": quotaDecl()},
	}

	result := provider.ResolveFacts(context.Background(), callRepoFile(t, opts, q), q, fixedAsOf)
	fact, ok := result.Facts["max_partitions"]
	if !ok {
		t.Fatal("requested output omitted — fail-open via absence")
	}
	if fact.State != provider.StateUnavailable {
		t.Fatalf("state=%q want unavailable for absent file", fact.State)
	}
	if fact.Value != nil {
		t.Fatalf("unavailable fact must drop value; got %#v (never resolved-empty)", fact.Value)
	}

	// Declared-root clip with no file under the root → unavailable (not repo-root fallback).
	t.Run("roots_clip_no_fallback_above", func(t *testing.T) {
		q := repoFileQuery("q-e5-s07-clip", []string{"max_partitions"})
		opts := builtin.RepoFileOpts{
			FS:           fsys,
			File:         "quota.yaml",
			Anchor:       "services/billing/app.yaml", // outside topics/
			Roots:        []string{"topics"},
			Declarations: map[string]provider.Declaration{"max_partitions": quotaDecl()},
		}
		result := provider.ResolveFacts(context.Background(), callRepoFile(t, opts, q), q, fixedAsOf)
		fact := result.Facts["max_partitions"]
		if fact.State != provider.StateUnavailable {
			t.Fatalf("state=%q want unavailable when anchor is outside declared roots", fact.State)
		}
		if fact.Value != nil {
			t.Fatalf("clipped miss must not resolve; value=%#v", fact.Value)
		}
	})

	// Empty document that exists but lacks the key must not pretend resolved presence
	// via nil/empty — invalid (file present, key absent), never resolved-empty.
	t.Run("present_file_missing_key_not_resolved_empty", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "quota.yaml"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		q := repoFileQuery("q-e5-s07-empty-key", []string{"max_partitions"})
		opts := builtin.RepoFileOpts{
			FS:           os.DirFS(dir),
			File:         "quota.yaml",
			Anchor:       "topics/x.yaml",
			Declarations: map[string]provider.Declaration{"max_partitions": quotaDecl()},
		}
		result := provider.ResolveFacts(context.Background(), callRepoFile(t, opts, q), q, fixedAsOf)
		fact := result.Facts["max_partitions"]
		if fact.State == provider.StateResolved {
			t.Fatalf("missing key must not resolve (got value %#v)", fact.Value)
		}
		if fact.Value != nil {
			t.Fatalf("non-resolved fact must drop value; got %#v", fact.Value)
		}
	})
}
