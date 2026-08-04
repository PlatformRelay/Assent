package lint

// facts_ref.go is the E3-S03 facts-reference hard error over every when/cel leaf,
// and the enforcement half of the fact-model `.value` DECISION (D-048).
//
// Detection is AST-based: each leaf is PARSED with cel-go's own parser (the same
// parser eval uses — parse only, no checker/declarations) and the parsed AST is
// walked for references to the `facts` identifier. This is deliberate and
// load-bearing: an earlier hand-rolled string-literal masker DIVERGED from
// cel-go's lexer on RAW / BYTES / TRIPLE-QUOTED strings (e.g. `r'\'` closes at its
// second quote — backslash is literal in a raw string — but the masker treated
// `\'` as an escaped quote, desynced, and masked away a real facts['owner'] token,
// emitting ZERO diagnostics while the runtime scan also missed it → a proven
// fail-open). Reading the AST cel-go actually produces removes that whole class of
// evasion: a `facts` fragment inside ANY string form is a string CONSTANT node,
// never an `facts` Ident, so it never false-positives and never hides a real ref.
//
// Two coupled defects, both closing a gap the E2 decision layer leaves open:
//
//	facts-reference-syntax — a `facts` reference not in the frozen dot form
//	    facts.<provider>.<name>. Three shapes are rejected:
//	      (A) a bracket index on facts — facts['owner'] / facts["owner"] (an INDEX
//	          Call node whose target is the facts Ident);
//	      (B) a bare `facts` — the whole map used as an identifier (size(facts),
//	          `facts == x`), naming no provider;
//	      (C) a dot reference the runtime scan would still MISS because that scan is
//	          whitespace-SENSITIVE — policy.factRefRe matches only a CONTIGUOUS
//	          `\bfacts\.<ident>`, so `facts . owner` (which cel-go's parser happily
//	          normalizes to a Select) evades it. The AST alone cannot see this — it
//	          erases whitespace — so a soundness cross-check (below) rejects any
//	          AST-detected dot provider the runtime regex does NOT capture.
//	    policy.factRefRe (the E2-S05 provider-posture scan) matches only
//	    `\bfacts\.<identifier>`, so each of A/B/C would let a controlling provider's
//	    failure: open posture slip. Rejecting all three makes the runtime scan SOUND
//	    BY CONSTRUCTION: after lint passes, every facts reference is a Select AND
//	    every dot provider is captured by policy.factRefRe.
//
//	facts-reference-shape — a dot reference spelling the `.value` accessor
//	    (facts.<p>.<n>.value). D-048 decides Option A (auto-unwrap): an authored
//	    facts.<provider>.<name> already addresses the typed fact VALUE directly;
//	    `.value` was the rejected Option-B spelling. `state`/`expiresAt`/
//	    `observedAt`/`reason`/`sensitive` are reserved envelope-escape accessors at
//	    the third segment and are PERMITTED (they read envelope metadata, e.g. the
//	    D-016 fixture's facts.owner.team.state); only `.value` is rejected, and
//	    value-tree indexing/navigation PAST facts.<provider>.<name> (e.g.
//	    facts.author.groups[0]) is permitted.
//
// Purity: cel-go's PARSER is deterministic and pure; internal/lint importing
// cel-go is purity-safe (TestCorePurity forbids clock/rand/env/net SELECTORS, not
// the cel-go import — internal/core/aggregate already imports it). This file uses
// no time.Now/rand/os.Getenv/net. The undeclared-scope compile check is E3-S04.

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/operators"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// Diagnostic codes owned by this check (defined here so lint.go's only S03 edit
// is the single checkFactsReferences call-site).
const (
	// CodeFactsReferenceSyntax: a `facts` reference not in dot syntax (bracket
	// index, bare whole-map, or a non-contiguous `facts . <p>` the whitespace-
	// sensitive runtime scan misses) — evades the E2-S05 provider-posture scan.
	CodeFactsReferenceSyntax = "facts-reference-syntax"
	// CodeFactsReferenceShape: a dot reference spelling the `.value` accessor,
	// which D-048's auto-unwrap convention forbids.
	CodeFactsReferenceShape = "facts-reference-shape"
)

// reservedValueAccessor is the single third-segment accessor D-048 forbids: under
// auto-unwrap the value is facts.<p>.<n> itself, so `.value` is never authored.
const reservedValueAccessor = "value"

