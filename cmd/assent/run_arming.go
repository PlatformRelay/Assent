package main

import (
	"regexp"
	"time"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// controllingFactRefRe matches facts.<provider> references in prove.when leaves.
var controllingFactRefRe = regexp.MustCompile(`\bfacts\.([A-Za-z_][A-Za-z0-9_]*)`)

// controllingFactsFreshAt is the ADR-0017 §4 arming precondition: every
// controlling authorization fact must still be within maxAge at armTime. Evaluation
// may have occurred earlier (one-shot); arming re-checks expiry before writes.
func controllingFactsFreshAt(mp *policy.MergePolicy, facts map[string]map[string]aggregate.Fact, armTime time.Time) bool {
	if mp == nil || len(facts) == 0 {
		return true
	}
	armTime = armTime.UTC()
	for _, prov := range controllingProviders(mp) {
		providerFacts, ok := facts[prov]
		if !ok || len(providerFacts) == 0 {
			return false
		}
		for _, fact := range providerFacts {
			if fact.State != "resolved" {
				return false
			}
			if fact.ExpiresAt == "" {
				continue
			}
			exp, err := time.Parse(time.RFC3339, fact.ExpiresAt)
			if err != nil || !exp.After(armTime) {
				return false
			}
		}
	}
	return true
}

// controllingProviders returns providers referenced by require-review obligation
// proofs (authorization-class facts per ADR-0017 §6 / policy.ValidateProviderPosture).
func controllingProviders(mp *policy.MergePolicy) []string {
	if mp == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range mp.Spec.Rules {
		if r.Prove == nil || r.OnFailure == nil || r.OnFailure.Effect != policy.EffectRequireReview {
			continue
		}
		for _, prov := range providersInAssertTree(r.Prove.When) {
			if !seen[prov] {
				seen[prov] = true
				out = append(out, prov)
			}
		}
	}
	return out
}

func providersInAssertTree(t policy.AssertTree) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(policy.AssertTree)
	walk = func(n policy.AssertTree) {
		if n.Leaf != nil {
			for _, m := range controllingFactRefRe.FindAllStringSubmatch(n.Leaf.CEL, -1) {
				if !seen[m[1]] {
					seen[m[1]] = true
					out = append(out, m[1])
				}
			}
		}
		for _, c := range n.All {
			walk(c)
		}
		for _, c := range n.Any {
			walk(c)
		}
		if n.Not != nil {
			walk(*n.Not)
		}
	}
	walk(t)
	return out
}
