// Package catalogue is assent's pure generated rule catalogue (D-017 B10). It
// walks the E2-loaded policy.* types (packs + bindings + config) into a
// deterministic, additive-tolerant Catalogue report: the single machine-readable
// source for generated docs and any second-order tooling, so there is never a
// hand-maintained rule registry that can drift from the authored packs.
//
// Purity: Build, MergePolicyForProfile, and CombinePolicies are pure — no
// clock/env/net/random. LoadFromDir performs the `.assent/**` walk + E2 strict
// loader calls (PCS-S01 / D-112); cmd/assent subcommands import it for catalogue,
// test, and compare activation. Guarded by internal/core/purity_test.go (which
// scans internal/catalogue for impure selectors) and TestCatalogueDoubleRunStable.
//
// Decide-and-log (E3-S07, judgment calls — see D-048):
//
//	(a) The catalogue is emitted by a DISTINCT `assent catalogue <dir>`
//	    subcommand (JSON to stdout), NOT `assent lint --catalogue`: catalogue
//	    generation is not a pass/fail gate, so conflating it with `assent lint`'s
//	    exit-code semantics muddies both.
//
//	(b) docs.url is GENERATED from the stable ID (<DocsBase>/<pack>/<rule>), NOT
//	    read from an authored field. ADR-0012's authored `docs: {url, summary}`
//	    envelope never entered the frozen v1alpha1 merge-policy schema
//	    (additionalProperties:false), so the loader would reject an authored
//	    `docs:` — the catalogue MINTS the canonical docs anchor per rule.
//
//	(c) NO deprecation/lifecycle metadata. The frozen v1alpha1 contract carries no
//	    such field on any schema, and `phase: off` is the ENTRY state of the
//	    off→observe→enforce rollout (ADR-0018 §1) — every newborn/pre-rollout rule
//	    — so inferring "deprecated" from it would publish new rules as retired.
//	    The catalogue surfaces the phase FAITHFULLY (authored Phase +
//	    ceiling-capped EffectivePhase) and fabricates no lifecycle state;
//	    deprecation metadata is deferred to a future schema lifecycle field
//	    (spec OQ, was REQ-E3-S07-03).
package catalogue

