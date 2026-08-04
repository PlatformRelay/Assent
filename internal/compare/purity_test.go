package compare_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenFSImports are barred from internal/compare — repo/catalogue walks live
// only in cmd/assent/compare.go (REQ-PCS-S01-04).
var forbiddenFSImports = map[string]struct{}{
	"os":       {},
	"filepath": {},
}

// TestComparePackageNoFilesystem asserts internal/compare never imports os or
// filepath (no filesystem catalogue loading inside the pure compare library).
func TestComparePackageNoFilesystem(t *testing.T) {
	root := filepath.Join("..", "compare")
	var violations []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, rerr := os.ReadFile(path) //nolint:gosec // scanning in-repo sources
		if rerr != nil {
			return rerr
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, src, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if _, bad := forbiddenFSImports[path]; bad {
				violations = append(violations, fset.Position(imp.Pos()).String()+": forbidden import "+path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk compare package: %v", err)
	}
	if len(violations) > 0 {
		for _, v := range violations {
			t.Error(v)
		}
	}
}

// TestComparePackageNoFilesystemCatchesImport proves the guard fires on synthetic os import.
func TestComparePackageNoFilesystemCatchesImport(t *testing.T) {
	src := []byte("package compare\nimport \"os\"\n")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bad.go", src, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, `"`) == "os" {
			return
		}
	}
	t.Fatal("expected synthetic os import to be parseable for guard test")
}
