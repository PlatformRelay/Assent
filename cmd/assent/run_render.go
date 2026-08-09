package main

import (
	"strings"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/decision"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/render"
)

// buildRenderContext assembles the ephemeral render.Context for forge thread bodies (E8-S08).
func buildRenderContext(
	opts render.Options,
	mp *policy.MergePolicy,
	bind *policy.Binding,
	cs change.ChangeSet,
	facts map[string]map[string]aggregate.Fact,
	info forge.MRInfo,
	mrAuthor string,
) render.Context {
	env := ""
	require := []string(nil)
	if bind != nil {
		env = bind.Environment
		require = bind.Require
	}
	in := buildEvaluationInput(cs, mrFrom(info, mrAuthor), require)
	if len(facts) > 0 {
		in.Facts = facts
	}
	var activation any = map[string]any{"env": env}
	if len(in.ChangeSet.Changes) > 0 {
		activation = aggregate.LeafActivation(in, in.ChangeSet.Changes[0], env)
	}
	threshold := 0
	if bind != nil {
		threshold = bind.Risk.Threshold
	}
	return render.Context{
		Options:       opts,
		Activation:    activation,
		Rules:         rulesMetaFromPolicy(mp),
		RiskThreshold: threshold,
	}
}

func rulesMetaFromPolicy(mp *policy.MergePolicy) map[string]render.RuleMeta {
	if mp == nil {
		return nil
	}
	out := make(map[string]render.RuleMeta, len(mp.Spec.Rules))
	for _, r := range mp.Spec.Rules {
		// DOC-03: do NOT mint `catalogue.DocsBase + "/" + pack + "/" + r.Name`
		// here. That URL space does not exist on the docs site — measured, both
		// `<site>/rules` and `<site>/rules/` 404 — so every finding posted into a
		// contributor's MR thread carried a dead "Full documentation" link, on the
		// one affordance a blocked contributor actually clicks. The run path now
		// carries only the rule's AUTHORED docs.url; `renderDocsSection` omits the
		// line entirely when it is empty, so no link beats a broken one.
		//
		// Today that fallback is DORMANT BY SCHEMA, not merely unused: the frozen
		// v1alpha1 merge-policy schema is `additionalProperties: false` over
		// [effect, match, message, name, onFailure, phase, points, prove], so an
		// authored rule-level `docs:` is REJECTED at load and `policy.Rule.Docs`
		// can never be populated from a conformant pack (audit ARCH-08). Adopters
		// cannot supply a URL here yet — the honest present-day behaviour is
		// therefore "no documentation link on the run path", which is the point.
		out[r.Name] = render.RuleMeta{
			Message: r.Message,
			Docs: render.RuleDocs{
				URL: strings.TrimSpace(r.Docs.URL),
			},
		}
	}
	return out
}

func presentationFinding(result aggregate.Result) decision.Finding {
	if len(result.Findings) == 0 {
		return decision.Finding{
			Rule:    "aggregate.changeset",
			Effect:  "require-review",
			Subject: "",
			Code:    "changeset.undecidable",
		}
	}
	f := result.Findings[0]
	return decision.Finding{
		Rule:       f.Rule,
		Obligation: f.Obligation,
		Effect:     string(f.Effect),
		Subject:    f.Subject,
		Points:     f.Points,
		Code:       f.Code,
	}
}
