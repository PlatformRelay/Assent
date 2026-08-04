package aggregate

import (
	"encoding/json"
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
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
// evaluated under the cost budget without error, and produced a boolean; every
// other outcome (undeclared reference, coercion/type error, cost overrun,
// non-boolean result) returns a non-nil error so the caller fails safe. It NEVER
// returns (true, nil) for a malformed or type-erroring predicate.
func evalLeaf(env *cel.Env, in EvaluationInput, ch EvalChange, envLabel, expr string) (bool, error) {
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return false, fmt.Errorf("compile when %q: %w", expr, iss.Err())
	}
	prg, err := env.Program(ast, cel.CostLimit(celCostBudget))
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
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("when %q result is %s, not bool", expr, out.Type().TypeName())
	}
	return b, nil
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
func toCEL(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
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
