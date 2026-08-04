package locale_test

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/render/locale"
)

// defaultThemeChromeIDs is the contract pinned for E8-S08 (finding-thread layout) and
// E8-S13 (summary body). S08/S13 must look up only ids listed here.
var defaultThemeChromeIDs = []string{
	// Finding-thread layout (ADR-0012 default, E8-S08).
	"resolve_thread",
	"why_this_check",
	"full_documentation",
	"evaluation_details",
	"matched_change",
	"facts_used",
	"score_contribution",
	"collapsed_paths",
	// Summary comment layout (E8-S13).
	"summary_headline",
	"summary_decision",
	"summary_score",
	"summary_threshold",
	"summary_findings",
}

// REQ-E8-S03-01: every default-theme chrome id used in S08/S13 exists in the en catalog.
func TestEnCatalogComplete(t *testing.T) {
	cat, warns := locale.ForLocale(locale.DefaultLocale)
	if len(warns) != 0 {
		t.Fatalf("en locale must not warn: %+v", warns)
	}
	for _, id := range defaultThemeChromeIDs {
		got, ok := cat.Lookup(id)
		if !ok {
			t.Errorf("en catalog missing chrome id %q", id)
			continue
		}
		if strings.TrimSpace(got) == "" {
			t.Errorf("en catalog id %q is empty", id)
		}
	}
}

// REQ-E8-S03-02: unknown locale falls back to en with a lint warning (not a render panic).
func TestUnknownLocaleFallback(t *testing.T) {
	enCat, _ := locale.ForLocale(locale.DefaultLocale)
	gotCat, warns := locale.ForLocale("sv")
	if len(warns) != 1 {
		t.Fatalf("unknown locale must emit exactly one warning, got %d: %+v", len(warns), warns)
	}
	if warns[0].Code != locale.CodeUnknownLocale {
		t.Fatalf("warning code = %q, want %q", warns[0].Code, locale.CodeUnknownLocale)
	}
	if !strings.Contains(warns[0].Message, "sv") {
		t.Fatalf("warning must name requested locale, got %q", warns[0].Message)
	}
	for _, id := range defaultThemeChromeIDs {
		want, ok := enCat.Lookup(id)
		if !ok {
			t.Fatalf("en missing %q", id)
		}
		got, ok := gotCat.Lookup(id)
		if !ok {
			t.Fatalf("fallback catalog missing %q", id)
		}
		if got != want {
			t.Errorf("fallback %q = %q, want en %q", id, got, want)
		}
	}
	if gotCat.Locale() != locale.DefaultLocale {
		t.Fatalf("effective locale = %q, want %q", gotCat.Locale(), locale.DefaultLocale)
	}
}

func TestEmptyLocaleUsesEn(t *testing.T) {
	cat, warns := locale.ForLocale("")
	if len(warns) != 0 {
		t.Fatalf("empty locale must not warn: %+v", warns)
	}
	if cat.Locale() != locale.DefaultLocale {
		t.Fatalf("locale = %q, want %q", cat.Locale(), locale.DefaultLocale)
	}
}

func TestLookupMissingID(t *testing.T) {
	cat, _ := locale.ForLocale(locale.DefaultLocale)
	if _, ok := cat.Lookup("not-a-real-id"); ok {
		t.Fatal("expected missing id lookup to fail")
	}
}
