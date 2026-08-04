package compare

import (
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// TestLoadBundleRejectsMalformed covers LoadBundle's fail paths: non-JSON input
// and a structurally-invalid bundle (missing the required evaluationInput) are
// both located rejections, never a nil-but-nil-error pass.
func TestLoadBundleRejectsMalformed(t *testing.T) {
	if _, err := LoadBundle([]byte("{not json")); err == nil {
		t.Fatal("malformed JSON must be rejected")
	}
	// Valid JSON but not a ReplayBundle (missing evaluationInput/pins/kind).
	if _, err := LoadBundle([]byte(`{"apiVersion":"assent.dev/v1alpha1","kind":"ReplayBundle"}`)); err == nil {
		t.Fatal("a bundle missing evaluationInput must be rejected by the frozen schema")
	}
}

// TestCompareNilInput guards the nil-EvaluationInput path.
func TestCompareNilInput(t *testing.T) {
	p := mkProfile("p", "true", "", policy.EffectBlock)
	if _, err := Compare(nil, p, p); err == nil {
		t.Fatal("nil evaluation input must error")
	}
}

// TestCompareEvalErrorSurfaces: a profile with a nil policy makes CoverWithProfile
// error, and Compare surfaces it wrapped (never a silent classification).
func TestCompareEvalErrorSurfaces(t *testing.T) {
	in := loadBundle(t)
	good := mkProfile("good", "true", "", policy.EffectBlock)
	bad := Profile{Name: "bad", Policy: nil, Bind: good.Bind, Ceiling: policy.PhaseEnforce}
	if _, err := Compare(in, bad, good); err == nil {
		t.Fatal("a nil-policy baseline must surface an evaluation error")
	}
	if _, err := Compare(in, good, bad); err == nil {
		t.Fatal("a nil-policy candidate must surface an evaluation error")
	}
}

// TestCompareIdenticalIsNoDelta: byte-identical profiles agree fully → zero Kind
// ("" = no delta) and a gate PASS (never a spurious explanation-only).
func TestCompareIdenticalIsNoDelta(t *testing.T) {
	in := loadBundle(t)
	p := mkProfile("p", "new >= old", "same message", policy.EffectBlock)
	got, err := Compare(in, p, p)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.Kind != "" {
		t.Fatalf("kind = %q, want empty (no delta) for byte-identical sides", got.Kind)
	}
	if got.Verdict != VerdictPass {
		t.Fatalf("verdict = %q, want PASS", got.Verdict)
	}
}

// TestClassifyEqualDecisionDifferingIdentitiesFailsClosed: the same decision with
// DIFFERENT finding identities (not a mere wording change) is a real finding-level
// semantic delta the seed does not own → fail-closed, never explanation-only.
func TestClassifyEqualDecisionDifferingIdentitiesFailsClosed(t *testing.T) {
	baseline := aggregate.Result{
		Decision: aggregate.DecisionBlock,
		Findings: []aggregate.Finding{{Rule: "a", Effect: aggregate.EffectBlock, Subject: "s", Code: "c1"}},
	}
	candidate := aggregate.Result{
		Decision: aggregate.DecisionBlock,
		Findings: []aggregate.Finding{{Rule: "b", Effect: aggregate.EffectBlock, Subject: "s", Code: "c2"}},
	}
	if _, err := classify(baseline, candidate); !errors.Is(err, ErrUnclassifiable) {
		t.Fatalf("differing finding identities at equal decision must fail closed, got %v", err)
	}
}
