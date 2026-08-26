package conformance

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// helpers_unit_test.go covers the pure helpers the extraction moved into
// importable Go. They were previously `_test.go` bodies exercised only as a side
// effect of a case running, so their fallback branches had never been executed by
// anything — moving them into production code made that visible rather than
// creating it.

func TestLoadCatalogMissingFileIsAnError(t *testing.T) {
	_, err := LoadCatalog("does-not-exist.yaml")
	if err == nil {
		t.Fatal("a missing catalog must be an error, not an empty catalog — an empty " +
			"catalog would make the coverage gate vacuously pass")
	}
	if !strings.Contains(err.Error(), "conformance catalog") {
		t.Fatalf("error must name the subsystem, got %v", err)
	}
}

func TestDecodeCatalogRejectsMalformedYAML(t *testing.T) {
	if _, err := DecodeCatalog([]byte("cases: [oh: no: yes")); err == nil {
		t.Fatal("malformed YAML must be rejected")
	}
}

// TestThreadOpTargetFallbacks pins the selection order: the operation matching the
// wanted id wins; failing that the FIRST operation is reported; failing that the
// empty string. The middle branch is what makes a receipt with operations but no
// match distinguishable from an empty receipt.
func TestThreadOpTargetFallbacks(t *testing.T) {
	want := forge.PublicationReceipt{Operations: []forge.Operation{
		{TargetID: "note/1"}, {TargetID: "note/2"},
	}}
	if got := threadOpTarget(want, "note/2"); got != "note/2" {
		t.Fatalf("exact match must win, got %q", got)
	}
	if got := threadOpTarget(want, "note/absent"); got != "note/1" {
		t.Fatalf("no match must fall back to the first operation, got %q", got)
	}
	if got := threadOpTarget(forge.PublicationReceipt{}, "note/1"); got != "" {
		t.Fatalf("an empty receipt must yield the empty string, got %q", got)
	}
}

func TestSuiteCaseIDsExcludesOtherPackagesAndDeferred(t *testing.T) {
	c := Catalog{Cases: []CatalogCase{
		{ID: "in-suite", Package: ConformancePackage, Forge: "gitlab"},
		{ID: "other-package", Package: "cmd/assent", Forge: "gitlab"},
		{ID: "deferred", Package: ConformancePackage, Forge: DeferredForge},
	}}
	got := c.SuiteCaseIDs()
	if len(got) != 1 || got[0] != "in-suite" {
		t.Fatalf("the denominator must be conformance-package, non-deferred rows only; got %v", got)
	}
}

// TestKnownAdapterListIsSorted keeps the error message deterministic — an
// unordered map render would make the strict-decode failure text flaky.
func TestKnownAdapterListIsSorted(t *testing.T) {
	got := knownAdapterList()
	if got != "[fake github gitlab]" {
		t.Fatalf("adapter list must render sorted and complete, got %q", got)
	}
}
