package change

import (
	"bytes"
	"errors"
	"testing"
)

// Fixtures drawn from examples/repos/service-catalog/catalog/prod/core-services.json (same
// apiVersion + service scalar fields), trimmed so no ARRAY is reached: this thin slice diffs a
// JSON object's scalar/nested-object fields exactly like YAML's mapping walk, but does not yet
// walk into services[] entries individually (that is E1-S05). The modify fixtures therefore use
// a nested object ("primary") in place of the services[] array.
const jsonBase = `{
  "apiVersion": "catalog/v1",
  "primary": {
    "name": "orders-api",
    "owner": "orders-team",
    "tier": 1
  }
}
`

// jsonHeadAPIVersion edits exactly one TOP-LEVEL scalar: apiVersion catalog/v1 -> catalog/v2.
const jsonHeadAPIVersion = `{
  "apiVersion": "catalog/v2",
  "primary": {
    "name": "orders-api",
    "owner": "orders-team",
    "tier": 1
  }
}
`

// jsonHeadTier edits exactly one NESTED scalar: primary.tier 1 -> 2.
const jsonHeadTier = `{
  "apiVersion": "catalog/v1",
  "primary": {
    "name": "orders-api",
    "owner": "orders-team",
    "tier": 2
  }
}
`

// REQ-E1-S03-02 — a JSON scalar-field edit produces the SAME Change shape (Kind:modify,
// JSON-Pointer Path, tag-qualified Old/New, positions) the YAML differ produces for an
// equivalent edit. Proven for both a top-level and a nested-object field, plus a byte-stable
// golden and a parity assertion against a YAML twin (only the source format differs).
func TestJSONAdapterModify(t *testing.T) {
	t.Run("top-level apiVersion", func(t *testing.T) {
		cs, err := Diff("core-services.json", []byte(jsonBase), []byte(jsonHeadAPIVersion))
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if cs.Opaque {
			t.Fatalf("expected decidable diff, got opaque: %s", cs.OpaqueReason)
		}
		if len(cs.Changes) != 1 {
			t.Fatalf("expected exactly 1 change, got %d: %+v", len(cs.Changes), cs.Changes)
		}
		c := cs.Changes[0]
		if c.File != "core-services.json" || c.Path != "/apiVersion" || c.Kind != KindModify {
			t.Errorf("File/Path/Kind = %q/%q/%q, want core-services.json//apiVersion/modify", c.File, c.Path, c.Kind)
		}
		if c.Old != `"catalog/v1"` || c.New != `"catalog/v2"` {
			t.Errorf("Old/New = %q/%q, want quoted catalog/v1 -> catalog/v2 (string renders JSON-quoted)", c.Old, c.New)
		}
		if c.OldPos == nil || c.NewPos == nil {
			t.Fatalf("modify must carry a position on both sides, got old=%v new=%v", c.OldPos, c.NewPos)
		}
		if c.OldPos.Line != 2 || c.NewPos.Line != 2 {
			t.Errorf("apiVersion value is on line 2, got old=%+v new=%+v", c.OldPos, c.NewPos)
		}
		const golden = `{"changes":[{"file":"core-services.json","kind":"modify","new":"\"catalog/v2\"","newPos":{"column":17,"line":2},"old":"\"catalog/v1\"","oldPos":{"column":17,"line":2},"path":"/apiVersion"}],"opaque":false}`
		if got := string(canonicalJSON(t, cs)); got != golden {
			t.Errorf("golden mismatch:\n got: %s\nwant: %s", got, golden)
		}
	})

	t.Run("nested primary.tier", func(t *testing.T) {
		cs, err := Diff("core-services.json", []byte(jsonBase), []byte(jsonHeadTier))
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if cs.Opaque {
			t.Fatalf("expected decidable diff, got opaque: %s", cs.OpaqueReason)
		}
		if len(cs.Changes) != 1 {
			t.Fatalf("expected exactly 1 change, got %d: %+v", len(cs.Changes), cs.Changes)
		}
		c := cs.Changes[0]
		if c.Path != "/primary/tier" || c.Kind != KindModify || c.Old != "1" || c.New != "2" {
			t.Errorf("nested change wrong: %+v (want /primary/tier modify 1->2)", c)
		}
		if c.OldPos == nil || c.NewPos == nil {
			t.Fatalf("nested modify must carry both positions, got old=%v new=%v", c.OldPos, c.NewPos)
		}
	})

	// Parity: the SAME logical edit expressed as YAML yields the same non-position Change fields
	// (Path/Kind/Old/New). Positions differ (different byte layouts) but the shape is identical —
	// "the same policy pack matches both formats without knowing which one it is looking at".
	t.Run("parity with YAML twin", func(t *testing.T) {
		yamlBase := "apiVersion: catalog/v1\nprimary:\n  name: orders-api\n  tier: 1\n"
		yamlHead := "apiVersion: catalog/v2\nprimary:\n  name: orders-api\n  tier: 1\n"
		jcs, err := Diff("x.json", []byte(`{"apiVersion":"catalog/v1","primary":{"name":"orders-api","tier":1}}`), []byte(`{"apiVersion":"catalog/v2","primary":{"name":"orders-api","tier":1}}`))
		if err != nil || jcs.Opaque {
			t.Fatalf("json diff failed: err=%v opaque=%v", err, jcs.OpaqueReason)
		}
		ycs, err := Diff("x.yaml", []byte(yamlBase), []byte(yamlHead))
		if err != nil || ycs.Opaque {
			t.Fatalf("yaml diff failed: err=%v opaque=%v", err, ycs.OpaqueReason)
		}
		if len(jcs.Changes) != 1 || len(ycs.Changes) != 1 {
			t.Fatalf("both must be one change, got json=%d yaml=%d", len(jcs.Changes), len(ycs.Changes))
		}
		j, y := jcs.Changes[0], ycs.Changes[0]
		if j.Path != y.Path || j.Kind != y.Kind || j.Old != y.Old || j.New != y.New {
			t.Errorf("shape parity broken:\n json: path=%q kind=%q old=%q new=%q\n yaml: path=%q kind=%q old=%q new=%q",
				j.Path, j.Kind, j.Old, j.New, y.Path, y.Kind, y.Old, y.New)
		}
		if j.OldPos == nil || j.NewPos == nil {
			t.Errorf("json modify must still carry positions like yaml, got old=%v new=%v", j.OldPos, j.NewPos)
		}
	})
}

