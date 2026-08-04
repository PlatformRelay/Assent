package change

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestEntriesListModeKeyedByIdentity proves Entries reconstructs each list-mode
// entry as its FULL typed object, keyed by the SAME `<label>:<identity>` EntryRef
// DiffEntries tags changes with — so a harness can bind an entry-scoped predicate
// against exactly the entry a change was attributed to (single source of truth, no
// drift).
func TestEntriesListModeKeyedByIdentity(t *testing.T) {
	data := []byte(`{"services":[{"name":"orders","owner":"team-a","replicas":2},{"name":"users","owner":"team-b","replicas":5}]}`)
	cfg := EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name", Label: "svc"}

	got, err := Entries("config.json", data, cfg)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}

	orders, ok := got["svc:orders"]
	if !ok {
		t.Fatalf("missing entry svc:orders; got keys %v", keysOf(got))
	}
	want := map[string]any{"name": "orders", "owner": "team-a", "replicas": json.Number("2")}
	if !reflect.DeepEqual(orders, want) {
		t.Fatalf("svc:orders = %#v, want %#v", orders, want)
	}

	// Keys must match exactly what DiffEntries tags a change with (a modify inside
	// the orders entry): no drift between the reconstructed object and the change.
	cs, err := DiffEntries("config.json", data,
		[]byte(`{"services":[{"name":"orders","owner":"team-a","replicas":3},{"name":"users","owner":"team-b","replicas":5}]}`), cfg)
	if err != nil {
		t.Fatalf("DiffEntries: %v", err)
	}
	if len(cs.Changes) == 0 {
		t.Fatal("expected a change inside svc:orders")
	}
	for _, ch := range cs.Changes {
		if _, ok := got[ch.EntryRef]; !ok {
			t.Fatalf("change EntryRef %q has no reconstructed entry (keys %v)", ch.EntryRef, keysOf(got))
		}
	}
}

// TestEntriesMapModeKeyedByKey proves map mode keys each entry by its mapping key
// (`<label>:<key>`), the same ref DiffEntries builds.
func TestEntriesMapModeKeyedByKey(t *testing.T) {
	data := []byte("services:\n  orders:\n    owner: team-a\n    replicas: 2\n")
	cfg := EntryConfig{Mode: ModeMap, Root: "/services", Label: "svc"}

	got, err := Entries("config.yaml", data, cfg)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	orders, ok := got["svc:orders"]
	if !ok {
		t.Fatalf("missing svc:orders; keys %v", keysOf(got))
	}
	want := map[string]any{"owner": "team-a", "replicas": json.Number("2")}
	if !reflect.DeepEqual(orders, want) {
		t.Fatalf("svc:orders = %#v, want %#v", orders, want)
	}
}

// TestEntriesAllOrNothingProjection is the REQ-E6-S02-07 crux at the source: an
// entry that cannot be FULLY projected (a YAML map-mode entry carrying a nested
// sequence its producer leaves unprojected) must return an ERROR — never a partial
// map with the sequence field silently dropped. A partial/empty entry would let
// `has(entry.x)` return false cleanly and take a more-permissive branch; the error
// forces the caller to leave Entry nil (scalar fallback -> fail-safe REVIEW).
func TestEntriesAllOrNothingProjection(t *testing.T) {
	// The `orders` entry has an owner (projectable) AND a `tags` sequence the YAML
	// producer does not project -> projection must fail for the WHOLE entry.
	data := []byte("services:\n  orders:\n    owner: team-a\n    tags: [a, b]\n")
	cfg := EntryConfig{Mode: ModeMap, Root: "/services", Label: "svc"}

	got, err := Entries("config.yaml", data, cfg)
	if err == nil {
		t.Fatalf("expected an all-or-nothing projection error, got entry %#v", got["svc:orders"])
	}
	if got != nil {
		t.Fatalf("a failed projection must return a nil map, not a partial one: %#v", got)
	}
}

// TestEntriesRejectsBadIdentityLikeDiffEntries proves the keying rejections are
// inherited from DiffEntries (single source of truth): a duplicate identity is
// rejected here exactly as DiffEntries rejects it (never first-wins).
func TestEntriesRejectsBadIdentityLikeDiffEntries(t *testing.T) {
	data := []byte(`{"services":[{"name":"dup","owner":"a"},{"name":"dup","owner":"b"}]}`)
	cfg := EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name", Label: "svc"}
	if _, err := Entries("config.json", data, cfg); err == nil {
		t.Fatal("expected a duplicate-identity rejection (never first-wins)")
	}
}

// TestEntriesDocumentModeEmpty proves document mode has no per-entry decomposition
// (DiffEntries tags no EntryRef there) -> an empty map, so a caller keeps the
// scalar binding for document-mode changes.
func TestEntriesDocumentModeEmpty(t *testing.T) {
	got, err := Entries("config.json", []byte(`{"a":1}`), EntryConfig{Mode: ModeDocument})
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("document mode must yield no entries, got %#v", got)
	}
}

// TestEntriesProjectsEveryScalarType exercises every projectScalar tag (number,
// string, bool, null) plus nested object and nested projected array (JSON) so a
// full entry is reconstructed with typed, injective values.
func TestEntriesProjectsEveryScalarType(t *testing.T) {
	data := []byte(`{"items":[{"id":"x","n":7,"s":"hi","b":true,"z":null,"nested":{"k":1},"arr":[1,2]}]}`)
	cfg := EntryConfig{Mode: ModeList, Root: "/items", Identity: "/id", Label: "it"}
	got, err := Entries("config.json", data, cfg)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	entry, ok := got["it:x"].(map[string]any)
	if !ok {
		t.Fatalf("it:x = %#v, want map", got["it:x"])
	}
	want := map[string]any{
		"id":     "x",
		"n":      json.Number("7"),
		"s":      "hi",
		"b":      true,
		"z":      nil,
		"nested": map[string]any{"k": json.Number("1")},
		"arr":    []any{json.Number("1"), json.Number("2")},
	}
	if !reflect.DeepEqual(entry, want) {
		t.Fatalf("entry = %#v, want %#v", entry, want)
	}
}

// TestEntriesErrorPaths locks the fail-safe rejections: a missing root, a wrong-kind
// root, a list without an identity pointer, and an unknown mode all return an error
// and a nil map (never a partial reconstruction).
func TestEntriesErrorPaths(t *testing.T) {
	jsonData := []byte(`{"services":[{"name":"a"}]}`)
	cases := []struct {
		name string
		file string
		data []byte
		cfg  EntryConfig
	}{
		{"missing root", "config.json", jsonData, EntryConfig{Mode: ModeList, Root: "/nope", Identity: "/name", Label: "svc"}},
		{"list root not a sequence", "config.json", []byte(`{"services":{"a":1}}`), EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name"}},
		{"map root not a mapping", "config.json", jsonData, EntryConfig{Mode: ModeMap, Root: "/services"}},
		{"list without identity", "config.json", jsonData, EntryConfig{Mode: ModeList, Root: "/services"}},
		{"unknown mode", "config.json", jsonData, EntryConfig{Mode: EntryMode("bogus"), Root: "/services"}},
		{"opaque input", "config.json", []byte(`not json`), EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Entries(tc.file, tc.data, tc.cfg)
			if err == nil {
				t.Fatalf("expected an error, got %#v", got)
			}
			if got != nil {
				t.Fatalf("a rejected reconstruction must return nil, got %#v", got)
			}
		})
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
