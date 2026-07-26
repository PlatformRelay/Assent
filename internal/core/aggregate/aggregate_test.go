package aggregate

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/schemas"
)

// oneChange is the walking-skeleton single-entry ChangeSet: partitions 6 -> 12.
func oneChange() change.ChangeSet {
	return change.ChangeSet{Changes: []change.Change{{
		File: "topics/prod/orders.events.v1.yaml",
		Path: "/partitions",
		Kind: change.KindModify,
		Old:  "6",
		New:  "12",
	}}}
}

// nonDestructiveBinding proves "non-destructive" by asserting the new partition
// count is not below the old — a NUMERIC comparison that MUST coerce via int().
func nonDestructiveBinding(when string, eff Effect) Binding {
	return Binding{
		Require: []string{"non-destructive"},
		Subject: "file:topics/prod/orders.events.v1.yaml",
		Rules: []Rule{{
			Name:       "kafka.non-destructive",
			Obligation: "non-destructive",
			When:       when,
			OnFailure:  OnFailure{Effect: eff, Code: "kafka.partitions.shrunk"},
		}},
	}
}

// REQ-P4-E1-S03-01: a satisfied obligation -> APPROVE; an unsatisfied one
// reflects the rule's onFailure.effect and is NEVER APPROVE.
func TestOneObligationDecision(t *testing.T) {
	t.Run("satisfied -> APPROVE", func(t *testing.T) {
		// int(new) 12 >= int(old) 6 is true: obligation proven.
		b := nonDestructiveBinding("int(new) >= int(old)", EffectBlock)
		res, err := Aggregate(b, oneChange(), "")
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		if res.Decision != DecisionApprove {
			t.Fatalf("decision = %s, want APPROVE", res.Decision)
		}
		if len(res.Findings) != 0 {
			t.Fatalf("satisfied obligation should emit no finding, got %+v", res.Findings)
		}
	})

	t.Run("unsatisfied block -> BLOCK, never APPROVE", func(t *testing.T) {
		// Assert new < old (12 < 6) — false, so onFailure(block) applies.
		b := nonDestructiveBinding("int(new) < int(old)", EffectBlock)
		res, err := Aggregate(b, oneChange(), "")
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		if res.Decision != DecisionBlock {
			t.Fatalf("decision = %s, want BLOCK", res.Decision)
		}
		if res.Decision == DecisionApprove {
			t.Fatal("unproven obligation must never APPROVE")
		}
	})

	t.Run("unsatisfied comment -> REVIEW, never APPROVE", func(t *testing.T) {
		b := nonDestructiveBinding("int(new) < int(old)", EffectComment)
		res, err := Aggregate(b, oneChange(), "")
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		if res.Decision != DecisionReview {
			t.Fatalf("decision = %s, want REVIEW", res.Decision)
		}
	})

	t.Run("unsatisfied require-review -> REVIEW, never APPROVE", func(t *testing.T) {
		b := nonDestructiveBinding("int(new) < int(old)", EffectRequireReview)
		res, err := Aggregate(b, oneChange(), "")
		if err != nil {
			t.Fatalf("Aggregate: %v", err)
		}
		if res.Decision != DecisionReview {
			t.Fatalf("decision = %s, want REVIEW", res.Decision)
		}
	})
}

