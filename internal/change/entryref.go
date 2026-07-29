package change

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// EntryMode selects how DiffEntries derives collection entries (E1-S05, ADR-0017 §5).
type EntryMode string

const (
	// ModeDocument is the default: the shipped single-root-mapping walk (delegates to Diff), so a
	// document-mode DiffEntries is byte-identical to Diff (REQ-E1-S05-04).
	ModeDocument EntryMode = "document"
	// ModeMap treats a mapping's keys as entries, each keyed by its own map key.
	ModeMap EntryMode = "map"
	// ModeList treats a sequence's elements as entries, each keyed by the value at Identity.
	ModeList EntryMode = "list"
)

// EntryConfig configures collection-mode entry derivation over one file (ADR-0017 §5 `entries:`).
type EntryConfig struct {
	Mode     EntryMode // document (default) | map | list
	Root     string    // RFC-6901 pointer to the collection ("" = document root)
	Identity string    // list mode ONLY: pointer within each element to its identity value
	Label    string    // EntryRef kind label (e.g. "workload", "service"); defaults to "entry"
}

// DiffEntries diffs one file in a collection mode, tagging each change with a stable, identity-
// derived EntryRef (E1-S05). It is a thin entry-point over the shared value tree and the
// document-mode walker (walkNode): document mode delegates verbatim to Diff; map and list mode
// derive entries and walk each entry's subtree with the same walker, so the fail-safe guards
// (opaque on a nested collection, a type flip, an unrenderable scalar) are inherited unchanged.
//
// PURE and deterministic: entries are processed in sorted key/identity order and the result is
// canonically sorted, so the output is byte-stable regardless of source order (REQ-E1-S05-06).
// A list with no declared Identity, or a duplicate/non-scalar identity, is REJECTED (opaque) —
// never silently index-keyed or first-wins (REQ-E1-S05-03).
func DiffEntries(file string, base, head []byte, cfg EntryConfig) (ChangeSet, error) {
	if cfg.Mode == "" || cfg.Mode == ModeDocument {
		return Diff(file, base, head) // byte-identical to the shipped document walk (REQ-05-04)
	}

	parse := producerFor(file)
	baseTree, reason := parse(base)
	if reason != "" {
		return opaque("base: " + reason)
	}
	headTree, reason := parse(head)
	if reason != "" {
		return opaque("head: " + reason)
	}

	label := cfg.Label
	if label == "" {
		label = "entry"
	}

	var changes []Change
	var derr string
	switch cfg.Mode {
	case ModeMap:
		derr = diffMapEntries(file, cfg, label, baseTree, headTree, &changes)
	case ModeList:
		derr = diffListEntries(file, cfg, label, baseTree, headTree, &changes)
	default:
		return opaque("unknown entries mode " + string(cfg.Mode))
	}
	if derr != "" {
		return opaque(derr)
	}
	sortChanges(changes)
	return ChangeSet{Changes: changes}, nil
}

// diffMapEntries derives entries from a mapping's keys (each key is an entry identity). A key
// present on both sides has its value subtree walked; a one-sided key is a whole-entry add/delete.
func diffMapEntries(file string, cfg EntryConfig, label string, baseTree, headTree *vnode, out *[]Change) string {
	baseColl, ok := resolvePointer(baseTree, cfg.Root)
	if !ok {
		return fmt.Sprintf("entries root %q not found in base", cfg.Root)
	}
	headColl, ok := resolvePointer(headTree, cfg.Root)
	if !ok {
		return fmt.Sprintf("entries root %q not found in head", cfg.Root)
	}
	if baseColl.kind != vMapping || headColl.kind != vMapping {
		return fmt.Sprintf("entries root %q is not a mapping (map mode requires an object)", cfg.Root)
	}

	for _, key := range unionKeys(baseColl.fields, headColl.fields) {
		bv, inBase := baseColl.fields[key]
		hv, inHead := headColl.fields[key]
		entryPtr := cfg.Root + "/" + escapePointer(key)
		ref := label + ":" + key
		if reason := diffOneEntry(file, entryPtr, ref, bv, inBase, hv, inHead, out); reason != "" {
			return reason
		}
	}
	return ""
}

