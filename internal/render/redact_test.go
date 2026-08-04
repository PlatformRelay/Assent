package render_test

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/render"
)

func TestSensitiveFactRedacted(t *testing.T) {
	secret := "super-secret-token-abc123"
	facts := map[string]map[string]aggregate.Fact{
		"secrets": {
			"apiKey": {
				State:     "resolved",
				Sensitive: true,
				Value:     secret,
			},
		},
		"quota": {
			"max_partitions": {
				State:     "resolved",
				Sensitive: false,
				Value:     int64(8),
			},
		},
	}

	display := render.RedactFacts(facts)
	if got := display["secrets"]["apiKey"]; got != render.RedactedDisplay {
		t.Fatalf("RedactFacts secrets.apiKey = %q, want %q", got, render.RedactedDisplay)
	}

	section := render.RenderFactsSection(facts, render.Options{Locale: "en"})
	if strings.Contains(section, secret) {
		t.Fatalf("RenderFactsSection leaked sensitive value %q:\n%s", secret, section)
	}
	if !strings.Contains(section, "redacted") {
		t.Fatalf("RenderFactsSection missing redacted placeholder:\n%s", section)
	}
	if got := render.FormatFactValue(facts["secrets"]["apiKey"]); !strings.Contains(got, "redacted") {
		t.Fatalf("FormatFactValue sensitive = %q, want escaped redacted placeholder", got)
	}
	if strings.Contains(render.FormatFactValue(facts["secrets"]["apiKey"]), secret) {
		t.Fatal("FormatFactValue leaked sensitive value")
	}
}

func TestNonSensitiveFactsVisible(t *testing.T) {
	visible := "platform-team"
	facts := map[string]map[string]aggregate.Fact{
		"author": {
			"groups": {
				State:     "resolved",
				Sensitive: false,
				Value:     []any{visible},
			},
		},
		"secrets": {
			"apiKey": {
				State:     "resolved",
				Sensitive: true,
				Value:     "must-not-appear",
			},
		},
	}

	display := render.RedactFacts(facts)
	if got := display["author"]["groups"]; !strings.Contains(got, visible) {
		t.Fatalf("RedactFacts author.groups = %q, want visible %q", got, visible)
	}
	if strings.Contains(display["author"]["groups"], render.RedactedDisplay) {
		t.Fatalf("non-sensitive fact must not be redacted: %q", display["author"]["groups"])
	}

	section := render.RenderFactsSection(facts, render.Options{Locale: "en"})
	if !strings.Contains(section, visible) {
		t.Fatalf("RenderFactsSection missing non-sensitive value %q:\n%s", visible, section)
	}
	if strings.Contains(section, "must-not-appear") {
		t.Fatalf("RenderFactsSection leaked sensitive value:\n%s", section)
	}
	if got := render.FormatFactValue(facts["author"]["groups"]); !strings.Contains(got, visible) {
		t.Fatalf("FormatFactValue non-sensitive = %q, want %q visible", got, visible)
	}
}
