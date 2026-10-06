package conformance

import (
	"sort"
	"strings"
	"testing"
)

// TestCatalogMatchesExecutedCases is REQ-E10-S01-02: extraction is a refactor, so
// the set of executed case IDs must equal the catalog's claim, with no case
// missing and none extra.
//
// The denominator is OBSERVED EXECUTION — ExecutedCaseIDs actually runs the suite
// and records each case as it is dispatched — not a hand-maintained list and not
// a scan for test-function names in source. That distinction is the whole point.
// A gate keyed on names would stay green for a case that still exists as a
// function but has been unhooked from the runner, which is a predicate over TEXT
// standing in for a property that is STRUCTURAL. This repo has paid for that
// substitution repeatedly; see D-164.
func TestCatalogMatchesExecutedCases(t *testing.T) {
	cat, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	executed := RunSuite(tbT{t}, fakeFactory)
	sort.Strings(executed)

	missing, extra := DiffCaseSets(cat.SuiteCaseIDs(), executed)
	if len(missing) > 0 {
		t.Errorf("catalog rows never executed by RunSuite: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("RunSuite executed cases with no catalog row: %v", extra)
	}
	if len(executed) == 0 {
		t.Fatal("RunSuite executed nothing — the denominator is empty, so this gate proves nothing")
	}
}

// TestDiffCaseSetsDetectsDrift is the MUTATION CONTROL for the gate above. It
// feeds DiffCaseSets the two failure shapes and requires each to be reported. If
// someone reduces the comparison to a length check, or to a one-directional
// containment test, this reds.
func TestDiffCaseSetsDetectsDrift(t *testing.T) {
	full := []string{"a", "b", "c"}

	// A case unhooked from the runner while its catalog row remains.
	missing, extra := DiffCaseSets(full, []string{"a", "c"})
	if len(missing) != 1 || missing[0] != "b" {
		t.Fatalf("unhooked case must be reported missing, got missing=%v extra=%v", missing, extra)
	}

	// A case running with no catalogued disposition.
	missing, extra = DiffCaseSets([]string{"a", "b"}, full)
	if len(extra) != 1 || extra[0] != "c" {
		t.Fatalf("uncatalogued case must be reported extra, got missing=%v extra=%v", missing, extra)
	}

	// Equal-length but disjoint: the shape a naive count comparison misses entirely.
	missing, extra = DiffCaseSets([]string{"a", "b"}, []string{"a", "z"})
	if len(missing) != 1 || len(extra) != 1 {
		t.Fatalf("same-size divergence must be reported on both axes, got missing=%v extra=%v", missing, extra)
	}
}

// TestCatalogStrictDecode is REQ-E10-S01-03: the loader rejects an unknown
// adapter name rather than silently ignoring it, and rejects unknown fields.
func TestCatalogStrictDecode(t *testing.T) {
	if _, err := LoadCatalog(catalogPath); err != nil {
		t.Fatalf("the shipped catalog must decode strictly: %v", err)
	}

	for _, tc := range []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{
			name: "unknown adapter name",
			yaml: `cases:
  - id: bogus
    level: L1
    req: REQ-X
    test: TestX
    package: internal/forge/conformance
    forge: gitlab
    adapters: [gitlub]
`,
			wantSub: "unknown adapter",
		},
		{
			name: "unknown field",
			yaml: `cases:
  - id: bogus
    level: L1
    req: REQ-X
    test: TestX
    package: internal/forge/conformance
    forge: gitlab
    adapters: [gitlab]
    frge: typo
`,
			wantSub: "field frge not found",
		},
		{
			name: "duplicate case id",
			yaml: `cases:
  - id: twice
    level: L1
    req: REQ-X
    test: TestX
    package: internal/forge/conformance
    forge: gitlab
    adapters: [gitlab]
  - id: twice
    level: L1
    req: REQ-X
    test: TestY
    package: internal/forge/conformance
    forge: gitlab
    adapters: [gitlab]
`,
			wantSub: `case id "twice" is declared more than once`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeCatalog([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("strict decode must reject %s, got nil error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error must name the cause; want substring %q, got %v", tc.wantSub, err)
			}
		})
	}

	// POSITIVE CONTROL: an otherwise identical row with a KNOWN adapter decodes.
	// Without this, every rejection above would also be satisfied by a loader that
	// rejects everything.
	ok := `cases:
  - id: fine
    level: L1
    req: REQ-X
    test: TestX
    package: internal/forge/conformance
    forge: gitlab
    adapters: [gitlab, github]
`
	if _, err := DecodeCatalog([]byte(ok)); err != nil {
		t.Fatalf("a well-formed row must decode, got %v", err)
	}
}

// TestEveryRowHasAdapterDisposition is REQ-E10-S14-01's named gate: every
// EXECUTABLE row names its adapters explicitly. A row whose adapters list is
// empty claims coverage against nothing — the exact overstatement a strict
// coverage index exists to prevent — so it is an error unless the row is a
// github-deferred stub, where the empty list is the row's way of saying
// "nothing executes this yet".
func TestEveryRowHasAdapterDisposition(t *testing.T) {
	cat, err := LoadCatalog(catalogPath)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	for _, row := range cat.Cases {
		if len(row.Adapters) == 0 && row.Forge != DeferredForge {
			t.Errorf("row %q carries no adapters disposition and is not %s — every executable row must name its adapters explicitly (REQ-E10-S14-01)",
				row.ID, DeferredForge)
		}
	}
}
