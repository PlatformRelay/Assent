package aggregate

import (
	"encoding/json"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/interpreter"
)

// celCostBudget bounds a single leaf evaluation (ADR-0013: predicates are
// cost-limited, non-Turing-complete). A predicate exceeding it errors -> the
// caller fails safe to REVIEW, never an unbounded evaluation.
const celCostBudget = 1_000_000

// newEvalEnv builds the E2-S02 cel-go environment binding EXACTLY the eleven
// frozen predicate-scope fields (docs/planning/predicate-scope.md) — no more, no
// less. old/new/entry/oldEntry are Dyn (typed change values: numeric compare on
// scalars, object navigation on trees); path/kind/file/env are strings; changes
// is the whole list; facts/mr are Dyn maps. It registers ZERO non-deterministic
// functions/macros (no time/now/rand), so evaluation is pure and deterministic.
// An `old`/`new` reference to a field not in this set (e.g. `input`) is an
// undeclared reference -> a COMPILE error, never a silent `<no value>` (ADR-0016).
func newEvalEnv() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("old", cel.DynType),
		cel.Variable("new", cel.DynType),
		cel.Variable("entry", cel.DynType),
		cel.Variable("oldEntry", cel.DynType),
		cel.Variable("path", cel.StringType),
		cel.Variable("kind", cel.StringType),
		cel.Variable("file", cel.StringType),
		cel.Variable("env", cel.StringType),
		cel.Variable("changes", cel.ListType(cel.DynType)),
		cel.Variable("facts", cel.DynType),
		cel.Variable("mr", cel.DynType),
	)
}

// evalLeaf compiles and evaluates one single-leaf CEL expression over the
// activation built for the handed change (E2-S02). It does NO matching/selection
// — the caller (a test here, E2-S04's coverage loop in production) hands it the
// change. It returns (satisfied, nil) ONLY when the expression compiled,
// evaluated under the cost budget without error, produced a boolean, AND ordered
// nothing lexically (textOrderGuard, D-129); every other outcome (undeclared
// reference, coercion/type error, cost overrun, non-boolean result, a relational
// compare over text) returns a non-nil error so the caller fails safe. It NEVER
// returns (true, nil) for a malformed or type-erroring predicate.
func evalLeaf(env *cel.Env, in EvaluationInput, ch EvalChange, envLabel, expr string) (bool, error) {
	checked, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return false, fmt.Errorf("compile when %q: %w", expr, iss.Err())
	}
	// The guard watches every operand of every relational operator AS IT IS
	// EVALUATED (D-129) — see newTextOrderGuard. It is per-program state, and this
	// program is built and used exactly once, here.
	guard := newTextOrderGuard(checked)
	prg, err := env.Program(checked, cel.CostLimit(celCostBudget), cel.CustomDecorator(guard.decorate))
	if err != nil {
		return false, fmt.Errorf("program when %q: %w", expr, err)
	}
	out, _, evalErr := prg.Eval(bindLeafActivation(in, ch, envLabel))
	if evalErr != nil {
		return false, fmt.Errorf("eval when %q: %w", expr, evalErr)
	}
	if out == nil || types.IsError(out) {
		return false, fmt.Errorf("eval when %q produced an error value", expr)
	}
	// BEFORE the boolean is trusted: if anything was ordered lexically the answer
	// is unsound in BOTH directions (lexically "6" >= "12" is true and "12" >= "6"
	// is false). Fail safe on either.
	if err := guard.err(); err != nil {
		return false, fmt.Errorf("eval when %q: %w", expr, err)
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("when %q result is %s, not bool", expr, out.Type().TypeName())
	}
	return b, nil
}

// textOrderGuard is the D-129 fail-safe guard. CEL defines < <= > >= over strings
// as a LEXICAL (character-by-character) compare, so an ordering rule such as the
// D-016 `new >= old` silently answers a BOOLEAN when its operands bind as text
// instead of numbers — and lexically "6" >= "12" is TRUE, which APPROVEs a
// partition shrink. Text binds routinely: internal/change tag-discriminates a
// YAML !!str, so `partitions: "12"` decodes to the Go string "12" BY DESIGN
// (evaldecode.DecodeCanonical) — a quoted numeric is not the number 12.
//
// The guard is VALUE-based, not syntax-based, because whether an operand is text
// is a property of the adopter's DATA, not of the policy text: `new >= old` is
// correct over numbers and unsound over strings, and no static check of the leaf
// can tell the two apart. (That is why this defect is not lintable, D-129: lint
// sees the rule, never the change it will judge.) It plants a watcher
// on each operand of each relational operator and inspects what that operand
// ACTUALLY produced, every time it was evaluated:
//
//   - a relational compare that short-circuited away never runs its watcher, so
//     an existing policy cannot flip to REVIEW for a compare that did not happen;
//   - EVERY iteration of a comprehension body is seen (watching beats reading the
//     post-eval EvalState, which keeps only the last value per node id);
//   - ANY text operand is refused, in either position and for either answer — a
//     wrong `false` (a legitimate grow judged destructive) is as unacceptable as
//     a wrong `true`. A mixed text/number relational already errors inside cel-go
//     (no such overload), so it never reaches here.
//
// Deliberate ordering stays expressible by coercing first — `int(new) >= int(old)`
// (already the repo's idiom), `double(...)`, or `timestamp(a) < timestamp(b)` for
// ISO-8601 dates. Ordering raw text is NOT expressible in tier-1 `assert` and
// graduates to Rego (ADR-0013 Amendment 1).
//
// Purity/determinism: watching only reads values that were computed anyway; it
// adds no clock, randomness or I/O, and the recorded hit is the FIRST in
// evaluation order, so the error is identical on every replay of the same input.
type textOrderGuard struct {
	// operands maps a relational operand's AST node id to the operator symbol it
	// feeds, e.g. `new`'s id in `new >= old` -> ">=".
	operands map[int64]string
	hitOp    string
	hitValue string
	hit      bool
}

