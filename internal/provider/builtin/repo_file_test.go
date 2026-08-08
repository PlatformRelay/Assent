package builtin_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
	"github.com/PlatformRelay/assent/internal/provider/builtin"
)

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

// containmentFixture builds a checkout-shaped tree whose contents are chosen so
// that ANY containment failure is observable as a WRONG VALUE, not merely as a
// different state:
//
//	quota.yaml               12   repo root  (above a "topics" clip)
//	topics/quota.yaml        24   inside the clip
//	topics/prod/quota.yaml    6   most specific
//	topics-archive/quota.yaml 99  SIBLING of the clip — sharing its name prefix
//	rootonly.yaml                 exists ONLY at the repo root
//
// 99 is the sibling-prefix tell: a containment check written as
// strings.HasPrefix(p, root) instead of HasPrefix(p, root+"/") treats
// "topics-archive/..." as inside "topics", and 99 leaks.
func containmentFixture(t *testing.T) fs.FS {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("quota.yaml", "max_partitions: 12\n")
	write("topics/quota.yaml", "max_partitions: 24\n")
	write("topics/prod/quota.yaml", "max_partitions: 6\n")
	write("topics-archive/quota.yaml", "max_partitions: 99\n")
	write("rootonly.yaml", "max_partitions: 77\n")
	return os.DirFS(dir)
}

