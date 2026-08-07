// Package core is intentionally empty of production code at P4-E1-S01: this
// file is the arch/purity guard proving the determinism boundary (GUIDELINES
// §5, ADR-0013, ADR-0011 Amendment 3). The guarded tree must reference no
// os.Getenv, os.Environ, time.Now, rand, or a network package — env and clock
// enter only through cmd/assent (an injected clock + pinned CI vars). As a
// test-only package, core contributes zero statements to the D-010 internal
// coverage aggregate.
//
// This walk is the CALL-level half of the D-123 boundary enforcement; the
// PACKAGE-level half (no port implementation, no net/** import anywhere in the
// same tree) is the golangci-lint depguard `pure-tree` rule in .golangci.yml,
// because depguard cannot tell `time.Now` from the `time.Time` type.
package core

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// forbiddenSelectors are pkg.Func call shapes barred from the pure engine tree.
var forbiddenSelectors = map[string][]string{
	"os":   {"Getenv", "Environ", "LookupEnv"},
	"time": {"Now"},
}

// forbiddenImportPrefixes are import paths barred from the pure engine tree:
// randomness and any network stack. crypto/rand and math/rand are both barred
// (nondeterminism); net and net/http pull the process off-box mid-decision.
var forbiddenImportPrefixes = []string{
	"math/rand",
	"crypto/rand",
	"net", // matches "net" and "net/*" (see importForbidden).
}

// violation is one impurity found in a scanned source file.
type violation struct {
	pos    string
	detail string
}

// importForbidden reports whether an import path is barred. "net" matches the
// net package and any net/* subpackage, but not unrelated paths that merely
// start with the letters "net" (e.g. a hypothetical "netbox").
func importForbidden(path string) bool {
	for _, pre := range forbiddenImportPrefixes {
		if path == pre || strings.HasPrefix(path, pre+"/") {
			return true
		}
	}
	return false
}

// scanSource parses one Go source file (name for positions, src bytes) and
// returns every purity violation: barred imports and barred pkg.Func selectors.
// It is the single check both the real tree walk and the adversarial in-memory
// negative case call, so the guard's logic is proven by a synthetic snippet
// and never has to scan a deliberately-impure file into the tree.
func scanSource(fset *token.FileSet, name string, src []byte) ([]violation, error) {
	f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	// Which local identifiers are bound to a forbidden package by import, so
	// `import t "time"; t.Now()` is caught, and a shadowed name is not.
	pkgAlias := map[string]string{} // localName -> canonical pkg ("os"/"time")
	var out []violation
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if importForbidden(path) {
			out = append(out, violation{pos: fset.Position(imp.Pos()).String(), detail: "forbidden import " + path})
		}
		if _, watched := forbiddenSelectors[path]; watched {
			local := path[strings.LastIndex(path, "/")+1:]
			if imp.Name != nil {
				local = imp.Name.Name
			}
			pkgAlias[local] = path
		}
	}

	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		canonical, watched := pkgAlias[ident.Name]
		if !watched {
			return true
		}
		for _, fn := range forbiddenSelectors[canonical] {
			if sel.Sel.Name == fn {
				out = append(out, violation{
					pos:    fset.Position(sel.Pos()).String(),
					detail: canonical + "." + fn,
				})
			}
		}
		return true
	})
	return out, nil
}

