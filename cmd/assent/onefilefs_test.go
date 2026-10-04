package main

import (
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// TestProviderHostDoesNotImportTestingFstest pins REQ-XREV-S04-03 (D18): the
// shipped provider host must not import testing/fstest, which would link a test
// package into the release binary. The run_checkout_containment_test.go test file
// may still import it (test-only, never linked); this scans the non-test source.
func TestProviderHostDoesNotImportTestingFstest(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "provider_host.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse provider_host.go: %v", err)
	}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("unquote import %s: %v", imp.Path.Value, err)
		}
		if p == "testing/fstest" {
			t.Fatal("provider_host.go imports testing/fstest — the release binary must not link it (D18; use oneFileFS)")
		}
	}
}