// runtimeFactRefRe MUST stay byte-identical to internal/core/policy.factRefRe: it
// models EXACTLY what the E2-S05 provider-posture scan captures at runtime, so the
// whitespace soundness cross-check (rule C) proves lint accepts no dot reference
// that scan would miss. If policy.factRefRe changes, change this in lockstep.
var runtimeFactRefRe = regexp.MustCompile(`\bfacts\.([A-Za-z_][A-Za-z0-9_]*)`)

// checkFactsReferences walks every prove.when leaf in every loaded pack and emits
// facts-reference-syntax / facts-reference-shape diagnostics. Deterministic:
// packs/rules are visited in whatever order, but the Report re-sorts to a total
// canonical key on output, so iteration order never leaks (REQ-E3-S03-04).
func checkFactsReferences(m *model, rep *Report) {
	// A parse-only environment: no variable/function declarations are needed to
	// PARSE (only Check consults them), so a bare env parses any syntactically
	// valid CEL. cel.NewEnv() with no options is effectively infallible.
	env, envErr := cel.NewEnv()
	for name, p := range m.packs {
		for i := range p.rules {
			r := p.rules[i]
			if r.Prove == nil {
				continue
			}
			checkRuleFactsRefs(env, envErr, r.Prove.When, Location{File: name, Name: r.Name}, rep)
		}
	}
}

// checkRuleFactsRefs scans every leaf of one rule's assert tree. The whitespace
// soundness cross-check (rule C) is computed over the WHOLE rule, matching the
// runtime scan's per-rule union scope (policy.providersReferenced walks the whole
// tree): a provider is only "missed" if NO leaf presents it in the contiguous
// dot form the runtime regex captures.
func checkRuleFactsRefs(env *cel.Env, envErr error, tree policy.AssertTree, loc Location, rep *Report) {
	cels := leafCELs(tree)

	// regexProviders: exactly what the runtime E2-S05 scan captures over this rule.
	regexProviders := map[string]bool{}
	for _, c := range cels {
		for _, mm := range runtimeFactRefRe.FindAllStringSubmatch(c, -1) {
			regexProviders[mm[1]] = true
		}
	}

	// astDotProviders: providers the AST recognizes via a dot Select on facts.
	astDotProviders := map[string]bool{}
	for _, c := range cels {
		if env == nil || envErr != nil {
			// Fail-safe: without a parser we cannot prove the leaf facts-free.
			rep.addError(CodeParseError, loc, fmt.Sprintf("cannot build a CEL parse environment to lint facts references in when/cel leaf %q: %v", c, envErr))
			continue
		}
		ast, iss := env.Parse(c)
		if iss != nil && iss.Err() != nil {
			// A leaf that does not parse must NOT be assumed facts-free (a raw/
			// bytes string form cel-go rejects could otherwise hide a reference).
			rep.addError(CodeParseError, loc, fmt.Sprintf("when/cel leaf %q does not parse as CEL: %v", c, iss.Err()))
			continue
		}
		scanLeafAST(ast, c, loc, astDotProviders, rep)
	}

	// Rule C — whitespace soundness net: a dot provider the whitespace-sensitive
	// runtime scan would miss (e.g. `facts . owner`) is rejected. Sorted for a
	// stable emission order (the Report re-sorts too, but keep the source stable).
	var missed []string
	for prov := range astDotProviders {
		if !regexProviders[prov] {
			missed = append(missed, prov)
		}
	}
	sort.Strings(missed)
	for _, prov := range missed {
		rep.addError(CodeFactsReferenceSyntax, loc, fmt.Sprintf(
			"facts reference to provider %q is not a contiguous `facts.%s` dot reference (interior whitespace or other non-dot spacing); the E2-S05 provider-posture scan (policy.factRefRe, a whitespace-sensitive `\\bfacts\\.` match) would miss it, so it is rejected (D-048)",
			prov, prov))
	}
}

