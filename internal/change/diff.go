// Package change implements assent's canonical change model: it diffs a base and head
// version of a single file into a byte-stable ChangeSet that predicates evaluate over
// (ADR-0003, ADR-0011).
//
// This slice (E1-S03, extending E1-S01/S02) diffs one document into modify, add, delete, and
// (opt-in) rename Changes, each carrying a source position (ADR-0003 Amendment 2). The differ
// no longer walks a format-specific parse tree directly: it first projects the parsed document
// into a format-NEUTRAL value tree (vnode) and then walks that tree, so a YAML producer and a
// JSON producer (diff_json.go) feed the SAME comparison logic and yield the SAME ChangeSet
// shape. Diff selects the producer by file extension (.json -> JSON, everything else -> YAML).
// The package is PURE by construction — it reads no clock, randomness, environment, or network,
// so its output is a deterministic function of its inputs (ADR-0011 invariant, GUIDELINES §5).
//
// Fail-safe direction (GUIDELINES §2, ADR-0015 §9): any input this differ cannot decide —
// unparseable bytes, dangerous alias expansion, a type flip, or a one-sided (added/deleted)
// key whose value is a MAPPING or SEQUENCE rather than a scalar — yields an OPAQUE ChangeSet,
// never a silent empty diff. Add/delete are first-class for a one-sided key with a SCALAR
// value (or a scalar leaf reached by recursing into a two-sided mapping); a one-sided key
// whose value is itself a collection is deferred to the collection-mode EntryRef derivation
// (E1-S05), and stays opaque here rather than risk a silent under-report of a whole added or
// removed subtree. A SEQUENCE value (a vSequence node) likewise stays opaque — list walking is
// E1-S05 territory — so the value tree carries a sequence kind but this slice never diffs into it.
//
// INJECTIVE VALUE COMPARISON (the property that closes the fail-open class): the differ never
// decodes a scalar into a Go-coerced value (a lossy step — e.g. two distinct integers >= 2^64
// both collapse to the same float64, and two distinct high-precision floats collapse likewise,
// silently MISSING a real change). Instead each scalar vnode carries a human RENDER (emitted as
// Old/New) and a TAG-QUALIFIED comparison key built from the RAW literal, and two scalars are
// equal iff their comparison keys match. The YAML producer builds these from *yaml.Node's tag +
// raw literal (render/compareKey); the JSON producer builds them from json.Number raw literals
// and JSON-quoted strings (never float64). A Change is emitted iff the comparison keys differ,
// so the differ can only ever OVER-report a change, never UNDER-report one. Over-reporting is
// the safe direction; a silent miss is the sole forbidden outcome. (This is why numeric
// cross-type equivalence such as int 1 == float 1.0 is intentionally NOT collapsed, and why
// big.Int re-parsing was rejected — see render.)
package change

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind is the classification of a single change (ADR-0011). This slice emits KindModify,
// KindAdd, and KindDelete; KindRename is produced only by the opt-in fold (rename.go, E1-S02).
type Kind string

const (
	// KindModify marks a scalar whose value changed between base and head.
	KindModify Kind = "modify"
	// KindAdd marks a scalar value present only in head (E1-S01).
	KindAdd Kind = "add"
	// KindDelete marks a scalar value present only in base (E1-S01).
	KindDelete Kind = "delete"
	// KindRename marks a delete+add pair with an IDENTICAL value that an opt-in fold collapsed
	// into one move (E1-S02). Its Path is the new (head) path and OldPath the old (base) path;
	// Old == New is the unchanged shared value. Only emitted under RenameDetect (never by default).
	KindRename Kind = "rename"
)

