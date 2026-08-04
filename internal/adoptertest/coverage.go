package adoptertest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/glob"
)

// coverage.go is the PURE `assent test --coverage` both-polarity computation
// (P5-E6-S07): the RUN-TIME counterpart to E3-S06's static tests-per-rule presence
// check. It computes, per loaded rule, whether the rule is exercised in BOTH
// polarities across a repo's cases:
//
//   - FAILING polarity: the rule is the DRIVING finding (it fires) in >=1 case, and
//   - PROVING polarity: the rule is satisfied / PROVEN-SILENT (does NOT fire,
//     ADR-0017 §2 "satisfied-is-silent") in >=1 case.
//
// A rule exercised in both is COVERED; a rule missing a polarity fails the gate,
// which names the missing polarity. Only STRUCTURED safety assertions count
// (decision / rule / effect / score) — NEVER `message~` (presentation, not safety;
// ADR-0014 amendment / D-054). `findings[].path` is unsupported (D-054) and is not
// counted.
//
// LOAD-BEARING FAIL-SAFE DIRECTION (D-059): `--coverage` is a COMPLETENESS gate, so
// the dangerous error is a FALSE "covered" (claiming a rule is both-polarity tested
// when it isn't → a rule ships with only its happy path). Every ambiguity therefore
// errs toward NOT-covered (fail the gate), never toward covered:
//   - an unwitnessed polarity stays FALSE (the zero value is "not covered");
//   - PROVING credit requires the rule to have actually MATCHED >=1 change AND been
//     silent — a rule that merely did-not-fire because it never APPLIED earns NO
//     proving credit (matched is re-derived faithfully from the pack's own rule
//     Match, mirroring aggregate.matchChanges);
//   - a match-derivation defect (an unsupported/absent domain — the same defect that
//     would error the engine's own Cover) ERRORS the whole computation (fail-closed),
//     never a silent "matched nothing".

// CoverageRule is one rule eligible for both-polarity coverage. The UNIVERSE is
// scoped to ENFORCE-effective-phase OBLIGATION (prove) rules (D-059): an off/observe
// rule never drives an enforcing finding (its finding is routed to the observed
// bucket and structurally excluded from the decision), and the engine's own
// obligation-coverage (aggregate.coverage.go) marks an obligation covered ONLY from
// an enforce-phase proving rule — so both-polarity is impossible/meaningless for a
// non-enforce or non-obligation rule, and requiring it would make the gate
// permanently un-satisfiable. Keyed by pack + rule (the catalogue's stable ID).
type CoverageRule struct {
	Pack string
	Rule string
}

// ID is the stable "<pack>/<rule>" identity (mirrors catalogue.RuleEntry.ID), the
// canonical sort key for the deterministic report.
func (r CoverageRule) ID() string { return r.Pack + "/" + r.Rule }

// CaseWitness is what one case witnesses about the enforce obligation rules of its
// pack: which it pins as FIRING (failing polarity) and which it proves SILENT
// (proving polarity). The CALLER credits a witness only for a PASSING case (a case
// whose own assertions did not hold is not valid coverage evidence). Rule names are
// pack-local; the caller pairs them with the pack when building credits.
type CaseWitness struct {
	// Failing is the enforce obligation rule names the case STRUCTURALLY pins as
	// firing (an ExpectFinding with a non-empty effect). A `message~`-only entry is
	// a presentation assertion and is NOT counted (REQ-E6-S07-02).
	Failing []string
	// Proving is the enforce obligation rule names the case exercised (matched >=1
	// change) AND that did NOT fire — satisfied-silent (proving polarity).
	Proving []string
}

// CoverageCredit is one (pack, rule) polarity credit contributed by a passing case.
type CoverageCredit struct {
	Pack    string
	Rule    string
	Failing bool
	Proving bool
}

// RulePolarity is one rule's accumulated both-polarity status across the corpus.
type RulePolarity struct {
	Pack    string
	Rule    string
	Failing bool
	Proving bool
}

// ID is the stable "<pack>/<rule>" identity.
func (r RulePolarity) ID() string { return r.Pack + "/" + r.Rule }

// Covered reports whether the rule was exercised in BOTH polarities.
func (r RulePolarity) Covered() bool { return r.Failing && r.Proving }

// CoverageReport is the deterministic per-rule both-polarity report: every universe
// rule in canonical ID order with its accumulated polarity status.
type CoverageReport struct {
	Rules []RulePolarity
}

// Complete reports whether every rule is both-polarity covered (the exit-0 gate).
func (rep CoverageReport) Complete() bool {
	for _, r := range rep.Rules {
		if !r.Covered() {
			return false
		}
	}
	return true
}

