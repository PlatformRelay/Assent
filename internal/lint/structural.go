package lint

// structural.go is the E3-S02 layer: three per-document structural hard errors
// over the SAME tolerant parse the S01 anchor produced (coverage.go's ingest) —
// no re-decode, no strict-loader duplication:
//
//	reserved-class            — a binding routing the reserved `assent-policy`
//	                            meta-class to a pack whose rule resolves to a
//	                            non-block/non-challenge outcome (ADR-0015 §1).
//	no-implicit-enforce-phase — a MergePolicy rule or a Pack manifest omitting the
//	                            required rollout `phase` (ADR-0018 §1, D-017 B2).
//	unkeyed-list              — a `mode: list` entry with no identity.pointer
//	                            (ADR-0017 §5).
//
// # Reserved-class: why lint is STRICTER than classify.ValidateRouting
//
// classify.ValidateRouting is the frozen runtime backstop: it rejects ONLY a
// reserved class routed to VOUCH (an APPROVE-arming, obligation-satisfying
// outcome), and its own doc notes it PERMITS a `review`/comment-style disposition
// on a reserved class. That is correct for the runtime guard. The AUTHORING gate
// (this lint) is deliberately tighter, and this divergence is DELIBERATE per the
// story's authoritative docs — spec REQ-E3-S02-01 ("reserved assent-policy routed
// to a non-block/challenge outcome → reserved-class error; block/challenge lints
// clean") and ADR-0015 §1 (a policy MR is block-by-default and may relax ONLY to
// challenge). So a `comment` (advisory, enforces nothing) rule on the policy
// class, which ValidateRouting would honour, still fails lint, because a policy
// MR that only comments never blocks itself. NOTE: this INTENTIONALLY overrides
// classify's "review permitted on a reserved class" note — the runtime backstop
// tolerates it; the authoring gate does not.
// We therefore use ValidateRouting as the reserved-class IDENTITY + the frozen
// VOUCH-rejection authority (its message names the ADR), then layer the
// {block, challenge} whitelist as documented additive authoring strictness. The
// two are consistent: everything ValidateRouting rejects, lint rejects; lint
// additionally rejects the advisory outcomes ADR-0015 §1 does not sanction.
//
// # Phase double-count dedupe (coordination with E3-S01)
//
// A missing `phase` ALSO makes the strict loader refuse the doc, which S01's
// tolerant bridge captures as one schema-invalid diagnostic (coverage.go). To
// avoid counting one defect under two codes, when no-implicit-enforce-phase fires
// for a doc we drop the schema-invalid for that same doc — but ONLY when the
// missing phase is its SOLE cause (removeSchemaInvalidForPhase / isPhaseOnly in
// coverage.go). A doc that is missing phase AND has an unrelated schema violation
// keeps its schema-invalid in full, so the co-located defect is never hidden
// (fail-many is preserved); the actionable no-implicit-enforce-phase is what an
// author reads for the phase itself.
//
// Purity: pure — imports only policy (types) and classify (reserved identity +
// ValidateRouting), both pure; no clock/rand/env/net. TestCorePurity scans this.

