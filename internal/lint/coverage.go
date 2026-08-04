package lint

// coverage.go is the E3-S01 obligation-coverage hard error plus the tolerant
// ingestion it consumes. Ingestion decodes each `.assent/**` doc into the E2
// policy.* types BEST-EFFORT and, separately, runs the strict E2 loader capturing
// any refusal as one schema-invalid diagnostic (see lint.go decision (b)).
//
// Obligation coverage reuses the E2-S04 vocabulary (aggregate/coverage.go): a
// Binding.Require[] obligation is proven by a rule whose prove.obligation names
// it. Here it is a STATIC SET check over the loaded types — no EvaluationInput, no
// evaluation, no matching. Deliberate divergence from E2's cover(): that loop
// marks an obligation covered ONLY at PhaseEnforce (coverage.go line ~150), but
// this static lint IGNORES phase — the grounding defines coverage as
// "prove.obligation == <name>" and the spec calls it a static set check. Phase is
// E3-S02's own hard error (no-implicit-enforce-phase); folding it in here would
// double-count one defect under two codes. Known limitation logged accordingly:
// an obligation proven only by a phase:off rule passes THIS static check and is
// caught at S02 / runtime, not S01.

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// schemaURIRe matches an absolute file:// schema URI the frozen jsonschema
// validator embeds in its error string (e.g.
// 'file:///Users/.../merge-policy.schema.json#'). The URI is derived from the
// process working directory at runtime, so leaving it verbatim would make a
// schema-invalid diagnostic CWD- and machine-dependent — breaking the diagnostic
// model's determinism the moment lint output is byte-compared across environments
// (E3-S08's golden corpus / CI parity). We normalize it to just the schema
// basename, keeping the actionable "at '<path>': <detail>" remainder intact.
var schemaURIRe = regexp.MustCompile(`file://[^\s'"]*/([^/\s'"]+)`)

// normalizeLoaderError strips the absolute, CWD-derived schema URI out of a
// strict-loader error, leaving a stable, environment-independent message.
func normalizeLoaderError(msg string) string {
	return schemaURIRe.ReplaceAllString(msg, "$1")
}

// schemaInvalid records one schema-invalid diagnostic located to path, with the
// strict-loader error normalized (URI stripped) so it is deterministic.
func schemaInvalid(rep *Report, path string, err error) {
	rep.addError(CodeSchemaInvalid, Location{File: path}, normalizeLoaderError(err.Error()))
}

// model is the tolerant-ingestion result: the loaded bindings (with their source
// file) and the packs keyed by pack name (the directory segment under packs/),
// each carrying its rules. It is intentionally minimal for S01; later stories
// widen it (config posture, tests-on-disk) additively.
type model struct {
	bindings []boundBinding
	packs    map[string]*loadedPack
}

// boundBinding is one Binding paired with the file it was authored in.
type boundBinding struct {
	file    string
	binding policy.Binding
}

// loadedPack is a named pack's accumulated rules (tolerantly decoded).
type loadedPack struct {
	rules []policy.Rule
}

// ingest walks the sources, classifies each by its `kind`, decodes it tolerantly
// into the policy.* types, and separately runs the strict loader — appending one
// schema-invalid diagnostic per refusal (the fail-many bridge). A doc that is not
// parseable YAML yields a parse-error diagnostic; a doc with an unrecognized kind
// (e.g. a tests/** fixture) is skipped.
func ingest(sources []Source, rep *Report) *model {
	m := &model{packs: map[string]*loadedPack{}}
	for _, s := range sources {
		kind, err := docKind(s.Bytes)
		if err != nil {
			rep.addError(CodeParseError, Location{File: s.Path}, "document is not parseable YAML: "+err.Error())
			continue
		}
		switch kind {
		case "RulesetBinding":
			ingestBinding(s, m, rep)
		case "MergePolicy":
			ingestMergePolicy(s, m, rep)
		case "Config":
			// Strict-validate so a malformed config surfaces tolerantly (S05 adds
			// the posture checks over the loaded value; S01 only bridges the loader).
			if _, lerr := policy.LoadConfig(s.Bytes); lerr != nil {
				schemaInvalid(rep, s.Path, lerr)
			}
		case "Pack":
			if _, lerr := policy.LoadPack(s.Bytes); lerr != nil {
				schemaInvalid(rep, s.Path, lerr)
			}
		default:
			// Unrecognized/absent kind — a non-policy doc (a tests/** fixture, a
			// README fragment). Not lint's surface; skipped, not an error.
		}
	}
	return m
}