// REQ-P4-E1-S03-02: a predicate that ERRORS fails safe to REVIEW, never APPROVE.
func TestPredicateErrorFailsSafe(t *testing.T) {
	cases := []struct {
		name string
		when string
		cs   change.ChangeSet
	}{
		{
			// References a field absent from the activation -> compile error.
			name: "unknown variable",
			when: "int(missing) > 0",
			cs:   oneChange(),
		},
		{
			// A non-numeric canonical value makes int() ERROR at eval (verified
			// empirically in the cel-go spike); this is the coercion fail-open path.
			name: "non-numeric coercion error",
			when: "int(new) > int(old)",
			cs: change.ChangeSet{Changes: []change.Change{{
				File: "f.yaml", Path: "/name", Kind: change.KindModify,
				Old: "\"alpha\"", New: "\"beta\"", // JSON-quoted strings: int() errors
			}}},
		},
		{
			// A `when` that returns a non-boolean must not be read as true/false.
			name: "non-bool result",
			when: "new", // a string, not a bool
			cs:   oneChange(),
		},
		{
			// A syntactically broken expression -> compile error -> REVIEW.
			name: "uncompilable expression",
			when: "int(new) >>> ",
			cs:   oneChange(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := nonDestructiveBinding(tc.when, EffectBlock)
			res, err := Aggregate(b, tc.cs, "")
			if err != nil {
				t.Fatalf("Aggregate returned a hard error (should fail safe internally): %v", err)
			}
			if res.Decision == DecisionApprove {
				t.Fatalf("a predicate error must NEVER yield APPROVE, got %s", res.Decision)
			}
			if res.Decision != DecisionReview {
				t.Fatalf("predicate error should fail safe to REVIEW, got %s", res.Decision)
			}
		})
	}
}

// The opaque-ChangeSet fail-open path: an undecidable differ result -> REVIEW.
func TestOpaqueChangeSetFailsSafe(t *testing.T) {
	b := nonDestructiveBinding("int(new) >= int(old)", EffectBlock)
	cs := change.ChangeSet{Opaque: true, OpaqueReason: "alias bomb"}
	res, err := Aggregate(b, cs, "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if res.Decision != DecisionReview {
		t.Fatalf("opaque changeset must fail safe to REVIEW, got %s", res.Decision)
	}
	if res.Decision == DecisionApprove {
		t.Fatal("opaque changeset must NEVER APPROVE")
	}
}

// The empty-changes fail-open path: no entries (schema minItems:1) -> REVIEW.
func TestEmptyChangeSetFailsSafe(t *testing.T) {
	b := nonDestructiveBinding("int(new) >= int(old)", EffectBlock)
	res, err := Aggregate(b, change.ChangeSet{}, "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if res.Decision != DecisionReview {
		t.Fatalf("empty changeset must fail safe to REVIEW, got %s", res.Decision)
	}
	if res.Decision == DecisionApprove {
		t.Fatal("empty changeset must NEVER APPROVE")
	}
}

// A required obligation with NO proving rule is a coverage gap -> REVIEW, never
// APPROVE (APPROVE modelled as coverage, not absence-of-failure).
func TestUncoveredObligationFailsSafe(t *testing.T) {
	b := Binding{
		Require: []string{"non-destructive", "reviewed"}, // "reviewed" has no rule
		Subject: "file:x.yaml",
		Rules: []Rule{{
			Name: "nd", Obligation: "non-destructive",
			When:      "int(new) >= int(old)",
			OnFailure: OnFailure{Effect: EffectBlock, Code: "c"},
		}},
	}
	res, err := Aggregate(b, oneChange(), "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if res.Decision != DecisionReview {
		t.Fatalf("uncovered obligation must fail safe to REVIEW, got %s", res.Decision)
	}
}

// REQ-P4-E1-S03-01 (constraint d): a tag-only change where Old==New string-equal
// is still a real change; the aggregator must not re-derive change-ness from
// Old vs New. Here presence of the entry drives evaluation.
func TestPresenceIsChangeSignal(t *testing.T) {
	// !!int 1 -> !!float 1 both render "1": the differ emitted the entry because
	// tags differ. A rule that inspects `changes` sees one entry regardless of
	// Old==New. Assert the obligation over the entry count.
	cs := change.ChangeSet{Changes: []change.Change{{
		File: "f.yaml", Path: "/x", Kind: change.KindModify, Old: "1", New: "1",
	}}}
	b := Binding{
		Require: []string{"present"},
		Subject: "file:f.yaml",
		Rules: []Rule{{
			Name: "one-entry", Obligation: "present",
			When:      "size(changes) == 1", // reasons over presence, not old!=new
			OnFailure: OnFailure{Effect: EffectBlock, Code: "c"},
		}},
	}
	res, err := Aggregate(b, cs, "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if res.Decision != DecisionApprove {
		t.Fatalf("presence-based rule should APPROVE (entry exists), got %s", res.Decision)
	}
}