// scanLeafAST walks one parsed leaf's AST, classifying every `facts` identifier
// node. String literals (including raw / bytes / triple-quoted) are constant
// nodes, never facts Idents, so a `facts` fragment inside a literal is invisible
// here by construction — no lexer to desync.
func scanLeafAST(ast *cel.Ast, celText string, loc Location, astDotProviders map[string]bool, rep *Report) {
	root := celast.NavigateAST(ast.NativeRep())
	var walk func(n celast.NavigableExpr)
	walk = func(n celast.NavigableExpr) {
		if n.Kind() == celast.IdentKind && n.AsIdent() == "facts" {
			classifyFactsIdent(n, celText, loc, astDotProviders, rep)
		}
		for _, c := range n.Children() {
			walk(c)
		}
	}
	walk(root)
}

// classifyFactsIdent classifies one `facts` Ident node by its parent: a Select
// selecting from it is the sanctioned dot form (shape-checked); a bracket index on
// it or any other/absent parent is a syntax error.
func classifyFactsIdent(n celast.NavigableExpr, celText string, loc Location, astDotProviders map[string]bool, rep *Report) {
	par, ok := n.Parent()

	// Dot form: parent is a Select whose operand IS this facts ident.
	if ok && par.Kind() == celast.SelectKind && par.AsSelect().Operand().ID() == n.ID() {
		segs := selectChainFields(n) // [provider, name, accessor, ...]
		if len(segs) >= 1 {
			astDotProviders[segs[0]] = true
		}
		if len(segs) >= 3 && segs[2] == reservedValueAccessor {
			rep.addError(CodeFactsReferenceShape, loc, fmt.Sprintf(
				"facts reference `facts.%s.%s.value` in when/cel leaf %q spells the `.value` accessor; under the auto-unwrap convention (D-048) facts.<provider>.<name> already addresses the typed fact value — drop `.value` (the rejected Option-B spelling); `.state`/`.expiresAt`/`.observedAt` remain reserved envelope escapes",
				segs[0], segs[1], celText))
		}
		return
	}

	// Bracket index on facts, e.g. facts['owner'] — an INDEX call (`_[_]`) whose
	// first argument is the facts ident. policy.factRefRe (dot-only) misses it.
	if ok && par.Kind() == celast.CallKind && par.AsCall().FunctionName() == operators.Index &&
		len(par.AsCall().Args()) > 0 && par.AsCall().Args()[0].ID() == n.ID() {
		rep.addError(CodeFactsReferenceSyntax, loc, fmt.Sprintf(
			"a `facts` reference in when/cel leaf %q is a bracket index (facts['<provider>']); a fact must be addressed as facts.<provider>.<name> in dot syntax — bracket indexing evades the E2-S05 provider-posture scan (policy.factRefRe) and is rejected (D-048)",
			celText))
		return
	}

	// Bare facts — the whole map as an identifier (size(facts), a comparison, or a
	// standalone leaf): names no provider and is never the sanctioned form.
	rep.addError(CodeFactsReferenceSyntax, loc, fmt.Sprintf(
		"a bare `facts` reference in when/cel leaf %q names no provider; a fact must be addressed as facts.<provider>.<name> — a bare or non-dot facts reference evades the E2-S05 provider-posture scan (policy.factRefRe) and is rejected (D-048)",
		celText))
}

// selectChainFields walks upward from a facts Ident, collecting the dot-selected
// field names in order (provider, name, accessor, ...). It stops at the first
// ancestor that is not a Select selecting from the running chain — so an index or
// call past facts.<provider>.<name> (facts.author.groups[0]) simply ends the
// chain, leaving value-tree navigation permitted.
func selectChainFields(factsIdent celast.NavigableExpr) []string {
	var segs []string
	cur := factsIdent
	for {
		par, ok := cur.Parent()
		if !ok || par.Kind() != celast.SelectKind {
			break
		}
		sel := par.AsSelect()
		if sel.Operand().ID() != cur.ID() {
			break
		}
		segs = append(segs, sel.FieldName())
		cur = par
	}
	return segs
}

// leafCELs returns the CEL text of every leaf in an assert tree (all/any/not
// walked recursively) — the same traversal policy.providersReferenced uses.
func leafCELs(t policy.AssertTree) []string {
	var out []string
	var walk func(policy.AssertTree)
	walk = func(n policy.AssertTree) {
		if n.Leaf != nil {
			out = append(out, n.Leaf.CEL)
		}
		for _, c := range n.All {
			walk(c)
		}
		for _, c := range n.Any {
			walk(c)
		}
		if n.Not != nil {
			walk(*n.Not)
		}
	}
	walk(t)
	return out
}