import (
	"regexp"
	"sort"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// DocsBase is the docs-site root the generated docs.url is minted under (the
// D-044 MkDocs site). A rule's docs.url is DocsBase + "/" + its stable ID.
const DocsBase = "https://platformrelay.github.io/Assent/rules"

// CapabilityApprovalEvidence is the capability a require-review rule needs: it is
// satisfiable only by forge-proven ApprovalEvidence (ADR-0017 §3), never a bare
// predicate — so the catalogue surfaces it as a required capability, distinct
// from the provider-supplied facts.
const CapabilityApprovalEvidence = "forge-approval-evidence"

// Catalogue is the generated, additive-tolerant rule catalogue. Rules is keyed by
// each entry's stable ID and canonically sorted, so adding a rule EXTENDS the
// report without reordering or renumbering any pre-existing entry (D-017 B10).
type Catalogue struct {
	Rules []RuleEntry `json:"rules"`
}

// RuleEntry is one catalogued rule — the D-017 B10 field set, derived wholly from
// the loaded packs/bindings/config (no handwritten registry). Every slice field
// is non-nil (an empty [] rather than null) and canonically sorted+deduped so the
// serialized report is byte-identical across runs.
type RuleEntry struct {
	// ID is the stable identity: "<pack>/<rule>". A deterministic function of
	// pack + rule name, NOT insertion order — the additive-tolerance anchor.
	ID   string `json:"id"`
	Pack string `json:"pack"`
	Rule string `json:"rule"`
	// DocsURL is the minted docs anchor (DocsBase + "/" + ID); see D-048(b).
	DocsURL string `json:"docsUrl"`
	// Phase is the AUTHORED rollout phase (off | observe | enforce), verbatim from
	// the rule — what the pack author declared, before any pack-ceiling cap.
	Phase string `json:"phase"`
	// EffectivePhase is the evaluation-time phase = min(rule.phase, pack ceiling)
	// on the off < observe < enforce order (ADR-0018 §1; the same cap
	// aggregate.CoverWithPhaseCeiling applies). Equal to Phase when the pack has no
	// manifest (absent ⇒ enforce, no cap). This is the phase that actually governs
	// whether the rule contributes to a decision — the truthful docs surface.
	EffectivePhase string `json:"effectivePhase"`
	// Obligation is the proof this rule contributes (prove.obligation), empty for
	// a non-obligation effect rule.
	Obligation string `json:"obligation,omitempty"`
	// RequiredFacts are the provider-supplied fact references the rule's when-tree
	// (and message) address, as dotted paths minus the `facts.` prefix
	// (e.g. "author.groups"). Sound because E3-S03 guarantees dot-syntax facts refs.
	RequiredFacts []string `json:"requiredFacts"`
	// Capabilities are non-fact capability requirements — today the forge
	// ApprovalEvidence a require-review posture needs (ADR-0017 §3).
	Capabilities []string `json:"capabilities"`
	// Classes are the change classes that route to this rule's pack, from the
	// binding graph (binding.class where binding.packs contains this pack).
	Classes []string `json:"classes"`
	// MatchDomains are the matcher domains the rule uses (files | values |
	// valueChanges | fileEvents), ADR-0017 §5.
	MatchDomains []string `json:"matchDomains"`
	// FindingCodes are the stable finding codes the rule can emit (onFailure.code).
	FindingCodes []string `json:"findingCodes"`
	// Effects are the possible outcomes: onFailure.effect and/or rule.effect.
	Effects []string `json:"effects"`
}

// Pack is one loaded pack: its name (the `packs/<name>/` directory token, which
// binding.packs[] references — the load-bearing join key), its MergePolicy
// documents, and an optional pack.yaml manifest. The name is threaded from the
// cmd/assent walk so the pure catalogue never touches the filesystem.
type Pack struct {
	Name     string
	Policies []*policy.MergePolicy
	Manifest *policy.Pack
}

// Input is the fully-loaded authoring surface the catalogue walks. Config is not
// among the D-017 B10 field derivations — a rule's classes come from the binding
// graph (binding.class), which carries the class NAME directly — so Config is
// deliberately absent; a later story that needs config-derived fields adds it then.
type Input struct {
	Packs    []Pack
	Bindings []*policy.RulesetBinding
}

// factRefRe matches a dot-syntax facts reference (facts.<provider>[.<path>...]).
// E3-S03's facts-reference lint guarantees authored refs are dot-syntax, so this
// scan is sound by construction (no bracket/whitespace forms to miss).
var factRefRe = regexp.MustCompile(`facts\.[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*`)

// Build walks the loaded packs/bindings/config into the deterministic Catalogue.
// It is pure: the same Input always yields a byte-identical report.
func Build(in Input) Catalogue {
	classIndex := indexClassesByPack(in.Bindings)

	var rules []RuleEntry
	for _, pk := range in.Packs {
		ceiling := packCeiling(pk.Manifest)
		for _, mp := range pk.Policies {
			if mp == nil {
				continue
			}
			for i := range mp.Spec.Rules {
				rules = append(rules, buildEntry(pk.Name, &mp.Spec.Rules[i], classIndex[pk.Name], ceiling))
			}
		}
	}

	sort.SliceStable(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return Catalogue{Rules: rules}
}

// buildEntry derives one RuleEntry from a loaded rule, the pack it belongs to, the
// classes that route to that pack, and the pack's phase ceiling.
func buildEntry(pack string, r *policy.Rule, classes []string, ceiling policy.Phase) RuleEntry {
	id := pack + "/" + r.Name
	e := RuleEntry{
		ID:             id,
		Pack:           pack,
		Rule:           r.Name,
		DocsURL:        DocsBase + "/" + id,
		Phase:          string(r.Phase),
		EffectivePhase: string(minPhase(r.Phase, ceiling)),
		RequiredFacts:  []string{},
		Capabilities:   []string{},
		Classes:        sortedDedup(classes),
		MatchDomains:   matchDomains(r.Match),
		FindingCodes:   []string{},
		Effects:        []string{},
	}

	var facts, effects, codes []string
	if r.Prove != nil {
		e.Obligation = r.Prove.Obligation
		facts = append(facts, collectFacts(&r.Prove.When)...)
	}
	facts = append(facts, factsIn(r.Message)...)
	if r.OnFailure != nil {
		if r.OnFailure.Effect != "" {
			effects = append(effects, string(r.OnFailure.Effect))
		}
		if r.OnFailure.Effect == policy.EffectRequireReview {
			e.Capabilities = append(e.Capabilities, CapabilityApprovalEvidence)
		}
		if r.OnFailure.Code != "" {
			codes = append(codes, r.OnFailure.Code)
		}
	}
	if r.Effect != "" {
		effects = append(effects, string(r.Effect))
	}

	e.RequiredFacts = sortedDedup(facts)
	e.Effects = sortedDedup(effects)
	e.FindingCodes = sortedDedup(codes)

	// NB: v1alpha1 carries NO lifecycle/deprecated field on any frozen schema, so
	// the catalogue surfaces no deprecation metadata — inventing it from `phase:
	// off` would be affirmatively wrong (off is the ENTRY state of the off →
	// observe → enforce rollout, ADR-0018 §1, i.e. every newborn rule), publishing
	// pre-rollout rules as retired. Deprecation is deferred to a future schema
	// lifecycle field (spec OQ). Phase/EffectivePhase are the honest surface.
	return e
}

// packCeiling returns the pack-manifest phase ceiling; an absent manifest means
// no cap (enforce), matching aggregate.CoverWithPhaseCeiling's default.
func packCeiling(m *policy.Pack) policy.Phase {
	if m == nil {
		return policy.PhaseEnforce
	}
	return m.Spec.Phase
}

// minPhase caps a rule's authored phase by the pack ceiling on the off < observe
// < enforce order — the same never-additive cap the evaluator applies.
func minPhase(rule, ceiling policy.Phase) policy.Phase {
	if phaseRank(ceiling) < phaseRank(rule) {
		return ceiling
	}
	return rule
}

// phaseRank orders the rollout phases; an unknown/empty value ranks highest so it
// never caps a valid phase (a post-load rule phase is always one of the three).
func phaseRank(p policy.Phase) int {
	switch p {
	case policy.PhaseOff:
		return 0
	case policy.PhaseObserve:
		return 1
	case policy.PhaseEnforce:
		return 2
	default:
		return 3
	}
}

// indexClassesByPack inverts the binding graph into pack -> the classes routed to
// it. Reads bindings.packs[] against binding.class (ADR-0008 routing).
func indexClassesByPack(bindings []*policy.RulesetBinding) map[string][]string {
	idx := map[string][]string{}
	for _, rb := range bindings {
		if rb == nil {
			continue
		}
		for _, b := range rb.Bindings {
			for _, pk := range b.Packs {
				idx[pk] = append(idx[pk], b.Class)
			}
		}
	}
	return idx
}

// collectFacts walks an AssertTree recursively (leaves nest under all/any/not) and
// returns every provider-fact reference across every leaf's CEL.
func collectFacts(t *policy.AssertTree) []string {
	if t == nil {
		return nil
	}
	var out []string
	if t.Leaf != nil {
		out = append(out, factsIn(t.Leaf.CEL)...)
		out = append(out, factsIn(t.Leaf.Message)...)
	}
	for i := range t.All {
		out = append(out, collectFacts(&t.All[i])...)
	}
	for i := range t.Any {
		out = append(out, collectFacts(&t.Any[i])...)
	}
	out = append(out, collectFacts(t.Not)...)
	return out
}

// factsIn extracts dot-syntax facts references from a CEL/template string and
// returns each as its dotted path minus the `facts.` prefix (e.g. "author.groups").
func factsIn(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, m := range factRefRe.FindAllString(s, -1) {
		out = append(out, m[len("facts."):])
	}
	return out
}

// matchDomains returns the matcher domains a rule uses (ADR-0017 §5), sorted.
func matchDomains(m policy.Match) []string {
	var out []string
	if m.Files != nil {
		out = append(out, "files")
	}
	if m.Values != nil {
		out = append(out, "values")
	}
	if m.ValueChanges != nil {
		out = append(out, "valueChanges")
	}
	if m.FileEvents != nil {
		out = append(out, "fileEvents")
	}
	return sortedDedup(out)
}

// sortedDedup returns a NON-NIL, canonically sorted, de-duplicated copy — the
// determinism primitive every derived slice passes through so map-iteration or
// authoring order never leaks into the report.
func sortedDedup(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