// F1 regression (P0 fail-open): a REAL multi-field diff must not fall through to
// APPROVE for a NON-numeric predicate. With 0 or >1 change entries the scalar
// old/new are UNBOUND, so any predicate referencing them errors -> REVIEW. The
// vulnerable case was a 2-entry diff + `when: "old == new"` binding both to ""
// -> "" == "" -> true -> APPROVE on an unproven obligation. Uses the real S02
// change.Diff so the reachability is genuine, not a synthetic ChangeSet.
func TestMultiEntryScalarPredicateFailsSafe(t *testing.T) {
	base := []byte("replicas: 3\npartitions: 6\n")
	head := []byte("replicas: 5\npartitions: 12\n")
	cs, err := change.Diff("deploy.yaml", base, head)
	if err != nil {
		t.Fatalf("change.Diff: %v", err)
	}
	if len(cs.Changes) != 2 {
		t.Fatalf("expected a genuine 2-entry diff from the real differ, got %d: %+v", len(cs.Changes), cs.Changes)
	}

	// The exact fail-open predicate: a non-numeric equality that would have read
	// two empty-string bindings as equal.
	b := Binding{
		Require: []string{"unchanged"},
		Subject: "file:deploy.yaml",
		Rules: []Rule{{
			Name: "no-op", Obligation: "unchanged",
			When:      "old == new",
			OnFailure: OnFailure{Effect: EffectBlock, Code: "changed"},
		}},
	}
	res, err := Aggregate(b, cs, "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if res.Decision == DecisionApprove {
		t.Fatal("F1 fail-open: a 2-entry diff with `old == new` must NOT APPROVE (scalar old/new must be unbound -> REVIEW)")
	}
	if res.Decision != DecisionReview {
		t.Fatalf("multi-entry scalar reference must fail safe to REVIEW, got %s", res.Decision)
	}
}

// The single-entry old==new tag-only case still evaluates the REAL bound values
// (the scalar bindings are present exactly when len==1). Here old==new is true
// for a genuine tag-only change (Old==New string-equal), proving the fix did not
// break the single-entry convenience binding.
func TestSingleEntryScalarStillBinds(t *testing.T) {
	cs := change.ChangeSet{Changes: []change.Change{{
		File: "f.yaml", Path: "/x", Kind: change.KindModify, Old: "1", New: "1",
	}}}
	b := Binding{
		Require: []string{"unchanged"},
		Subject: "file:f.yaml",
		Rules: []Rule{{
			Name: "eq", Obligation: "unchanged",
			When:      "old == new", // real bound values "1" == "1" -> true
			OnFailure: OnFailure{Effect: EffectBlock, Code: "changed"},
		}},
	}
	res, err := Aggregate(b, cs, "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if res.Decision != DecisionApprove {
		t.Fatalf("single-entry old==new must evaluate real bound values (\"1\"==\"1\" -> APPROVE), got %s", res.Decision)
	}
}

// REQ-P4-E1-S07-01 SEAM: the reserved assent-policy class DOMINATES to BLOCK
// before any predicate, even one that would evaluate to a satisfied obligation.
func TestReservedPolicyClassDominatesToBlock(t *testing.T) {
	// A binding whose rule WOULD be satisfied (int(new) >= int(old) is true).
	b := nonDestructiveBinding("int(new) >= int(old)", EffectComment)
	res, err := Aggregate(b, oneChange(), ReservedPolicyClass)
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if res.Decision != DecisionBlock {
		t.Fatalf("assent-policy class must BLOCK independent of the predicate, got %s", res.Decision)
	}
}

