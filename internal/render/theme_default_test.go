package render_test

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/decision"
	"github.com/PlatformRelay/assent/internal/render"
)

func standardThemeFixture() (decision.PresentationModel, decision.Finding, render.Context) {
	pm := decision.PresentationModel{
		APIVersion: "assent.dev/v1alpha1",
		Kind:       "PresentationModel",
		Decision:   "REVIEW",
		Findings: []decision.Finding{
			{
				Rule:       "partitions-must-not-shrink",
				Obligation: "non-destructive",
				Effect:     "challenge",
				Subject:    "topic-registry:orders.events.v1",
				Points:     0,
				Code:       "partition-count-shrunk",
			},
		},
	}
	finding := pm.Findings[0]
	ctx := render.Context{
		Options: render.Options{
			Verbosity:         "standard",
			Emoji:             true,
			CollapseThreshold: render.DefaultCollapseThreshold,
			Locale:            "en",
		},
		Activation: celMessageActivation(),
		Rules: map[string]render.RuleMeta{
			"partitions-must-not-shrink": {
				Message: "Retention shrinks from {{ old }} to {{ new }} — data loss possible. Sure?",
				Docs: render.RuleDocs{
					Summary: "Shrinking retention deletes data irreversibly once segments roll.",
					URL:     "https://example.com/policies/retention",
				},
				Debug: []string{"quota max {{ facts.quota.max_partitions }}"},
			},
		},
	}
	return pm, finding, ctx
}

// REQ-E8-S08-01: default layout includes resolve CTA + evaluation <details> for standard verbosity.
func TestDefaultThemeStandard(t *testing.T) {
	pm, finding, ctx := standardThemeFixture()

	got, err := render.RenderFindingThread(pm, finding, ctx)
	if err != nil {
		t.Fatalf("RenderFindingThread: %v", err)
	}

	resolve, _ := render.Chrome(ctx.Options, "resolve_thread")
	if !strings.Contains(got, resolve) {
		t.Fatalf("body missing resolve CTA %q:\n%s", resolve, got)
	}
	evalTitle, _ := render.Chrome(ctx.Options, "evaluation_details")
	if !strings.Contains(got, "<details>") || !strings.Contains(got, evalTitle) {
		t.Fatalf("body missing evaluation <details> with %q:\n%s", evalTitle, got)
	}
	if !strings.Contains(got, "**") {
		t.Fatalf("body missing bold headline:\n%s", got)
	}
	if !strings.Contains(got, "5") || !strings.Contains(got, "10") {
		t.Fatalf("body missing interpolated headline values:\n%s", got)
	}
}

// REQ-E8-S08-02: minimal verbosity omits evaluation details block.
func TestDefaultThemeMinimal(t *testing.T) {
	pm, finding, ctx := standardThemeFixture()
	ctx.Options.Verbosity = "minimal"

	got, err := render.RenderFindingThread(pm, finding, ctx)
	if err != nil {
		t.Fatalf("RenderFindingThread: %v", err)
	}

	evalTitle, _ := render.Chrome(ctx.Options, "evaluation_details")
	if strings.Contains(got, evalTitle) {
		t.Fatalf("minimal verbosity must omit evaluation details, got %q in:\n%s", evalTitle, got)
	}
	resolve, _ := render.Chrome(ctx.Options, "resolve_thread")
	if !strings.Contains(got, resolve) {
		t.Fatalf("minimal body must still include resolve CTA:\n%s", got)
	}
}

func TestDefaultThemeCollapsePaths(t *testing.T) {
	pm, finding, ctx := standardThemeFixture()
	ctx.Options.CollapseThreshold = 1
	pm.Findings = append(pm.Findings, decision.Finding{
		Rule: finding.Rule, Code: finding.Code, Effect: finding.Effect,
		Subject: "topic-registry:payments.v1",
	})

	got, err := render.RenderFindingThread(pm, finding, ctx)
	if err != nil {
		t.Fatalf("RenderFindingThread: %v", err)
	}
	collapsed, _ := render.Chrome(ctx.Options, "collapsed_paths")
	if !strings.Contains(got, collapsed) {
		t.Fatalf("expected collapsed paths line with %q:\n%s", collapsed, got)
	}
}

func TestDefaultThemeRequireReviewEmoji(t *testing.T) {
	pm, _, ctx := standardThemeFixture()
	finding := decision.Finding{
		Rule: "topic-owner-must-approve", Effect: "require-review",
		Subject: "s", Code: "ownership-approval-missing",
	}
	pm.Findings = []decision.Finding{finding}

	got, err := render.RenderFindingThread(pm, finding, ctx)
	if err != nil {
		t.Fatalf("RenderFindingThread: %v", err)
	}
	if !strings.Contains(got, "👀") {
		t.Fatalf("require-review headline should include emoji:\n%s", got)
	}
}