// TestRepoFileContainment — REQ-AUD-S13-03 (TEST-06), security-adjacent.
//
// Pins the PATH-CONTAINMENT surface (cleanRel / cleanRoots / underAnyRoot): a
// traversal, absolute, or root-escaping path must never yield a fact, and the
// declared roots must actually clip the walk-up. The load-bearing assertion on
// every rejecting case is that `fact.Value` is nil — the story's invariant is
// "never a fact from outside the roots", so a rejection that still carried a
// value would be the failure, whatever state label it wore.
//
// SCOPE (see the PR body): these are STRING-level guards. `os.DirFS` is
// explicitly not a security boundary in Go, and a real on-disk symlink under a
// declared root is NOT rejected by this code. The "symlink-shaped" inputs below
// are traversal-shaped path strings, which is what cleanRel/underAnyRoot can
// actually decide.
func TestRepoFileContainment(t *testing.T) {
	fsys := containmentFixture(t)

	cases := []struct {
		name      string
		nilFS     bool
		file      string
		anchor    string
		roots     []string
		wantState string
		// wantValue is asserted only when wantState is resolved.
		wantValue any
		// wantReason is a substring the non-resolved fact's reason must carry, so
		// a case cannot pass by being rejected for an unrelated reason.
		wantReason string
	}{
		{
			// Control: the fixture CAN produce a fact. Without this, every
			// rejection below could be an inert harness rather than a guard.
			name: "control_under_declared_root_resolves", file: "quota.yaml",
			anchor: "topics/prod/orders.yaml", roots: []string{"topics"},
			wantState: provider.StateResolved, wantValue: float64(6),
		},
		{
			name: "anchor_absolute_rejected", file: "quota.yaml",
			anchor:    "/etc/passwd",
			wantState: provider.StateInvalid, wantReason: "anchor must be relative",
		},
		{
			name: "anchor_traversal_rejected", file: "quota.yaml",
			anchor:    "../../etc/passwd",
			wantState: provider.StateInvalid, wantReason: "anchor escapes filesystem root",
		},
		{
			// Traversal that only escapes AFTER normalization — the shape a naive
			// strings.Contains("..") check would catch but a naive one would not.
			name: "anchor_traversal_after_normalization_rejected", file: "quota.yaml",
			anchor:    "topics/prod/../../../outside/x.yaml",
			wantState: provider.StateInvalid, wantReason: "anchor escapes filesystem root",
		},
		{
			// Backslash-separated traversal is normalized to "/" first, so a
			// Windows-shaped escape cannot slip past the "../" check.
			name: "anchor_backslash_traversal_rejected", file: "quota.yaml",
			anchor:    `..\..\etc\passwd`,
			wantState: provider.StateInvalid, wantReason: "anchor escapes filesystem root",
		},
		{
			name: "root_absolute_rejected", file: "quota.yaml",
			anchor: "topics/prod/orders.yaml", roots: []string{"/etc"},
			wantState: provider.StateInvalid, wantReason: "root",
		},
		{
			name: "root_traversal_rejected", file: "quota.yaml",
			anchor: "topics/prod/orders.yaml", roots: []string{"../outside"},
			wantState: provider.StateInvalid, wantReason: "root",
		},
		{
			// A "." root is the whole FS, i.e. NO clip — so the walk-up reaches the
			// repo root. Polarity partner for the two root rejections above.
			name: "root_dot_is_whole_fs_not_a_clip", file: "quota.yaml",
			anchor: "services/billing/app.yaml", roots: []string{"."},
			wantState: provider.StateResolved, wantValue: float64(12),
		},
		{
			// THE SIBLING-PREFIX CASE. "topics-archive" shares "topics"' prefix but
			// is a different directory: it is NOT under the declared root, so no
			// fact — and in particular never the 99 that lives there.
			name: "sibling_prefix_directory_is_not_under_root", file: "quota.yaml",
			anchor: "topics-archive/orders.yaml", roots: []string{"topics"},
			wantState: provider.StateUnavailable, wantReason: "anchor outside declared roots",
		},
		{
			// The clip must also stop the WALK-UP, not just the anchor check: the
			// file exists only above the root, so it must stay invisible.
			name: "walkup_above_root_is_clipped", file: "rootonly.yaml",
			anchor: "topics/prod/orders.yaml", roots: []string{"topics"},
			wantState: provider.StateUnavailable, wantReason: "no matching file",
		},
		{
			// An empty anchor normalizes to "." (the repo root). The anchor guard
			// deliberately exempts "." (`&& anchor != "."`), so the clip is enforced
			// one step later, by the walk-up: every candidate is outside the root, so
			// the root-level quota.yaml (12) stays invisible and NO fact is produced.
			// The reason is therefore the walk-up's, not the anchor guard's.
			name: "empty_anchor_with_roots_yields_no_fact", file: "quota.yaml",
			anchor: "", roots: []string{"topics"},
			wantState: provider.StateUnavailable, wantReason: "no matching file",
		},
		{
			// File is a BASENAME, never a path: a traversal in it is basenamed away,
			// so it resolves from inside the roots (6) — it cannot reach ../../etc.
			name: "file_traversal_is_basenamed_not_followed", file: "../../etc/quota.yaml",
			anchor: "topics/prod/orders.yaml", roots: []string{"topics"},
			wantState: provider.StateResolved, wantValue: float64(6),
		},
		{
			name: "file_dot_rejected", file: ".",
			anchor:    "topics/prod/orders.yaml",
			wantState: provider.StateInvalid, wantReason: "File must be a basename",
		},
		{
			name: "file_dotdot_rejected", file: "..",
			anchor:    "topics/prod/orders.yaml",
			wantState: provider.StateInvalid, wantReason: "File must be a basename",
		},
		{
			name: "file_empty_rejected", file: "   ",
			anchor:    "topics/prod/orders.yaml",
			wantState: provider.StateInvalid, wantReason: "FS and File are required",
		},
		{
			name: "nil_fs_rejected", nilFS: true, file: "quota.yaml",
			anchor:    "topics/prod/orders.yaml",
			wantState: provider.StateInvalid, wantReason: "FS and File are required",
		},
	}
	// Positive control: a table that lost its cases would sweep an empty set.
	if len(cases) != 16 {
		t.Fatalf("table lost cases: have %d, want 16", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := repoFileQuery("q-aud-s13-"+tc.name, []string{"max_partitions"})
			opts := builtin.RepoFileOpts{
				FS:           fsys,
				File:         tc.file,
				Anchor:       tc.anchor,
				Roots:        tc.roots,
				Declarations: map[string]provider.Declaration{"max_partitions": quotaDecl()},
			}
			if tc.nilFS {
				opts.FS = nil
			}

			result := provider.ResolveFacts(context.Background(), callRepoFile(t, opts, q), q, fixedAsOf)
			fact, ok := result.Facts["max_partitions"]
			if !ok {
				t.Fatal("requested output omitted from the result — fail-open via absence")
			}
			if fact.State != tc.wantState {
				t.Fatalf("state = %q (reason %q), want %q", fact.State, fact.Reason, tc.wantState)
			}

			if tc.wantState == provider.StateResolved {
				if fact.Value != tc.wantValue {
					t.Fatalf("value = %#v, want %#v", fact.Value, tc.wantValue)
				}
				return
			}

			// The invariant: a rejected path NEVER carries a value, whatever the
			// state label. This is what "never a fact from outside the roots" means.
			if fact.Value != nil {
				t.Fatalf("a rejected path must carry NO value; got %#v (reason %q)", fact.Value, fact.Reason)
			}
			if !strings.Contains(fact.Reason, tc.wantReason) {
				t.Fatalf("reason = %q, want it to contain %q (rejected for the wrong cause?)", fact.Reason, tc.wantReason)
			}
			// Belt and braces against the sibling-prefix leak specifically.
			if fact.Value == float64(99) {
				t.Fatal("the sibling directory's value escaped the declared root")
			}
		})
	}
}

