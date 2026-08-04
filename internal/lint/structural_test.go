package lint

import (
	"strings"
	"testing"
)

// hasCode reports whether any diagnostic carries the given code (optionally also
// naming `name` in its Location or Message). Asserting a SPECIFIC diagnostic is
// present is preferred over exact totals so an incidental co-fire from another
// check never makes a structural test brittle (advisor #4).
func hasCode(diags []Diagnostic, code, name string) bool {
	for _, d := range diags {
		if d.Code != code {
			continue
		}
		if name == "" || strings.Contains(d.Location.Name, name) || strings.Contains(d.Message, name) {
			return true
		}
	}
	return false
}

// countCode returns how many diagnostics carry the given code.
func countCode(diags []Diagnostic, code string) int {
	n := 0
	for _, d := range diags {
		if d.Code == code {
			n++
		}
	}
	return n
}

// policyBindingSrc is a schema-valid RulesetBinding routing the reserved
// `assent-policy` meta-class to pack `pack`, requiring the given obligations
// (empty by default so a non-proving rule does not also trip obligation-coverage).
func policyBindingSrc(pack, require string) Source {
	return Source{
		Path: ".assent/policy-binding.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: RulesetBinding
bindings:
  - class: assent-policy
    environment: prod
    packs: [` + pack + `]
    risk: { threshold: 0 }
    require: [` + require + `]
`),
	}
}

// provingRuleSrc is a MergePolicy under `pack` with one obligation-satisfying
// (prove) rule — the APPROVE-arming "vouch" disposition ValidateRouting rejects
// on a reserved class. Carries an explicit phase so no-implicit-enforce-phase
// does not co-fire.
func provingRuleSrc(pack, name, obligation string) Source {
	return Source{
		Path: ".assent/packs/" + pack + "/rules/" + name + ".yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: ` + name + `
spec:
  rules:
    - name: ` + name + `
      phase: enforce
      match:
        files:
          paths: [".assent/**/*.yaml"]
      prove:
        obligation: ` + obligation + `
        when: "entry.owner in facts.author.groups"
      onFailure:
        effect: require-review
        code: ` + obligation + `.unproven
`),
	}
}

