package catalogue_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/PlatformRelay/assent/internal/catalogue"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

func catalogueFixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "cmd", "assent", "testdata", "catalogue")
}

// REQ-PCS-S01-01: LoadFromDir walks .assent and assembles catalogue.Input.
func TestLoadFromDir(t *testing.T) {
	dir := catalogueFixtureDir(t)
	in, err := catalogue.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if len(in.Packs) != 1 {
		t.Fatalf("packs = %d, want 1 (topics)", len(in.Packs))
	}
	if in.Packs[0].Name != "topics" {
		t.Fatalf("pack name = %q, want topics", in.Packs[0].Name)
	}
	if len(in.Packs[0].Policies) != 2 {
		t.Fatalf("topics policies = %d, want 2", len(in.Packs[0].Policies))
	}
	if len(in.Bindings) != 1 {
		t.Fatalf("bindings = %d, want 1", len(in.Bindings))
	}
}

func TestLoadFromDirMissingTree(t *testing.T) {
	if _, err := catalogue.LoadFromDir(t.TempDir()); err == nil {
		t.Fatal("expected error for missing .assent tree")
	}
}

func TestLoadFromDirRejectsInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".assent", "packs", "p", "rules")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.yaml"), []byte(":\n- not a mapping"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := catalogue.LoadFromDir(dir); err == nil {
		t.Fatal("expected load error for invalid YAML kind header")
	}
}

func TestLoadFromDirLoadsBindingsAndManifests(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".assent/bindings.yaml", `apiVersion: assent.dev/v1alpha1
kind: RulesetBinding
bindings:
  - class: c
    environment: prod
    packs: [p]
    risk: { threshold: 4 }
    require: [a]
`)
	write(".assent/packs/p/pack.yaml", `apiVersion: assent.dev/v1alpha1
kind: Pack
metadata:
  name: p
spec:
  phase: observe
`)
	write(".assent/packs/p/rules/r.yaml", `apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: r
spec:
  rules:
    - name: r
      phase: enforce
      match:
        files:
          paths: ["**/*"]
      prove:
        obligation: a
        when: "true"
      onFailure:
        effect: block
        code: c
`)
	in, err := catalogue.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if len(in.Bindings) != 1 || in.Packs[0].Manifest == nil || in.Packs[0].Manifest.Spec.Phase != policy.PhaseObserve {
		t.Fatalf("unexpected bindings/manifest load: %+v", in)
	}
}

func TestPhaseCeilingForProfile(t *testing.T) {
	in := catalogue.Input{
		Packs: []catalogue.Pack{
			{Name: "capped", Manifest: &policy.Pack{Spec: policy.PackSpec{Phase: policy.PhaseObserve}}},
			{Name: "open"},
		},
	}
	p := profileWithPacks("mix", "capped", "open")
	if got := catalogue.PhaseCeilingForProfile(p, in); got != policy.PhaseObserve {
		t.Fatalf("ceiling = %s, want observe (strictest across activated packs)", got)
	}
}

func TestMergePolicyForProfileNilProfile(t *testing.T) {
	if _, err := catalogue.MergePolicyForProfile(nil, catalogue.Input{}); err == nil {
		t.Fatal("expected error for nil profile")
	}
}

func TestMergePolicyForProfileCombineError(t *testing.T) {
	entry := policy.Entry{Mode: "list", Root: "/a", Identity: policy.Identity{Pointer: "/id"}}
	in := catalogue.Input{Packs: []catalogue.Pack{{
		Name: "broken",
		Policies: []*policy.MergePolicy{
			{Spec: policy.MergePolicySpec{Entries: map[string]policy.Entry{"x": entry}}},
			{Spec: policy.MergePolicySpec{Entries: map[string]policy.Entry{"x": {Mode: "list", Root: "/b", Identity: policy.Identity{Pointer: "/id"}}}}},
		},
	}}}
	if _, err := catalogue.MergePolicyForProfile(profileWithPacks("broken", "broken"), in); err == nil {
		t.Fatal("expected combine error to propagate")
	}
}
