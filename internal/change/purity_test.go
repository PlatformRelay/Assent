package change

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// forbiddenImports are packages whose presence would let the differ read a clock, entropy,
// the environment, or the network — breaking the ADR-0011 purity invariant that makes the
// output a deterministic function of its inputs (GUIDELINES §5). This guard is local to
// internal/change; the cross-package purity test (internal/core) is owned by S01.
var forbiddenImports = []string{
	"os",
	"os/exec",
	"time",
	"math/rand",
	"crypto/rand",
	"net",
	"net/http",
}

// TestChangePackagePurity parses every non-test .go file in this package and fails if any
// imports a forbidden package. Adversarial guard: adding e.g. an os.Getenv call (which needs
// importing "os") into diff.go fails this check.
func TestChangePackagePurity(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	forbidden := make(map[string]struct{}, len(forbiddenImports))
	for _, p := range forbiddenImports {
		forbidden[p] = struct{}{}
	}

	fset := token.NewFileSet()
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", name, err)
			}
			if _, bad := forbidden[path]; bad {
				t.Errorf("%s imports forbidden package %q — internal/change must stay pure (no clock/env/network/random)", name, path)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no non-test source files scanned — purity guard would vacuously pass")
	}
}
