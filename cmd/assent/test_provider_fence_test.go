package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAssentTestNeverCallsProviderHost — REQ-E5-S05-03 / ADR-0014 / E6 fence:
// `assent test` stubs facts via facts.yaml (adoptertest.MapFacts) and must NEVER
// import or invoke the live provider host (ResolveFacts / CallHTTP / CallExec).
func TestAssentTestNeverCallsProviderHost(t *testing.T) {
	// 1. cmd/assent/test.go must not import internal/provider.
	forbidProviderImport(t, "test.go")

	// 2. The pure adoptertest harness (facts.yaml → resolved envelope) must not
	//    pull in the live host either — MapFacts is the only fact path for tests.
	adopterDir := filepath.Join("..", "..", "internal", "adoptertest")
	entries, err := os.ReadDir(adopterDir)
	if err != nil {
		t.Fatalf("read adoptertest: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(adopterDir, e.Name())
		src, err := os.ReadFile(path) // #nosec G304 -- test-controlled package path
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		file, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if p == "github.com/PlatformRelay/assent/internal/provider" ||
				strings.HasPrefix(p, "github.com/PlatformRelay/assent/internal/provider/") {
				t.Errorf("adoptertest %s imports live provider host %q — E6 fence broken (facts.yaml stubs only)", e.Name(), p)
			}
		}
		text := string(src)
		for _, banned := range []string{"ResolveFacts", "ResolveFactsChecked", "CallHTTP", "CallExec"} {
			if strings.Contains(text, banned) {
				t.Errorf("adoptertest %s references %s — assent test must stub via MapFacts, never live resolve", e.Name(), banned)
			}
		}
	}
}

func forbidProviderImport(t *testing.T, file string) {
	t.Helper()
	src, err := os.ReadFile(file) // #nosec G304 -- test-controlled source next to this file
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if p == "github.com/PlatformRelay/assent/internal/provider" ||
			strings.HasPrefix(p, "github.com/PlatformRelay/assent/internal/provider/") {
			t.Errorf("%s imports live provider host %q — assent test must stay on facts.yaml stubs (ADR-0014)", file, p)
		}
	}
	text := string(src)
	for _, banned := range []string{"ResolveFacts(", "ResolveFactsChecked(", "CallHTTP(", "CallExec(", "resolveRunFacts("} {
		if strings.Contains(text, banned) {
			t.Errorf("%s calls %s — E6 fence: assent test must never invoke the live provider host", file, banned)
		}
	}
}