// scanTree walks dir and returns the violations found across every non-test
// .go file, together with the number of files actually scanned. _test.go files
// are skipped: this guard file itself names the forbidden tokens as string
// literals, and scanning tests would flag it.
//
// A missing or empty directory is NOT tolerated. The guard used to return nil
// for an absent dir (internal/change was being built by a parallel lane at
// P4-E1-S01); with the D-123 tree that leniency is a fail-open — a mistyped or
// moved directory would make the walk silently scan nothing and report green.
// The caller asserts scanned > 0, which is this guard's positive control.
func scanTree(dir string) (all []violation, scanned int, err error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, 0, fmt.Errorf("guarded directory %s is not readable (D-123: the walk must not silently cover nothing): %w", dir, err)
	}
	if !info.IsDir() {
		return nil, 0, fmt.Errorf("guarded path %s is not a directory (D-123)", dir)
	}
	fset := token.NewFileSet()
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path) // #nosec G304,G122 -- path comes from walking a fixed in-repo source tree at test time, not user input; no symlink surface.
		if err != nil {
			return err
		}
		vs, err := scanSource(fset, path, src)
		if err != nil {
			return err
		}
		scanned++
		all = append(all, vs...)
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("walk %s: %w", dir, err)
	}
	return all, scanned, nil
}

// guardedDirs is the D-123 pure tree, relative to this package directory
// (tests run with cwd = internal/core, so ".." is internal/ and "../.." is the
// repo root):
//
//   - "."            — internal/core/** (engine).
//   - "../change"    — the differ tree.
//   - "../glob"      — the shared glob matcher the decision path (aggregate
//     coverage) depends on for determinism.
//   - "../lint"      — the E3 policy-lint check library (pure: it takes
//     already-read bytes/types; all I/O lives in cmd/assent).
//   - "../catalogue" — the E3-S07 generated rule catalogue (pure: it walks
//     already-loaded policy.* types; the `.assent/**` walk lives in cmd/assent).
//   - "../evaldecode" — D-123: engine INPUT decode sits on the decision path.
//   - "../compare"    — D-123: the D-116/D-117 compare gates must be
//     reproducible, so they inherit the determinism guard.
//   - "../../schemas" — D-123: embedded compile-time contract authority.
//
// The last three deliberately EXTEND the AGENTS.md rule-7 tree; the extension
// and its rationale are recorded in D-123 and ADR-0011 Amendment 3.
var guardedDirs = []string{
	".", "../change", "../glob", "../lint", "../catalogue",
	"../evaldecode", "../compare", "../../schemas",
}

// TestCorePurity asserts the D-123 guarded tree references no os.Getenv/
// os.Environ/os.LookupEnv, time.Now, rand, or network package
// (REQ-P4-E1-S01-03, REQ-AUD-S07-02).
func TestCorePurity(t *testing.T) {
	for _, dir := range guardedDirs {
		vs, scanned, err := scanTree(dir)
		if err != nil {
			t.Errorf("purity walk over %s: %v", dir, err)
			continue
		}
		// Positive control: a walk that scanned nothing would report green
		// forever. Every guarded directory holds production Go sources today,
		// so zero scanned files means the path is wrong, not that the tree is
		// clean. Asserted per directory so a single stale path cannot hide.
		if scanned == 0 {
			t.Errorf("guarded directory %s: scanned 0 non-test .go files — the purity walk covers nothing there (D-123)", dir)
		}
		if len(vs) > 0 {
			sort.Slice(vs, func(i, j int) bool { return vs[i].pos < vs[j].pos })
			for _, v := range vs {
				t.Errorf("purity violation in %s: %s at %s", dir, v.detail, v.pos)
			}
		}
	}
}

