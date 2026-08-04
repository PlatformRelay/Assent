package main

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/decision"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/forge/gitlab"
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
	rctx := buildRenderContext(render.DefaultOptions(), mp, bind, cs, nil, gitlab.MRInfo{}, "")

	cfg := runConfig{project: "42", mr: "7", subject: "file:topics/orders.yaml"}
	info := gitlab.MRInfo{SourceSHA: "src", TargetSHA: "tgt"}
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
	info := gitlab.MRInfo{SourceSHA: "src", TargetSHA: "tgt"}
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
