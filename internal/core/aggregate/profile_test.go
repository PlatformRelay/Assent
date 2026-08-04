package aggregate

// profile_test.go is the E2-S09 profile-resolution + single-writer-authority
// suite (ADR-0018 §2). It proves:
//   - resolution picks the SINGLE covering profile by coverage → specificity
//     (narrower scope wins) → Config.profiles precedence-order tie-break;
//   - the single-writer invariant (at most one covering writes:true profile per
//     binding) is enforced — two covering writers are REJECTED, never last-one-wins;
//   - write-authority is surfaced into the Result, a recorder-only (writes:false)
//     profile NEVER claims it, and an uncovered binding defaults to no authority;
//   - resolution is decided by the precedence TABLE, not input-slice/map order
//     (determinism).

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// prof builds a Profile with the given name/writes/scope for the tests.
func prof(name string, writes bool, envs, classes []string) *policy.Profile {
	return &policy.Profile{
		APIVersion: "assent.dev/v1alpha1",
		Kind:       "PolicyProfile",
		Metadata:   policy.Metadata{Name: name},
		Spec: policy.ProfileSpec{
			Writes:       writes,
			Environments: envs,
			Classes:      classes,
		},
	}
}

// refs builds an ordered Config.profiles precedence table from names.
func refs(names ...string) []policy.ProfileRef {
	out := make([]policy.ProfileRef, len(names))
	for i, n := range names {
		out[i] = policy.ProfileRef{Name: n}
	}
	return out
}

// TestProfileSpecificityThenConfigOrder (REQ-E2-S09-01): two profiles covering
// the same (env, class) with different scope breadth → the NARROWER wins
// (specificity); a TRUE tie (equal breadth) is broken by Config.profiles order.
func TestProfileSpecificityThenConfigOrder(t *testing.T) {
	// Specificity beats config order: `broad` is FIRST in the precedence table,
	// yet the narrower `narrow` (concrete env AND class) wins.
	narrow := prof("narrow", false, []string{"prod"}, []string{"topic-registry"})
	broad := prof("broad", false, []string{"*"}, []string{"*"})
	rp, resolved, err := ResolveProfile(refs("broad", "narrow"),
		[]*policy.Profile{broad, narrow}, "prod", "topic-registry")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if !resolved {
		t.Fatal("expected a covering profile")
	}
	if rp.Name != "narrow" {
		t.Errorf("specificity: got %q, want narrow (narrower scope must beat config order)", rp.Name)
	}

	// A TRUE tie (both specificity 1 — one concrete dimension each) is decided by
	// Config.profiles order: the earlier row wins.
	envSpecific := prof("env-specific", false, []string{"prod"}, []string{"*"})
	classSpecific := prof("class-specific", false, []string{"*"}, []string{"topic-registry"})
	set := []*policy.Profile{envSpecific, classSpecific}

	rp, _, err = ResolveProfile(refs("env-specific", "class-specific"), set, "prod", "topic-registry")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if rp.Name != "env-specific" {
		t.Errorf("tie-break: got %q, want env-specific (earlier in precedence)", rp.Name)
	}
	// Reverse the precedence table → the tie flips to the now-earlier row.
	rp, _, err = ResolveProfile(refs("class-specific", "env-specific"), set, "prod", "topic-registry")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if rp.Name != "class-specific" {
		t.Errorf("tie-break reversed: got %q, want class-specific (now earlier in precedence)", rp.Name)
	}
}

// TestTwoWritersRejected (REQ-E2-S09-02): two profiles both covering a binding
// with writes:true → REJECTED (single-writer invariant), never last-one-wins.
func TestTwoWritersRejected(t *testing.T) {
	writerA := prof("writer-a", true, []string{"prod"}, []string{"topic-registry"})
	writerB := prof("writer-b", true, []string{"*"}, []string{"*"})
	if _, _, err := ResolveProfile(refs("writer-a", "writer-b"),
		[]*policy.Profile{writerA, writerB}, "prod", "topic-registry"); err == nil {
		t.Fatal("expected single-writer rejection: two covering writes:true profiles must never both resolve")
	}

	// Control: one writer + one recorder covering the same binding is NOT a
	// double-writer — the writer resolves cleanly.
	recorder := prof("recorder", false, []string{"*"}, []string{"*"})
	rp, resolved, err := ResolveProfile(refs("writer-a", "recorder"),
		[]*policy.Profile{writerA, recorder}, "prod", "topic-registry")
	if err != nil {
		t.Fatalf("one writer + one recorder must not be rejected: %v", err)
	}
	if !resolved || rp.Name != "writer-a" || !rp.Writes {
		t.Errorf("got %+v (resolved=%v), want writer-a with writes:true", rp, resolved)
	}

	// Control: two writers where only ONE covers this binding is not a conflict
	// (the invariant is per-binding coverage, not global).
	writerC := prof("writer-c", true, []string{"dev"}, []string{"topic-registry"})
	if _, _, err := ResolveProfile(refs("writer-a", "writer-c"),
		[]*policy.Profile{writerA, writerC}, "prod", "topic-registry"); err != nil {
		t.Fatalf("two writers where only one covers must not be rejected: %v", err)
	}
}

