package conformance

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// catalog.go loads the forge-neutral conformance catalog (E10-S01,
// REQ-E10-S01-03).
//
// It is STRICT (P3-E2): an unknown field or an unknown adapter name is an error,
// never a silently ignored row. That matters specifically here — the catalog is
// the artifact that claims which backends a case is proven against, so a typo'd
// adapter name that decoded to "nothing" would make the catalog overstate
// coverage while staying green. Silent tolerance in a coverage index is
// indistinguishable from lying about coverage.

// ConformancePackage is the package whose rows RunSuite executes. Rows in other
// packages (cmd/assent, test/e2e) are catalogued conformance cases too, but they
// are driven by their own tests, so they are outside the suite's denominator.
const ConformancePackage = "internal/forge/conformance"

// DeferredForge marks a row nothing executes yet.
const DeferredForge = "github-deferred"

// KnownAdapters is the closed set of adapter names a row may list. It is closed
// on purpose: adding "github" here is a deliberate act in E10-S06+, not something
// a catalog row can do on its own by naming a backend that does not exist.
var KnownAdapters = map[string]bool{
	"fake":   true,
	"gitlab": true,
	"github": true,
}

// CatalogCase is one row.
type CatalogCase struct {
	ID       string   `yaml:"id"`
	ADR      string   `yaml:"adr"`
	Level    string   `yaml:"level"`
	Req      string   `yaml:"req"`
	Test     string   `yaml:"test"`
	Package  string   `yaml:"package"`
	Forge    string   `yaml:"forge"`
	Adapters []string `yaml:"adapters"`
	Note     string   `yaml:"note"`
}

// Catalog is the decoded catalog file.
type Catalog struct {
	Cases []CatalogCase `yaml:"cases"`
}

// DecodeCatalog strict-decodes the catalog from raw YAML.
func DecodeCatalog(raw []byte) (Catalog, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var c Catalog
	if err := dec.Decode(&c); err != nil {
		return Catalog{}, fmt.Errorf("conformance catalog: %w", err)
	}
	for _, row := range c.Cases {
		for _, a := range row.Adapters {
			if !KnownAdapters[a] {
				return Catalog{}, fmt.Errorf(
					"conformance catalog: case %q lists unknown adapter %q (known: %s)",
					row.ID, a, knownAdapterList())
			}
		}
	}
	return c, nil
}

// LoadCatalog reads and strict-decodes the catalog at path.
func LoadCatalog(path string) (Catalog, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // co-located catalog fixture.
	if err != nil {
		return Catalog{}, fmt.Errorf("conformance catalog: %w", err)
	}
	return DecodeCatalog(raw)
}

// SuiteCaseIDs is the catalog's claim about what RunSuite executes: every row in
// the conformance package that is not deferred. This is one half of
// REQ-E10-S01-02; the other half is what running the suite ACTUALLY dispatches,
// and the gate fails when they differ in either direction.
func (c Catalog) SuiteCaseIDs() []string {
	var ids []string
	for _, row := range c.Cases {
		if row.Package != ConformancePackage || row.Forge == DeferredForge {
			continue
		}
		ids = append(ids, row.ID)
	}
	sort.Strings(ids)
	return ids
}

func knownAdapterList() string {
	var names []string
	for a := range KnownAdapters {
		names = append(names, a)
	}
	sort.Strings(names)
	return fmt.Sprint(names)
}

// DiffCaseSets reports catalog rows with no executed case (missing) and executed
// cases with no catalog row (extra). Both directions matter: `missing` catches a
// case that stopped running while its row still claims coverage, and `extra`
// catches a case running with no catalogued disposition at all.
//
// It is a pure function on two slices precisely so it can be MUTATION-TESTED —
// the gate that uses it feeds it real data, and TestDiffCaseSetsDetectsDrift
// feeds it deliberately broken data and requires it to complain. A comparison
// that only ever sees matching inputs has never been shown to be able to fail.
func DiffCaseSets(catalog, executed []string) (missing, extra []string) {
	inExec := map[string]bool{}
	for _, id := range executed {
		inExec[id] = true
	}
	inCat := map[string]bool{}
	for _, id := range catalog {
		inCat[id] = true
	}
	for _, id := range catalog {
		if !inExec[id] {
			missing = append(missing, id)
		}
	}
	for _, id := range executed {
		if !inCat[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}
