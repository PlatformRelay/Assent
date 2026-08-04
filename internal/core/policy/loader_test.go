package policy_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// Fixture roots, relative to this package (internal/core/policy).
const (
	d016Dir   = "../../../examples/contracts/d016-strict-fixture"
	compatDir = "../../../schemas/testdata/compat/strict-decode"
)

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // hardcoded test-fixture path
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return raw
}

// REQ-E2-S01-01: the frozen d016 MergePolicy + RulesetBinding load into engine
// types with every field the engine will consume surfaced, no lossy round-trip.
func TestLoadMergePolicyFixture(t *testing.T) {
	mp, err := policy.LoadMergePolicy(readFixture(t, filepath.Join(d016Dir, "merge-policy.json")))
	if err != nil {
		t.Fatalf("load merge-policy: %v", err)
	}
	if mp.Metadata.Name != "topic-safety" {
		t.Errorf("metadata.name = %q, want topic-safety", mp.Metadata.Name)
	}
	e, ok := mp.Spec.Entries["topic-registry"]
	if !ok {
		t.Fatal("entries[topic-registry] missing")
	}
	if e.Mode != "document" || e.Identity.Pointer != "/metadata/name" {
		t.Errorf("entry = %+v, want mode=document identity.pointer=/metadata/name", e)
	}
	if len(mp.Spec.Rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(mp.Spec.Rules))
	}

	owner := mp.Spec.Rules[0]
	if owner.Name != "topic-owner-must-approve" || owner.Phase != policy.PhaseEnforce {
		t.Errorf("rule[0] name/phase = %q/%q", owner.Name, owner.Phase)
	}
	if owner.Prove == nil || owner.Prove.Obligation != "ownership" {
		t.Fatalf("rule[0].prove = %+v, want obligation=ownership", owner.Prove)
	}
	if owner.Prove.When.Leaf == nil || owner.Prove.When.Leaf.CEL != "facts.owner.team.state == 'resolved'" {
		t.Errorf("rule[0].prove.when leaf = %+v", owner.Prove.When.Leaf)
	}
	if owner.OnFailure == nil || owner.OnFailure.Effect != policy.EffectRequireReview || owner.OnFailure.Code != "ownership-approval-missing" {
		t.Errorf("rule[0].onFailure = %+v", owner.OnFailure)
	}

	part := mp.Spec.Rules[1]
	if part.Name != "partitions-must-not-shrink" {
		t.Errorf("rule[1].name = %q", part.Name)
	}
	if part.Points != 10 {
		t.Errorf("rule[1].points = %d, want 10 (lane F authored)", part.Points)
	}
	if part.Prove == nil || part.Prove.When.Leaf == nil || part.Prove.When.Leaf.CEL != "new >= old" {
		t.Errorf("rule[1].prove.when = %+v (want leaf cel 'new >= old' after lane F)", part.Prove)
	}
	if part.OnFailure == nil || part.OnFailure.Effect != policy.EffectBlock {
		t.Errorf("rule[1].onFailure = %+v", part.OnFailure)
	}
	if part.Match.ValueChanges == nil {
		t.Errorf("rule[1].match.valueChanges must be set, got %+v", part.Match)
	}

	rb, err := policy.LoadRulesetBinding(readFixture(t, filepath.Join(d016Dir, "ruleset-binding.json")))
	if err != nil {
		t.Fatalf("load ruleset-binding: %v", err)
	}
	if len(rb.Bindings) != 1 {
		t.Fatalf("bindings = %d, want 1", len(rb.Bindings))
	}
	b := rb.Bindings[0]
	if b.Class != "topic-registry" || b.Environment != "prod" {
		t.Errorf("binding class/env = %q/%q", b.Class, b.Environment)
	}
	if b.Risk.Threshold != 1 {
		t.Errorf("binding risk.threshold = %d, want 1", b.Risk.Threshold)
	}
	if !reflect.DeepEqual(b.Require, []string{"ownership", "non-destructive"}) {
		t.Errorf("binding require = %v", b.Require)
	}
	if !reflect.DeepEqual(b.Packs, []string{"topic-safety"}) {
		t.Errorf("binding packs = %v", b.Packs)
	}
}

