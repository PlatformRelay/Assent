package adoptertest

import (
	"reflect"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// fileEventsPack is a minimal document-mode pack whose sole rule governs a
// whole-file add/delete over topics/*.yaml: prove.when kind != "delete" is proven
// by an ADD (proving polarity) and failed by a DELETE (failing polarity).
func fileEventsPack(t *testing.T) (*policy.MergePolicy, *policy.Binding) {
	t.Helper()
	pol := &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: []policy.Rule{{
		Name:      "topic-lifecycle",
		Phase:     policy.PhaseEnforce,
		Match:     policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"topics/*.yaml"}, Kinds: []string{"add", "delete"}}},
		Prove:     &policy.Prove{Obligation: "reviewed-deletion", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: `kind != "delete"`}}},
		OnFailure: &policy.OnFailure{Effect: policy.EffectRequireReview, Code: "topic.deletion-needs-review"},
	}}}}
	return pol, &policy.Binding{Environment: "prod", Require: []string{"reviewed-deletion"}}
}

// TestOneSidedPresenceMintsFileEvent (REQ-EFE-S02-02) — a case whose base/↔head/
// presence is ONE-SIDED mints a clean whole-file event (path=="", subject
// file:<path>) via change.FileEvent instead of going opaque through change.Diff:
// absent base + present head = ADD; present base + absent head = DELETE; and the
// S01 fileEvents matcher selects it end-to-end.
func TestOneSidedPresenceMintsFileEvent(t *testing.T) {
	pol, bind := fileEventsPack(t)
	present := []byte("enabled: true\n")

	t.Run("absent base + present head -> file-ADD (proving polarity)", func(t *testing.T) {
		c := Case{Name: "add", Policy: pol, Bind: bind, File: "topics/orders.yaml", Base: nil, Head: present}
		in, decidable, err := assemble(c)
		if err != nil || !decidable {
			t.Fatalf("assemble: decidable=%v err=%v", decidable, err)
		}
		if len(in.ChangeSet.Changes) != 1 {
			t.Fatalf("want one minted change, got %+v", in.ChangeSet.Changes)
		}
		ch := in.ChangeSet.Changes[0]
		if ch.Path != "" || ch.Kind != "add" || ch.Subject != "file:topics/orders.yaml" || ch.File != "topics/orders.yaml" {
			t.Fatalf("minted change shape wrong: %+v", ch)
		}
		res, err := Evaluate(c)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if res.Decision != aggregate.DecisionApprove {
			t.Fatalf("a governed file-ADD proves reviewed-deletion -> APPROVE, got %q (%+v)", res.Decision, res.Findings)
		}
	})

	t.Run("present base + absent head -> file-DELETE (failing polarity)", func(t *testing.T) {
		c := Case{Name: "delete", Policy: pol, Bind: bind, File: "topics/orders.yaml", Base: present, Head: nil}
		in, decidable, err := assemble(c)
		if err != nil || !decidable {
			t.Fatalf("assemble: decidable=%v err=%v", decidable, err)
		}
		ch := in.ChangeSet.Changes[0]
		if ch.Path != "" || ch.Kind != "delete" || ch.Subject != "file:topics/orders.yaml" {
			t.Fatalf("minted change shape wrong: %+v", ch)
		}
		res, err := Evaluate(c)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if res.Decision != aggregate.DecisionReview {
			t.Fatalf("a governed file-DELETE fails reviewed-deletion -> REVIEW, got %q (%+v)", res.Decision, res.Findings)
		}
	})
}

