package aggregate

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// TestUnmatchedFileDeleteFailsSafeReview (REQ-EFE-S02-04, Judgment call (a) / D-063,
// D-064) pins the LOAD-BEARING fail-safe default: a whole-file DELETE event
// (path=="", kind==delete) that NO evaluated fileEvents rule governs escalates the
// decision to at-least-REVIEW — NEVER APPROVE — so a destructive delete no rule
// covers can never silently ship. The escalation is DELETE-only (an add is
// non-destructive, D-063) and GOVERNED-aware (a delete a fileEvents rule actually
// selects is NOT escalated — otherwise "blanket-REVIEW every delete" would pass and
// make fileEvents rules pointless).
func TestUnmatchedFileDeleteFailsSafeReview(t *testing.T) {
	del := EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "delete"}
	delIn := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{del}}}

	feRule := func(paths []string, phase policy.Phase, when string) policy.Rule {
		return policy.Rule{
			Name:      "topic-lifecycle",
			Phase:     phase,
			Match:     policy.Match{FileEvents: &policy.FileEventsMatch{Paths: paths, Kinds: []string{"add", "delete"}}},
			Prove:     &policy.Prove{Obligation: "reviewed-deletion", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: when}}},
			OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "topic.deletion-needs-review"},
		}
	}
	pol := func(r policy.Rule) *policy.MergePolicy {
		return &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{r}}}
	}
	// require EMPTY on purpose: absent the escalation the delete would earn APPROVE
	// (the covered-obligation guard never fires), so a passing REVIEW here can only
	// come from the unmatched-delete escalation, not an unrelated fail-safe.
	bind := &policy.Binding{Environment: "prod"}

	hasUnmatchedFinding := func(fs []Finding) bool {
		for _, f := range fs {
			if f.Rule == ruleUnmatchedDelete {
				return true
			}
		}
		return false
	}

	t.Run("no fileEvents rule governs the delete (wrong glob) -> REVIEW", func(t *testing.T) {
		got, err := Cover(pol(feRule([]string{"schemas/*.yaml"}, policy.PhaseEnforce, `kind != "delete"`)), bind, delIn)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionReview {
			t.Fatalf("unmatched whole-file delete must fail safe -> REVIEW, got %q (%+v)", got.Decision, got.Findings)
		}
		if len(got.Findings) != 1 {
			t.Fatalf("want exactly one escalation finding, got %+v", got.Findings)
		}
		f := got.Findings[0]
		if f.Rule != ruleUnmatchedDelete || f.Effect != EffectRequireReview || f.Subject != del.Subject || f.Code != "fileEvent.unmatchedDelete" {
			t.Fatalf("escalation finding shape wrong: %+v", f)
		}
	})

	t.Run("no rules at all -> REVIEW", func(t *testing.T) {
		got, err := Cover(&policy.MergePolicy{}, bind, delIn)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionReview || !hasUnmatchedFinding(got.Findings) {
			t.Fatalf("a delete with no governing rule must escalate to REVIEW, got %q (%+v)", got.Decision, got.Findings)
		}
	})

	t.Run("an OFF-phase fileEvents rule does NOT govern -> REVIEW", func(t *testing.T) {
		got, err := Cover(pol(feRule([]string{"topics/*.yaml"}, policy.PhaseOff, `kind != "delete"`)), bind, delIn)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionReview || !hasUnmatchedFinding(got.Findings) {
			t.Fatalf("an off (disabled) fileEvents rule must not suppress the escalation, got %q (%+v)", got.Decision, got.Findings)
		}
	})

	// OBSERVE must NOT suppress the escalation: observe findings are structurally
	// excluded from the decision (coverage.go), so treating observe as "governed"
	// would suppress escalation → APPROVE — a D-063 fail-OPEN (P0 catch on EFE-S02).
	t.Run("an OBSERVE-phase fileEvents rule does NOT govern -> REVIEW", func(t *testing.T) {
		got, err := Cover(pol(feRule([]string{"topics/*.yaml"}, policy.PhaseObserve, `kind == "delete"`)), bind, delIn)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionReview || !hasUnmatchedFinding(got.Findings) {
			t.Fatalf("an observe-phase fileEvents rule must not suppress the unmatched-delete escalation (findings are decision-excluded), got %q findings=%+v observed=%+v", got.Decision, got.Findings, got.Observed)
		}
	})

	// Pack ceiling observe caps an enforce rule to observe — same fail-open if the
	// escalation treats effective PhaseObserve as governing. Mirror covered[]:
	// only effective PhaseEnforce may suppress.
	t.Run("pack-ceiling OBSERVE does NOT govern an enforce fileEvents rule -> REVIEW", func(t *testing.T) {
		got, err := CoverWithPhaseCeiling(pol(feRule([]string{"topics/*.yaml"}, policy.PhaseEnforce, `kind == "delete"`)), bind, delIn, nil, policy.PhaseObserve)
		if err != nil {
			t.Fatalf("CoverWithPhaseCeiling: %v", err)
		}
		if got.Decision != DecisionReview || !hasUnmatchedFinding(got.Findings) {
			t.Fatalf("pack-ceiling observe must not suppress the unmatched-delete escalation, got %q findings=%+v observed=%+v", got.Decision, got.Findings, got.Observed)
		}
	})

	t.Run("GOVERNED: a matched clean-true fileEvents prove rule -> APPROVE, no escalation", func(t *testing.T) {
		// prove.when kind == "delete" is clean-TRUE for this delete -> obligation proven
		// -> APPROVE; the delete IS governed, so the escalation must stay silent (pins
		// that the net checks GOVERNED, not merely is-a-delete).
		reqBind := &policy.Binding{Environment: "prod", Require: []string{"reviewed-deletion"}}
		got, err := Cover(pol(feRule([]string{"topics/*.yaml"}, policy.PhaseEnforce, `kind == "delete"`)), reqBind, delIn)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionApprove {
			t.Fatalf("a governed delete proven clean-true -> APPROVE, got %q (%+v)", got.Decision, got.Findings)
		}
		if hasUnmatchedFinding(got.Findings) {
			t.Fatalf("a governed delete must not emit the unmatched-delete escalation: %+v", got.Findings)
		}
	})

	t.Run("a value-level (path!=\"\") delete is NOT a whole-file event -> no escalation", func(t *testing.T) {
		valDel := EvalChange{Subject: "topics/orders.yaml", File: "topics/orders.yaml", Path: "/partitions", Kind: "delete"}
		in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{valDel}}}
		got, err := Cover(&policy.MergePolicy{}, bind, in)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionApprove || hasUnmatchedFinding(got.Findings) {
			t.Fatalf("a value-level delete must not trigger the whole-file escalation, got %q (%+v)", got.Decision, got.Findings)
		}
	})

	t.Run("a whole-file ADD unmatched does NOT escalate (adding isn't destructive, D-063)", func(t *testing.T) {
		add := EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "add"}
		in := &EvaluationInput{ChangeSet: ChangeSet{Changes: []EvalChange{add}}}
		got, err := Cover(&policy.MergePolicy{}, bind, in)
		if err != nil {
			t.Fatalf("Cover: %v", err)
		}
		if got.Decision != DecisionApprove || hasUnmatchedFinding(got.Findings) {
			t.Fatalf("an unmatched whole-file add must not escalate, got %q (%+v)", got.Decision, got.Findings)
		}
	})
}
