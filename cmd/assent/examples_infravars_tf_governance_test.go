package main

import (
	"os"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/glob"
)

// examples_infravars_tf_governance_test.go closes the REQ-EX-S05-05 non-vacuity
// gap independent review found in lane/ex-s05: `assent test`/`--coverage`
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
// REQ-EX-S05-05 names TWO mutations that must redden ("the .tf fixture OR
// class-path extension is deleted"). Round 1 closed only the class-path-glob
// disjunct (TestInfraVarsTFFixtureIsGovernedByItsClass, below). Round 2 (this
// addition, TestInfraVarsTFFixtureFilesExist) closes the other one: deleting
// the tf-opaque case's base/+head/ .tf files outright (leaving config.yaml's
// glob intact) is ALSO invisible to `--coverage` (rule-level, not case-count
// aware — a vanished no-findings case is simply absent from the run) and to
// hack/docs/example_format_inventory_test.sh (derives FORMATS from config.yaml's
// declared glob extensions, never checks a file exists on disk). The first
// round's glob-match assertion doesn't catch this either — it only checks a
// hardcoded path STRING against the class pattern, never os.Stats the fixture.
//
// Both tests prove governance/existence directly against the real filesystem
// and the SAME matcher the engine's routing/coverage code uses (internal/glob.Match
// — shared by internal/core/classify and internal/core/aggregate, imported here
// as a read-only consumer, not reimplemented) — so together they redden on
// EITHER of REQ-EX-S05-05's named mutations, exactly what `assent test`/
// `--coverage`/inventory cannot see.
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

// tfOpaqueCaseFiles are the vars/tf-opaque case's base/head fixture files
// (repo-relative to cmd/assent), the two files REQ-EX-S05-05's "the .tf
// fixture ... is deleted" disjunct is about.
var tfOpaqueCaseFiles = []string{
	"../../examples/packs/infra-vars/.assent/tests/vars/tf-opaque/base/envs/prod/backend.tf",
	"../../examples/packs/infra-vars/.assent/tests/vars/tf-opaque/head/envs/prod/backend.tf",
}

// TestInfraVarsTFFixtureFilesExist closes REQ-EX-S05-05's OTHER disjunct
// (round 2 of independent review): deleting the tf-opaque case's base/+head/
// .tf files outright, while leaving config.yaml's *.tf glob intact, is
// invisible to `assent test --coverage` (the case simply vanishes from the
// run — --coverage counts rule polarity, not case count) and to
// hack/docs/example_format_inventory_test.sh (FORMATS comes from config.yaml's
// declared extensions, never from checking a file exists). Neither this file's
// class-match test proves the fixture is still there: it only checks a
// hardcoded path string against a glob pattern, never touching the filesystem.
// This test os.Stats the real files directly, so it reddens the instant either
// one is deleted or replaced by a directory/non-regular file.
func TestInfraVarsTFFixtureFilesExist(t *testing.T) {
	for _, p := range tfOpaqueCaseFiles {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("vars/tf-opaque fixture file missing: %s: %v — REQ-EX-S05-05's"+
				" honesty case can be silently deleted with config.yaml's *.tf glob"+
				" left intact, and assent test/--coverage/inventory would not notice", p, err)
		}
		if info.IsDir() {
			t.Fatalf("vars/tf-opaque fixture path is a directory, not a file: %s", p)
		}
		if info.Size() == 0 {
			t.Fatalf("vars/tf-opaque fixture file is empty: %s — an empty base/head pair"+
				" also collapses to the same undecidable REVIEW/no-findings result as a"+
				" real resource/module block (F2, round-1 review), so a zeroed-out"+
				" fixture would silently stop proving anything", p)
		}
	}
}

// TestInfraVarsTFFixtureFilesExistAssertionCanFail is the mutation control for
// the test above: os.Stat over a path that does not exist must error, proving
// the assertion is capable of failing (not vacuously true because os.Stat
// never errors in this environment).
func TestInfraVarsTFFixtureFilesExistAssertionCanFail(t *testing.T) {
	missing := "../../examples/packs/infra-vars/.assent/tests/vars/tf-opaque/base/envs/prod/does-not-exist.tf"
	if _, err := os.Stat(missing); err == nil {
		t.Fatalf("mutation control did not redden: os.Stat unexpectedly succeeded for"+
			" a path that should not exist: %s — TestInfraVarsTFFixtureFilesExist would"+
			" be vacuous", missing)
	}
}
