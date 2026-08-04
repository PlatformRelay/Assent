package lint

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// scopeDiags runs only the predicate-scope check over a one-pack model built from
// the given rules — the exact surface checkPredicateScope walks. Built directly
// (not via YAML) so each case isolates its scope defect from schema/coverage noise.
func scopeDiags(rules ...policy.Rule) []Diagnostic {
	m := &model{packs: map[string]*loadedPack{"p": {rules: rules}}}
	rep := &Report{}
	checkPredicateScope(m, rep)
	return rep.Diagnostics()
}

// leafRule is a prove rule whose when is a single bare-CEL leaf.
func leafRule(name, cel string) policy.Rule {
	return policy.Rule{
		Name:  name,
		Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: cel}}},
	}
}

// codesAt returns the codes of diagnostics carrying the given code.
func diagsWithCode(diags []Diagnostic, code string) []Diagnostic {
	var out []Diagnostic
	for _, d := range diags {
		if d.Code == code {
			out = append(out, d)
		}
	}
	return out
}

// TestUndeclaredPredicateScopeInLeaf — REQ-E3-S04-01: a leaf referencing an
// out-of-scope top-level identifier (the pre-fix D-016 `input.new >= input.old`
// typo) yields exactly one undeclared-predicate-scope diagnostic naming `input`
// (deduped though cel-go reports `input` twice), the same defect E2-S02 catches at
// compile, now caught statically before any MR (ADR-0016 §2).
func TestUndeclaredPredicateScopeInLeaf(t *testing.T) {
	diags := scopeDiags(leafRule("r", "input.new >= input.old"))
	scoped := diagsWithCode(diags, CodeUndeclaredPredicateScope)
	if len(scoped) != 1 {
		t.Fatalf("want exactly one undeclared-predicate-scope (deduped), got %d: %v", len(scoped), codesOf(diags))
	}
	if !strings.Contains(scoped[0].Message, `"input"`) {
		t.Errorf("diagnostic must name the offending identifier %q, got %q", "input", scoped[0].Message)
	}
	if !strings.Contains(scoped[0].Message, "input.new >= input.old") {
		t.Errorf("diagnostic must quote the offending leaf, got %q", scoped[0].Message)
	}
}

// TestFrozenScopeExactlyElevenFields — REQ-E3-S04-02: each of the eleven frozen
// predicate-scope fields (docs/planning/predicate-scope.md) compiles clean, and a
// twelfth invented field does NOT — proving the scope is exactly the frozen closed
// set, reusing the aggregate env as the single authority (no duplicated list).
func TestFrozenScopeExactlyElevenFields(t *testing.T) {
	frozen := []string{"old", "new", "entry", "oldEntry", "path", "kind", "file", "env", "changes", "facts", "mr"}
	if len(frozen) != 11 {
		t.Fatalf("the frozen scope table has exactly 11 fields; test lists %d", len(frozen))
	}
	for _, f := range frozen {
		diags := scopeDiags(leafRule("r", f))
		if scoped := diagsWithCode(diags, CodeUndeclaredPredicateScope); len(scoped) != 0 {
			t.Errorf("frozen field %q must compile clean, got %v", f, scoped)
		}
	}
	// A twelfth invented field is undeclared -> error naming it.
	diags := scopeDiags(leafRule("r", "mystery == 1"))
	scoped := diagsWithCode(diags, CodeUndeclaredPredicateScope)
	if len(scoped) != 1 {
		t.Fatalf("an invented twelfth field must error, got %d diagnostics", len(scoped))
	}
	if !strings.Contains(scoped[0].Message, `"mystery"`) {
		t.Errorf("diagnostic must name the invented field, got %q", scoped[0].Message)
	}
}

// TestExportedCompileHelperInAggregate — REQ-E3-S04-04: the compile-only helper is
// EXPORTED from internal/core/aggregate (this test imports it), not from
// internal/core/policy (which stays cel-go-free). It reuses newEvalEnv, so an
// in-scope identifier compiles clean and an out-of-scope one errors.
func TestExportedCompileHelperInAggregate(t *testing.T) {
	if err := aggregate.CompileCheck("new >= old"); err != nil {
		t.Errorf("an in-scope leaf must compile clean via aggregate.CompileCheck, got %v", err)
	}
	if err := aggregate.CompileCheck("input.new >= input.old"); err == nil {
		t.Error("an out-of-scope leaf must return a compile error from aggregate.CompileCheck")
	} else if !strings.Contains(err.Error(), "undeclared reference") {
		t.Errorf("the compile error must surface the undeclared reference, got %v", err)
	}
}

