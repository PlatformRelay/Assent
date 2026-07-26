package main

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// policyDoc is the YAML shape of a walking-skeleton policy (P4-E1-S10). It maps
// 1:1 onto an aggregate.Binding. The format is deliberately small — one subject,
// a require list, and assert rules that each prove one obligation:
//
//	subject: "file:topics/orders.events.v1.yaml"
//	require: [partitions-not-decreased]
//	rules:
//	  - name: partitions-monotonic
//	    obligation: partitions-not-decreased
//	    when: "int(new) >= int(old)"
//	    onFailure: { effect: challenge, code: PARTITIONS_DECREASED }
//
// This is a PURE helper (it reads only its input bytes — no clock, env, or
// network) so cmd/assent stays the only side-effecting edge. Parsing is
// FAIL-CLOSED: any structural defect (bad YAML, missing subject/require, an
// unknown effect, an empty rule field) returns an error and the caller performs
// NO forge writes.
type policyDoc struct {
	Subject string         `yaml:"subject"`
	Require []string       `yaml:"require"`
	Rules   []policyRuleYA `yaml:"rules"`
}

type policyRuleYA struct {
	Name       string `yaml:"name"`
	Obligation string `yaml:"obligation"`
	When       string `yaml:"when"`
	OnFailure  struct {
		Effect string `yaml:"effect"`
		Code   string `yaml:"code"`
	} `yaml:"onFailure"`
}

// validEffects is the closed set of onFailure effect strings, mapped to the
// aggregate.Effect enum. An effect outside this set is a policy error (fail
// closed) rather than a silently-dropped rule.
var validEffects = map[string]aggregate.Effect{
	"comment":        aggregate.EffectComment,
	"challenge":      aggregate.EffectChallenge,
	"block":          aggregate.EffectBlock,
	"require-review": aggregate.EffectRequireReview,
}

// ParsePolicy parses policy YAML bytes into an aggregate.Binding, failing closed
// on any structural defect. It uses KnownFields(true) so an unrecognised key is
// an error, never silently ignored — a typo'd `obligaton:` must not degrade into
// an uncovered obligation that fails open.
func ParsePolicy(raw []byte) (aggregate.Binding, error) {
	var doc policyDoc
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return aggregate.Binding{}, fmt.Errorf("parse policy: %w", err)
	}

	if doc.Subject == "" {
		return aggregate.Binding{}, fmt.Errorf("parse policy: subject is required")
	}
	if len(doc.Require) == 0 {
		// An empty require would let the aggregator APPROVE by default (nothing to
		// prove). Fail closed: a policy that arms nothing is a defect.
		return aggregate.Binding{}, fmt.Errorf("parse policy: require must list at least one obligation")
	}

	rules := make([]aggregate.Rule, 0, len(doc.Rules))
	for i, r := range doc.Rules {
		if r.Name == "" {
			return aggregate.Binding{}, fmt.Errorf("parse policy: rule %d: name is required", i)
		}
		if r.Obligation == "" {
			return aggregate.Binding{}, fmt.Errorf("parse policy: rule %q: obligation is required", r.Name)
		}
		if r.When == "" {
			return aggregate.Binding{}, fmt.Errorf("parse policy: rule %q: when expression is required", r.Name)
		}
		effect, ok := validEffects[r.OnFailure.Effect]
		if !ok {
			return aggregate.Binding{}, fmt.Errorf("parse policy: rule %q: unknown onFailure effect %q", r.Name, r.OnFailure.Effect)
		}
		if r.OnFailure.Code == "" {
			return aggregate.Binding{}, fmt.Errorf("parse policy: rule %q: onFailure.code is required", r.Name)
		}
		rules = append(rules, aggregate.Rule{
			Name:       r.Name,
			Obligation: r.Obligation,
			When:       r.When,
			OnFailure:  aggregate.OnFailure{Effect: effect, Code: r.OnFailure.Code},
		})
	}

	return aggregate.Binding{
		Require: doc.Require,
		Rules:   rules,
		Subject: doc.Subject,
	}, nil
}
