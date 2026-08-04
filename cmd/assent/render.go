package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PlatformRelay/assent/internal/render"
)

// render.go is the thin filesystem+command shell for `assent render` (E8-S10).
// It loads a committed examples/render/<case>/ fixture (presentation-model.json +
// render-context.json), validates via the strict render loaders, renders markdown to
// stdout, and exits non-zero on validation/render errors with located stderr.

const (
	renderArtifactFindingThread = "finding-thread"
	renderArtifactSummary       = "summary"
)

type renderConfig struct {
	findingDir string
	artifact   string
	minimal    bool
	full       bool
}

// runRender is the testable entry point for `assent render`. Exit codes: 0 success,
// 1 render/validation failure, 2 usage/flag error.
func runRender(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseRenderFlags(expandFixtureAlias(args), stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 2
		}
		_, _ = fmt.Fprintln(stderr, "assent render:", err)
		return 2
	}
	if cfg.minimal && cfg.full {
		_, _ = fmt.Fprintln(stderr, "assent render: --presentation-minimal and --presentation-full are mutually exclusive")
		return 2
	}

	pmPath := filepath.Join(cfg.findingDir, "presentation-model.json")
	pmRaw, err := os.ReadFile(pmPath) // #nosec G703 G304 -- operator-supplied fixture dir, read-only.
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "assent render: read %s: %v\n", pmPath, err)
		return 1
	}
	ctxPath := filepath.Join(cfg.findingDir, "render-context.json")
	ctxRaw, err := os.ReadFile(ctxPath) // #nosec G703 G304 -- operator-supplied fixture dir, read-only.
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "assent render: read %s: %v\n", ctxPath, err)
		return 1
	}

	fx, err := render.LoadPresentationModel(pmRaw)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "assent render: %s: %v\n", pmPath, err)
		return 1
	}
	ctx, err := render.LoadRenderContext(ctxRaw)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "assent render: %s: %v\n", ctxPath, err)
		return 1
	}
	if cfg.minimal {
		ctx.Options.Verbosity = render.VerbosityMinimal
	}
	if cfg.full {
		ctx.Options.Verbosity = render.VerbosityFull
	}

	var out string
	switch cfg.artifact {
	case renderArtifactFindingThread:
		if len(fx.Presentation.Findings) == 0 {
			_, _ = fmt.Fprintf(stderr, "assent render: %s: presentation-model has no findings\n", pmPath)
			return 1
		}
		out, err = render.RenderFindingThread(fx.Presentation, fx.Presentation.Findings[0], ctx)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "assent render: %s: %v\n", cfg.findingDir, err)
			return 1
		}
	case renderArtifactSummary:
		_, _ = fmt.Fprintln(stderr, "assent render: artifact summary: not implemented yet (E8-S13 RenderSummary)")
		return 1
	default:
		_, _ = fmt.Fprintf(stderr, "assent render: unknown artifact %q (want finding-thread or summary)\n", cfg.artifact)
		return 2
	}

	_, _ = fmt.Fprint(stdout, out)
	return 0
}

func parseRenderFlags(args []string, stderr io.Writer) (renderConfig, error) {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg renderConfig
	cfg.artifact = renderArtifactFindingThread
	fs.StringVar(&cfg.findingDir, "finding", "", "render fixture directory under examples/render/<case> (required)")
	fs.StringVar(&cfg.artifact, "artifact", renderArtifactFindingThread, "artifact to render: finding-thread or summary")
	fs.BoolVar(&cfg.minimal, "presentation-minimal", false, "omit evaluation details (verbosity=minimal)")
	fs.BoolVar(&cfg.full, "presentation-full", false, "show all evaluation detail blocks (verbosity=full)")
	if err := fs.Parse(args); err != nil {
		return renderConfig{}, err
	}
	if cfg.findingDir == "" {
		return renderConfig{}, fmt.Errorf("--finding is required (usage: assent render --finding examples/render/<case> [--artifact finding-thread|summary] [--presentation-minimal|--presentation-full])")
	}
	return cfg, nil
}

// expandFixtureAlias rewrites the undocumented --fixture alias to --finding (D-097).
func expandFixtureAlias(args []string) []string {
	if len(args) == 0 {
		return args
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--fixture":
			if i+1 < len(args) {
				out = append(out, "--finding", args[i+1])
				i++
			} else {
				out = append(out, a)
			}
		case strings.HasPrefix(a, "--fixture="):
			out = append(out, "--finding="+strings.TrimPrefix(a, "--fixture="))
		default:
			out = append(out, a)
		}
	}
	return out
}
