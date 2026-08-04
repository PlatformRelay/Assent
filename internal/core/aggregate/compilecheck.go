package aggregate

// compilecheck.go is the E3-S04 exported compile-only helper. It exists so the
// pure `assent lint` check library (internal/lint) can surface an undeclared
// predicate-scope identifier — a reference to anything outside the frozen eleven
// fields (docs/planning/predicate-scope.md) — as a HARD ERROR before any MR
// (ADR-0016 §2: unknown-field refs are load-time errors, never a silent
// `<no value>`), WITHOUT re-declaring the field list anywhere or adding cel-go to
// internal/core/policy (deliberately kept off cel-go).
//
// It is a THIN, ADDITIVE wrapper over the SAME newEvalEnv() the E2-S02 evalLeaf
// primitive compiles against — ONE frozen-scope source, so the lint gate and the
// runtime engine can never drift. Compile (parse + CHECK against the declared env)
// is what turns an out-of-scope identifier into an error; a bare Parse (what the
// facts-reference lint uses) would not. cel-go's standard macros bind comprehension
// variables, so a bound `c` in `changes.exists(c, ...)` is NOT a free identifier
// and never mis-reports — only genuinely undeclared references error.
//
// It does NOT alter evalLeaf/newEvalEnv/bindLeafActivation; it only reuses them.
// Purity: cel-go compile is pure/deterministic; no clock/rand/env/net.

import (
	"fmt"

	"github.com/google/cel-go/cel"
)

// CompileCheck compiles expr against the frozen eleven-field predicate-scope env
// (newEvalEnv — the single shared source) and returns the compile error, or nil
// when expr type-checks clean. An out-of-scope top-level identifier surfaces in
// the returned error as cel-go's `undeclared reference to '<name>'`. The resulting
// program is budgeted with celCostBudget so a leaf lint accepts is exactly one
// evalLeaf can build and evaluate; the undeclared-reference detection itself lives
// entirely in Compile. It is compile-ONLY: it never evaluates, binds no activation,
// and has no side effects.
func CompileCheck(expr string) error {
	env, err := newEvalEnv()
	if err != nil {
		return fmt.Errorf("build predicate-scope env: %w", err)
	}
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return fmt.Errorf("compile %q: %w", expr, iss.Err())
	}
	if _, err := env.Program(ast, cel.CostLimit(celCostBudget)); err != nil {
		return fmt.Errorf("program %q: %w", expr, err)
	}
	return nil
}
