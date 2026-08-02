package aggregate

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const d016EvalInput = "../../../examples/contracts/d016-strict-fixture/evaluation-input.json"

func loadD016Input(t *testing.T) *EvaluationInput {
	t.Helper()
	raw, err := os.ReadFile(d016EvalInput) //nolint:gosec // hardcoded fixture path
	if err != nil {
		t.Fatalf("read evaluation-input: %v", err)
	}
	in, err := LoadEvaluationInput(raw)
	if err != nil {
		t.Fatalf("load evaluation-input: %v", err)
	}
	return in
}

// changeAtPath returns the first change with the given file+path.
func changeAtPath(t *testing.T, in *EvaluationInput, file, path string) EvalChange {
	t.Helper()
	for _, c := range in.ChangeSet.Changes {
		if c.File == file && c.Path == path {
			return c
		}
	}
	t.Fatalf("no change at %s%s", file, path)
	return EvalChange{}
}

// REQ-E2-S02-01: a single-leaf `when` evaluates over a real EvaluationInput's
// change using the top-level old/new (numeric) and facts activation bindings.
func TestSingleLeafNewOldOverEvaluationInput(t *testing.T) {
	in := loadD016Input(t)
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}

	// partitions modify: old=12, new=6 -> `new >= old` is false (obligation unproven).
	part := changeAtPath(t, in, "topics/prod/orders-events.yaml", "/partitions")
	got, err := evalLeaf(env, *in, part, "prod", "new >= old")
	if err != nil {
		t.Fatalf("eval new>=old: %v", err)
	}
	if got {
		t.Error("new(6) >= old(12) must be false")
	}

	// The owner rule's fact guard over the expired fact -> false (not resolved).
	got, err = evalLeaf(env, *in, part, "prod", "facts.owner.team.state == 'resolved'")
	if err != nil {
		t.Fatalf("eval fact guard: %v", err)
	}
	if got {
		t.Error("expired fact must not satisfy state == 'resolved'")
	}
}

// REQ-E2-S02-02: a `when` referencing an identifier outside the frozen
// predicate scope (input, foo) is rejected at compile — never a silent false.
func TestUndeclaredPredicateReferenceRejected(t *testing.T) {
	in := loadD016Input(t)
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	part := changeAtPath(t, in, "topics/prod/orders-events.yaml", "/partitions")

	// The pre-fix D-016 typo — proves lane F is required, not optional.
	_, err = evalLeaf(env, *in, part, "prod", "input.new >= input.old")
	if err == nil {
		t.Fatal("input.new/input.old must be rejected (undeclared reference)")
	}
	if !strings.Contains(err.Error(), "input") {
		t.Errorf("error must name the undeclared reference, got: %v", err)
	}
	if _, err := evalLeaf(env, *in, part, "prod", "foo == 1"); err == nil {
		t.Error("an undeclared 'foo' must be rejected")
	}
	// The in-scope form compiles and evaluates cleanly.
	if _, err := evalLeaf(env, *in, part, "prod", "new >= old"); err != nil {
		t.Errorf("in-scope new>=old must compile, got: %v", err)
	}
}

// REQ-E2-S02-03: each of the eleven frozen predicate-scope fields resolves to
// its EvaluationInput-derived value; a twelfth invented field does not compile.
func TestFrozenPredicateScopeExactlyBound(t *testing.T) {
	in := loadD016Input(t)
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	// The rename change carries whole-document old/new objects, so entry/oldEntry
	// and old/new object navigation all resolve.
	rename := changeAtPath(t, in, "topics/prod/orders-events.yaml", "")

	inScope := []string{
		`old.partitions == 12`,
		`new.partitions == 6`,
		`entry.metadata.name == "orders.events.v1"`,
		`oldEntry.metadata.name == "orders.events.v1"`,
		`path == ""`,
		`kind == "rename"`,
		`file == "topics/prod/orders-events.yaml"`,
		`env == "prod"`,
		`size(changes) == 3`,
		`facts.owner.team.state == "expired"`,
		`mr.author == "dev-a"`,
	}
	for _, expr := range inScope {
		got, err := evalLeaf(env, *in, rename, "prod", expr)
		if err != nil {
			t.Errorf("in-scope %q errored: %v", expr, err)
			continue
		}
		if !got {
			t.Errorf("in-scope %q evaluated false (should resolve true)", expr)
		}
	}
	if _, err := evalLeaf(env, *in, rename, "prod", "object.x == 1"); err == nil {
		t.Error("a twelfth invented field must not compile")
	}
}

