package schemas

import (
	"strings"
	"testing"
)

// TestStrictDecodeConfig proves REQ-E8-S02-01: unknown presentation enum values
// reject at schema validation (strict decode), while a clean presentation block
// still accepts.
func TestStrictDecodeConfig(t *testing.T) {
	t.Run("unknown_presentation_enum", func(t *testing.T) {
		doc := readFixture(t, "config/unknown-presentation-enum.json")
		err := validateJSON(ConfigSchema, doc)
		if err == nil {
			t.Fatal("expected unknown presentation verbosity enum to be rejected")
		}
		if !strings.Contains(err.Error(), "verbosity") {
			t.Fatalf("expected error to name verbosity, got: %v", err)
		}
	})

	t.Run("presentation_valid", func(t *testing.T) {
		doc := readFixture(t, "config/presentation-valid.json")
		if err := validateJSON(ConfigSchema, doc); err != nil {
			t.Fatalf("expected presentation fixture to decode cleanly, got: %v", err)
		}
	})
}