// REQ-E1-S03-03 — two JSON numeric literals that COLLAPSE under a lossy float64 decode
// (9007199254740993 = 2^53+1 vs 9007199254740992 = 2^53 — both round to the same float64) MUST
// be detected as changed. UseNumber keeps the raw literal, so the injective comparison holds for
// JSON exactly as render/compareKey holds for YAML: never a silent miss.
func TestJSONNumericInjective(t *testing.T) {
	base := `{"maxSafe": 9007199254740993}`
	head := `{"maxSafe": 9007199254740992}`
	cs, err := Diff("n.json", []byte(base), []byte(head))
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
	}
	if len(cs.Changes) != 1 {
		t.Fatalf("2^53+1 vs 2^53 must emit exactly one change (float64 would collapse them to a silent miss), got %d: %+v", len(cs.Changes), cs.Changes)
	}
	c := cs.Changes[0]
	if c.Old != "9007199254740993" || c.New != "9007199254740992" {
		t.Errorf("Old/New = %q/%q, want the raw json.Number literals (not a float64)", c.Old, c.New)
	}
	if c.Old == c.New {
		t.Errorf("literals collapsed to %q — the values were coerced through float64", c.Old)
	}
}

// REQ-E1-S03-04 — malformed/truncated JSON, or a root that is not an object (bare array, bare
// scalar), fails opaque with a non-empty reason and ZERO partial changes — never a silent empty
// diff. Mirrors TestOpaqueInputFailsSafe's YAML contract for the JSON adapter.
func TestJSONOpaqueFailsSafe(t *testing.T) {
	const goodObj = `{"a": 1}`
	cases := []struct{ name, base, head string }{
		{"truncated object (no close)", goodObj, `{"a": 1`},
		{"truncated at value", goodObj, `{"a": `},
		{"malformed bytes", goodObj, `{"a": @@@}`},
		{"trailing garbage after object", goodObj, `{"a": 1} {"b": 2}`},
		{"bare array root", goodObj, `[1, 2]`},
		{"bare scalar root (number)", goodObj, `42`},
		{"bare scalar root (string)", goodObj, `"hello"`},
		{"bare null root", goodObj, `null`},
		{"empty input", goodObj, ``},
		{"malformed base", `{"a": `, goodObj},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.json", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("expected Opaque=true (fail-safe), got Opaque=false with %d changes; a caller checking len(Changes)==0 would read this as safe", len(cs.Changes))
			}
			if cs.OpaqueReason == "" {
				t.Errorf("Opaque=true but OpaqueReason empty; caller cannot map to REVIEW with a reason")
			}
			if len(cs.Changes) != 0 {
				t.Errorf("opaque ChangeSet must carry no partial changes, got %d", len(cs.Changes))
			}
			if err == nil || !errors.Is(err, ErrOpaque) {
				t.Errorf("expected error wrapping ErrOpaque, got %v", err)
			}
		})
	}
}

