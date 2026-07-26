package classify_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/classify"
)

// satisfiablePolicyBinding returns a change on `.assent/config.yml` plus a
// binding whose single obligation IS PROVEN by a rule whose `when` cleanly
// evaluates to TRUE over that change. Evaluated with class "" it therefore
// APPROVES — this is the "would-be-satisfied predicate" the meta-class must
// dominate. The single-change set scalar-binds old/new so the predicate reads a
// real value, not the fail-safe REVIEW path.
func satisfiablePolicyBinding() (change.ChangeSet, aggregate.Binding) {
	cs := change.ChangeSet{
		Changes: []change.Change{{
			File: ".assent/config.yml",
			Path: "/enabled",
			Kind: change.KindModify,
			Old:  "false",
			New:  "true",
		}},
	}
	b := aggregate.Binding{
		Require: []string{"policy-ok"},
		Subject: "file:.assent/config.yml",
		Rules: []aggregate.Rule{{
			Name:       "policy-enabled",
			Obligation: "policy-ok",
			// Cleanly TRUE over the change above (new == "true"). If the
			// meta-class did not dominate, this would APPROVE.
			When:      `new == "true"`,
			OnFailure: aggregate.OnFailure{Effect: aggregate.EffectBlock, Code: "policy.disabled"},
		}},
	}
	return cs, b
}

// TestAssentPolicyBlockGolden is the mandatory "MR edits its own policy → BLOCK,
// never vouch" trust boundary (REQ-P4-E1-S07-01, ADR-0015 §1). It proves the
// assent-policy meta-class DOMINATES: the SAME binding + change that APPROVES
// with no class instead BLOCKs once the classifier routes the `.assent/**` edit
// to assent-policy. The contrast is the whole point — asserting BLOCK alone
// would be near-tautological, so we first assert the predicate WOULD have
// satisfied (APPROVE with class "").
func TestAssentPolicyBlockGolden(t *testing.T) {
	cs, b := satisfiablePolicyBinding()

	// Control: with NO class, the satisfiable predicate arms APPROVE. This
	// proves the golden below is not asserting a decision the rules would have
	// reached anyway.
	control, err := aggregate.Aggregate(b, cs, "")
	if err != nil {
		t.Fatalf("control aggregate error: %v", err)
	}
	if control.Decision != aggregate.DecisionApprove {
		t.Fatalf("control: expected the satisfiable predicate to APPROVE with no class, got %s (findings %#v)",
			control.Decision, control.Findings)
	}

	// The real path: classify the `.assent/**` change, then aggregate with the
	// resulting class. classify → assent-policy → aggregator short-circuits BLOCK
	// BEFORE the (would-be-true) predicate runs.
	class := classify.Classify(cs)
	if class != classify.ClassAssentPolicy {
		t.Fatalf("classify(.assent/**) = %q, want %q", class, classify.ClassAssentPolicy)
	}
	got, err := aggregate.Aggregate(b, cs, class)
	if err != nil {
		t.Fatalf("policy aggregate error: %v", err)
	}
	if got.Decision != aggregate.DecisionBlock {
		t.Fatalf("assent-policy MR: expected BLOCK (meta-class dominates the satisfiable predicate), got %s (findings %#v)",
			got.Decision, got.Findings)
	}

	// Double-run: the whole classify→aggregate flow, executed twice, is
	// byte-identical (decision + sorted findings), ADR-0013/ADR-0017 §9.
	class2 := classify.Classify(cs)
	got2, err := aggregate.Aggregate(b, cs, class2)
	if err != nil {
		t.Fatalf("second-run aggregate error: %v", err)
	}
	if !reflect.DeepEqual(got, got2) {
		t.Fatalf("double-run not identical:\n first: %#v\nsecond: %#v", got, got2)
	}
}

// TestAssentPolicyDominatesMixedChangeSet proves the dominance rule for a MIXED
// MR: a change set touching BOTH a governed entry and `.assent/**` still routes
// to assent-policy → BLOCK. A policy edit cannot be laundered by burying it among
// non-policy changes.
func TestAssentPolicyDominatesMixedChangeSet(t *testing.T) {
	cs := change.ChangeSet{
		Changes: []change.Change{
			{File: "topics/orders.yml", Path: "/partitions", Kind: change.KindModify, Old: "3", New: "6"},
			{File: ".assent/packs/topic.yml", Path: "/threshold", Kind: change.KindModify, Old: "10", New: "1"},
		},
	}
	if class := classify.Classify(cs); class != classify.ClassAssentPolicy {
		t.Fatalf("mixed set with a `.assent/**` change: class = %q, want %q", class, classify.ClassAssentPolicy)
	}
	b := aggregate.Binding{Require: []string{"x"}, Subject: "file:topics/orders.yml"}
	got, err := aggregate.Aggregate(b, cs, classify.Classify(cs))
	if err != nil {
		t.Fatalf("aggregate error: %v", err)
	}
	if got.Decision != aggregate.DecisionBlock {
		t.Fatalf("mixed policy MR: expected BLOCK, got %s", got.Decision)
	}
}

