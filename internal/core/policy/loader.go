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
// schema, then decodes it into engine types. An authored match.fileEvents
// domain — allowed by the schema but deferred by E1 — is rejected here.
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
			return nil, fmt.Errorf("merge-policy: rule %q (index %d): match.fileEvents domain is deferred (E1 fast-follow, not implemented in E2); use files, values, or valueChanges", r.Name, i)
		}
	}
	return &mp, nil
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
