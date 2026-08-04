package policy

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/schemas"
)

// schemaValidator is the minimal surface of a compiled jsonschema.Schema the
// loader needs. Declaring it locally lets policy validate against the frozen
// schemas without importing jsonschema directly.
type schemaValidator interface {
	Validate(any) error
}

// LoadMergePolicy validates raw (YAML or JSON) against the frozen MergePolicy
// schema, then decodes it into engine types. A match.fileEvents domain is
// accepted when its kinds are a subset of the whole-file lifecycle kinds the
// engine can emit and match ({add, delete}); a kinds entry of modify or rename —
// for which no whole-file event is ever minted (EFE-S01) — is rejected here with
// a located error. This loader-level NARROWING sits on top of the frozen schema
// (which still accepts the full add/modify/delete/rename enum): it closes a
// vacuous-cover fail-open — a required obligation proven only by an un-emittable
// kind would otherwise be silently covered -> APPROVE (Judgment call (b)).
func LoadMergePolicy(raw []byte) (*MergePolicy, error) {
	if err := validate("merge-policy", schemas.MergePolicySchema, raw); err != nil {
		return nil, err
	}
	var mp MergePolicy
	if err := decode(raw, &mp); err != nil {
		return nil, fmt.Errorf("merge-policy decode: %w", err)
	}
	for i, r := range mp.Spec.Rules {
		if r.Match.FileEvents != nil {
			for _, k := range r.Match.FileEvents.Kinds {
				if !fileEventKindEmittable(k) {
					return nil, fmt.Errorf("merge-policy: rule %q (index %d): match.fileEvents kind %q is not supported — only add and delete whole-file events are emitted; modify and rename are deferred", r.Name, i, k)
				}
			}
		}
	}
	return &mp, nil
}

// fileEventKindEmittable reports whether a fileEvents kind names a whole-file
// lifecycle event the engine can actually emit and match. Only add and delete
// qualify (EFE-S01); modify and rename are deferred (no minting path exists), so
// a rule naming them is rejected at load rather than left to match nothing.
func fileEventKindEmittable(kind string) bool {
	return kind == "add" || kind == "delete"
}

// LoadRulesetBinding validates and decodes a RulesetBinding.
func LoadRulesetBinding(raw []byte) (*RulesetBinding, error) {
	if err := validate("ruleset-binding", schemas.RulesetBindingSchema, raw); err != nil {
		return nil, err
	}
	var rb RulesetBinding
	if err := decode(raw, &rb); err != nil {
		return nil, fmt.Errorf("ruleset-binding decode: %w", err)
	}
	return &rb, nil
}

// LoadConfig validates and decodes a Config.
func LoadConfig(raw []byte) (*Config, error) {
	if err := validate("config", schemas.ConfigSchema, raw); err != nil {
		return nil, err
	}
	var c Config
	if err := decode(raw, &c); err != nil {
		return nil, fmt.Errorf("config decode: %w", err)
	}
	return &c, nil
}

// LoadPack validates and decodes a Pack manifest.
func LoadPack(raw []byte) (*Pack, error) {
	if err := validate("pack", schemas.PackSchema, raw); err != nil {
		return nil, err
	}
	var p Pack
	if err := decode(raw, &p); err != nil {
		return nil, fmt.Errorf("pack decode: %w", err)
	}
	return &p, nil
}

// validate normalizes raw (YAML or JSON) to a JSON value tree and validates it
// against the frozen schema. The schema is the single authority for
// unknown-field / unknown-enum / duplicate-id / required rejection, returning a
// located error (e.g. naming the offending field). Numbers are decoded as
// json.Number so the injective, non-lossy comparison the schemas rely on holds.
func validate(kind string, schema schemaValidator, raw []byte) error {
	var tree any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		return fmt.Errorf("%s: parse: %w", kind, err)
	}
	jsonBytes, err := json.Marshal(tree)
	if err != nil {
		return fmt.Errorf("%s: normalize: %w", kind, err)
	}
	dec := json.NewDecoder(bytes.NewReader(jsonBytes))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return fmt.Errorf("%s: normalize: %w", kind, err)
	}
	if err := schema.Validate(doc); err != nil {
		return fmt.Errorf("%s: %w", kind, err)
	}
	return nil
}

// decode decodes raw (YAML or JSON — yaml.v3 parses JSON as a YAML subset) into
// target. Strictness is already enforced by validate against the frozen schema,
// so this decode is lenient (schema-unknown fields cannot reach here).
func decode(raw []byte, target any) error {
	return yaml.Unmarshal(raw, target)
}

// UnmarshalYAML decodes an assert tree node structurally into exactly one of
// Leaf / All / Any / Not (ADR-0013): a scalar is a bare-CEL Leaf shorthand; a
// mapping is a {cel[,message]} leaf or an all/any/not combinator. The CEL text
// is stored verbatim, never compiled — E2-S02 owns cel.Compile.
func (a *AssertTree) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		a.Leaf = &Leaf{CEL: value.Value}
		return nil
	case yaml.MappingNode:
		keys := map[string]*yaml.Node{}
		for i := 0; i+1 < len(value.Content); i += 2 {
			keys[value.Content[i].Value] = value.Content[i+1]
		}
		switch {
		case keys["all"] != nil:
			return keys["all"].Decode(&a.All)
		case keys["any"] != nil:
			return keys["any"].Decode(&a.Any)
		case keys["not"] != nil:
			a.Not = &AssertTree{}
			return keys["not"].Decode(a.Not)
		case keys["cel"] != nil:
			leaf := &Leaf{CEL: keys["cel"].Value}
			if m := keys["message"]; m != nil {
				leaf.Message = m.Value
			}
			a.Leaf = leaf
			return nil
		default:
			return fmtAssertErr("mapping has none of all/any/not/cel")
		}
	default:
		return fmtAssertErr("node is neither a scalar nor a mapping")
	}
}
