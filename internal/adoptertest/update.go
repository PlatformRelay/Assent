package adoptertest

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// update.go is the PURE half of the S05 `--update` golden-refresh flow. On a FAILING
// case, `assent test --update` rewrites the case's expect.yaml so the golden reflects
// the produced actuals — but the ONLY justification for --update over hand-editing is
// review-by-diff, which a naive yaml.Marshal(actual) defeats by clobbering the author's
// explanatory comments (the real fixtures carry `# partitions 12 -> 16 ...`, Judgment
// call (e)). UpdateExpectationBlock therefore rewrites the expectation payload IN PLACE
// via yaml.v3 Node surgery, preserving the comments authored against the surrounding
// nodes, rather than re-emitting a fresh comment-free block.
//
// This function is pure (bytes -> bytes): no filesystem, clock, env, network, or random.
// The FS write lives in cmd/assent/test.go (the sanctioned I/O boundary); this library
// stays pure. It is fail-closed: the rewritten bytes MUST re-validate against the frozen
// test-expectation schema (LoadExpectation), so --update never writes a golden the runner
// would reject.

// UpdateExpectationBlock rewrites the `#/$defs/expectation` payload of an authored
// expect.yaml (`original`) with the produced `actual` expectation, PRESERVING the
// authored comments on the surrounding nodes (a HeadComment before a key, an inline
// LineComment, a FootComment). It merges key-by-key: a key present in both keeps the
// author's original key node — and thus its comments — while taking the actual's value;
// a value node carries forward the original value node's comments when the fresh value
// has none; a key only in `actual` is appended (fresh); a key only in `original` (no
// longer part of the golden — e.g. a now-empty `findings`) is dropped.
//
// Determinism (REQ-E6-S05-04): the fresh block is built from `actual`'s stable struct
// field order, the merge walks the original's declared order then appends fresh-only
// keys in field order, and the whole document is re-encoded at the fixtures' 2-space
// indentation — so the same actuals over the same authored bytes yield byte-identical
// output. Re-emitting at 2-space (not yaml.v3's 4-space default) keeps the --update diff
// to the payload change, not a whole-file reflow that would defeat review-by-diff.
//
// FAIL-CLOSED (REQ-E6-S05-01): the rewritten bytes are re-validated via LoadExpectation
// (the frozen-schema strict decode). A rewrite that would not re-validate is an error,
// never silent bytes.
func UpdateExpectationBlock(original []byte, actual Expectation) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(original, &doc); err != nil {
		return nil, fmt.Errorf("parse expect.yaml for --update: %w", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expect.yaml is not a single expectation mapping (cannot --update in place)")
	}
	root := doc.Content[0]

	// Build the fresh expectation mapping from `actual` (stable struct field order).
	freshBytes, err := yaml.Marshal(actual)
	if err != nil {
		return nil, fmt.Errorf("marshal actual expectation: %w", err)
	}
	var freshDoc yaml.Node
	if err := yaml.Unmarshal(freshBytes, &freshDoc); err != nil {
		return nil, fmt.Errorf("parse actual expectation: %w", err)
	}
	if len(freshDoc.Content) != 1 || freshDoc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("actual expectation did not marshal to a mapping")
	}
	fresh := freshDoc.Content[0]

	doc.Content[0] = mergeExpectationMapping(root, fresh)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("re-encode expect.yaml for --update: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close --update encoder: %w", err)
	}
	out := buf.Bytes()

	// Fail-closed: the rewritten golden MUST re-validate against the frozen schema.
	if _, err := LoadExpectation(out); err != nil {
		return nil, fmt.Errorf("--update produced an expect.yaml that does not re-validate against the frozen schema: %w", err)
	}
	return out, nil
}

// mergeExpectationMapping merges the fresh expectation mapping onto the original,
// preserving the original's comments. It walks the original's key order first (so a key
// present in both keeps its authored position AND its key-node comments, taking only the
// fresh value), then appends keys that exist only in fresh. A key present only in the
// original is dropped (it is no longer part of the golden). Deterministic: both inputs
// are ordered node slices; no Go map is ranged to build output.
func mergeExpectationMapping(original, fresh *yaml.Node) *yaml.Node {
	out := &yaml.Node{
		Kind:        yaml.MappingNode,
		Tag:         original.Tag,
		Style:       original.Style,
		HeadComment: original.HeadComment,
		LineComment: original.LineComment,
		FootComment: original.FootComment,
	}
	used := make(map[string]bool, len(fresh.Content)/2)

	// Shared keys, in the author's original order: keep the original key node (comments)
	// and take the fresh value, carrying forward the original value node's comments when
	// the fresh value has none.
	for i := 0; i+1 < len(original.Content); i += 2 {
		key := original.Content[i]
		oldVal := original.Content[i+1]
		freshVal := mappingValue(fresh, key.Value)
		if freshVal == nil {
			continue // dropped: no longer part of the golden.
		}
		used[key.Value] = true
		carryValueComments(freshVal, oldVal)
		out.Content = append(out.Content, key, freshVal)
	}

	// Fresh-only keys (e.g. a `score` the stale golden omitted), in fresh field order.
	for i := 0; i+1 < len(fresh.Content); i += 2 {
		key := fresh.Content[i]
		if used[key.Value] {
			continue
		}
		out.Content = append(out.Content, key, fresh.Content[i+1])
	}
	return out
}

// mappingValue returns the value node for key in a mapping node, or nil when absent.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// carryValueComments copies the original value node's comments onto the fresh value node
// only where the fresh node has none — so an inline comment authored against a scalar
// value (e.g. `threshold: 4 # binding cap`) survives a value refresh, without overriding
// any comment the fresh value already carries.
func carryValueComments(fresh, original *yaml.Node) {
	if fresh.HeadComment == "" {
		fresh.HeadComment = original.HeadComment
	}
	if fresh.LineComment == "" {
		fresh.LineComment = original.LineComment
	}
	if fresh.FootComment == "" {
		fresh.FootComment = original.FootComment
	}
}
