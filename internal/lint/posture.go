package lint

// posture.go is the E3-S05 config-posture layer: two hard errors over the
// authored config.yaml graph, both reusing an E2 primitive rather than
// re-deriving it.
//
// # 1. fail-open (WIDENED) — a controlling/authorization provider configured
// `failure: open` (ADR-0017 §6: "controlling facts never fail open").
//
// A provider is only ever discoverably referenced as `facts.<provider>.<name>`
// in a rule's prove.when (plain-effect rules carry no `when`; a message is
// presentation, not control). E3-S03 guarantees every such reference is
// contiguous dot syntax, so the `facts.<provider>` scan is sound by construction
// — the exact soundness the E2-S05 provider-posture scan relies on. What makes a
// referencing rule *controlling* splits into the three archetypes
// lint-hard-errors.md names, each detected independently:
//
//	(a) require-review-proof — the rule's prove escalates to require-review on
//	    failure. This is EXACTLY what policy.ValidateProviderPosture already
//	    catches; we REUSE it verbatim (its header marks widening as "E3's assent
//	    lint"), never re-implementing the require-review-proof detection.
//	(b) entries-identity — the rule's prove.when reads the `entry`/`oldEntry`
//	    predicate scope: it authorizes a change against an identified governed
//	    entry (the pack declares entries with an identity.pointer; ADR-0017 §5), so
//	    a provider co-referenced there resolves that entry's authorization. The
//	    E2-S05 scan cannot reach this — it keys only on the require-review effect.
//	(c) approval-eligibility — the rule proves an obligation a RulesetBinding
//	    lists in require[] (a REQUIRED authorization obligation, ADR-0017 §2/§3):
//	    the provider feeds an approval-eligibility gate even when onFailure is a
//	    plain block. Also beyond the E2-S05 require-review key.
//
// An ADVISORY provider — referenced only by a non-controlling proof (a plain
// comment/challenge/block obligation that is neither entry-scoped nor required)
// — MAY be configured failure: open and lints clean (ADR-0017 §6 restricts only
// controlling facts). A provider not declared in config.providers has no failure
// posture and is skipped.
//
// Archetypes (b)/(c) are deduped by provider name within a config (one fail-open
// per provider); (a) is reused as-is (policy.ValidateProviderPosture reports the
// first controlling require-review-proof provider — its documented limitation).
//
// # 2. single-writer-profile — per (environment, class) binding, exactly one
// covering write-authoritative PolicyProfile (ADR-0018 §2, D-017 B3).
//
// This REUSES aggregate.ResolveProfile, which resolves the covering profile via
// the Config.profiles PRECEDENCE TABLE (never Go map order) and fails closed when
// two covering profiles declare writes:true (the single-writer invariant lives in
// policy.CoveringProfiles). The lint layer only classifies the result: an error
// (>1 covering writer, or a dangling precedence ref) or a binding with no
// resolved write authority (zero writers) → single-writer-profile; a single
// resolved writer → clean. The check runs only when Config.profiles is present
// (its declared scope). Never last-one-wins.
//
// Purity: this file adds no clock/rand/env/net; it imports policy + aggregate
// (both pure) and cel-go's PARSER only (as facts_ref.go does — a purity-safe
// import, TestCorePurity forbids only the impure selectors). Config + Profile
// documents are re-decoded tolerantly from the already-read source bytes; the
// model carries neither (ingest strict-validates a Config then discards it and
// skips PolicyProfile docs), so posture reaches to `sources` for exactly those
// two kinds and uses the model for rules/entries.