// relationalOperators are the four CEL operators whose (string, string) overload
// compares LEXICALLY — the only standard operators that silently turn a text
// value into an ordering answer. Equality (== / !=), membership (in) and the
// string member functions are exact, not ordering, and are deliberately excluded.
var relationalOperators = map[string]string{
	operators.Less:          "<",
	operators.LessEquals:    "<=",
	operators.Greater:       ">",
	operators.GreaterEquals: ">=",
}

// newTextOrderGuard collects the operand node ids of every relational call in the
// checked expression. A guard with no operands decorates nothing.
func newTextOrderGuard(checked *cel.Ast) *textOrderGuard {
	g := &textOrderGuard{operands: map[int64]string{}}
	if checked == nil {
		return g
	}
	calls := ast.MatchDescendants(ast.NavigateAST(checked.NativeRep()), func(e ast.NavigableExpr) bool {
		if e.Kind() != ast.CallKind {
			return false
		}
		_, ok := relationalOperators[e.AsCall().FunctionName()]
		return ok
	})
	for _, call := range calls {
		op := relationalOperators[call.AsCall().FunctionName()]
		for _, arg := range call.AsCall().Args() {
			g.operands[arg.ID()] = op
		}
	}
	return g
}

// decorate wraps the planned program step for a relational operand so the value
// it yields is inspected. Only those operands are wrapped: their sole consumer is
// the binary relational call itself, which needs nothing beyond Interpretable, so
// wrapping cannot disturb attribute/qualifier planning elsewhere in the program.
func (g *textOrderGuard) decorate(i interpreter.Interpretable) (interpreter.Interpretable, error) {
	if op, ok := g.operands[i.ID()]; ok {
		return &textOrderWatch{Interpretable: i, guard: g, op: op}, nil
	}
	return i, nil
}

// record keeps the FIRST text operand seen (deterministic: evaluation order is
// fixed for a given expression and input).
func (g *textOrderGuard) record(op, value string) {
	if g.hit {
		return
	}
	g.hit, g.hitOp, g.hitValue = true, op, value
}

// err reports the adopter-facing refusal when the evaluation ordered text.
func (g *textOrderGuard) err() error {
	if !g.hit {
		return nil
	}
	return fmt.Errorf(
		"the %s comparison received the text value %q; assent will not order text, because ordering text sorts it character by character (which puts \"6\" after \"12\"). If that value is a number quoted in the YAML, unquote it or compare with int(...) or double(...); compare dates with timestamp(...)",
		g.hitOp, g.hitValue)
}

// textOrderWatch is a pass-through program step that reports a text result to its
// guard. It never alters the value or the control flow.
type textOrderWatch struct {
	interpreter.Interpretable
	guard *textOrderGuard
	op    string
}

func (w *textOrderWatch) Eval(activation interpreter.Activation) ref.Val {
	v := w.Interpretable.Eval(activation)
	if s, isText := v.(types.String); isText {
		w.guard.record(w.op, string(s))
	}
	return v
}

// entryOr returns the reconstructed entry object when one is present, else the
// scalar fallback (ch.New/ch.Old). A nil entry is the current, all-callers state
// and yields the exact pre-S02 scalar binding — an absent/unreconstructable
// entry NEVER fabricates a permissive binding (fail-safe: the additive richer
// bind can only be added, never removed).
func entryOr(entry, fallback any) any {
	if entry != nil {
		return entry
	}
	return fallback
}

// LeafActivation builds the CEL activation for one change (E8-S08 render wiring).
func LeafActivation(in EvaluationInput, ch EvalChange, envLabel string) map[string]any {
	return bindLeafActivation(in, ch, envLabel)
}

