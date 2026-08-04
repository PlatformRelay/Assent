// Package adoptertest is the PURE library behind `assent test` (P5-E6). It turns
// ADR-0014's frozen adopter test-fixture format into a runnable decision assertion:
// given one directory case's in-memory bytes (expect.yaml, facts.yaml, and a
// base/↔head/ file pair) plus the loaded pack, it strict-decodes the expectation
// against the frozen schema, lifts the authored facts into the resolved-fact
// envelope the engine binds, diffs base↔head with the PRODUCTION differ, and
// evaluates the pack via aggregate.Cover — asserting the produced Decision equals
// the pinned one.
//
// It sits UNDER internal/ (not internal/core), so — exactly like the extracted
// internal/evaldecode boundary it reuses — it may import internal/change and
// internal/core/aggregate while internal/core stays forge/change-free. It performs
// NO filesystem I/O, no clock/env/network/random: the case-directory walk is the
// caller's job (cmd/assent/test.go, the sanctioned boundary). Every function is pure
// and deterministic (ADR-0014 golden L0: a case runs twice byte-identical).
//
// S01 is the input-side anchor: it asserts the coarse Decision only. The
// finding/absent/score matcher (S03), whole multi-rule entry-tree replay (S02), and
// the inline cases.yaml front-end (S06) build on this base.
package adoptertest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/evaldecode"
	"github.com/PlatformRelay/assent/schemas"
)

// stateResolved is the ONLY fact state that exposes a `value` binding to a predicate
// (aggregate.factsToCEL gates on it). Declared here to keep this library free of any
// internal/core dependency beyond the exported aggregate types.
const stateResolved = "resolved"

// Expectation is the decoded expect.yaml (the frozen #/$defs/expectation). S01
// consumed only Decision; S03 projects the finding/absent/score/message assertions
// onto this struct so Match can check them against the engine's aggregate.Result.
// These fields are decoded from the SAME frozen contract LoadExpectation already
// strict-validates — no new schema is authored, only the Go projection widened.
type Expectation struct {
	Decision string `yaml:"decision"`
	// Findings is must-contain by DEFAULT (each listed finding must fire; extras
	// allowed). Exact:true closes the list (nothing else may fire). An OMITTED
	// exact never silently closes it (the frozen must-contain default). The
	// omitempty tags are for the S04 OUTPUT path only (MarshalExpectation emits a
	// ready-to-copy block that must strict-decode against the frozen schema — an
	// empty findings[]/absent[] or a nil score must be OMITTED, not rendered as
	// `null`/`[]`); omitempty never affects the decode/Match path.
	Findings []ExpectFinding `yaml:"findings,omitempty"`
	Exact    bool            `yaml:"exact,omitempty"`
	// Absent names rules that must NOT fire.
	Absent []string `yaml:"absent,omitempty"`
	// Score pins the risk arithmetic. A pointer so an ABSENT score never silently
	// asserts the zero {total:0, threshold:0} (which would fail-open on a real
	// non-zero total): nil ⇒ no score assertion.
	Score *ExpectScore `yaml:"score,omitempty"`
}

// ExpectFinding is one expected finding (#/$defs/finding). Rule+Effect identify it;
// Obligation, when set, further constrains the match. Path is DECIDED
// error-as-unsupported (D-054): aggregate.Finding carries no Path, so a Path
// assertion errors the case (fail-closed) rather than silently passing. Message
// (the frozen `message~` key) is a DISCOURAGED substring/regex match on the rendered
// finding message — a presentation assertion, never a safety one (not counted by
// --coverage, ADR-0014 amendment).
type ExpectFinding struct {
	Rule       string `yaml:"rule"`
	Obligation string `yaml:"obligation,omitempty"`
	Effect     string `yaml:"effect"`
	Path       string `yaml:"path,omitempty"`
	Message    string `yaml:"message~,omitempty"`
}

// ExpectScore pins the aggregated risk arithmetic: Total is the summed points and
// Threshold the binding's approve threshold (ADR-0007 sum(points) <= threshold).
type ExpectScore struct {
	Total     int `yaml:"total"`
	Threshold int `yaml:"threshold"`
}