// TestComprehensionVariableNotFlagged — cel-go's standard macros bind comprehension
// variables (exists/all/map over the frozen `changes` list), so a bound variable is
// NOT a free identifier and must not be mis-flagged; only the genuinely free
// out-of-scope identifier is reported.
func TestComprehensionVariableNotFlagged(t *testing.T) {
	// c is bound by exists -> not undeclared; `old` is in scope -> clean.
	if scoped := diagsWithCode(scopeDiags(leafRule("r", "changes.exists(c, c.old == old)")), CodeUndeclaredPredicateScope); len(scoped) != 0 {
		t.Errorf("a comprehension-bound variable must not be flagged, got %v", scoped)
	}
	// c is bound; `input` is the only genuinely free out-of-scope identifier.
	scoped := diagsWithCode(scopeDiags(leafRule("r", "changes.exists(c, c.foo == input)")), CodeUndeclaredPredicateScope)
	if len(scoped) != 1 {
		t.Fatalf("want exactly one diagnostic naming `input`, got %d: %v", len(scoped), scoped)
	}
	if strings.Contains(scoped[0].Message, `"c"`) || !strings.Contains(scoped[0].Message, `"input"`) {
		t.Errorf("must flag only the free `input`, never the bound `c`, got %q", scoped[0].Message)
	}
}

// TestMessageTemplateScope — REQ-E3-S04-03: a `{{ }}` message template referencing
// an out-of-scope field yields message-template-scope; in-scope templates are clean.
// Covers BOTH a leaf message and a rule message (the E2-S03 deferral, one shared
// activation model, ADR-0013 residual #5).
func TestMessageTemplateScope(t *testing.T) {
	// Leaf message with an out-of-scope `quota` -> message-template-scope.
	bad := policy.Rule{
		Name: "over-quota",
		Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{
			Leaf: &policy.Leaf{CEL: "new >= old", Message: "over {{ quota.max }}"},
		}},
	}
	scoped := diagsWithCode(scopeDiags(bad), CodeMessageTemplateScope)
	if len(scoped) != 1 {
		t.Fatalf("an out-of-scope `{{ quota.max }}` must yield message-template-scope, got %d", len(scoped))
	}
	if !strings.Contains(scoped[0].Message, `"quota"`) {
		t.Errorf("message-template-scope must name the out-of-scope identifier, got %q", scoped[0].Message)
	}

	// In-scope leaf templates ({{ old }}, {{ new }}, {{ facts.quota.max_partitions }}) are clean.
	clean := policy.Rule{
		Name: "clean",
		Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{
			Leaf: &policy.Leaf{CEL: "new >= old", Message: "was {{ old }} now {{ new }}, limit {{ facts.quota.max_partitions }}"},
		}},
	}
	if scoped := diagsWithCode(scopeDiags(clean), CodeMessageTemplateScope); len(scoped) != 0 {
		t.Errorf("in-scope `{{ }}` templates must be clean, got %v", scoped)
	}

	// A rule-level message is also scope-checked.
	ruleMsg := policy.Rule{Name: "eff", Effect: policy.EffectBlock, Message: "exceeds {{ object.size }}"}
	scoped = diagsWithCode(scopeDiags(ruleMsg), CodeMessageTemplateScope)
	if len(scoped) != 1 {
		t.Fatalf("a rule-level out-of-scope template must yield message-template-scope, got %d", len(scoped))
	}
	if !strings.Contains(scoped[0].Message, `"object"`) {
		t.Errorf("rule-message diagnostic must name the out-of-scope identifier, got %q", scoped[0].Message)
	}
}

// TestScopeDoubleRunStable — REQ-E3-S04-04: the scope check is pure (compile-only,
// no clock/env/net/random) and its canonically-sorted output is byte-identical
// across runs, even over a model whose defects span leaves and message templates.
func TestScopeDoubleRunStable(t *testing.T) {
	rules := []policy.Rule{
		leafRule("a", "input.new >= input.old"),
		{Name: "b", Effect: policy.EffectBlock, Message: "exceeds {{ object.size }} and {{ mystery }}"},
		{Name: "c", Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{
			Leaf: &policy.Leaf{CEL: "new >= old", Message: "over {{ quota.max }}"},
		}}},
	}
	first := renderScope(rules)
	second := renderScope(rules)
	if first != second {
		t.Fatalf("double run not byte-identical:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if first == "" {
		t.Fatal("expected diagnostics for a model with scope defects")
	}
}

func renderScope(rules []policy.Rule) string {
	m := &model{packs: map[string]*loadedPack{"p": {rules: rules}}}
	rep := &Report{}
	checkPredicateScope(m, rep)
	return rep.Render()
}
