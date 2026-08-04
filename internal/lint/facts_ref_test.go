package lint

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// factsModel builds a minimal tolerant-ingestion model carrying one pack "p"
// whose rules each prove `ownership` with the given CEL leaf as prove.when — the
// exact surface checkFactsReferences walks. Built directly (not via YAML) so each
// case isolates its facts-reference defect from schema/coverage noise.
func factsModel(cels ...string) *model {
	var rules []policy.Rule
	for i, c := range cels {
		rules = append(rules, policy.Rule{
			Name:  fmt.Sprintf("rule-%d", i),
			Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: c}}},
		})
	}
	return &model{packs: map[string]*loadedPack{"p": {rules: rules}}}
}

// factsDiags runs only the facts-reference check over leaves with the given CELs.
func factsDiags(cels ...string) []Diagnostic {
	rep := &Report{}
	checkFactsReferences(factsModel(cels...), rep)
	return rep.Diagnostics()
}

// codesOf returns the diagnostic codes present, for concise assertions.
func codesOf(diags []Diagnostic) []string {
	out := make([]string, len(diags))
	for i, d := range diags {
		out[i] = d.Code
	}
	return out
}

// TestNonDotFactsReferenceRejected — REQ-E3-S03-01: a facts reference that is NOT
// dot-syntax (bracket index, interior whitespace, bare whole-map) is a
// facts-reference-syntax error. This closes the E2-S05 evasion: a controlling
// provider reached only via facts['owner'] slips policy.factRefRe's `\bfacts\.`
// scan today; lint rejects it so the runtime posture scan is sound by
// construction. Driven end-to-end through Lint() to prove the check is wired AND
// that the bracket text survives the YAML→AssertTree decode.
func TestNonDotFactsReferenceRejected(t *testing.T) {
	// The headline adversarial case, through the real ingest path.
	bracketRule := Source{
		Path: ".assent/packs/p/rules/owns.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: owns
spec:
  rules:
    - name: owns
      phase: enforce
      match:
        files:
          paths: ["topics/**/*.yaml"]
      prove:
        obligation: ownership
        when: "facts['owner'].team == 'resolved'"
      onFailure:
        effect: require-review
        code: ownership.unproven
`),
	}
	rep := Lint([]Source{
		bindingSrc("ownership"), // covered, so no obligation-coverage noise
		bracketRule,
	})
	var found bool
	for _, d := range rep.Diagnostics() {
		if d.Code == CodeFactsReferenceSyntax {
			found = true
			if !strings.Contains(d.Message, "facts['owner']") {
				t.Errorf("syntax diagnostic must quote the offending reference, got %q", d.Message)
			}
		}
	}
	if !found {
		t.Fatal("facts['owner'] must yield a facts-reference-syntax error (E2-S05 evasion left open)")
	}
	if !rep.HasErrors() {
		t.Error("a non-dot facts reference must signal a non-zero exit")
	}

	// The full non-dot family, unit-isolated: bracket (single/double quote),
	// interior whitespace, and a bare whole-map reference — each one error.
	for _, cel := range []string{
		`facts['owner'].team == 'x'`,
		`facts["owner"].team == 'x'`,
		`facts . owner.team == 'x'`, // interior whitespace before the dot
		`facts.  owner == 'x'`,      // whitespace after the dot
		`size(facts) > 0`,           // bare whole-map, names no provider
	} {
		diags := factsDiags(cel)
		if len(diags) != 1 || diags[0].Code != CodeFactsReferenceSyntax {
			t.Errorf("cel %q: want exactly one facts-reference-syntax, got %v", cel, codesOf(diags))
		}
	}
}

// TestFactsReferenceShapeConvention — REQ-E3-S03-02: under the D-048 auto-unwrap
// convention (Option A) a dot reference that spells the .value accessor violates
// the shape and is a facts-reference-shape error; a conformant bare-value
// reference lints clean. Also pins the segment discriminator: facts.quota.value
// (a fact NAMED "value", two segments → the value itself) must NOT be confused
// with facts.owner.team.value (an accessor at the third segment → flagged).
func TestFactsReferenceShapeConvention(t *testing.T) {
	// Conformant: bare value access, no .value — the whole authored corpus shape.
	for _, clean := range []string{
		`entry.owner in facts.author.groups`,
		`new <= facts.quota.max_partitions`,
		`facts.quota.value > 0`, // fact literally NAMED value: two segments, clean
		// Value-tree indexing PAST facts.<provider>.<name> is permitted — the
		// dot-form requirement is enforced only at the provider segment (that is
		// all policy.factRefRe needs to capture the provider); indexing into the
		// unwrapped value is legitimate CEL, not a posture evasion.
		`facts.author.groups[0] == 'x'`,
		`facts.owner.team.members[0].id == 'x'`,
	} {
		if diags := factsDiags(clean); len(diags) != 0 {
			t.Errorf("conformant reference %q must lint clean, got %v", clean, codesOf(diags))
		}
	}

	// Violation: the Option-B .value accessor at the third segment.
	for _, bad := range []string{
		`facts.owner.team.value == 'x'`,
		`new <= facts.quota.max_partitions.value`,
	} {
		diags := factsDiags(bad)
		if len(diags) != 1 || diags[0].Code != CodeFactsReferenceShape {
			t.Fatalf("reference %q must yield exactly one facts-reference-shape, got %v", bad, codesOf(diags))
		}
		if !strings.Contains(diags[0].Message, ".value") {
			t.Errorf("shape diagnostic must name the .value accessor, got %q", diags[0].Message)
		}
	}
}

// TestEnvelopeEscapeAccessorsPermitted — REQ-E3-S03-03: the reserved envelope
// escapes (state/expiresAt/observedAt/reason/sensitive) at the third segment are
// PERMITTED — the D-016 fixture's facts.owner.team.state must not be flagged.
func TestEnvelopeEscapeAccessorsPermitted(t *testing.T) {
	for _, escape := range []string{
		`facts.owner.team.state == 'resolved'`,
		`facts.owner.team.expiresAt > mr.author`,
		`facts.owner.team.observedAt != ''`,
		`facts.owner.team.reason == ''`,
		`facts.owner.team.sensitive == true`,
	} {
		if diags := factsDiags(escape); len(diags) != 0 {
			t.Errorf("reserved envelope escape %q must be permitted, got %v", escape, codesOf(diags))
		}
	}
}

// TestFactsRefDoubleRunStable — REQ-E3-S03-04: the check is pure (token/segment
// scan, no clock/env/net/random) and byte-identical across runs, including
// leaves nested under all/any/not and facts fragments hidden in string literals
// (which must NOT false-positive).
func TestFactsRefDoubleRunStable(t *testing.T) {
	// Nested tree with a mix of clean, syntax-bad, shape-bad, and literal-embedded
	// leaves across two rules — a superset that would expose any ordering leak.
	m := &model{packs: map[string]*loadedPack{
		"p": {rules: []policy.Rule{
			{Name: "a", Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{All: []policy.AssertTree{
				{Leaf: &policy.Leaf{CEL: `facts['owner'].team == 'x'`}},
				{Not: &policy.AssertTree{Leaf: &policy.Leaf{CEL: `facts.owner.team.value == 'y'`}}},
			}}}},
			{Name: "b", Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Any: []policy.AssertTree{
				{Leaf: &policy.Leaf{CEL: `entry.owner in facts.author.groups`}},         // clean
				{Leaf: &policy.Leaf{CEL: `path.contains('facts') && facts . x == 'z'`}}, // literal facts is NOT a ref; facts . x is
			}}}},
		}},
		"q": {rules: []policy.Rule{
			{Name: "c", Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: `facts["role"].value != ''`}}}},
		}},
	}}
	render := func() string {
		rep := &Report{}
		checkFactsReferences(m, rep)
		return rep.Render()
	}
	first, second := render(), render()
	if first != second {
		t.Fatalf("facts-ref check not byte-stable across runs:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

// TestRawStringBracketAccessRejected — F1 regression (a PROVEN fail-open the old
// hand-rolled string-masker let through): a bracket access on a controlling
// provider hidden behind a RAW / BYTES / TRIPLE-QUOTED string, whose backslash
// semantics diverge from an escaped-quote lexer. The AST-based check parses with
// cel-go itself, so the raw string is a constant node and facts['owner'] is a real
// bracket reference that is caught. Proves the standard non-dot forms still fire.
func TestRawStringBracketAccessRejected(t *testing.T) {
	// Raw string `r'\'` closes at its SECOND quote (backslash literal); the old
	// masker desynced here and masked away facts[ → zero diagnostics. Must now fire.
	rawPayload := `r'\' == 'x' && facts['owner'].team == 'y'`
	diags := factsDiags(rawPayload)
	if len(diags) != 1 || diags[0].Code != CodeFactsReferenceSyntax {
		t.Fatalf("raw-string-hidden facts['owner'] must yield facts-reference-syntax, got %v", codesOf(diags))
	}

	// Triple-quoted raw string, same mechanism — the bracket ref is still real.
	triplePayload := `r'''\''' == 'x' && facts['owner'].team == 'y'`
	if d := factsDiags(triplePayload); len(d) != 1 || d[0].Code != CodeFactsReferenceSyntax {
		t.Fatalf("triple-quoted-raw-hidden facts['owner'] must yield facts-reference-syntax, got %v", codesOf(d))
	}

	// Bytes-raw prefix cel-go REJECTS as a parse error — fail-safe: the leaf must
	// NOT be silently assumed facts-free (it must surface an error diagnostic).
	bytesPayload := `rb'\' == b'x' && facts['owner'].team == 'y'`
	bd := factsDiags(bytesPayload)
	var haveErr bool
	for _, d := range bd {
		if d.Code == CodeParseError {
			haveErr = true
		}
	}
	if !haveErr {
		t.Fatalf("an unparseable (bytes-raw) leaf hiding facts['owner'] must surface a parse-error, not be silently skipped; got %v", codesOf(bd))
	}

	// Standard non-dot forms still caught (the reviewer's explicit re-proof).
	for _, cel := range []string{
		`facts['owner'].team == 'x'`, // bracket
		`facts . owner == 'x'`,       // interior whitespace (runtime-scan evasion)
		`size(facts) > 0`,            // bare whole-map
	} {
		if d := factsDiags(cel); len(d) != 1 || d[0].Code != CodeFactsReferenceSyntax {
			t.Errorf("standard non-dot form %q must still yield facts-reference-syntax, got %v", cel, codesOf(d))
		}
	}
}

// TestFactsInStringLiteralNotFlagged — false-positive guard: a `facts` fragment
// inside a CEL string literal is a constant node in the parsed AST, never a facts
// Ident, so it must never produce a diagnostic; a REAL facts reference in the same
// leaf still is. (AST detection makes this true by construction — no lexer.)
func TestFactsInStringLiteralNotFlagged(t *testing.T) {
	// Pure literal occurrences — zero diagnostics.
	for _, clean := range []string{
		`path.contains('facts')`,
		`'needs-facts-review' in mr.labels`,
		`file == "config/facts.yaml"`,
		`"facts['owner']" == mr.sourceBranch`, // the bracket form is INSIDE a literal
	} {
		if diags := factsDiags(clean); len(diags) != 0 {
			t.Errorf("literal-only leaf %q must lint clean (masking failed), got %v", clean, codesOf(diags))
		}
	}
	// A real reference beside a literal — exactly one diagnostic for the real one.
	diags := factsDiags(`path.contains('facts') && facts['owner'].team == 'x'`)
	if len(diags) != 1 || diags[0].Code != CodeFactsReferenceSyntax {
		t.Fatalf("want one syntax error for the real facts['owner'] beside a literal, got %v", codesOf(diags))
	}
}
