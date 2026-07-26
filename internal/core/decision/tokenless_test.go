package decision_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/decision"
)

// REQ-P4-E1-S07-03 — the approve/merge CREDENTIAL is structurally unreachable
// from the evaluation/decision code path (ADR-0015 §7: providers run tokenless).
// The one `assert` reads only old/new/changes — no fact, no token. This file
// enforces that boundary in TWO independent ways, each with a POSITIVE CONTROL
// so neither degenerates into a tautology that would pass regardless:
//
//  1. IMPORT SCAN — the pure decision/aggregate packages do not import
//     internal/forge (or any write-token-carrying forge package). A synthetic
//     snippet that DOES import forge is flagged (control).
//  2. FIELD SCAN — the public decision-function INPUT types (aggregate.Binding,
//     aggregate.Result, change.ChangeSet reached via Binding, decision.Pins)
//     carry no credential/token/secret field, recursively. A synthetic struct
//     that DOES carry a nested Token field is flagged (control).
//
// If someone THREADED a write token into the decision inputs — a token field on
// Pins/Binding/Result, or a direct internal/forge import into internal/core —
// the corresponding assertion below FAILS. The import scan reads the AST import
// PATH STRINGS only (never `go list -deps`, never a reference to the forge
// package), so it needs no forge package to exist: the sibling forge lane can be
// absent and this test still enforces the absence-of-dependency.

// ---- shared detectors (also driven by the positive controls) ----

// forgeWriteImportPrefixes are the import paths whose presence in the pure
// decision path would put a write-capable forge dependency (and thus a merge/
// approve credential surface) in reach of the evaluation function.
var forgeWriteImportPrefixes = []string{
	"github.com/PlatformRelay/assent/internal/forge",
}

// importReaches reports whether an import path is (or is under) a barred
// write-token-carrying forge package.
func importReaches(path string) bool {
	for _, pre := range forgeWriteImportPrefixes {
		if path == pre || strings.HasPrefix(path, pre+"/") {
			return true
		}
	}
	return false
}

// scanImports parses one Go source (name for positions, src bytes) and returns
// the barred forge import paths it declares. Non-test files only are fed in by
// the tree walk; the control feeds a synthetic snippet.
func scanImports(fset *token.FileSet, name string, src []byte) ([]string, error) {
	f, err := parser.ParseFile(fset, name, src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var bad []string
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if importReaches(p) {
			bad = append(bad, p)
		}
	}
	return bad, nil
}

// tokenFieldNameSubstrings are field-name fragments that name a write
// credential. A public input field whose name contains any of these would be a
// threaded-in token — the exact adversarial vector.
var tokenFieldNameSubstrings = []string{"token", "credential", "secret", "auth", "password", "apikey", "bearer"}

// suspiciousFieldName reports whether a struct field name looks like a write
// credential.
func suspiciousFieldName(name string) bool {
	low := strings.ToLower(name)
	for _, frag := range tokenFieldNameSubstrings {
		if strings.Contains(low, frag) {
			return true
		}
	}
	return false
}

// scanTypeForCredential walks a type recursively (structs, pointers, slices,
// maps, arrays) collecting any field whose NAME looks like a credential. It
// guards against cycles via a seen set keyed on the reflect.Type.
func scanTypeForCredential(t reflect.Type, path string, seen map[reflect.Type]bool, out *[]string) {
	if t == nil || seen[t] {
		return
	}
	seen[t] = true
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		scanTypeForCredential(t.Elem(), path+"[]", seen, out)
	case reflect.Map:
		scanTypeForCredential(t.Key(), path+"{key}", seen, out)
		scanTypeForCredential(t.Elem(), path+"{val}", seen, out)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			fieldPath := path + "." + f.Name
			if suspiciousFieldName(f.Name) {
				*out = append(*out, fieldPath)
			}
			scanTypeForCredential(f.Type, fieldPath, seen, out)
		}
	}
}

// ---- the boundary assertions ----

