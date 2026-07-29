// Package change implements assent's canonical change model: it diffs a base and head
// version of a single file into a byte-stable ChangeSet that predicates evaluate over
// (ADR-0003, ADR-0011).
//
// This slice (E1-S01, extending P4-E1-S02) diffs one YAML document into modify, add, and
// delete Changes, each carrying a source position (ADR-0003 Amendment 2). Rename folding
// (E1-S02) and multi-format adapters (E1-S03/S04) are later stories. The package is PURE by
// construction — it reads no clock, randomness, environment, or network, so its output is a
// deterministic function of its inputs (ADR-0011 invariant, GUIDELINES §5).
//
// Fail-safe direction (GUIDELINES §2, ADR-0015 §9): any input this differ cannot decide —
// unparseable bytes, dangerous alias expansion, a type flip, or a one-sided (added/deleted)
// key whose value is a MAPPING or SEQUENCE rather than a scalar — yields an OPAQUE ChangeSet,
// never a silent empty diff. Add/delete are first-class for a one-sided key with a SCALAR
// value (or a scalar leaf reached by recursing into a two-sided mapping); a one-sided key
// whose value is itself a collection is deferred to the value-tree era (E1-S03) and the
// collection-mode EntryRef derivation (E1-S05), and stays opaque here rather than risk a
// silent under-report of a whole added/removed subtree. This narrows what fails closed (a
// scalar add/delete is now decidable); it does not license under-reporting a real change.
//
// INJECTIVE VALUE COMPARISON (the property that closes the fail-open class): the differ never
// decodes YAML scalars into Go-coerced values (a lossy step — e.g. two distinct integers >= 2^64
// both collapse to the same float64, and two distinct high-precision floats collapse likewise,
// silently MISSING a real change). Instead it decodes into *yaml.Node, which retains each
// scalar's TAG and RAW literal, and compares scalars by a TAG-QUALIFIED key (resolved tag +
// canonical render — see compareKey). Two scalars are equal iff they share BOTH the tag AND the
// render, so the equality is injective over (tag, literal): distinct literals never collapse
// (no lossy Go coercion), and neither do distinct-typed scalars sharing a literal (an explicit
// `!!int 1` vs `!!float 1`). A Change is emitted iff the tag-qualified keys differ; the differ
// can therefore only ever OVER-report a change, never UNDER-report one. Over-reporting is the
// safe direction; a silent miss is the sole forbidden outcome. (This is why numeric cross-type
// equivalence such as int 1 == float 1.0 is intentionally NOT collapsed, and why big.Int
// re-parsing was rejected — see render.)
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

// Kind is the classification of a single change (ADR-0011). This slice (E1-S01) emits
// KindModify, KindAdd, and KindDelete. Rename folding (KindRename) is E1-S02.
type Kind string

const (
	// KindModify marks a scalar whose value changed between base and head.
	KindModify Kind = "modify"
	// KindAdd marks a scalar value present only in head (E1-S01).
	KindAdd Kind = "add"
	// KindDelete marks a scalar value present only in base (E1-S01).
	KindDelete Kind = "delete"
)

// Position is a 1-indexed source location (line/column) within a file's byte stream, as
// reported by the YAML parser. It anchors a Change to the exact spot a value lives so a forge
// adapter can post an inline comment at the right line (ADR-0003 Amendment 2, "first-class
// positions"). A Change carries a position on each side where a value EXISTS: both sides for a
// modify, the head side only for an add, the base side only for a delete.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Change is one field-level delta within a file, shaped after ADR-0011's Change.
// Classes/Environment are populated later by the classifier (ADR-0008) and are empty here.
type Change struct {
	File string `json:"file"`
	Path string `json:"path"` // RFC-6901 JSON pointer within the file
	Kind Kind   `json:"kind"`

	// Old and New are the CANONICAL, TAG-DISCRIMINATING string forms of the base and head
	// scalar values, produced by render(): !!int/!!float render as their RAW literal ("12",
	// "18446744073709551617"), !!bool as its raw literal ("true"), !!null as "null", and !!str
	// values are JSON-QUOTED (the string "12" renders as "\"12\"", not "12"). Quoting keeps a
	// bool/number/null distinct from a same-spelled string, and using the raw literal (never a
	// Go-coerced number) keeps distinct numeric literals distinct — no lossy collapse.
	//
	// CONTRACT for downstream consumers (S03 aggregation / CEL asserts): these diverge from
	// ADR-0011's typed `Value` — they are strings, not typed scalars, and numeric ones are the
	// raw YAML literal (so `016` stays "016", not "14" or "16"). Any NUMERIC or boolean
	// comparison (e.g. `new > 12`) MUST coerce Old/New at the CEL boundary; a naive string
	// comparison is lexical and WRONG (lexically `"9" > "12"` is true). S03 owns that coercion;
	// this slice only guarantees a byte-stable, injective canonical string.
	Old string `json:"old"`
	New string `json:"new"`

	// OldPos / NewPos are the source positions of the base and head values. A pointer so the
	// absent side of an add/delete is nil (omitted from JSON), not a misleading {0,0}: NewPos is
	// nil on a delete, OldPos is nil on an add, both are set on a modify.
	OldPos *Position `json:"oldPos,omitempty"`
	NewPos *Position `json:"newPos,omitempty"`

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

// Diff computes the modify-only ChangeSet between the base and head bytes of one YAML file.
//
// It is a pure function: identical inputs always produce byte-identical output. On any input
// it cannot decide (unparseable YAML, alias/anchor/merge expansion, a structural delta beyond
// a scalar modify, or a non-string-keyed / duplicate-keyed mapping) it returns an opaque
// ChangeSet plus a wrapped ErrOpaque — never a silent empty diff.
func Diff(file string, base, head []byte) (ChangeSet, error) {
	baseRoot, err := parseSingleMappingDoc(base)
	if err != nil {
		return opaque("base: " + err.Error())
	}
	headRoot, err := parseSingleMappingDoc(head)
	if err != nil {
		return opaque("head: " + err.Error())
	}

	var changes []Change
	if reason := walkMap(file, "", baseRoot, headRoot, &changes); reason != "" {
		return opaque(reason)
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].File != changes[j].File {
			return changes[i].File < changes[j].File
		}
		return changes[i].Path < changes[j].Path
	})
	return ChangeSet{Changes: changes}, nil
}