// effectRuleSrc is a MergePolicy under `pack` with one non-obligation rule
// applying `effect`, with an explicit `phase` (so no-implicit-enforce-phase does
// NOT co-fire — advisor #4). Used to exercise reserved-class routing.
func effectRuleSrc(pack, name, effect string) Source {
	return Source{
		Path: ".assent/packs/" + pack + "/rules/" + name + ".yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: ` + name + `
spec:
  rules:
    - name: ` + name + `
      phase: enforce
      match:
        files:
          paths: [".assent/**/*.yaml"]
      effect: ` + effect + `
      message: "policy edit"
`),
	}
}

// TestReservedClassRouting — REQ-E3-S02-01: a binding routing the reserved
// `assent-policy` class to a pack whose rule resolves to a non-block/non-challenge
// outcome (an obligation-satisfying prove, or a `comment` effect) is a
// reserved-class error naming the rule; block and challenge lint clean.
func TestReservedClassRouting(t *testing.T) {
	// A prove/obligation-satisfying rule ARMS APPROVE (vouch) on the policy class —
	// the ADR-0015 §1 self-vouch ValidateRouting rejects.
	provRep := Lint([]Source{
		policyBindingSrc("pol", "policy-ok"), // require it so obligation-coverage stays clean
		provingRuleSrc("pol", "selfvouch", "policy-ok"),
	})
	if !hasCode(provRep.Diagnostics(), CodeReservedClass, "selfvouch") {
		t.Errorf("a prove/vouch rule on the assent-policy class must yield reserved-class naming the rule, got %v", codesOf(provRep.Diagnostics()))
	}

	// A `comment` effect is non-block/non-challenge — advisory, enforces nothing.
	// It must still error (a policy MR that only comments never blocks itself).
	commentRep := Lint([]Source{
		policyBindingSrc("pol", ""),
		effectRuleSrc("pol", "advise", "comment"),
	})
	if !hasCode(commentRep.Diagnostics(), CodeReservedClass, "advise") {
		t.Errorf("a comment-effect rule on the assent-policy class must yield reserved-class, got %v", codesOf(commentRep.Diagnostics()))
	}

	// block and challenge are the two permitted policy-class dispositions.
	for _, effect := range []string{"block", "challenge"} {
		rep := Lint([]Source{
			policyBindingSrc("pol", ""),
			effectRuleSrc("pol", "guard", effect),
		})
		if countCode(rep.Diagnostics(), CodeReservedClass) != 0 {
			t.Errorf("effect %q on the assent-policy class must lint clean, got %v", effect, codesOf(rep.Diagnostics()))
		}
	}

	// A NON-reserved class routed to a vouch/comment is fine — the check is scoped
	// to the reserved meta-class only.
	nonReserved := Lint([]Source{
		bindingSrc("ownership"), // class: kafka-topic
		ruleSrc("owns", "ownership"),
	})
	if countCode(nonReserved.Diagnostics(), CodeReservedClass) != 0 {
		t.Errorf("a non-reserved class must never yield reserved-class, got %v", codesOf(nonReserved.Diagnostics()))
	}
}

// TestNoImplicitEnforcePhase — REQ-E3-S02-02: a MergePolicy rule OR a Pack
// manifest missing `phase` yields no-implicit-enforce-phase naming the offender;
// explicit phase lints clean. The critical dedupe assertions: a doc whose SOLE
// defect is the missing phase yields exactly ONE diagnostic (the actionable
// no-implicit-enforce-phase), NOT also the schema-invalid strict-loader capture;
// a doc missing phase AND carrying an unrelated schema violation keeps BOTH.
func TestNoImplicitEnforcePhase(t *testing.T) {
	// A rule missing ONLY phase (otherwise schema-valid). Driven through Lint() so
	// the strict loader runs and WOULD emit schema-invalid absent the dedupe.
	missingPhaseRule := Source{
		Path: ".assent/packs/p/rules/owns.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: owns
spec:
  rules:
    - name: owns
      match:
        files:
          paths: ["topics/**/*.yaml"]
      prove:
        obligation: ownership
        when: "entry.owner in facts.author.groups"
      onFailure:
        effect: require-review
        code: ownership.unproven
`),
	}
	// A test case for the rule so tests-per-rule (E3-S06, wired into the shared
	// Lint) does not add a diagnostic — this test isolates the phase dedupe (exactly
	// one diagnostic for a sole missing-phase defect).
	diags := Lint([]Source{missingPhaseRule, caseDirSrc("p", "ownership")}).Diagnostics()
	if !hasCode(diags, CodeNoImplicitEnforcePhase, "owns") {
		t.Errorf("a rule missing phase must yield no-implicit-enforce-phase naming the rule, got %v", codesOf(diags))
	}
	// DEDUPE: the missing-phase defect must be counted ONCE — no schema-invalid for
	// the same doc when it is the sole cause.
	if countCode(diags, CodeSchemaInvalid) != 0 {
		t.Errorf("missing-phase must be deduped: no schema-invalid when phase is the sole cause, got %v", codesOf(diags))
	}
	if len(diags) != 1 {
		t.Errorf("a solely-missing-phase rule must yield exactly ONE diagnostic (no double-count), got %d: %v", len(diags), codesOf(diags))
	}

	// A Pack manifest missing spec.phase → no-implicit-enforce-phase naming the pack,
	// deduped from schema-invalid the same way.
	missingPackPhase := Source{
		Path: ".assent/packs/p/pack.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: Pack
metadata:
  name: p
spec:
  version: "1.0"
  description: test
`),
	}
	packDiags := Lint([]Source{missingPackPhase}).Diagnostics()
	if !hasCode(packDiags, CodeNoImplicitEnforcePhase, "p") {
		t.Errorf("a Pack manifest missing phase must yield no-implicit-enforce-phase naming the pack, got %v", codesOf(packDiags))
	}
	if countCode(packDiags, CodeSchemaInvalid) != 0 {
		t.Errorf("Pack missing-phase must be deduped from schema-invalid, got %v", codesOf(packDiags))
	}

	// SURGICAL dedupe (the retention proof): a rule missing phase AND carrying an
	// unrelated schema violation (bad onFailure.effect enum) keeps BOTH diagnostics
	// — the schema-invalid is retained so the co-located defect is not hidden.
	multiDefect := Source{
		Path: ".assent/packs/p/rules/owns.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: owns
spec:
  rules:
    - name: owns
      match:
        files:
          paths: ["topics/**/*.yaml"]
      prove:
        obligation: ownership
        when: "entry.owner in facts.author.groups"
      onFailure:
        effect: bogus-effect
        code: ownership.unproven
`),
	}
	md := Lint([]Source{multiDefect}).Diagnostics()
	if !hasCode(md, CodeNoImplicitEnforcePhase, "owns") {
		t.Errorf("multi-defect doc must still yield no-implicit-enforce-phase, got %v", codesOf(md))
	}
	if countCode(md, CodeSchemaInvalid) != 1 {
		t.Errorf("multi-defect doc must RETAIN schema-invalid (phase not the sole cause), got %v", codesOf(md))
	}

	// Explicit phase lints clean (no over-fire).
	clean := Lint([]Source{ruleSrc("owns", "ownership")}).Diagnostics()
	if countCode(clean, CodeNoImplicitEnforcePhase) != 0 {
		t.Errorf("an explicit-phase rule must not yield no-implicit-enforce-phase, got %v", codesOf(clean))
	}

	// The effect/onFailure rollout workaround is still flagged: a rule that omits
	// phase but leans on effect to approximate a rollout is pointed at the phase field.
	workaround := effectRuleSrc("p", "approx", "block")
	workaround.Bytes = []byte(strings.Replace(string(workaround.Bytes), "      phase: enforce\n", "", 1))
	wa := Lint([]Source{workaround}).Diagnostics()
	if !hasCode(wa, CodeNoImplicitEnforcePhase, "approx") {
		t.Errorf("a phase-less rule using effect as a rollout workaround must still be flagged, got %v", codesOf(wa))
	}
}

