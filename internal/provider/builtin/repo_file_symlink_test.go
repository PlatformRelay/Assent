package builtin_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/provider"
	"github.com/PlatformRelay/assent/internal/provider/builtin"
)

// requireSymlinks skips (loudly, never silently) on hosts where the test process
// cannot create a symlink — unprivileged Windows runners, exotic filesystems.
// Every other platform MUST run these cases: they are the containment proof.
func requireSymlinks(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "target"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable on %s (%v) — filesystem containment cannot be proven here", runtime.GOOS, err)
	}
}

// symlinkTree materialises the containment fixture and returns (repoRoot, outsideDir).
//
//	<tmp>/outside/cluster-secrets.yaml   max_partitions: 31337   (host file, off-tree)
//	<tmp>/outside/quota.yaml             max_partitions: 999     (host file, off-tree)
//	<tmp>/repo/quota.yaml                max_partitions: 12
//	<tmp>/repo/topics/quota.yaml         max_partitions: 24
//	<tmp>/repo/topics/prod/orders.yaml   (the governed anchor)
//	<tmp>/repo/secrets/quota.yaml        max_partitions: 4242    (in-FS, OUTSIDE Roots)
func symlinkTree(t *testing.T) (repo, outside string) {
	t.Helper()
	tmp := t.TempDir()
	repo = filepath.Join(tmp, "repo")
	outside = filepath.Join(tmp, "outside")
	for _, dir := range []string{
		outside,
		filepath.Join(repo, "topics", "prod"),
		filepath.Join(repo, "secrets"),
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(outside, "cluster-secrets.yaml"):       "max_partitions: 31337\n",
		filepath.Join(outside, "quota.yaml"):                 "max_partitions: 999\n",
		filepath.Join(repo, "quota.yaml"):                    "max_partitions: 12\n",
		filepath.Join(repo, "topics", "quota.yaml"):          "max_partitions: 24\n",
		filepath.Join(repo, "secrets", "quota.yaml"):         "max_partitions: 4242\n",
		filepath.Join(repo, "topics", "prod", "orders.yaml"): "partitions: 6\n",
	}
	for name, body := range files {
		if err := os.WriteFile(name, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return repo, outside
}

func repoFileFact(t *testing.T, opts builtin.RepoFileOpts, id string) provider.Fact {
	t.Helper()
	q := repoFileQuery(id, []string{"max_partitions"})
	opts.Declarations = map[string]provider.Declaration{"max_partitions": quotaDecl()}
	result := provider.ResolveFacts(context.Background(), callRepoFile(t, opts, q), q, fixedAsOf)
	fact, ok := result.Facts["max_partitions"]
	if !ok {
		t.Fatal("requested output omitted from result — fail-open via absence")
	}
	return fact
}

// TestRepoFileSymlinkContainment — OQ-28 / D-129: `builtin/repo-file` must never
// answer with a fact read through a symlink. Path containment (cleanRel /
// underAnyRoot) is a STRING guard; os.DirFS is documented as NOT a security
// boundary and fs.Stat follows links, so before D-129 both escapes below resolved.
//
// The matrix runs every case against BOTH filesystem flavours:
//   - "dirfs"  — a plain os.DirFS: proves the BUILTIN refuses on its own (layer 2).
//   - "rootfs" — (*os.Root).FS(): the symlink-safe root the host now injects (layer 1).
//
// Both must reach the same verdict; a case that only passes under "rootfs" would
// be testing os.Root, not this package.
func TestRepoFileSymlinkContainment(t *testing.T) {
	requireSymlinks(t)

	cases := []struct {
		name string
		// link creates the attack symlink inside repo; returns nothing.
		link func(t *testing.T, repo, outside string)
		// anchor/roots for the query.
		anchor string
		roots  []string
		// wantResolved: only the control case may resolve.
		wantResolved bool
		wantValue    string // JSON-ish scalar rendering, checked when resolved
		why          string
	}{
		{
			name:   "control_real_file_still_resolves",
			link:   func(t *testing.T, _, _ string) { t.Helper() },
			anchor: "topics/prod/orders.yaml",
			roots:  []string{"topics"},
			// No topics/prod/quota.yaml exists → walk-up hits topics/quota.yaml.
			wantResolved: true,
			wantValue:    "24",
			why:          "legitimate in-root walk-up must keep working",
		},
		{
			name: "directory_symlink_escape_refused",
			link: func(t *testing.T, repo, outside string) {
				t.Helper()
				mustSymlink(t, outside, filepath.Join(repo, "topics", "evil"))
			},
			anchor: "topics/evil/orders.yaml",
			roots:  []string{"topics"},
			why:    "topics/evil -> <outside> made outside/quota.yaml (999) a resolved fact",
		},
		{
			name: "file_symlink_at_legitimate_in_root_path_refused",
			link: func(t *testing.T, repo, outside string) {
				t.Helper()
				mustSymlink(t, filepath.Join(outside, "cluster-secrets.yaml"),
					filepath.Join(repo, "topics", "prod", "quota.yaml"))
			},
			anchor: "topics/prod/orders.yaml",
			roots:  []string{"topics"},
			why:    "absolute host path reached via a wholly legitimate in-root candidate (31337)",
		},
		{
			name: "relative_file_symlink_escape_refused",
			link: func(t *testing.T, repo, _ string) {
				t.Helper()
				mustSymlink(t, filepath.Join("..", "..", "..", "outside", "cluster-secrets.yaml"),
					filepath.Join(repo, "topics", "prod", "quota.yaml"))
			},
			anchor: "topics/prod/orders.yaml",
			roots:  []string{"topics"},
			why:    "relative ../ escape must be refused exactly like the absolute one",
		},
		{
			name: "in_fs_symlink_outside_declared_roots_refused",
			link: func(t *testing.T, repo, _ string) {
				t.Helper()
				mustSymlink(t, filepath.Join("..", "..", "secrets", "quota.yaml"),
					filepath.Join(repo, "topics", "prod", "quota.yaml"))
			},
			anchor: "topics/prod/orders.yaml",
			roots:  []string{"topics"},
			why:    "a symlink that stays inside the FS root still bypasses the declared-roots clip (4242); os.Root cannot see Roots, so only the builtin can refuse this",
		},
		{
			name: "in_root_symlink_inside_declared_roots_refused",
			link: func(t *testing.T, repo, _ string) {
				t.Helper()
				mustSymlink(t, filepath.Join("..", "quota.yaml"),
					filepath.Join(repo, "topics", "prod", "quota.yaml"))
			},
			anchor: "topics/prod/orders.yaml",
			roots:  []string{"topics"},
			why:    "D-129 refuses ALL symlinked candidates — one rule, no per-link reasoning; the walk must NOT silently fall back to topics/quota.yaml either",
		},
	}

	for _, tc := range cases {
		for _, flavour := range []string{"dirfs", "rootfs"} {
			t.Run(tc.name+"/"+flavour, func(t *testing.T) {
				repo, outside := symlinkTree(t)
				tc.link(t, repo, outside)

				opts := builtin.RepoFileOpts{
					FS:     openFlavour(t, flavour, repo),
					File:   "quota.yaml",
					Anchor: tc.anchor,
					Roots:  tc.roots,
				}
				fact := repoFileFact(t, opts, "q-d129-"+tc.name+"-"+flavour)

				if !tc.wantResolved {
					if fact.State == provider.StateResolved {
						t.Fatalf("CONTAINMENT ESCAPE: state=resolved value=%#v (%s)", fact.Value, tc.why)
					}
					if fact.Value != nil {
						t.Fatalf("non-resolved fact must drop its value; got %#v", fact.Value)
					}
					for _, leaked := range []string{"31337", "999", "4242", "12"} {
						if strings.Contains(strings.TrimSpace(scalar(fact.Value)), leaked) {
							t.Fatalf("value %q leaked through a refused candidate", leaked)
						}
					}
					if fact.Reason == "" {
						t.Fatal("a refusal must state a reason the operator can act on")
					}
					return
				}
				if fact.State != provider.StateResolved {
					t.Fatalf("state=%q reason=%q — %s", fact.State, fact.Reason, tc.why)
				}
				if got := scalar(fact.Value); got != tc.wantValue {
					t.Fatalf("value=%s want %s (%s)", got, tc.wantValue, tc.why)
				}
			})
		}
	}
}

// TestRepoFileSymlinkRefusalIsDistinguishable — the refusal must be reported as a
// containment refusal naming the offending candidate, not silently folded into the
// generic "no matching file" miss: an operator has to be able to see an MR trying
// to read outside the roots. Fail direction stays unavailable (same class as
// "anchor outside declared roots"), never resolved.
func TestRepoFileSymlinkRefusalIsDistinguishable(t *testing.T) {
	requireSymlinks(t)
	repo, outside := symlinkTree(t)
	mustSymlink(t, filepath.Join(outside, "cluster-secrets.yaml"),
		filepath.Join(repo, "topics", "prod", "quota.yaml"))

	fact := repoFileFact(t, builtin.RepoFileOpts{
		FS:     os.DirFS(repo),
		File:   "quota.yaml",
		Anchor: "topics/prod/orders.yaml",
		Roots:  []string{"topics"},
	}, "q-d129-reason")

	if fact.State != provider.StateUnavailable {
		t.Fatalf("state=%q want unavailable (containment refusal, not malformed input)", fact.State)
	}
	if !strings.Contains(fact.Reason, "symlink") {
		t.Fatalf("reason %q must name the symlink refusal", fact.Reason)
	}
	if !strings.Contains(fact.Reason, "topics/prod/quota.yaml") {
		t.Fatalf("reason %q must name the refused candidate", fact.Reason)
	}
	// ADR-0012: contributor-facing text, never a Go/OS internal.
	for _, internal := range []string{"statat", "openat", "escapes from parent", "*os."} {
		if strings.Contains(fact.Reason, internal) {
			t.Fatalf("reason %q leaks a Go/OS internal into MR-facing text", fact.Reason)
		}
	}
}

// openFlavour builds the injected filesystem under test.
//
//	dirfs  — os.DirFS: explicitly NOT a security boundary (Go docs). Cases must
//	         still be refused here: that is the builtin's own guarantee.
//	rootfs — (*os.Root).FS(): the symlink-safe root cmd/assent now injects.
func openFlavour(t *testing.T, flavour, dir string) fs.FS {
	t.Helper()
	switch flavour {
	case "dirfs":
		return os.DirFS(dir)
	case "rootfs":
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatalf("open root %s: %v", dir, err)
		}
		t.Cleanup(func() { _ = root.Close() })
		return root.FS()
	default:
		t.Fatalf("unknown fs flavour %q", flavour)
		return nil
	}
}

// scalar renders a fact value the way the wire (and the MR comment) would see it.
func scalar(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "<unmarshalable>"
	}
	return string(raw)
}

