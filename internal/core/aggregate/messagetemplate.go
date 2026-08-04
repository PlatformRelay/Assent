package aggregate

// messagetemplate.go parses and compile-checks {{ }} message templates against the
// shared predicate-scope CEL env (E8-S07 / E3-S04). Runtime interpolation with
// redaction and escaping lives in internal/render.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/cel-go/cel"
)

var messageSlotRE = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// MessageSlotRE is the shared {{ expr }} pattern for compile-check and runtime
// substitution (E8-S07). One regex so parse/compile/render never drift.
var MessageSlotRE = messageSlotRE

// ReplaceMessageSlots substitutes each {{ expr }} slot. replacer receives the
// trimmed CEL expression; the first error aborts.
func ReplaceMessageSlots(tmpl string, replacer func(expr string) (string, error)) (string, error) {
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}
	var evalErr error
	out := messageSlotRE.ReplaceAllStringFunc(tmpl, func(token string) string {
		if evalErr != nil {
			return token
		}
		sub := messageSlotRE.FindStringSubmatch(token)
		expr := ""
		if len(sub) >= 2 {
			expr = strings.TrimSpace(sub[1])
		}
		if expr == "" {
			return token
		}
		replacement, err := replacer(expr)
		if err != nil {
			evalErr = err
			return token
		}
		return replacement
	})
	if evalErr != nil {
		return "", evalErr
	}
	return out, nil
}

// FactRefFromExpr returns provider and fact name when expr selects under facts.*.
func FactRefFromExpr(expr string) (provider, name string, ok bool) {
	if !strings.HasPrefix(expr, "facts.") {
		return "", "", false
	}
	rest := strings.TrimPrefix(expr, "facts.")
	parts := strings.Split(rest, ".")
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// SensitiveFactAt reports whether activation binds a sensitive fact at provider/name.
func SensitiveFactAt(activation any, provider, name string) bool {
	act, ok := activation.(map[string]any)
	if !ok {
		return false
	}
	facts, ok := act["facts"].(map[string]any)
	if !ok {
		return false
	}
	byName, ok := facts[provider].(map[string]any)
	if !ok {
		return false
	}
	envelope, ok := byName[name].(map[string]any)
	if !ok {
		return false
	}
	sensitive, _ := envelope["sensitive"].(bool)
	return sensitive
}

// MessageSlot is one {{ expr }} region in a message template.
type MessageSlot struct {
	Expr   string
	Offset int // byte offset of '{{' in the template
	Line   int
	Column int
}

// ParseMessageSlots extracts CEL slots from a message template.
func ParseMessageSlots(tmpl string) []MessageSlot {
	matches := messageSlotRE.FindAllStringSubmatchIndex(tmpl, -1)
	out := make([]MessageSlot, 0, len(matches))
	for _, m := range matches {
		expr := tmpl[m[2]:m[3]]
		offset := m[0]
		line, col := messageSlotPosition(tmpl, offset)
		out = append(out, MessageSlot{
			Expr:   strings.TrimSpace(expr),
			Offset: offset,
			Line:   line,
			Column: col,
		})
	}
	return out
}

func messageSlotPosition(s string, offset int) (line, col int) {
	line, col = 1, 1
	for i := 0; i < offset && i < len(s); i++ {
		if s[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// CompileMessageTemplate checks every {{ }} slot against newEvalEnv. Unknown
// top-level identifiers fail at compile time with a located error — never
// "<no value>" at render (ADR-0016 §2, D-095).
func CompileMessageTemplate(tmpl string) error {
	if !strings.Contains(tmpl, "{{") {
		return nil
	}
	env, err := newEvalEnv()
	if err != nil {
		return fmt.Errorf("build predicate-scope env: %w", err)
	}
	for _, slot := range ParseMessageSlots(tmpl) {
		if slot.Expr == "" {
			continue
		}
		if err := compileMessageSlot(env, slot); err != nil {
			return err
		}
	}
	return nil
}

func compileMessageSlot(env *cel.Env, slot MessageSlot) error {
	ast, iss := env.Compile(slot.Expr)
	if iss == nil || iss.Err() == nil {
		if _, err := env.Program(ast, cel.CostLimit(celCostBudget)); err != nil {
			return formatMessageSlotError(slot, err)
		}
		return nil
	}
	return formatMessageSlotIssue(slot, iss)
}

func formatMessageSlotIssue(slot MessageSlot, iss *cel.Issues) error {
	errs := iss.Errors()
	if len(errs) == 0 {
		return fmt.Errorf("message template:%d:%d: %w", slot.Line, slot.Column, iss.Err())
	}
	e := errs[0]
	line, col := slot.Line, slot.Column
	if e.Location != nil {
		if e.Location.Line() == 1 {
			col = slot.Column + e.Location.Column()
		} else {
			line = slot.Line + e.Location.Line() - 1
			col = e.Location.Column()
		}
	}
	return fmt.Errorf("message template:%d:%d: %s", line, col, e.Message)
}

func formatMessageSlotError(slot MessageSlot, err error) error {
	return fmt.Errorf("message template:%d:%d: %w", slot.Line, slot.Column, err)
}