// Position is a 1-indexed source location (line/column) within a file's byte stream. It anchors
// a Change to the exact spot a value lives so a forge adapter can post an inline comment at the
// right line (ADR-0003 Amendment 2, "first-class positions"). A Change carries a position on each
// side where a value EXISTS: both sides for a modify, the head side only for an add, the base
// side only for a delete. YAML positions come from the parser; JSON positions are derived from
// each value's byte offset (see diff_json.go).
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Change is one field-level delta within a file, shaped after ADR-0011's Change.
// Classes/Environment are populated later by the classifier (ADR-0008) and are empty here.
type Change struct {
	File string `json:"file"`
	Path string `json:"path"` // RFC-6901 JSON pointer within the file (the NEW path for a rename)
	Kind Kind   `json:"kind"`

	// OldPath is set ONLY on a KindRename: the base-side (old) pointer the value moved FROM.
	// Path holds the head-side (new) pointer. Empty/omitted for add/delete/modify.
	OldPath string `json:"oldPath,omitempty"`

	// Old and New are the CANONICAL, TAG-DISCRIMINATING string forms of the base and head
	// scalar values, produced by the producer's render step: !!int/!!float render as their RAW
	// literal ("12", "18446744073709551617"), !!bool as its raw literal ("true"), !!null as
	// "null", and !!str values are JSON-QUOTED (the string "12" renders as "\"12\"", not "12").
	// Quoting keeps a bool/number/null distinct from a same-spelled string, and using the raw
	// literal (never a Go-coerced number) keeps distinct numeric literals distinct — no lossy
	// collapse. The JSON producer follows the same discipline (json.Number literals, JSON-quoted
	// strings) so a JSON edit yields the same Old/New a YAML edit would.
	//
	// CONTRACT for downstream consumers (S03 aggregation / CEL asserts): these diverge from
	// ADR-0011's typed `Value` — they are strings, not typed scalars, and numeric ones are the
	// raw literal (so `016` stays "016", not "14" or "16"). Any NUMERIC or boolean comparison
	// (e.g. `new > 12`) MUST coerce Old/New at the CEL boundary; a naive string comparison is
	// lexical and WRONG (lexically `"9" > "12"` is true). S03 owns that coercion; this slice only
	// guarantees a byte-stable, injective canonical string.
	Old string `json:"old"`
	New string `json:"new"`

	// OldPos / NewPos are the source positions of the base and head values. A pointer so the
	// absent side of an add/delete is nil (omitted from JSON), not a misleading {0,0}: NewPos is
	// nil on a delete, OldPos is nil on an add, both are set on a modify.
	OldPos *Position `json:"oldPos,omitempty"`
	NewPos *Position `json:"newPos,omitempty"`

	// EntryRef is the stable, identity-derived subject of a collection entry this change belongs
	// to (E1-S05), e.g. "workload:orders-api" or "service:orders-api". It is set only by the
	// collection-mode differ (DiffEntries, map/list mode); document-mode changes carry none. It
	// lets a rule or a forge comment refer to the same entry across a reorder (ADR-0017 §5).
	EntryRef string `json:"entryRef,omitempty"`

	Classes     []string `json:"classes,omitempty"`
	Environment string   `json:"environment,omitempty"`
}

// ChangeSet is the canonical, order-stable set of changes for one file (ADR-0011).
//
// Opaque is an IN-BAND fail-safe signal: when true the differ could not decide the diff and
// the caller MUST map the result to REVIEW. It lives on the value (not only in a returned
// error) precisely so a caller that checks only len(Changes)==0 cannot mistake an
// undecidable input for "nothing changed -> safe" (GUIDELINES §2).
type ChangeSet struct { //nolint:revive // ChangeSet is the ADR-0011-pinned public contract name consumed by S03/S04
	Changes      []Change `json:"changes,omitempty"`
	Opaque       bool     `json:"opaque"`
	OpaqueReason string   `json:"opaqueReason,omitempty"`
}

// ErrOpaque accompanies every opaque ChangeSet so error-checking callers also fail closed.
var ErrOpaque = errors.New("change: opaque input, cannot decide diff")

// opaque builds the fail-safe result: an empty change list, the Opaque flag set, a reason,
// and a wrapped ErrOpaque. It deliberately carries NO partial changes.
func opaque(reason string) (ChangeSet, error) {
	return ChangeSet{Opaque: true, OpaqueReason: reason}, fmt.Errorf("%w: %s", ErrOpaque, reason)
}

// Diff computes the ChangeSet between the base and head bytes of one file. It selects a value-tree
// producer by file extension (.json -> JSON, .yaml/.yml and everything else -> YAML), projects
// each side into a format-neutral value tree, and walks the two trees.
//
// It is a pure function: identical inputs always produce byte-identical output. On any input it
// cannot decide (unparseable bytes, alias/anchor/merge expansion, a structural delta beyond a
// scalar modify or scalar add/delete, a non-string-keyed / duplicate-keyed mapping, or a
// non-object root) it returns an opaque ChangeSet plus a wrapped ErrOpaque — never a silent
// empty diff.
func Diff(file string, base, head []byte) (ChangeSet, error) {
	parse := producerFor(file)

	baseTree, reason := parse(base)
	if reason != "" {
		return opaque("base: " + reason)
	}
	headTree, reason := parse(head)
	if reason != "" {
		return opaque("head: " + reason)
	}

	var changes []Change
	if reason := walkNode(file, "", baseTree, headTree, &changes); reason != "" {
		return opaque(reason)
	}
	sortChanges(changes)
	return ChangeSet{Changes: changes}, nil
}

