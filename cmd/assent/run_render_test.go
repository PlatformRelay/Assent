package main

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/catalogue"
	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/decision"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/render"
)

// REQ-E8-S08-03: buildDesired uses renderer output (no fmt.Sprintf stub).
func TestBuildDesiredUsesRenderer(t *testing.T) {
	result := aggregate.Result{
		Decision: aggregate.DecisionReview,
		Findings: []aggregate.Finding{{
			Rule:       "partitions-must-not-shrink",
			Obligation: "non-destructive",
			Effect:     aggregate.EffectChallenge,
			Subject:    "topic-registry:orders.events.v1",
			Code:       "partition-count-shrunk",
		}},
	}
	pins := decision.Pins{
		ToolVersion: "test",
		ToolDigest:  "sha256:abc",
		PolicySha:   "sha256:def",
		SourceSha:   "src",
		TargetSha:   "tgt",
		MergeResult: decision.SkeletonMergeGap(),
	}
	report, err := decision.Build(result, pins)
	if err != nil {
		t.Fatalf("decision.Build: %v", err)
	}
	recordJSON, err := report.MarshalRecord()
	if err != nil {
		t.Fatalf("MarshalRecord: %v", err)
	}

	mp := &policy.MergePolicy{
		Metadata: policy.Metadata{Name: "topic-safety"},
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:    "partitions-must-not-shrink",
				Message: "Partitions {{ old }} -> {{ new }}",
			}},
		},
	}
	bind := &policy.Binding{Environment: "prod"}
	cs := change.ChangeSet{Changes: []change.Change{{
		Path: "/partitions",
		Kind: change.KindModify,
		Old:  "5",
		New:  "3",
	}}}
	rctx := buildRenderContext(render.DefaultOptions(), mp, bind, cs, nil, forge.MRInfo{}, "")

	cfg := runConfig{project: "42", mr: "7", subject: "file:topics/orders.yaml"}
	info := forge.MRInfo{SourceSHA: "src", TargetSHA: "tgt"}
	head := []byte("head-bytes")

	desired, _ := buildDesired(cfg, info, cfg.subject, head, result, recordJSON, false, report.Presentation, rctx)
	if desired.Thread == nil {
		t.Fatal("expected thread for REVIEW")
	}
	body := desired.Thread.Body
	if strings.Contains(body, "assent review required:") {
		t.Fatalf("buildDesired still uses fmt.Sprintf stub:\n%s", body)
	}
	if !strings.Contains(body, "Resolve this thread to confirm") {
		t.Fatalf("thread body missing renderer resolve CTA:\n%s", body)
	}
	if !strings.Contains(body, "Evaluation details") {
		t.Fatalf("thread body missing evaluation details block:\n%s", body)
	}
	if strings.Contains(body, "assent:marker") {
		t.Fatalf("buildDesired must pass renderer body only; forge CreateThread owns Envelope:\n%s", body)
	}
}

// REQ-E8-S13-01: buildDesired populates Summary with renderer output for REVIEW/BLOCK/APPROVE.
func TestBuildDesiredSummaryUsesRenderer(t *testing.T) {
	mp := &policy.MergePolicy{
		Metadata: policy.Metadata{Name: "topic-safety"},
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:    "partitions-must-not-shrink",
				Message: "Partitions {{ old }} -> {{ new }}",
			}},
		},
	}
	bind := &policy.Binding{Environment: "prod", Risk: policy.Risk{Threshold: 4}}
	cs := change.ChangeSet{Changes: []change.Change{{
		Path: "/partitions", Kind: change.KindModify, Old: "5", New: "3",
	}}}
	cfg := runConfig{project: "42", mr: "7", subject: "file:topics/orders.yaml"}
	info := forge.MRInfo{SourceSHA: "src", TargetSHA: "tgt"}
	head := []byte("head-bytes")

	cases := []struct {
		name     string
		decision aggregate.Decision
		findings []aggregate.Finding
		wantIn   string
	}{
		{
			name:     "REVIEW",
			decision: aggregate.DecisionReview,
			findings: []aggregate.Finding{{
				Rule: "partitions-must-not-shrink", Obligation: "non-destructive",
				Effect: aggregate.EffectChallenge, Subject: "topic-registry:orders.events.v1",
				Code: "partition-count-shrunk", Points: 10,
			}},
			wantIn: "Policy evaluation",
		},
		{
			name:     "BLOCK",
			decision: aggregate.DecisionBlock,
			findings: []aggregate.Finding{{
				Rule: "partitions-must-not-shrink", Obligation: "non-destructive",
				Effect: aggregate.EffectBlock, Subject: "topic-registry:orders.events.v1",
				Code: "partition-count-shrunk", Points: 10,
			}},
			wantIn: "BLOCK",
		},
		{
			name:     "APPROVE",
			decision: aggregate.DecisionApprove,
			findings: nil,
			wantIn:   "APPROVE",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := aggregate.Result{Decision: tc.decision, Findings: tc.findings}
			report, err := decision.Build(result, decision.Pins{
				ToolVersion: "test", ToolDigest: "sha256:abc", PolicySha: "sha256:def",
				SourceSha: "src", TargetSha: "tgt", MergeResult: decision.SkeletonMergeGap(),
			})
			if err != nil {
				t.Fatalf("decision.Build: %v", err)
			}
			recordJSON, err := report.MarshalRecord()
			if err != nil {
				t.Fatalf("MarshalRecord: %v", err)
			}
			rctx := buildRenderContext(render.DefaultOptions(), mp, bind, cs, nil, info, "")
			desired, _ := buildDesired(cfg, info, cfg.subject, head, result, recordJSON, false, report.Presentation, rctx)
			if desired.Summary == nil {
				t.Fatal("expected Summary for all decision outcomes")
			}
			body := desired.Summary.Body
			if !strings.Contains(body, tc.wantIn) {
				t.Fatalf("summary body missing %q:\n%s", tc.wantIn, body)
			}
			if strings.Contains(body, "assent:marker") {
				t.Fatalf("buildDesired must pass renderer body only; forge UpsertComment owns Envelope:\n%s", body)
			}
			if desired.Summary.Marker.Artifact.Kind != "summary-comment" {
				t.Fatalf("summary marker kind = %q, want summary-comment", desired.Summary.Marker.Artifact.Kind)
			}
		})
	}
}