// TestNonPolicyChangeIsUnclassified proves the classifier does NOT over-route: a
// change with no `.assent/**` path is unclassified, so it does NOT trigger the
// meta-block (the control side of the boundary — without this the block could be
// hiding an "everything BLOCKs" bug).
func TestNonPolicyChangeIsUnclassified(t *testing.T) {
	cs := change.ChangeSet{Changes: []change.Change{
		{File: "topics/orders.yml", Path: "/partitions", Kind: change.KindModify, Old: "3", New: "6"},
	}}
	if class := classify.Classify(cs); class != classify.ClassUnclassified {
		t.Fatalf("non-policy change: class = %q, want %q", class, classify.ClassUnclassified)
	}
}

// TestClassifyFilePaths exercises the per-file matcher's branches directly
// (prefix, bare marker, ./ normalisation, nested non-match, plain non-match).
func TestClassifyFilePaths(t *testing.T) {
	cases := []struct {
		file string
		want classify.Class
	}{
		{".assent/config.yml", classify.ClassAssentPolicy},
		{".assent/packs/topic.yml", classify.ClassAssentPolicy},
		{".assent", classify.ClassAssentPolicy},
		{"./.assent/config.yml", classify.ClassAssentPolicy},
		{"topics/orders.yml", classify.ClassUnclassified},
		{"src/.assent/nested.yml", classify.ClassUnclassified}, // nested, not the repo policy root
		{".assentfoo/x.yml", classify.ClassUnclassified},       // prefix boundary: not `.assent/`
	}
	for _, tc := range cases {
		if got := classify.FileClass(tc.file); got != tc.want {
			t.Errorf("FileClass(%q) = %q, want %q", tc.file, got, tc.want)
		}
	}
}

// TestClassifyEmptyAndOpaque proves an empty/opaque ChangeSet routes to
// unclassified — the classifier never fabricates a policy match it cannot see;
// the aggregator independently fails such a set safe to REVIEW.
func TestClassifyEmptyAndOpaque(t *testing.T) {
	if got := classify.Classify(change.ChangeSet{}); got != classify.ClassUnclassified {
		t.Errorf("empty ChangeSet: got %q, want %q", got, classify.ClassUnclassified)
	}
	if got := classify.Classify(change.ChangeSet{Opaque: true, OpaqueReason: "bad"}); got != classify.ClassUnclassified {
		t.Errorf("opaque ChangeSet: got %q, want %q", got, classify.ClassUnclassified)
	}
}

// TestReservedClassRoutingRejected is the ADVERSARIAL reserved-class lint
// (REQ-P4-E1-S07-01 adversarial, ADR-0008 amendment, ADR-0015 §1). A pack rule
// that tries to route a reserved class (assent-policy / unclassified) to a VOUCH
// (APPROVE-arming) disposition is REJECTED, not honoured — a self-editing policy
// MR cannot vouch for itself. Every non-vouch disposition on a reserved class —
// including `challenge`, the relaxation ADR-0015 §1 EXPLICITLY permits — and any
// routing of a non-reserved class, are honoured.
func TestReservedClassRoutingRejected(t *testing.T) {
	rejected := []struct {
		name        string
		class       classify.Class
		disposition classify.Disposition
	}{
		{"assent-policy → vouch", classify.ClassAssentPolicy, classify.DispositionVouch},
		{"unclassified → vouch", classify.ClassUnclassified, classify.DispositionVouch},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			err := classify.ValidateRouting(tc.class, tc.disposition)
			if err == nil {
				t.Fatalf("expected routing %s → %s to be REJECTED, but it was honoured", tc.class, tc.disposition)
			}
			var rce *classify.ErrReservedClassRouting
			if !errors.As(err, &rce) {
				t.Fatalf("expected *ErrReservedClassRouting, got %T: %v", err, err)
			}
			if rce.Class != tc.class || rce.Disposition != tc.disposition {
				t.Fatalf("error carried (%s,%s), want (%s,%s)", rce.Class, rce.Disposition, tc.class, tc.disposition)
			}
			// The message must name the offending class and disposition so the lint
			// is actionable (and exercises Error()).
			if msg := rce.Error(); !strings.Contains(msg, string(tc.class)) || !strings.Contains(msg, string(tc.disposition)) {
				t.Fatalf("error message %q must name class %q and disposition %q", msg, tc.class, tc.disposition)
			}
		})
	}

	allowed := []struct {
		name        string
		class       classify.Class
		disposition classify.Disposition
	}{
		{"assent-policy → block (default, allowed)", classify.ClassAssentPolicy, classify.DispositionBlock},
		// ADR-0015 §1 explicitly permits relaxing assent-policy to challenge.
		{"assent-policy → challenge (explicitly permitted relaxation)", classify.ClassAssentPolicy, classify.DispositionChallenge},
		{"assent-policy → review (non-arming)", classify.ClassAssentPolicy, classify.DispositionReview},
		{"unclassified → block", classify.ClassUnclassified, classify.DispositionBlock},
		{"topic (non-reserved) → vouch", "topic", classify.DispositionVouch},
	}
	for _, tc := range allowed {
		t.Run(tc.name, func(t *testing.T) {
			if err := classify.ValidateRouting(tc.class, tc.disposition); err != nil {
				t.Fatalf("expected routing %s → %s to be honoured, got rejection: %v", tc.class, tc.disposition, err)
			}
		})
	}
}