// treeProducer parses one file's bytes into the canonical value tree, returning a non-empty
// reason (routed to opaque) on any input it cannot faithfully represent.
type treeProducer func(data []byte) (*vnode, string)

// producerFor selects the value-tree producer by file extension. `.json` routes to the JSON
// producer (E1-S03); `.yaml`/`.yml` and every other extension route to the YAML producer (the
// default is deliberately YAML so an unknown extension keeps the pre-S03 behaviour).
func producerFor(file string) treeProducer {
	switch {
	case strings.HasSuffix(file, ".json"):
		return parseJSON
	case strings.HasSuffix(file, ".tfvars"):
		return parseHCL
	default:
		return parseYAML
	}
}

// sortChanges orders changes canonically by (File, Path) so a ChangeSet is byte-stable
// regardless of map iteration order. Shared by Diff and the rename fold (E1-S02).
func sortChanges(changes []Change) {
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].File != changes[j].File {
			return changes[i].File < changes[j].File
		}
		return changes[i].Path < changes[j].Path
	})
}

// vkind is the classification of a value-tree node. This slice compares vScalar and recurses
// into vMapping; a vSequence is carried in the tree (so the model is complete) but stays opaque
// when reached — per-element list walking is E1-S05 territory.
type vkind int

const (
	vScalar vkind = iota
	vMapping
	vSequence
)

// vnode is one node of the canonical, format-NEUTRAL value tree the walker compares. A YAML or
// JSON producer projects its parsed document into this shape so the comparison logic is written
// once, independent of source format (ADR-0011 canonical change model).
//
//   - vScalar carries `render` (the injective human string emitted as Old/New) and `cmpKey`
//     (a tag-qualified comparison key, never emitted — two scalars are equal iff cmpKey matches).
//   - vMapping carries `fields`, a string-keyed child map (the only mapping shape a field-level
//     differ can walk; non-string keys make the producer fail closed).
//   - vSequence is a leaf marker in this slice (no elements are projected): any sequence reached
//     during the walk is opaque, deferred to E1-S05's list/EntryRef mode.
//
// `pos` is the 1-indexed source position of the value; it is emitted only for scalars (mappings
// recurse and sequences are opaque, so their positions are never surfaced).
type vnode struct {
	kind   vkind
	render string
	cmpKey string
	fields map[string]*vnode
	// elems holds a sequence's element nodes. It is populated by producers that support
	// collection-mode list walking (the JSON producer, for E1-S05's `list` mode) and left nil by
	// producers/paths that do not. The document-mode walker (walkNode) treats every vSequence as
	// opaque REGARDLESS of elems, so populating this field is additive and never moves a
	// document-mode golden — only DiffEntries `list` mode reads it (E1-S05).
	elems []*vnode
	// elemsProjected marks a vSequence whose elements WERE projected into elems by the producer
	// (true for JSON, false for YAML/HCL which leave sequences as opaque leaves). It is the signal
	// that distinguishes a genuinely EMPTY projected sequence (elems nil, elemsProjected true) from
	// an UNPROJECTED one (elems nil, elemsProjected false): list mode fails CLOSED on the latter
	// rather than silently reporting zero entries — the fail-open E1-S05's review caught.
	elemsProjected bool
	pos            *Position
}

