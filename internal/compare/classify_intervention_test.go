package compare

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// classify_intervention_test.go defends the intervention-effect predicates against
// the mutant the 2026-08-18 audit demonstrated survives every wired gate (TEST-02):
// deleting `|| e == aggregate.EffectChallenge` from isStricterInterventionEffect
// left `go test ./internal/compare/...` AND `task dogfood-comparison` green.
//
// The cases below are written so the mutant is reached, not short-circuited. classify
// resolves by priority (D-117): missed intervention > uncovered > newly-auto-mergeable
// > score-threshold > stricter-added. A challenge delta lands in the LAST slot, so the
// fixture must keep the four earlier detectors silent or the assertion is decorative:
//   - baseline has no findings          -> detectMissedIntervention cannot fire
//   - no `obligation.uncovered` codes   -> isObligationUncovered cannot fire
//   - candidate is REVIEW, not APPROVE  -> the newly-auto-mergeable branch is skipped
//   - intervention keys differ          -> isScoreThresholdChange cannot fire
//
// The baseline side must also stay free of challenge findings:
// interventionFindingsByIdentity consults the SAME predicate, so a mutated build
// would drop a baseline challenge out of baseByID and report a spurious `true`,
// masking the kill.

// REQ-AUD2-S04-01 / REQ-AUD2-S04-02: baseline APPROVE with no intervention, candidate
// adds a finding whose effect is `challenge` -> stricter-intervention-added. This is the
// named defence of the EffectChallenge term in isStricterInterventionEffect: deleting
// that term makes classify fail closed (ErrUnclassifiable) and this test red.
func TestClassifyStricterInterventionAddedChallengeEffect(t *testing.T) {
	baseline := aggregate.Result{
		Decision: aggregate.DecisionApprove,
		Findings: nil,
	}
	candidate := aggregate.Result{
		Decision: aggregate.DecisionReview,
		Findings: []aggregate.Finding{{
			Rule:       "retention-ack",
			Obligation: "retention-ack",
			Effect:     aggregate.EffectChallenge,
			Subject:    "topic-registry:orders.events.v1",
			Code:       "retention.shrunk",
		}},
	}

	got, err := classify(baseline, candidate)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if got != KindStricterInterventionAdded {
		t.Fatalf("kind = %q, want %q", got, KindStricterInterventionAdded)
	}
}

// REQ-AUD2-S04-01: the predicate itself, asserted directly so the mutant dies even if
// a future classify refactor reroutes the challenge delta to another taxonomy slot.
// isMissedInterventionEffect is asserted alongside it because the two predicates differ
// by exactly one term, and that difference is deliberate (a challenge is NOT a missed
// destructive/authorization intervention) — pinning both stops a "fix" that unifies them.
func TestInterventionEffectPredicates(t *testing.T) {
	stricter := map[aggregate.Effect]bool{
		aggregate.EffectBlock:         true,
		aggregate.EffectRequireReview: true,
		aggregate.EffectChallenge:     true,
		aggregate.EffectComment:       false,
	}
	for effect, want := range stricter {
		if got := isStricterInterventionEffect(effect); got != want {
			t.Errorf("isStricterInterventionEffect(%q) = %v, want %v", effect, got, want)
		}
	}

	missed := map[aggregate.Effect]bool{
		aggregate.EffectBlock:         true,
		aggregate.EffectRequireReview: true,
		aggregate.EffectChallenge:     false,
		aggregate.EffectComment:       false,
	}
	for effect, want := range missed {
		if got := isMissedInterventionEffect(effect); got != want {
			t.Errorf("isMissedInterventionEffect(%q) = %v, want %v", effect, got, want)
		}
	}
}

// REQ-AUD2-S04-01: a challenge intervention the baseline ALREADY carries on the same
// delta identity is not "added". This pins the interventionFindingsByIdentity half of
// the predicate's use — the half a lone stricter-added assertion leaves untested, and
// the half through which a mutated build could otherwise answer correctly by accident.
func TestStricterInterventionAddedIgnoresPreexistingChallenge(t *testing.T) {
	finding := aggregate.Finding{
		Rule:       "retention-ack",
		Obligation: "retention-ack",
		Effect:     aggregate.EffectChallenge,
		Subject:    "topic-registry:orders.events.v1",
		Code:       "retention.shrunk",
	}
	baseline := aggregate.Result{
		Decision: aggregate.DecisionApprove,
		Findings: []aggregate.Finding{finding},
	}
	candidate := aggregate.Result{
		Decision: aggregate.DecisionReview,
		Findings: []aggregate.Finding{finding},
	}

	if detectStricterInterventionAdded(baseline, candidate) {
		t.Fatal("detectStricterInterventionAdded = true for a challenge the baseline already carried, want false")
	}
}