// TestWriteAuthoritySurfacedRecorderOnlyNever (REQ-E2-S09-03): a resolved
// writes:true profile surfaces write-authority + identity into the Result; a
// recorder-only resolution surfaces identity but write-authority FALSE; an
// uncovered binding defaults to no authority.
func TestWriteAuthoritySurfacedRecorderOnlyNever(t *testing.T) {
	// A resolved writer surfaces authority true + identity.
	writer := prof("prod-writer", true, []string{"prod"}, []string{"topic-registry"})
	rp, resolved, err := ResolveProfile(refs("prod-writer"),
		[]*policy.Profile{writer}, "prod", "topic-registry")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	res := Result{Decision: DecisionApprove}.WithProfile(rp, resolved)
	if !res.WriteAllowed {
		t.Error("a resolved writes:true profile must surface WriteAllowed true")
	}
	if res.Profile != "prod-writer" {
		t.Errorf("resolved profile identity = %q, want prod-writer", res.Profile)
	}

	// A recorder-only (writes:false) profile RESOLVES and its identity is
	// surfaced, but it NEVER claims write authority.
	recorder := prof("shadow", false, []string{"*"}, []string{"*"})
	rp, resolved, err = ResolveProfile(refs("shadow"),
		[]*policy.Profile{recorder}, "prod", "topic-registry")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	res = Result{Decision: DecisionApprove}.WithProfile(rp, resolved)
	if res.WriteAllowed {
		t.Error("a recorder-only (writes:false) profile must NEVER claim write authority")
	}
	if res.Profile != "shadow" {
		t.Errorf("recorder identity = %q, want shadow (identity still surfaced)", res.Profile)
	}

	// No covering profile → safe default: no authority, no identity.
	_, resolved, err = ResolveProfile(refs("prod-writer"),
		[]*policy.Profile{writer}, "dev", "topic-registry")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if resolved {
		t.Error("no profile covers (dev, topic-registry); expected resolved=false")
	}
	res = Result{Decision: DecisionApprove}.WithProfile(ResolvedProfile{}, resolved)
	if res.WriteAllowed || res.Profile != "" {
		t.Errorf("uncovered binding must default to no write authority / no identity, got %+v", res)
	}

	// Author-controlled fail-safe (documented, not a bug): a narrower RECORDER
	// covering the same binding as a broader WRITER resolves to recorder-only —
	// the writer is out-specified and write authority is withheld.
	narrowRecorder := prof("narrow-recorder", false, []string{"prod"}, []string{"topic-registry"})
	broadWriter := prof("broad-writer", true, []string{"*"}, []string{"*"})
	rp, resolved, err = ResolveProfile(refs("broad-writer", "narrow-recorder"),
		[]*policy.Profile{broadWriter, narrowRecorder}, "prod", "topic-registry")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	res = Result{Decision: DecisionApprove}.WithProfile(rp, resolved)
	if res.Profile != "narrow-recorder" || res.WriteAllowed {
		t.Errorf("narrower recorder must out-specify a broader writer (recorder-only), got %+v", res)
	}

	// Integration: CoverWithProfile stamps write-authority onto the REAL decision
	// without altering the findings — the D-016 fixture with a covering writer.
	mp, bind, in := loadD016(t)
	d016Writer := prof("d016-writer", true, []string{bind.Environment}, []string{bind.Class})
	full, err := CoverWithProfile(mp, bind, in, nil, policy.PhaseEnforce,
		refs("d016-writer"), []*policy.Profile{d016Writer})
	if err != nil {
		t.Fatalf("CoverWithProfile: %v", err)
	}
	if full.Decision != DecisionBlock {
		t.Errorf("decision = %q, want BLOCK (profile resolution must not alter the decision)", full.Decision)
	}
	if !full.WriteAllowed || full.Profile != "d016-writer" {
		t.Errorf("write-authority not surfaced into the decision: %+v", full)
	}
	if len(full.Findings) != 3 {
		t.Errorf("got %d findings, want 3 (profile resolution must not alter the finding set)", len(full.Findings))
	}
}

// TestProfileResolutionDoubleRunStable (REQ-E2-S09-04): resolution is decided by
// the precedence TABLE, not the input slice / map iteration order.
func TestProfileResolutionDoubleRunStable(t *testing.T) {
	a := prof("a", false, []string{"prod"}, []string{"*"})
	b := prof("b", false, []string{"*"}, []string{"topic-registry"})
	c := prof("c", false, []string{"*"}, []string{"*"})
	precedence := refs("a", "b", "c")

	// Same precedence table, DIFFERENTLY shuffled input slices → identical result.
	r1, res1, err1 := ResolveProfile(precedence, []*policy.Profile{a, b, c}, "prod", "topic-registry")
	r2, res2, err2 := ResolveProfile(precedence, []*policy.Profile{c, b, a}, "prod", "topic-registry")
	if err1 != nil || err2 != nil {
		t.Fatalf("ResolveProfile errors: %v / %v", err1, err2)
	}
	if r1 != r2 || res1 != res2 {
		t.Errorf("resolution must be precedence-table driven, not input-order: %+v vs %+v", r1, r2)
	}
	// a and b are both specificity 1; a is earlier in the table → a wins.
	if r1.Name != "a" {
		t.Errorf("got %q, want a (earlier in precedence at equal specificity)", r1.Name)
	}
}
