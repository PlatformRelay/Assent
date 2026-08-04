package schemadrift_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/schemadrift"
)

func TestCheckGitFrozenOrD088PresentationOnly(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	if err := schemadrift.CheckGitFrozenOrD088PresentationOnly(repoRoot); err != nil {
		diffCmd := exec.Command("git", "diff", "origin/main", "--name-only", "--", "schemas/")
		diffCmd.Dir = repoRoot
		diff, _ := diffCmd.Output()
		t.Fatalf("schema drift check: %v\nchanged files vs origin/main:\n%s", err, diff)
	}
}

func TestAllowedD088ConfigSchemaChange(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	baselinePath := filepath.Join(repoRoot, "schemas/policy/v1alpha1/config.schema.json")
	baseline, err := os.ReadFile(baselinePath) //nolint:gosec // fixed schema path under repoRoot
	if err != nil {
		t.Fatalf("read baseline schema: %v", err)
	}
	originCmd := exec.Command("git", "show", "origin/main:schemas/policy/v1alpha1/config.schema.json")
	originCmd.Dir = repoRoot
	originMain, err := originCmd.Output()
	if err != nil {
		t.Fatalf("read origin/main schema: %v", err)
	}

	t.Run("presentation_only_add_passes", func(t *testing.T) {
		if err := schemadrift.AllowedD088ConfigSchemaChange(originMain, baseline); err != nil {
			t.Fatalf("expected lane presentation block to pass structural check, got: %v", err)
		}
	})

	t.Run("identical_passes", func(t *testing.T) {
		if err := schemadrift.AllowedD088ConfigSchemaChange(originMain, originMain); err != nil {
			t.Fatalf("identical baseline must pass, got: %v", err)
		}
	})

	t.Run("co_mingled_non_presentation_hunk_fails", func(t *testing.T) {
		tampered := string(originMain)
		tampered = strings.Replace(tampered,
			`"minItems": 1,
      "items": { "$ref": "#/$defs/namedMatch" },
      "x-uniqueKeys": ["name"],
      "description": "Environment classifier`,
			`"minItems": 2,
      "items": { "$ref": "#/$defs/namedMatch" },
      "x-uniqueKeys": ["name"],
      "description": "Environment classifier`,
			1)
		tampered = strings.Replace(tampered,
			`"providers": {
      "type": "object",
      "additionalProperties": { "$ref": "#/$defs/provider" },
      "description": "Keyed by provider name (surfaced to rules as facts.<name>.*, ADR-0004)."
    }
  },
  "$defs": {`,
			`"providers": {
      "type": "object",
      "additionalProperties": { "$ref": "#/$defs/provider" },
      "description": "Keyed by provider name (surfaced to rules as facts.<name>.*, ADR-0004)."
    },
    "presentation": {
      "$ref": "#/$defs/presentation",
      "description": "presentation knobs"
    }
  },
  "$defs": {
    "presentation": { "type": "object", "additionalProperties": false, "properties": {} },`,
			1)
		if err := schemadrift.AllowedD088ConfigSchemaChange([]byte(originMain), []byte(tampered)); err == nil {
			t.Fatal("expected co-mingled environments minItems tweak + presentation add to fail")
		} else if !strings.Contains(err.Error(), "environments") && !strings.Contains(err.Error(), "properties") {
			t.Fatalf("expected located non-presentation error, got: %v", err)
		}
	})

	t.Run("presentation_substring_in_description_bypass_fails", func(t *testing.T) {
		tampered := strings.Replace(string(originMain),
			`"description": "Environment classifier; last match wins as the documented default (ADR-0010)."`,
			`"description": "Environment classifier; presentation detail; last match wins as the documented default (ADR-0010)."`,
			1)
		if err := schemadrift.AllowedD088ConfigSchemaChange([]byte(originMain), []byte(tampered)); err == nil {
			t.Fatal("expected unrelated description change to fail closed")
		}
	})

	t.Run("added_non_presentation_property_fails", func(t *testing.T) {
		tampered := strings.Replace(string(originMain),
			`"presentation": {
      "$ref": "#/$defs/presentation",
      "description": "Tier-0 renderer knobs (ADR-0016 §1, D-088): verbosity, emoji, collapse threshold, locale, and optional per-environment overrides."
    }
  },`,
			`"presentation": {
      "$ref": "#/$defs/presentation",
      "description": "Tier-0 renderer knobs (ADR-0016 §1, D-088): verbosity, emoji, collapse threshold, locale, and optional per-environment overrides."
    },
    "telemetry": { "type": "object" }
  },`,
			1)
		err := schemadrift.AllowedD088ConfigSchemaChange([]byte(originMain), []byte(tampered))
		if err == nil {
			t.Fatal("expected added non-presentation property to fail")
		}
		if !strings.Contains(err.Error(), "telemetry") {
			t.Fatalf("expected error to name telemetry, got: %v", err)
		}
	})

	t.Run("other_schema_file_change_fails", func(t *testing.T) {
		err := schemadrift.ValidateSchemaPathDrift([]string{
			"schemas/policy/v1alpha1/config.schema.json",
			"schemas/policy/v1alpha1/merge-policy.schema.json",
		})
		if err == nil {
			t.Fatal("expected non-config schema path to fail")
		}
		if !strings.Contains(err.Error(), "merge-policy.schema.json") {
			t.Fatalf("expected error to name merge-policy.schema.json, got: %v", err)
		}
	})

	t.Run("non_schema_json_paths_ignored", func(t *testing.T) {
		err := schemadrift.ValidateSchemaPathDrift([]string{
			"schemas/strictdecode_config_presentation_test.go",
			"schemas/testdata/compat/strict-decode/config/presentation-valid.json",
		})
		if err != nil {
			t.Fatalf("non-.schema.json paths under schemas/ must not trigger drift guard: %v", err)
		}
	})

	t.Run("invalid_json_fails", func(t *testing.T) {
		if err := schemadrift.AllowedD088ConfigSchemaChange([]byte(`{`), []byte(`{}`)); err == nil {
			t.Fatal("expected invalid baseline JSON to fail")
		}
	})

	t.Run("top_level_title_change_fails", func(t *testing.T) {
		tampered := strings.Replace(string(originMain), `"title": "Config"`, `"title": "Config presentation"`, 1)
		if err := schemadrift.AllowedD088ConfigSchemaChange(originMain, []byte(tampered)); err == nil {
			t.Fatal("expected top-level title change to fail")
		}
	})

	t.Run("non_object_properties_fails", func(t *testing.T) {
		err := schemadrift.AllowedD088ConfigSchemaChange(
			[]byte(`{"properties":{"apiVersion":{}}}`),
			[]byte(`{"properties":"not-an-object"}`),
		)
		if err == nil {
			t.Fatal("expected non-object properties to fail")
		}
	})

	t.Run("removed_top_level_key_fails", func(t *testing.T) {
		tampered := strings.Replace(string(originMain), `"title": "Config",`+"\n", "", 1)
		if err := schemadrift.AllowedD088ConfigSchemaChange(originMain, []byte(tampered)); err == nil {
			t.Fatal("expected removed top-level key to fail")
		}
	})

	t.Run("removed_non_presentation_property_fails", func(t *testing.T) {
		tampered := strings.Replace(string(originMain),
			`"providers": {
      "type": "object",
      "additionalProperties": { "$ref": "#/$defs/provider" },
      "description": "Keyed by provider name (surfaced to rules as facts.<name>.*, ADR-0004)."
    },
`,
			"",
			1)
		if err := schemadrift.AllowedD088ConfigSchemaChange(originMain, []byte(tampered)); err == nil {
			t.Fatal("expected removed providers property to fail")
		}
	})

	t.Run("added_top_level_key_fails", func(t *testing.T) {
		tampered := strings.Replace(string(originMain), `"title": "Config",`, `"title": "Config",`+"\n"+`  "telemetryVersion": 1,`, 1)
		if err := schemadrift.AllowedD088ConfigSchemaChange(originMain, []byte(tampered)); err == nil {
			t.Fatal("expected added top-level key to fail")
		}
	})

	t.Run("changed_non_presentation_property_fails", func(t *testing.T) {
		tampered := strings.Replace(string(originMain), `"const": "Config"`, `"const": "ConfigV2"`, 1)
		if err := schemadrift.AllowedD088ConfigSchemaChange(originMain, []byte(tampered)); err == nil {
			t.Fatal("expected changed kind const to fail")
		}
	})

	t.Run("added_non_presentation_def_fails", func(t *testing.T) {
		tampered := strings.Replace(string(originMain),
			`"$defs": {
    "profileRef": {`,
			`"$defs": {
    "telemetry": { "type": "object" },
    "profileRef": {`,
			1)
		if err := schemadrift.AllowedD088ConfigSchemaChange(originMain, []byte(tampered)); err == nil {
			t.Fatal("expected added non-presentation $defs entry to fail")
		}
	})
}
