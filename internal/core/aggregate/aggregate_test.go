package aggregate

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/schemas"
)

// liveOneChange is a single-entry EvaluationInput (partitions 6 -> 12).
func liveOneChange() *EvaluationInput {
	return &EvaluationInput{
		ChangeSet: ChangeSet{Changes: []EvalChange{{
			Subject: "file:topics/prod/orders.events.v1.yaml",
			File:    "topics/prod/orders.events.v1.yaml",
			Path:    "/partitions",
			Kind:    "modify",
			Old:     "6",
			New:     "12",
		}}},
		Facts:   map[string]map[string]Fact{},
		Require: []string{"o"},
	}
}

// livePolicyBinding is a minimal one-rule policy proving obligation "o" with the
// given `when` and onFailure effect.
func livePolicyBinding(when string, eff policy.Effect) (*policy.MergePolicy, *policy.Binding) {
	return &policy.MergePolicy{
			Spec: policy.MergePolicySpec{
				Rules: []policy.Rule{{
					Name:      "r",
					Phase:     policy.PhaseEnforce,
					Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**"}}},
					Prove:     &policy.Prove{Obligation: "o", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: when}}},
					OnFailure: &policy.OnFailure{Effect: eff, Code: "c"},
				}},
			},
		},
		&policy.Binding{Require: []string{"o"}}
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

// Every emitted Finding must satisfy the frozen DecisionRecord #/$defs/finding
// object (rule+subject minLength:1, closed effect enum) so the fail-safe paths
// serialize into a VALID DecisionRecord. Findings are collected from the live
// entry points: the guarded Decide (reserved-class BLOCK, opaque/empty REVIEW)
// and the coverage loop (unsatisfied effects, predicate error, uncovered
// obligation).
func TestFindingsValidateAgainstDecisionRecordSchema(t *testing.T) {
	var all []Finding
	collect := func(res Result) { all = append(all, res.Findings...) }

	in := liveOneChange()
	// Unsatisfied block + comment: `12 < 6` is false.
	pol, bind := livePolicyBinding("int(new) < int(old)", policy.EffectBlock)
	r, _ := Cover(pol, bind, in)
	collect(r)
	polC, bindC := livePolicyBinding("int(new) < int(old)", policy.EffectComment)
	r, _ = Cover(polC, bindC, in)
	collect(r)
	// Predicate error: `missing` is unbound.
	polE, bindE := livePolicyBinding("int(missing) > 0", policy.EffectBlock)
	r, _ = Cover(polE, bindE, in)
	collect(r)
	// Uncovered obligation: require names "o" but no rule proves it.
	polU := &policy.MergePolicy{}
	bindU := &policy.Binding{Require: []string{"o"}}
	r, _ = Cover(polU, bindU, in)
	collect(r)
	// Reserved-class BLOCK and opaque/empty REVIEW via the guarded entry.
	r, _ = Decide(DecideRequest{Subject: "file:.assent/x.yml", SubjectClass: ReservedPolicyClass, Policy: pol, Binding: bind, Input: in})
	collect(r)
	r, _ = Decide(DecideRequest{Subject: "file:x.yml", Opaque: true, Policy: pol, Binding: bind, Input: in})
	collect(r)
	r, _ = Decide(DecideRequest{Subject: "file:x.yml", Policy: pol, Binding: bind, Input: &EvaluationInput{ChangeSet: ChangeSet{}, Facts: map[string]map[string]Fact{}}})
	collect(r)

	if len(all) == 0 {
		t.Fatal("no findings collected")
	}

	// The run path fills empty finding subjects (sanitizeSubjects, N1) before
	// serializing the DecisionRecord; mimic it so the uncovered-obligation finding
	// (which cover emits with Subject:"") validates.
	for i := range all {
		if all[i].Subject != "" {
			continue
		}
		if all[i].Obligation != "" {
			all[i].Subject = "obligation:" + all[i].Obligation
		} else {
			all[i].Subject = "file:x"
		}
	}

	// Wrap each finding in a minimal DecisionRecord (findings.enforcing) and
	// validate against the frozen DecisionRecord schema. points is stamped 0
	// (schema requires it >=0).
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

// Constraint (b) verification: the change projection the engine reasons over
// (subject = "file:"+File) validates against the frozen evaluation-input schema.
func TestChangeProjectionValidatesAgainstSchema(t *testing.T) {
	cs := change.ChangeSet{Changes: []change.Change{{
		File: "topics/prod/orders.events.v1.yaml", Path: "/partitions", Kind: change.KindModify, Old: "6", New: "12",
	}}}
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