// TestRepoFileUndecodableDocument — REQ-AUD-S13-03 (TEST-06), the third
// rejection axis in answerRepoFile's fail-closed list ("undecodable body →
// invalid"), alongside containment and expiry.
//
// A repo file the provider cannot parse, and a file that parses to nothing, are
// both states where the provider knows nothing. Neither may become a fact: an
// undecodable document must be invalid, and an empty/null one must NOT become a
// resolved-empty fact pretending the key was present.
func TestRepoFileUndecodableDocument(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantState  string
		wantValue  any
		wantReason string
	}{
		{
			// Control: the same harness DOES produce a fact from a good document.
			name:      "control_valid_document_resolves",
			body:      "max_partitions: 6\n",
			wantState: provider.StateResolved, wantValue: float64(6),
		},
		{
			name:      "unparseable_yaml_is_invalid",
			body:      "max_partitions: [unclosed\n",
			wantState: provider.StateInvalid, wantReason: "decode",
		},
		{
			// A top-level sequence is well-formed YAML but not a mapping, so it
			// cannot be keyed by output name — undecodable for this provider.
			name:      "top_level_sequence_is_invalid",
			body:      "- a\n- b\n",
			wantState: provider.StateInvalid, wantReason: "decode",
		},
		{
			// Empty document: present-but-empty mapping, so the requested key is
			// ABSENT (invalid) — never a resolved fact carrying a nil/zero value.
			name:      "empty_document_is_key_absent_not_resolved_empty",
			body:      "",
			wantState: provider.StateInvalid, wantReason: "key absent",
		},
		{
			// Explicit YAML null decodes to a nil document — same treatment.
			name:      "null_document_is_key_absent_not_resolved_empty",
			body:      "null\n",
			wantState: provider.StateInvalid, wantReason: "key absent",
		},
	}
	// Positive control: a table that lost its cases would sweep an empty set.
	if len(cases) != 5 {
		t.Fatalf("table lost cases: have %d, want 5", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "quota.yaml"), []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			q := repoFileQuery("q-aud-s13-doc-"+tc.name, []string{"max_partitions"})
			opts := builtin.RepoFileOpts{
				FS:           os.DirFS(dir),
				File:         "quota.yaml",
				Anchor:       "topics/prod/orders.yaml",
				Declarations: map[string]provider.Declaration{"max_partitions": quotaDecl()},
			}

			result := provider.ResolveFacts(context.Background(), callRepoFile(t, opts, q), q, fixedAsOf)
			fact, ok := result.Facts["max_partitions"]
			if !ok {
				t.Fatal("requested output omitted from the result — fail-open via absence")
			}
			if fact.State != tc.wantState {
				t.Fatalf("state = %q (reason %q), want %q", fact.State, fact.Reason, tc.wantState)
			}
			if tc.wantState == provider.StateResolved {
				if fact.Value != tc.wantValue {
					t.Fatalf("value = %#v, want %#v", fact.Value, tc.wantValue)
				}
				return
			}
			if fact.Value != nil {
				t.Fatalf("an undecodable/empty document must yield NO value; got %#v", fact.Value)
			}
			if !strings.Contains(fact.Reason, tc.wantReason) {
				t.Fatalf("reason = %q, want it to contain %q", fact.Reason, tc.wantReason)
			}
		})
	}
}