import (
	"fmt"
	"sort"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// Diagnostic codes owned by the config-posture checks (defined here so lint.go's
// only S05 edit is the single checkConfigPosture call-site).
const (
	// CodeFailOpen: a controlling/authorization provider configured failure: open
	// (ADR-0017 §6, lint-hard-errors.md).
	CodeFailOpen = "fail-open"
	// CodeSingleWriterProfile: a (environment, class) binding with zero or more
	// than one covering write-authoritative PolicyProfile (ADR-0018 §2, D-017 B3).
	CodeSingleWriterProfile = "single-writer-profile"
)

// failureOpen is the Provider.failure posture a controlling fact may never carry.
const failureOpen = "open"

// configDoc is one tolerantly re-decoded Config paired with its source path.
type configDoc struct {
	path string
	cfg  *policy.Config
}

// checkConfigPosture runs the two E3-S05 hard errors. It reaches to `sources`
// only for the Config + PolicyProfile documents (which the model does not carry)
// and uses the model for the loaded rules/bindings. Wired into Lint() (lint.go).
func checkConfigPosture(sources []Source, m *model, rep *Report) {
	configs := parsePostureConfigs(sources)
	profiles := parsePostureProfiles(sources)
	checkFailOpen(configs, m, rep)
	checkSingleWriterProfile(configs, profiles, m, rep)
}

// parsePostureConfigs tolerantly re-decodes every Config document from sources
// (ingest strict-validated and discarded them). A malformed Config already
// produced a schema-invalid diagnostic at ingest, so a decode failure here is
// simply skipped — never double-reported.
func parsePostureConfigs(sources []Source) []configDoc {
	var out []configDoc
	for _, s := range sources {
		kind, err := docKind(s.Bytes)
		if err != nil || kind != "Config" {
			continue
		}
		var c policy.Config
		if uerr := yaml.Unmarshal(s.Bytes, &c); uerr != nil {
			continue
		}
		out = append(out, configDoc{path: s.Path, cfg: &c})
	}
	return out
}

// parsePostureProfiles tolerantly re-decodes every PolicyProfile document from
// sources (ingest skips the PolicyProfile kind entirely). The kind const is
// PolicyProfile — matching profile.schema.json — not Profile.
func parsePostureProfiles(sources []Source) []*policy.Profile {
	var out []*policy.Profile
	for _, s := range sources {
		kind, err := docKind(s.Bytes)
		if err != nil || kind != "PolicyProfile" {
			continue
		}
		var p policy.Profile
		if uerr := yaml.Unmarshal(s.Bytes, &p); uerr != nil {
			continue
		}
		out = append(out, &p)
	}
	return out
}

// checkFailOpen emits fail-open for each controlling provider configured
// failure: open, across all three archetypes. See the file header.
func checkFailOpen(configs []configDoc, m *model, rep *Report) {
	rules := allRuleRefs(m)
	required := requiredObligations(m)
	mp := combinedMergePolicy(rules)
	// A declaration-free parse env: only Check consults declarations, so this
	// parses any syntactically valid CEL leaf. Built once (as facts_ref.go does).
	env, envErr := cel.NewEnv()

	for _, cd := range configs {
		cfg := cd.cfg

		// Archetype (a): REUSE the E2-S05 scan verbatim for the require-review-proof
		// case (do not re-implement provider-posture). It reports the first such
		// provider; its message names the provider, obligation, and rule.
		if err := policy.ValidateProviderPosture(cfg, mp); err != nil {
			rep.addError(CodeFailOpen, Location{File: cd.path},
				err.Error()+" [archetype: require-review-proof, via policy.ValidateProviderPosture (E2-S05)]")
		}

		// Archetypes (b) entries-identity + (c) approval-eligibility: the widening.
		flagged := map[string]bool{}
		for _, rr := range rules {
			r := rr.rule
			if r.Prove == nil {
				continue
			}
			entryScoped := whenReadsEntryScope(env, envErr, r.Prove.When)
			requiredOb := required[r.Prove.Obligation]
			if !entryScoped && !requiredOb {
				continue // this rule confers no (b)/(c) control
			}
			for _, prov := range providersInWhen(r.Prove.When) {
				p, ok := cfg.Providers[prov]
				if !ok || p.Failure != failureOpen {
					continue
				}
				if flagged[prov] {
					continue
				}
				flagged[prov] = true
				if entryScoped {
					rep.addError(CodeFailOpen, Location{File: cd.path, Name: prov}, fmt.Sprintf(
						"provider %q is a controlling entries-identity provider — its facts.%s.* are read by proof rule %q whose prove.when authorizes an identified governed entry via the entry/oldEntry scope (ADR-0017 §5) — and may not be configured failure: open; a controlling fact must fail closed (ADR-0017 §6, lint-hard-errors.md)",
						prov, prov, ruleLabel(r, rr.idx)))
				} else {
					rep.addError(CodeFailOpen, Location{File: cd.path, Name: prov}, fmt.Sprintf(
						"provider %q is a controlling approval-eligibility provider — its facts.%s.* prove obligation %q, which a RulesetBinding lists in require[] (a required authorization obligation, ADR-0017 §2) — and may not be configured failure: open; a controlling fact must fail closed (ADR-0017 §6, lint-hard-errors.md)",
						prov, prov, r.Prove.Obligation))
				}
			}
		}
	}
}

// checkSingleWriterProfile emits single-writer-profile for any (environment,
// class) binding that does not resolve to exactly one covering write-authoritative
// PolicyProfile, when Config.profiles is present (ADR-0018 §2). Resolution is the
// reused aggregate.ResolveProfile (precedence-table driven, fail-closed on the
// two-writer invariant).
func checkSingleWriterProfile(configs []configDoc, profiles []*policy.Profile, m *model, rep *Report) {
	for _, cd := range configs {
		if len(cd.cfg.Profiles) == 0 {
			continue // the check's declared scope is "Config.profiles present"
		}
		for _, bb := range m.bindings {
			env, class := bb.binding.Environment, bb.binding.Class
			loc := Location{File: bb.file, Name: bindingName(bb.binding)}
			rp, resolved, err := aggregate.ResolveProfile(cd.cfg.Profiles, profiles, env, class)
			if err != nil {
				// >1 covering writer (single-writer invariant) or a dangling ref —
				// fail-closed, never last-one-wins.
				rep.addError(CodeSingleWriterProfile, loc, fmt.Sprintf(
					"binding (class=%q, environment=%q): %v — never last-one-wins (ADR-0018 §2, D-017 B3)",
					class, env, err))
				continue
			}
			if !resolved || !rp.Writes {
				rep.addError(CodeSingleWriterProfile, loc, fmt.Sprintf(
					"binding (class=%q, environment=%q) resolves no single writes:true PolicyProfile from the Config.profiles precedence table; with Config.profiles present a binding must have exactly one covering write-authoritative profile (zero writers is rejected fail-closed, never last-one-wins) (ADR-0018 §2, D-017 B3)",
					class, env))
			}
		}
	}
}

// ruleRef is one loaded rule with its source doc path and index (for a stable
// Location label). Rules are gathered from the tolerant per-doc view (model.docs)
// so each carries its real authoring file.
type ruleRef struct {
	file string
	idx  int
	rule policy.Rule
}

// allRuleRefs returns every MergePolicy rule in the model paired with its doc.
func allRuleRefs(m *model) []ruleRef {
	var out []ruleRef
	for _, d := range m.docs {
		if d.isPack {
			continue
		}
		for i := range d.rules {
			out = append(out, ruleRef{file: d.path, idx: i, rule: d.rules[i]})
		}
	}
	return out
}

// combinedMergePolicy assembles a MergePolicy carrying every loaded rule, the
// shape policy.ValidateProviderPosture consumes (it reads only Spec.Rules).
func combinedMergePolicy(rules []ruleRef) *policy.MergePolicy {
	mp := &policy.MergePolicy{}
	for _, rr := range rules {
		mp.Spec.Rules = append(mp.Spec.Rules, rr.rule)
	}
	return mp
}

// requiredObligations is the union of every binding's require[] — the set of
// authorization obligations (archetype (c)'s approval-eligibility key).
func requiredObligations(m *model) map[string]bool {
	req := map[string]bool{}
	for _, bb := range m.bindings {
		for _, o := range bb.binding.Require {
			req[o] = true
		}
	}
	return req
}

// providersInWhen returns, sorted, the providers a rule's prove.when references
// as facts.<provider>. It reuses leafCELs + runtimeFactRefRe (facts_ref.go), the
// exact per-rule union scope policy.providersReferenced walks — sound because
// E3-S03 guarantees contiguous dot syntax.
func providersInWhen(tree policy.AssertTree) []string {
	seen := map[string]bool{}
	for _, c := range leafCELs(tree) {
		for _, mm := range runtimeFactRefRe.FindAllStringSubmatch(c, -1) {
			seen[mm[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// whenReadsEntryScope reports whether any leaf of a rule's prove.when references
// the `entry` or `oldEntry` predicate-scope identifier — the archetype (b) signal
// that the rule authorizes a change against an identified governed entry. It
// parses each leaf with cel-go's parser and inspects the AST (a string literal
// containing "entry" is a constant node, never an Ident, so it never false-fires;
// consistent with facts_ref.go's evasion-proof approach). A leaf that cannot be
// parsed is left to facts_ref.go's parse-error diagnostic and does not confer (b).
func whenReadsEntryScope(env *cel.Env, envErr error, tree policy.AssertTree) bool {
	if env == nil || envErr != nil {
		return false
	}
	for _, c := range leafCELs(tree) {
		ast, iss := env.Parse(c)
		if iss != nil && iss.Err() != nil {
			continue
		}
		if astReferencesEntry(ast) {
			return true
		}
	}
	return false
}

// astReferencesEntry walks a parsed leaf for an Ident node named `entry` or
// `oldEntry`.
func astReferencesEntry(ast *cel.Ast) bool {
	found := false
	root := celast.NavigateAST(ast.NativeRep())
	var walk func(n celast.NavigableExpr)
	walk = func(n celast.NavigableExpr) {
		if found {
			return
		}
		if n.Kind() == celast.IdentKind {
			if id := n.AsIdent(); id == "entry" || id == "oldEntry" {
				found = true
				return
			}
		}
		for _, c := range n.Children() {
			walk(c)
		}
	}
	walk(root)
	return found
}