// YAML node kinds (gopkg.in/yaml.v3 does not export named constants for these).
const (
	kindDocument = yaml.DocumentNode
	kindMapping  = yaml.MappingNode
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

// parseSingleMappingDoc is the single CONFIDENCE GATE for the modify-only field differ. It
// returns a non-error result ONLY when data is EXACTLY ONE fully-understood YAML document whose
// ROOT is a string-keyed mapping — the only shape a field-level modify-only differ can
// meaningfully diff. Everything else fails CLOSED (returns an error -> the caller emits opaque):
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
// expansion — are enforced during the walk, because decoding into *yaml.Node (unlike decoding
// into `any`) does NOT itself reject them: aliases are recorded unexpanded and duplicate keys
// are kept, so this differ must re-assert those guards explicitly.
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

// walkMap compares two mapping nodes key-by-key. A key present on only one side is a first-class
// add (head-only) or delete (base-only) when its value is a scalar (emitOneSided); a one-sided
// key whose value is a mapping/sequence stays opaque (deferred to E1-S03/S05). Any non-string
// key, duplicate key, or alias/anchor/merge key makes the whole diff opaque (fail-safe), because
// none of those can be diffed without risking a silent miss.
func walkMap(file, pointer string, base, head *yaml.Node, out *[]Change) string {
	baseVals, reason := mappingEntries(pointer, base)
	if reason != "" {
		return reason
	}
	headVals, reason := mappingEntries(pointer, head)
	if reason != "" {
		return reason
	}

	// Union of keys, sorted for determinism.
	seen := make(map[string]struct{}, len(baseVals)+len(headVals))
	keys := make([]string, 0, len(baseVals)+len(headVals))
	for k := range baseVals {
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	for k := range headVals {
		if _, ok := seen[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		bv, inBase := baseVals[k]
		hv, inHead := headVals[k]
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
			if reason := walk(file, child, bv, hv, out); reason != "" {
				return reason
			}
		}
	}
	return ""
}

// mappingEntries extracts the string keys and their value nodes from a mapping node, failing
// closed on any construct this thin differ cannot safely diff: a non-string key, a duplicate
// key, or an alias/anchor/merge key. A non-empty reason routes the caller to opaque.
func mappingEntries(pointer string, m *yaml.Node) (map[string]*yaml.Node, string) {
	vals := make(map[string]*yaml.Node, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		v := m.Content[i+1]
		if k.Kind == kindAlias || k.Anchor != "" || k.ShortTag() == tagMerge {
			return nil, fmt.Sprintf("alias/anchor/merge key in mapping at %q — not decidable in modify-only slice", pointerOrRoot(pointer))
		}
		if k.Kind != kindScalar || k.ShortTag() != tagStr {
			return nil, fmt.Sprintf("non-string mapping key (kind %d, tag %q) at %q — not decidable in modify-only slice", k.Kind, k.ShortTag(), pointerOrRoot(pointer))
		}
		if _, dup := vals[k.Value]; dup {
			return nil, fmt.Sprintf("duplicate mapping key %q at %q — not decidable in modify-only slice", k.Value, pointerOrRoot(pointer))
		}
		vals[k.Value] = v
	}
	return vals, ""
}

// walk compares base and head value nodes at pointer, appending a KindModify change when two
// scalars render differently. It recurses into string-keyed mappings; any other shape — a
// sequence, an alias/anchor node, a mapping-vs-scalar type flip, or an unrenderable/unknown
// scalar tag — returns a non-empty reason (-> opaque), never a silent drop.
func walk(file, pointer string, base, head *yaml.Node, out *[]Change) string {
	// Aliases/anchors are never expanded (expansion re-arms alias bombs) — they are opaque.
	if base.Kind == kindAlias || head.Kind == kindAlias || base.Anchor != "" || head.Anchor != "" {
		return fmt.Sprintf("alias/anchor node at %q — not decidable in modify-only slice", pointerOrRoot(pointer))
	}

	baseIsMap := base.Kind == kindMapping
	headIsMap := head.Kind == kindMapping
	switch {
	case baseIsMap && headIsMap:
		return walkMap(file, pointer, base, head, out)
	case baseIsMap != headIsMap:
		return fmt.Sprintf("type flip at %q (map vs non-map) — structural delta is out of modify-only scope", pointerOrRoot(pointer))
	}

	// Both non-mappings: they must both be renderable scalars. A sequence, or any scalar with
	// a tag render() does not understand, fails closed.
	bo, baseOK := render(base)
	ho, headOK := render(head)
	if !baseOK || !headOK {
		return fmt.Sprintf(
			"non-scalar or unrenderable value at %q (base kind %d tag %q, head kind %d tag %q) — not decidable in modify-only slice",
			pointerOrRoot(pointer), base.Kind, base.ShortTag(), head.Kind, head.ShortTag(),
		)
	}

	// A Change is emitted iff the two scalars differ by their TAG-QUALIFIED key. Comparing on
	// (tag, render) — not render alone — is what makes the equality INJECTIVE over (tag, literal):
	// two scalars are equal iff they share BOTH the resolved tag AND the canonical render. This
	// closes the trio hole where an explicitly-tagged `!!int 1` and `!!float 1` (or `!!float 1`
	// and `!!bool 1`) share the raw literal "1" and would otherwise render-equal and be silently
	// dropped on a changed doc. `!!null` still normalizes to one key (`~` == `null`), and string
	// quoting keeps `!!str "1"` distinct from `!!int 1`. The EMITTED Old/New stay the human
	// render (bare "12", quoted strings) so goldens are unchanged — only the comparator is
	// tag-qualified. render never coerces a numeric literal through a lossy Go type, so distinct
	// literals also never collapse: the comparison can over-report but never silently miss.
	if compareKey(base, bo) != compareKey(head, ho) {
		*out = append(*out, Change{
			File: file, Path: pointer, Kind: KindModify,
			Old: bo, New: ho,
			OldPos: posOf(base), NewPos: posOf(head),
		})
	}
	return ""
}

// emitOneSided emits an add (side == head, kind KindAdd) or a delete (side == base, kind
// KindDelete) for a value present on exactly one side of a mapping. Only a SCALAR value is
// decidable here: it yields one Change carrying the rendered value and a position on the present
// side. A MAPPING or SEQUENCE value (a whole added/removed collection subtree), an alias/anchor
// node, or an unrenderable scalar returns a non-empty reason so the diff fails closed to opaque.
// Collection add/delete is deferred UNIFORMLY to the value-tree era and E1-S05's EntryRef
// derivation — this story does not widen what is decidable beyond a scalar key add/delete, and
// deliberately does not recurse into a one-sided collection (which would risk a silent miss on
// an empty subtree, and has no single "canonical render" the way a scalar does).
func emitOneSided(file, pointer string, node *yaml.Node, kind Kind, out *[]Change) string {
	if node.Kind == kindAlias || node.Anchor != "" {
		return fmt.Sprintf("alias/anchor node at %q — not decidable", pointerOrRoot(pointer))
	}
	if node.Kind != kindScalar {
		// A mapping/sequence on a one-sided key: add/delete of a whole collection is E1-S05
		// territory, so fail closed rather than under-report or partially derive it here.
		return fmt.Sprintf(
			"one-sided %s of a non-scalar value at %q (kind %d) — collection add/delete is out of scope (E1-S05)",
			kind, pointerOrRoot(pointer), node.Kind,
		)
	}
	r, ok := render(node)
	if !ok {
		return fmt.Sprintf(
			"unrenderable scalar at %q (kind %d tag %q) — not decidable",
			pointerOrRoot(pointer), node.Kind, node.ShortTag(),
		)
	}
	ch := Change{File: file, Path: pointer, Kind: kind}
	if kind == KindAdd {
		ch.New, ch.NewPos = r, posOf(node)
	} else {
		ch.Old, ch.OldPos = r, posOf(node)
	}
	*out = append(*out, ch)
	return ""
}

// posOf reads a node's 1-indexed source position. Returns nil for a nil node (the absent side of
// an add/delete never calls this — the pointer field is simply left nil).
func posOf(n *yaml.Node) *Position {
	if n == nil {
		return nil
	}
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
