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

	// The non-dot family that maps to a NON-Select facts Ident — bracket index
	// (single/double quote) and a bare whole-map reference — is a lone syntax error
	// (the facts Ident's parent is an Index/size Call, so the Option-B shape rule,
	// which only fires on dot Selects, never co-fires).
	for _, cel := range []string{
		`facts['owner'].team == 'x'`,
		`facts["owner"].team == 'x'`,
		`size(facts) > 0`, // bare whole-map, names no provider
	} {
		diags := factsDiags(cel)
		if len(diags) != 1 || diags[0].Code != CodeFactsReferenceSyntax {
			t.Errorf("cel %q: want exactly one facts-reference-syntax, got %v", cel, codesOf(diags))
		}
	}

	// Interior/after-dot whitespace normalizes to a dot Select with <3 segments, so
	// under Option B it is BOTH a whitespace-scan evasion (syntax) AND a bare-shape
	// violation — it co-emits both codes. The syntax rejection (the security-
	// critical piece) MUST still be present; the co-emitted shape is fail-safe.
	for _, cel := range []string{
		`facts . owner.team == 'x'`, // interior whitespace before the dot
		`facts.  owner == 'x'`,      // whitespace after the dot
	} {
		diags := factsDiags(cel)
		if !hasCode(diags, CodeFactsReferenceSyntax, "") {
			t.Errorf("cel %q: whitespace evasion must still yield a facts-reference-syntax, got %v", cel, codesOf(diags))
		}
	}
}

// TestFactsReferenceShapeConvention — REQ-E3-S03-02: under the D-051 Option B
// convention (SUPERSEDES D-049 Option A) the fact VALUE is at facts.<p>.<n>.value
// and value-tree navigation goes THROUGH `.value`. A third segment that is `value`
// (with optional deeper .field/[i] navigation) or a reserved envelope escape lints
// clean; ANY other third segment, or a bare facts.<p>.<n> with no accessor (the
// envelope map — an author almost always means `.value`), is a facts-reference-
// shape error. Pins the segment discriminator: facts.owner.team.value (three
// segments → the value) is PERMITTED, while facts.quota.value (two segments, a fact
// NAMED "value" addressed bare) is now a bare-shape error (needs .value.value).
func TestFactsReferenceShapeConvention(t *testing.T) {
	// Conformant under Option B: the value at `.value`, plus value-tree navigation
	// THROUGH `.value` (deeper .field selection and [i] indexing).
	for _, clean := range []string{
		`entry.owner in facts.author.groups.value`,
		`new <= facts.quota.max_partitions.value`,
		`facts.owner.team.value == 'x'`,               // bare `.value`
		`facts.author.groups.value[0] == 'x'`,         // index THROUGH `.value`
		`facts.owner.team.value.members[0].id == 'x'`, // deep nav THROUGH `.value`
	} {
		if diags := factsDiags(clean); len(diags) != 0 {
			t.Errorf("conformant Option-B reference %q must lint clean, got %v", clean, codesOf(diags))
		}
	}

	// Violation — an unknown third segment: value navigation must go through
	// `.value` (facts.band.max_replicas.max → facts.band.max_replicas.value.max).
	for _, bad := range []string{
		`facts.band.max_replicas.max == 'x'`,    // unknown 3rd segment
		`facts.owner.team.members[0].id == 'x'`, // value-tree nav NOT through `.value`
	} {
		diags := factsDiags(bad)
		if len(diags) != 1 || diags[0].Code != CodeFactsReferenceShape {
			t.Fatalf("unknown-3rd-segment reference %q must yield exactly one facts-reference-shape, got %v", bad, codesOf(diags))
		}
		if !strings.Contains(diags[0].Message, ".value") {
			t.Errorf("shape diagnostic must point at the `.value` accessor, got %q", diags[0].Message)
		}
	}

	// Violation — a bare facts.<p>.<n> (the envelope map, no accessor), including a
	// fact literally NAMED `value` addressed bare and a bare envelope indexed.
	for _, bare := range []string{
		`facts.owner.team == 'x'`,            // bare envelope, 2 segments
		`entry.owner in facts.author.groups`, // bare envelope (was clean under Option A)
		`facts.author.groups[0] == 'x'`,      // bare envelope indexed (must be `.value[0]`)
		`facts.quota.value > 0`,              // fact NAMED value, addressed bare (needs `.value.value`)
	} {
		diags := factsDiags(bare)
		if len(diags) != 1 || diags[0].Code != CodeFactsReferenceShape {
			t.Fatalf("bare facts.<p>.<n> reference %q must yield exactly one facts-reference-shape, got %v", bare, codesOf(diags))
		}
		if !strings.Contains(diags[0].Message, ".value") {
			t.Errorf("shape diagnostic must point at the `.value` accessor, got %q", diags[0].Message)
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
				{Leaf: &policy.Leaf{CEL: `facts['owner'].team == 'x'`}},                                   // syntax (bracket)
				{Not: &policy.AssertTree{Leaf: &policy.Leaf{CEL: `facts.owner.team.maxReplicas == 'y'`}}}, // shape (unknown 3rd, Option B)
			}}}},
			{Name: "b", Prove: &policy.Prove{Obligation: "ownership", When: policy.AssertTree{Any: []policy.AssertTree{
				{Leaf: &policy.Leaf{CEL: `entry.owner in facts.author.groups.value`}},   // clean (Option B .value)
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

	// Standard non-dot forms still caught (the reviewer's explicit re-proof). The
	// bracket and bare-whole-map forms are a lone syntax error; the whitespace
	// evasion co-emits an Option-B bare-shape error too, but the security-critical
	// syntax rejection MUST still be present.
	for _, cel := range []string{
		`facts['owner'].team == 'x'`, // bracket
		`size(facts) > 0`,            // bare whole-map
	} {
		if d := factsDiags(cel); len(d) != 1 || d[0].Code != CodeFactsReferenceSyntax {
			t.Errorf("standard non-dot form %q must still yield facts-reference-syntax, got %v", cel, codesOf(d))
		}
	}
	if d := factsDiags(`facts . owner == 'x'`); !hasCode(d, CodeFactsReferenceSyntax, "") { // interior whitespace (runtime-scan evasion)
		t.Errorf("whitespace-evasion `facts . owner` must still yield facts-reference-syntax, got %v", codesOf(d))
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