// DoD (pinned in the E1-S03 spec) — a Change on services[] (a JSON array) surfaces the same way
// the differ surfaces any non-mapping-walkable shape today: OPAQUE, until E1-S05 lands array/list
// walking. Mirrors YAML's TestSequenceIsOpaque for the JSON adapter and covers the vSequence walk
// branch.
func TestJSONArrayIsOpaque(t *testing.T) {
	base := `{"apiVersion": "catalog/v1", "services": [{"name": "orders-api", "tier": 1}]}`
	head := `{"apiVersion": "catalog/v1", "services": [{"name": "orders-api", "tier": 2}]}`
	cases := []struct{ name, base, head string }{
		{"two-sided services[] array", base, head},
		{"root array", `[1, 2]`, `[1, 3]`},
		{"nested array value changed", `{"tags": ["a"]}`, `{"tags": ["b"]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.json", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("a JSON array %q must be opaque (E1-S05 territory), got %d changes: %+v", tc.name, len(cs.Changes), cs.Changes)
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

// REQ-E1-S03-05 — the JSON path introduces no env/clock/net/random (proven generally by
// TestChangePackagePurity / TestCorePurity over internal/change/**), and every JSON golden
// double-runs byte-identical. This locks the determinism of the offset->line/col derivation and
// the json.Number comparison.
func TestJSONAdapterDoubleRunStable(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"top-level modify", jsonBase, jsonHeadAPIVersion},
		{"nested modify", jsonBase, jsonHeadTier},
		{"numeric injective", `{"maxSafe": 9007199254740993}`, `{"maxSafe": 9007199254740992}`},
		{"add key", `{"a": 1}`, `{"a": 1, "b": 2}`},
		{"delete key", `{"a": 1, "b": 2}`, `{"a": 1}`},
		{"opaque array", `{"s": [1]}`, `{"s": [2]}`},
		{"opaque malformed", `{"a": 1}`, `{"a":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs1, _ := Diff("f.json", []byte(tc.base), []byte(tc.head))
			cs2, _ := Diff("f.json", []byte(tc.base), []byte(tc.head))
			g1 := canonicalJSON(t, cs1)
			g2 := canonicalJSON(t, cs2)
			if !bytes.Equal(g1, g2) {
				t.Errorf("double-run diverged:\n run1: %s\n run2: %s", g1, g2)
			}
			if cs1.OpaqueReason != cs2.OpaqueReason {
				t.Errorf("opaque reason not stable: %q vs %q", cs1.OpaqueReason, cs2.OpaqueReason)
			}
		})
	}
}

