package adoptertest

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/schemas"
)

// cases_file.go is the inline `cases.yaml` front-end (E6-S06). The frozen
// test-expectation contract is ONE oneOf: the directory-case `expect.yaml`
// (#/$defs/expectation, S01) and the inline `cases.yaml` shorthand
// (#/$defs/casesFile) are two spellings of the same fixture, and every
// `cases[].expect` reuses #/$defs/expectation verbatim. This file is only an
// ALTERNATE LOADER: it strict-decodes a cases.yaml against the frozen casesFile
// fragment, marshals each case's inline `base`/`head` CONTENTS to the file's own
// format (so the PRODUCTION change.Diff sees the same bytes a directory case's
// base/↔head/ files would), maps the inline facts, and builds the SAME
// adoptertest.Case the directory loader builds — which then runs the identical
// Evaluate/Match pipeline. It authors no new schema and re-implements none of the
// assembler.
//
// PURE: no filesystem I/O, clock, env, network, or randomness. The cases.yaml
// read (the only I/O) is the caller's job (cmd/assent/test.go).

// casesFileDoc / inlineCaseDoc decode the inline shorthand AFTER the raw bytes have
// strict-validated against the frozen #/$defs/casesFile fragment, so this non-strict
// yaml decode never has to re-police unknown keys — the schema already did. base/head/
// facts decode as `any` (the inline resource contents are of any shape); a `null` side
// (a new/deleted-file case) decodes to a nil interface, distinguishable from an empty
// document (an empty but non-nil map), which the format re-marshal preserves.
type casesFileDoc struct {
	Cases []inlineCaseDoc `yaml:"cases"`
}

type inlineCaseDoc struct {
	Name   string      `yaml:"name"`
	File   string      `yaml:"file"`
	Base   any         `yaml:"base"`
	Head   any         `yaml:"head"`
	Facts  any         `yaml:"facts"`
	Expect Expectation `yaml:"expect"`
}

// LoadInlineCases strict-decodes an inline cases.yaml against the FROZEN
// #/$defs/casesFile fragment (schemas.CasesFileSchema — the same contract, no new
// schema) and returns one adoptertest.Case per inline case, ready for RunCase over
// the shared pipeline. An unknown top-level key or a malformed inline `expect` is a
// LOCATED rejection (the fragment reports at the offending instance pointer, e.g.
// /cases/0/expect/decision, never a muddy top-level oneOf failure) — never a silent
// skip.
//
// Each case's inline `base`/`head` CONTENTS are marshaled to the `file`'s own format
// (matching change.Diff's extension-driven producer selection) so the case feeds the
// PRODUCTION differ exactly as a directory case would. A `null` `base` (new-file
// case) or `null` `head` (deleted-file case) marshals to ABSENT bytes: the production
// differ maps a whole-file add/delete's unparseable absent side to an opaque diff,
// which Evaluate fails safe to REVIEW — never a silent APPROVE (mirroring the CLI's
// dirCheckout.FileContents contract).
func LoadInlineCases(raw []byte, pol *policy.MergePolicy, bind *policy.Binding) ([]Case, error) {
	doc, err := jsonNumberTree(raw)
	if err != nil {
		return nil, fmt.Errorf("cases.yaml: %w", err)
	}
	if err := schemas.CasesFileSchema.Validate(doc); err != nil {
		return nil, fmt.Errorf("cases.yaml: %w", err)
	}
	var cf casesFileDoc
	if err := yaml.Unmarshal(raw, &cf); err != nil {
		return nil, fmt.Errorf("cases.yaml decode: %w", err)
	}

	out := make([]Case, 0, len(cf.Cases))
	for _, ic := range cf.Cases {
		base, err := marshalInlineContent(ic.File, ic.Base)
		if err != nil {
			return nil, fmt.Errorf("cases.yaml: case %q: base: %w", ic.Name, err)
		}
		head, err := marshalInlineContent(ic.File, ic.Head)
		if err != nil {
			return nil, fmt.Errorf("cases.yaml: case %q: head: %w", ic.Name, err)
		}
		facts, err := inlineFacts(ic.Facts)
		if err != nil {
			return nil, fmt.Errorf("cases.yaml: case %q: %w", ic.Name, err)
		}
		out = append(out, Case{
			Name:   ic.Name,
			Policy: pol,
			Bind:   bind,
			File:   ic.File,
			Base:   base,
			Head:   head,
			Facts:  facts,
			Expect: ic.Expect,
		})
	}
	return out, nil
}

// marshalInlineContent turns one inline `base`/`head` value into the bytes
// change.Diff consumes, marshaling to the file's OWN format so the producer
// change.Diff selects by extension parses them faithfully: `.json` → JSON,
// `.yaml`/`.yml`/anything-else → YAML (mirroring change.producerFor). A `.tfvars`
// file has no lossless inline marshal (its HCL producer round-trips no arbitrary
// value tree), so it is a clear error rather than a silently mis-parsed side.
//
// A NIL value is the absent side of a whole-file add/delete (an inline `null`): it
// returns nil bytes, which the production differ renders opaque → the fail-safe REVIEW
// (never a fabricated empty document that could diff into silent adds/deletes).
func marshalInlineContent(file string, v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	switch {
	case strings.HasSuffix(file, ".json"):
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal inline content as JSON: %w", err)
		}
		return b, nil
	case strings.HasSuffix(file, ".tfvars"):
		return nil, fmt.Errorf("inline base/head for a .tfvars file %q is unsupported (its HCL form has no lossless inline marshal — use a directory case)", file)
	default:
		b, err := yaml.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal inline content as YAML: %w", err)
		}
		return b, nil
	}
}

// inlineFacts lifts an inline case's optional `facts` block into the resolved-fact
// envelope via the SAME MapFacts the directory loader uses. An absent/null facts
// block yields the empty envelope (no fact is fabricated). The value is marshaled to
// JSON — valid input for MapFacts's yaml-or-json normalization.
func inlineFacts(v any) (map[string]map[string]aggregate.Fact, error) {
	if v == nil {
		return MapFacts(nil)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal inline facts: %w", err)
	}
	return MapFacts(b)
}