// TestUnkeyedListEntry — REQ-E3-S02-03: a `mode: list` entry with no
// identity.pointer yields unkeyed-list naming the entry; a `mode: document`
// entry and a keyed list (identity.pointer present) lint clean.
func TestUnkeyedListEntry(t *testing.T) {
	unkeyed := Source{
		Path: ".assent/packs/p/rules/topics.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: topics
spec:
  entries:
    topic:
      mode: list
      root: "/topics"
  rules: []
`),
	}
	ud := Lint([]Source{unkeyed}).Diagnostics()
	if !hasCode(ud, CodeUnkeyedList, "topic") {
		t.Errorf("a mode:list entry without identity.pointer must yield unkeyed-list naming the entry, got %v", codesOf(ud))
	}

	// mode:document (no identity required) + a keyed list (identity.pointer set)
	// both lint clean.
	clean := Source{
		Path: ".assent/packs/p/rules/keyed.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: keyed
spec:
  entries:
    doc:
      mode: document
      root: "/config"
    partition:
      mode: list
      root: "/partitions"
      identity:
        pointer: "/id"
  rules: []
`),
	}
	cd := Lint([]Source{clean}).Diagnostics()
	if countCode(cd, CodeUnkeyedList) != 0 {
		t.Errorf("mode:document and a keyed list must lint clean, got %v", codesOf(cd))
	}
}