// walkNode compares two value-tree nodes at pointer, appending a KindModify change when two
// scalars have different comparison keys. It recurses into two mappings; any other shape — a
// sequence, a mapping-vs-scalar type flip — returns a non-empty reason (-> opaque), never a
// silent drop. This is the format-neutral generalization of the pre-S03 *yaml.Node walk: the
// guards (alias/anchor, unrenderable scalar, non-string key, duplicate key) now run when the
// producer BUILDS the tree, so a bad node cannot reach here.
func walkNode(file, pointer string, base, head *vnode, out *[]Change) string {
	baseIsMap := base.kind == vMapping
	headIsMap := head.kind == vMapping
	switch {
	case baseIsMap && headIsMap:
		return walkMapping(file, pointer, base, head, out)
	case baseIsMap != headIsMap:
		return fmt.Sprintf("type flip at %q (map vs non-map) — structural delta is out of modify-only scope", pointerOrRoot(pointer))
	}

	// Both non-mappings: only two scalars are decidable. A sequence (vSequence) on either side
	// is opaque — list walking is E1-S05 territory — exactly as the pre-S03 differ treated a
	// sequence value.
	if base.kind != vScalar || head.kind != vScalar {
		return fmt.Sprintf("non-scalar value at %q (sequence or list) — not decidable in modify-only slice (E1-S05)", pointerOrRoot(pointer))
	}

	// A Change is emitted iff the two scalars differ by their TAG-QUALIFIED comparison key.
	// Comparing on cmpKey — not render — is what makes the equality INJECTIVE over (tag, literal):
	// two scalars are equal iff they share BOTH the resolved tag AND the canonical render. The
	// EMITTED Old/New stay the human render, so goldens are unchanged — only the comparator is
	// tag-qualified. The render never coerces a numeric literal through a lossy Go type, so
	// distinct literals also never collapse: the comparison can over-report but never silently miss.
	if base.cmpKey != head.cmpKey {
		*out = append(*out, Change{
			File: file, Path: pointer, Kind: KindModify,
			Old: base.render, New: head.render,
			OldPos: base.pos, NewPos: head.pos,
		})
	}
	return ""
}

