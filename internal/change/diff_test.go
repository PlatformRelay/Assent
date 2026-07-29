package change

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/hash"
)

// baseYAML mirrors examples/repos/topic-registry/topics/prod/orders.events.v1.yaml —
// a single governed topic entry. head changes exactly one scalar: partitions 12 -> 24.
const baseYAML = `name: orders.events.v1
owner: orders-team
partitions: 12
replication_factor: 3
retention_hours: 168
cleanup_policy: delete
`

const headYAML = `name: orders.events.v1
owner: orders-team
partitions: 24
replication_factor: 3
retention_hours: 168
cleanup_policy: delete
`

// canonicalJSON marshals a ChangeSet to canonical JSON via the shared hash helper so
// goldens are byte-stable regardless of Go map iteration order.
func canonicalJSON(t *testing.T, cs ChangeSet) []byte {
	t.Helper()
	raw, err := json.Marshal(cs)
	if err != nil {
		t.Fatalf("marshal ChangeSet: %v", err)
	}
	canon, err := hash.Canonicalize(raw)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	return canon
}

// REQ-P4-E1-S02-01 — one scalar field changed -> exactly one modify entry.
func TestModifyOnlyYAMLDiff(t *testing.T) {
	cs, err := Diff("orders.events.v1.yaml", []byte(baseYAML), []byte(headYAML))
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("expected decidable diff, got opaque: %s", cs.OpaqueReason)
	}
	if len(cs.Changes) != 1 {
		t.Fatalf("expected exactly 1 change, got %d: %+v", len(cs.Changes), cs.Changes)
	}
	c := cs.Changes[0]
	if c.File != "orders.events.v1.yaml" {
		t.Errorf("File = %q, want orders.events.v1.yaml", c.File)
	}
	if c.Path != "/partitions" {
		t.Errorf("Path = %q, want /partitions", c.Path)
	}
	if c.Kind != KindModify {
		t.Errorf("Kind = %q, want %q", c.Kind, KindModify)
	}
	if c.Old != "12" {
		t.Errorf("Old = %q, want 12", c.Old)
	}
	if c.New != "24" {
		t.Errorf("New = %q, want 24", c.New)
	}

	// REQ-E1-S01-03 — the shipped modify golden now carries a position for BOTH Old and New
	// pointing at the `partitions:` value (line 3, column 13) in each byte stream. File / Path /
	// Kind / Old / New are UNCHANGED from the P4-E1 golden; only oldPos/newPos are added.
	const golden = `{"changes":[{"file":"orders.events.v1.yaml","kind":"modify","new":"24","newPos":{"column":13,"line":3},"old":"12","oldPos":{"column":13,"line":3},"path":"/partitions"}],"opaque":false}`
	got := canonicalJSON(t, cs)
	if string(got) != golden {
		t.Errorf("golden mismatch:\n got: %s\nwant: %s", got, golden)
	}
	if c.OldPos == nil || c.NewPos == nil {
		t.Fatalf("modify must carry a position on both sides, got oldPos=%v newPos=%v", c.OldPos, c.NewPos)
	}
	if c.OldPos.Line != 3 || c.NewPos.Line != 3 {
		t.Errorf("positions should point at the partitions line (3), got old=%+v new=%+v", c.OldPos, c.NewPos)
	}
}

// REQ-E1-S01-01 — a key present in head but absent in base emits one Kind:add Change with the
// head value, an empty Old, and a non-nil position on the HEAD side only (no longer opaque).
func TestAddedKeyEmitsAddChange(t *testing.T) {
	base := "a: 1\n"
	head := "a: 1\nb: 2\n"
	cs, err := Diff("f.yaml", []byte(base), []byte(head))
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("added key must be decidable now, got opaque: %s", cs.OpaqueReason)
	}
	if len(cs.Changes) != 1 {
		t.Fatalf("expected exactly one add change, got %d: %+v", len(cs.Changes), cs.Changes)
	}
	c := cs.Changes[0]
	if c.Kind != KindAdd {
		t.Errorf("Kind = %q, want %q", c.Kind, KindAdd)
	}
	if c.Path != "/b" {
		t.Errorf("Path = %q, want /b", c.Path)
	}
	if c.New != "2" {
		t.Errorf("New = %q, want 2", c.New)
	}
	if c.Old != "" {
		t.Errorf("Old = %q, want empty for an add", c.Old)
	}
	if c.NewPos == nil {
		t.Fatalf("add must carry a non-nil position on the head side")
	}
	if c.OldPos != nil {
		t.Errorf("add must have a nil position on the base side, got %+v", c.OldPos)
	}
	if c.NewPos.Line != 2 {
		t.Errorf("added key position should be line 2, got %+v", c.NewPos)
	}
}

