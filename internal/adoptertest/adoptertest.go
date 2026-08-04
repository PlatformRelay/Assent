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

// Expectation is the decoded expect.yaml. S01 consumes only Decision (the coarse
// assertion); findings/absent/score are matched by S03, so they are validated by the
// frozen schema here but not yet projected onto this struct.
type Expectation struct {
	Decision string `yaml:"decision"`
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
}

// Outcome is a single case's evaluated result: whether the produced Decision matched
// the expectation, plus the values for reporting (the rich diff UX is S04).
type Outcome struct {
	Name     string
	Pass     bool
	Expected string
	Actual   string
}

// Evaluate assembles the EvaluationInput from the case's base/↔head/ diff (the
// production change.Diff + the reused evaldecode.BuildEvaluationInput), attaches the
// stubbed resolved facts, and evaluates the pack via aggregate.Cover. An opaque
// (undecidable) diff maps to the fail-safe REVIEW decision — never a silent APPROVE
// (GUIDELINES §2), mirroring the CLI run path. Pure and deterministic.
func Evaluate(c Case) (aggregate.Result, error) {
	cs, err := change.Diff(c.File, c.Base, c.Head)
	if err != nil {
		// The differ returns a wrapped ErrOpaque alongside the fail-safe ChangeSet;
		// that is an undecidable INPUT, not a harness failure -> REVIEW.
		if errors.Is(err, change.ErrOpaque) {
			return aggregate.Result{Decision: aggregate.DecisionReview}, nil
		}
		return aggregate.Result{}, fmt.Errorf("diff %s: %w", c.File, err)
	}
	in := evaldecode.BuildEvaluationInput(cs, aggregate.MR{}, requireOf(c.Bind))
	if len(c.Facts) > 0 {
		in.Facts = c.Facts
	}
	res, err := aggregate.Cover(c.Policy, c.Bind, &in)
	if err != nil {
		return aggregate.Result{}, fmt.Errorf("cover %q: %w", c.Name, err)
	}
	return res, nil
}

// RunCase evaluates the case and asserts the produced Decision equals expect.yaml's
// decision. S01 asserts the decision only; the finding/absent/score matcher is S03.
func RunCase(c Case) (Outcome, error) {
	res, err := Evaluate(c)
	if err != nil {
		return Outcome{}, fmt.Errorf("case %q: %w", c.Name, err)
	}
	actual := string(res.Decision)
	return Outcome{
		Name:     c.Name,
		Pass:     actual == c.Expect.Decision,
		Expected: c.Expect.Decision,
		Actual:   actual,
	}, nil
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
