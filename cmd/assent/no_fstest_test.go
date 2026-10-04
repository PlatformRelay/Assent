package main

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestCmdAssentDoesNotImportTestingFstest pins REQ-XREV-S04-03 (D18): no non-test
// source file in the shipped `assent` binary may import testing/fstest, which would
// link a test package into the release binary. Test files may import it (never
// linked); they are skipped.
func TestCmdAssentDoesNotImportTestingFstest(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	fset := token.NewFileSet()
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		scanned++
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import %s in %s: %v", imp.Path.Value, name, err)
			}
			if p == "testing/fstest" {
				t.Fatalf("%s imports testing/fstest — the release binary must not link it (D18; use oneFileFS)", name)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no non-test .go files scanned — the fstest-import pin would be vacuous")
	}
}
