package policy

// profile_test.go covers the E2-S09 Profile loader + coverage predicate +
// single-writer load rejection (ADR-0018 §2). Winner-selection (specificity /
// config-order) lives in internal/core/aggregate; this file owns the frozen-schema
// decode, the Covers predicate, and the single-writer invariant.

import "testing"

func TestLoadProfile(t *testing.T) {
	t.Run("writer profile decodes every field", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyProfile",
			"metadata": {"name": "prod-writer"},
			"spec": {"writes": true, "environments": ["prod"], "classes": ["topic-registry"]}
		}`
		p, err := LoadProfile([]byte(doc))
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		if p.Metadata.Name != "prod-writer" || !p.Spec.Writes {
			t.Errorf("got %+v, want name prod-writer writes true", p)
		}
		if len(p.Spec.Environments) != 1 || p.Spec.Environments[0] != "prod" {
			t.Errorf("environments = %v, want [prod]", p.Spec.Environments)
		}
		if len(p.Spec.Classes) != 1 || p.Spec.Classes[0] != "topic-registry" {
			t.Errorf("classes = %v, want [topic-registry]", p.Spec.Classes)
		}
	})

	t.Run("recorder-only profile with packs decodes", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyProfile",
			"metadata": {"name": "shadow"},
			"spec": {"writes": false, "environments": ["*"], "classes": ["*"], "packs": ["topics"]}
		}`
		p, err := LoadProfile([]byte(doc))
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		if p.Spec.Writes {
			t.Error("recorder-only profile must decode writes false")
		}
		if len(p.Spec.Packs) != 1 || p.Spec.Packs[0] != "topics" {
			t.Errorf("packs = %v, want [topics]", p.Spec.Packs)
		}
	})

	t.Run("missing writes is rejected by the frozen schema (no silent default)", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyProfile",
			"metadata": {"name": "undecorated"},
			"spec": {"environments": ["prod"], "classes": ["topic-registry"]}
		}`
		if _, err := LoadProfile([]byte(doc)); err == nil {
			t.Fatal("expected a profile missing writes to fail load")
		}
	})

	t.Run("unknown top-level field is rejected", func(t *testing.T) {
		const doc = `{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PolicyProfile",
			"metadata": {"name": "bogus"},
			"spec": {"writes": false, "environments": ["*"], "classes": ["*"]},
			"bogus": true
		}`
		if _, err := LoadProfile([]byte(doc)); err == nil {
			t.Fatal("expected an unknown top-level field to fail load")
		}
	})
}

func TestProfileCovers(t *testing.T) {
	cases := []struct {
		name       string
		envs       []string
		classes    []string
		env, class string
		want       bool
	}{
		{"exact both", []string{"prod"}, []string{"topic-registry"}, "prod", "topic-registry", true},
		{"wildcard both", []string{"*"}, []string{"*"}, "prod", "topic-registry", true},
		{"wildcard env only", []string{"*"}, []string{"topic-registry"}, "dev", "topic-registry", true},
		{"env miss", []string{"prod"}, []string{"topic-registry"}, "dev", "topic-registry", false},
		{"class miss", []string{"prod"}, []string{"topic-registry"}, "prod", "schema", false},
		{"multi-env concrete hit", []string{"prod", "staging"}, []string{"*"}, "staging", "x", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Profile{Spec: ProfileSpec{Environments: tc.envs, Classes: tc.classes}}
			if got := p.Covers(tc.env, tc.class); got != tc.want {
				t.Errorf("Covers(%q,%q) = %v, want %v", tc.env, tc.class, got, tc.want)
			}
		})
	}
}

func TestCoveringProfilesSingleWriter(t *testing.T) {
	writerA := &Profile{Metadata: Metadata{Name: "writer-a"}, Spec: ProfileSpec{Writes: true, Environments: []string{"prod"}, Classes: []string{"topic-registry"}}}
	writerB := &Profile{Metadata: Metadata{Name: "writer-b"}, Spec: ProfileSpec{Writes: true, Environments: []string{"*"}, Classes: []string{"*"}}}
	recorder := &Profile{Metadata: Metadata{Name: "recorder"}, Spec: ProfileSpec{Writes: false, Environments: []string{"*"}, Classes: []string{"*"}}}

	precedence := []ProfileRef{{Name: "writer-a"}, {Name: "writer-b"}}
	if _, err := CoveringProfiles(precedence, []*Profile{writerA, writerB}, "prod", "topic-registry"); err == nil {
		t.Fatal("expected single-writer rejection for two covering writes:true profiles")
	}

	// One writer + one recorder covering → allowed; both returned in precedence order.
	precedence = []ProfileRef{{Name: "writer-a"}, {Name: "recorder"}}
	got, err := CoveringProfiles(precedence, []*Profile{writerA, recorder}, "prod", "topic-registry")
	if err != nil {
		t.Fatalf("one writer + one recorder must not be rejected: %v", err)
	}
	if len(got) != 2 || got[0].Metadata.Name != "writer-a" || got[1].Metadata.Name != "recorder" {
		t.Errorf("covering set = %v, want [writer-a recorder] in precedence order", names(got))
	}
}

func TestCoveringProfilesDanglingRef(t *testing.T) {
	writerA := &Profile{Metadata: Metadata{Name: "writer-a"}, Spec: ProfileSpec{Writes: true, Environments: []string{"prod"}, Classes: []string{"topic-registry"}}}
	precedence := []ProfileRef{{Name: "writer-a"}, {Name: "ghost"}}
	if _, err := CoveringProfiles(precedence, []*Profile{writerA}, "prod", "topic-registry"); err == nil {
		t.Fatal("expected a dangling precedence ref (no matching Profile document) to be rejected")
	}
}

func names(ps []*Profile) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Metadata.Name
	}
	return out
}
