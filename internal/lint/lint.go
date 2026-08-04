// Package lint is assent's pure `assent lint` check library over the `.assent/**`
// authoring surface. It is the E3 anchor: the diagnostic model, the TOLERANT
// ingestion, and the first hard error (obligation coverage). Every later E3 check
// (structural, facts-reference, predicate-scope, config-posture, tests-per-rule)
// plugs into the same Report accumulator.
//
// Decide-and-log (E3-S01, judgment calls (a) and (b)):
//
//	(a) `assent lint` is a SUBCOMMAND of the single `assent` binary, not a second
//	    binary — it reuses cmd/assent/main.go's dispatch (see cmd/assent/lint.go).
//	    An `assent-lint` alias would be an operator-driven OQ; none is opened here.
//
//	(b) Lint ingests TOLERANTLY (fail-many), NOT via the strict E2 loader as the
//	    sole gate. policy.LoadMergePolicy et al. die on the FIRST unknown-field /
//	    unknown-enum / missing-phase error — they refuse the exact malformed packs
//	    lint exists to diagnose. So Ingest decodes each doc BEST-EFFORT with a
//	    non-strict YAML pass into the SAME policy.* types AND runs the strict loader
//	    separately, capturing its error as ONE schema-invalid diagnostic located to
//	    the offending doc. A strict-schema violation therefore becomes one
//	    diagnostic among the hard errors, never a first-error abort — this is the
//	    load-bearing architectural decision of the epic.
//
// Purity: this package is pure — no clock/env/net/random. The tolerant decode and
// the strict-loader reuse are both pure (yaml + the frozen JSON-schema validators);
// the only I/O — the `.assent/**` directory walk — lives in cmd/assent, the
// sanctioned boundary. (TestCorePurity scans internal/core/**; lint stays pure by
// construction, with TestLintReportDoubleRunStable as the live determinism guard.)
package lint

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// Severity is a diagnostic's severity. Every E3 hard error is an error (it fails
// the run); the type exists so a future advisory tier is additive, not a rewrite.
type Severity string

// SeverityError is the only severity E3 emits: a hard error that fails the run.
const SeverityError Severity = "error"

// Diagnostic codes. Each names a distinct hard error (or an ingestion fault) so
// the sort key and any per-code assertion is stable across releases.
const (
	// CodeObligationCoverage: a Binding require[] obligation no bound rule proves.
	CodeObligationCoverage = "obligation-coverage"
	// CodeSchemaInvalid: a doc the strict loader rejects (the tolerant-ingestion
	// bridge — the strict loader's first-error abort captured as one diagnostic).
	CodeSchemaInvalid = "schema-invalid"
	// CodeParseError: a doc that is not parseable YAML at all.
	CodeParseError = "parse-error"
)

// Location points a diagnostic at a source doc and, when the fault is attributable
// to a named construct, the offending rule/binding. File is the repo-relative,
// slash-separated path; Name is empty for a whole-document fault.
type Location struct {
	File string
	Name string
}

// String renders the location for printing: "file (name)" when a construct is
// named, else "file". It is also the location component of the sort key.
func (l Location) String() string {
	if l.Name == "" {
		return l.File
	}
	return l.File + " (" + l.Name + ")"
}

// Diagnostic is one located, actionable finding. Severity is always an error in
// E3 (a hard error fails the run, ADR-0010 / lint-hard-errors.md).
type Diagnostic struct {
	Code     string
	Severity Severity
	Location Location
	Message  string
}

// sortKey is the TOTAL canonical ordering key over everything a diagnostic prints
// (code, then location, then message). Determinism is by this key alone — no
// map-iteration or ingest order leaks into output (REQ-E3-S01-05).
func sortKey(d Diagnostic) string {
	return d.Code + "\x00" + d.Location.String() + "\x00" + d.Message
}

// Report accumulates diagnostics from every check over one ingestion. It is the
// shared accumulator every E3 check appends to.
type Report struct {
	diags []Diagnostic
}

// add appends one diagnostic.
func (r *Report) add(d Diagnostic) { r.diags = append(r.diags, d) }

// addError appends an error-severity diagnostic — the E3 convenience path.
func (r *Report) addError(code string, loc Location, msg string) {
	r.add(Diagnostic{Code: code, Severity: SeverityError, Location: loc, Message: msg})
}

// Diagnostics returns the accumulated diagnostics in canonical order (a fresh,
// sorted copy each call — the accumulator is not mutated).
func (r *Report) Diagnostics() []Diagnostic {
	out := make([]Diagnostic, len(r.diags))
	copy(out, r.diags)
	sort.SliceStable(out, func(i, j int) bool { return sortKey(out[i]) < sortKey(out[j]) })
	return out
}

// HasErrors reports whether any error-severity diagnostic is present — the
// non-zero-exit signal for `assent lint`.
func (r *Report) HasErrors() bool {
	for _, d := range r.diags {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Render returns the canonical multi-line rendering (one diagnostic per line, in
// sort order). It is the byte-identical-double-run surface (REQ-E3-S01-05) and
// what cmd/assent prints to stderr.
func (r *Report) Render() string {
	var b []byte
	for _, d := range r.Diagnostics() {
		b = append(b, fmt.Sprintf("%s: [%s] %s: %s\n", d.Severity, d.Code, d.Location, d.Message)...)
	}
	return string(b)
}

// Source is one ingested `.assent/**` document: its repo-relative slash path (for
// the Location + pack-name derivation) and raw bytes. cmd/assent walks the tree
// and hands these in; internal/lint does no filesystem I/O.
type Source struct {
	Path  string
	Bytes []byte
}

// Lint ingests the sources tolerantly and runs every E3-S01 check, returning the
// populated Report. It is the single entry point cmd/assent calls.
func Lint(sources []Source) *Report {
	rep := &Report{}
	model := ingest(sources, rep)
	checkObligationCoverage(model, rep)
	checkFactsReferences(model, rep)
	checkStructural(model, rep)
	checkPredicateScope(model, rep)
	return rep
}

// docKind decodes only the top-level `kind` discriminator, tolerantly. A doc that
// does not parse as a YAML mapping returns a non-nil error (surfaced as a parse
// diagnostic); a doc with no `kind` returns "" (skipped as a non-policy doc).
func docKind(raw []byte) (string, error) {
	var header struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(raw, &header); err != nil {
		return "", err
	}
	return header.Kind, nil
}