// REQ-E1-S01-02 — a key present in base but absent in head emits one Kind:delete Change with the
// base value in Old, an empty New, and a non-nil position on the BASE side only. Adversarial: an
// add, a delete, and an unrelated modify in the SAME document are reported independently (never
// merged/conflated), proven by a byte-stable golden of all three.
func TestRemovedKeyEmitsDeleteChange(t *testing.T) {
	base := "keep: 1\ngone: 2\nmod: 3\n"
	head := "keep: 1\nmod: 4\nadded: 5\n"
	cs, err := Diff("f.yaml", []byte(base), []byte(head))
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("add+delete+modify doc must be decidable, got opaque: %s", cs.OpaqueReason)
	}
	if len(cs.Changes) != 3 {
		t.Fatalf("expected exactly 3 changes (add, delete, modify), got %d: %+v", len(cs.Changes), cs.Changes)
	}
	// Canonical order is by (File, Path): /added, /gone, /mod.
	const golden = `{"changes":[{"file":"f.yaml","kind":"add","new":"5","newPos":{"column":8,"line":3},"old":"","path":"/added"},{"file":"f.yaml","kind":"delete","new":"","old":"2","oldPos":{"column":7,"line":2},"path":"/gone"},{"file":"f.yaml","kind":"modify","new":"4","newPos":{"column":6,"line":2},"old":"3","oldPos":{"column":6,"line":3},"path":"/mod"}],"opaque":false}`
	got := canonicalJSON(t, cs)
	if string(got) != golden {
		t.Errorf("golden mismatch:\n got: %s\nwant: %s", got, golden)
	}
	// The delete entry specifically: value in Old, empty New, base-side position only.
	var del Change
	for _, c := range cs.Changes {
		if c.Kind == KindDelete {
			del = c
		}
	}
	if del.Path != "/gone" || del.Old != "2" || del.New != "" {
		t.Errorf("delete entry wrong: %+v", del)
	}
	if del.OldPos == nil || del.NewPos != nil {
		t.Errorf("delete must carry a base-side position only, got old=%v new=%v", del.OldPos, del.NewPos)
	}
}

// REQ-P4-E1-S02-02 / REQ-E1-S01-05 — double-run byte-stable, now covering add and delete
// (not just modify): the new add/delete/position code must be as deterministic as the modify path.
func TestDiffDoubleRunStable(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"modify", baseYAML, headYAML},
		{"add", "a: 1\n", "a: 1\nb: 2\n"},
		{"delete", "a: 1\nb: 2\n", "a: 1\n"},
		{"add+delete+modify", "keep: 1\ngone: 2\nmod: 3\n", "keep: 1\nmod: 4\nadded: 5\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs1, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if err != nil {
				t.Fatalf("run 1: %v", err)
			}
			cs2, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if err != nil {
				t.Fatalf("run 2: %v", err)
			}
			g1 := canonicalJSON(t, cs1)
			g2 := canonicalJSON(t, cs2)
			if !bytes.Equal(g1, g2) {
				t.Errorf("double-run diverged:\n run1: %s\n run2: %s", g1, g2)
			}
		})
	}
}

// REQ-P4-E1-S02-03 — unparseable / opaque input fails safe to an opaque marker,
// never a silent empty ChangeSet.
func TestOpaqueInputFailsSafe(t *testing.T) {
	cases := []struct {
		name string
		base string
		head string
	}{
		{
			name: "billion-laughs alias expansion",
			base: "partitions: 12\n",
			// If yaml.v3 expanded this alias bomb it would parse to a single key "partitions"
			// (same key set as base) — so an opaque result here can ONLY come from parse-time
			// rejection of the expansion, not the structural-delta path.
			head: "anchors: &a [\"x\",\"x\",\"x\",\"x\",\"x\",\"x\",\"x\",\"x\",\"x\"]\nb: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a]\nc: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b]\npartitions: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c]\n",
		},
		{
			name: "invalid YAML bytes in head",
			base: baseYAML,
			head: "key: [unterminated\n\t: : :bad",
		},
		{
			name: "invalid YAML bytes in base",
			base: "\t: : :bad\nkey: [unterminated",
			head: headYAML,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("expected Opaque=true (fail-safe), got Opaque=false with %d changes; a caller checking len(Changes)==0 would read this as safe", len(cs.Changes))
			}
			if cs.OpaqueReason == "" {
				t.Errorf("Opaque=true but OpaqueReason empty; caller cannot map to REVIEW with a reason")
			}
			if len(cs.Changes) != 0 {
				t.Errorf("opaque ChangeSet must carry no partial changes, got %d", len(cs.Changes))
			}
			if err == nil {
				t.Errorf("expected a non-nil error accompanying the opaque marker")
			}
		})
	}
}

