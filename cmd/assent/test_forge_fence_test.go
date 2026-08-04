package main

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// TestAssentTestNeverCallsForgeResolve — REQ-E4-S06-03 / E6 fence: `assent test`
// stubs ApprovalEvidence via approval.yaml and must NEVER invoke live forge Resolve.
func TestAssentTestNeverCallsForgeResolve(t *testing.T) {
	forbidForgeResolveImport(t, "test.go")

	adopterDir := strings.Join([]string{"..", "..", "internal", "adoptertest"}, string(os.PathSeparator))
	entries, err := os.ReadDir(adopterDir)
	if err != nil {
		t.Fatalf("read adoptertest: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := adopterDir + string(os.PathSeparator) + e.Name()
		src, err := os.ReadFile(path) // #nosec G304 -- test-controlled package path
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(src)
		for _, banned := range []string{".Resolve(", "ResolveRequest", "resolveRunApproval("} {
			if strings.Contains(text, banned) {
				t.Errorf("adoptertest %s references %s — assent test must stub approval, never live Resolve", e.Name(), banned)
			}
		}
	}
}

func forbidForgeResolveImport(t *testing.T, file string) {
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
		if p == "github.com/PlatformRelay/assent/internal/forge/gitlab" {
			t.Errorf("%s imports gitlab adapter — assent test must not call live forge Resolve", file)
		}
	}
	text := string(src)
	for _, banned := range []string{".Resolve(", "resolveRunApproval(", "client.Resolve("} {
		if strings.Contains(text, banned) {
			t.Errorf("%s calls %s — E6 fence: assent test must never invoke live forge Resolve", file, banned)
		}
	}
}