// ingestBinding tolerantly decodes a RulesetBinding into the model AND runs the
// strict loader, capturing any refusal as one schema-invalid diagnostic.
func ingestBinding(s Source, m *model, rep *Report) {
	var rb policy.RulesetBinding
	if err := yaml.Unmarshal(s.Bytes, &rb); err == nil {
		for _, b := range rb.Bindings {
			m.bindings = append(m.bindings, boundBinding{file: s.Path, binding: b})
		}
	}
	if _, lerr := policy.LoadRulesetBinding(s.Bytes); lerr != nil {
		schemaInvalid(rep, s.Path, lerr)
	}
}

// ingestMergePolicy tolerantly decodes a MergePolicy's rules under the pack the
// doc lives in (the segment after packs/ in its path) AND runs the strict loader.
func ingestMergePolicy(s Source, m *model, rep *Report) {
	var mp policy.MergePolicy
	if err := yaml.Unmarshal(s.Bytes, &mp); err == nil {
		name := packName(s.Path)
		p := m.packs[name]
		if p == nil {
			p = &loadedPack{}
			m.packs[name] = p
		}
		p.rules = append(p.rules, mp.Spec.Rules...)
	}
	if _, lerr := policy.LoadMergePolicy(s.Bytes); lerr != nil {
		schemaInvalid(rep, s.Path, lerr)
	}
}

// packName derives the pack a MergePolicy doc belongs to from its path: the
// segment immediately after a "packs/" segment (`.assent/packs/<name>/rules/x.yaml`
// → "<name>"). A doc not under a packs/ tree returns "" — it belongs to no named
// pack, so no binding references it (its obligations never satisfy a require[]).
func packName(path string) string {
	segs := strings.Split(path, "/")
	for i, seg := range segs {
		if seg == "packs" && i+1 < len(segs) {
			return segs[i+1]
		}
	}
	return ""
}

// checkObligationCoverage is the E3-S01 hard error: for each loaded binding, every
// name in Require[] must be prove.obligation'd by ≥1 rule in the bound packs. An
// uncovered obligation → one obligation-coverage error naming the binding
// (class, environment) and the uncovered obligation (ADR-0017 §2). Static set
// check — no evaluation.
func checkObligationCoverage(m *model, rep *Report) {
	for _, bb := range m.bindings {
		proven := provenObligations(m, bb.binding.Packs)
		for _, req := range bb.binding.Require {
			if proven[req] {
				continue
			}
			rep.addError(
				CodeObligationCoverage,
				Location{File: bb.file, Name: bindingName(bb.binding)},
				fmt.Sprintf("binding (class=%q, environment=%q) requires obligation %q, but no rule in the bound packs %v proves it (prove.obligation)",
					bb.binding.Class, bb.binding.Environment, req, bb.binding.Packs),
			)
		}
	}
}

// provenObligations is the union of prove.obligation values across every rule in
// the named packs — the static "proven set" (reused E2-S04 require↔prove mapping).
func provenObligations(m *model, packs []string) map[string]bool {
	proven := map[string]bool{}
	for _, name := range packs {
		p := m.packs[name]
		if p == nil {
			continue
		}
		for i := range p.rules {
			if pr := p.rules[i].Prove; pr != nil && pr.Obligation != "" {
				proven[pr.Obligation] = true
			}
		}
	}
	return proven
}

// bindingName is the canonical human identity of a binding for a diagnostic
// Location — its (class, environment) pair.
func bindingName(b policy.Binding) string {
	return fmt.Sprintf("class=%s environment=%s", b.Class, b.Environment)
}