// LoadExpectation strict-decodes an expect.yaml against the FROZEN
// test-expectation contract's #/$defs/expectation fragment (schemas.ExpectationSchema
// — no new schema is authored). An unknown field or a non-{APPROVE,REVIEW,BLOCK}
// decision is a LOCATED rejection (the fragment yields a field-level error, not a
// muddy top-level oneOf failure). Numbers normalize via json.Number so the frozen
// schema's numeric keywords hold.
func LoadExpectation(raw []byte) (Expectation, error) {
	doc, err := jsonNumberTree(raw)
	if err != nil {
		return Expectation{}, fmt.Errorf("expect.yaml: %w", err)
	}
	if err := schemas.ExpectationSchema.Validate(doc); err != nil {
		return Expectation{}, fmt.Errorf("expect.yaml: %w", err)
	}
	var e Expectation
	if err := yaml.Unmarshal(raw, &e); err != nil {
		return Expectation{}, fmt.Errorf("expect.yaml decode: %w", err)
	}
	return e, nil
}

// MapFacts lifts an authored facts.yaml (a provider -> name -> authored-value map)
// into the resolved-fact envelope map[provider]map[name]aggregate.Fact the engine
// binds. Each PRESENT (provider, name) becomes Fact{State:"resolved", Value:<authored
// value>}; numerics are carried as json.Number so aggregate.toCEL binds them as
// int64/float64 (a numeric compare), mirroring the differ's numeric discipline.
//
// An ABSENT provider or name yields NO Fact entry — never a fabricated resolved
// value. A predicate reading `.value` on the absent fact then ERRORS in cel-go, which
// the engine fails safe to REVIEW; fabricating a value would instead let a run APPROVE
// on a fact it never resolved (a fail-OPEN). Empty/whitespace input is an empty map.
func MapFacts(raw []byte) (map[string]map[string]aggregate.Fact, error) {
	out := map[string]map[string]aggregate.Fact{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return out, nil
	}
	tree, err := jsonNumberTree(raw)
	if err != nil {
		return nil, fmt.Errorf("facts.yaml: %w", err)
	}
	if tree == nil {
		return out, nil
	}
	top, ok := tree.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("facts.yaml: top level must be a provider map, got %T", tree)
	}
	for provider, namesAny := range top {
		names, ok := namesAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("facts.yaml: provider %q must map fact names to values, got %T", provider, namesAny)
		}
		byName := make(map[string]aggregate.Fact, len(names))
		for name, val := range names {
			byName[name] = aggregate.Fact{State: stateResolved, Value: val}
		}
		out[provider] = byName
	}
	return out, nil
}

// Case is one fully-loaded directory case ready to evaluate. The caller reads the
// files off disk (the only I/O) and hands the bytes + the loaded pack here.
type Case struct {
	Name   string
	Policy *policy.MergePolicy
	Bind   *policy.Binding
	// File is the changed file's repo-relative path; it drives change.Diff's
	// format selection (by extension) and the change's File field.
	File   string
	Base   []byte
	Head   []byte
	Facts  map[string]map[string]aggregate.Fact
	Expect Expectation
	// MR is the merge-request metadata bound to the `mr` predicate scope (S02).
	// The zero value is the ADR-0014 "mr.yaml absent" default (empty author, no
	// labels, no branches) — the fail-safe default, since ADR-0014 pins no specific
	// values. MapMR lifts an optional case mr.yaml into it.
	MR aggregate.MR
	// Approval is the optional stubbed approval context (S02): the evaluated
	// sourceSha + per-subject ApprovalEvidence that can SATISFY a require-review
	// obligation without a live forge. nil ⇒ no evidence (exactly Cover) ⇒ a
	// require-review obligation stays unsatisfied → REVIEW (never satisfied by
	// absence). MapApproval lifts an optional case approval.yaml into it.
	Approval *aggregate.ApprovalContext
}

// Outcome is a single case's evaluated result: whether the produced Decision AND the
// S03 finding/absent/score/message assertions matched, plus the values for reporting
// (the rich diff UX is S04). Reasons carries every mismatch (empty ⇒ Pass), in the
// deterministic decision-then-matcher order.
type Outcome struct {
	Name     string
	Pass     bool
	Expected string
	Actual   string
	// Reasons enumerates the mismatches that failed the case (decision first, then
	// the matcher's finding/absent/score reasons). Empty exactly when Pass is true.
	Reasons []string
	// ActualExpect is the expect.yaml block the produced Result would satisfy — the
	// decision, the findings that actually fired, and the score arithmetic,
	// reconstructed by ActualExpectation. S04's RenderFailure serializes it into a
	// ready-to-copy actual block; S05's --update reuses the SAME model to WRITE it.
	// Populated for every evaluated case (pass or fail); the diff UX renders it only
	// on failure.
	ActualExpect Expectation
}

