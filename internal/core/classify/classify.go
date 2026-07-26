// Package classify is the PURE, minimal change-classifier for the P4-E1 walking
// skeleton (P4-E1-S07-01, ADR-0008 §1, ADR-0015 §1). It routes a change to a
// built-in change class from the change's FILE PATH(S) ALONE — no clock,
// randomness, environment, or network (internal/core/** purity boundary,
// GUIDELINES §5, ADR-0013; internal/core/purity_test.go scans this tree).
//
// The single boundary this slice closes: an MR that edits its OWN policy under
// `.assent/**` must land in the reserved meta-class `assent-policy`, which the
// aggregator (internal/core/aggregate) SHORT-CIRCUITS to BLOCK before any
// predicate evaluation. A self-editing policy MR can therefore never vouch for
// itself (ADR-0015 §1). The classifier decides the class; the aggregator
// enforces the block; this test-composed pair is the mandatory
// "MR edits its own policy → BLOCK" trust boundary.
//
// Fail-safe direction (GUIDELINES §2): the SAFE error is to OVER-match
// `.assent/**` (routing a non-policy edit to BLOCK is merely conservative),
// while UNDER-matching a real `.assent/**` edit is the forbidden self-vouch. A
// path that matches no built-in class gets the implicit `unclassified` class
// (ADR-0008 §1), which — like `assent-policy` — no vouch rule may match.
package classify

import (
	"strings"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// Class is a built-in change class label. Repos define their own domain classes
// in later epics; this skeleton emits only the two engine-reserved classes plus
// the implicit unclassified fallback.
type Class = string

const (
	// ClassAssentPolicy is the reserved meta-class for a change under
	// `.assent/**` (ADR-0015 §1). It re-exports aggregate.ReservedPolicyClass so
	// the classifier and the aggregator agree on the exact string the
	// short-circuit keys on — a drift here would silently defeat the block.
	ClassAssentPolicy Class = aggregate.ReservedPolicyClass // "assent-policy"

	// ClassUnclassified is the implicit fallback for a change that matches no
	// built-in class (ADR-0008 §1). No vouch rule may match it (reserved).
	ClassUnclassified Class = "unclassified"
)

// policyPrefix is the trusted-input policy tree. A file at this prefix (or a
// bare `.assent` policy file) is policy the MR must not vouch for itself.
const policyPrefix = ".assent/"

// isPolicyPath reports whether a single file path is under the `.assent/**`
// policy tree. The skeleton matches the ROOT `.assent/` prefix (and the bare
// `.assent` directory marker); a nested `foo/.assent/` is intentionally NOT
// treated as the repo policy tree in this slice — the repo's policy lives at the
// checkout root (ADR-0010 layout). This choice is documented so a future
// multi-root layout can widen it deliberately rather than by accident. Matching
// is over the path string only (purity): no filesystem access.
func isPolicyPath(file string) bool {
	// Normalise a leading "./" so "./.assent/x" is recognised.
	file = strings.TrimPrefix(file, "./")
	return file == ".assent" || strings.HasPrefix(file, policyPrefix)
}

// FileClass routes a single file path to its built-in class.
func FileClass(file string) Class {
	if isPolicyPath(file) {
		return ClassAssentPolicy
	}
	return ClassUnclassified
}

// Classify routes a whole ChangeSet to one dominating class (ADR-0008 §1,
// ADR-0015 §1). If ANY change in the set touches `.assent/**`, the entire set
// routes to `assent-policy`: a mixed MR that edits both policy and a governed
// entry cannot escape the self-edit block by burying the policy change among
// non-policy ones. An opaque or empty ChangeSet has no decidable policy path, so
// it routes to `unclassified` (the aggregator independently fails such a set
// safe to REVIEW); this classifier never fabricates a policy match it cannot see.
func Classify(cs change.ChangeSet) Class {
	for _, c := range cs.Changes {
		if isPolicyPath(c.File) {
			return ClassAssentPolicy
		}
	}
	return ClassUnclassified
}

// reservedClasses are the engine-reserved classes that NO vouch/obligation-
// satisfying rule may route to (ADR-0008 amendment (b), ADR-0015 §1). A pack
// that tries to route one of these to a vouch disposition is rejected at load,
// not merely discouraged.
var reservedClasses = map[Class]struct{}{
	ClassAssentPolicy: {},
	ClassUnclassified: {},
}

// Disposition is what a pack ROUTING does with a matched class — the property
// the reserved-class lint discriminates on. Crucially this is NOT an aggregate
// onFailure Effect: in the engine NO onFailure effect arms APPROVE (see
// aggregate.effectDecision — block→BLOCK, everything else→REVIEW), so "vouch"
// cannot be represented as an effect. Vouch is the APPROVE-ARMING routing (a
// rule whose clean pass SATISFIES a required obligation and thereby contributes
// to APPROVE). A pack routes a class to exactly one disposition; the reserved
// classes may take any disposition EXCEPT vouch.
type Disposition string

const (
	// DispositionVouch is the APPROVE-arming routing: a matched class's rule can
	// SATISFY an obligation and move the decision toward APPROVE. This is the ONLY
	// disposition a reserved class may not take (ADR-0008 amendment, ADR-0015 §1).
	DispositionVouch Disposition = "vouch"
	// DispositionBlock hard-blocks the matched class (the `assent-policy` default).
	DispositionBlock Disposition = "block"
	// DispositionChallenge is the author-resolvable relaxation ADR-0015 §1
	// EXPLICITLY permits for `assent-policy` ("relax only to challenge, never to
	// vouch"). It does not arm APPROVE, so it is a permitted reserved routing.
	DispositionChallenge Disposition = "challenge"
	// DispositionReview routes the class to fail-safe REVIEW (require-review /
	// comment-style non-arming). Also non-vouch, so permitted on a reserved class.
	DispositionReview Disposition = "review"
)

// isVouch reports whether a disposition ARMS APPROVE (is a vouch). Only the
// explicit vouch disposition does; every other routing denies or defers, so it
// is safe for a reserved class.
func isVouch(d Disposition) bool { return d == DispositionVouch }

// ErrReservedClassRouting is returned when a pack rule attempts to route an
// engine-reserved class to a vouch (APPROVE-arming) disposition (ADR-0008
// amendment). It is a hard load-time rejection, not a warning.
type ErrReservedClassRouting struct {
	Class       Class
	Disposition Disposition
}

func (e *ErrReservedClassRouting) Error() string {
	return "classify: reserved class " + string(e.Class) + " cannot be routed to vouch disposition " +
		string(e.Disposition) + " (a self-editing/unclassified change must never vouch — ADR-0008 amendment, ADR-0015 §1)"
}

// ValidateRouting is the reserved-class lint (ADR-0008 amendment (b), ADR-0015
// §1). Given a pack routing (the class a rule targets and the disposition it
// would apply on that class), it REJECTS — returns a non-nil
// *ErrReservedClassRouting — exactly when a reserved class is routed to VOUCH.
// A reserved class routed to any non-vouch disposition (block, and the
// explicitly-permitted challenge, and review), and any routing of a non-reserved
// class, are honoured (nil). This is the guard that refuses "let this `.assent/**`
// MR vouch for itself" at load, rather than evaluating and honouring it.
func ValidateRouting(class Class, disposition Disposition) error {
	if _, reserved := reservedClasses[class]; !reserved {
		return nil
	}
	if isVouch(disposition) {
		return &ErrReservedClassRouting{Class: class, Disposition: disposition}
	}
	return nil
}