// TestRepoFileExpiry — REQ-AUD-S13-03 (TEST-06), the expiry half.
//
// `expiresAt` is the only thing standing between a stale repo file and a fact
// the engine treats as current. An undeclared, unparseable, zero or negative
// maxAge is UNDECIDABLE freshness, so it must synthesize a non-resolved state —
// never a fact with no expiry, and never a fact that outlives its window.
//
// The boundary is asserted at both polarities one nanosecond apart: expiry is
// EXCLUSIVE (a fact is already expired AT its expiresAt instant).
func TestRepoFileExpiry(t *testing.T) {
	fsys := containmentFixture(t)
	const day = 24 * time.Hour

	cases := []struct {
		name   string
		maxAge string
		// now is the evaluation instant handed to ResolveFacts; the query's AsOf
		// stays fixedAsOf, so expiresAt is always fixedAsOf+maxAge.
		now time.Time

		// wantProviderState/Reason are asserted on the builtin's OWN response
		// bytes. They pin `expiresAt` itself: for a malformed maxAge the host's
		// schema gate rejects the echoed declaration first and overwrites the
		// reason, so asserting only the host outcome would never see which
		// expiresAt branch actually fired.
		wantProviderState  string
		wantProviderReason string

		// wantState/wantReason are the host outcome after ResolveFacts.
		wantState  string
		wantReason string
		// wantExpiry is the expiresAt a resolved fact must carry.
		wantExpiry time.Time
	}{
		{
			// Polarity partner for every rejection below.
			name: "valid_maxAge_resolves_with_expiry", maxAge: "24h", now: fixedAsOf,
			wantProviderState: provider.StateResolved,
			wantState:         provider.StateResolved, wantExpiry: fixedAsOf.Add(day),
		},
		{
			name: "missing_maxAge_is_invalid", maxAge: "", now: fixedAsOf,
			wantProviderState: provider.StateInvalid, wantProviderReason: "maxAge is required",
			wantState: provider.StateInvalid,
		},
		{
			name: "unparseable_maxAge_is_invalid", maxAge: "24", now: fixedAsOf,
			wantProviderState: provider.StateInvalid, wantProviderReason: `maxAge "24"`,
			wantState: provider.StateInvalid,
		},
		{
			name: "zero_maxAge_is_invalid", maxAge: "0s", now: fixedAsOf,
			wantProviderState: provider.StateInvalid, wantProviderReason: "must be positive",
			wantState: provider.StateInvalid, wantReason: "must be positive",
		},
		{
			name: "negative_maxAge_is_invalid", maxAge: "-1h", now: fixedAsOf,
			wantProviderState: provider.StateInvalid, wantProviderReason: "must be positive",
			wantState: provider.StateInvalid,
		},
		{
			// Boundary, fresh side: one nanosecond before expiry it still resolves.
			name: "one_ns_before_expiry_still_resolves", maxAge: "24h",
			now:               fixedAsOf.Add(day - time.Nanosecond),
			wantProviderState: provider.StateResolved,
			wantState:         provider.StateResolved, wantExpiry: fixedAsOf.Add(day),
		},
		{
			// Boundary, stale side: AT the expiry instant it is ALREADY expired
			// (the host's check is `!expiresAt.After(now)` — exclusive). The
			// provider still answers resolved; the host is what ages it out.
			name: "exactly_at_expiry_instant_is_expired", maxAge: "24h",
			now:               fixedAsOf.Add(day),
			wantProviderState: provider.StateResolved,
			wantState:         provider.StateExpired, wantReason: "not after the evaluation instant",
		},
		{
			name: "past_expiry_is_expired", maxAge: "24h",
			now:               fixedAsOf.Add(day + time.Hour),
			wantProviderState: provider.StateResolved,
			wantState:         provider.StateExpired, wantReason: "not after the evaluation instant",
		},
	}
	// Positive control: a table that lost its cases would sweep an empty set.
	if len(cases) != 8 {
		t.Fatalf("table lost cases: have %d, want 8", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decl := quotaDecl()
			decl.MaxAge = tc.maxAge

			q := repoFileQuery("q-aud-s13-exp-"+tc.name, []string{"max_partitions"})
			opts := builtin.RepoFileOpts{
				FS:           fsys,
				File:         "quota.yaml",
				Anchor:       "topics/prod/orders.yaml",
				Declarations: map[string]provider.Declaration{"max_partitions": decl},
			}

			// Layer 1 — the builtin's own answer, which is where expiresAt decides.
			raw, err := builtin.CallRepoFile(context.Background(), opts, q)
			if err != nil {
				t.Fatalf("CallRepoFile: %v", err)
			}
			var resp provider.FactResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				t.Fatalf("unmarshal provider response: %v", err)
			}
			if len(resp.Facts) != 1 {
				t.Fatalf("provider returned %d facts, want 1", len(resp.Facts))
			}
			pf := resp.Facts[0]
			if pf.State != tc.wantProviderState {
				t.Fatalf("provider state = %q (reason %q), want %q", pf.State, pf.Reason, tc.wantProviderState)
			}
			if tc.wantProviderReason != "" && !strings.Contains(pf.Reason, tc.wantProviderReason) {
				t.Fatalf("provider reason = %q, want it to contain %q "+
					"(a different expiresAt branch fired)", pf.Reason, tc.wantProviderReason)
			}
			if pf.State != provider.StateResolved && pf.Value != nil {
				t.Fatalf("a non-resolved provider fact must drop its value; got %#v", pf.Value)
			}

			// Layer 2 — the host outcome the engine actually consumes.
			result := provider.ResolveFacts(context.Background(), callRepoFile(t, opts, q), q, tc.now)
			fact, ok := result.Facts["max_partitions"]
			if !ok {
				t.Fatal("requested output omitted from the result — fail-open via absence")
			}
			if fact.State != tc.wantState {
				t.Fatalf("host state = %q (reason %q), want %q", fact.State, fact.Reason, tc.wantState)
			}

			if tc.wantState == provider.StateResolved {
				if fact.Value != float64(6) {
					t.Fatalf("value = %#v, want 6", fact.Value)
				}
				if fact.ExpiresAt == nil {
					t.Fatal("a resolved fact must carry expiresAt — an unbounded fact never goes stale")
				}
				if !fact.ExpiresAt.Equal(tc.wantExpiry) {
					t.Fatalf("expiresAt = %s, want %s", fact.ExpiresAt, tc.wantExpiry)
				}
				return
			}

			// Undecidable or elapsed freshness must never leave a usable value behind.
			if fact.Value != nil {
				t.Fatalf("a non-resolved fact must drop its value; got %#v", fact.Value)
			}
			if tc.wantReason != "" && !strings.Contains(fact.Reason, tc.wantReason) {
				t.Fatalf("host reason = %q, want it to contain %q", fact.Reason, tc.wantReason)
			}
		})
	}
}