// REQ-E2-S01-02: unknown field / unknown enum / duplicate collection id are all
// rejected at load with a located reason; the corresponding valid fixture loads.
func TestStrictDecodeRejectsUnknownAndDuplicate(t *testing.T) {
	type loadFn func([]byte) error
	mp := func(b []byte) error { _, err := policy.LoadMergePolicy(b); return err }
	rb := func(b []byte) error { _, err := policy.LoadRulesetBinding(b); return err }
	cfg := func(b []byte) error { _, err := policy.LoadConfig(b); return err }

	resources := map[string]struct {
		dir  string
		load loadFn
	}{
		"merge-policy":    {"merge-policy", mp},
		"ruleset-binding": {"ruleset-binding", rb},
		"config":          {"config", cfg},
	}

	for name, r := range resources {
		t.Run(name+"/unknown-field", func(t *testing.T) {
			err := r.load(readFixture(t, filepath.Join(compatDir, r.dir, "unknown-field.json")))
			if err == nil {
				t.Fatal("expected unknown-field rejection")
			}
			if !strings.Contains(err.Error(), "bogusField") {
				t.Errorf("expected located error naming bogusField, got: %v", err)
			}
		})
		t.Run(name+"/duplicate-id", func(t *testing.T) {
			if err := r.load(readFixture(t, filepath.Join(compatDir, r.dir, "duplicate-id.json"))); err == nil {
				t.Fatal("expected duplicate-id rejection")
			}
		})
		t.Run(name+"/unknown-enum", func(t *testing.T) {
			if err := r.load(readFixture(t, filepath.Join(compatDir, r.dir, "unknown-enum.json"))); err == nil {
				t.Fatal("expected unknown-enum rejection")
			}
		})
		t.Run(name+"/valid", func(t *testing.T) {
			if err := r.load(readFixture(t, filepath.Join(compatDir, r.dir, "valid.json"))); err != nil {
				t.Errorf("valid fixture must load, got: %v", err)
			}
		})
	}
}