// BuildCoverageReport folds the per-case polarity credits onto the enforce
// obligation rule UNIVERSE. The universe is authoritative: a credit for a rule NOT
// in it is ignored (a case can only witness rules the pack actually declares). A
// rule with no credit for a polarity keeps that polarity FALSE — the fail-safe
// default (an unwitnessed polarity is NOT covered). The result is sorted by ID, so
// a double run over the same inputs is byte-identical (REQ-E6-S07-04).
func BuildCoverageReport(universe []CoverageRule, credits []CoverageCredit) CoverageReport {
	byID := map[string]*RulePolarity{}
	var order []string
	for _, u := range universe {
		id := u.ID()
		if _, ok := byID[id]; ok {
			continue
		}
		byID[id] = &RulePolarity{Pack: u.Pack, Rule: u.Rule}
		order = append(order, id)
	}
	for _, c := range credits {
		rp, ok := byID[(CoverageRule{Pack: c.Pack, Rule: c.Rule}).ID()]
		if !ok {
			continue
		}
		if c.Failing {
			rp.Failing = true
		}
		if c.Proving {
			rp.Proving = true
		}
	}
	sort.Strings(order)
	rules := make([]RulePolarity, 0, len(order))
	for _, id := range order {
		rules = append(rules, *byID[id])
	}
	return CoverageReport{Rules: rules}
}

// RenderCoverage renders the deterministic coverage report: every rule in canonical
// ID order with its both-polarity status, a rule missing a polarity NAMING which one
// (REQ-E6-S07-01), and a final summary line. Byte-identical across runs
// (REQ-E6-S07-04) — it ranges the pre-sorted Rules slice and never a Go map.
func RenderCoverage(rep CoverageReport) string {
	var b strings.Builder
	missing := 0
	for _, r := range rep.Rules {
		switch {
		case r.Covered():
			fmt.Fprintf(&b, "COVERED  %s (proving + failing)\n", r.ID())
		case r.Proving && !r.Failing:
			missing++
			fmt.Fprintf(&b, "MISSING  %s: no failing case (proven silent but never driven to fire)\n", r.ID())
		case r.Failing && !r.Proving:
			missing++
			fmt.Fprintf(&b, "MISSING  %s: no proving case (fires but never proven silent)\n", r.ID())
		default:
			missing++
			fmt.Fprintf(&b, "MISSING  %s: no proving case, no failing case (rule never exercised)\n", r.ID())
		}
	}
	if missing == 0 {
		fmt.Fprintf(&b, "coverage: OK — %d rule(s), every rule tested in both polarities\n", len(rep.Rules))
	} else {
		fmt.Fprintf(&b, "coverage: FAIL — %d of %d rule(s) missing a polarity\n", missing, len(rep.Rules))
	}
	return b.String()
}

// RunCaseCoverage evaluates the case ONCE and returns BOTH its Outcome (identical
// to RunCase) and its coverage CaseWitness. enforceObl is the set of enforce
// obligation rule NAMES in the case's pack (the universe restricted to this pack).
// The caller credits the witness only for a PASSING Outcome. An undecidable
// (opaque/empty) case exercises no rule → an empty proving set.
//
// Fail-closed: a match-derivation defect for any rule (an unsupported/absent match
// domain) errors — never a silent "matched nothing" that could mis-credit proving.
func RunCaseCoverage(c Case, enforceObl map[string]bool) (Outcome, CaseWitness, error) {
	in, decidable, err := assemble(c)
	if err != nil {
		return Outcome{}, CaseWitness{}, fmt.Errorf("case %q: %w", c.Name, err)
	}

	var res aggregate.Result
	if decidable {
		res, err = aggregate.CoverWithApproval(c.Policy, c.Bind, &in, c.Approval)
		if err != nil {
			return Outcome{}, CaseWitness{}, fmt.Errorf("case %q: cover: %w", c.Name, err)
		}
	} else {
		res = aggregate.Result{Decision: aggregate.DecisionReview}
	}

	outcome, err := outcomeFrom(c, res)
	if err != nil {
		return Outcome{}, CaseWitness{}, err
	}

	w := CaseWitness{Failing: failingPins(c.Expect, enforceObl)}
	if decidable {
		proving, perr := provingSilent(c.Policy.Spec.Rules, in.ChangeSet.Changes, res, enforceObl)
		if perr != nil {
			return Outcome{}, CaseWitness{}, fmt.Errorf("case %q: coverage: %w", c.Name, perr)
		}
		w.Proving = proving
	}
	return outcome, w, nil
}

// failingPins returns the enforce obligation rule names a case STRUCTURALLY pins as
// firing: an ExpectFinding whose Rule is in enforceObl and whose Effect is non-empty
// (a structured safety assertion). A `message~`-only finding (Effect empty) is a
// PRESENTATION assertion and is IGNORED (REQ-E6-S07-02 — `--coverage` counts only
// decision/rule/effect/score, never `message~`). For a PASSING case a pinned finding
// necessarily fired (else the must-contain matcher would have reported it missing),
// so a structured pin is a faithful failing-polarity witness.
func failingPins(exp Expectation, enforceObl map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, ef := range exp.Findings {
		if ef.Effect == "" {
			continue // message~-only presentation entry: not a structured safety assertion
		}
		if !enforceObl[ef.Rule] {
			continue
		}
		if seen[ef.Rule] {
			continue
		}
		seen[ef.Rule] = true
		out = append(out, ef.Rule)
	}
	sort.Strings(out)
	return out
}

