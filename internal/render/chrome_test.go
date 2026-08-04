package render_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/render"
	"github.com/PlatformRelay/assent/internal/render/locale"
)

func TestChromeLookup(t *testing.T) {
	opts := render.DefaultOptions()
	got, warns := render.Chrome(opts, "resolve_thread")
	if len(warns) != 0 {
		t.Fatalf("en locale: unexpected warnings: %+v", warns)
	}
	if got == "" {
		t.Fatal("expected non-empty resolve_thread chrome")
	}
}

func TestChromeUnknownLocale(t *testing.T) {
	opts := render.DefaultOptions()
	opts.Locale = "de"
	got, warns := render.Chrome(opts, "resolve_thread")
	if len(warns) != 1 || warns[0].Code != locale.CodeUnknownLocale {
		t.Fatalf("expected unknown-locale warning, got %+v", warns)
	}
	if got == "" {
		t.Fatal("expected fallback string")
	}
}

func TestCatalogFor(t *testing.T) {
	cat, warns := render.CatalogFor(render.DefaultOptions())
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %+v", warns)
	}
	if cat.Locale() != locale.DefaultLocale {
		t.Fatalf("locale = %q, want %q", cat.Locale(), locale.DefaultLocale)
	}
}

func TestChromeMissingID(t *testing.T) {
	got, warns := render.Chrome(render.DefaultOptions(), "not-a-real-id")
	if got != "" {
		t.Fatalf("missing id should return empty, got %q", got)
	}
	if len(warns) != 0 {
		t.Fatalf("missing id should not add warnings: %+v", warns)
	}
}