// Evaluate assembles the EvaluationInput from the case's base/↔head/ diff, attaches
// the stubbed resolved facts, reconstructs the per-EntryRef entry tree (S02) so an
// entry-scoped predicate binds the whole entry object, threads the case MR and the
// stubbed ApprovalContext, and evaluates the WHOLE pack via aggregate.CoverWithApproval.
// An opaque (undecidable) diff maps to the fail-safe REVIEW decision — never a silent
// APPROVE (GUIDELINES §2), mirroring the CLI run path. Pure and deterministic.
//
// The differ path is chosen by the pack: a pack declaring a governed collection
// (spec.entries, S02) is diffed by the collection-aware change.DiffEntries so a
// list/map file is decidable AND each change is tagged with a stable EntryRef; a
// pack with no entries takes the S01 document-mode change.Diff verbatim (so every
// S01 case stays byte-identical). With no entries, no MR, and a nil Approval, this
// is exactly the S01 Cover path.
func Evaluate(c Case) (aggregate.Result, error) {
	in, decidable, err := assemble(c)
	if err != nil {
		return aggregate.Result{}, err
	}
	if !decidable {
		// Mirror the CLI run path's undecidable guard (run.go's decide): an opaque OR
		// EMPTY changeset is fail-safe REVIEW, never a silent APPROVE.
		return aggregate.Result{Decision: aggregate.DecisionReview}, nil
	}
	res, err := aggregate.CoverWithApproval(c.Policy, c.Bind, &in, c.Approval)
	if err != nil {
		return aggregate.Result{}, fmt.Errorf("cover %q: %w", c.Name, err)
	}
	return res, nil
}

// oneSidedFileEvent detects an UNAMBIGUOUS whole-file lifecycle from one-sided
// presence: exactly one of base/head ABSENT. An absent base + present head is a
// file-ADD; a present base + absent head is a file-DELETE. It returns (kind, true)
// ONLY for that unambiguous case; every other shape falls through to change.Diff —
// both present (a value diff or opaque), both absent (undecidable), or an
// empty-but-PRESENT side.
//
// THE AMBIGUITY INVARIANT (EFE-S02, Judgment call (a)): the presence signal is
// nil-vs-non-nil, NEVER len()==0. An empty-but-present document ({} / empty bytes)
// is non-nil and MUST NOT be mistaken for a delete — treating it as one is a
// fail-OPEN. This is the S06 nil-interface (`null`=absent) vs empty-map
// (`{}`=empty document) line: marshalInlineContent returns literal nil only for a
// `null` side and never a non-nil-empty slice for any present value, so nil is a
// clean absence signal. A rename is never synthesized here — a case governs one
// file, so at most one file-event is ever minted.
func oneSidedFileEvent(base, head []byte) (change.Kind, bool) {
	baseAbsent := base == nil
	headAbsent := head == nil
	switch {
	case baseAbsent && !headAbsent:
		return change.KindAdd, true
	case !baseAbsent && headAbsent:
		return change.KindDelete, true
	default:
		return "", false
	}
}

