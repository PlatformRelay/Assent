package aggregate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

const d016Dir = "../../../examples/contracts/d016-strict-fixture"

func readD016(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(d016Dir, name)) //nolint:gosec // hardcoded fixture path
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

// loadD016 loads the three frozen D-016 inputs via the E2-S01 loaders.
func loadD016(t *testing.T) (*policy.MergePolicy, *policy.Binding, *EvaluationInput) {
	t.Helper()
	mp, err := policy.LoadMergePolicy(readD016(t, "merge-policy.json"))
	if err != nil {
		t.Fatalf("LoadMergePolicy: %v", err)
	}
	rb, err := policy.LoadRulesetBinding(readD016(t, "ruleset-binding.json"))
	if err != nil {
		t.Fatalf("LoadRulesetBinding: %v", err)
	}
	if len(rb.Bindings) == 0 {
		t.Fatal("ruleset-binding has no bindings")
	}
	in, err := LoadEvaluationInput(readD016(t, "evaluation-input.json"))
	if err != nil {
		t.Fatalf("LoadEvaluationInput: %v", err)
	}
	return mp, &rb.Bindings[0], in
}

// TestObligationCoveragePerSubject is the D-016 golden: with require:
// [ownership, non-destructive] and two governed subjects, ownership is unproven
// for BOTH subjects (expired fact) and non-destructive fails for the ONE subject
// whose /partitions shrank — the exact three enforcing findings of the golden
// decision-record, subject-scoped, decision BLOCK. (REQ-E2-S04-01)
func TestObligationCoveragePerSubject(t *testing.T) {
	mp, bind, in := loadD016(t)

	got, err := Cover(mp, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}

	// F5 note: non-destructive is a COVERED obligation (an enforce rule proves it),
	// but its valueChanges /partitions-modify match selects only the orders
	// subject, so payments gets NO non-destructive finding — covered-but-silent,
	// NOT uncovered. That is why there are exactly three findings, not four.
	want := []Finding{
		{Rule: "topic-owner-must-approve", Obligation: "ownership", Effect: EffectRequireReview, Subject: "topic-registry:orders.events.v1", Points: 0, Code: "ownership-approval-missing"},
		{Rule: "partitions-must-not-shrink", Obligation: "non-destructive", Effect: EffectBlock, Subject: "topic-registry:orders.events.v1", Points: 10, Code: "partition-count-shrunk"},
		{Rule: "topic-owner-must-approve", Obligation: "ownership", Effect: EffectRequireReview, Subject: "topic-registry:payments.settled.v2", Points: 0, Code: "ownership-approval-missing"},
	}
	// The golden's enforcing findings, canonically sorted the same way Cover sorts.
	sortFindings(want)

	if got.Decision != DecisionBlock {
		t.Errorf("decision = %q, want BLOCK", got.Decision)
	}
	if len(got.Findings) != len(want) {
		t.Fatalf("got %d findings, want %d: %+v", len(got.Findings), len(want), got.Findings)
	}
	for i := range want {
		if got.Findings[i] != want[i] {
			t.Errorf("finding[%d] = %+v, want %+v", i, got.Findings[i], want[i])
		}
	}
}