// REQ-E1-S01-04 — a genuinely-undecidable STRUCTURAL delta must surface as opaque, NEVER be
// silently dropped (fail-open guard, GUIDELINES §2). E1-S01 makes a SCALAR key add/delete
// decidable (see TestAddedKeyEmitsAddChange / TestRemovedKeyEmitsDeleteChange), so the former
// "key added"/"key removed" scalar cases moved there; this suite now locks the shapes that MUST
// stay opaque — type flips (map <-> scalar) and one-sided keys whose value is a COLLECTION
// (mapping/sequence), which are E1-S05 territory, not a bare mapping-scalar add/delete.
func TestStructuralDeltaFailsSafe(t *testing.T) {
	cases := []struct {
		name string
		base string
		head string
	}{
		{
			name: "scalar flips to map",
			base: "a: 1\n",
			head: "a: {x: 1}\n",
		},
		{
			name: "map flips to scalar",
			base: "a: {x: 1}\n",
			head: "a: 1\n",
		},
		{
			name: "added key with a mapping value (collection add -> E1-S05, opaque here)",
			base: "a: 1\n",
			head: "a: 1\nb: {x: 1}\n",
		},
		{
			name: "removed key with a mapping value (collection delete -> E1-S05, opaque here)",
			base: "a: 1\nb: {x: 1}\n",
			head: "a: 1\n",
		},
		{
			name: "added key with a sequence value (collection add -> E1-S05, opaque here)",
			base: "a: 1\n",
			head: "a: 1\nb: [1, 2]\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("structural delta %q must be opaque (not silently dropped), got Opaque=false with %d changes", tc.name, len(cs.Changes))
			}
			if len(cs.Changes) != 0 {
				t.Errorf("opaque result must carry no partial changes, got %d", len(cs.Changes))
			}
			if err == nil {
				t.Errorf("expected non-nil error for structural delta")
			}
		})
	}
}

// A no-op edit (only whitespace / comment differences) yields an empty, non-opaque
// ChangeSet — that is a genuine "nothing changed", distinct from the opaque signal.
func TestNoChangeIsEmptyNotOpaque(t *testing.T) {
	cs, err := Diff("f.yaml", []byte("a: 1\nb: 2\n"), []byte("a: 1  # comment\nb: 2\n"))
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("no-op edit must not be opaque: %s", cs.OpaqueReason)
	}
	if len(cs.Changes) != 0 {
		t.Errorf("no-op edit must yield zero changes, got %d: %+v", len(cs.Changes), cs.Changes)
	}
}

// Nested scalar change resolves to a JSON-pointer path with escaping.
func TestNestedScalarPointer(t *testing.T) {
	cs, err := Diff("f.yaml", []byte("schema:\n  format: avro\n"), []byte("schema:\n  format: json\n"))
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
	}
	if len(cs.Changes) != 1 || cs.Changes[0].Path != "/schema/format" {
		t.Fatalf("expected one change at /schema/format, got %+v", cs.Changes)
	}
	if cs.Changes[0].Old != `"avro"` || cs.Changes[0].New != `"json"` {
		t.Errorf("old/new = %q/%q, want quoted avro/json", cs.Changes[0].Old, cs.Changes[0].New)
	}
}