// assemble builds the case's EvaluationInput from the base/↔head/ diff, the
// stubbed resolved facts, and the reconstructed entry tree — the pure input side
// shared by Evaluate and the S07 coverage witness (RunCaseCoverage), so the
// coverage path sees the EXACT same ChangeSet and Result the plain run does.
// decidable is false for an OPAQUE or EMPTY changeset (an undecidable input):
// Cover treats a 0-change set as "the obligation does not apply" and reduces to a
// VACUOUS APPROVE, so both callers map decidable==false to the fail-safe outcome
// (REVIEW / no exercised rule) rather than a silent pass. The field check also
// hardens the opaque arm rather than relying solely on the Opaque⟹ErrOpaque
// invariant.
func assemble(c Case) (in aggregate.EvaluationInput, decidable bool, err error) {
	// One-sided presence = an UNAMBIGUOUS whole-file lifecycle (EFE-S02): mint a clean
	// file-event via change.FileEvent that the S01 fileEvents matcher can select,
	// instead of letting change.Diff go opaque on the nil side (the pre-S02 behaviour).
	// This runs BEFORE the entries/document differ split because a wholesale file
	// add/delete is a file-event in either mode. Every AMBIGUOUS shape (both present,
	// both absent, or an empty-but-PRESENT side) falls through to the differ unchanged.
	if kind, ok := oneSidedFileEvent(c.Base, c.Head); ok {
		cs := change.ChangeSet{Changes: []change.Change{change.FileEvent(c.File, kind)}}
		in = evaldecode.BuildEvaluationInput(cs, c.MR, requireOf(c.Bind))
		if len(c.Facts) > 0 {
			in.Facts = c.Facts
		}
		return in, true, nil
	}

	cfg, entered := singleEntryConfig(c.Policy)

	var cs change.ChangeSet
	if entered {
		cs, err = change.DiffEntries(c.File, c.Base, c.Head, cfg)
	} else {
		cs, err = change.Diff(c.File, c.Base, c.Head)
	}
	if err != nil {
		// The differ returns a wrapped ErrOpaque alongside the fail-safe ChangeSet;
		// that is an undecidable INPUT, not a harness failure.
		if errors.Is(err, change.ErrOpaque) {
			return aggregate.EvaluationInput{}, false, nil
		}
		return aggregate.EvaluationInput{}, false, fmt.Errorf("diff %s: %w", c.File, err)
	}
	if cs.Opaque || len(cs.Changes) == 0 {
		return aggregate.EvaluationInput{}, false, nil
	}
	in = evaldecode.BuildEvaluationInput(cs, c.MR, requireOf(c.Bind))
	if len(c.Facts) > 0 {
		in.Facts = c.Facts
	}
	if entered {
		// Reconstruct the whole-entry objects for the changed file's EntryRefs and
		// populate EvalChange.Entry/OldEntry (Part A binds them when present, else
		// the scalar fallback). Fail-safe: on any reconstruction error every Entry
		// stays nil (scalar fallback), never a partial/permissive entry (REQ-07).
		populateEntries(&in, cs, c, cfg)
	}
	return in, true, nil
}

// RunCase evaluates the case, asserts the produced Decision equals expect.yaml's
// decision, THEN runs the S03 finding/absent/score/message matcher over the same
// Result. The decision is checked first (the coarse safety assertion); the matcher's
// reasons are appended after it. A fail-closed matcher error (an unevaluable
// findings[].path assertion, D-054) propagates as a case error — never a silent
// pass. Pass is true exactly when the decision matched AND the matcher found no
// mismatch.
func RunCase(c Case) (Outcome, error) {
	res, err := Evaluate(c)
	if err != nil {
		return Outcome{}, fmt.Errorf("case %q: %w", c.Name, err)
	}
	return outcomeFrom(c, res)
}

// outcomeFrom builds the case Outcome from an already-produced Result: it asserts
// the coarse decision first, then appends the S03 matcher's finding/absent/score
// reasons. Shared by RunCase and RunCaseCoverage (S07) so a single evaluation
// yields the same Outcome on both paths. A fail-closed matcher error (an
// unevaluable findings[].path assertion, D-054) propagates — never a silent pass.
func outcomeFrom(c Case, res aggregate.Result) (Outcome, error) {
	actual := string(res.Decision)

	var reasons []string
	if actual != c.Expect.Decision {
		reasons = append(reasons, fmt.Sprintf("decision: expected %s, got %s", c.Expect.Decision, actual))
	}

	matchReasons, err := Match(c.Expect, res, thresholdOf(c.Bind))
	if err != nil {
		return Outcome{}, fmt.Errorf("case %q: %w", c.Name, err)
	}
	reasons = append(reasons, matchReasons...)

	return Outcome{
		Name:         c.Name,
		Pass:         len(reasons) == 0,
		Expected:     c.Expect.Decision,
		Actual:       actual,
		Reasons:      reasons,
		ActualExpect: ActualExpectation(res, thresholdOf(c.Bind)),
	}, nil
}

// thresholdOf returns the binding's risk approve threshold (nil-safe). The engine
// reduces to REVIEW when sum(points) exceeds it (ADR-0007); Match reads it because
// aggregate.Result exposes no threshold of its own.
func thresholdOf(b *policy.Binding) int {
	if b == nil {
		return 0
	}
	return b.Risk.Threshold
}

// requireOf returns the binding's required obligations (nil-safe).
func requireOf(b *policy.Binding) []string {
	if b == nil {
		return nil
	}
	return b.Require
}

// jsonNumberTree normalizes YAML (or JSON) bytes into the any-tree the frozen JSON
// schemas validate, decoding numbers as json.Number so numeric schema keywords hold
// and lifted fact values stay injective (no float64 collapse). It mirrors the E2
// policy loader's strict-decode normalization.
func jsonNumberTree(raw []byte) (any, error) {
	var tree any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	jsonBytes, err := json.Marshal(tree)
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(jsonBytes))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}
	return doc, nil
}
