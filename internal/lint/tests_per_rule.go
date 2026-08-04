package lint

// tests_per_rule.go is the E3-S06 tests-per-rule hard error: every loaded rule
// must have ≥1 test case on disk exercising it, else `assent lint` fails
// (ADR-0010 "assent lint fails packs without tests"; ADR-0014 adopter test
// format). This is a STATIC PRESENCE check, NOT the `assent test` runner (E6): it
// never executes a case, loads a facts.yaml, or diffs decisions — it asks only
// "does ≥1 case directory / inline case exist and reference this rule?".
//
// # Rule → test-case mapping convention (name-based)
//
// The frozen adopter layout (ADR-0014) is `.assent/tests/<pack>/<case>/…`
// (base/head/facts/expect fixtures), or an inline `.assent/tests/<pack>/cases.yaml`
// carrying many small cases. `<pack>` is the SAME segment the model keys a pack by
// — the directory following `packs/` in a MergePolicy's path (packName(), e.g.
// `.assent/packs/topics/rules/x.yaml` → "topics", and tests live under
// `.assent/tests/topics/…`). A rule is considered TESTED iff, within its pack,
// some case's NAME matches one of the rule's identity tokens:
//
//	{ rule.Name, rule.Prove.Obligation }
//
// The observed corpus (examples/packs/{topic-registry,service-catalog,infra-vars})
// names every case directory after the OBLIGATION it exercises (never rule.Name) —
// e.g. `tests/topics/ownership/` for the `ownership`-proving rule; infra-vars even
// maps four rules onto two obligation-named dirs (ownership, bounded-change). So
// the obligation token is what clears the real packs; rule.Name is accepted too so
// a pack that names a case after the rule itself is equally valid. A rule with
// NEITHER token (an unnamed, obligation-less plain-effect rule) cannot be mapped to
// any case and is flagged FAIL-SAFE — an unmappable rule is never silently passed.
//
// This is DELIBERATELY presence-by-name, not by-reference: a positive/satisfied
// case carries `findings: []` (it references no rule in its expect.yaml), so the
// case NAME is the only durable, execution-free link between a rule and its
// fixture. Reading expect.yaml's findings would (a) miss every positive case and
// (b) drift toward running the case — out of scope for S06 (that is E6).
//
// OQ (documented default, not an operator gate): an inline `cases.yaml` case's
// `name` is a case label, and the ADR shorthand example names cases after the
// scenario ("partition-increase-ok"), not the rule. With no rule reference in the
// shorthand and no inline case in the real corpus, S06 adopts the SAME name-based
// convention for inline cases (case `name` ∈ the rule's tokens). An adopter using
// scenario-named inline cases should name at least one case per rule after the rule
// or its obligation, or use the directory form. If a real inline corpus later needs
// richer attribution, raise it as an OQ then.
//
// Purity: pure — imports only policy (types) + yaml (inline-case name decode); no
// clock/rand/env/net. Determinism is by-set membership + the Report's canonical
// sort, so neither map iteration nor source order leaks into output.

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// CodeTestsPerRule is the diagnostic code for a loaded rule with zero test cases
// exercising it on disk (ADR-0010/0014, lint-hard-errors.md).
const CodeTestsPerRule = "tests-per-rule"

// inlineCasesFile is the reserved basename of the inline (multi-case) fixture,
// living directly under `.assent/tests/<pack>/` (ADR-0014 shorthand form).
const inlineCasesFile = "cases.yaml"

// checkTestsPerRule emits tests-per-rule for every loaded rule with no matching
// test case on disk. It reads `sources` for the `.assent/tests/**` layout (which
// the tolerant model does not carry) and the model for the loaded rules. Wired
// into Lint() (lint.go). Presence-only: no case is executed.
func checkTestsPerRule(sources []Source, m *model, rep *Report) {
	cases := testCaseNames(sources) // pack → set of case names present on disk
	for _, d := range m.docs {
		if d.isPack {
			continue
		}
		pack := packName(d.path)
		present := cases[pack]
		for i := range d.rules {
			r := d.rules[i]
			if ruleHasCase(present, r) {
				continue
			}
			rep.addError(CodeTestsPerRule, Location{File: d.path, Name: ruleLabel(r, i)}, fmt.Sprintf(
				"rule %q in pack %q has no test case exercising it (tokens tried: %v); add a directory-form case .assent/tests/%s/<%s>/ (base/head/facts/expect) or an inline .assent/tests/%s/cases.yaml case named for the rule — a rule with no test is rejected (ADR-0010/0014, lint-hard-errors.md)",
				ruleLabel(r, i), pack, ruleTestTokens(r), pack, preferredCaseName(r), pack))
		}
	}
}