// REQ-P4-E1-S03-03: shuffling rule input yields a byte-identical decision +
// canonically-sorted findings. The golden DOUBLE-RUNS (executes twice, byte-diff).
func TestAggregationOrderIndependent(t *testing.T) {
	// Three obligations, one failing (block), one failing (comment), one satisfied
	// — so findings is a multi-element set whose order must be canonical.
	base := []Rule{
		{Name: "r-block", Obligation: "o-block", When: "int(new) < int(old)",
			OnFailure: OnFailure{Effect: EffectBlock, Code: "b"}},
		{Name: "r-comment", Obligation: "o-comment", When: "int(new) < int(old)",
			OnFailure: OnFailure{Effect: EffectComment, Code: "c"}},
		{Name: "r-ok", Obligation: "o-ok", When: "int(new) >= int(old)",
			OnFailure: OnFailure{Effect: EffectBlock, Code: "d"}},
	}
	mk := func(rules []Rule) Binding {
		return Binding{
			Require: []string{"o-block", "o-comment", "o-ok"},
			Subject: "file:topics/prod/orders.events.v1.yaml",
			Rules:   rules,
		}
	}

	canonical := func(b Binding) []byte {
		// Double-run: execute twice, assert byte-identical, return the bytes.
		res1, err := Aggregate(b, oneChange(), "")
		if err != nil {
			t.Fatalf("Aggregate run 1: %v", err)
		}
		res2, err := Aggregate(b, oneChange(), "")
		if err != nil {
			t.Fatalf("Aggregate run 2: %v", err)
		}
		j1, _ := json.Marshal(res1)
		j2, _ := json.Marshal(res2)
		if !bytes.Equal(j1, j2) {
			t.Fatalf("double-run diverged:\n run1: %s\n run2: %s", j1, j2)
		}
		return j1
	}

	want := canonical(mk(base))

	// Shuffle the rule order several times; every permutation must match.
	rng := rand.New(rand.NewSource(1)) //nolint:gosec // test-only deterministic shuffle
	for i := 0; i < 20; i++ {
		shuffled := append([]Rule(nil), base...)
		rng.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		got := canonical(mk(shuffled))
		if !bytes.Equal(got, want) {
			t.Fatalf("shuffle %d produced a different Result:\n want: %s\n got:  %s", i, want, got)
		}
	}

	// The decision must be BLOCK (union of denies: block dominates comment).
	var res Result
	if err := json.Unmarshal(want, &res); err != nil {
		t.Fatal(err)
	}
	if res.Decision != DecisionBlock {
		t.Fatalf("decision = %s, want BLOCK (union of denies)", res.Decision)
	}
}

// TestSortFindingsTotalKey exercises every tiebreaker in the canonical finding
// sort (subject, rule, obligation, code, effect) so a shuffled slice of findings
// that share leading keys still sorts deterministically (REQ-P4-E1-S03-03).
func TestSortFindingsTotalKey(t *testing.T) {
	want := []Finding{
		{Subject: "a", Rule: "r", Obligation: "o1", Code: "c", Effect: EffectBlock},
		{Subject: "a", Rule: "r", Obligation: "o2", Code: "a", Effect: EffectBlock},
		{Subject: "a", Rule: "r", Obligation: "o2", Code: "b", Effect: EffectBlock},
		{Subject: "a", Rule: "r", Obligation: "o2", Code: "b", Effect: EffectComment},
		{Subject: "a", Rule: "r", Obligation: "o2", Code: "b", Effect: EffectRequireReview},
		{Subject: "a", Rule: "s", Obligation: "o1", Code: "c", Effect: EffectBlock},
		{Subject: "b", Rule: "r", Obligation: "o1", Code: "c", Effect: EffectBlock},
	}
	// Shuffle a copy and re-sort; it must return to `want`.
	shuffled := append([]Finding(nil), want...)
	rng := rand.New(rand.NewSource(7)) //nolint:gosec // test-only deterministic shuffle
	rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	sortFindings(shuffled)
	for i := range want {
		if shuffled[i] != want[i] {
			t.Fatalf("sortFindings not total at %d:\n got %+v\nwant %+v", i, shuffled[i], want[i])
		}
	}
}