// diffListEntries derives entries from a sequence's elements, keyed by the scalar value at
// Identity within each element. Matching is by IDENTITY, never by index — so a reorder with no
// content change yields zero changes (REQ-E1-S05-02). An absent Identity declaration, or a
// missing/non-scalar/duplicate identity value, is rejected (REQ-E1-S05-03).
func diffListEntries(file string, cfg EntryConfig, label string, baseTree, headTree *vnode, out *[]Change) string {
	if cfg.Identity == "" {
		return "list mode requires an identity pointer — an unkeyed list is rejected (never index-keyed)"
	}
	baseColl, ok := resolvePointer(baseTree, cfg.Root)
	if !ok {
		return fmt.Sprintf("entries root %q not found in base", cfg.Root)
	}
	headColl, ok := resolvePointer(headTree, cfg.Root)
	if !ok {
		return fmt.Sprintf("entries root %q not found in head", cfg.Root)
	}
	if baseColl.kind != vSequence || headColl.kind != vSequence {
		return fmt.Sprintf("entries root %q is not a sequence (list mode requires an array)", cfg.Root)
	}
	// Fail CLOSED when the sequence's elements were not projected by the producer (YAML/HCL leave
	// sequences as opaque leaves). Without this a non-empty unprojected list would range zero
	// elements and look "unchanged" — a silent miss. An empty PROJECTED list (JSON `[]`) is fine:
	// elemsProjected distinguishes it from an unprojected one.
	if !baseColl.elemsProjected || !headColl.elemsProjected {
		return fmt.Sprintf("entries root %q is a sequence this format does not project for list mode — rejected fail-closed (list mode currently supports JSON arrays)", cfg.Root)
	}

	baseByID, reason := indexByIdentity(baseColl, cfg.Identity)
	if reason != "" {
		return reason
	}
	headByID, reason := indexByIdentity(headColl, cfg.Identity)
	if reason != "" {
		return reason
	}

	for _, id := range unionKeys(baseByID, headByID) {
		be, inBase := baseByID[id]
		he, inHead := headByID[id]
		entryPtr := cfg.Root + "/" + escapePointer(id)
		ref := label + ":" + id
		if reason := diffOneEntry(file, entryPtr, ref, be, inBase, he, inHead, out); reason != "" {
			return reason
		}
	}
	return ""
}

// diffOneEntry diffs one entry (present on either or both sides) and tags every resulting change
// with the entry's EntryRef. A both-sides entry has its subtree walked by the shared walker (so a
// nested collection inside it is opaque, REQ-E1-S05-05); a one-sided entry is a whole-entry
// add/delete reported as a single structural Change at the entry pointer.
func diffOneEntry(file, entryPtr, ref string, bv *vnode, inBase bool, hv *vnode, inHead bool, out *[]Change) string {
	var local []Change
	switch {
	case inBase && !inHead:
		local = append(local, Change{File: file, Path: entryPtr, Kind: KindDelete, OldPos: bv.pos})
	case !inBase && inHead:
		local = append(local, Change{File: file, Path: entryPtr, Kind: KindAdd, NewPos: hv.pos})
	default:
		if reason := walkNode(file, entryPtr, bv, hv, &local); reason != "" {
			return reason
		}
	}
	for i := range local {
		local[i].EntryRef = ref
	}
	*out = append(*out, local...)
	return ""
}

// indexByIdentity builds a map from each element's identity value (cleaned to a display string) to
// the element node. It rejects (non-empty reason) an element that is not an object, an element
// missing the identity pointer, a non-scalar identity, or a duplicate identity across elements —
// never falling back to index-based identity or first-wins (REQ-E1-S05-03).
func indexByIdentity(seq *vnode, identityPtr string) (map[string]*vnode, string) {
	out := make(map[string]*vnode, len(seq.elems))
	for _, el := range seq.elems {
		if el.kind != vMapping {
			return nil, "list entry is not an object — cannot derive a stable identity"
		}
		idNode, ok := resolvePointer(el, identityPtr)
		if !ok {
			return nil, fmt.Sprintf("list entry missing identity pointer %q — unkeyed, rejected (never index-keyed)", identityPtr)
		}
		if idNode.kind != vScalar {
			return nil, fmt.Sprintf("list entry identity at %q is not a scalar — cannot key", identityPtr)
		}
		id := entryKeyLabel(idNode.render)
		if _, dup := out[id]; dup {
			return nil, fmt.Sprintf("duplicate identity %q across list entries — rejected (never first-wins)", id)
		}
		out[id] = el
	}
	return out, ""
}

// resolvePointer walks an RFC-6901 pointer through mapping fields to the target node. An empty
// pointer is the node itself. A step into a non-mapping, or a missing key, returns (nil, false).
func resolvePointer(node *vnode, pointer string) (*vnode, bool) {
	if pointer == "" || pointer == "/" {
		return node, true
	}
	cur := node
	for _, tok := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		if cur.kind != vMapping {
			return nil, false
		}
		next, ok := cur.fields[unescapePointer(tok)]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// unionKeys returns the sorted union of two maps' keys (deterministic entry order).
func unionKeys(a, b map[string]*vnode) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	keys := make([]string, 0, len(a)+len(b))
	for k := range a {
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := seen[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// entryKeyLabel produces the human identity string for an EntryRef: a JSON-quoted string render
// (e.g. `"orders-api"`) is unquoted to `orders-api`; any other render is used as-is. Used for both
// matching and display, so two elements whose identities collapse to the same label are a
// duplicate-identity rejection (fail-safe) rather than a silent collision.
func entryKeyLabel(render string) string {
	if len(render) >= 2 && render[0] == '"' {
		var s string
		if err := json.Unmarshal([]byte(render), &s); err == nil {
			return s
		}
	}
	return render
}

// unescapePointer reverses escapePointer (RFC 6901: ~1 -> /, ~0 -> ~; ~1 first).
func unescapePointer(token string) string {
	token = strings.ReplaceAll(token, "~1", "/")
	token = strings.ReplaceAll(token, "~0", "~")
	return token
}