// REQ-E2-S02-04: numeric comparison is injective (no float64 collapse) and
// fails safe on a cross-type compare, never a silent wrong bool or a panic.
func TestNumericCoercionInjectiveOrFailSafe(t *testing.T) {
	in := loadD016Input(t)
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}

	// Two integers that collapse to the same float64 (2^53 and 2^53+1) must
	// still compare as distinct — injective int64, not lossy double.
	big := EvalChange{
		File: "x", Path: "/n", Kind: "modify",
		Old: json.Number("9007199254740992"), // 2^53
		New: json.Number("9007199254740993"), // 2^53 + 1
	}
	got, err := evalLeaf(env, *in, big, "prod", "new == old")
	if err != nil {
		t.Fatalf("big int compare errored: %v", err)
	}
	if got {
		t.Error("2^53 and 2^53+1 must compare unequal (no float64 collapse)")
	}

	// A numeric leaf over the rename change's OBJECT old/new must fail safe
	// (error), never a silent wrong bool and never a panic.
	rename := changeAtPath(t, in, "topics/prod/orders-events.yaml", "")
	if _, err := evalLeaf(env, *in, rename, "prod", "new >= old"); err == nil {
		t.Error("a numeric compare over object old/new must fail safe to error")
	}
}

// REQ-E2-S02-05: the same input + leaf evaluated twice yields the same result
// (determinism; purity is enforced separately by TestCorePurity).
func TestEvaluatorDoubleRunStable(t *testing.T) {
	in := loadD016Input(t)
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	part := changeAtPath(t, in, "topics/prod/orders-events.yaml", "/partitions")
	a, errA := evalLeaf(env, *in, part, "prod", "new >= old")
	b, errB := evalLeaf(env, *in, part, "prod", "new >= old")
	if errA != nil || errB != nil {
		t.Fatalf("errs: %v %v", errA, errB)
	}
	if a != b {
		t.Errorf("double-run diverged: %v vs %v", a, b)
	}
}

// LoadEvaluationInput rejects a schema-invalid document and malformed JSON,
// failing closed (never a partial/empty input silently accepted).
func TestLoadEvaluationInputRejectsBad(t *testing.T) {
	// Missing the required changeSet — schema violation.
	bad := `{"apiVersion":"assent.dev/v1alpha1","kind":"EvaluationInput"}`
	if _, err := LoadEvaluationInput([]byte(bad)); err == nil {
		t.Error("a schema-invalid EvaluationInput must be rejected")
	}
	// Malformed JSON — parse error.
	if _, err := LoadEvaluationInput([]byte("{")); err == nil {
		t.Error("malformed JSON must be rejected")
	}
}

// evalLeaf rejects a non-boolean result (a `when` that yields a string/int must
// not be read as true/false); toCEL handles decimals and arrays.
func TestEvalLeafNonBoolAndTypedValues(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	in := EvaluationInput{}

	// A non-boolean `when` result is malformed -> error, never true/false.
	strCh := EvalChange{File: "x", Path: "/s", Kind: "modify", Old: "a", New: "b"}
	if _, err := evalLeaf(env, in, strCh, "prod", `"literal"`); err == nil {
		t.Error("a non-boolean result must be rejected")
	}

	// A decimal old value (toCEL float64 branch) compares as a double.
	dec := EvalChange{File: "x", Path: "/d", Kind: "modify", Old: json.Number("1.5"), New: json.Number("2.5")}
	got, err := evalLeaf(env, in, dec, "prod", "new > old")
	if err != nil || !got {
		t.Errorf("2.5 > 1.5 must be true, got %v err=%v", got, err)
	}

	// An array old value (toCEL []any branch) navigates as a list.
	arr := EvalChange{File: "x", Path: "/a", Kind: "modify", Old: []any{json.Number("1"), json.Number("2")}, New: []any{}}
	got, err = evalLeaf(env, in, arr, "prod", "size(old) == 2")
	if err != nil || !got {
		t.Errorf("size(old)==2 must be true, got %v err=%v", got, err)
	}
}

// Fail-safe field-selection seams (the E2-S05 foundation): selecting a field on
// a SCALAR entry, and reading `.value` on a non-resolved (value-absent) fact,
// both ERROR — never a silent zero/false that would fail open. Regression-locks
// the seams a later fact-tri-state story depends on.
func TestFailSafeFieldSelectionSeams(t *testing.T) {
	in := loadD016Input(t)
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}

	// entry for the /partitions modify is the scalar 6 (not a document) —
	// selecting a field on it must error, not read a zero.
	part := changeAtPath(t, in, "topics/prod/orders-events.yaml", "/partitions")
	if _, err := evalLeaf(env, *in, part, "prod", "entry.metadata == 1"); err == nil {
		t.Error("field selection on a scalar entry must fail safe (error), not read a default")
	}

	// The owner fact is expired -> no `value` key. Reading it must error, so a
	// value-referencing predicate over a stale fact can never satisfy.
	if _, err := evalLeaf(env, *in, part, "prod", "facts.owner.team.value == 6"); err == nil {
		t.Error("reading .value on a value-absent (expired) fact must fail safe (error)")
	}
	// The state guard over the same expired fact evaluates cleanly to false.
	got, err := evalLeaf(env, *in, part, "prod", "facts.owner.team.state == 'expired'")
	if err != nil || !got {
		t.Errorf("state read on an expired fact must resolve true, got %v err=%v", got, err)
	}
}
