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
