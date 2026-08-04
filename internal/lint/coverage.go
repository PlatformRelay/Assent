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
	"sort"
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
// strict-loader error made deterministic: the absolute schema URI is stripped
// (normalizeLoaderError) AND the sibling cause lines are canonicalized into a
// stable order (canonicalizeSchemaError). The latter is REQUIRED because the
// jsonschema validator emits a multi-cause error's sibling causes in
// map-iteration (non-deterministic) order — a doc with two defects (e.g. a
// missing phase AND a missing identity) would otherwise render its schema-invalid
// message in a different line order run-to-run, breaking the diagnostic model's
// determinism (REQ-E3-S01-05 / REQ-E3-S02-04) and S08's cross-environment golden.
func schemaInvalid(rep *Report, path string, err error) {
	rep.addError(CodeSchemaInvalid, Location{File: path}, canonicalizeSchemaError(normalizeLoaderError(err.Error())))
}

// canonicalizeSchemaError re-orders the sibling cause lines of a jsonschema error
// into a deterministic order. The validator renders causes as an indentation tree
// ("- at '<loc>': <detail>", nested causes indented two spaces further) but emits
// siblings in non-deterministic map order; this parses that tree by indentation
// and sorts siblings (recursively) by their rendered subtree, preserving the
// header line first. Pure string transform — no clock/rand/env/net.
func canonicalizeSchemaError(msg string) string {
	lines := strings.Split(msg, "\n")
	if len(lines) <= 1 {
		return msg
	}
	roots := parseIndentForest(lines[1:])
	sortIndentForest(roots)
	var b strings.Builder
	b.WriteString(lines[0])
	for _, r := range roots {
		b.WriteString("\n")
		b.WriteString(renderIndentNode(r))
	}
	return b.String()
}

// indentNode is one line of a jsonschema error plus its more-indented children.
type indentNode struct {
	line     string
	indent   int
	children []*indentNode
}

// parseIndentForest builds the indentation forest of a jsonschema error's cause
// lines: each line is a child of the nearest preceding line with a smaller
// indent. Blank lines are skipped.
func parseIndentForest(lines []string) []*indentNode {
	var roots, stack []*indentNode
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		ind := len(ln) - len(strings.TrimLeft(ln, " "))
		n := &indentNode{line: ln, indent: ind}
		for len(stack) > 0 && stack[len(stack)-1].indent >= ind {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, n)
		} else {
			top := stack[len(stack)-1]
			top.children = append(top.children, n)
		}
		stack = append(stack, n)
	}
	return roots
}

// sortIndentForest sorts siblings (recursively, children first) by their rendered
// subtree, giving a total canonical order independent of the validator's emission
// order.
func sortIndentForest(nodes []*indentNode) {
	for _, n := range nodes {
		sortIndentForest(n.children)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return renderIndentNode(nodes[i]) < renderIndentNode(nodes[j])
	})
}

// renderIndentNode renders a node and its (already-sorted) children back to text.
func renderIndentNode(n *indentNode) string {
	var b strings.Builder
	b.WriteString(n.line)
	for _, c := range n.children {
		b.WriteString("\n")
		b.WriteString(renderIndentNode(c))
	}
	return b.String()
}

// removeSchemaInvalidForPhase is the E3-S02 phase double-count dedupe hook (see
// structural.go): when no-implicit-enforce-phase fires for a doc, the same
// missing-phase defect the strict loader captured as a schema-invalid on that doc
// is dropped — but ONLY when the missing phase is that schema-invalid's SOLE
// cause. A doc that is missing phase AND has an unrelated schema violation keeps
// its schema-invalid in full, so the co-located defect is never hidden
// (fail-many preserved). Idempotent: firing for several rules of one doc removes
// the single schema-invalid at most once.
func (r *Report) removeSchemaInvalidForPhase(path string) {
	kept := r.diags[:0]
	for _, d := range r.diags {
		if d.Code == CodeSchemaInvalid && d.Location.File == path && isPhaseOnlyLoaderError(d.Message) {
			continue // drop: the actionable no-implicit-enforce-phase supersedes it
		}
		kept = append(kept, d)
	}
	r.diags = kept
}

// isPhaseOnlyLoaderError reports whether a normalized strict-loader message
// attributes the refusal SOLELY to one or more missing `phase` properties. The
// jsonschema validator bundles ALL failures into one message as `- at '<loc>':
// <detail>` lines (with `validation failed` wrapper lines around nested causes);
// the error is phase-only iff at least one leaf detail is `missing property
// 'phase'` and no leaf detail is anything else. A non-empty INVALID phase yields
// `value must be one of ...` (an enum violation), which this correctly does NOT
// treat as phase-only, so it is never deduped.
func isPhaseOnlyLoaderError(msg string) bool {
	hasPhase := false
	for _, line := range strings.Split(msg, "\n") {
		// A complaint line is `... at '<loc>': <detail>`; the loc is a
		// space-free JSON pointer, so the first "': " reliably splits loc/detail.
		// Lines without it (the header) are not complaints.
		idx := strings.Index(line, "': ")
		if idx < 0 {
			continue
		}
		switch strings.TrimSpace(line[idx+len("': "):]) {
		case "validation failed":
			// A nesting wrapper around child causes — not itself a leaf complaint.
		case "missing property 'phase'":
			hasPhase = true
		default:
			return false // some other defect — not phase-only, keep the schema-invalid
		}
	}
	return hasPhase
}

// model is the tolerant-ingestion result: the loaded bindings (with their source
// file) and the packs keyed by pack name (the directory segment under packs/),
// each carrying its rules. It is intentionally minimal for S01; later stories
// widen it (config posture, tests-on-disk) additively.
type model struct {
	bindings []boundBinding
	packs    map[string]*loadedPack
	// docs is the E3-S02 per-document view (source path + rules + entries + pack
	// phase) the structural checks consume. It is populated alongside packs by
	// ingest; see structural.go's ingestedDoc for why it is a separate view.
	docs []ingestedDoc
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
			ingestPack(s, m, rep)
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
		// The E3-S02 per-doc view: rules + entries with this doc's real path.
		m.docs = append(m.docs, ingestedDoc{path: s.Path, rules: mp.Spec.Rules, entries: mp.Spec.Entries})
	}
	if _, lerr := policy.LoadMergePolicy(s.Bytes); lerr != nil {
		schemaInvalid(rep, s.Path, lerr)
	}
}

// ingestPack tolerantly decodes a Pack manifest into the E3-S02 per-doc view
// (its spec.phase + name, for no-implicit-enforce-phase) AND runs the strict
// loader, capturing any refusal as one schema-invalid diagnostic.
func ingestPack(s Source, m *model, rep *Report) {
	var pk policy.Pack
	if err := yaml.Unmarshal(s.Bytes, &pk); err == nil {
		m.docs = append(m.docs, ingestedDoc{path: s.Path, isPack: true, packPhase: pk.Spec.Phase, packName: pk.Metadata.Name})
	}
	if _, lerr := policy.LoadPack(s.Bytes); lerr != nil {
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