// testCaseNames indexes, per pack, the set of test-case names present under
// `.assent/tests/<pack>/…`. A directory-form case contributes its top-level case
// segment (`tests/<pack>/<case>/…` → "<case>", so a nested `negative/` variant is
// still attributed to its parent case). An inline `tests/<pack>/cases.yaml`
// contributes each of its `cases[].name` values. Purely path-driven (plus the
// inline case-name decode) — no case is opened or executed.
func testCaseNames(sources []Source) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	add := func(pack, name string) {
		if name == "" {
			return
		}
		if out[pack] == nil {
			out[pack] = map[string]bool{}
		}
		out[pack][name] = true
	}
	for _, s := range sources {
		segs := strings.Split(s.Path, "/")
		i := testsSegmentIndex(segs)
		if i < 0 || i+2 >= len(segs) {
			continue // not under `.assent/tests/<pack>/<something>`
		}
		pack := segs[i+1]
		candidate := segs[i+2]
		// Inline form: `.assent/tests/<pack>/cases.yaml` (the file sits directly
		// under the pack's tests dir, no case subdirectory).
		if candidate == inlineCasesFile && i+3 == len(segs) {
			for _, name := range inlineCaseNames(s.Bytes) {
				add(pack, name)
			}
			continue
		}
		// Directory form: `candidate` is the case directory name.
		add(pack, candidate)
	}
	return out
}

// testsSegmentIndex returns the index of the `.assent/tests` `tests` segment (the
// segment named "tests" immediately following ".assent"), or -1 if the path is not
// under a `.assent/tests` tree. Anchoring on the ".assent" predecessor avoids
// matching a pack or file coincidentally named "tests" elsewhere in the path.
func testsSegmentIndex(segs []string) int {
	for i := 1; i < len(segs); i++ {
		if segs[i] == "tests" && segs[i-1] == ".assent" {
			return i
		}
	}
	return -1
}

// ruleHasCase reports whether any of the rule's identity tokens is present in the
// pack's on-disk case-name set. A rule with no tokens (empty set) can never match
// → flagged fail-safe by the caller.
func ruleHasCase(present map[string]bool, r policy.Rule) bool {
	if present == nil {
		return false
	}
	for _, tok := range ruleTestTokens(r) {
		if present[tok] {
			return true
		}
	}
	return false
}

// ruleTestTokens is a rule's set of case-name identity tokens, in a stable order
// (for the deterministic diagnostic message): its Name, then its prove obligation.
// An unnamed, obligation-less rule yields an empty set (→ fail-safe flag).
func ruleTestTokens(r policy.Rule) []string {
	var toks []string
	if r.Name != "" {
		toks = append(toks, r.Name)
	}
	if r.Prove != nil && r.Prove.Obligation != "" {
		toks = append(toks, r.Prove.Obligation)
	}
	return toks
}

// preferredCaseName is the token an author is most likely to name a case after —
// the obligation when present (matching the observed corpus), else the rule name,
// else a placeholder. Presentation only (the actionable message's <case> hint).
func preferredCaseName(r policy.Rule) string {
	if r.Prove != nil && r.Prove.Obligation != "" {
		return r.Prove.Obligation
	}
	if r.Name != "" {
		return r.Name
	}
	return "case-name"
}

// inlineCaseNames decodes the case names from an inline `cases.yaml` document,
// sorted for determinism. A doc that does not parse (or has no cases) yields none;
// it is never executed — only the case `name`s are read.
func inlineCaseNames(raw []byte) []string {
	var doc struct {
		Cases []struct {
			Name string `yaml:"name"`
		} `yaml:"cases"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	names := make([]string, 0, len(doc.Cases))
	for _, c := range doc.Cases {
		if c.Name != "" {
			names = append(names, c.Name)
		}
	}
	sort.Strings(names)
	return names
}
