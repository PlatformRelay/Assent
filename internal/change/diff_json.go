package change

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// parseJSON is the JSON producer for the canonical value tree (E1-S03). It projects a JSON
// document into the SAME *vnode tree the YAML producer builds, so the format-neutral walker
// (walkNode) compares JSON exactly as it compares YAML and yields the same ChangeSet shape.
//
// Fail-safe / injective discipline, matching the YAML producer:
//   - The root MUST be a JSON object; a bare array, bare scalar, or bare null root is opaque
//     (not a field-diffable shape) — mirrors YAML's root-must-be-a-mapping gate.
//   - Numbers are decoded with UseNumber() and carried as their RAW json.Number literal, never a
//     float64 — so two distinct integers that would collapse under a lossy float64 decode
//     (e.g. 2^53+1 vs 2^53) stay distinct and a real change is never silently missed.
//   - Strings render JSON-quoted (so a string never collides with a number/bool/null spelling);
//     the comparison key is tag-qualified (a synthetic per-type tag + the render), so a scalar's
//     KIND change (number 1 -> string "1") is a detected change, never a collapse.
//   - Duplicate object keys are rejected (encoding/json would silently keep last-wins).
//   - Malformed/truncated bytes, or trailing content after the root document, are opaque.
//
// An ARRAY is projected as a vSequence LEAF (its elements are consumed to keep parsing honest but
// not walked) — per-element list walking is E1-S05 territory, so any array reached during the walk
// is opaque, exactly as the YAML differ treats a sequence value.
//
// PURE: no clock/env/network/random. Positions are derived deterministically from each value's
// byte offset in the input (offsetToPos), so the JSON path double-runs byte-identical.
func parseJSON(data []byte) (*vnode, string) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	root, reason := parseJSONValue(dec, data)
	if reason != "" {
		return nil, reason
	}
	if root.kind != vMapping {
		return nil, "JSON root is not an object — not a field-diffable shape"
	}
	// Reject trailing content after the root document (e.g. `{...} {...}`): the next token must
	// be clean EOF. A non-EOF token, or any non-EOF error, fails closed.
	if _, err := dec.Token(); !errorsIsEOF(err) {
		if err != nil {
			return nil, "trailing content did not decode cleanly: " + err.Error()
		}
		return nil, "trailing content after JSON document — more than one value"
	}
	return root, ""
}

// parseJSONValue reads exactly one JSON value from the decoder and projects it into a *vnode,
// recording the value's 1-indexed source position (the offset of its first non-separator byte).
// It returns a non-empty reason on any decode error or unrepresentable shape.
func parseJSONValue(dec *json.Decoder, data []byte) (*vnode, string) {
	// The value begins at the first byte at/after the current offset that is not whitespace or a
	// structural separator (`:` before an object value, `,` between elements). A JSON value never
	// starts with one of those bytes, so skipping them lands exactly on the value's first byte.
	start := valueStart(data, int(dec.InputOffset()))

	tok, err := dec.Token()
	if err != nil {
		if errorsIsEOF(err) {
			return nil, "no JSON value present (empty or truncated input)"
		}
		return nil, "does not decode as JSON: " + err.Error()
	}

	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return parseJSONObject(dec, data, start)
		case '[':
			elems, reason := parseJSONArray(dec, data)
			if reason != "" {
				return nil, reason
			}
			return &vnode{kind: vSequence, elems: elems, elemsProjected: true, pos: offsetToPos(data, start)}, ""
		default: // '}' or ']' with no matching opener — malformed
			return nil, fmt.Sprintf("unexpected JSON delimiter %q", t)
		}
	case string:
		render := jsonQuote(t)
		return &vnode{kind: vScalar, render: render, cmpKey: "s\x00" + render, pos: offsetToPos(data, start)}, ""
	case json.Number:
		lit := t.String()
		return &vnode{kind: vScalar, render: lit, cmpKey: "n\x00" + lit, pos: offsetToPos(data, start)}, ""
	case bool:
		lit := "false"
		if t {
			lit = "true"
		}
		return &vnode{kind: vScalar, render: lit, cmpKey: "b\x00" + lit, pos: offsetToPos(data, start)}, ""
	case nil:
		return &vnode{kind: vScalar, render: "null", cmpKey: "z\x00null", pos: offsetToPos(data, start)}, ""
	default:
		return nil, fmt.Sprintf("unsupported JSON token %T", tok)
	}
}

// parseJSONObject reads the body of a JSON object (the opening '{' already consumed) into a
// vMapping, recursing on each value. It rejects a duplicate key (fail-closed — encoding/json would
// silently keep the last value, a silent-miss risk) and any decode error.
func parseJSONObject(dec *json.Decoder, data []byte, start int) (*vnode, string) {
	fields := make(map[string]*vnode)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, "object key did not decode: " + err.Error()
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Sprintf("non-string JSON object key %v", keyTok)
		}
		if _, dup := fields[key]; dup {
			return nil, fmt.Sprintf("duplicate JSON object key %q — not decidable (last-wins would silently drop a change)", key)
		}
		val, reason := parseJSONValue(dec, data)
		if reason != "" {
			return nil, reason
		}
		fields[key] = val
	}
	// Consume the closing '}'.
	if _, err := dec.Token(); err != nil {
		return nil, "object not closed: " + err.Error()
	}
	return &vnode{kind: vMapping, fields: fields, pos: offsetToPos(data, start)}, ""
}

// parseJSONArray reads an array body to its matching ']' (the '[' already consumed), projecting
// each element into a vnode (recursively, so nested objects/arrays are represented). The elements
// populate vSequence.elems, which E1-S05's `list` mode walks; the document-mode walker still
// treats the vSequence as opaque regardless. A decode error inside the array fails closed.
func parseJSONArray(dec *json.Decoder, data []byte) ([]*vnode, string) {
	var elems []*vnode
	for dec.More() {
		el, reason := parseJSONValue(dec, data)
		if reason != "" {
			return nil, reason
		}
		elems = append(elems, el)
	}
	// Consume the closing ']'.
	if _, err := dec.Token(); err != nil {
		return nil, "array not closed: " + err.Error()
	}
	return elems, ""
}

// valueStart returns the offset of a value's first byte, scanning from `from` past JSON
// whitespace and the structural separators (`:` and `,`) the decoder consumes between tokens. A
// JSON value never begins with one of those, so the first byte outside that set is the value.
func valueStart(data []byte, from int) int {
	i := from
	for i < len(data) {
		switch data[i] {
		case ' ', '\t', '\r', '\n', ':', ',':
			i++
		default:
			return i
		}
	}
	return from
}

// offsetToPos converts a 0-indexed byte offset into a 1-indexed line/column Position, counting
// newlines up to the offset. Deterministic (pure function of data + offset).
func offsetToPos(data []byte, offset int) *Position {
	line, col := 1, 1
	if offset > len(data) {
		offset = len(data)
	}
	for i := 0; i < offset; i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return &Position{Line: line, Column: col}
}

// jsonQuote renders a string as its canonical JSON-quoted literal, so a string scalar never
// collides with a same-spelled number/bool/null (mirrors the YAML producer's tagStr quoting).
func jsonQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		// json.Marshal of a string cannot fail in practice; fall back to a quoted form.
		return "\"" + s + "\""
	}
	return string(b)
}

// errorsIsEOF reports whether err is io.EOF (the clean end of the JSON stream).
func errorsIsEOF(err error) bool { return err == io.EOF }