// walkMapping compares two mapping nodes key-by-key over the union of their keys (sorted for
// determinism). A key present on only one side is a first-class add (head-only) or delete
// (base-only) when its value is a scalar (emitOneSided); a one-sided key whose value is a
// collection stays opaque (deferred to E1-S05).
func walkMapping(file, pointer string, base, head *vnode, out *[]Change) string {
	seen := make(map[string]struct{}, len(base.fields)+len(head.fields))
	keys := make([]string, 0, len(base.fields)+len(head.fields))
	for k := range base.fields {
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	for k := range head.fields {
		if _, ok := seen[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		bv, inBase := base.fields[k]
		hv, inHead := head.fields[k]
		child := pointer + "/" + escapePointer(k)
		switch {
		case inBase && !inHead:
			if reason := emitOneSided(file, child, bv, KindDelete, out); reason != "" {
				return reason
			}
		case !inBase && inHead:
			if reason := emitOneSided(file, child, hv, KindAdd, out); reason != "" {
				return reason
			}
		default:
			if reason := walkNode(file, child, bv, hv, out); reason != "" {
				return reason
			}
		}
	}
	return ""
}

// emitOneSided emits an add (side == head, kind KindAdd) or a delete (side == base, kind
// KindDelete) for a value present on exactly one side of a mapping. Only a SCALAR value is
// decidable here: it yields one Change carrying the rendered value and a position on the present
// side. A MAPPING or SEQUENCE value (a whole added/removed collection subtree) returns a
// non-empty reason so the diff fails closed to opaque — collection add/delete is deferred
// UNIFORMLY to E1-S05's EntryRef derivation.
func emitOneSided(file, pointer string, node *vnode, kind Kind, out *[]Change) string {
	if node.kind != vScalar {
		return fmt.Sprintf(
			"one-sided %s of a non-scalar value at %q — collection add/delete is out of scope (E1-S05)",
			kind, pointerOrRoot(pointer),
		)
	}
	ch := Change{File: file, Path: pointer, Kind: kind}
	if kind == KindAdd {
		ch.New, ch.NewPos = node.render, node.pos
	} else {
		ch.Old, ch.OldPos = node.render, node.pos
	}
	*out = append(*out, ch)
	return ""
}

// ---- YAML producer -------------------------------------------------------------------------
//
// The YAML producer projects a parsed *yaml.Node document into the canonical value tree, applying
// every fail-safe guard at BUILD time (the checks the pre-S03 differ ran lazily during the walk):
// a non-string / duplicate / alias-anchor-merge KEY, an alias/anchor VALUE node (below the root),
// and an unrenderable/unknown-tag scalar each make the producer fail closed. The ROOT mapping's
// OWN anchor is deliberately NOT rejected (an unused root anchor still diffs its children).

// YAML node kinds (gopkg.in/yaml.v3 does not export named constants for these).
const (
	kindDocument = yaml.DocumentNode
	kindMapping  = yaml.MappingNode
	kindSequence = yaml.SequenceNode
	kindScalar   = yaml.ScalarNode
	kindAlias    = yaml.AliasNode
)

// Well-known YAML resolved tags used for scalar comparison.
const (
	tagStr   = "!!str"
	tagInt   = "!!int"
	tagFloat = "!!float"
	tagBool  = "!!bool"
	tagNull  = "!!null"
	tagMerge = "!!merge" // the `<<` merge key
)

// parseYAML is the YAML treeProducer: it gates the document down to a single string-keyed-mapping
// root (parseSingleMappingDoc) and then projects it into the value tree (yamlMapping).
func parseYAML(data []byte) (*vnode, string) {
	root, err := parseSingleMappingDoc(data)
	if err != nil {
		return nil, err.Error()
	}
	return yamlMapping("", root)
}

// parseSingleMappingDoc is the single CONFIDENCE GATE for the YAML producer. It returns a
// non-error result ONLY when data is EXACTLY ONE fully-understood YAML document whose ROOT is a
// string-keyed mapping — the only shape a field-level differ can meaningfully diff. Everything
// else fails CLOSED (returns an error -> the caller emits opaque):
//
//   - zero documents (empty input, only comments, or a bare "---" with nothing after it)
//   - more than one document in the stream (`---`-separated) — decoding only the first would
//     silently drop changes confined to later documents (fail-OPEN)
//   - a root that is not a mapping (a bare scalar `42`, a bare sequence `- a`, or an explicit
//     `null`/`~` document) — not a field-diffable shape
//   - any yaml.v3 decode error (malformed bytes)
//
// It returns the DocumentNode's single content node (the mapping root). Deeper fail-safe checks
// — non-string keys, duplicate keys, aliases, anchors, and merge keys, sequences, dangerous alias
// expansion — are enforced by yamlMapping/yamlValue when the tree is built, because decoding into
// *yaml.Node (unlike decoding into `any`) does NOT itself reject them: aliases are recorded
// unexpanded and duplicate keys are kept, so this producer must re-assert those guards explicitly.
func parseSingleMappingDoc(data []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))

	var first yaml.Node
	if err := dec.Decode(&first); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("no YAML document present (empty, comments-only, or zero documents)")
		}
		return nil, fmt.Errorf("does not decode as YAML: %w", err)
	}

	// Reject a stream carrying a second document. A non-EOF error here is also a decode
	// failure and must fail closed.
	var second yaml.Node
	if err := dec.Decode(&second); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("trailing YAML content did not decode cleanly: %w", err)
		}
		return nil, fmt.Errorf("multi-document YAML stream (more than one document) — not decidable in modify-only slice")
	}

	if first.Kind != kindDocument || len(first.Content) != 1 {
		return nil, fmt.Errorf("expected exactly one YAML document node, got kind %d with %d content nodes", first.Kind, len(first.Content))
	}
	root := first.Content[0]
	if root.Kind != kindMapping {
		return nil, fmt.Errorf("document root is not a mapping (kind %d, tag %q) — not a field-diffable shape", root.Kind, root.ShortTag())
	}
	return root, nil
}

// yamlMapping projects a YAML mapping node into a vMapping node. It applies the key guards
// (non-string, duplicate, alias/anchor/merge) and builds each value via yamlValue. It does NOT
// check the mapping node's OWN anchor: at the root that is intentional (an unused root anchor
// still diffs its children), and for a nested mapping the anchor was already checked by the
// yamlValue that reached it.
func yamlMapping(pointer string, m *yaml.Node) (*vnode, string) {
	fields := make(map[string]*vnode, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		v := m.Content[i+1]
		if k.Kind == kindAlias || k.Anchor != "" || k.ShortTag() == tagMerge {
			return nil, fmt.Sprintf("alias/anchor/merge key in mapping at %q — not decidable in modify-only slice", pointerOrRoot(pointer))
		}
		if k.Kind != kindScalar || k.ShortTag() != tagStr {
			return nil, fmt.Sprintf("non-string mapping key (kind %d, tag %q) at %q — not decidable in modify-only slice", k.Kind, k.ShortTag(), pointerOrRoot(pointer))
		}
		if _, dup := fields[k.Value]; dup {
			return nil, fmt.Sprintf("duplicate mapping key %q at %q — not decidable in modify-only slice", k.Value, pointerOrRoot(pointer))
		}
		child := pointer + "/" + escapePointer(k.Value)
		vn, reason := yamlValue(child, v)
		if reason != "" {
			return nil, reason
		}
		fields[k.Value] = vn
	}
	return &vnode{kind: vMapping, fields: fields, pos: posOf(m)}, ""
}

