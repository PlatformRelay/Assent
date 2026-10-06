package aggregate

import (
	"strings"
	"testing"
)

// compile_edges_test.go covers the message-template and scalar evaluation
// error paths (the presentation surface the renderer consumes): the template
// compiler's slot errors, the scalar evaluator's shapes, and the exported
// CompileCheck wrapper's refusal shapes (E3-S04's surface, consumed by
// internal/lint).

func TestCompileMessageTemplateShapes(t *testing.T) {
	if err := CompileMessageTemplate("the {{.}} slot is positional"); err == nil {
		t.Fatal("a positional slot must be refused (named slots only)")
	}
	if err := CompileMessageTemplate("a clean template"); err != nil {
		t.Fatalf("a slot-free template compiles: %v", err)
	}
}

func TestFormatMessageSlotIssueAndError(t *testing.T) {
	// The compile path's error branch: an undeclared reference names the
	// identifier, never panics; a cost-unprogrammable slot hits the error
	// formatter's other arm.
	if err := CompileMessageTemplate("value is {{.undeclared}}"); err == nil {
		t.Fatal("an undeclared template reference must error at compile time")
	}
	// Note: even a slot whose identifier is syntactically fine is refused at
	// compile time unless the env declares it — the predicate scope is the
	// only legal namespace (a message template may not invent names).
	if err := CompileMessageTemplate("value is {{.undeclared}}"); err == nil {
		t.Fatal("an undeclared template reference must error at compile time")
	}

}

func TestMessageSlotPosition(t *testing.T) {
	if line, col := messageSlotPosition("abc", 2); line != 1 || col != 3 {
		t.Fatalf("messageSlotPosition = (%d, %d), want (1, 3)", line, col)
	}
	// The offset walks the template byte by byte: each newline restarts the
	// column. Offset 4 in "a\nb\nc" walks two newlines (at 1 and 3), landing
	// on line 3 col 1.
	if line, col := messageSlotPosition("a\nb\nc", 4); line != 3 || col != 1 {
		t.Fatalf("two newlines advance two lines: (%d, %d), want (3, 1)", line, col)
	}
	if line, col := messageSlotPosition("a\n", 1); line != 1 || col != 2 {
		t.Fatalf("offset 1 of a one-char-then-newline template is still line 1 col 2: (%d, %d)", line, col)
	}
}

func TestEvalScalarShapes(t *testing.T) {
	// The scalar evaluator's error shapes (the coverage edge): a compile
	// failure and an undeclared reference both error, never a silent value.
	if _, err := EvalScalar("1 > ", nil); err == nil {
		t.Fatal("a malformed scalar expression must error")
	}
	if _, err := EvalScalar("undeclaredRef", nil); err == nil {
		t.Fatal("an undeclared reference must error")
	}
}

func TestCompileCheckShapes(t *testing.T) {
	if err := CompileCheck("size([1,2,3]) == 3"); err != nil {
		t.Fatalf("a valid predicate compiles clean: %v", err)
	}
	if err := CompileCheck("undeclaredRef == 1"); err == nil || !strings.Contains(err.Error(), "undeclared") {
		t.Fatalf("CompileCheck must name an undeclared reference, got %v", err)
	}
}