// TestAmbiguousLifecycleStaysOpaque (REQ-EFE-S02-03, fail-safe · ambiguity) — a
// clean file-event is minted ONLY on unambiguous one-sided presence (exactly one
// side ABSENT). Every ambiguity stays a value-diff / opaque: an empty-but-present
// side is NOT a delete; a both-present content-opaque case stays opaque; NO
// path=="" event is fabricated (a wrongly-minted event is a fail-OPEN).
func TestAmbiguousLifecycleStaysOpaque(t *testing.T) {
	pol, bind := fileEventsPack(t)

	noWholeFileEvent := func(t *testing.T, in aggregate.EvaluationInput) {
		t.Helper()
		for _, ch := range in.ChangeSet.Changes {
			if ch.Path == "" {
				t.Fatalf("a clean whole-file event must NOT be minted for an ambiguous case, got %+v", ch)
			}
		}
	}

	t.Run("empty-but-present head is NOT a delete (present, not absent)", func(t *testing.T) {
		// base is a real document, head is an EMPTY-but-present document ({}). The
		// file still EXISTS on head, so this is a value-diff (a scalar delete), never a
		// whole-file DELETE event.
		c := Case{Name: "empty-head", Policy: pol, Bind: bind, File: "topics/orders.yaml", Base: []byte("enabled: true\n"), Head: []byte("{}\n")}
		in, decidable, err := assemble(c)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if decidable {
			noWholeFileEvent(t, in)
		}
	})

	t.Run("both-present content-opaque stays opaque (no event)", func(t *testing.T) {
		// A nested list makes the modify-only differ go opaque (E1-S05 territory) -> the
		// case is undecidable -> fail-safe REVIEW, and NO whole-file event is minted.
		c := Case{Name: "opaque", Policy: pol, Bind: bind, File: "topics/orders.yaml",
			Base: []byte("items:\n  - a\n  - b\n"), Head: []byte("items:\n  - a\n  - c\n")}
		in, decidable, err := assemble(c)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if decidable {
			noWholeFileEvent(t, in)
			t.Fatalf("a content-opaque both-present case must be undecidable, got %+v", in.ChangeSet.Changes)
		}
		res, err := Evaluate(c)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if res.Decision != aggregate.DecisionReview {
			t.Fatalf("an opaque case must fail safe -> REVIEW, got %q", res.Decision)
		}
	})

	t.Run("both absent stays undecidable (no fabricated event)", func(t *testing.T) {
		c := Case{Name: "both-absent", Policy: pol, Bind: bind, File: "topics/orders.yaml", Base: nil, Head: nil}
		_, decidable, err := assemble(c)
		if err != nil {
			t.Fatalf("assemble: %v", err)
		}
		if decidable {
			t.Fatalf("both-absent must be undecidable (no one-sided lifecycle), got decidable")
		}
	})
}

// TestUnmatchedFileDeleteFailsSafeReview (REQ-EFE-S02-04, 🔴 Judgment call (a)) —
// end-to-end through the harness: a minted whole-file DELETE that NO fileEvents rule
// governs evaluates to REVIEW (never APPROVE). Pinned so the fail-safe default
// cannot silently regress to APPROVE.
func TestUnmatchedFileDeleteFailsSafeReview(t *testing.T) {
	// A pack with NO fileEvents rule: the minted delete is ungoverned. require is
	// empty so, absent the escalation, the delete would earn a vacuous APPROVE.
	pol := &policy.MergePolicy{}
	bind := &policy.Binding{Environment: "prod"}
	c := Case{Name: "ungoverned-delete", Policy: pol, Bind: bind, File: "topics/orders.yaml", Base: []byte("enabled: true\n"), Head: nil}

	res, err := Evaluate(c)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Decision != aggregate.DecisionReview {
		t.Fatalf("an unmatched whole-file DELETE must fail safe -> REVIEW, got %q (%+v)", res.Decision, res.Findings)
	}
}

// TestFileEventDoubleRunStable (REQ-EFE-S02-05, determinism) — the file-event
// pipeline (mint + evaluate) double-runs byte-identical for both polarities.
func TestFileEventDoubleRunStable(t *testing.T) {
	pol, bind := fileEventsPack(t)
	cases := []Case{
		{Name: "add", Policy: pol, Bind: bind, File: "topics/orders.yaml", Base: nil, Head: []byte("enabled: true\n")},
		{Name: "delete", Policy: pol, Bind: bind, File: "topics/orders.yaml", Base: []byte("enabled: true\n"), Head: nil},
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			in1, d1, err1 := assemble(c)
			in2, d2, err2 := assemble(c)
			if err1 != nil || err2 != nil || d1 != d2 || !reflect.DeepEqual(in1, in2) {
				t.Fatalf("assemble not stable: (%v,%v) vs (%v,%v)", d1, err1, d2, err2)
			}
			r1, e1 := Evaluate(c)
			r2, e2 := Evaluate(c)
			if e1 != nil || e2 != nil || !reflect.DeepEqual(r1, r2) {
				t.Fatalf("Evaluate not stable: %+v vs %+v", r1, r2)
			}
		})
	}
}
