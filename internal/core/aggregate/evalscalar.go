package aggregate

// evalscalar.go evaluates a single CEL expression against a predicate-scope
// activation for message interpolation (E8-S07). It reuses newEvalEnv — the same
// eleven-field environment as evalLeaf and CompileCheck — so assert leaves and
// {{ }} messages share one type system (D-095 / ADR-0016 §2).

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// EvalScalar compiles and evaluates expr against the frozen predicate-scope env
// and activation. It returns the native Go value on success. Compile failures,
// eval errors, cost overruns, and non-scalar error values propagate as errors —
// never a silent "<no value>".
func EvalScalar(expr string, activation any) (any, error) {
	env, err := newEvalEnv()
	if err != nil {
		return nil, fmt.Errorf("build predicate-scope env: %w", err)
	}
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("compile %q: %w", expr, iss.Err())
	}
	prg, err := env.Program(ast, cel.CostLimit(celCostBudget))
	if err != nil {
		return nil, fmt.Errorf("program %q: %w", expr, err)
	}
	out, _, evalErr := prg.Eval(activation)
	if evalErr != nil {
		return nil, fmt.Errorf("eval %q: %w", expr, evalErr)
	}
	if out == nil || types.IsError(out) {
		return nil, fmt.Errorf("eval %q produced an error value", expr)
	}
	return out.Value(), nil
}