// Scalar type rendering must be canonical and byte-stable across YAML scalar kinds.
func TestScalarTypesRenderCanonically(t *testing.T) {
	cases := []struct {
		name             string
		base, head       string
		wantOld, wantNew string
	}{
		{"bool", "flag: true\n", "flag: false\n", "true", "false"},
		{"int", "n: 3\n", "n: 5\n", "3", "5"},
		{"float", "r: 1.5\n", "r: 2.25\n", "1.5", "2.25"},
		{"null-to-scalar-string", "v: ~\n", "v: set\n", "null", `"set"`},
		{"string", "s: avro\n", "s: json\n", `"avro"`, `"json"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}
			if cs.Opaque {
				t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
			}
			if len(cs.Changes) != 1 {
				t.Fatalf("want 1 change, got %d: %+v", len(cs.Changes), cs.Changes)
			}
			if cs.Changes[0].Old != tc.wantOld || cs.Changes[0].New != tc.wantNew {
				t.Errorf("old/new = %q/%q, want %q/%q", cs.Changes[0].Old, cs.Changes[0].New, tc.wantOld, tc.wantNew)
			}
		})
	}
}

// A sequence on either side is undecidable in the modify-only slice -> opaque, both when
// nested under a key and at the document root.
func TestSequenceIsOpaque(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"sequence in base value", "a: [1,2]\n", "a: 1\n"},
		{"sequence in head value", "a: 1\n", "a: [1,2]\n"},
		{"root sequence", "[1,2]\n", "[1,3]\n"},
		{"both root sequences", "[1,2]\n", "[3,4]\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("sequence %q must be opaque, got %d changes", tc.name, len(cs.Changes))
			}
			if err == nil {
				t.Errorf("expected error for sequence case")
			}
		})
	}
}

// A document root that is not a string-keyed mapping (a bare scalar here) is not a
// field-diffable shape and is rejected at parse -> opaque, with a reason naming the root type.
func TestRootTypeFlipOpaque(t *testing.T) {
	cs, err := Diff("f.yaml", []byte("scalar\n"), []byte("k: 1\n"))
	if !cs.Opaque {
		t.Fatalf("root non-mapping must be opaque, got %d changes", len(cs.Changes))
	}
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(cs.OpaqueReason, "root") {
		t.Errorf("reason should explain the non-mapping root, got %q", cs.OpaqueReason)
	}
}

// ErrOpaque is exported so error-checking callers can classify the failure.
func TestErrOpaqueWrapped(t *testing.T) {
	_, err := Diff("f.yaml", []byte("a: 1\n"), []byte(": : bad ["))
	if err == nil || !errors.Is(err, ErrOpaque) {
		t.Fatalf("expected error wrapping ErrOpaque, got %v", err)
	}
}

// F1 regression (P1) — non-string YAML map keys must NEVER void the fail-safe contract.
// yaml.v3 decodes any map with a non-string key into map[interface{}]interface{}, which is
// neither map[string]any nor []any. Before the fix these fell through to scalar
// stringification: a real value change was silently dropped (fail-OPEN) or emitted as a
// garbage "map[...]" change. Every one of these MUST be Opaque=true, zero changes, with err.
func TestNonStringMapKeyFailsSafe(t *testing.T) {
	cases := []struct{ name, base, head string }{
		// (a) SAFETY-CRITICAL: a real value change under a numeric key — the exact silent
		// drop REQ-S02-03 forbids. Must be opaque, not "nothing changed".
		{"numeric key int->float value change", "1: 1\n", "1: 1.0\n"},
		{"numeric key string value change", "1: alpha\n", "1: beta\n"},
		// (b) garbage-change guard: a numeric key nested under a string key must not emit
		// Old:"map[1:alpha]" New:"map[1:beta]".
		{"nested numeric-key map value change", "cfg:\n  1: alpha\n", "cfg:\n  1: beta\n"},
		// (c) structural change under a non-string key must be opaque, not a bogus modify.
		{"numeric key added", "1: a\n", "1: a\n2: b\n"},
		{"numeric key removed", "1: a\n2: b\n", "1: a\n"},
		// type-flip UNDER a non-string key.
		{"numeric key scalar->map flip", "1: a\n", "1: {x: 1}\n"},
		{"numeric key map->scalar flip", "1: {x: 1}\n", "1: a\n"},
		// boolean and null keys, for good measure (also non-string keys).
		{"boolean key value change", "true: a\n", "true: b\n"},
		{"null key value change", "null: a\n", "null: b\n"},
		// a non-string-keyed map that is byte-for-byte equal must STILL be opaque — the
		// differ cannot prove it unchanged, so it fails closed rather than reporting "safe".
		{"identical non-string-keyed map is still opaque", "1: a\n", "1: a\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("non-string-key case %q must be Opaque=true (fail closed), got Opaque=false with %d changes: %+v — a caller checking len(Changes)==0 would read this as safe", tc.name, len(cs.Changes), cs.Changes)
			}
			if len(cs.Changes) != 0 {
				t.Errorf("opaque result must carry no partial/garbage changes, got %d: %+v", len(cs.Changes), cs.Changes)
			}
			if err == nil || !errors.Is(err, ErrOpaque) {
				t.Errorf("expected error wrapping ErrOpaque, got %v", err)
			}
		})
	}
}

// The non-string-key opaque path must remain deterministic (double-run byte-stable) and the
// package must stay pure — properties proven generally elsewhere, re-asserted on this path.
func TestNonStringKeyOpaqueIsDeterministic(t *testing.T) {
	base, head := []byte("1: 1\n"), []byte("1: 2\n")
	cs1, err1 := Diff("f.yaml", base, head)
	cs2, err2 := Diff("f.yaml", base, head)
	if !cs1.Opaque || !cs2.Opaque {
		t.Fatalf("both runs must be opaque")
	}
	if err1 == nil || err2 == nil {
		t.Fatalf("both runs must error")
	}
	if !bytes.Equal(canonicalJSON(t, cs1), canonicalJSON(t, cs2)) {
		t.Errorf("opaque double-run diverged")
	}
	if cs1.OpaqueReason != cs2.OpaqueReason {
		t.Errorf("opaque reason not stable: %q vs %q", cs1.OpaqueReason, cs2.OpaqueReason)
	}
}

// F1 (second manifestation, same fail-open class) — distinct scalar KINDS whose string
// spellings coincide must NOT collapse to "nothing changed". A realistic quoting/unquoting
// edit (`enabled: true` -> `enabled: "true"`) is a real change and must be detected, never
// silently dropped (GUIDELINES §2). scalarString is type-discriminating: strings render
// JSON-quoted, so a string never collides with a bool/number/null of the same spelling.
func TestScalarKindChangeNotCollapsed(t *testing.T) {
	cases := []struct {
		name             string
		base, head       string
		wantOld, wantNew string
	}{
		{"bool true -> string \"true\"", "enabled: true\n", "enabled: \"true\"\n", "true", `"true"`},
		{"int 1 -> string \"1\"", "n: 1\n", "n: \"1\"\n", "1", `"1"`},
		{"null -> string \"null\"", "v: null\n", "v: \"null\"\n", "null", `"null"`},
		{"float 1.5 -> string \"1.5\"", "r: 1.5\n", "r: \"1.5\"\n", "1.5", `"1.5"`},
		{"string \"true\" -> bool true", "enabled: \"true\"\n", "enabled: true\n", `"true"`, "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}
			if cs.Opaque {
				t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
			}
			if len(cs.Changes) != 1 {
				t.Fatalf("kind change %q must be detected as exactly one modify, got %d changes: %+v (a zero here is the silent-drop fail-open)", tc.name, len(cs.Changes), cs.Changes)
			}
			if cs.Changes[0].Old != tc.wantOld || cs.Changes[0].New != tc.wantNew {
				t.Errorf("old/new = %q/%q, want %q/%q", cs.Changes[0].Old, cs.Changes[0].New, tc.wantOld, tc.wantNew)
			}
		})
	}
}

// Scalar comparison is INJECTIVE (by canonical tag+raw-literal), not numeric-equivalence.
// The skeleton differ compares scalars precisely; numeric cross-type equivalence (int 1 ==
// float 1.0) is intentionally NOT collapsed, because the lossy numeric DECODE that a collapse
// would require is exactly what caused F-01's silent-miss fail-open (two distinct ints >= 2^64
// both coercing to one float64). Precise OVER-reporting is safe; a silent under-report is the
// only forbidden direction. So:
//   - identical literals and literals that render identically -> no change,
//   - any literal difference (including int 1 vs float 1.0, differing tags) -> a detected change.
func TestScalarComparisonIsInjective(t *testing.T) {
	noChange := []struct{ name, base, head string }{
		{"identical int", "n: 16\n", "n: 16\n"},
		{"identical float literal", "n: 1.5\n", "n: 1.5\n"},
		{"identical string", "s: avro\n", "s: avro\n"},
		{"null tilde vs null keyword (both null)", "v: ~\n", "v: null\n"},
	}
	for _, tc := range noChange {
		t.Run("nochange/"+tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}
			if cs.Opaque {
				t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
			}
			if len(cs.Changes) != 0 {
				t.Errorf("%q must be no change, got %+v", tc.name, cs.Changes)
			}
		})
	}

	// Distinct literals — including int-vs-float of "the same" mathematical value — are changes.
	change := []struct{ name, base, head, wantOld, wantNew string }{
		{"int 1 vs float 1.0 (different tags)", "n: 1\n", "n: 1.0\n", "1", "1.0"},
		{"float 1.0 vs 1.00 (over-report, safe)", "n: 1.0\n", "n: 1.00\n", "1.0", "1.00"},
		{"hex 0x10 vs decimal 16 (over-report, safe)", "n: 0x10\n", "n: 16\n", "0x10", "16"},
		{"int 1 vs 01 leading-zero (over-report, safe)", "n: 1\n", "n: 01\n", "1", "01"},
	}
	for _, tc := range change {
		t.Run("change/"+tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}
			if cs.Opaque {
				t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
			}
			if len(cs.Changes) != 1 {
				t.Fatalf("%q must be exactly one change, got %d: %+v", tc.name, len(cs.Changes), cs.Changes)
			}
			if cs.Changes[0].Old != tc.wantOld || cs.Changes[0].New != tc.wantNew {
				t.Errorf("old/new = %q/%q, want %q/%q", cs.Changes[0].Old, cs.Changes[0].New, tc.wantOld, tc.wantNew)
			}
		})
	}
}

// F-01 / F-02 (P1) — the lossy-numeric-decode fail-open: distinct large integers and distinct
// high-precision floats must ALWAYS emit a Change (old != new), never silently miss. Before the
// yaml.Node rewrite, ints >= 2^64 and near-equal floats collapsed to one float64 and were
// dropped. These lock the injective comparison across the int64/uint64/overflow boundaries.
func TestLosslessNumericComparison(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"2^63-1 vs 2^63", "n: 9223372036854775807\n", "n: 9223372036854775808\n"},
		{"2^63 vs 2^63+1", "n: 9223372036854775808\n", "n: 9223372036854775809\n"},
		{"2^64-1 vs 2^64", "n: 18446744073709551615\n", "n: 18446744073709551616\n"},
		{"2^64 vs 2^64+1", "n: 18446744073709551616\n", "n: 18446744073709551617\n"},
		{"headline: 2^64+1 vs 2^64+2", "n: 18446744073709551617\n", "n: 18446744073709551618\n"},
		{"high-precision floats", "n: 1.0000000000000001\n", "n: 1.0000000000000002\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}
			if cs.Opaque {
				t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
			}
			if len(cs.Changes) != 1 {
				t.Fatalf("%q: distinct numeric literals must emit exactly one change (no silent miss), got %d: %+v", tc.name, len(cs.Changes), cs.Changes)
			}
			if cs.Changes[0].Old == cs.Changes[0].New {
				t.Errorf("%q: change has old==new (%q) — the values were coerced/collapsed", tc.name, cs.Changes[0].Old)
			}
		})
	}
}

// F1 (third manifestation, same fail-open class) — a multi-document YAML stream must fail
// closed. A single yaml.Unmarshal decodes ONLY the first document and silently discards the
// rest, so a change confined to document 2+ would read as "nothing changed -> safe". Every
// multi-doc shape (2nd-doc modify, 2nd-doc key-add, single-vs-multi on either side) is opaque.
func TestMultiDocumentFailsSafe(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"2nd-doc modify", "a: 1\n---\nb: 2\n", "a: 1\n---\nb: 3\n"},
		{"2nd-doc key-add", "a: 1\n---\nb: 2\n", "a: 1\n---\nb: 2\nc: 4\n"},
		{"single base, multidoc head", "a: 1\n", "a: 1\n---\nb: 2\n"},
		{"multidoc base, single head", "a: 1\n---\nb: 2\n", "a: 1\n"},
		{"both multidoc", "a: 1\n---\nb: 2\n", "a: 2\n---\nb: 2\n"},
		{"trailing separator produces empty 2nd doc", "a: 1\n---\n", "a: 2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("multi-doc %q must be Opaque=true (fail closed), got Opaque=false with %d changes — a change in doc 2+ would be silently dropped", tc.name, len(cs.Changes))
			}
			if len(cs.Changes) != 0 {
				t.Errorf("opaque result must carry no changes, got %d", len(cs.Changes))
			}
			if err == nil || !errors.Is(err, ErrOpaque) {
				t.Errorf("expected error wrapping ErrOpaque, got %v", err)
			}
		})
	}
}

// Structural-shape audit: the differ emits a non-opaque ChangeSet ONLY when it fully parsed
// exactly one string-keyed-mapping document whose leaves are renderable scalars. Every
// NON-SCALAR / STRUCTURAL shape it cannot account for MUST fail closed to opaque. (Scalar VALUE
// differences are handled separately — those correctly produce changes, not opacity; injective
// value comparison is proven in TestScalarComparisonIsInjective / TestLosslessNumericComparison.)
// This test locks each structural axis so a reviewer probing an exotic YAML SHAPE lands on
// opaque, never a silent empty ChangeSet or a garbage change.
func TestInputSpaceFailsClosed(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"empty base", "", "a: 1\n"},
		{"empty head", "a: 1\n", ""},
		{"both empty", "", ""},
		{"comments-only base (zero documents)", "# only a comment\n", "a: 1\n"},
		{"comments-only head (zero documents)", "a: 1\n", "# only a comment\n"},
		{"root bare scalar", "42\n", "43\n"},
		{"root bare sequence", "- a\n- b\n", "- a\n- c\n"},
		{"root explicit null (~)", "~\n", "a: 1\n"},
		{"root explicit null (null)", "null\n", "a: 1\n"},
		{"root null on head", "a: 1\n", "null\n"},
		{"non-string map key at root", "1: a\n", "1: b\n"},
		{"unparseable bytes", "a: 1\n", ": : [bad"},
		{"duplicate key in base", "a: 1\na: 2\n", "a: 3\n"},
		{"duplicate key in head", "a: 1\n", "a: 3\na: 4\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("input %q must be Opaque=true (fail closed), got Opaque=false with %d changes: %+v", tc.name, len(cs.Changes), cs.Changes)
			}
			if len(cs.Changes) != 0 {
				t.Errorf("opaque result must carry no changes, got %d", len(cs.Changes))
			}
			if err == nil || !errors.Is(err, ErrOpaque) {
				t.Errorf("expected error wrapping ErrOpaque, got %v", err)
			}
			if cs.OpaqueReason == "" {
				t.Errorf("Opaque=true must carry a non-empty reason")
			}
		})
	}
}

// Positive control: a single-document mapping with a leading `---` separator is still exactly
// one document and must diff normally (the strict gate must not over-reject valid single docs).
func TestLeadingSeparatorSingleDocDiffsNormally(t *testing.T) {
	cs, err := Diff("f.yaml", []byte("---\na: 1\n"), []byte("---\na: 2\n"))
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("leading-separator single doc must not be opaque: %s", cs.OpaqueReason)
	}
	if len(cs.Changes) != 1 || cs.Changes[0].Path != "/a" {
		t.Fatalf("expected one change at /a, got %+v", cs.Changes)
	}
}

// The multi-document opaque path is deterministic (double-run byte-stable), preserving the
// double-run gate on the newly-added fail-safe branch.
func TestMultiDocOpaqueDeterministic(t *testing.T) {
	b, h := []byte("a: 1\n---\nb: 2\n"), []byte("a: 1\n---\nb: 3\n")
	cs1, _ := Diff("f.yaml", b, h)
	cs2, _ := Diff("f.yaml", b, h)
	if !cs1.Opaque || !cs2.Opaque {
		t.Fatalf("both runs must be opaque")
	}
	if !bytes.Equal(canonicalJSON(t, cs1), canonicalJSON(t, cs2)) {
		t.Errorf("multi-doc opaque double-run diverged")
	}
	if cs1.OpaqueReason != cs2.OpaqueReason {
		t.Errorf("opaque reason not stable: %q vs %q", cs1.OpaqueReason, cs2.OpaqueReason)
	}
}

// Two scalar fields modified in one document -> a ChangeSet with TWO entries in canonical
// sorted order (/a before /b), proven by a byte-stable golden. This exercises the multi-change
// ordering path (the sort comparator) that single-change tests cannot, and locks stable
// ordering — load-bearing for the REQ-01 byte-stable golden and REQ-02 double-run guarantees.
func TestTwoFieldModifyGolden(t *testing.T) {
	base := "a: 1\nb: 2\n"
	head := "a: 9\nb: 8\n"
	cs, err := Diff("multi.yaml", []byte(base), []byte(head))
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
	}
	if len(cs.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %+v", len(cs.Changes), cs.Changes)
	}
	// Canonical order is by (File, Path): /a before /b.
	if cs.Changes[0].Path != "/a" || cs.Changes[1].Path != "/b" {
		t.Fatalf("changes not in canonical /a,/b order: %+v", cs.Changes)
	}
	const golden = `{"changes":[{"file":"multi.yaml","kind":"modify","new":"9","newPos":{"column":4,"line":1},"old":"1","oldPos":{"column":4,"line":1},"path":"/a"},{"file":"multi.yaml","kind":"modify","new":"8","newPos":{"column":4,"line":2},"old":"2","oldPos":{"column":4,"line":2},"path":"/b"}],"opaque":false}`
	got := canonicalJSON(t, cs)
	if string(got) != golden {
		t.Errorf("golden mismatch:\n got: %s\nwant: %s", got, golden)
	}
}

// The two-field golden must double-run byte-identically (multi-change ordering is stable).
func TestTwoFieldDoubleRunStable(t *testing.T) {
	base, head := []byte("a: 1\nb: 2\n"), []byte("a: 9\nb: 8\n")
	g1 := canonicalJSON(t, mustDiff(t, base, head))
	g2 := canonicalJSON(t, mustDiff(t, base, head))
	if !bytes.Equal(g1, g2) {
		t.Errorf("two-field double-run diverged:\n %s\n %s", g1, g2)
	}
}

func mustDiff(t *testing.T, base, head []byte) ChangeSet {
	t.Helper()
	cs, err := Diff("multi.yaml", base, head)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	return cs
}

// Decoding into *yaml.Node (unlike decoding into `any`) does NOT auto-reject aliases, anchors,
// or merge keys, nor does it expand alias bombs — so the differ re-asserts those guards
// explicitly. Every such construct must be opaque (expanding an alias would re-arm a bomb; a
// merge key changes semantics; an anchored value could be aliased elsewhere), never diffed.
func TestAliasAnchorMergeAreOpaque(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"anchor on a value node", "a: &x 1\nb: 2\n", "a: &x 1\nb: 3\n"},
		{"alias value node", "a: 1\nb: *x\n", "a: 1\nb: 2\n"},
		{"value defined via alias", "a: &x 1\nb: *x\n", "a: 1\nb: 1\n"},
		{"merge key <<", "m:\n  <<: {x: 1}\n  y: 2\n", "m:\n  <<: {x: 1}\n  y: 3\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("%q must be Opaque=true (fail closed), got %d changes: %+v", tc.name, len(cs.Changes), cs.Changes)
			}
			if err == nil || !errors.Is(err, ErrOpaque) {
				t.Errorf("expected error wrapping ErrOpaque, got %v", err)
			}
			if len(cs.Changes) != 0 {
				t.Errorf("opaque result must carry no changes, got %d", len(cs.Changes))
			}
		})
	}
}

// Scalar tags this thin slice does not understand (!!timestamp, !!binary, and any unknown/custom
// tag) are opaque rather than compared — comparing an unmodelled tag risks a mis-comparison.
func TestUnmodelledScalarTagsAreOpaque(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"timestamp", "t: 2020-01-01\n", "t: 2020-01-02\n"},
		{"binary", "b: !!binary aGk=\n", "b: !!binary Ynll\n"},
		{"custom tag", "v: !mytag foo\n", "v: !mytag bar\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("%q must be Opaque=true, got %d changes: %+v", tc.name, len(cs.Changes), cs.Changes)
			}
			if err == nil || !errors.Is(err, ErrOpaque) {
				t.Errorf("expected error wrapping ErrOpaque, got %v", err)
			}
		})
	}
}

// A lone anchor on the ROOT mapping (not aliased anywhere) hides nothing: the root's children
// are still fully walked and compared, so a change under it is detected. If the anchor WERE
// aliased anywhere, the alias node makes the whole diff opaque. Belt-and-suspenders lock so the
// root-anchor case is provably not a silent-miss hole.
func TestUnusedRootAnchorStillDiffsChildren(t *testing.T) {
	cs, err := Diff("f.yaml", []byte("&r\na: 1\n"), []byte("&r\na: 2\n"))
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("unused root anchor should not force opaque: %s", cs.OpaqueReason)
	}
	if len(cs.Changes) != 1 || cs.Changes[0].Path != "/a" {
		t.Fatalf("expected one change at /a (child fully walked), got %+v", cs.Changes)
	}
}

// F5-01 regression (P1) — same raw LITERAL under a DIFFERENT explicit tag must NOT collapse.
// render alone returns the bare literal for the !!int/!!float/!!bool trio, so `!!int 1` and
// `!!float 1` share the render "1"; comparing on render-only silently missed a real change on
// a changed doc. The comparator is now tag-qualified (compareKey), so these emit a change.
//
// DELETE-THE-FEATURE GUARD: these cases pass ONLY because the comparator qualifies by tag.
// With a render-only comparator (bo != ho) every one of them would be Opaque=false + 0 changes
// — the silent miss. That was verified by temporarily reverting compareKey to render-only, at
// which point each subtest fails on "must be exactly one change, got 0".
func TestSameLiteralDifferentTagIsAChange(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"explicit !!int -> !!float, same literal", "port: !!int 8080\n", "port: !!float 8080\n"},
		{"explicit !!float -> !!int, same literal", "port: !!float 8080\n", "port: !!int 8080\n"},
		{"explicit !!float 1 -> !!bool 1", "v: !!float 1\n", "v: !!bool 1\n"},
		{"plain int 1 -> explicit !!float 1 (same literal)", "n: 1\n", "n: !!float 1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}
			if cs.Opaque {
				t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
			}
			if len(cs.Changes) != 1 {
				t.Fatalf("%q: tag change on same literal must emit exactly one change (no silent miss), got %d: %+v", tc.name, len(cs.Changes), cs.Changes)
			}
		})
	}
}

// The tag-qualified comparator must NOT regress the null equivalence: `~`, `null`, and an empty
// (implicit-null) value all resolve to !!null and must compare equal (no spurious change).
func TestNullEquivalencePreservedUnderTagComparison(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"tilde vs null keyword", "v: ~\n", "v: null\n"},
		{"null keyword vs tilde", "v: null\n", "v: ~\n"},
		{"implicit null vs tilde", "v:\n", "v: ~\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.yaml", []byte(tc.base), []byte(tc.head))
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}
			if cs.Opaque {
				t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
			}
			if len(cs.Changes) != 0 {
				t.Errorf("%q: all-null forms must compare equal, got %+v", tc.name, cs.Changes)
			}
		})
	}
}
