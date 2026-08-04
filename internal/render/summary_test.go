package render_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/render"
)

// REQ-E8-S13-03: summary markdown golden in examples/render/<case>/expect.summary.md.
func TestRenderSummaryGolden(t *testing.T) {
	for _, name := range renderGoldenCases {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(renderExamplesRoot, name)
			pmRaw, err := os.ReadFile(filepath.Join(dir, "presentation-model.json")) //nolint:gosec // explicit fixture tree
			if err != nil {
				t.Fatalf("read presentation-model.json: %v", err)
			}
			ctxRaw, err := os.ReadFile(filepath.Join(dir, "render-context.json")) //nolint:gosec // explicit fixture tree
			if err != nil {
				t.Fatalf("read render-context.json: %v", err)
			}
			expectRaw, err := os.ReadFile(filepath.Join(dir, "expect.summary.md")) //nolint:gosec // explicit fixture tree
			if err != nil {
				t.Fatalf("read expect.summary.md: %v", err)
			}

			fx, err := render.LoadPresentationModel(pmRaw)
			if err != nil {
				t.Fatalf("LoadPresentationModel: %v", err)
			}
			ctx, err := render.LoadRenderContext(ctxRaw)
			if err != nil {
				t.Fatalf("LoadRenderContext: %v", err)
			}

			got, err := render.RenderSummary(fx.Presentation, ctx)
			if err != nil {
				t.Fatalf("RenderSummary: %v", err)
			}
			got = normalizeGoldenMarkdown(got)
			want := normalizeGoldenMarkdown(string(expectRaw))
			if got != want {
				t.Fatalf("summary golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
			}
		})
	}
}

func TestGenerateRenderSummaryGolden(t *testing.T) {
	if os.Getenv("ASSENT_UPDATE_RENDER_GOLDENS") != "1" {
		t.Skip("set ASSENT_UPDATE_RENDER_GOLDENS=1 to rewrite expect.summary.md files")
	}
	for _, name := range renderGoldenCases {
		dir := filepath.Join(renderExamplesRoot, name)
		pmRaw, err := os.ReadFile(filepath.Join(dir, "presentation-model.json")) //nolint:gosec // explicit fixture tree
		if err != nil {
			t.Fatalf("%s: read presentation-model: %v", name, err)
		}
		ctxRaw, err := os.ReadFile(filepath.Join(dir, "render-context.json")) //nolint:gosec // explicit fixture tree
		if err != nil {
			t.Fatalf("%s: read render-context: %v", name, err)
		}
		fx, err := render.LoadPresentationModel(pmRaw)
		if err != nil {
			t.Fatalf("%s: LoadPresentationModel: %v", name, err)
		}
		ctx, err := render.LoadRenderContext(ctxRaw)
		if err != nil {
			t.Fatalf("%s: LoadRenderContext: %v", name, err)
		}
		got, err := render.RenderSummary(fx.Presentation, ctx)
		if err != nil {
			t.Fatalf("%s: RenderSummary: %v", name, err)
		}
		out := filepath.Join(dir, "expect.summary.md")
		if err := os.WriteFile(out, []byte(normalizeGoldenMarkdown(got)), 0o600); err != nil { //nolint:gosec // G306/G703 — golden refresh; out is under fixed renderExamplesRoot/<case>/
			t.Fatalf("%s: write expect.summary.md: %v", name, err)
		}
	}
}