// TestOpenRepoRoot pins the contract helper hosts must use: a real directory
// yields a symlink-safe FS that refuses an escaping read, and a bad directory is
// an error (never a silently unusable FS).
func TestOpenRepoRoot(t *testing.T) {
	requireSymlinks(t)
	repo, outside := symlinkTree(t)
	mustSymlink(t, filepath.Join(outside, "cluster-secrets.yaml"),
		filepath.Join(repo, "topics", "prod", "quota.yaml"))

	fsys, closer, err := builtin.OpenRepoRoot(repo)
	if err != nil {
		t.Fatalf("OpenRepoRoot(%s): %v", repo, err)
	}
	defer func() { _ = closer.Close() }()

	if _, err := fs.ReadFile(fsys, "topics/quota.yaml"); err != nil {
		t.Fatalf("a real in-root file must still read: %v", err)
	}
	if raw, err := fs.ReadFile(fsys, "topics/prod/quota.yaml"); err == nil {
		t.Fatalf("escaping symlink read through the root FS: %q", raw)
	}

	t.Run("missing_dir_is_an_error", func(t *testing.T) {
		if _, _, err := builtin.OpenRepoRoot(filepath.Join(repo, "no-such-dir")); err == nil {
			t.Fatal("OpenRepoRoot must fail loudly on a directory it cannot open")
		}
	})
}

// TestRepoFileAnchorRootHasNoPrefixes covers the degenerate walk: an empty anchor
// resolves the repo-root candidate with no directory components to inspect.
func TestRepoFileAnchorRootHasNoPrefixes(t *testing.T) {
	repo, _ := symlinkTree(t)
	fact := repoFileFact(t, builtin.RepoFileOpts{
		FS:     os.DirFS(repo),
		File:   "quota.yaml",
		Anchor: "",
	}, "q-d129-root-anchor")
	if fact.State != provider.StateResolved || scalar(fact.Value) != "12" {
		t.Fatalf("root-anchored lookup = %q/%s, want resolved/12", fact.State, scalar(fact.Value))
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}
