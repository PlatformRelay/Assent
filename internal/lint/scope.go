package lint

// scope.go is the E3-S04 undeclared-predicate-scope hard error over TWO authoring
// surfaces, closing the E2-S03 message-scope deferral (ADR-0013 residual #5):
//
//	undeclared-predicate-scope — a when/cel leaf referencing a top-level identifier
//	    outside the frozen eleven predicate-scope fields
//	    (docs/planning/predicate-scope.md). The pre-fix D-016 typo `input.new >=
//	    input.old` is the archetype: `input` is undeclared. E2-S02 catches it at
//	    runtime compile and fails safe; this catches it STATICALLY before any MR
//	    (ADR-0016 §2: unknown-field refs are load-time errors, never `<no value>`).
//	message-template-scope — a leaf or rule `message` whose `{{ }}` template expands
//	    a field outside the SAME frozen scope (the E2-S03 deferral). One shared
//	    activation model: a message may only interpolate the fields a predicate sees.
//
// # One frozen-scope source, no duplicated field list
//
// The scope authority is aggregate.CompileCheck — the E3-S04 exported wrapper over
// the SAME newEvalEnv() evalLeaf compiles against. Detection is by COMPILE (parse +
// check), not parse: only a compile against the declared env surfaces an
// out-of-scope identifier as an error. This file never re-declares the eleven
// fields (that would drift from the engine); cel-go and the frozen env are the sole
// authority. cel-go's standard macros bind comprehension variables, so a bound `c`
// in `changes.exists(c, ...)` is never mis-flagged — only genuinely free
// identifiers error, and CompileCheck names each in `undeclared reference to '<n>'`.
//
// # Fail-safe, never assumed in-scope (like S03)
//
// A when/cel leaf that does not compile for a NON-scope reason (a syntax error the
// AST parse also rejects, or a type error) is NOT proven in-scope, so it is surfaced
// as a distinct fail-safe diagnostic (CodeParseError with a scope-context message),
// never silently assumed clean. A message template is advisory display text, not the
// decision path, and the runtime expander leaves an unresolvable `{{ }}` literal —
// so a message body that fails to compile for a non-scope reason is left to that
// lenient runtime, and only a genuine out-of-scope FIELD is flagged.
//
// Purity: aggregate.CompileCheck is a pure cel-go compile; this file uses only
// regexp/sort/strings/fmt — no clock/rand/env/net. TestCorePurity scans it.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// Diagnostic codes owned by this check (defined here so lint.go's only S04 edit is
// the single checkPredicateScope call-site).
const (
	// CodeUndeclaredPredicateScope: a when/cel leaf references a top-level
	// identifier outside the frozen eleven predicate-scope fields (ADR-0016 §2).
	CodeUndeclaredPredicateScope = "undeclared-predicate-scope"
	// CodeMessageTemplateScope: a `{{ }}` message template expands a field outside
	// the same frozen scope (the E2-S03 deferral, ADR-0013 residual #5).
	CodeMessageTemplateScope = "message-template-scope"
)

// undeclaredRefRe extracts every identifier cel-go names in a compile error: each
// out-of-scope free reference renders as `undeclared reference to '<name>'`. A
// leaf may name one identifier several times (e.g. `input.new >= input.old`
// reports `input` twice); the caller dedupes.
var undeclaredRefRe = regexp.MustCompile(`undeclared reference to '([^']*)'`)

// tmplPlaceholderRe matches a `{{ ... }}` message placeholder (non-greedy, no
// nested braces). It intentionally MIRRORS aggregate/asserttree.go's unexported
// tmplPlaceholder: that file is off-limits to this story and the shape is a frozen
// one-liner, so it is re-declared here rather than imported. The captured body is a
// dotted field path into the same activation a CEL leaf evaluates over.
var tmplPlaceholderRe = regexp.MustCompile(`\{\{[^{}]*\}\}`)