import (
	"fmt"
	"sort"

	"github.com/PlatformRelay/assent/internal/core/classify"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// Diagnostic codes owned by the structural checks (defined here so lint.go's only
// S02 edit is the single checkStructural call-site).
const (
	// CodeReservedClass: a binding routes the reserved assent-policy meta-class to
	// a rule whose disposition is not block/challenge (ADR-0015 §1).
	CodeReservedClass = "reserved-class"
	// CodeNoImplicitEnforcePhase: a rule or Pack manifest omits the required
	// rollout phase (ADR-0018 §1, D-017 B2).
	CodeNoImplicitEnforcePhase = "no-implicit-enforce-phase"
	// CodeUnkeyedList: a mode:list entry declares no identity.pointer (ADR-0017 §5).
	CodeUnkeyedList = "unkeyed-list"
)

// listMode is the Entry.mode value that requires a content-derived identity
// pointer (a keyed collection). `document` mode needs no identity.
const listMode = "list"

// ingestedDoc is one tolerantly-decoded MergePolicy or Pack doc paired with its
// real source path. It DUPLICATES the rules already accumulated in
// loadedPack.rules deliberately: the S01/S03 checks (coverage.go / facts_ref.go)
// read loadedPack.rules and are file-ownership-frozen for this story, so rather
// than widen loadedPack to carry a per-rule source path (which would ripple into
// those frozen readers) S02 tracks its own per-doc view — carrying the source
// path the phase dedupe needs to correlate with the schema-invalid, plus the
// entries the unkeyed-list check needs. Populated by ingest (coverage.go).
type ingestedDoc struct {
	path      string
	isPack    bool
	rules     []policy.Rule           // MergePolicy rules (phase + reserved-class)
	entries   map[string]policy.Entry // MergePolicy entries (unkeyed-list)
	packPhase policy.Phase            // Pack manifest spec.phase (isPack only)
	packName  string                  // Pack manifest metadata.name (isPack only)
}

// checkStructural runs the three E3-S02 structural hard errors over the tolerant
// model, then applies the phase double-count dedupe. Wired into Lint() after the
// S01/S03 checks (lint.go).
func checkStructural(m *model, rep *Report) {
	checkReservedClass(m, rep)
	checkNoImplicitEnforcePhase(m, rep)
	checkUnkeyedLists(m, rep)
}

// checkReservedClass emits reserved-class for any rule, in a pack bound by an
// `assent-policy` binding, whose disposition is not block/challenge. See the file
// header for why this is stricter than classify.ValidateRouting.
func checkReservedClass(m *model, rep *Report) {
	// The set of packs a reserved (assent-policy) binding routes to. Only these
	// packs' rules are policy-class dispositions.
	reservedPacks := map[string]bool{}
	for _, bb := range m.bindings {
		if bb.binding.Class != classify.ClassAssentPolicy {
			continue
		}
		for _, pk := range bb.binding.Packs {
			reservedPacks[pk] = true
		}
	}
	if len(reservedPacks) == 0 {
		return
	}
	for _, d := range m.docs {
		if d.isPack || len(d.rules) == 0 {
			continue
		}
		if !reservedPacks[packName(d.path)] {
			continue
		}
		for i := range d.rules {
			r := d.rules[i]
			disp := ruleDisposition(r)
			loc := Location{File: d.path, Name: ruleLabel(r, i)}
			// Frozen authority for the vouch case: ValidateRouting rejects a
			// reserved class routed to an APPROVE-arming disposition, with a
			// message naming the ADR.
			if err := classify.ValidateRouting(classify.ClassAssentPolicy, disp); err != nil {
				rep.addError(CodeReservedClass, loc, fmt.Sprintf(
					"rule routes the reserved %q class to an APPROVE-arming outcome: %v",
					classify.ClassAssentPolicy, err))
				continue
			}
			// Additive authoring strictness: ADR-0015 §1 permits a policy MR to
			// relax only to challenge, never to an advisory/non-enforcing outcome
			// (a comment effect enforces nothing) — stricter than ValidateRouting.
			if disp != classify.DispositionBlock && disp != classify.DispositionChallenge {
				rep.addError(CodeReservedClass, loc, fmt.Sprintf(
					"rule routes the reserved %q class to a non-block/non-challenge outcome (%s); a policy change MR is block-by-default and may relax only to challenge, never to an advisory/comment or vouch outcome (ADR-0015 §1)",
					classify.ClassAssentPolicy, disp))
			}
		}
	}
}

// ruleDisposition maps a loaded rule to the classify.Disposition it applies on
// its matched class. A prove/obligation rule ARMS APPROVE when its assert tree
// passes → vouch (regardless of its onFailure effect: the passing path is the
// self-vouch risk). A plain-effect rule maps block→block, challenge→challenge;
// every other effect (comment, require-review, or an absent effect) is a
// non-enforcing/advisory outcome → review. This is the single place rule outcome
// → disposition is derived.
func ruleDisposition(r policy.Rule) classify.Disposition {
	if r.Prove != nil {
		return classify.DispositionVouch
	}
	switch r.Effect {
	case policy.EffectBlock:
		return classify.DispositionBlock
	case policy.EffectChallenge:
		return classify.DispositionChallenge
	default:
		return classify.DispositionReview
	}
}

// checkNoImplicitEnforcePhase emits no-implicit-enforce-phase for every rule with
// an empty (missing) phase and every Pack manifest with an empty spec.phase,
// pointing at the sanctioned off/observe/enforce field. It keys on Phase == ""
// (MISSING) only — a non-empty invalid phase is an enum violation the strict
// loader already reports as schema-invalid, and must not be deduped. When it
// fires for a doc it drops that doc's phase-only schema-invalid (the dedupe).
func checkNoImplicitEnforcePhase(m *model, rep *Report) {
	for _, d := range m.docs {
		fired := false
		if d.isPack {
			if d.packPhase == "" {
				rep.addError(CodeNoImplicitEnforcePhase, Location{File: d.path, Name: d.packName}, fmt.Sprintf(
					"pack manifest %q omits the required rollout phase; add spec.phase: off | observe | enforce (rollout has no default — an undecorated pack is rejected rather than silently enforcing) (ADR-0018 §1, D-017 B2)",
					d.packName))
				fired = true
			}
		} else {
			for i := range d.rules {
				if d.rules[i].Phase == "" {
					rep.addError(CodeNoImplicitEnforcePhase, Location{File: d.path, Name: ruleLabel(d.rules[i], i)}, fmt.Sprintf(
						"rule %q omits the required rollout phase; add phase: off | observe | enforce (rollout has no default — do not approximate a rollout via effect/onFailure) (ADR-0018 §1, D-017 B2)",
						ruleLabel(d.rules[i], i)))
					fired = true
				}
			}
		}
		if fired {
			// Dedupe: the same missing-phase defect the strict loader captured as a
			// schema-invalid on this doc is now reported by the actionable code —
			// drop the schema-invalid, but only when phase is its SOLE cause.
			rep.removeSchemaInvalidForPhase(d.path)
		}
	}
}

// checkUnkeyedLists emits unkeyed-list for every `mode: list` entry that declares
// no identity.pointer (ADR-0017 §5). Entry keys are visited in sorted order for a
// stable source emission (the Report re-sorts on output regardless).
func checkUnkeyedLists(m *model, rep *Report) {
	for _, d := range m.docs {
		if d.isPack || len(d.entries) == 0 {
			continue
		}
		names := make([]string, 0, len(d.entries))
		for name := range d.entries {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			e := d.entries[name]
			if e.Mode == listMode && e.Identity.Pointer == "" {
				rep.addError(CodeUnkeyedList, Location{File: d.path, Name: name}, fmt.Sprintf(
					"entry %q is a `mode: list` collection with no identity.pointer; a keyed list must declare a content-derived identity pointer rather than have one guessed (ADR-0017 §5)",
					name))
			}
		}
	}
}

// ruleLabel is a rule's human identity for a diagnostic Location — its name, or a
// positional fallback when a tolerantly-decoded rule has no name.
func ruleLabel(r policy.Rule, idx int) string {
	if r.Name != "" {
		return r.Name
	}
	return fmt.Sprintf("rule[%d]", idx)
}
