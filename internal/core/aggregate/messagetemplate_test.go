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
