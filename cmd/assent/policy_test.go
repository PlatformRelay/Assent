package main

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

const goodPolicy = `subject: "file:topics/orders.events.v1.yaml"
require: [partitions-not-decreased]
rules:
  - name: partitions-monotonic
    obligation: partitions-not-decreased
    when: "int(new) >= int(old)"
    onFailure: { effect: challenge, code: PARTITIONS_DECREASED }
`

func TestParsePolicyGood(t *testing.T) {
	b, err := ParsePolicy([]byte(goodPolicy))
	if err != nil {
		t.Fatalf("ParsePolicy: %v", err)
	}
	if b.Subject != "file:topics/orders.events.v1.yaml" {
		t.Errorf("subject = %q", b.Subject)
	}
	if len(b.Require) != 1 || b.Require[0] != "partitions-not-decreased" {
		t.Errorf("require = %v", b.Require)
	}
	if len(b.Rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(b.Rules))
	}
	r := b.Rules[0]
	if r.Name != "partitions-monotonic" || r.Obligation != "partitions-not-decreased" {
		t.Errorf("rule name/obligation = %q/%q", r.Name, r.Obligation)
	}
	if r.When != "int(new) >= int(old)" {
		t.Errorf("when = %q", r.When)
	}
	if r.OnFailure.Effect != aggregate.EffectChallenge {
		t.Errorf("effect = %q, want challenge", r.OnFailure.Effect)
	}
	if r.OnFailure.Code != "PARTITIONS_DECREASED" {
		t.Errorf("code = %q", r.OnFailure.Code)
	}
}

func TestParsePolicyFailClosed(t *testing.T) {
	cases := map[string]string{
		"bad yaml":           "\tnot: yaml: [",
		"no subject":         "require: [x]\nrules: []\n",
		"empty require":      "subject: file:a\nrequire: []\nrules: []\n",
		"unknown effect":     "subject: file:a\nrequire: [x]\nrules:\n  - name: r\n    obligation: x\n    when: \"true\"\n    onFailure: { effect: nuke, code: C }\n",
		"missing code":       "subject: file:a\nrequire: [x]\nrules:\n  - name: r\n    obligation: x\n    when: \"true\"\n    onFailure: { effect: block }\n",
		"missing when":       "subject: file:a\nrequire: [x]\nrules:\n  - name: r\n    obligation: x\n    onFailure: { effect: block, code: C }\n",
		"missing name":       "subject: file:a\nrequire: [x]\nrules:\n  - obligation: x\n    when: \"true\"\n    onFailure: { effect: block, code: C }\n",
		"unknown field":      "subject: file:a\nrequire: [x]\nbogus: true\nrules: []\n",
		"missing obligation": "subject: file:a\nrequire: [x]\nrules:\n  - name: r\n    when: \"true\"\n    onFailure: { effect: block, code: C }\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePolicy([]byte(raw)); err == nil {
				t.Fatalf("%s: expected error, got nil", name)
			}
		})
	}
}

func TestParsePolicyAllEffects(t *testing.T) {
	for effStr, effEnum := range map[string]aggregate.Effect{
		"comment":        aggregate.EffectComment,
		"challenge":      aggregate.EffectChallenge,
		"block":          aggregate.EffectBlock,
		"require-review": aggregate.EffectRequireReview,
	} {
		raw := "subject: file:a\nrequire: [x]\nrules:\n  - name: r\n    obligation: x\n    when: \"true\"\n    onFailure: { effect: " + effStr + ", code: C }\n"
		b, err := ParsePolicy([]byte(raw))
		if err != nil {
			t.Fatalf("effect %q: %v", effStr, err)
		}
		if b.Rules[0].OnFailure.Effect != effEnum {
			t.Errorf("effect %q mapped to %q", effStr, b.Rules[0].OnFailure.Effect)
		}
	}
}

func TestParsePolicyErrorMentionsRule(t *testing.T) {
	raw := "subject: file:a\nrequire: [x]\nrules:\n  - name: myrule\n    obligation: x\n    when: \"true\"\n    onFailure: { effect: bogus, code: C }\n"
	_, err := ParsePolicy([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "myrule") {
		t.Fatalf("error should name the rule: %v", err)
	}
}