// provingSilent returns the enforce obligation rule names PROVEN SILENT by a
// decidable case: those whose match domain selects >=1 change in the case (re-derived
// faithfully from the pack's own rule Match) AND that did NOT fire in the engine
// Result. A satisfied obligation is silent (aggregate.coverSubject returns
// contributes==false ONLY when the obligation is satisfied for every matched
// subject), so matched-AND-not-fired == proven for this case.
//
// Fail-safe: a rule that did not MATCH any change earns NO proving credit — it was
// not exercised, so its silence proves nothing (the load-bearing guard against a
// false "covered"). A rule that fired is failing, not proving, and is excluded here.
// A match-derivation defect errors the call (fail-closed).
func provingSilent(rules []policy.Rule, changes []aggregate.EvalChange, res aggregate.Result, enforceObl map[string]bool) ([]string, error) {
	fired := map[string]bool{}
	for i := range res.Findings {
		fired[res.Findings[i].Rule] = true
	}
	seen := map[string]bool{}
	var out []string
	for i := range rules {
		r := rules[i]
		if !enforceObl[r.Name] {
			continue
		}
		matched, merr := ruleMatchesAny(r.Match, changes)
		if merr != nil {
			return nil, fmt.Errorf("rule %q: %w", r.Name, merr)
		}
		if !matched {
			continue // not exercised in this case: no proving credit (fail-safe)
		}
		if fired[r.Name] {
			continue // fired -> failing polarity, never counted as proving
		}
		if seen[r.Name] {
			continue
		}
		seen[r.Name] = true
		out = append(out, r.Name)
	}
	sort.Strings(out)
	return out, nil
}

// kindModify is the change.Kind string the values domain implies (matches
// aggregate.kindModify — declared locally to avoid importing internal/change here).
const kindModify = "modify"

// ruleMatchesAny reports whether the rule's match domain selects >=1 change. It
// MIRRORS aggregate.matchChanges (internal/core/aggregate/coverage.go) — the SAME
// files/values/valueChanges glob semantics + the same implied kind:modify for the
// values domain — so the harness's "did this rule apply" agrees with what the engine
// actually evaluated. An unsupported (fileEvents) or absent domain is an ERROR
// (fail-closed), exactly as the engine's matchChanges errors: reaching it means a
// policy defect the loader should have rejected, never a fail-open "matched nothing".
func ruleMatchesAny(m policy.Match, changes []aggregate.EvalChange) (bool, error) {
	switch {
	case m.FileEvents != nil:
		return false, fmt.Errorf("match.fileEvents is deferred (E1 fast-follow); unsupported for coverage")
	case m.Files != nil:
		for i := range changes {
			if matchesAnyGlob(m.Files.Paths, changes[i].File) {
				return true, nil
			}
		}
		return false, nil
	case m.ValueChanges != nil:
		vc := m.ValueChanges
		for i := range changes {
			ch := changes[i]
			if !matchesAnyGlob(vc.Pointers, ch.Path) {
				continue
			}
			if len(vc.Kinds) > 0 && !containsStr(vc.Kinds, ch.Kind) {
				continue
			}
			if len(vc.Paths) > 0 && !matchesAnyGlob(vc.Paths, ch.File) {
				continue
			}
			return true, nil
		}
		return false, nil
	case m.Values != nil:
		v := m.Values
		// At least one selector must be present, else the domain matches nothing
		// (fail-closed, never a wildcard) — matches aggregate.matchChanges.
		if len(v.Pointers) == 0 && len(v.Paths) == 0 {
			return false, fmt.Errorf("match.values declares neither pointers nor paths")
		}
		for i := range changes {
			ch := changes[i]
			if ch.Kind != kindModify {
				continue
			}
			if len(v.Pointers) > 0 && !matchesAnyGlob(v.Pointers, ch.Path) {
				continue
			}
			if len(v.Paths) > 0 && !matchesAnyGlob(v.Paths, ch.File) {
				continue
			}
			return true, nil
		}
		return false, nil
	default:
		return false, fmt.Errorf("rule match declares no supported domain (files, values, or valueChanges)")
	}
}

// matchesAnyGlob reports whether s matches any of the glob patterns (mirrors
// aggregate.matchesAnyGlob, over the same internal/glob engine).
func matchesAnyGlob(patterns []string, s string) bool {
	for _, p := range patterns {
		if glob.Match(p, s) {
			return true
		}
	}
	return false
}

// containsStr reports exact-string set membership.
func containsStr(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
