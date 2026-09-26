package aggregate

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// TestUnmatchedEditFailsSafeReview (RVW-S02, report finding C-1) pins the
// load-bearing fail-safe default: a value-level EDIT (path!="") that NO
// enforce-phase prove rule selects, under a binding that requires an obligation
// some rule proves, escalates the decision to at-least-REVIEW — NEVER APPROVE —
// so a change no rule vouches for can never silently ship. Before this guard the
// reproduction below returned APPROVE with an EMPTY finding set: the required
// obligation was marked covered by a rule whose `match` selected none of the
// governed changes, so the uncovered-obligation guard never fired.
//
// Mirrors TestUnmatchedFileDeleteFailsSafeReview (D-063/D-064). The escalation is
// value-level-EDIT only (whole-file deletes are the D-064 guard's subject, adds
// are non-destructive), and GOVERNED-aware (a change an enforce prove rule
// actually selects is NOT escalated).
func TestUnmatchedEditFailsSafeReview(t *testing.T) {
	// The report's exact reproduction: the only rule proves non-destructive but
	// matches valueChanges pointers ["/partitions"]; the governed change is at
	// /replicas, so no rule selects it.
	partitionRule := func(phase policy.Phase) *policy.MergePolicy {
		return &policy.MergePolicy{
			Spec: policy.MergePolicySpec{
				Rules: []policy.Rule{{
					Name:      "partitions-must-not-shrink",
					Phase:     phase,
					Match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
					Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
					OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "partition-count-shrunk"},
				}},
			},
		}
	}
	replicas := EvalChange{Subject: "topic-registry:orders.events.v1", File: "topics/prod/orders-events.yaml", Path: "/replicas", Kind: "modify", Old: intNum(3), New: intNum(6)}
	replicasIn := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{replicas}}}
	// require non-destructive: some rule proves it, so it is "covered" and the
	// uncovered guard does NOT fire — a passing REVIEW can only come from the new
	// unmatched-edit escalation.
	bind := &policy.Binding{Require: []string{"non-destructive"}, Environment: "prod"}

	hasUnmatchedEdit := func(fs []Finding) bool {
		for _, f := range fs {
			if f.Rule == ruleUnmatchedEdit {
				return true
			}
		}
		return false
	}

	t.Run("REPRODUCTION: an unmatched value-level edit -> REVIEW, not APPROVE", func(t *testing.T) {
		got, err := Cover(partitionRule(policy.PhaseEnforce), bind, replicasIn)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionReview {
			t.Fatalf("an unmatched edit must fail safe -> REVIEW, got %q (%+v)", got.Decision, got.Findings)
		}
		if len(got.Findings) != 1 {
			t.Fatalf("want exactly one escalation finding, got %+v", got.Findings)
		}
		f := got.Findings[0]
		if f.Rule != ruleUnmatchedEdit || f.Effect != EffectRequireReview || f.Subject != replicas.Subject || f.Code != "change.unmatchedEdit" {
			t.Fatalf("escalation finding shape wrong: %+v", f)
		}
	})

	t.Run("an unmatched edit outside every rule's FILE glob -> REVIEW", func(t *testing.T) {
		// A file-glob rule over topics/** never selects services/x.yaml.
		pol := &policy.MergePolicy{
			Spec: policy.MergePolicySpec{
				Rules: []policy.Rule{{
					Name:      "topics-only",
					Phase:     policy.PhaseEnforce,
					Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"topics/**"}}},
					Prove:     &policy.Prove{Obligation: "non-destructive", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "new >= old"}}},
					OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "shrunk"},
				}},
			},
		}
		svc := EvalChange{Subject: "service:orders", File: "services/x.yaml", Path: "/replicas", Kind: "modify", Old: intNum(1), New: intNum(2)}
		got, err := Cover(pol, bind, &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{svc}}})
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionReview || !hasUnmatchedEdit(got.Findings) {
			t.Fatalf("an edit outside every file glob must escalate to REVIEW, got %q (%+v)", got.Decision, got.Findings)
		}
	})

	t.Run("GOVERNED: a matched clean-true edit -> APPROVE, no escalation", func(t *testing.T) {
		matched := EvalChange{Subject: "topic-registry:orders.events.v1", File: "topics/prod/orders-events.yaml", Path: "/partitions", Kind: "modify", Old: intNum(3), New: intNum(6)}
		got, err := Cover(partitionRule(policy.PhaseEnforce), bind, &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{matched}}})
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionApprove || hasUnmatchedEdit(got.Findings) {
			t.Fatalf("a matched, clean-true edit must prove the obligation -> APPROVE with no escalation, got %q (%+v)", got.Decision, got.Findings)
		}
	})

	t.Run("GOVERNED: a matched clean-FALSE edit fires the rule, not the escalation", func(t *testing.T) {
		matched := EvalChange{Subject: "topic-registry:orders.events.v1", File: "topics/prod/orders-events.yaml", Path: "/partitions", Kind: "modify", Old: intNum(6), New: intNum(3)}
		got, err := Cover(partitionRule(policy.PhaseEnforce), bind, &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{matched}}})
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionBlock || hasUnmatchedEdit(got.Findings) {
			t.Fatalf("a governed edit must earn its OWN rule effect (BLOCK), not the escalation, got %q (%+v)", got.Decision, got.Findings)
		}
		if len(got.Findings) != 1 || got.Findings[0].Code != "partition-count-shrunk" {
			t.Fatalf("want exactly the authored block finding, got %+v", got.Findings)
		}
	})

	// The gate: with NO required obligation proven by any rule there is no
	// obligation layer to vouch for, so the escalation stays silent. This preserves
	// the engine's documented vacuous-APPROVE reading for an empty require (D-184
	// owns the run-path refusal) and keeps the D-016 golden's semantics intact.
	t.Run("no require (no obligation layer) -> no escalation, APPROVE", func(t *testing.T) {
		got, err := Cover(partitionRule(policy.PhaseEnforce), &policy.Binding{Environment: "prod"}, replicasIn)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionApprove || hasUnmatchedEdit(got.Findings) {
			t.Fatalf("with no require the escalation must not fire, got %q (%+v)", got.Decision, got.Findings)
		}
	})

	t.Run("require with no proving rule -> uncovered guard, not the edit escalation", func(t *testing.T) {
		// The rule proves a NON-required obligation, so nothing the binding requires
		// is "covered" -> the escalation is gated off; the uncovered guard REVIEWs.
		pol := &policy.MergePolicy{
			Spec: policy.MergePolicySpec{
				Rules: []policy.Rule{{
					Name:      "unrelated",
					Phase:     policy.PhaseEnforce,
					Match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"topics/**"}}},
					Prove:     &policy.Prove{Obligation: "unrelated", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "true"}}},
					OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "c"},
				}},
			},
		}
		got, err := Cover(pol, bind, replicasIn)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionReview || hasUnmatchedEdit(got.Findings) {
			t.Fatalf("an uncovered require must REVIEW via the uncovered guard only, got %q (%+v)", got.Decision, got.Findings)
		}
		if len(got.Findings) != 1 || got.Findings[0].Rule != ruleUncovered {
			t.Fatalf("want exactly one uncovered finding, got %+v", got.Findings)
		}
	})

	// Scope: whole-file lifecycle events belong to the fileEvents domain. A delete
	// is the D-064 guard's subject (its own finding), an add is non-destructive.
	// An empty binding isolates the scope check from the obligation layer.
	noRequire := &policy.Binding{Environment: "prod"}
	t.Run("whole-file DELETE uses the D-064 guard, not the edit escalation", func(t *testing.T) {
		del := EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "delete"}
		got, err := Cover(&policy.MergePolicy{}, noRequire, &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{del}}})
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionReview || hasUnmatchedEdit(got.Findings) {
			t.Fatalf("a whole-file delete must escalate via D-064 only, got %q (%+v)", got.Decision, got.Findings)
		}
		if len(got.Findings) != 1 || got.Findings[0].Rule != ruleUnmatchedDelete {
			t.Fatalf("want exactly the D-064 delete finding, got %+v", got.Findings)
		}
	})

	t.Run("whole-file ADD is not an edit -> no escalation", func(t *testing.T) {
		add := EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "add"}
		got, err := Cover(&policy.MergePolicy{}, noRequire, &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{add}}})
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionApprove || hasUnmatchedEdit(got.Findings) {
			t.Fatalf("a whole-file add must not escalate (non-destructive, D-063), got %q (%+v)", got.Decision, got.Findings)
		}
	})

	// OBSERVE must NOT suppress the escalation: observe findings are structurally
	// excluded from the decision, so treating observe as "governed" would suppress
	// escalation → APPROVE — the same D-063 fail-open the delete guard documents.
	// The enforce rule covers the required obligation (so the gate opens) but does
	// not select the change; the observe rule selects it and must NOT govern.
	t.Run("an OBSERVE-phase rule does NOT govern -> REVIEW", func(t *testing.T) {
		pol := partitionRule(policy.PhaseEnforce)
		obs := partitionRule(policy.PhaseObserve).Spec.Rules[0]
		obs.Name = "observe-replicas"
		obs.Match = policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/replicas"}, Kinds: []string{"modify"}}}
		pol.Spec.Rules = append(pol.Spec.Rules, obs)
		got, err := Cover(pol, bind, replicasIn)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionReview || !hasUnmatchedEdit(got.Findings) {
			t.Fatalf("an observe-phase rule must not suppress the unmatched-edit escalation, got %q findings=%+v observed=%+v", got.Decision, got.Findings, got.Observed)
		}
	})

	// One finding per unvouched SUBJECT: a subject carrying several unmatched
	// changes must not emit duplicate identical findings.
	t.Run("several unmatched changes on one subject -> one finding", func(t *testing.T) {
		in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{
			{Subject: "s:1", File: "a.yaml", Path: "/replicas", Kind: "modify", Old: intNum(1), New: intNum(2)},
			{Subject: "s:1", File: "a.yaml", Path: "/shards", Kind: "modify", Old: intNum(1), New: intNum(2)},
		}}}
		got, err := Cover(partitionRule(policy.PhaseEnforce), bind, in)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		n := 0
		for _, f := range got.Findings {
			if f.Rule == ruleUnmatchedEdit {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("want one escalation finding per subject, got %d in %+v", n, got.Findings)
		}
	})
}