// checkPredicateScope walks every when/cel leaf and every leaf/rule message in
// every loaded pack, emitting undeclared-predicate-scope / message-template-scope.
// Deterministic: packs/rules are visited in whatever map order, but the Report
// re-sorts to a total canonical key on output, so iteration order never leaks
// (REQ-E3-S04-04).
func checkPredicateScope(m *model, rep *Report) {
	for name, p := range m.packs {
		for i := range p.rules {
			r := p.rules[i]
			loc := Location{File: name, Name: ruleLabel(r, i)}
			if r.Prove != nil {
				for _, c := range leafCELs(r.Prove.When) {
					checkLeafScope(c, loc, rep)
				}
				for _, msg := range leafMessages(r.Prove.When) {
					checkMessageScope(msg, loc, rep)
				}
			}
			checkMessageScope(r.Message, loc, rep)
			checkMessageScope(r.Docs.Summary, loc, rep)
			for _, line := range r.Debug {
				checkMessageScope(line, loc, rep)
			}
		}
	}
}

// checkLeafScope compiles one when/cel leaf against the frozen scope. An undeclared
// reference → one undeclared-predicate-scope per distinct out-of-scope identifier
// (deduped, sorted). A compile failure with NO undeclared reference (a syntax or
// type error) is fail-safe: the leaf is not proven in-scope, so it is surfaced as a
// distinct CodeParseError rather than silently assumed clean.
func checkLeafScope(cel string, loc Location, rep *Report) {
	err := aggregate.CompileCheck(cel)
	if err == nil {
		return
	}
	names := undeclaredNames(err.Error())
	if len(names) == 0 {
		rep.addError(CodeParseError, loc, fmt.Sprintf(
			"when/cel leaf %q does not compile against the frozen predicate scope (fail-safe: not assumed in-scope): %v", cel, err))
		return
	}
	for _, n := range names {
		rep.addError(CodeUndeclaredPredicateScope, loc, fmt.Sprintf(
			"when/cel leaf %q references undeclared predicate-scope identifier %q; only the frozen eleven fields (old, new, entry, oldEntry, path, kind, file, env, changes, facts, mr — docs/planning/predicate-scope.md) are in scope (ADR-0016 §2)",
			cel, n))
	}
}

// checkMessageScope compiles each `{{ }}` template body in a message against the
// frozen scope. An out-of-scope field → message-template-scope naming it. A message
// is advisory display text (the runtime expander leaves an unresolvable `{{ }}`
// literal), so only a genuine out-of-scope FIELD is reported; a body that fails to
// compile for a non-scope reason is left to the lenient runtime.
func checkMessageScope(msg string, loc Location, rep *Report) {
	if !strings.Contains(msg, "{{") {
		return // fast path: no template
	}
	for _, token := range tmplPlaceholderRe.FindAllString(msg, -1) {
		body := strings.TrimSpace(token[2 : len(token)-2])
		if body == "" {
			continue
		}
		err := aggregate.CompileCheck(body)
		if err == nil {
			continue
		}
		for _, n := range undeclaredNames(err.Error()) {
			rep.addError(CodeMessageTemplateScope, loc, fmt.Sprintf(
				"message template %s references out-of-scope field %q; a `{{ }}` message may only interpolate the frozen predicate-scope fields (docs/planning/predicate-scope.md, ADR-0013 residual #5)",
				token, n))
		}
	}
}

// undeclaredNames returns the distinct, sorted identifier names cel-go reported as
// undeclared in a compile error string. Deduped (a leaf may name one identifier
// several times) and sorted for a stable source emission order (the Report re-sorts
// on output too, but keep the source deterministic).
func undeclaredNames(errText string) []string {
	seen := map[string]bool{}
	for _, m := range undeclaredRefRe.FindAllStringSubmatch(errText, -1) {
		seen[m[1]] = true
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// leafMessages returns the non-empty message of every leaf in an assert tree
// (all/any/not walked recursively) — the message half of the same traversal
// leafCELs (facts_ref.go) uses for CEL text.
func leafMessages(t policy.AssertTree) []string {
	var out []string
	var walk func(policy.AssertTree)
	walk = func(n policy.AssertTree) {
		if n.Leaf != nil && n.Leaf.Message != "" {
			out = append(out, n.Leaf.Message)
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