// yamlValue projects a single YAML value node into a vnode. An alias node or a node carrying an
// anchor is opaque (expanding an alias re-arms alias bombs; an anchored value could be aliased
// elsewhere) — this is the guard the pre-S03 walk ran on every value node BELOW the root. A
// mapping recurses; a sequence becomes a vSequence leaf (opaque when walked); a scalar renders
// via the injective render/compareKey (unrenderable/unknown-tag scalars fail closed).
func yamlValue(pointer string, n *yaml.Node) (*vnode, string) {
	if n.Kind == kindAlias || n.Anchor != "" {
		return nil, fmt.Sprintf("alias/anchor node at %q — not decidable in modify-only slice", pointerOrRoot(pointer))
	}
	switch n.Kind {
	case kindMapping:
		return yamlMapping(pointer, n)
	case kindSequence:
		return &vnode{kind: vSequence, pos: posOf(n)}, ""
	case kindScalar:
		r, ok := render(n)
		if !ok {
			return nil, fmt.Sprintf("unrenderable scalar at %q (kind %d tag %q) — not decidable", pointerOrRoot(pointer), n.Kind, n.ShortTag())
		}
		return &vnode{kind: vScalar, render: r, cmpKey: compareKey(n, r), pos: posOf(n)}, ""
	default:
		return nil, fmt.Sprintf("unsupported YAML node kind %d at %q — not decidable", n.Kind, pointerOrRoot(pointer))
	}
}

// posOf reads a YAML node's 1-indexed source position.
func posOf(n *yaml.Node) *Position {
	return &Position{Line: n.Line, Column: n.Column}
}

// compareKey qualifies a scalar's human render with its resolved tag so the equality test is
// injective over (tag, literal). The NUL separator cannot appear in a YAML resolved short tag,
// so no (tag, render) pair can alias another. Used ONLY for equality — never emitted.
func compareKey(n *yaml.Node, humanRender string) string {
	return n.ShortTag() + "\x00" + humanRender
}

// render produces the canonical, INJECTIVE string form of a scalar node from its resolved tag
// and RAW literal value — never through a lossy numeric Go type. The second return is false for
// anything that is not a renderable scalar (a mapping, sequence, alias, or unknown/custom tag),
// which the caller routes to opaque.
//
// Design note (why no math/big / no numeric normalization): comparing by the raw literal is
// injective and cannot under-report. Two distinct integers >= 2^64 (which yaml.v3 resolves to
// !!float) and two distinct high-precision floats keep their distinct literals and so are
// detected as changes — the exact fail-open class this closes. big.Int canonicalization of
// !!int was deliberately REJECTED: (a) it does not even catch the headline overflow case, which
// is tagged !!float, and (b) re-parsing an int literal risks disagreeing with YAML's own
// resolver (e.g. `016` is decimal 16 in YAML 1.2 core but octal 14 to Go's base-0 parser),
// which would re-introduce a silent miss. Per the fail-safe principle, `0x10` vs `16` and `1`
// vs `01` are reported as changes (safe over-report), not collapsed.
func render(n *yaml.Node) (string, bool) {
	if n.Kind != kindScalar {
		return "", false
	}
	switch n.ShortTag() {
	case tagNull:
		// Constant so `~` and `null` (both null) never report a spurious change.
		return "null", true
	case tagStr:
		// JSON-quote so a string scalar never collides with a bool/number/null spelling.
		b, err := json.Marshal(n.Value)
		if err != nil {
			return "", false
		}
		return string(b), true
	case tagInt, tagFloat, tagBool:
		// Raw literal: injective, no lossy coercion.
		return n.Value, true
	default:
		// !!timestamp, !!binary, and any unknown/custom tag: not understood by this thin
		// slice -> opaque rather than risk a mis-comparison.
		return "", false
	}
}

// escapePointer applies RFC 6901 token escaping (~ -> ~0, / -> ~1).
func escapePointer(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return token
}

func pointerOrRoot(pointer string) string {
	if pointer == "" {
		return "/"
	}
	return pointer
}