// fileEventsPolicyBase is the shared MergePolicy skeleton whose single rule's
// match is substituted per case. Used by the EFE-S01 loader tests below.
const fileEventsPolicyBase = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "MergePolicy",
  "metadata": {"name": "t"},
  "spec": {"rules": [{"name": "r", "phase": "enforce", "match": {%s}, "effect": "comment"}]}
}`

// REQ-EFE-S01-01: a match.fileEvents domain whose kinds ⊆ {add, delete} now
// LOADS (superseding E2's REQ-E2-S01-03 whole-domain reject) and surfaces
// Match.FileEvents.{Paths,Kinds}; files/values/valueChanges still load normally.
func TestFileEventsAddDeleteAcceptedAtLoad(t *testing.T) {
	for _, kinds := range []string{`["add"]`, `["delete"]`, `["add", "delete"]`} {
		doc := strings.Replace(fileEventsPolicyBase, "%s",
			`"fileEvents": {"paths": ["topics/**"], "kinds": `+kinds+`}`, 1)
		mp, err := policy.LoadMergePolicy([]byte(doc))
		if err != nil {
			t.Fatalf("fileEvents kinds %s must load, got: %v", kinds, err)
		}
		fe := mp.Spec.Rules[0].Match.FileEvents
		if fe == nil {
			t.Fatalf("kinds %s: Match.FileEvents not surfaced", kinds)
		}
		if len(fe.Paths) != 1 || fe.Paths[0] != "topics/**" {
			t.Errorf("kinds %s: paths not surfaced, got %v", kinds, fe.Paths)
		}
		if len(fe.Kinds) == 0 {
			t.Errorf("kinds %s: kinds not surfaced", kinds)
		}
	}

	for _, ok := range []string{
		`"files": {"paths": ["topics/**"]}`,
		`"values": {"pointers": ["/partitions"]}`,
		`"valueChanges": {"pointers": ["/partitions"], "kinds": ["modify"]}`,
	} {
		doc := strings.Replace(fileEventsPolicyBase, "%s", ok, 1)
		if _, err := policy.LoadMergePolicy([]byte(doc)); err != nil {
			t.Errorf("supported match %s must load, got: %v", ok, err)
		}
	}
}

// REQ-EFE-S01-02: a match.fileEvents whose kinds names modify or rename (not a
// subset of {add, delete}) is REJECTED at load with a located error naming the
// rule and the offending kind — the vacuous-cover fail-open closer (Judgment
// call (b)). The frozen schema still accepts the enum; the loader narrows.
func TestFileEventsModifyRenameRejectedAtLoad(t *testing.T) {
	for _, bad := range []string{"modify", "rename"} {
		doc := strings.Replace(fileEventsPolicyBase, "%s",
			`"fileEvents": {"paths": ["topics/**"], "kinds": ["`+bad+`"]}`, 1)
		_, err := policy.LoadMergePolicy([]byte(doc))
		if err == nil {
			t.Fatalf("fileEvents kind %q must be rejected at load", bad)
		}
		if !strings.Contains(err.Error(), "fileEvents") {
			t.Errorf("kind %q: error must name the fileEvents domain, got: %v", bad, err)
		}
		if !strings.Contains(err.Error(), bad) {
			t.Errorf("kind %q: error must name the offending kind, got: %v", bad, err)
		}
		if !strings.Contains(err.Error(), `"r"`) {
			t.Errorf("kind %q: error must locate the offending rule, got: %v", bad, err)
		}
	}

	// A mixed kinds list containing an un-emittable kind is rejected too (the
	// add/delete members do not launder the modify member into acceptance).
	doc := strings.Replace(fileEventsPolicyBase, "%s",
		`"fileEvents": {"paths": ["topics/**"], "kinds": ["add", "modify"]}`, 1)
	if _, err := policy.LoadMergePolicy([]byte(doc)); err == nil {
		t.Fatal("fileEvents kinds [add, modify] must be rejected (modify is un-emittable)")
	}
}

// REQ-E2-S01-04: the loader is deterministic — decoding the same document twice
// yields structurally identical engine types.
func TestLoaderDoubleRunStable(t *testing.T) {
	raw := readFixture(t, filepath.Join(d016Dir, "merge-policy.json"))
	a, err := policy.LoadMergePolicy(raw)
	if err != nil {
		t.Fatalf("load a: %v", err)
	}
	b, err := policy.LoadMergePolicy(raw)
	if err != nil {
		t.Fatalf("load b: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Error("two loads of the same document must be structurally identical")
	}
}

// REQ-E2-S01-01 (Pack): a Pack manifest loads with its phase ceiling surfaced;
// an invalid phase enum is rejected by the frozen schema.
func TestLoadPack(t *testing.T) {
	good := `{"apiVersion":"assent.dev/v1alpha1","kind":"Pack","metadata":{"name":"p"},"spec":{"phase":"observe","version":"1.0.0"}}`
	p, err := policy.LoadPack([]byte(good))
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	if p.Metadata.Name != "p" || p.Spec.Phase != policy.PhaseObserve {
		t.Errorf("pack = %+v, want name=p phase=observe", p)
	}
	bad := `{"apiVersion":"assent.dev/v1alpha1","kind":"Pack","metadata":{"name":"p"},"spec":{"phase":"audit"}}`
	if _, err := policy.LoadPack([]byte(bad)); err == nil {
		t.Error("expected an unknown phase enum to be rejected")
	}
}

// REQ-E2-S01-01 (structural assertTree): all/any/not/cel-object forms decode
// structurally (not compiled) — the S01 shapes S03 later evaluates.
func TestAssertTreeCombinatorForms(t *testing.T) {
	doc := `{
  "apiVersion": "assent.dev/v1alpha1", "kind": "MergePolicy", "metadata": {"name": "t"},
  "spec": {"rules": [{
    "name": "r", "phase": "enforce",
    "match": {"values": {"pointers": ["/partitions"]}},
    "prove": {"obligation": "o", "when": {"all": [
      {"cel": "new >= old", "message": "shrunk"},
      {"any": [{"cel": "x > 0"}, {"not": {"cel": "y < 0"}}]}
    ]}},
    "onFailure": {"effect": "block", "code": "c"}
  }]}
}`
	mp, err := policy.LoadMergePolicy([]byte(doc))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	when := mp.Spec.Rules[0].Prove.When
	if len(when.All) != 2 {
		t.Fatalf("when.All = %d, want 2", len(when.All))
	}
	if when.All[0].Leaf == nil || when.All[0].Leaf.CEL != "new >= old" || when.All[0].Leaf.Message != "shrunk" {
		t.Errorf("all[0] leaf = %+v", when.All[0].Leaf)
	}
	inner := when.All[1]
	if len(inner.Any) != 2 {
		t.Fatalf("all[1].Any = %d, want 2", len(inner.Any))
	}
	if inner.Any[0].Leaf == nil || inner.Any[0].Leaf.CEL != "x > 0" {
		t.Errorf("any[0] leaf = %+v", inner.Any[0].Leaf)
	}
	if inner.Any[1].Not == nil || inner.Any[1].Not.Leaf == nil || inner.Any[1].Not.Leaf.CEL != "y < 0" {
		t.Errorf("any[1].Not = %+v", inner.Any[1].Not)
	}
}

// REQ-E2-S01-01 (assertTree decode guards): the structural decoder rejects a
// mapping that is none of all/any/not/cel and a non-scalar/non-mapping node —
// fail-closed, never a silently-empty tree. (Exercised directly because the
// frozen schema would reject these shapes before Load reaches the decoder.)
func TestAssertTreeDecodeGuards(t *testing.T) {
	var wrap struct {
		When policy.AssertTree `yaml:"when"`
	}
	if err := yaml.Unmarshal([]byte("when: {foo: bar}"), &wrap); err == nil {
		t.Error("mapping with none of all/any/not/cel must be rejected")
	}
	if err := yaml.Unmarshal([]byte("when: [a, b]"), &wrap); err == nil {
		t.Error("a sequence node must be rejected")
	}
	// The scalar and cel-object happy paths remain valid.
	if err := yaml.Unmarshal([]byte(`when: "new >= old"`), &wrap); err != nil {
		t.Errorf("bare-string leaf must decode, got: %v", err)
	}
}
