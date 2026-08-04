package aggregate

import (
	"encoding/json"
	"testing"
)

// REQ-E6-S02-01 (Part A · ENGINE): an EvalChange carrying a reconstructed Entry
// object binds CEL `entry`/`oldEntry` to that whole-entry object, so an
// entry-scoped predicate (`entry.<field>`) resolves against the reconstructed
// entry — not the change's scalar New/Old. Before the binding fix this errored,
// because `entry` bound to the scalar `toCEL(ch.New)` (e.g. int64(16)) and
// selecting `.owner` on a scalar fails safe (error). Exercised through the real
// evalLeaf/bindLeafActivation path, mirroring evaluate_test.go.
func TestBindLeafActivationBindsEntryObjectWhenPresent(t *testing.T) {
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	in := EvaluationInput{}

	// A sub-value change (partitions 12 -> 16) whose scalar New/Old are the
	// numbers, but whose reconstructed whole-entry object carries the sibling
	// `owner` field an entry-scoped predicate needs.
	ch := EvalChange{
		File: "topics/prod/orders.yaml", Path: "/partitions", Kind: "modify",
		Old: json.Number("12"),
		New: json.Number("16"),
		Entry: map[string]any{
			"owner":      "team-a",
			"partitions": json.Number("16"),
		},
		OldEntry: map[string]any{
			"owner":      "team-a",
			"partitions": json.Number("12"),
		},
	}

	// entry.<field> binds the reconstructed object -> true. Without the fix this
	// bound the scalar 16 and errored on the field selection.
	got, err := evalLeaf(env, in, ch, "prod", `entry.owner == "team-a"`)
	if err != nil {
		t.Fatalf("entry.owner over reconstructed entry errored: %v", err)
	}
	if !got {
		t.Error("entry.owner == \"team-a\" must resolve true when Entry is populated")
	}

	// The scalar new/old bindings are untouched by the enrichment.
	got, err = evalLeaf(env, in, ch, "prod", "new == 16 && old == 12")
	if err != nil {
		t.Fatalf("scalar new/old over enriched change errored: %v", err)
	}
	if !got {
		t.Error("new/old must still bind the change scalars when Entry is populated")
	}

	// oldEntry.<field> binds the pre-image object symmetrically.
	got, err = evalLeaf(env, in, ch, "prod", "oldEntry.partitions == 12")
	if err != nil {
		t.Fatalf("oldEntry.partitions over reconstructed pre-image errored: %v", err)
	}
	if !got {
		t.Error("oldEntry.partitions == 12 must resolve true when OldEntry is populated")
	}
}

// REQ-E6-S02-02 (Part A · ENGINE · fail-safe): an EvalChange with NO
// reconstructed entry preserves the EXACT pre-S02 scalar binding — entry==new,
// oldEntry==old — so every existing evaluation is byte-identical. The additive
// entryOr fallback must never fabricate a permissive binding for an absent entry.
func TestBindLeafActivationPreservesScalarWhenEntryAbsent(t *testing.T) {
	in := EvaluationInput{}

	// A whole-entry (rename-like) change with object New/Old and Entry==nil: the
	// binding must fall back to the scalars, exactly as before this story.
	newObj := map[string]any{"owner": "team-a", "partitions": json.Number("16")}
	oldObj := map[string]any{"owner": "team-b", "partitions": json.Number("12")}
	ch := EvalChange{
		File: "topics/prod/orders.yaml", Path: "", Kind: "rename",
		Old: oldObj,
		New: newObj,
		// Entry and OldEntry intentionally left nil (the current, all-callers state).
	}

	// Byte-identical fallback: the activation's entry/oldEntry are exactly
	// toCEL(New)/toCEL(Old) when the reconstructed entry is absent.
	act := bindLeafActivation(in, ch, "prod")
	if !celValueEqual(act["entry"], toCEL(ch.New)) {
		t.Errorf("entry binding with nil Entry must equal toCEL(New); got %#v", act["entry"])
	}
	if !celValueEqual(act["oldEntry"], toCEL(ch.Old)) {
		t.Errorf("oldEntry binding with nil OldEntry must equal toCEL(Old); got %#v", act["oldEntry"])
	}

	// And through the real eval path a predicate reads entry as the New object.
	env, err := newEvalEnv()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	got, err := evalLeaf(env, in, ch, "prod", `entry.owner == "team-a"`)
	if err != nil {
		t.Fatalf("entry.owner over fallback (New object) errored: %v", err)
	}
	if !got {
		t.Error("with Entry absent, entry must bind the change's New object (scalar fallback)")
	}
}

// TestEntryOrFallsBackWhenNil locks entryOr directly: a non-nil entry is
// returned as-is; a nil entry falls back to the provided value. This is the
// whole fail-safe seam — a nil (absent/unreconstructable) entry NEVER synthesises
// a value, it defers to the current behaviour.
func TestEntryOrFallsBackWhenNil(t *testing.T) {
	entry := map[string]any{"owner": "team-a"}
	fallback := json.Number("16")

	if got := entryOr(entry, fallback); !celValueEqual(got, entry) {
		t.Errorf("entryOr must return the entry when non-nil; got %#v", got)
	}
	if got := entryOr(nil, fallback); !celValueEqual(got, fallback) {
		t.Errorf("entryOr must return the fallback when entry is nil; got %#v", got)
	}
}

// celValueEqual compares two toCEL-shaped values structurally (maps/slices/
// scalars) for the activation-equality assertions above.
func celValueEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