// bindLeafActivation builds the CEL activation for one change: the change-scoped
// fields from ch (old/new/entry/oldEntry typed via toCEL, path/kind/file/env
// strings), plus the shared changes/facts/mr. entry/oldEntry bind the
// reconstructed whole-entry object for the change's EntryRef WHEN ONE IS PRESENT
// (ch.Entry/ch.OldEntry, populated by the Part-B adopter-test harness), and fall
// back to the change's scalar new/old value trees when absent — so every existing
// evaluation (all current callers leave Entry nil) is byte-identical and only a
// populated entry object changes the binding (fail-safe: an absent entry can
// never fabricate a permissive bind).
func bindLeafActivation(in EvaluationInput, ch EvalChange, envLabel string) map[string]any {
	changesList := make([]any, len(in.ChangeSet.Changes))
	for i, c := range in.ChangeSet.Changes {
		changesList[i] = map[string]any{
			"subject": c.Subject,
			"file":    c.File,
			"path":    c.Path,
			"kind":    c.Kind,
			"old":     toCEL(c.Old),
			"new":     toCEL(c.New),
		}
	}
	return map[string]any{
		"old":      toCEL(ch.Old),
		"new":      toCEL(ch.New),
		"entry":    toCEL(entryOr(ch.Entry, ch.New)),
		"oldEntry": toCEL(entryOr(ch.OldEntry, ch.Old)),
		"path":     ch.Path,
		"kind":     ch.Kind,
		"file":     ch.File,
		"env":      envLabel,
		"changes":  changesList,
		"facts":    factsToCEL(in.Facts),
		"mr":       mrToCEL(in.MR),
	}
}

// stateResolved is the ONLY fact state that exposes a `value` binding. The other
// three frozen states (unavailable/invalid/expired) are non-resolved and never
// bind a value — reading `facts.<p>.<n>.value` on them errors, which is fail-safe
// by effect (ADR-0007 F6 / ADR-0017 §6). Declared here (not imported) to keep the
// pure engine self-contained.
const stateResolved = "resolved"

// factsToCEL flattens facts to CEL-navigable maps keyed provider->name->field.
// The `value` key is bound ONLY for a RESOLVED fact (E2-S05): a non-resolved fact
// (unavailable/invalid/expired) NEVER exposes value — even a malformed/stale
// in-memory Fact that carries one — so a predicate reading `facts.<p>.<n>.value`
// on it errors -> fail-safe, never a permissive silent bind. This state gate is
// load-bearing: without it a stale value on a non-resolved controlling fact could
// bind and let the run evaluate permissively (fail-open).
func factsToCEL(facts map[string]map[string]Fact) map[string]any {
	out := make(map[string]any, len(facts))
	for provider, byName := range facts {
		pm := make(map[string]any, len(byName))
		for name, f := range byName {
			fm := map[string]any{
				"state":      f.State,
				"sensitive":  f.Sensitive,
				"observedAt": f.ObservedAt,
			}
			if f.ExpiresAt != "" {
				fm["expiresAt"] = f.ExpiresAt
			}
			if f.Reason != "" {
				fm["reason"] = f.Reason
			}
			if f.State == stateResolved && f.Value != nil {
				fm["value"] = toCEL(f.Value)
			}
			pm[name] = fm
		}
		out[provider] = pm
	}
	return out
}

// mrToCEL builds the `mr` activation map.
func mrToCEL(mr MR) map[string]any {
	labels := make([]any, len(mr.Labels))
	for i, l := range mr.Labels {
		labels[i] = l
	}
	return map[string]any{
		"author":       mr.Author,
		"sourceBranch": mr.SourceBranch,
		"targetBranch": mr.TargetBranch,
		"labels":       labels,
	}
}

// toCEL converts a JSON-decoded value (json.Number, map, slice, scalar) into the
// native Go types cel-go adapts: an integral json.Number -> int64 (injective, no
// float64 collapse — mirroring internal/change's numeric discipline), a decimal
// -> float64, maps/slices recursively, everything else passed through.
//
// A numeric literal that fits NEITHER int64 nor float64 (|v| beyond ~1.8e308)
// binds a CEL ERROR value, not its string form (D-129). The string form was a
// silent demotion to text: `new > old` over 9e399 and 1e400 became the lexical
// "9e399" > "1e400" = TRUE, the numerically wrong answer, with no error. An error
// value propagates through every operator that touches it -> the caller's
// fail-safe path. Only the unrepresentable extreme is refused: an over-int64
// literal that IS float64-representable still binds (lossily) as before — the
// documented ADR-0013 residual #1, unchanged.
func toCEL(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return types.NewErr("the number %s is too large for assent to compare (it exceeds the representable numeric range)", x.String())
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[k] = toCEL(val)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i, val := range x {
			s[i] = toCEL(val)
		}
		return s
	default:
		return v
	}
}
