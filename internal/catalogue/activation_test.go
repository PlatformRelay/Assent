package catalogue_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/catalogue"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

func strictRule() policy.Rule {
	return policy.Rule{
		Name:  "non-destructive",
		Phase: policy.PhaseEnforce,
		Prove: &policy.Prove{
			Obligation: "non-destructive",
			When:       policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old", Message: "must not shrink"}},
		},
		OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "partitions.shrunk"},
	}
}

func permissiveRule() policy.Rule {
	r := strictRule()
	r.Prove.When.Leaf.CEL = "true"
	return r
}

func packPolicies(name string, rules ...policy.Rule) catalogue.Pack {
	mp := &policy.MergePolicy{}
	mp.Spec.Rules = append(mp.Spec.Rules, rules...)
	return catalogue.Pack{Name: name, Policies: []*policy.MergePolicy{mp}}
}

func profileWithPacks(name string, packs ...string) *policy.Profile {
	return &policy.Profile{
		Metadata: policy.Metadata{Name: name},
		Spec:     policy.ProfileSpec{Writes: false, Environments: []string{"*"}, Classes: []string{"*"}, Packs: packs},
	}
}

// REQ-PCS-S01-02: MergePolicyForProfile unions rules from every declared pack.
func TestMergePolicyForProfileCombinesPacks(t *testing.T) {
	in := catalogue.Input{
		Packs: []catalogue.Pack{
			packPolicies("strict", strictRule()),
			packPolicies("permissive", permissiveRule()),
		},
	}
	mp, err := catalogue.MergePolicyForProfile(profileWithPacks("both", "strict", "permissive"), in)
	if err != nil {
		t.Fatalf("MergePolicyForProfile: %v", err)
	}
	if mp == nil {
		t.Fatal("expected a combined policy")
	}
	if len(mp.Spec.Rules) != 2 {
		t.Fatalf("rules = %d, want 2 (one per pack)", len(mp.Spec.Rules))
	}
}

// REQ-PCS-S01-03: unknown pack names fail closed with no partial policy.
func TestMergePolicyForProfileUnknownPack(t *testing.T) {
	in := catalogue.Input{Packs: []catalogue.Pack{packPolicies("strict", strictRule())}}
	if _, err := catalogue.MergePolicyForProfile(profileWithPacks("bad", "strict", "missing"), in); err == nil {
		t.Fatal("expected fail-closed error for unknown pack")
	}
}