// TestMultiCauseSchemaInvalidIsDeterministic — REQ-E3-S02-04 determinism
// regression: a single doc with TWO schema defects (a missing-phase rule AND a
// missing-identity list entry) makes the strict loader emit a multi-cause
// jsonschema error whose sibling causes come back in NON-DETERMINISTIC
// map-iteration order. The schema-invalid capture must canonicalize that ordering
// so lint output is byte-stable run-to-run (else S08's golden corpus flakes). This
// doc is NOT phase-only, so its schema-invalid is retained (the phase dedupe is
// surgical) — hence the co-located non-determinism must be neutralized in place.
func TestMultiCauseSchemaInvalidIsDeterministic(t *testing.T) {
	twoDefects := Source{
		Path: ".assent/packs/q/rules/x.yaml",
		Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: x
spec:
  entries:
    unkeyed:
      mode: list
      root: "/xs"
  rules:
    - name: x
      match:
        files:
          paths: ["a/**"]
      effect: block
`),
	}
	// Many runs: any surviving map-order leak would eventually differ.
	base := Lint([]Source{twoDefects}).Render()
	for i := 0; i < 64; i++ {
		if got := Lint([]Source{twoDefects}).Render(); got != base {
			t.Fatalf("multi-cause schema-invalid not byte-stable on run %d:\n--- base ---\n%s\n--- got ---\n%s", i, base, got)
		}
	}
	// Sanity: the retained schema-invalid still carries BOTH causes.
	var schemaMsg string
	for _, d := range Lint([]Source{twoDefects}).Diagnostics() {
		if d.Code == CodeSchemaInvalid {
			schemaMsg = d.Message
		}
	}
	if !strings.Contains(schemaMsg, "identity") || !strings.Contains(schemaMsg, "phase") {
		t.Errorf("canonicalized schema-invalid must retain both causes, got %q", schemaMsg)
	}
}

// TestStructuralChecksComposeDeterministic — REQ-E3-S02-04: the three structural
// checks compose with the S01 obligation-coverage check in one deterministic run
// (double-run byte-identical). Purity is guarded by TestCorePurity over
// internal/lint; this asserts the ordering/composition half.
func TestStructuralChecksComposeDeterministic(t *testing.T) {
	sources := []Source{
		// assent-policy binding requiring an obligation no rule proves (obligation-
		// coverage) AND routing to a comment rule (reserved-class).
		policyBindingSrc("pol", "policy-ok"),
		effectRuleSrc("pol", "advise", "comment"),
		// a rule missing phase (no-implicit-enforce-phase) + an unkeyed list entry.
		Source{Path: ".assent/packs/q/rules/x.yaml", Bytes: []byte(`apiVersion: assent.dev/v1alpha1
kind: MergePolicy
metadata:
  name: x
spec:
  entries:
    unkeyed:
      mode: list
      root: "/xs"
  rules:
    - name: x
      match:
        files:
          paths: ["a/**"]
      effect: block
`)},
	}
	first := Lint(sources).Render()
	second := Lint(sources).Render()
	if first != second {
		t.Fatalf("structural composition not byte-identical across runs:\n--- first ---\n%s--- second ---\n%s", first, second)
	}
	diags := Lint(sources).Diagnostics()
	// Every one of the four codes must be represented (composition, not mutual
	// suppression) — reserved-class, no-implicit-enforce-phase, unkeyed-list, and
	// the S01 obligation-coverage.
	for _, code := range []string{CodeReservedClass, CodeNoImplicitEnforcePhase, CodeUnkeyedList, CodeObligationCoverage} {
		if countCode(diags, code) == 0 {
			t.Errorf("composed run must include a %q diagnostic, got %v", code, codesOf(diags))
		}
	}
	// Canonically sorted.
	for i := 1; i < len(diags); i++ {
		if sortKey(diags[i-1]) > sortKey(diags[i]) {
			t.Errorf("composed diagnostics not canonically sorted at %d", i)
		}
	}
}
