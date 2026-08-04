package render_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/render"
)

const minimalConfig = `
apiVersion: assent.dev/v1alpha1
kind: Config
environments:
  - name: prod
    match: { paths: ["topics/prod/**"] }
  - name: dev
    match: { paths: ["**"] }
classes:
  - name: kafka-topic
    match: { paths: ["topics/**"] }
`

// REQ-E8-S02-02: missing presentation block yields D-089 defaults.
func TestDefaultPresentationOptions(t *testing.T) {
	cfg, err := policy.LoadConfig([]byte(minimalConfig))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	got := render.OptionsForEnvironment(cfg, "prod")
	want := render.Options{
		Verbosity:         render.VerbosityStandard,
		Emoji:             true,
		CollapseThreshold: render.DefaultCollapseThreshold,
		Locale:            render.DefaultLocale,
	}
	if got != want {
		t.Fatalf("OptionsForEnvironment() = %+v, want %+v", got, want)
	}
}

// REQ-E8-S02-03: per-environment override wins over global for matched env name.
func TestPresentationEnvOverride(t *testing.T) {
	const withPresentation = minimalConfig + `
presentation:
  verbosity: full
  emoji: false
  collapseThreshold: 10
  locale: en
  environments:
    - name: prod
      verbosity: minimal
      emoji: true
      collapseThreshold: 3
    - name: dev
      locale: sv
`
	cfg, err := policy.LoadConfig([]byte(withPresentation))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	prod := render.OptionsForEnvironment(cfg, "prod")
	wantProd := render.Options{
		Verbosity:         render.VerbosityMinimal,
		Emoji:             true,
		CollapseThreshold: 3,
		Locale:            render.DefaultLocale,
	}
	if prod != wantProd {
		t.Fatalf("prod OptionsForEnvironment() = %+v, want %+v", prod, wantProd)
	}

	dev := render.OptionsForEnvironment(cfg, "dev")
	wantDev := render.Options{
		Verbosity:         render.VerbosityFull,
		Emoji:             false,
		CollapseThreshold: 10,
		Locale:            "sv",
	}
	if dev != wantDev {
		t.Fatalf("dev OptionsForEnvironment() = %+v, want %+v", dev, wantDev)
	}

	unknown := render.OptionsForEnvironment(cfg, "staging")
	wantGlobal := render.Options{
		Verbosity:         render.VerbosityFull,
		Emoji:             false,
		CollapseThreshold: 10,
		Locale:            render.DefaultLocale,
	}
	if unknown != wantGlobal {
		t.Fatalf("unknown env OptionsForEnvironment() = %+v, want global %+v", unknown, wantGlobal)
	}
}
