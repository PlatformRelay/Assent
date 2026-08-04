package render_test

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/render"
)

// celMessageActivation is a fixed predicate-scope activation for message interpolation tests.
func celMessageActivation() map[string]any {
	return map[string]any{
		"old":  int64(5),
		"new":  int64(10),
		"path": "/partitions",
		"kind": "modify",
		"file": "topics/prod/a.yaml",
		"env":  "prod",
		"facts": map[string]any{
			"quota": map[string]any{
				"max_partitions": map[string]any{
					"state":     "resolved",
					"sensitive": false,
					"value":     int64(24),
				},
			},
		},
		"changes": []any{},
		"mr":      map[string]any{"author": "alice"},
	}
}

// REQ-E8-S07-01: {{ old }}, {{ new }}, {{ facts.quota.max_partitions }} on fixed activation.
func TestCELMessageInterpolation(t *testing.T) {
	ctx := render.Context{Activation: celMessageActivation()}

	got, err := render.EvalMessage(
		"partitions {{ old }} -> {{ new }}, quota {{ facts.quota.max_partitions }}",
		ctx,
	)
	if err != nil {
		t.Fatalf("EvalMessage: %v", err)
	}
	want := "partitions 5 -> 10, quota 24"
	if got != want {
		t.Fatalf("EvalMessage = %q, want %q", got, want)
	}
}

// REQ-E8-S07-02: unknown field fails at compile with located error.
func TestCELMessageCompileError(t *testing.T) {
	ctx := render.Context{Activation: celMessageActivation()}

	_, err := render.EvalMessage("quota {{ unknown_field }} exceeded", ctx)
	if err == nil {
		t.Fatal("expected compile error for unknown_field")
	}
	msg := err.Error()
	if strings.Contains(msg, "<no value>") {
		t.Fatalf("must never surface <no value>: %s", msg)
	}
	if !strings.Contains(msg, "message template:") {
		t.Fatalf("want positioned message template error, got %s", msg)
	}
}

func TestCELMessageSensitiveFactRedacted(t *testing.T) {
	secret := "super-secret-token"
	act := celMessageActivation()
	facts := act["facts"].(map[string]any)
	quota := facts["quota"].(map[string]any)
	quota["apiKey"] = map[string]any{
		"state":     "resolved",
		"sensitive": true,
		"value":     secret,
	}
	ctx := render.Context{Activation: act}

	got, err := render.EvalMessage("key={{ facts.quota.apiKey }}", ctx)
	if err != nil {
		t.Fatalf("EvalMessage: %v", err)
	}
	if strings.Contains(got, secret) {
		t.Fatalf("EvalMessage leaked sensitive value: %q", got)
	}
	if !strings.Contains(got, "redacted") {
		t.Fatalf("EvalMessage = %q, want redacted placeholder", got)
	}
}

func TestCELMessageSensitiveValueAccessorRedacted(t *testing.T) {
	secret := "super-secret-token-abc123"
	act := celMessageActivation()
	facts := act["facts"].(map[string]any)
	facts["x"] = map[string]any{
		"secret": map[string]any{
			"state":     "resolved",
			"sensitive": true,
			"value":     secret,
		},
	}
	ctx := render.Context{Activation: act}

	got, err := render.EvalMessage("token={{ facts.x.secret.value }}", ctx)
	if err != nil {
		t.Fatalf("EvalMessage: %v", err)
	}
	if strings.Contains(got, secret) {
		t.Fatalf("EvalMessage leaked sensitive .value accessor: %q", got)
	}
	if !strings.Contains(got, "redacted") {
		t.Fatalf("EvalMessage = %q, want redacted placeholder", got)
	}
}

func TestCELMessageEscapeAndClamp(t *testing.T) {
	act := celMessageActivation()
	act["new"] = `<script>alert(1)</script>`
	ctx := render.Context{Activation: act}

	got, err := render.EvalMessage("value={{ new }}", ctx)
	if err != nil {
		t.Fatalf("EvalMessage: %v", err)
	}
	if strings.Contains(got, "<script>") {
		t.Fatalf("EvalMessage must escape HTML: %q", got)
	}
}

func TestEvalMessageNoPlaceholder(t *testing.T) {
	ctx := render.Context{Activation: celMessageActivation()}
	got, err := render.EvalMessage("plain text", ctx)
	if err != nil || got != "plain text" {
		t.Fatalf("EvalMessage plain = %q, %v", got, err)
	}
}

func TestEvalMessageRuleMetaErrors(t *testing.T) {
	ctx := render.Context{Activation: celMessageActivation()}
	if _, err := render.EvalRuleMessage("missing", ctx); err == nil {
		t.Fatal("expected unknown rule error")
	}
	if _, err := render.EvalDocsSummary("missing", ctx); err == nil {
		t.Fatal("expected unknown rule error")
	}
	if _, err := render.EvalDebugLine("missing", 0, ctx); err == nil {
		t.Fatal("expected unknown rule error")
	}
	ctx.Rules = map[string]render.RuleMeta{"r": {Debug: []string{"x"}}}
	if _, err := render.EvalDebugLine("r", 1, ctx); err == nil {
		t.Fatal("expected debug index out of range")
	}
}

func TestEvalMessageEmptySlot(t *testing.T) {
	ctx := render.Context{Activation: celMessageActivation()}
	got, err := render.EvalMessage("before {{}} after", ctx)
	if err != nil {
		t.Fatalf("EvalMessage: %v", err)
	}
	if got != "before {{}} after" {
		t.Fatalf("empty slot left literal: %q", got)
	}
}

func TestEvalMessageFromRuleMeta(t *testing.T) {
	ctx := render.Context{
		Activation: celMessageActivation(),
		Rules: map[string]render.RuleMeta{
			"bounded": {
				Message: "limit {{ facts.quota.max_partitions }}",
				Docs:    render.RuleDocs{Summary: "max {{ new }}"},
				Debug:   []string{"old={{ old }}"},
			},
		},
	}

	msg, err := render.EvalRuleMessage("bounded", ctx)
	if err != nil {
		t.Fatalf("EvalRuleMessage: %v", err)
	}
	if msg != "limit 24" {
		t.Fatalf("EvalRuleMessage = %q, want %q", msg, "limit 24")
	}

	summary, err := render.EvalDocsSummary("bounded", ctx)
	if err != nil {
		t.Fatalf("EvalDocsSummary: %v", err)
	}
	if summary != "max 10" {
		t.Fatalf("EvalDocsSummary = %q, want %q", summary, "max 10")
	}

	debug, err := render.EvalDebugLine("bounded", 0, ctx)
	if err != nil {
		t.Fatalf("EvalDebugLine: %v", err)
	}
	if debug != "old=5" {
		t.Fatalf("EvalDebugLine = %q, want %q", debug, "old=5")
	}
}

func TestEvalMessageUsesAggregateEnv(t *testing.T) {
	// Compile-time: undeclared top-level identifier matches aggregate.CompileCheck.
	if err := aggregate.CompileCheck("input.new"); err == nil {
		t.Fatal("CompileCheck should reject input.new")
	}
	ctx := render.Context{Activation: celMessageActivation()}
	if _, err := render.EvalMessage("bad {{ input.new }}", ctx); err == nil {
		t.Fatal("EvalMessage should reject undeclared input at compile time")
	}
}
