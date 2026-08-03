package policy

import (
	"fmt"
	"regexp"
)

// provider.go enforces the ADR-0017 §6 / lint-hard-errors.md "fail-open
// restriction": a config.yaml providers entry backing a controlling or
// authorization fact may never declare `failure: open` — a controlling fact must
// fail closed.
//
// Deciding "controlling/authorization-class" at LOAD time.
//
// The frozen provider $def (config.schema.json) carries only type/url/failure —
// there is NO explicit `controlling` marker to key off, and the runtime
// `sensitive` flag that distinguishes authorization facts (D-016's owner fact is
// sensitive:true, the advisory quota fact sensitive:false) lives on the RESOLVED
// EvaluationInput fact, not on the provider config, so it is unavailable here.
// So "controlling/authorization-class" is derived STRUCTURALLY from the pack: a
// provider is authorization-class iff some rule whose obligation escalates to
// `require-review` on failure (ADR-0017 §3 — the forge-proven eligible-approval
// effect, the authorization mechanism) reads that provider's facts in its
// prove.when. Such a provider configured `failure: open` is rejected; an advisory
// provider (referenced only by comment/challenge/block rules, or by no obligation
// proof at all) may fail open.
//
// This is the NARROWEST defensible rule the frozen contract supports (it invents
// no schema field). It catches the ownership-style archetype in
// lint-hard-errors.md (a provider read by a require-review proof). It does NOT
// catch the other two archetypes that table names — an entries-identity provider
// referenced only from an Entry's identity.pointer, or an approval-eligibility
// provider proven via injected ApprovalEvidence (E2-S07, never a facts.<provider>
// reference). Widening to those is E3's `assent lint`, the authoritative,
// human-facing version of this rule; this engine-side check is its structural-
// refusal twin for the ownership case. policy stays off cel-go (S01), so the
// reference scan is a token match on the verbatim when text, not a CEL compile —
// sound because the frozen predicate scope addresses facts by dot syntax
// (facts.<provider>.<name>...), never bracket indexing.

// factRefRe matches a `facts.<provider>` dot reference in a verbatim CEL leaf,
// capturing the provider identifier.
var factRefRe = regexp.MustCompile(`\bfacts\.([A-Za-z_][A-Za-z0-9_]*)`)

// ValidateProviderPosture rejects a provider that backs a controlling/
// authorization fact (feeds a require-review obligation proof) while configured
// `failure: open` (ADR-0017 §6). It is pure (no clock/env/network/random) and a
// no-op when either argument is nil.
func ValidateProviderPosture(cfg *Config, mp *MergePolicy) error {
	if cfg == nil || mp == nil {
		return nil
	}
	for _, r := range mp.Spec.Rules {
		if r.Prove == nil || r.OnFailure == nil || r.OnFailure.Effect != EffectRequireReview {
			continue
		}
		for _, prov := range providersReferenced(r.Prove.When) {
			p, ok := cfg.Providers[prov]
			if !ok {
				continue
			}
			if p.Failure == "open" {
				return fmt.Errorf("provider %q backs authorization obligation %q (rule %q escalates to require-review) and may not be configured failure: open — a controlling fact must fail closed (ADR-0017 §6, lint-hard-errors.md)", prov, r.Prove.Obligation, r.Name)
			}
		}
	}
	return nil
}

// providersReferenced returns, deduplicated in first-appearance order, the
// providers named by facts.<provider> references across every leaf of an assert
// tree (all/any/not walked recursively).
func providersReferenced(t AssertTree) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(AssertTree)
	walk = func(n AssertTree) {
		if n.Leaf != nil {
			for _, m := range factRefRe.FindAllStringSubmatch(n.Leaf.CEL, -1) {
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
