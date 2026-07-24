package schemas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// doNotGeneralizeFixtureDir holds ADR-0017 §9 do-not-generalize adversarial
// fixtures (P3-E2-S05): one permanent invalid case per named lint-error code
// so loosening a schema guard shows up as a fixture that suddenly validates.
const doNotGeneralizeFixtureDir = "testdata/compat/do-not-generalize"

// doNotGeneralizeCatalog is the frozen lint-error catalog restated in
// API_STABILITY.md. Codes are stable public identifiers for `assent lint`
// hard errors (Phase 5); fixtures prove the authored-schema surface already
// rejects each generalization today.
var doNotGeneralizeCatalog = []struct {
	code    string
	schema  *jsonschema.Schema
	fixture string
}{
	{code: "no-user-defined-effects", schema: MergePolicySchema, fixture: "no-user-defined-effects.json"},
	{code: "no-custom-aggregators", schema: RulesetBindingSchema, fixture: "no-custom-aggregators.json"},
	{code: "no-obligation-anyof", schema: RulesetBindingSchema, fixture: "no-obligation-anyof.json"},
	{code: "no-entry-selector-query", schema: MergePolicySchema, fixture: "no-entry-selector-query.json"},
	{code: "no-lcd-forge-api", schema: ConfigSchema, fixture: "no-lcd-forge-api.json"},
}

func readDoNotGeneralizeFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(doNotGeneralizeFixtureDir, name)) //nolint:gosec // hardcoded test-fixture literal
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(raw)
}

// TestDoNotGeneralize proves REQ-P3-E2-S05-03: each do-not-generalize catalog
// entry has an adversarial-invalid fixture that the corresponding
// safety-bearing schema rejects. Removing a guard (loosening the schema so
// the fixture validates) fails this test; deleting a fixture fails on read.
func TestDoNotGeneralize(t *testing.T) {
	if len(doNotGeneralizeCatalog) != 5 {
		t.Fatalf("catalog must list exactly five ADR-0017 §9 do-not-generalize codes, got %d", len(doNotGeneralizeCatalog))
	}

	for _, tc := range doNotGeneralizeCatalog {
		t.Run(tc.code, func(t *testing.T) {
			doc := readDoNotGeneralizeFixture(t, tc.fixture)
			if err := validateJSON(tc.schema, doc); err == nil {
				t.Fatalf("expected catalog entry %q fixture to be rejected; removing this guard is a silent regression", tc.code)
			}
		})
	}

	t.Run("catalog_documented", func(t *testing.T) {
		// REQ-P3-E2-S05-01/02: the public stability doc must name every code
		// and keep the do-not-generalize / graduation sections findable.
		raw, err := os.ReadFile(filepath.Join("..", "API_STABILITY.md")) //nolint:gosec // repo-root stability doc, fixed relative path
		if err != nil {
			t.Fatalf("read API_STABILITY.md: %v", err)
		}
		body := string(raw)
		if !strings.Contains(strings.ToLower(body), "policy schema") {
			t.Fatal("API_STABILITY.md must document the policy schema contract")
		}
		if !strings.Contains(strings.ToLower(body), "graduation") {
			t.Fatal("API_STABILITY.md must document graduation criteria")
		}
		if !strings.Contains(strings.ToLower(body), "do-not-generalize") {
			t.Fatal("API_STABILITY.md must contain a do-not-generalize section")
		}
		if !strings.Contains(body, "ADR-0017") {
			t.Fatal("API_STABILITY.md must cite ADR-0017")
		}
		for _, tc := range doNotGeneralizeCatalog {
			if !strings.Contains(body, tc.code) {
				t.Fatalf("API_STABILITY.md must document stable lint error code %q", tc.code)
			}
		}
	})
}
