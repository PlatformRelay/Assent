package adoptertest

// White-box tests for the REQ-E6-S02-07 fail-safety seam: they reach the unexported
// populateEntries to prove the "FULL entry object OR nil" dichotomy directly — a
// partial/empty entry is NEVER bound (the Part-A review F2 carry-forward).

import (
	"encoding/json"
	"testing"

	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/evaldecode"
)

// TestPartialEntryNeverBindsPermissively (REQ-E6-S02-07) proves the harness either
// binds the FULL entry object or leaves Entry nil — never a partial/empty map. An
// empty/partial map is the fail-OPEN the Part-A review flagged: `has(entry.x)` on it
// returns false CLEANLY (a `has(entry.x) ? … : true` predicate would take the
// more-permissive branch), whereas leaving Entry nil falls back to the scalar
// binding on which the selection ERRORS → fail-safe REVIEW.
func TestPartialEntryNeverBindsPermissively(t *testing.T) {
	// (a) A FULLY reconstructable list-mode entry populates EVERY field.
	t.Run("full entry binds every field", func(t *testing.T) {
		cfg := change.EntryConfig{Mode: change.ModeList, Root: "/services", Identity: "/name", Label: "svc"}
		c := Case{
			File: "config.json",
			Base: []byte(`{"services":[{"name":"orders","owner":"team-a","replicas":2}]}`),
			Head: []byte(`{"services":[{"name":"orders","owner":"team-a","replicas":3}]}`),
		}
		cs, err := change.DiffEntries(c.File, c.Base, c.Head, cfg)
		if err != nil {
			t.Fatalf("DiffEntries: %v", err)
		}
		in := evaldecode.BuildEvaluationInput(cs, aggregate.MR{}, nil)
		populateEntries(&in, cs, c, cfg)

		if len(in.ChangeSet.Changes) == 0 {
			t.Fatal("expected a change")
		}
		entry, ok := in.ChangeSet.Changes[0].Entry.(map[string]any)
		if !ok {
			t.Fatalf("Entry = %#v, want a full object map", in.ChangeSet.Changes[0].Entry)
		}
		// Every field of the head entry is present (full, not partial).
		for _, k := range []string{"name", "owner", "replicas"} {
			if _, present := entry[k]; !present {
				t.Fatalf("reconstructed entry missing field %q (partial entry): %#v", k, entry)
			}
		}
		if entry["owner"] != "team-a" || entry["replicas"] != json.Number("3") {
			t.Fatalf("entry field values wrong: %#v", entry)
		}
	})

	// (b) An entry that cannot be FULLY reconstructed (a map-mode YAML entry with a
	// nested, unprojectable sequence) leaves Entry nil — NEVER a partial/empty map.
	t.Run("unreconstructable entry leaves Entry nil", func(t *testing.T) {
		cfg := change.EntryConfig{Mode: change.ModeMap, Root: "/services", Label: "svc"}
		c := Case{
			File: "config.yaml",
			Base: []byte("services:\n  orders:\n    owner: team-a\n    tags: [a, b]\n"),
			Head: []byte("services:\n  orders:\n    owner: team-b\n    tags: [a, b]\n"),
		}
		// change.Entries must error on this (the nested sequence is unprojectable),
		// confirming the precondition of the fail-safe branch.
		if _, err := change.Entries(c.File, c.Head, cfg); err == nil {
			t.Fatal("precondition: expected change.Entries to fail on an unprojectable entry")
		}

		// Build a changeset carrying the svc:orders EntryRef so populateEntries has a
		// change to (not) populate. A hand-built change suffices — the point is the
		// nil-fallback, independent of what the differ produced.
		cs := change.ChangeSet{Changes: []change.Change{{
			File: c.File, Path: "/services/orders/owner", Kind: change.KindModify,
			EntryRef: "svc:orders", Old: `"team-a"`, New: `"team-b"`,
		}}}
		in := evaldecode.BuildEvaluationInput(cs, aggregate.MR{}, nil)
		populateEntries(&in, cs, c, cfg)

		if in.ChangeSet.Changes[0].Entry != nil {
			t.Fatalf("unreconstructable entry must leave Entry nil (scalar fallback), got %#v", in.ChangeSet.Changes[0].Entry)
		}
		if in.ChangeSet.Changes[0].OldEntry != nil {
			t.Fatalf("unreconstructable entry must leave OldEntry nil, got %#v", in.ChangeSet.Changes[0].OldEntry)
		}
	})

	// (b2) The REQ-E6-S02-07 case in its LITERAL framing: a mode:list entry whose
	// reconstruction cannot be completed (a duplicate identity — change.Entries
	// rejects it exactly as DiffEntries does) leaves Entry nil, never a partial map.
	t.Run("list-mode unreconstructable entry leaves Entry nil", func(t *testing.T) {
		cfg := change.EntryConfig{Mode: change.ModeList, Root: "/services", Identity: "/name", Label: "svc"}
		c := Case{
			File: "config.json",
			Base: []byte(`{"services":[{"name":"dup","owner":"a"},{"name":"dup","owner":"b"}]}`),
			Head: []byte(`{"services":[{"name":"dup","owner":"c"},{"name":"dup","owner":"d"}]}`),
		}
		if _, err := change.Entries(c.File, c.Head, cfg); err == nil {
			t.Fatal("precondition: a duplicate list identity must reject reconstruction")
		}
		cs := change.ChangeSet{Changes: []change.Change{{
			File: c.File, Path: "/services/dup/owner", Kind: change.KindModify,
			EntryRef: "svc:dup", Old: `"a"`, New: `"c"`,
		}}}
		in := evaldecode.BuildEvaluationInput(cs, aggregate.MR{}, nil)
		populateEntries(&in, cs, c, cfg)
		if in.ChangeSet.Changes[0].Entry != nil {
			t.Fatalf("list-mode unreconstructable entry must leave Entry nil, got %#v", in.ChangeSet.Changes[0].Entry)
		}
	})

	// (c) A one-sided entry (a delete: present on base, absent on head) leaves the
	// ABSENT side nil — never substitutes an empty {} object.
	t.Run("one-sided entry leaves absent side nil", func(t *testing.T) {
		cfg := change.EntryConfig{Mode: change.ModeList, Root: "/services", Identity: "/name", Label: "svc"}
		c := Case{
			File: "config.json",
			Base: []byte(`{"services":[{"name":"orders","owner":"team-a"},{"name":"gone","owner":"team-b"}]}`),
			Head: []byte(`{"services":[{"name":"orders","owner":"team-a"}]}`),
		}
		cs, err := change.DiffEntries(c.File, c.Base, c.Head, cfg)
		if err != nil {
			t.Fatalf("DiffEntries: %v", err)
		}
		in := evaldecode.BuildEvaluationInput(cs, aggregate.MR{}, nil)
		populateEntries(&in, cs, c, cfg)

		for i := range in.ChangeSet.Changes {
			if in.ChangeSet.Changes[i].Subject == "svc:gone" {
				// Deleted on head: Entry (head side) must be nil, never {}.
				if in.ChangeSet.Changes[i].Entry != nil {
					t.Fatalf("deleted entry must have nil head Entry, got %#v", in.ChangeSet.Changes[i].Entry)
				}
				return
			}
		}
		t.Fatal("expected a svc:gone delete change")
	})
}
