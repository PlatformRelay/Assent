package celspike

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/interpreter"
)

// TriState is ADR-0007 amendment 1: true / false / error.
type TriState int

// Tri-state outcomes for a leaf or tree node.
const (
	// TriPass is predicate true.
	TriPass TriState = iota
	// TriFail is predicate false.
	TriFail
	// TriError is evaluation/compile failure (ADR-0007 fail-safe).
	TriError
)

func (s TriState) String() string {
	switch s {
	case TriPass:
		return "pass"
	case TriFail:
		return "fail"
	case TriError:
		return "error"
	default:
		return "unknown"
	}
}

// ErrorClass distinguishes the four Spike A error classes in per-leaf traces.
type ErrorClass string

// Spike A error classes distinguishable in LeafTrace.
const (
	// ClassNone is set on pass/fail traces (no error).
	ClassNone ErrorClass = ""
	// ClassMissingFact is a missing map key / undeclared reference at eval.
	ClassMissingFact ErrorClass = "missing_fact"
	// ClassUnknownField is a compile-time undefined field on a typed object.
	ClassUnknownField ErrorClass = "unknown_field"
	// ClassTypeMismatch is overload / conversion failure.
	ClassTypeMismatch ErrorClass = "type_mismatch"
	// ClassCostLimit is cel.CostLimit exceeded.
	ClassCostLimit ErrorClass = "cost_limit"
)

// LeafTrace is one leaf's explain record (id, result, message, optional error class).
type LeafTrace struct {
	ID         string
	State      TriState
	Message    string
	ErrorClass ErrorClass
	Err        error
}

// Noder is an all/any/not tree node or a CEL leaf.
type Noder interface {
	isNode()
}

// Leaf is a CEL expression with optional message template (unevaluated here).
type Leaf struct {
	ID      string
	CEL     string
	Message string
}

func (Leaf) isNode() { /* marker only: seals Noder to this package's node set */ }

// All requires every child to pass (errors short-circuit as error).
type All struct {
	Children []Noder
}

func (All) isNode() { /* marker only: seals Noder to this package's node set */ }

// Any requires at least one child to pass.
type Any struct {
	Children []Noder
}

func (Any) isNode() { /* marker only: seals Noder to this package's node set */ }

// Not inverts pass/fail; error stays error.
type Not struct {
	Child Noder
}

func (Not) isNode() { /* marker only: seals Noder to this package's node set */ }

// WalkResult is the tree outcome plus per-leaf traces.
type WalkResult struct {
	State  TriState
	Traces []LeafTrace
}

// Walk evaluates an all/any/not tree, capturing a LeafTrace per leaf.
func Walk(env *cel.Env, root Noder, activation any) WalkResult {
	var traces []LeafTrace
	state := walk(env, root, activation, &traces)
	return WalkResult{State: state, Traces: traces}
}

func walk(env *cel.Env, n Noder, activation any, traces *[]LeafTrace) TriState {
	switch x := n.(type) {
	case Leaf:
		tr := evalLeaf(env, x, activation)
		*traces = append(*traces, tr)
		return tr.State
	case All:
		agg := TriPass
		for _, c := range x.Children {
			s := walk(env, c, activation, traces)
			switch s {
			case TriError:
				return TriError
			case TriFail:
				agg = TriFail
			}
		}
		return agg
	case Any:
		sawFail := false
		for _, c := range x.Children {
			s := walk(env, c, activation, traces)
			switch s {
			case TriPass:
				return TriPass
			case TriError:
				return TriError
			case TriFail:
				sawFail = true
			}
		}
		if sawFail {
			return TriFail
		}
		return TriFail
	case Not:
		s := walk(env, x.Child, activation, traces)
		switch s {
		case TriPass:
			return TriFail
		case TriFail:
			return TriPass
		default:
			return TriError
		}
	default:
		return TriError
	}
}

func evalLeaf(env *cel.Env, leaf Leaf, activation any) LeafTrace {
	tr := LeafTrace{ID: leaf.ID, Message: leaf.Message}
	ast, iss := env.Compile(leaf.CEL)
	if iss.Err() != nil {
		tr.State = TriError
		tr.Err = iss.Err()
		tr.ErrorClass = classifyError(iss.Err())
		return tr
	}
	ok, _, err := EvalBool(env, ast, activation)
	if err != nil {
		tr.State = TriError
		tr.Err = err
		tr.ErrorClass = classifyError(err)
		return tr
	}
	if ok {
		tr.State = TriPass
	} else {
		tr.State = TriFail
	}
	return tr
}

func classifyError(err error) ErrorClass {
	return ClassifyError(err)
}

// ClassifyError maps a cel-go / adapter error onto a Spike A error class.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ClassNone
	}
	msg := err.Error()
	var cancelled interpreter.EvalCancelledError
	if errors.As(err, &cancelled) && cancelled.Cause == interpreter.CostLimitExceeded {
		return ClassCostLimit
	}
	if strings.Contains(msg, "cost limit") || strings.Contains(msg, "actual cost limit exceeded") {
		return ClassCostLimit
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "no such key"),
		strings.Contains(lower, "no such attribute"),
		strings.Contains(lower, "undeclared reference"):
		return ClassMissingFact
	case strings.Contains(lower, "undefined field"),
		strings.Contains(lower, "does not have"):
		return ClassUnknownField
	case strings.Contains(lower, "no such overload"),
		strings.Contains(lower, "found no matching overload"),
		strings.Contains(lower, "type conversion"),
		strings.Contains(msg, errQuotedNumeric.Error()),
		strings.Contains(msg, errCrossTypeString.Error()),
		strings.Contains(msg, errInt64Overflow.Error()),
		strings.Contains(lower, "mismatched"):
		return ClassTypeMismatch
	default:
		return ClassTypeMismatch
	}
}

// VouchSatisfied maps tri-state onto ADR-0007 vouch polarity:
// only TriPass satisfies; TriFail and TriError do not (error never proves).
func VouchSatisfied(state TriState) bool {
	return state == TriPass
}

// FormatTraceSummary is a debug helper for tests.
func FormatTraceSummary(traces []LeafTrace) string {
	parts := make([]string, 0, len(traces))
	for _, tr := range traces {
		parts = append(parts, fmt.Sprintf("%s=%s/%s", tr.ID, tr.State, tr.ErrorClass))
	}
	return strings.Join(parts, ",")
}