// TestGoldenDecisionRecordMatch cross-checks Cover's findings against the frozen
// decision-record.json's `enforcing` array byte-for-byte (the golden is the
// source of truth; this guards against the two drifting).
func TestGoldenDecisionRecordMatch(t *testing.T) {
	mp, bind, in := loadD016(t)
	got, err := Cover(mp, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}

	var record struct {
		Decision string `json:"decision"`
		Findings struct {
			Enforcing []Finding `json:"enforcing"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(readD016(t, "decision-record.json"), &record); err != nil {
		t.Fatalf("unmarshal decision-record: %v", err)
	}
	if string(got.Decision) != record.Decision {
		t.Errorf("decision = %q, want %q", got.Decision, record.Decision)
	}
	want := record.Findings.Enforcing
	sortFindings(want)
	if len(got.Findings) != len(want) {
		t.Fatalf("got %d findings, golden has %d", len(got.Findings), len(want))
	}
	for i := range want {
		if got.Findings[i] != want[i] {
			t.Errorf("finding[%d] = %+v, golden %+v", i, got.Findings[i], want[i])
		}
	}
}

// TestUncoveredObligationFailsSafe — a require[] obligation with NO enforce-phase
// proving rule can never be proven; coverage emits an uncovered fail-safe finding
// and the run can never APPROVE. (REQ-E2-S04-02)
func TestUncoveredObligationFailsSafeCoverage(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "some-rule",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/x"}, Kinds: []string{"modify"}}},
				Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "x-shrunk"},
			}},
		},
	}
	// require ownership, but only non-destructive is proven -> ownership uncovered.
	bind := &policy.Binding{Require: []string{"ownership"}, Environment: "prod"}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "f.yaml", Path: "/x", Kind: "modify", Old: intNum(3), New: intNum(6)},
	}}}

	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if got.Decision == DecisionApprove {
		t.Fatal("uncovered obligation must never APPROVE")
	}
	if len(got.Findings) != 1 || got.Findings[0].Rule != ruleUncovered || got.Findings[0].Obligation != "ownership" {
		t.Fatalf("want one uncovered ownership finding, got %+v", got.Findings)
	}
}

// TestNoAnyOfObligationComposition — an authored alternative-proof composition
// (an `anyOf` key in a rule) is rejected at load by the frozen strict schema; AND
// -only conjunction is the only obligation semantics. (REQ-E2-S04-03)
func TestNoAnyOfObligationComposition(t *testing.T) {
	raw := []byte(`{
	  "apiVersion": "assent.dev/v1alpha1",
	  "kind": "MergePolicy",
	  "metadata": {"name": "bad"},
	  "spec": {
	    "entries": {},
	    "rules": [
	      {
	        "name": "either-owner-or-quota",
	        "match": {"files": {"paths": ["**.yaml"]}},
	        "anyOf": [
	          {"prove": {"obligation": "ownership", "when": "true"}},
	          {"prove": {"obligation": "quota", "when": "true"}}
	        ],
	        "phase": "enforce"
	      }
	    ]
	  }
	}`)
	_, err := policy.LoadMergePolicy(raw)
	if err == nil {
		t.Fatal("an anyOf alternative-proof composition must be rejected at load, not accepted")
	}
	// Reject for the RIGHT reason: the frozen schema refuses the `anyOf`
	// composition key (additionalProperties:false), not a generic parse typo.
	if !strings.Contains(err.Error(), "anyOf") {
		t.Errorf("rejection must name the anyOf composition, got: %v", err)
	}
}

// TestFullyCoveredEmitsNoObligationFinding — every require[] obligation proven
// true for every governed subject contributes NO finding and does not by itself
// prevent APPROVE. (REQ-E2-S04-04)
func TestFullyCoveredEmitsNoObligationFinding(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "partitions-grow",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
				Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "shrunk"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "a.yaml", Path: "/partitions", Kind: "modify", Old: intNum(3), New: intNum(6)},
		{Subject: "s:2", File: "b.yaml", Path: "/partitions", Kind: "modify", Old: intNum(4), New: intNum(4)},
	}}}

	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if len(got.Findings) != 0 {
		t.Errorf("fully-covered MR must emit no finding, got %+v", got.Findings)
	}
	if got.Decision != DecisionApprove {
		t.Errorf("fully-covered coverage must not lower APPROVE, got %q", got.Decision)
	}
}

// TestCoverageOrderIndependent — shuffling the rule order or the change order
// yields a byte-identical Result after the canonical sort. (REQ-E2-S04-05)
func TestCoverageOrderIndependent(t *testing.T) {
	mp, bind, in := loadD016(t)

	base, err := Cover(mp, bind, in)
	if err != nil {
		t.Fatalf("Cover base: %v", err)
	}
	baseJSON, _ := json.Marshal(base)

	// Reverse both the rules and the changes.
	shuffled := *mp
	shuffled.Spec.Rules = reversedRules(mp.Spec.Rules)
	shIn := *in
	shIn.ChangeSet.Changes = reversedChanges(in.ChangeSet.Changes)

	got, err := Cover(&shuffled, bind, &shIn)
	if err != nil {
		t.Fatalf("Cover shuffled: %v", err)
	}
	gotJSON, _ := json.Marshal(got)
	if string(baseJSON) != string(gotJSON) {
		t.Errorf("coverage not order-independent:\n base=%s\n got =%s", baseJSON, gotJSON)
	}

	// Double-run stability: the same input twice is byte-identical.
	again, _ := Cover(mp, bind, in)
	againJSON, _ := json.Marshal(again)
	if string(baseJSON) != string(againJSON) {
		t.Errorf("coverage not double-run stable:\n a=%s\n b=%s", baseJSON, againJSON)
	}
}

// TestCoverPredicateErrorFailsSafe — a leaf that errors (references an absent
// fact value) makes the (rule, subject) unproven -> require-review, never APPROVE.
func TestCoverPredicateErrorFailsSafe(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:  "quota-not-exceeded",
				Phase: policy.PhaseEnforce,
				Match: policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
				// facts.quota.max.value is absent -> eval error -> fail-safe.
				Prove:     &policy.Prove{Obligation: "quota", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "int(new) <= facts.quota.max.value"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "over-quota"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"quota"}, Environment: "prod"}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "a.yaml", Path: "/partitions", Kind: "modify", Old: intNum(3), New: intNum(6)},
	}}}

	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if got.Decision == DecisionApprove {
		t.Fatal("an erroring predicate must never APPROVE")
	}
	if len(got.Findings) != 1 || got.Findings[0].Effect != EffectRequireReview || got.Findings[0].Code != "predicate.error" {
		t.Fatalf("want one predicate.error require-review finding, got %+v", got.Findings)
	}
}

// TestCoverTreeWhenEvaluates — an all/any/not assert tree is now walked by the
// E2-S03 combinator walker through Cover (superseding the S04 fail-safe stub that
// mapped any tree to predicate.error). A tree whose second conjunct fails yields
// the rule's onFailure effect (NOT predicate.error), proving the walker is wired
// into the coverage loop; a tree that holds proves the obligation (no finding).
func TestCoverTreeWhenEvaluates(t *testing.T) {
	mkPol := func() *policy.MergePolicy {
		return &policy.MergePolicy{
			Spec: policy.MergePolicySpec{
				Rules: []policy.Rule{{
					Name:  "tree-rule",
					Phase: policy.PhaseEnforce,
					Match: policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
					Prove: &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{All: []policy.AssertTree{
						{Leaf: &policy.Leaf{CEL: "kind == 'modify'"}},
						{Leaf: &policy.Leaf{CEL: "new >= old", Message: "must not shrink"}},
					}}},
					OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "shrunk"},
				}},
			},
		}
	}
	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}

	// Second conjunct fails (6 -> 3) -> clean-false -> block onFailure (NOT predicate.error).
	shrink := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "a.yaml", Path: "/x", Kind: "modify", Old: intNum(6), New: intNum(3)},
	}}}
	got, err := Cover(mkPol(), bind, shrink)
	if err != nil {
		t.Fatalf("Cover on a tree when: %v", err)
	}
	if got.Decision != DecisionBlock {
		t.Errorf("failing tree conjunct must BLOCK, got %q", got.Decision)
	}
	if len(got.Findings) != 1 || got.Findings[0].Code != "shrunk" || got.Findings[0].Effect != EffectBlock {
		t.Fatalf("want one block/shrunk finding from the walked tree, got %+v", got.Findings)
	}
	if got.Findings[0].Message != "must not shrink" {
		t.Errorf("finding must carry the failing leaf's message, got %q", got.Findings[0].Message)
	}

	// Both conjuncts hold (3 -> 6) -> obligation proven -> no finding, APPROVE.
	grow := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "a.yaml", Path: "/x", Kind: "modify", Old: intNum(3), New: intNum(6)},
	}}}
	got, err = Cover(mkPol(), bind, grow)
	if err != nil {
		t.Fatalf("Cover on a satisfied tree: %v", err)
	}
	if got.Decision != DecisionApprove || len(got.Findings) != 0 {
		t.Errorf("a satisfied tree must prove the obligation (APPROVE, no finding), got %q %+v", got.Decision, got.Findings)
	}
}

// TestCoverValuesMatch — the Values (implicit-modify) match domain selects a
// change by pointer membership.
func TestCoverValuesMatch(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "retention-monotonic",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"/retentionMs"}}},
				Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "retention-shrunk"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "a.yaml", Path: "/retentionMs", Kind: "modify", Old: intNum(10), New: intNum(5)},
		{Subject: "s:1", File: "a.yaml", Path: "/other", Kind: "modify", Old: intNum(1), New: intNum(2)},
	}}}

	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if len(got.Findings) != 1 || got.Findings[0].Code != "retention-shrunk" {
		t.Fatalf("want one retention-shrunk block finding, got %+v", got.Findings)
	}
	if got.Decision != DecisionBlock {
		t.Errorf("decision = %q, want BLOCK", got.Decision)
	}
}

// TestCoverWildcardPointerNeverApproves is the F1 regression: valueChanges
// pointers are GLOBS (the schema imposes no pattern, and E1/glob treat them as
// globs), so a wildcard pointer rule must match a violating change and NEVER
// fail-open to APPROVE via exact-membership. A rule on "/config/*/enabled" over a
// modify at "/config/db/enabled" that shrinks (new < old) must BLOCK.
func TestCoverWildcardPointerNeverApproves(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "config-monotonic",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/config/*/enabled"}, Kinds: []string{"modify"}}},
				Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "config-shrunk"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "svc.yaml", Path: "/config/db/enabled", Kind: "modify", Old: intNum(10), New: intNum(5)},
	}}}

	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if got.Decision == DecisionApprove {
		t.Fatal("a wildcard-pointer rule over a violating change must never APPROVE (fail-open)")
	}
	if len(got.Findings) != 1 || got.Findings[0].Code != "config-shrunk" || got.Findings[0].Effect != EffectBlock {
		t.Fatalf("want one config-shrunk block finding, got %+v", got.Findings)
	}
}

// TestCoverValuesPathsFileGlob is the F2 regression: the Values domain's `paths`
// are FILE globs (over ch.File), AND-narrowing with `pointers` (over ch.Path) —
// not an OR-widening exact compare against the pointer. A change in a
// non-matching file must be excluded even when its pointer matches.
func TestCoverValuesPathsFileGlob(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "prod-retention-monotonic",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{Values: &policy.ValuesMatch{Paths: []string{"topics/prod/**.yaml"}, Pointers: []string{"/retentionMs"}}},
				Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "retention-shrunk"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		// prod file, matching pointer, shrinks -> selected -> BLOCK.
		{Subject: "s:prod", File: "topics/prod/a.yaml", Path: "/retentionMs", Kind: "modify", Old: intNum(10), New: intNum(5)},
		// staging file (paths glob excludes it), same pointer + shrink -> NOT selected.
		{Subject: "s:stg", File: "topics/staging/b.yaml", Path: "/retentionMs", Kind: "modify", Old: intNum(10), New: intNum(5)},
	}}}

	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if len(got.Findings) != 1 {
		t.Fatalf("want exactly one finding (staging file excluded by paths glob), got %+v", got.Findings)
	}
	if got.Findings[0].Subject != "s:prod" || got.Findings[0].Code != "retention-shrunk" {
		t.Errorf("finding must be the prod subject only, got %+v", got.Findings[0])
	}
}

// TestCoverNilOnFailureFailsSafe is the F4 regression: a hand-built prove rule
// whose When is cleanly false but that carries no OnFailure is a shape error —
// Cover must fail SAFE to require-review, never panic and never APPROVE.
func TestCoverNilOnFailureFailsSafe(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:  "no-onfailure",
				Phase: policy.PhaseEnforce,
				Match: policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
				Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "false"}}},
				// OnFailure deliberately nil.
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"ownership"}, Environment: "prod"}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: "s:1", File: "a.yaml", Path: "/x", Kind: "modify", Old: intNum(1), New: intNum(2)},
	}}}

	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if got.Decision == DecisionApprove {
		t.Fatal("a clean-false rule with nil OnFailure must never APPROVE")
	}
	if len(got.Findings) != 1 || got.Findings[0].Effect != EffectRequireReview || got.Findings[0].Code != "policy.shape-error" {
		t.Fatalf("want one policy.shape-error require-review finding, got %+v", got.Findings)
	}
}

// TestMatchChangesFileEventsDisjoint — REQ-EFE-S01-03. The fileEvents matcher
// selects a whole-file (path=="") add/delete iff kind ∈ kinds AND the file glob
// matches; and domain disjointness holds BOTH ways — a value-glob domain never
// selects a path=="" file-event, and fileEvents never selects a path!="" value
// change. The value-domain guards are guard-DEPENDENT here (glob "**" matches ""):
// absent the ch.Path!="" guard these cases would leak.
func TestMatchChangesFileEventsDisjoint(t *testing.T) {
	fileAdd := EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "add"}
	fileDelete := EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "delete"}
	fileModify := EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "modify"}
	valueAdd := EvalChange{Subject: "topics/orders.yaml", File: "topics/orders.yaml", Path: "/partitions", Kind: "add"}

	feAddDelete := policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"topics/*.yaml"}, Kinds: []string{"add", "delete"}}}
	feDeleteOnly := policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"topics/*.yaml"}, Kinds: []string{"delete"}}}
	feWrongGlob := policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"schemas/*.yaml"}, Kinds: []string{"add", "delete"}}}
	files := policy.Match{Files: &policy.FilesMatch{Paths: []string{"topics/*.yaml"}}}
	values := policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"**"}}}
	valueChanges := policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"**"}, Kinds: []string{"add"}}}

	cases := []struct {
		name    string
		m       policy.Match
		changes []EvalChange
		want    int
	}{
		{"fileEvents selects the whole-file add", feAddDelete, []EvalChange{fileAdd}, 1},
		{"fileEvents selects the whole-file delete", feAddDelete, []EvalChange{fileDelete}, 1},
		{"fileEvents kind not in kinds -> no select", feDeleteOnly, []EvalChange{fileAdd}, 0},
		{"fileEvents glob does not match -> no select", feWrongGlob, []EvalChange{fileAdd}, 0},
		// disjoint direction 1: fileEvents never selects a value-level (path!="") change.
		{"fileEvents does not select a value-level add", feAddDelete, []EvalChange{valueAdd}, 0},
		// disjoint direction 2: value domains never select a path=="" file-event
		// (guard-dependent — files ignores path; values/valueChanges globs match "").
		{"files does not select a path=='' add", files, []EvalChange{fileAdd}, 0},
		{"files does not select a path=='' delete", files, []EvalChange{fileDelete}, 0},
		{"values does not select a path=='' modify", values, []EvalChange{fileModify}, 0},
		{"valueChanges does not select a path=='' add", valueChanges, []EvalChange{fileAdd}, 0},
		// sanity: the value domains DO still select their value-level change.
		{"files still selects a value-level change", files, []EvalChange{valueAdd}, 1},
		{"valueChanges still selects a value-level add", valueChanges, []EvalChange{valueAdd}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := matchChanges(tc.m, tc.changes)
			if err != nil {
				t.Fatalf("matchChanges: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("selected %d, want %d (%+v)", len(got), tc.want, got)
			}
		})
	}
}

// TestCoverFileEventsEndToEnd — REQ-EFE-S01-05. Driving Cover with a fileEvents
// rule over a hand-built whole-file EvalChange proves the domain through the full
// decision path: a whole-file ADD proves the obligation -> APPROVE; a whole-file
// DELETE fails the obligation fail-safe -> REVIEW; a `when` that reads new on a
// delete errors -> REVIEW (fail-safe). This also confirms bindLeafActivation
// binds a file-event (nil old/new/entry, path=="", string kind/file) unchanged.
func TestCoverFileEventsEndToEnd(t *testing.T) {
	// prove.when: kind != "delete" proves the obligation for an add, fails for a delete.
	polKind := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "topic-lifecycle",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"topics/*.yaml"}, Kinds: []string{"add", "delete"}}},
				Prove:     &policy.Prove{Obligation: "reviewed-deletion", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: `kind != "delete"`}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "topic.deletion-needs-review"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"reviewed-deletion"}, Environment: "prod"}
	subject := "file:topics/orders.yaml"

	addIn := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: subject, File: "topics/orders.yaml", Path: "", Kind: "add"},
	}}}
	got, err := Cover(polKind, bind, addIn)
	if err != nil {
		t.Fatalf("Cover(add): %v", err)
	}
	if got.Decision != DecisionApprove {
		t.Fatalf("whole-file add proves reviewed-deletion -> want APPROVE, got %q (%+v)", got.Decision, got.Findings)
	}

	delIn := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
		{Subject: subject, File: "topics/orders.yaml", Path: "", Kind: "delete"},
	}}}
	got, err = Cover(polKind, bind, delIn)
	if err != nil {
		t.Fatalf("Cover(delete): %v", err)
	}
	if got.Decision != DecisionReview {
		t.Fatalf("whole-file delete must fail safe -> want REVIEW, got %q (%+v)", got.Decision, got.Findings)
	}
	if len(got.Findings) != 1 || got.Findings[0].Effect != EffectRequireReview || got.Findings[0].Subject != subject {
		t.Fatalf("want one require-review finding on %q, got %+v", subject, got.Findings)
	}

	// A `when` that reads `new` on a delete (new is nil) errors -> predicate.error
	// -> REVIEW (fail-safe) — proving the existing binding binds a file-event's nil
	// old/new/entry with no evaluate.go change.
	polReadsNew := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "topic-lifecycle-reads-new",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"topics/*.yaml"}, Kinds: []string{"delete"}}},
				Prove:     &policy.Prove{Obligation: "reviewed-deletion", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: `new.enabled == true`}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "unreached"},
			}},
		},
	}
	got, err = Cover(polReadsNew, bind, delIn)
	if err != nil {
		t.Fatalf("Cover(reads-new): %v", err)
	}
	if got.Decision != DecisionReview {
		t.Fatalf("reading new on a delete must error -> REVIEW (fail-safe), got %q (%+v)", got.Decision, got.Findings)
	}
	if len(got.Findings) != 1 || got.Findings[0].Code != "predicate.error" {
		t.Fatalf("want one predicate.error require-review finding, got %+v", got.Findings)
	}
}

// TestCoverEmptyMatchRejected — a rule whose match declares no supported domain
// is a defect; Cover errors rather than fail-open to "matches nothing".
func TestCoverEmptyMatchRejected(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "empty",
				Phase:     policy.PhaseEnforce,
				Match:     policy.Match{},
				Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "true"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "c"},
			}},
		},
	}
	bind := &policy.Binding{Require: []string{"ownership"}}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{{Subject: "s:1", File: "a.yaml"}}}}
	if _, err := Cover(pol, bind, in); err == nil {
		t.Fatal("a rule with no match domain must be rejected by Cover")
	}
}

// TestCoverObservePhaseIgnored — an observe/off phase rule does NOT feed the
// decision (only enforce does); its obligation still counts as covered so no
// uncovered finding fires for it.
func TestCoverObservePhaseIgnored(t *testing.T) {
	pol := &policy.MergePolicy{
		Spec: policy.MergePolicySpec{
			Rules: []policy.Rule{{
				Name:      "observed-only",
				Phase:     policy.PhaseObserve,
				Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"**.yaml"}}},
				Prove:     &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "false"}}},
				OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "c"},
			}},
		},
	}
	// No enforce rule proves ownership -> ownership is uncovered (fail-safe),
	// and the observe rule's clean-false does NOT contribute a block finding.
	bind := &policy.Binding{Require: []string{"ownership"}}
	in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{{Subject: "s:1", File: "a.yaml"}}}}
	got, err := Cover(pol, bind, in)
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if got.Decision == DecisionBlock {
		t.Error("an observe-phase rule must not drive a BLOCK")
	}
	if len(got.Findings) != 1 || got.Findings[0].Rule != ruleUncovered {
		t.Fatalf("want one uncovered finding (observe rule ignored), got %+v", got.Findings)
	}
}

// TestCoverNilArgs — nil policy/binding/input is a programmer error, rejected.
func TestCoverNilArgs(t *testing.T) {
	if _, err := Cover(nil, nil, nil); err == nil {
		t.Fatal("Cover(nil,nil,nil) must error")
	}
}

// intNum mirrors the json.Number decoding LoadEvaluationInput produces so a
// hand-built EvalChange compares numerically through toCEL, not lexically.
func intNum(i int) any { return json.Number(itoa(i)) }

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func reversedRules(in []policy.Rule) []policy.Rule {
	out := make([]policy.Rule, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}

func reversedChanges(in []EvalChange) []EvalChange {
	out := make([]EvalChange, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}
