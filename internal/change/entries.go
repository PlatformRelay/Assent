package change

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Entries reconstructs, from ONE parsed file, the whole-entry OBJECT for every
// EntryRef DiffEntries would tag a change with — keyed by the SAME EntryRef string
// (`<label>:<identity>`) DiffEntries uses. It exists so an adopter-test harness can
// bind an entry-scoped predicate (`entry.owner`) against the SAME object the
// collection-mode differ reasoned about, with NO risk of the bound entry drifting
// from the entry a change was tagged to (P5-E6-S02 Part B).
//
// SINGLE SOURCE OF TRUTH (no drift): it reuses the EXACT producer (producerFor),
// root/identity resolution (resolvePointer), and identity keying (indexByIdentity /
// entryKeyLabel plus the `label + ":" + id` ref construction) that DiffEntries uses.
// The map/list rejection rules are therefore inherited verbatim — a list with no
// declared identity, or a duplicate/non-scalar/missing identity, is rejected here
// exactly as DiffEntries rejects it (opaque), so the two never disagree on which
// entry a key names.
//
// ALL-OR-NOTHING projection (fail-safe, REQ-E6-S02-07): each entry's value subtree
// is projected to a typed `any` (json.Number / string / bool / null / nested
// map/slice) via the vnode's own tag-discriminating cmpKey — change OWNS that
// render, so this is not a second copy of any inverse. A node that cannot be
// FULLY and faithfully represented (an unprojected sequence — a YAML/HCL list left
// opaque by its producer — or an unknown scalar tag) makes the WHOLE call return a
// non-nil error, never a partial map with a field silently dropped. A caller must
// treat that error as "leave Entry nil and fall back to the scalar binding": a
// partial/empty entry map would let `has(entry.x)` return false CLEANLY (a
// more-permissive branch), whereas the scalar fallback errors -> fail-safe REVIEW.
//
// PURE and deterministic (no clock/env/network/random): the result is a map, so
// entry order is irrelevant, and each entry object is a pure function of the bytes.
//
// Document mode has no per-entry decomposition (DiffEntries delegates to Diff and
// tags no EntryRef), so it returns an EMPTY map — the caller keeps the scalar
// binding for document-mode changes.
func Entries(file string, data []byte, cfg EntryConfig) (map[string]any, error) {
	if cfg.Mode == "" || cfg.Mode == ModeDocument {
		return map[string]any{}, nil
	}

	parse := producerFor(file)
	tree, reason := parse(data)
	if reason != "" {
		return nil, fmt.Errorf("entries: %s", reason)
	}

	label := cfg.Label
	if label == "" {
		label = "entry"
	}

	out := map[string]any{}
	switch cfg.Mode {
	case ModeMap:
		coll, ok := resolvePointer(tree, cfg.Root)
		if !ok {
			return nil, fmt.Errorf("entries root %q not found", cfg.Root)
		}
		if coll.kind != vMapping {
			return nil, fmt.Errorf("entries root %q is not a mapping (map mode requires an object)", cfg.Root)
		}
		for key, node := range coll.fields {
			obj, err := projectValue(node)
			if err != nil {
				return nil, fmt.Errorf("entry %q: %w", key, err)
			}
			out[label+":"+key] = obj
		}
	case ModeList:
		if cfg.Identity == "" {
			return nil, fmt.Errorf("list mode requires an identity pointer — an unkeyed list is rejected")
		}
		coll, ok := resolvePointer(tree, cfg.Root)
		if !ok {
			return nil, fmt.Errorf("entries root %q not found", cfg.Root)
		}
		if coll.kind != vSequence {
			return nil, fmt.Errorf("entries root %q is not a sequence (list mode requires an array)", cfg.Root)
		}
		if !coll.elemsProjected {
			return nil, fmt.Errorf("entries root %q is an unprojected sequence — cannot reconstruct entries", cfg.Root)
		}
		byID, ireason := indexByIdentity(coll, cfg.Identity)
		if ireason != "" {
			return nil, fmt.Errorf("entries: %s", ireason)
		}
		for id, node := range byID {
			obj, err := projectValue(node)
			if err != nil {
				return nil, fmt.Errorf("entry %q: %w", id, err)
			}
			out[label+":"+id] = obj
		}
	default:
		return nil, fmt.Errorf("unknown entries mode %q", cfg.Mode)
	}
	return out, nil
}

// projectValue projects a canonical value-tree node into a typed Go value the
// engine's toCEL binds directly: a scalar to its typed value (json.Number / string
// / bool / nil), a mapping to a map[string]any, a sequence to a []any. It is
// ALL-OR-NOTHING: an unprojected sequence (a producer that did not project its
// elements — the fail-closed list-mode signal) or an unknown scalar tag returns a
// non-nil error rather than a partial value, so a caller never binds an incomplete
// entry (REQ-E6-S02-07).
func projectValue(n *vnode) (any, error) {
	switch n.kind {
	case vScalar:
		return projectScalar(n)
	case vMapping:
		out := make(map[string]any, len(n.fields))
		for k, child := range n.fields {
			pv, err := projectValue(child)
			if err != nil {
				return nil, err
			}
			out[k] = pv
		}
		return out, nil
	case vSequence:
		if !n.elemsProjected {
			return nil, fmt.Errorf("value contains an unprojected sequence — cannot fully reconstruct")
		}
		out := make([]any, len(n.elems))
		for i, el := range n.elems {
			pv, err := projectValue(el)
			if err != nil {
				return nil, err
			}
			out[i] = pv
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown value node kind %d", n.kind)
	}
}

// projectScalar recovers a scalar's TYPED value from its cmpKey, which every
// producer builds tag-first as "<tag>\x00<render>" (the NUL separator never appears
// in a tag). The tag is single-letter for the JSON producer (n|s|b|z — diff_json.go)
// and a YAML short tag for the YAML producer (!!int|!!float|!!str|!!bool|!!null —
// diff.go compareKey); this handles BOTH so the projection is format-neutral. Using
// the tag keeps the typed value change's OWN data (not a second copy of an inverse
// living elsewhere). A numeric literal becomes json.Number (toCEL binds it
// int64/float64, injective), a JSON-quoted string its unquoted value, a bool the
// bool (all six YAML spellings, matching evaldecode.DecodeCanonical), a null nil.
func projectScalar(n *vnode) (any, error) {
	sep := strings.IndexByte(n.cmpKey, 0x00)
	if sep < 0 {
		return nil, fmt.Errorf("malformed scalar comparison key")
	}
	switch n.cmpKey[:sep] {
	case "n", tagInt, tagFloat: // JSON number | YAML !!int/!!float -> raw literal
		return json.Number(n.render), nil
	case "s", tagStr: // JSON string | YAML !!str -> render is JSON-quoted
		var s string
		if err := json.Unmarshal([]byte(n.render), &s); err != nil {
			return nil, fmt.Errorf("undecodable string scalar %q", n.render)
		}
		return s, nil
	case "b", tagBool: // JSON bool (true/false) | YAML !!bool (six core spellings)
		switch n.render {
		case "true", "True", "TRUE":
			return true, nil
		case "false", "False", "FALSE":
			return false, nil
		default:
			return nil, fmt.Errorf("unrecognized bool literal %q", n.render)
		}
	case "z", tagNull: // JSON null | YAML !!null
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown scalar tag in comparison key")
	}
}