// Adds/deletes of a scalar key in a JSON object are first-class (same as YAML), proving the JSON
// producer feeds the shared one-sided path.
func TestJSONAddDeleteScalar(t *testing.T) {
	add, err := Diff("f.json", []byte(`{"a": 1}`), []byte(`{"a": 1, "b": 2}`))
	if err != nil || add.Opaque {
		t.Fatalf("add: err=%v opaque=%v", err, add.OpaqueReason)
	}
	if len(add.Changes) != 1 || add.Changes[0].Kind != KindAdd || add.Changes[0].Path != "/b" || add.Changes[0].New != "2" {
		t.Fatalf("expected one add at /b (New 2), got %+v", add.Changes)
	}
	if add.Changes[0].NewPos == nil || add.Changes[0].OldPos != nil {
		t.Errorf("add must carry head-side position only, got old=%v new=%v", add.Changes[0].OldPos, add.Changes[0].NewPos)
	}

	del, err := Diff("f.json", []byte(`{"a": 1, "b": 2}`), []byte(`{"a": 1}`))
	if err != nil || del.Opaque {
		t.Fatalf("delete: err=%v opaque=%v", err, del.OpaqueReason)
	}
	if len(del.Changes) != 1 || del.Changes[0].Kind != KindDelete || del.Changes[0].Path != "/b" || del.Changes[0].Old != "2" {
		t.Fatalf("expected one delete at /b (Old 2), got %+v", del.Changes)
	}
}

// A one-sided key whose JSON value is a nested OBJECT (a whole added/removed subtree) stays
// opaque, mirroring YAML — collection add/delete is E1-S05 territory.
func TestJSONOneSidedObjectIsOpaque(t *testing.T) {
	cs, err := Diff("f.json", []byte(`{"a": 1}`), []byte(`{"a": 1, "b": {"x": 1}}`))
	if !cs.Opaque {
		t.Fatalf("one-sided nested object must be opaque, got %+v", cs.Changes)
	}
	if err == nil || !errors.Is(err, ErrOpaque) {
		t.Errorf("expected error wrapping ErrOpaque, got %v", err)
	}
}

// JSON scalar KINDS whose spellings coincide must not collapse (mirrors YAML's
// TestScalarKindChangeNotCollapsed): string "1" vs number 1, string "true" vs bool true, etc.
func TestJSONScalarKindChangeNotCollapsed(t *testing.T) {
	cases := []struct{ name, base, head, wantOld, wantNew string }{
		{"number 1 -> string \"1\"", `{"v": 1}`, `{"v": "1"}`, "1", `"1"`},
		{"bool true -> string \"true\"", `{"v": true}`, `{"v": "true"}`, "true", `"true"`},
		{"null -> string \"null\"", `{"v": null}`, `{"v": "null"}`, "null", `"null"`},
		{"bool false -> bool true", `{"v": false}`, `{"v": true}`, "false", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.json", []byte(tc.base), []byte(tc.head))
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}
			if cs.Opaque {
				t.Fatalf("unexpected opaque: %s", cs.OpaqueReason)
			}
			if len(cs.Changes) != 1 {
				t.Fatalf("%q must be exactly one change (no silent collapse), got %d: %+v", tc.name, len(cs.Changes), cs.Changes)
			}
			if cs.Changes[0].Old != tc.wantOld || cs.Changes[0].New != tc.wantNew {
				t.Errorf("old/new = %q/%q, want %q/%q", cs.Changes[0].Old, cs.Changes[0].New, tc.wantOld, tc.wantNew)
			}
		})
	}
}

// A duplicate key in a JSON object fails closed (encoding/json would silently keep last-wins).
func TestJSONDuplicateKeyIsOpaque(t *testing.T) {
	cs, err := Diff("f.json", []byte(`{"a": 1}`), []byte(`{"a": 1, "a": 2}`))
	if !cs.Opaque {
		t.Fatalf("duplicate JSON key must be opaque, got %+v", cs.Changes)
	}
	if err == nil || !errors.Is(err, ErrOpaque) {
		t.Errorf("expected error wrapping ErrOpaque, got %v", err)
	}
}
