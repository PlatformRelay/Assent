package render_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/render"
)

const renderExamplesRoot = "../../examples/render"

// renderGoldenCases are the E8-S09 committed corpus directories (D-092).
var renderGoldenCases = []string{"challenge", "block", "require-review"}

// REQ-E8-S09-01 / REQ-E8-S09-02: each examples/render/<case>/ fixture renders
// byte-identically to expect.finding-thread.md; double-run is enforced by -count=2.
func TestRenderGoldens(t *testing.T) {
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
			expectRaw, err := os.ReadFile(filepath.Join(dir, "expect.finding-thread.md")) //nolint:gosec // explicit fixture tree
			if err != nil {
				t.Fatalf("read expect.finding-thread.md: %v", err)
			}

			fx, err := render.LoadPresentationModel(pmRaw)
			if err != nil {
				t.Fatalf("LoadPresentationModel: %v", err)
			}
			ctx, err := render.LoadRenderContext(ctxRaw)
			if err != nil {
				t.Fatalf("LoadRenderContext: %v", err)
			}
			if len(fx.Presentation.Findings) == 0 {
				t.Fatal("presentation-model has no findings to render")
			}
			finding := fx.Presentation.Findings[0]

			got, err := render.RenderFindingThread(fx.Presentation, finding, ctx)
			if err != nil {
				t.Fatalf("RenderFindingThread: %v", err)
			}
			got = normalizeGoldenMarkdown(got)
			want := normalizeGoldenMarkdown(string(expectRaw))
			if got != want {
				t.Fatalf("golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
			}
		})
	}
}

// normalizeGoldenMarkdown canonicalizes line endings so goldens are portable and
// byte-comparable on every platform (REQ-E8-S09-01).
func normalizeGoldenMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n") + "\n"
}

// TestLoadRenderContextFixture exercises LoadRenderContext against a committed case.
func TestLoadRenderContextFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(renderExamplesRoot, "challenge", "render-context.json")) //nolint:gosec // explicit fixture tree
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	ctx, err := render.LoadRenderContext(raw)
	if err != nil {
		t.Fatalf("LoadRenderContext: %v", err)
	}
	if ctx.Options.Verbosity != render.VerbosityStandard {
		t.Fatalf("verbosity = %q, want standard", ctx.Options.Verbosity)
	}
	if ctx.Activation == nil {
		t.Fatal("expected activation")
	}
	if len(ctx.Rules) == 0 {
		t.Fatal("expected rules metadata")
	}
	if _, ok := ctx.Activation.(map[string]any); !ok {
		t.Fatalf("activation type = %T, want map[string]any", ctx.Activation)
	}
}

// generateRenderGolden is a dev helper invoked manually when seeding expects; kept
// here so `go test -run generateRenderGolden` can refresh goldens locally.
func TestGenerateRenderGolden(t *testing.T) {
	if os.Getenv("ASSENT_UPDATE_RENDER_GOLDENS") != "1" {
		t.Skip("set ASSENT_UPDATE_RENDER_GOLDENS=1 to rewrite expect.finding-thread.md files")
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
		got, err := render.RenderFindingThread(fx.Presentation, fx.Presentation.Findings[0], ctx)
		if err != nil {
			t.Fatalf("%s: RenderFindingThread: %v", name, err)
		}
		out := filepath.Join(dir, "expect.finding-thread.md")
		if err := os.WriteFile(out, []byte(normalizeGoldenMarkdown(got)), 0o600); err != nil { //nolint:gosec // G306/G703 — golden refresh; out is under fixed renderExamplesRoot/<case>/
			t.Fatalf("%s: write expect: %v", name, err)
		}
		if !bytes.HasSuffix([]byte(got), []byte("\n")) {
			t.Logf("%s: renderer output had no trailing newline before normalize", name)
		}
	}
}
