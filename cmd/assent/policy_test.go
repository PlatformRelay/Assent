package main

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/forge"
)

// selectBinding routes to the single covering binding and fails CLOSED on zero or
// ambiguous (multi-binding) documents — the (class, environment) matcher is not
// wired in this lane, so it never silently picks one.
func TestSelectBindingSingle(t *testing.T) {
	rb := &policy.RulesetBinding{Bindings: []policy.Binding{{
		Class: "topic-registry", Environment: "prod", Require: []string{"non-destructive"},
	}}}
	got, err := selectBinding(rb)
	if err != nil {
		t.Fatalf("selectBinding: %v", err)
	}
	if got.Class != "topic-registry" || got.Environment != "prod" {
		t.Errorf("selected = %+v, want the single topic-registry/prod binding", got)
	}
}

func TestSelectBindingFailsClosed(t *testing.T) {
	cases := map[string]*policy.RulesetBinding{
		"nil":       nil,
		"zero":      {Bindings: nil},
		"ambiguous": {Bindings: []policy.Binding{{Class: "a"}, {Class: "b"}}},
	}
	for name, rb := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := selectBinding(rb); err == nil {
				t.Fatalf("%s: expected selectBinding to fail closed, got nil", name)
			}
		})
	}
}

// sanitizeSubjects gives every finding a non-empty subject (N1): an uncovered
// obligation (empty subject) gets a per-obligation sentinel so two uncovered
// obligations never collide on the (rule, subject) uniqueKey; a finding already
// carrying a subject is untouched; a subjectless, obligationless finding falls back.
func TestSanitizeSubjects(t *testing.T) {
	res := aggregate.Result{Findings: []aggregate.Finding{
		{Rule: "aggregate.uncovered", Obligation: "ownership", Effect: aggregate.EffectRequireReview},
		{Rule: "aggregate.uncovered", Obligation: "non-destructive", Effect: aggregate.EffectRequireReview},
		{Rule: "real", Obligation: "x", Subject: "file:topics/a.yaml", Effect: aggregate.EffectBlock},
		{Rule: "bare", Effect: aggregate.EffectRequireReview}, // no subject, no obligation
	}}
	out := sanitizeSubjects(res, "file:fallback.yaml")
	want := []string{"obligation:ownership", "obligation:non-destructive", "file:topics/a.yaml", "file:fallback.yaml"}
	for i, w := range want {
		if out.Findings[i].Subject != w {
			t.Errorf("finding[%d].Subject = %q, want %q", i, out.Findings[i].Subject, w)
		}
	}
	// No finding may end up with an empty subject (the schema minLength:1 invariant).
	for _, f := range out.Findings {
		if f.Subject == "" {
			t.Errorf("finding still has empty subject: %+v", f)
		}
	}
}

// The reserved-class self-edit result reconstructs the aggregate.Aggregate block
// exactly (GUARD 1): BLOCK with the assent-policy.self-edit finding on the subject.
func TestReservedClassBlock(t *testing.T) {
	res := reservedClassBlock("file:topics/orders.yaml")
	if res.Decision != aggregate.DecisionBlock {
		t.Fatalf("decision = %q, want BLOCK", res.Decision)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(res.Findings))
	}
	f := res.Findings[0]
	if f.Rule != aggregate.ReservedPolicyClass || f.Effect != aggregate.EffectBlock || f.Code != "assent-policy.self-edit" {
		t.Errorf("reserved finding = %+v, want assent-policy self-edit block", f)
	}
	if f.Subject != "file:topics/orders.yaml" {
		t.Errorf("subject = %q, want the governed subject", f.Subject)
	}
}

func TestReservedSelfEditBlock(t *testing.T) {
	if !reservedSelfEditBlock(reservedClassBlock("file:x.yaml")) {
		t.Fatal("assent-policy.self-edit BLOCK must be detected for reconcile skip")
	}
	if reservedSelfEditBlock(undecidableReview("file:x.yaml")) {
		t.Fatal("REVIEW must not trigger reconcile skip")
	}
}

// The undecidable (opaque/empty) result fails safe to REVIEW with an auditable
// finding — never a silent APPROVE.
func TestUndecidableReview(t *testing.T) {
	res := undecidableReview("file:topics/orders.yaml")
	if res.Decision != aggregate.DecisionReview {
		t.Fatalf("decision = %q, want REVIEW", res.Decision)
	}
	if len(res.Findings) != 1 || res.Findings[0].Code != "changeset.undecidable" {
		t.Errorf("findings = %+v, want one changeset.undecidable finding", res.Findings)
	}
}

// subjectOf prefers a collection EntryRef and falls back to file:<path>.
func TestSubjectOf(t *testing.T) {
	if got := subjectOf(change.Change{EntryRef: "topic-registry:orders.events.v1", File: "topics/x.yaml"}); got != "topic-registry:orders.events.v1" {
		t.Errorf("subjectOf(entryRef) = %q, want the EntryRef", got)
	}
	if got := subjectOf(change.Change{File: "topics/x.yaml"}); got != "file:topics/x.yaml" {
		t.Errorf("subjectOf(no entryRef) = %q, want file:<path> fallback", got)
	}
}

// mrFrom threads branch names and MR author from forge Snapshot heads (E4-S06).
func TestMRFrom(t *testing.T) {
	mr := mrFrom(forge.MRInfo{SourceBranch: "feature", TargetBranch: "main"}, "alice")
	if mr.SourceBranch != "feature" || mr.TargetBranch != "main" {
		t.Errorf("mrFrom = %+v, want source=feature target=main", mr)
	}
	if mr.Author != "alice" {
		t.Errorf("author = %q, want alice from Snapshot heads", mr.Author)
	}
}
