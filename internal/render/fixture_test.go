package render_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/render"
)

func TestLoadPresentationFixture(t *testing.T) {
	t.Run("invalid JSON", func(t *testing.T) {
		if _, err := render.LoadPresentationModel([]byte(`{`)); err == nil {
			t.Fatal("expected parse error")
		}
	})

	t.Run("missing required decision", func(t *testing.T) {
		raw := []byte(`{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PresentationModel",
			"findings": []
		}`)
		if _, err := render.LoadPresentationModel(raw); err == nil {
			t.Fatal("expected error for missing decision")
		}
	})

	t.Run("invalid decision enum", func(t *testing.T) {
		raw := []byte(`{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PresentationModel",
			"decision": "MAYBE",
			"findings": []
		}`)
		if _, err := render.LoadPresentationModel(raw); err == nil {
			t.Fatal("expected error for invalid decision enum")
		} else if !strings.Contains(err.Error(), "presentation-model") {
			t.Fatalf("expected located presentation-model error, got: %v", err)
		}
	})

	t.Run("missing required findings", func(t *testing.T) {
		raw := []byte(`{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PresentationModel",
			"decision": "APPROVE"
		}`)
		if _, err := render.LoadPresentationModel(raw); err == nil {
			t.Fatal("expected error for missing findings")
		}
	})

	t.Run("valid minimal fixture bytes", func(t *testing.T) {
		raw := []byte(`{
			"apiVersion": "assent.dev/v1alpha1",
			"kind": "PresentationModel",
			"decision": "APPROVE",
			"findings": []
		}`)
		fx, err := render.LoadPresentationModel(raw)
		if err != nil {
			t.Fatalf("LoadPresentationModel: %v", err)
		}
		if fx.Presentation.Decision != "APPROVE" {
			t.Fatalf("decision = %q, want APPROVE", fx.Presentation.Decision)
		}
	})

	t.Run("examples/render case path", func(t *testing.T) {
		path := filepath.Join("..", "..", "examples", "render", "minimal", "presentation-model.json")
		raw, err := os.ReadFile(path) //nolint:gosec // explicit test fixture path
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		fx, err := render.LoadPresentationModel(raw)
		if err != nil {
			t.Fatalf("LoadPresentationModel: %v", err)
		}
		if fx.Presentation.Kind != "PresentationModel" {
			t.Fatalf("kind = %q, want PresentationModel", fx.Presentation.Kind)
		}
	})
}

func TestLoadD016Fixture(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "contracts", "d016-strict-fixture", "presentation-model.json")
	raw, err := os.ReadFile(path) //nolint:gosec // explicit test fixture path
	if err != nil {
		t.Fatalf("read d016 fixture: %v", err)
	}
	fx, err := render.LoadPresentationModel(raw)
	if err != nil {
		t.Fatalf("LoadPresentationModel: %v", err)
	}
	if fx.Presentation.Decision != "BLOCK" {
		t.Fatalf("decision = %q, want BLOCK", fx.Presentation.Decision)
	}
	if len(fx.Presentation.Findings) != 3 {
		t.Fatalf("findings len = %d, want 3", len(fx.Presentation.Findings))
	}
}

func TestRenderFindingThreadStub(t *testing.T) {
	_, err := render.RenderFindingThread(render.Fixture{}, render.Context{})
	if err == nil {
		t.Fatal("expected stub error before E8-S08")
	}
}