// TestDeterminismDoubleRun is the named double-run gate subject (REQ-P4-E1-S12-01):
// the aggregator over a fixed input is byte-stable across repeated executions.
//
// A MULTI-FINDING binding is used (not the original single-finding one) so the
// gate has a multi-element Findings slice to be stable over — the strongest form
// of the byte-diff gate, and the general defence against ANY output-affecting
// nondeterminism (map iteration or otherwise). N runs are byte-diffed against the
// first; any divergence fails the job.
//
// On the spec's adversarial framing ("injecting a map-iteration-order dependence
// into aggregation makes the double-run diff fail"): in THIS implementation that
// dependence is not reachable — Aggregate builds Findings by ranging the
// deterministic `Require` slice (and the per-obligation rule slices), and the
// `rulesByObligation` map is lookup-only, never an output-ordering source. So Go's
// per-range map randomization cannot leak into the result, which is a positive
// property, not a hole (a `-count` run with sortFindings removed still passes,
// because finding order already comes from `Require`, not the map). `sortFindings`
// is defence-in-depth; INPUT-order independence over the meaningful perturbation
// (shuffling the rule slice) is separately gated by TestAggregationOrderIndependent
// (20 shuffles, byte-identical). Together those two are the S12-01 determinism gate.
func TestDeterminismDoubleRun(t *testing.T) {
	// Multiple failing obligations -> a multi-element Findings set whose order is
	// the only place map-iteration nondeterminism could leak.
	b := Binding{
		Require: []string{"o-block", "o-comment", "o-review", "o-ok"},
		Subject: "file:topics/prod/orders.events.v1.yaml",
		Rules: []Rule{
			{Name: "r-block", Obligation: "o-block", When: "int(new) < int(old)",
				OnFailure: OnFailure{Effect: EffectBlock, Code: "b"}},
			{Name: "r-comment", Obligation: "o-comment", When: "int(new) < int(old)",
				OnFailure: OnFailure{Effect: EffectComment, Code: "c"}},
			{Name: "r-review", Obligation: "o-review", When: "int(new) < int(old)",
				OnFailure: OnFailure{Effect: EffectRequireReview, Code: "r"}},
			{Name: "r-ok", Obligation: "o-ok", When: "int(new) >= int(old)",
				OnFailure: OnFailure{Effect: EffectBlock, Code: "d"}},
		},
	}

	first, err := Aggregate(b, oneChange(), "")
	if err != nil {
		t.Fatal(err)
	}
	// More than one finding is emitted, so map-order really can perturb output.
	if len(first.Findings) < 2 {
		t.Fatalf("determinism gate needs a multi-finding result to exercise map order, got %d", len(first.Findings))
	}
	want, _ := json.Marshal(first)

	// N runs: Go re-randomizes map iteration each range, so repeated runs sample
	// distinct rulesByObligation orderings. Every one must be byte-identical.
	const runs = 50
	for i := 0; i < runs; i++ {
		r, err := Aggregate(b, oneChange(), "")
		if err != nil {
			t.Fatalf("Aggregate run %d: %v", i, err)
		}
		got, _ := json.Marshal(r)
		if !bytes.Equal(got, want) {
			t.Fatalf("double-run diverged at run %d (map-iteration-order leak):\n want: %s\n got:  %s", i, want, got)
		}
	}
}

// An empty require list is a legal "no obligations" binding (ADR-0017 §2); it
// APPROVEs by design. Pinned explicitly given the "any path to APPROVE on
// nothing-proven is a P0" posture — this APPROVE is intended (zero required
// obligations), not an emergent fall-through.
func TestEmptyRequireApprovesByDesign(t *testing.T) {
	b := Binding{Require: nil, Subject: "file:x.yaml"}
	res, err := Aggregate(b, oneChange(), "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if res.Decision != DecisionApprove {
		t.Fatalf("empty require is a no-obligation binding -> APPROVE, got %s", res.Decision)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("no obligations -> no findings, got %+v", res.Findings)
	}
}

