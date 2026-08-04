// Package core is intentionally empty of production code at P4-E1-S01: this
// file is the arch/purity guard proving the determinism boundary (GUIDELINES
// §5, ADR-0013). internal/core/** and internal/change/** must reference no
// os.Getenv, os.Environ, time.Now, rand, or a network package — env and clock
// enter only through cmd/assent (an injected clock + pinned CI vars). As a
// test-only package, core contributes zero statements to the D-010 internal
// coverage aggregate.
package core

import (
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

// scanTree walks dir (if it exists) and returns violations across every
// non-test .go file. _test.go files are skipped: this guard file itself names
// the forbidden tokens as string literals, and scanning tests would flag it.
func scanTree(t *testing.T, dir string) []violation {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		// internal/change is built by a parallel lane and may not exist yet;
		// absence is not a failure — the guard must at least cover core.
		return nil
	}
	fset := token.NewFileSet()
	var all []violation
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
		all = append(all, vs...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return all
}

// TestCorePurity asserts internal/core/** and internal/change/** reference no
// os.Getenv/os.Environ, time.Now, rand, or network package (REQ-P4-E1-S01-03).
func TestCorePurity(t *testing.T) {
	// The test runs with cwd = the package dir (internal/core), so "." is core,
	// "../change" is the sibling differ tree, "../glob" is the shared glob
	// matcher the decision path (aggregate coverage) depends on for determinism,
	// and "../lint" is the E3 policy-lint check library (pure: it takes
	// already-read bytes/types, all I/O lives in cmd/assent) — all guarded pure
	// (no clock/rand/env/net).
	for _, dir := range []string{".", "../change", "../glob", "../lint"} {
		vs := scanTree(t, dir)
		if len(vs) > 0 {
			sort.Slice(vs, func(i, j int) bool { return vs[i].pos < vs[j].pos })
			for _, v := range vs {
				t.Errorf("purity violation in %s: %s at %s", dir, v.detail, v.pos)
			}
		}
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
