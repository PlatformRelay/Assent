package aggregate_test

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

func TestParseMessageSlots(t *testing.T) {
	slots := aggregate.ParseMessageSlots("a {{ old }} b {{ new }}")
	if len(slots) != 2 {
		t.Fatalf("got %d slots, want 2", len(slots))
	}
	if slots[0].Expr != "old" || slots[1].Expr != "new" {
		t.Fatalf("unexpected slots: %+v", slots)
	}
}

func TestCompileMessageTemplateUnknownField(t *testing.T) {
	err := aggregate.CompileMessageTemplate("bad {{ unknown_field }}")
	if err == nil {
		t.Fatal("expected compile error")
	}
	if !strings.Contains(err.Error(), "message template:") {
		t.Fatalf("want located error, got %v", err)
	}
}

func TestEvalScalarOldNew(t *testing.T) {
	act := map[string]any{"old": int64(3), "new": int64(7)}
	v, err := aggregate.EvalScalar("new", act)
	if err != nil {
		t.Fatalf("EvalScalar: %v", err)
	}
	if v != int64(7) {
		t.Fatalf("got %v, want 7", v)
	}
}

func TestFactRefFromExpr(t *testing.T) {
	p, n, ok := aggregate.FactRefFromExpr("facts.quota.apiKey.value")
	if !ok || p != "quota" || n != "apiKey" {
		t.Fatalf("FactRefFromExpr = (%q, %q, %v)", p, n, ok)
	}
	if _, _, ok := aggregate.FactRefFromExpr("new"); ok {
		t.Fatal("non-facts expr must not match")
	}
}

func TestSensitiveFactAt(t *testing.T) {
	act := map[string]any{
		"facts": map[string]any{
			"x": map[string]any{
				"secret": map[string]any{"sensitive": true},
			},
		},
	}
	if !aggregate.SensitiveFactAt(act, "x", "secret") {
		t.Fatal("expected sensitive fact")
	}
	if aggregate.SensitiveFactAt(act, "x", "missing") {
		t.Fatal("missing fact must not be sensitive")
	}
}

func TestReplaceMessageSlots(t *testing.T) {
	got, err := aggregate.ReplaceMessageSlots("plain", func(expr string) (string, error) {
		t.Fatalf("replacer must not run: %q", expr)
		return "", nil
	})
	if err != nil || got != "plain" {
		t.Fatalf("plain template = %q, %v", got, err)
	}

	got, err = aggregate.ReplaceMessageSlots("x {{ old }} y", func(expr string) (string, error) {
		if expr != "old" {
			t.Fatalf("expr = %q", expr)
		}
		return "5", nil
	})
	if err != nil || got != "x 5 y" {
		t.Fatalf("replaced = %q, %v", got, err)
	}
}

func TestEvalScalarFactsValue(t *testing.T) {
	act := map[string]any{
		"facts": map[string]any{
			"quota": map[string]any{
				"max_partitions": map[string]any{
					"state": "resolved",
					"value": int64(24),
				},
			},
		},
	}
	v, err := aggregate.EvalScalar("facts.quota.max_partitions.value", act)
	if err != nil {
		t.Fatalf("EvalScalar: %v", err)
	}
	if v != int64(24) {
		t.Fatalf("got %v, want 24", v)
	}
}