// TestCorePurityWalkReachesEveryGuardedDir is the anti-tautology control for
// the walk itself (as distinct from TestCorePurityCatchesImpurity, which proves
// the SCANNER). It plants a synthetic impurity inside a copy of the guarded
// tree layout under t.TempDir() and asserts the walk finds it — proving that
// scanTree really opens and scans the files it claims to, and that a
// non-existent directory is a hard failure rather than a silent pass.
func TestCorePurityWalkReachesEveryGuardedDir(t *testing.T) {
	for _, dir := range guardedDirs {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(dir, "../", "up-"), ".", "self"), func(t *testing.T) {
			// Mirror the real relative path under a temp root so the exact
			// string used in production is exercised, not a simplified stand-in.
			root := t.TempDir()
			from := filepath.Join(root, "internal", "core")
			target := filepath.Clean(filepath.Join(from, dir))
			if err := os.MkdirAll(target, 0o750); err != nil {
				t.Fatalf("mkdir %s: %v", target, err)
			}
			if err := os.MkdirAll(from, 0o750); err != nil {
				t.Fatalf("mkdir %s: %v", from, err)
			}
			src := "package p\n\nimport (\n\t\"os\"\n\t\"time\"\n)\n\nfunc leak() (string, interface{}) { return os.Getenv(\"X\"), time.Now() }\n"
			if err := os.WriteFile(filepath.Join(target, "planted.go"), []byte(src), 0o600); err != nil {
				t.Fatalf("write planted source: %v", err)
			}

			t.Chdir(from)

			vs, scanned, err := scanTree(dir)
			if err != nil {
				t.Fatalf("scan planted tree at %s: %v", dir, err)
			}
			if scanned != 1 {
				t.Fatalf("expected the walk to scan the 1 planted file under %s, scanned %d", dir, scanned)
			}
			var details []string
			for _, v := range vs {
				details = append(details, v.detail)
			}
			sort.Strings(details)
			if len(details) != 2 || details[0] != "os.Getenv" || details[1] != "time.Now" {
				t.Fatalf("planted impurity under %s not reported: got %#v", dir, details)
			}
		})
	}
}

// TestCorePurityWalkFailsClosedOnMissingDir proves the fail-closed half of
// scanTree: a guarded path that does not exist must abort the guard, never pass
// green. Before D-123 this returned (nil, nil) — a stale path in guardedDirs
// would have made TestCorePurity report a clean tree it never read.
func TestCorePurityWalkFailsClosedOnMissingDir(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	if _, _, err := scanTree("./definitely-not-here"); err == nil {
		t.Error("scanTree accepted a missing guarded directory — the walk would silently cover nothing")
	}
	// A path that exists but is a FILE is equally a mis-specified guard.
	file := filepath.Join(root, "notadir.go")
	if err := os.WriteFile(file, []byte("package p\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", file, err)
	}
	if _, _, err := scanTree("./notadir.go"); err == nil {
		t.Error("scanTree accepted a file as a guarded directory")
	}
	// Positive control: a directory that DOES exist must not error, otherwise
	// the two assertions above would pass no matter what scanTree did.
	if _, _, err := scanTree("."); err != nil {
		t.Errorf("scanTree rejected an existing directory: %v — the control is broken", err)
	}
}

// TestCorePurityCatchesImpurity is the adversarial subcase: it proves the guard
// WOULD fail if an os.Getenv (or time.Now, or a net import) were present in the
// pure tree — checked against a synthetic in-memory snippet so the impure code
// never enters the scanned tree and can never actually contaminate core.
func TestCorePurityCatchesImpurity(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "os.Getenv",
			src:  "package core\nimport \"os\"\nfunc leak() string { return os.Getenv(\"SECRET\") }\n",
			want: "os.Getenv",
		},
		{
			name: "time.Now",
			src:  "package core\nimport \"time\"\nfunc leak() interface{} { return time.Now() }\n",
			want: "time.Now",
		},
		{
			name: "aliased time.Now",
			src:  "package core\nimport t \"time\"\nfunc leak() interface{} { return t.Now() }\n",
			want: "time.Now",
		},
		{
			name: "net import",
			src:  "package core\nimport _ \"net/http\"\n",
			want: "forbidden import net/http",
		},
		{
			name: "math/rand import",
			src:  "package core\nimport \"math/rand\"\nfunc r() int { return rand.Int() }\n",
			want: "forbidden import math/rand",
		},
	}
	fset := token.NewFileSet()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vs, err := scanSource(fset, tc.name+".go", []byte(tc.src))
			if err != nil {
				t.Fatalf("parse synthetic snippet: %v", err)
			}
			found := false
			for _, v := range vs {
				if v.detail == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected the purity scan to flag %q, got %#v", tc.want, vs)
			}
		})
	}
}