// DOC-03 (audit 2026-08-09): the run path must not MINT a documentation URL.
//
// `internal/catalogue.DocsBase + "/" + pack + "/" + rule` was attached to every
// finding on the run path and rendered as `📖 [Full documentation](…)` in the
// contributor's MR thread — but no `rules/` space exists on the docs site
// (measured: `<site>/rules` and `<site>/rules/` both 404), so the one affordance
// a blocked contributor clicks was dead on every finding of every MR.
//
// Both polarities, because "no link" is also what deleting the feature outright
// would produce and that must not pass as a fix:
//
//	absent  → no link line at all (and no docs-site URL anywhere in the body);
//	authored → the AUTHORED url, verbatim, is the one that renders.
//
// The authored case is unreachable from a conformant pack TODAY: the frozen
// v1alpha1 merge-policy schema is `additionalProperties: false` over
// [effect, match, message, name, onFailure, phase, points, prove], so a rule-level
// `docs:` is rejected at load (audit ARCH-08). It is pinned here anyway so that
// closing ARCH-08 wires an authored URL through instead of re-minting a dead one.
func TestRunPathDocsLinkIsAuthoredNeverMinted(t *testing.T) {
	for _, tc := range []struct {
		name      string
		docsURL   string
		wantLink  bool
		wantInURL string
	}{
		{name: "absent_authored_url_renders_no_link", docsURL: "", wantLink: false},
		{name: "authored_url_renders_verbatim", docsURL: "https://docs.example.test/rules/partitions", wantLink: true, wantInURL: "https://docs.example.test/rules/partitions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := aggregate.Result{
				Decision: aggregate.DecisionReview,
				Findings: []aggregate.Finding{{
					Rule:       "partitions-must-not-shrink",
					Obligation: "non-destructive",
					Effect:     aggregate.EffectChallenge,
					Subject:    "topic-registry:orders.events.v1",
					Code:       "partition-count-shrunk",
				}},
			}
			pins := decision.Pins{
				ToolVersion: "test",
				ToolDigest:  "sha256:abc",
				PolicySha:   "sha256:def",
				SourceSha:   "src",
				TargetSha:   "tgt",
				MergeResult: decision.SkeletonMergeGap(),
			}
			report, err := decision.Build(result, pins)
			if err != nil {
				t.Fatalf("decision.Build: %v", err)
			}
			recordJSON, err := report.MarshalRecord()
			if err != nil {
				t.Fatalf("MarshalRecord: %v", err)
			}

			mp := &policy.MergePolicy{
				Metadata: policy.Metadata{Name: "topic-safety"},
				Spec: policy.MergePolicySpec{
					Rules: []policy.Rule{{
						Name:    "partitions-must-not-shrink",
						Message: "Partitions {{ old }} -> {{ new }}",
						Docs:    policy.RuleDocs{URL: tc.docsURL},
					}},
				},
			}
			bind := &policy.Binding{Environment: "prod"}
			cs := change.ChangeSet{Changes: []change.Change{{
				Path: "/partitions",
				Kind: change.KindModify,
				Old:  "5",
				New:  "3",
			}}}
			rctx := buildRenderContext(render.DefaultOptions(), mp, bind, cs, nil, forge.MRInfo{}, "")

			cfg := runConfig{project: "42", mr: "7", subject: "file:topics/orders.yaml"}
			info := forge.MRInfo{SourceSHA: "src", TargetSHA: "tgt"}
			desired, _ := buildDesired(cfg, info, cfg.subject, []byte("head-bytes"), result, recordJSON, false, report.Presentation, rctx)
			if desired.Thread == nil {
				t.Fatal("expected thread for REVIEW")
			}
			body := desired.Thread.Body

			// The dead space must never reach a contributor. The needle is derived
			// from catalogue.DocsBase rather than written out, so it tracks that
			// constant instead of going stale beside it — and so this file holds no
			// literal docs-site URL for the DOC-02 site_url pin to trip over.
			for _, dead := range []string{catalogue.DocsBase, "github.io", "/rules/topic-safety"} {
				if strings.Contains(body, dead) {
					t.Errorf("run path minted a docs URL containing %q into the MR thread:\n%s", dead, body)
				}
			}

			hasLink := strings.Contains(body, "Full documentation")
			if hasLink != tc.wantLink {
				t.Errorf("Full documentation link present = %v, want %v:\n%s", hasLink, tc.wantLink, body)
			}
			if tc.wantInURL != "" && !strings.Contains(body, tc.wantInURL) {
				t.Errorf("authored docs.url %q did not reach the thread body:\n%s", tc.wantInURL, body)
			}
		})
	}
}
