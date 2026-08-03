package policy_test

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// authRule builds an authorization rule (onFailure escalates to require-review)
// whose prove.when is the given assert tree.
func authRule(when policy.AssertTree) policy.Rule {
	return policy.Rule{
		Name:      "auth",
		Phase:     policy.PhaseEnforce,
		Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**"}}},
		Prove:     &policy.Prove{Obligation: "ownership", When: when},
		OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "owner-missing"},
	}
}

func mpWith(rules ...policy.Rule) *policy.MergePolicy {
	return &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: rules}}
}

func cfgWith(providers map[string]policy.Provider) *policy.Config {
	return &policy.Config{Providers: providers}
}

func TestValidateProviderPostureNilArgsIsNoOp(t *testing.T) {
	if err := policy.ValidateProviderPosture(nil, nil); err != nil {
		t.Errorf("nil args must be a no-op, got: %v", err)
	}
	if err := policy.ValidateProviderPosture(cfgWith(nil), nil); err != nil {
		t.Errorf("nil mp must be a no-op, got: %v", err)
	}
}

func TestValidateProviderPostureRejectsControllingOpenLeaf(t *testing.T) {
	mp := mpWith(authRule(policy.AssertTree{Leaf: &policy.Leaf{CEL: "facts.owner.team.state == 'resolved'"}}))
	cfg := cfgWith(map[string]policy.Provider{"owner": {Type: "http", Failure: "open"}})
	err := policy.ValidateProviderPosture(cfg, mp)
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("controlling provider failure:open must be rejected naming owner, got: %v", err)
	}
}

// A provider referenced only inside an all/any/not combinator is still detected —
// the walker recurses the whole tree, not just a bare leaf.
func TestValidateProviderPostureWalksCombinatorTree(t *testing.T) {
	when := policy.AssertTree{All: []policy.AssertTree{
		{Leaf: &policy.Leaf{CEL: "new >= old"}},
		{Any: []policy.AssertTree{
			{Not: &policy.AssertTree{Leaf: &policy.Leaf{CEL: "facts.approvers.team.value == 0"}}},
		}},
	}}
	mp := mpWith(authRule(when))
	cfg := cfgWith(map[string]policy.Provider{"approvers": {Type: "http", Failure: "open"}})
	if err := policy.ValidateProviderPosture(cfg, mp); err == nil || !strings.Contains(err.Error(), "approvers") {
		t.Fatalf("a controlling provider nested in an all/any/not tree must be rejected, got: %v", err)
	}
}

// A require-review rule referencing a provider absent from config is not an error
// (config may legitimately omit a provider that resolves via a built-in default).
func TestValidateProviderPostureUnknownProviderIgnored(t *testing.T) {
	mp := mpWith(authRule(policy.AssertTree{Leaf: &policy.Leaf{CEL: "facts.unknown.x.state == 'resolved'"}}))
	cfg := cfgWith(map[string]policy.Provider{"owner": {Type: "http", Failure: "open"}})
	if err := policy.ValidateProviderPosture(cfg, mp); err != nil {
		t.Errorf("a provider not present in config must not error, got: %v", err)
	}
}

// A provider referenced only by a NON-require-review (advisory) rule may fail
// open even if a require-review rule elsewhere reads a DIFFERENT provider.
func TestValidateProviderPostureAdvisoryProviderMayFailOpen(t *testing.T) {
	blockRule := policy.Rule{
		Name:      "advisory",
		Phase:     policy.PhaseEnforce,
		Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**"}}},
		Prove:     &policy.Prove{Obligation: "bounded", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new <= facts.quota.max.value"}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "over"},
	}
	mp := mpWith(
		authRule(policy.AssertTree{Leaf: &policy.Leaf{CEL: "facts.owner.team.state == 'resolved'"}}),
		blockRule,
	)
	cfg := cfgWith(map[string]policy.Provider{
		"owner": {Type: "http", Failure: "closed"},
		"quota": {Type: "http", Failure: "open"}, // advisory -> allowed
	})
	if err := policy.ValidateProviderPosture(cfg, mp); err != nil {
		t.Errorf("an advisory-only provider may fail open, got: %v", err)
	}
}

// The default (empty) failure posture is treated as closed — only the explicit
// "open" string is rejected.
func TestValidateProviderPostureEmptyPostureIsClosed(t *testing.T) {
	mp := mpWith(authRule(policy.AssertTree{Leaf: &policy.Leaf{CEL: "facts.owner.team.state == 'resolved'"}}))
	cfg := cfgWith(map[string]policy.Provider{"owner": {Type: "http"}}) // Failure == "" (default closed)
	if err := policy.ValidateProviderPosture(cfg, mp); err != nil {
		t.Errorf("default (empty) posture is closed and must be accepted, got: %v", err)
	}
}