// Every emitted Finding must satisfy the frozen DecisionRecord #/$defs/finding
// object (rule+subject minLength:1, closed effect enum) so S04 can serialize the
// fail-safe paths into a VALID DecisionRecord. The most safety-critical outputs
// (opaque/empty/uncovered REVIEW findings) are exercised here — an empty rule
// would fail the schema, so this guards the fail-safe finding shape.
func TestFindingsValidateAgainstDecisionRecordSchema(t *testing.T) {
	// Collect findings across every fail-safe + unsatisfied path.
	var all []Finding
	collect := func(res Result) { all = append(all, res.Findings...) }

	r, _ := Aggregate(nonDestructiveBinding("int(new) < int(old)", EffectBlock), oneChange(), "")
	collect(r) // unsatisfied block
	r, _ = Aggregate(nonDestructiveBinding("int(new) < int(old)", EffectComment), oneChange(), "")
	collect(r) // unsatisfied comment
	r, _ = Aggregate(nonDestructiveBinding("int(missing) > 0", EffectBlock), oneChange(), "")
	collect(r) // predicate error
	r, _ = Aggregate(nonDestructiveBinding("int(new) >= int(old)", EffectBlock),
		change.ChangeSet{Opaque: true, OpaqueReason: "x"}, "")
	collect(r) // opaque -> undecidable sentinel
	r, _ = Aggregate(nonDestructiveBinding("int(new) >= int(old)", EffectBlock),
		change.ChangeSet{}, "")
	collect(r) // empty -> undecidable sentinel
	r, _ = Aggregate(Binding{Require: []string{"orphan"}, Subject: "file:x.yaml"}, oneChange(), "")
	collect(r) // uncovered obligation sentinel
	r, _ = Aggregate(nonDestructiveBinding("int(new) >= int(old)", EffectComment),
		oneChange(), ReservedPolicyClass)
	collect(r) // reserved-class block

	if len(all) == 0 {
		t.Fatal("no findings collected")
	}

	// Wrap each finding in a minimal DecisionRecord (findings.enforcing) and
	// validate against the frozen DecisionRecord schema. points is stamped 0
	// (schema requires it >=0); S04 owns real points provenance.
	for i, f := range all {
		fj, _ := json.Marshal(f)
		var fm map[string]any
		if err := json.Unmarshal(fj, &fm); err != nil {
			t.Fatal(err)
		}
		if _, ok := fm["points"]; !ok {
			fm["points"] = 0
		}
		rec := map[string]any{
			"apiVersion": "assent.dev/v1alpha1",
			"kind":       "DecisionRecord",
			"decision":   "REVIEW",
			"findings":   map[string]any{"observed": []any{}, "enforcing": []any{fm}},
			"pins": map[string]any{
				"toolVersion": "0.0.0-dev", "toolDigest": "sha256:t",
				"policySha": "p", "sourceSha": "s", "targetSha": "tg",
				"mergeResultDigest": "sha256:m", "factsResolvedAt": map[string]any{},
			},
		}
		raw, _ := json.Marshal(rec)
		parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := schemas.DecisionRecordSchema.Validate(parsed); err != nil {
			t.Fatalf("finding %d (%+v) does not satisfy the DecisionRecord finding schema: %v", i, f, err)
		}
	}
}

// Constraint (b) verification: the change projection the aggregator reasons over
// (subject = "file:"+File) validates against the frozen evaluation-input schema.
// This proves the canonical-string old/new + synthesized subject shape is
// schema-valid (the same projection the S01 adapter marshals).
func TestChangeProjectionValidatesAgainstSchema(t *testing.T) {
	cs := oneChange()
	// Build the schema-shaped changeSet the same way the aggregator/adapter do.
	changes := make([]map[string]any, len(cs.Changes))
	for i, c := range cs.Changes {
		changes[i] = map[string]any{
			"subject": "file:" + c.File,
			"file":    c.File,
			"path":    c.Path,
			"kind":    string(c.Kind),
			"old":     c.Old,
			"new":     c.New,
		}
	}
	doc := map[string]any{
		"apiVersion": "assent.dev/v1alpha1",
		"kind":       "EvaluationInput",
		"changeSet":  map[string]any{"changes": changes},
		"facts":      map[string]any{},
		"mr": map[string]any{
			"author": "alice", "sourceBranch": "feature", "targetBranch": "main",
		},
		"require": []any{"non-destructive"},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if err := schemas.EvaluationInputSchema.Validate(parsed); err != nil {
		t.Fatalf("canonical-string change projection must validate against the frozen schema: %v\ndoc: %s", err, raw)
	}
}