// TestEvaluationIsProviderless is the top-level S07-03 boundary test. It runs
// both scans over the REAL trees/types and asserts the write credential is
// structurally unreachable.
func TestEvaluationIsProviderless(t *testing.T) {
	t.Run("no forge import in the pure decision path", func(t *testing.T) {
		// "." is internal/core/decision (cwd at test time); "../aggregate" is the
		// evaluation core; "../classify" is the routing seam that sits in the eval
		// path. None may import a write-token-carrying forge package.
		for _, dir := range []string{".", "../aggregate", "../classify"} {
			bad := scanImportsTree(t, dir)
			for _, b := range bad {
				t.Errorf("forge write-token import reachable from pure decision path: %s", b)
			}
		}
	})

	t.Run("decision input types carry no credential field", func(t *testing.T) {
		inputs := []struct {
			name string
			typ  reflect.Type
		}{
			// aggregate.Aggregate's public inputs are (Binding, change.ChangeSet,
			// string): Binding embeds Rule/OnFailure, and ChangeSet embeds Change —
			// BOTH are scanned so a token threaded onto Change (not just Binding)
			// is caught. aggregate.Result is Build's input; decision.Pins is Build's
			// other input.
			{"aggregate.Binding", reflect.TypeOf(aggregate.Binding{})},
			{"change.ChangeSet", reflect.TypeOf(change.ChangeSet{})},
			{"aggregate.Result", reflect.TypeOf(aggregate.Result{})},
			{"decision.Pins", reflect.TypeOf(decision.Pins{})},
		}
		for _, in := range inputs {
			var found []string
			scanTypeForCredential(in.typ, in.name, map[reflect.Type]bool{}, &found)
			for _, f := range found {
				t.Errorf("credential-shaped field reachable from decision input %s: %s", in.name, f)
			}
		}
	})
}

// TestTokenlessScansAreNonTautological is the ADVERSARIAL control: it proves
// BOTH detectors WOULD flag a threaded-in write token, so the green result above
// is real. Without these, either scan could be a no-op that passes regardless.
func TestTokenlessScansAreNonTautological(t *testing.T) {
	t.Run("import scan flags a forge import", func(t *testing.T) {
		// A synthetic core-package source that imports internal/forge — the exact
		// forbidden dependency. It never enters the scanned tree.
		src := "package aggregate\n" +
			"import _ \"github.com/PlatformRelay/assent/internal/forge\"\n"
		bad, err := scanImports(token.NewFileSet(), "synthetic.go", []byte(src))
		if err != nil {
			t.Fatalf("parse synthetic snippet: %v", err)
		}
		if len(bad) == 0 {
			t.Fatal("import scan failed to flag a synthetic internal/forge import — the boundary check is a tautology")
		}
	})

	t.Run("field scan flags a threaded-in token", func(t *testing.T) {
		// A synthetic input type with a write token threaded in via a NESTED
		// struct — proving the recursion (not just top-level fields) catches it.
		type forgeCreds struct{ MergeToken string }
		type poisonedPins struct {
			PolicySha string
			Write     forgeCreds // nested credential — must be caught
		}
		var found []string
		scanTypeForCredential(reflect.TypeOf(poisonedPins{}), "poisonedPins", map[reflect.Type]bool{}, &found)
		if len(found) == 0 {
			t.Fatal("field scan failed to flag a nested MergeToken — the boundary check is a tautology")
		}
	})
}

// scanImportsTree walks dir and returns every barred forge import across
// non-test .go files. _test.go files are skipped: this very test names the forge
// path as a string literal and would otherwise flag itself.
func scanImportsTree(t *testing.T, dir string) []string {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("scan dir %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	var all []string
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path) // #nosec G304,G122 -- fixed in-repo source tree walked at test time, not user input; no symlink surface.
		if err != nil {
			return err
		}
		bad, err := scanImports(fset, path, src)
		if err != nil {
			return err
		}
		all = append(all, bad...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return all
}
