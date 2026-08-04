package render

import (
	"fmt"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// EvalMessage renders a message template whose {{ }} slots are CEL expressions
// over ctx.Activation using the same predicate-scope env as aggregate evaluation
// (D-095). Unknown fields fail at compile time; interpolated scalars are
// redacted (when a fact envelope is sensitive) then EscapeAndClamp'd (D-090/D-091).
func EvalMessage(tmpl string, ctx Context) (string, error) {
	if err := aggregate.CompileMessageTemplate(tmpl); err != nil {
		return "", err
	}
	return evalMessageTemplate(tmpl, ctx.Activation)
}

// EvalRuleMessage renders the pack rule message for ruleName from ctx.Rules.
func EvalRuleMessage(ruleName string, ctx Context) (string, error) {
	meta, ok := ctx.Rules[ruleName]
	if !ok {
		return "", fmt.Errorf("render: unknown rule %q", ruleName)
	}
	return EvalMessage(meta.Message, ctx)
}

// EvalDocsSummary renders docs.summary for ruleName from ctx.Rules.
func EvalDocsSummary(ruleName string, ctx Context) (string, error) {
	meta, ok := ctx.Rules[ruleName]
	if !ok {
		return "", fmt.Errorf("render: unknown rule %q", ruleName)
	}
	return EvalMessage(meta.Docs.Summary, ctx)
}

// EvalDebugLine renders one debug: line for ruleName from ctx.Rules.
func EvalDebugLine(ruleName string, index int, ctx Context) (string, error) {
	meta, ok := ctx.Rules[ruleName]
	if !ok {
		return "", fmt.Errorf("render: unknown rule %q", ruleName)
	}
	if index < 0 || index >= len(meta.Debug) {
		return "", fmt.Errorf("render: rule %q debug index %d out of range", ruleName, index)
	}
	return EvalMessage(meta.Debug[index], ctx)
}

func evalMessageTemplate(tmpl string, activation any) (string, error) {
	return aggregate.ReplaceMessageSlots(tmpl, func(expr string) (string, error) {
		return evalMessageSlot(expr, activation)
	})
}

func evalMessageSlot(expr string, activation any) (string, error) {
	if provider, name, ok := aggregate.FactRefFromExpr(expr); ok {
		if aggregate.SensitiveFactAt(activation, provider, name) {
			return EscapeAndClamp(RedactedDisplay), nil
		}
	}
	v, err := aggregate.EvalScalar(expr, activation)
	if err != nil {
		return "", err
	}
	return EscapeAndClamp(formatMessageScalar(v)), nil
}

// formatMessageScalar stringifies a CEL result for forge-facing interpolation.
// A fact envelope map (state/sensitive/value keys) displays via the same redaction
// rules as RenderFactsSection; other scalars use %v formatting.
func formatMessageScalar(v any) string {
	if fm, ok := v.(map[string]any); ok {
		if _, hasState := fm["state"]; hasState {
			f := aggregate.Fact{State: fmt.Sprint(fm["state"])}
			if s, ok := fm["sensitive"].(bool); ok {
				f.Sensitive = s
			}
			if val, ok := fm["value"]; ok {
				f.Value = val
			}
			return displayFactValue(f)
		}
	}
	return fmt.Sprint(v)
}
