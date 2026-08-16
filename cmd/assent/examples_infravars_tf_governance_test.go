package main

import (
	"os"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/glob"
)

// examples_infravars_tf_governance_test.go closes the REQ-EX-S05-05 non-vacuity
// gap an independent review found in lane/ex-s05: `assent test`/`--coverage`
// never consult Config.Classes at all (catalogue.Input deliberately omits Config
// per D-017 B10 — "a rule's classes come from the binding graph (binding.class),
// which carries the class NAME directly"; see internal/catalogue/catalogue.go's
// Input doc comment). `assent test`'s selectBindingForTest picks ONE binding for
// the whole pack regardless of the case's File, so deleting `envs/**/*.tf` from
// infra-vars' class match.paths while leaving the vars/tf-opaque fixture in
// place produces byte-identical `assent test`/`--coverage` output (PASS
// vars/tf-opaque (REVIEW), exit 0) — the opaque-changeset collapse in
// adoptertest.Evaluate's undecidable guard cannot distinguish "governed change
// hit the opaque fallback" from "nothing here was governed at all", and no
// other engine code path reads Config.Classes to tell them apart either.
//
// This test proves governance directly against the SAME matcher the engine's
// routing/coverage code uses (internal/glob.Match — shared by
// internal/core/classify and internal/core/aggregate, imported here as a
// read-only consumer, not reimplemented), over the REAL config.yaml on disk —
// so it reddens the instant `envs/**/*.tf` is removed from the infra-vars
// class, exactly the mutation `assent test` itself cannot see.
func TestInfraVarsTFFixtureIsGovernedByItsClass(t *testing.T) {
	const (
		configPath = "../../examples/packs/infra-vars/.assent/config.yaml"
		className  = "infra-vars"
		// The vars/tf-opaque case's changed-file path (repo-relative, matching
		// how change.Diff and the real matcher both see it).
		tfFixturePath = "envs/prod/backend.tf"
	)

	raw, err := os.ReadFile(configPath) //nolint:gosec // fixed in-repo path relative to cmd/assent.
	if err != nil {
		t.Fatalf("read %s: %v", configPath, err)
	}
	cfg, err := policy.LoadConfig(raw)
	if err != nil {
		t.Fatalf("LoadConfig %s: %v", configPath, err)
	}

	var class *policy.NamedMatch
	for i := range cfg.Classes {
		if cfg.Classes[i].Name == className {
			class = &cfg.Classes[i]
			break
		}
	}
	if class == nil {
		t.Fatalf("config.yaml declares no class named %q", className)
	}

	matched := false
	for _, pattern := range class.Match.Paths {
		if glob.Match(pattern, tfFixturePath) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("class %q's match.paths %v does not cover %q — the vars/tf-opaque"+
			" fixture would be ungoverned even though assent test/--coverage cannot"+
			" see that (REQ-EX-S05-05); restore the envs/**/*.tf glob", className, class.Match.Paths, tfFixturePath)
	}
}

// TestInfraVarsTFGovernanceAssertionCanFail is the mutation control (matching
// this repo's house style, e.g. hack/lint/depguard_test.sh): it proves the
// assertion above is not vacuously true by re-running the exact same glob.Match
// check against an in-memory class whose match.paths never mention .tf — the
// same shape the tf-opaque case would be left in if `envs/**/*.tf` were deleted
// from the real config.yaml. It must fail to match.
func TestInfraVarsTFGovernanceAssertionCanFail(t *testing.T) {
	withoutTF := policy.NamedMatch{
		Name:  "infra-vars",
		Match: policy.PathMatch{Paths: []string{"envs/**/*.tfvars"}},
	}
	if glob.Match(withoutTF.Match.Paths[0], "envs/prod/backend.tf") {
		t.Fatalf("mutation control did not redden: %q unexpectedly matched %q — the"+
			" assertion in TestInfraVarsTFFixtureIsGovernedByItsClass would be vacuous",
			withoutTF.Match.Paths[0], "envs/prod/backend.tf")
	}
}
