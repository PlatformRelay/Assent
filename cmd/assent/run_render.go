package main

import (
	"github.com/PlatformRelay/assent/internal/catalogue"
	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/decision"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/forge/gitlab"
	"github.com/PlatformRelay/assent/internal/render"
)

// buildRenderContext assembles the ephemeral render.Context for forge thread bodies (E8-S08).
func buildRenderContext(
	opts render.Options,
	mp *policy.MergePolicy,
	bind *policy.Binding,
	cs change.ChangeSet,
	facts map[string]map[string]aggregate.Fact,
	info gitlab.MRInfo,
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
	pack := mp.Metadata.Name
	out := make(map[string]render.RuleMeta, len(mp.Spec.Rules))
	for _, r := range mp.Spec.Rules {
		stableID := pack + "/" + r.Name
		out[r.Name] = render.RuleMeta{
			Message: r.Message,
			Docs: render.RuleDocs{
				URL: catalogue.DocsBase + "/" + stableID,
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
