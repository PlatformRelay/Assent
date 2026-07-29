package change

import (
	"bytes"
	"errors"
	"testing"
)

// --- map mode (tfvars workloads, modelled on examples/repos/infra-vars/.../compute.tfvars) ------

const mapBase = `workloads = {
  orders-api = {
    owner        = "orders-team"
    min_replicas = 3
  }
  payments-gateway = {
    owner        = "payments-team"
    min_replicas = 4
  }
}
`

const mapHeadOrdersReplicas = `workloads = {
  orders-api = {
    owner        = "orders-team"
    min_replicas = 5
  }
  payments-gateway = {
    owner        = "payments-team"
    min_replicas = 4
  }
}
`

// REQ-E1-S05-01 — map mode over `workloads`: a change to one workload's min_replicas yields a
// Change tagged EntryRef workload:orders-api alongside its Path, and no other workload is touched.
func TestMapModeEntryRef(t *testing.T) {
	cfg := EntryConfig{Mode: ModeMap, Root: "/workloads", Label: "workload"}
	cs, err := DiffEntries("compute.tfvars", []byte(mapBase), []byte(mapHeadOrdersReplicas), cfg)
	if err != nil {
		t.Fatalf("DiffEntries: %v", err)
	}
	if cs.Opaque {
		t.Fatalf("expected decidable, got opaque: %s", cs.OpaqueReason)
	}
	if len(cs.Changes) != 1 {
		t.Fatalf("expected exactly 1 change (only orders-api touched), got %d: %+v", len(cs.Changes), cs.Changes)
	}
	c := cs.Changes[0]
	if c.Path != "/workloads/orders-api/min_replicas" || c.Kind != KindModify || c.Old != "3" || c.New != "5" {
		t.Errorf("change wrong: %+v (want /workloads/orders-api/min_replicas modify 3->5)", c)
	}
	if c.EntryRef != "workload:orders-api" {
		t.Errorf("EntryRef = %q, want workload:orders-api", c.EntryRef)
	}
}

// --- list mode (JSON services[], modelled on .../service-catalog/.../core-services.json) --------

const listBase = `{"services":[{"name":"orders-api","tier":1},{"name":"payments-gateway","tier":1}]}`
const listHeadTier = `{"services":[{"name":"orders-api","tier":2},{"name":"payments-gateway","tier":1}]}`

// Head reordered vs listBase but with IDENTICAL content per service.
const listReordered = `{"services":[{"name":"payments-gateway","tier":1},{"name":"orders-api","tier":1}]}`

// REQ-E1-S05-02 — list mode keyed by /name: a change to one service's tier yields EntryRef
// service:orders-api (identity-derived). Adversarial: reordering services[] with NO content change
// yields ZERO changes — proving identity-keyed, not index-keyed, comparison.
func TestListModeEntryRefIdentityNotIndex(t *testing.T) {
	cfg := EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name", Label: "service"}

	cs, err := DiffEntries("core-services.json", []byte(listBase), []byte(listHeadTier), cfg)
	if err != nil || cs.Opaque {
		t.Fatalf("tier change: err=%v opaque=%v", err, cs.OpaqueReason)
	}
	if len(cs.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(cs.Changes), cs.Changes)
	}
	c := cs.Changes[0]
	if c.Path != "/services/orders-api/tier" || c.Kind != KindModify || c.Old != "1" || c.New != "2" {
		t.Errorf("change wrong: %+v (want /services/orders-api/tier modify 1->2)", c)
	}
	if c.EntryRef != "service:orders-api" {
		t.Errorf("EntryRef = %q, want service:orders-api (identity-keyed)", c.EntryRef)
	}

	// Adversarial: pure reorder, no content change -> zero changes (identity, not index).
	reord, err := DiffEntries("core-services.json", []byte(listBase), []byte(listReordered), cfg)
	if err != nil || reord.Opaque {
		t.Fatalf("reorder: err=%v opaque=%v", err, reord.OpaqueReason)
	}
	if len(reord.Changes) != 0 {
		t.Fatalf("a pure reorder must yield ZERO changes (identity-keyed), got %d: %+v", len(reord.Changes), reord.Changes)
	}
}

// REQ-E1-S05-03 — a list with no declared identity, or a duplicate identity value across two
// entries, is REJECTED (opaque) — never silently index-keyed or first-wins.
func TestUnkeyedOrDuplicateIdentityRejected(t *testing.T) {
	t.Run("no identity declared", func(t *testing.T) {
		cfg := EntryConfig{Mode: ModeList, Root: "/services", Label: "service"} // Identity unset
		cs, err := DiffEntries("f.json", []byte(listBase), []byte(listHeadTier), cfg)
		if !cs.Opaque {
			t.Fatalf("an unkeyed list must be rejected (opaque), got %+v", cs.Changes)
		}
		if err == nil || !errors.Is(err, ErrOpaque) {
			t.Errorf("expected ErrOpaque, got %v", err)
		}
	})
	t.Run("duplicate identity value", func(t *testing.T) {
		cfg := EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name", Label: "service"}
		dup := `{"services":[{"name":"orders-api","tier":1},{"name":"orders-api","tier":2}]}`
		cs, err := DiffEntries("f.json", []byte(listBase), []byte(dup), cfg)
		if !cs.Opaque {
			t.Fatalf("a duplicate identity must be rejected (opaque), got %+v", cs.Changes)
		}
		if err == nil || !errors.Is(err, ErrOpaque) {
			t.Errorf("expected ErrOpaque, got %v", err)
		}
	})
}

// REQ-E1-S05-03 (remaining reject sub-cases the spec enumerates) — a non-scalar identity, a
// missing identity in one element, and a non-object element must each be rejected fail-closed
// (opaque), never silently keyed.
func TestListModeIdentityRejectSubCases(t *testing.T) {
	cfg := EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name", Label: "service"}
	cases := []struct{ name, head string }{
		{"non-scalar identity (name is an object)", `{"services":[{"name":{"x":1},"tier":1}]}`},
		{"missing identity in one element", `{"services":[{"name":"orders-api","tier":1},{"tier":2}]}`},
		{"non-object element (a bare scalar)", `{"services":[{"name":"orders-api","tier":1},5]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := DiffEntries("f.json", []byte(listBase), []byte(tc.head), cfg)
			if !cs.Opaque {
				t.Fatalf("%q must be rejected (opaque), got %+v", tc.name, cs.Changes)
			}
			if err == nil || !errors.Is(err, ErrOpaque) {
				t.Errorf("expected ErrOpaque, got %v", err)
			}
		})
	}
}

// F1 regression (fail-open the review caught) — list mode over a sequence the producer did NOT
// project (YAML/tfvars leave sequences as opaque leaves) must fail CLOSED, never silently report
// zero entries for a real content change.
func TestListModeUnprojectedSequenceFailsClosed(t *testing.T) {
	cfg := EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name", Label: "service"}
	// A YAML list with a genuine content change (tier 1 -> 99). If list mode silently ranged an
	// empty (unprojected) elems slice, this would wrongly report zero changes.
	base := "services:\n  - name: orders-api\n    tier: 1\n"
	head := "services:\n  - name: orders-api\n    tier: 99\n"
	cs, err := DiffEntries("svc.yaml", []byte(base), []byte(head), cfg)
	if !cs.Opaque {
		t.Fatalf("list mode over an unprojected YAML sequence must fail CLOSED, got opaque=false changes=%+v", cs.Changes)
	}
	if err == nil || !errors.Is(err, ErrOpaque) {
		t.Errorf("expected ErrOpaque, got %v", err)
	}
}

// REQ-E1-S05-05 (map-mode arm) — a nested collection inside a MAP-mode entry (a JSON object entry
// containing an array) is opaque, mirroring the list-mode nested case.
func TestMapModeNestedCollectionFailsSafe(t *testing.T) {
	cfg := EntryConfig{Mode: ModeMap, Root: "/workloads", Label: "workload"}
	base := `{"workloads":{"orders-api":{"tags":["a"]}}}`
	head := `{"workloads":{"orders-api":{"tags":["b"]}}}`
	cs, err := DiffEntries("w.json", []byte(base), []byte(head), cfg)
	if !cs.Opaque {
		t.Fatalf("a nested array inside a map-mode entry must be opaque, got %+v", cs.Changes)
	}
	if err == nil || !errors.Is(err, ErrOpaque) {
		t.Errorf("expected ErrOpaque, got %v", err)
	}
}

// REQ-E1-S05-04 — document mode (the default / empty config) is byte-identical to Diff: this story
// is additive and does not disturb the shipped single-root-mapping walk.
func TestDocumentModeUnchanged(t *testing.T) {
	base := "name: orders\npartitions: 12\n"
	head := "name: orders\npartitions: 24\n"
	got, gerr := DiffEntries("f.yaml", []byte(base), []byte(head), EntryConfig{}) // empty = document
	want, werr := Diff("f.yaml", []byte(base), []byte(head))
	if (gerr == nil) != (werr == nil) {
		t.Fatalf("error mismatch: DiffEntries=%v Diff=%v", gerr, werr)
	}
	if !bytes.Equal(canonicalJSON(t, got), canonicalJSON(t, want)) {
		t.Errorf("document mode diverged from Diff:\n entries: %s\n diff:    %s", canonicalJSON(t, got), canonicalJSON(t, want))
	}
	// Explicit ModeDocument behaves the same.
	got2, _ := DiffEntries("f.yaml", []byte(base), []byte(head), EntryConfig{Mode: ModeDocument})
	if !bytes.Equal(canonicalJSON(t, got2), canonicalJSON(t, want)) {
		t.Errorf("explicit document mode diverged from Diff")
	}
}

// REQ-E1-S05-05 — a nested collection beyond one level (a list-mode entry whose value contains a
// further list) is opaque, never a silent partial derivation — inherited from the shared walker.
func TestNestedCollectionFailsSafe(t *testing.T) {
	cfg := EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name", Label: "service"}
	base := `{"services":[{"name":"orders-api","tags":["a"]}]}`
	head := `{"services":[{"name":"orders-api","tags":["b"]}]}`
	cs, err := DiffEntries("f.json", []byte(base), []byte(head), cfg)
	if !cs.Opaque {
		t.Fatalf("a nested list inside a list entry must be opaque (E1-S05 fence), got %+v", cs.Changes)
	}
	if err == nil || !errors.Is(err, ErrOpaque) {
		t.Errorf("expected ErrOpaque, got %v", err)
	}
}

// REQ-E1-S05-06 — entry derivation is order-independent and deterministic: a double-run is
// byte-identical, and a reordered-but-equal input yields the same (empty) result. (Purity of the
// package is proven by TestCorePurity over internal/change/**.)
func TestEntryRefOrderIndependent(t *testing.T) {
	mapCfg := EntryConfig{Mode: ModeMap, Root: "/workloads", Label: "workload"}
	g1 := canonicalJSON(t, mustDiffEntries(t, "compute.tfvars", mapBase, mapHeadOrdersReplicas, mapCfg))
	g2 := canonicalJSON(t, mustDiffEntries(t, "compute.tfvars", mapBase, mapHeadOrdersReplicas, mapCfg))
	if !bytes.Equal(g1, g2) {
		t.Errorf("map-mode double-run diverged:\n %s\n %s", g1, g2)
	}

	listCfg := EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name", Label: "service"}
	l1 := canonicalJSON(t, mustDiffEntries(t, "s.json", listBase, listHeadTier, listCfg))
	l2 := canonicalJSON(t, mustDiffEntries(t, "s.json", listBase, listHeadTier, listCfg))
	if !bytes.Equal(l1, l2) {
		t.Errorf("list-mode double-run diverged:\n %s\n %s", l1, l2)
	}
}

// A whole entry added or removed from a keyed collection is reported as a single structural
// add/delete Change at the entry pointer, tagged with the entry's EntryRef (map and list mode).
func TestEntryOneSidedAddDelete(t *testing.T) {
	t.Run("map mode: workload added", func(t *testing.T) {
		head := `workloads = {
  orders-api = {
    owner        = "orders-team"
    min_replicas = 3
  }
  payments-gateway = {
    owner        = "payments-team"
    min_replicas = 4
  }
  inventory = {
    owner        = "inventory-team"
    min_replicas = 2
  }
}
`
		cfg := EntryConfig{Mode: ModeMap, Root: "/workloads", Label: "workload"}
		cs, err := DiffEntries("compute.tfvars", []byte(mapBase), []byte(head), cfg)
		if err != nil || cs.Opaque {
			t.Fatalf("map add: err=%v opaque=%v", err, cs.OpaqueReason)
		}
		if len(cs.Changes) != 1 {
			t.Fatalf("expected one entry-add change, got %+v", cs.Changes)
		}
		c := cs.Changes[0]
		if c.Kind != KindAdd || c.Path != "/workloads/inventory" || c.EntryRef != "workload:inventory" {
			t.Errorf("entry add wrong: %+v (want add /workloads/inventory, EntryRef workload:inventory)", c)
		}
		if c.NewPos == nil || c.OldPos != nil {
			t.Errorf("entry add must carry head-side position only, got old=%v new=%v", c.OldPos, c.NewPos)
		}
	})

	t.Run("list mode: service removed", func(t *testing.T) {
		// listBase has orders-api + payments-gateway; head drops payments-gateway.
		head := `{"services":[{"name":"orders-api","tier":1}]}`
		cfg := EntryConfig{Mode: ModeList, Root: "/services", Identity: "/name", Label: "service"}
		cs, err := DiffEntries("s.json", []byte(listBase), []byte(head), cfg)
		if err != nil || cs.Opaque {
			t.Fatalf("list delete: err=%v opaque=%v", err, cs.OpaqueReason)
		}
		if len(cs.Changes) != 1 {
			t.Fatalf("expected one entry-delete change, got %+v", cs.Changes)
		}
		c := cs.Changes[0]
		if c.Kind != KindDelete || c.Path != "/services/payments-gateway" || c.EntryRef != "service:payments-gateway" {
			t.Errorf("entry delete wrong: %+v (want delete /services/payments-gateway)", c)
		}
	})
}

func mustDiffEntries(t *testing.T, file, base, head string, cfg EntryConfig) ChangeSet {
	t.Helper()
	cs, err := DiffEntries(file, []byte(base), []byte(head), cfg)
	if err != nil {
		t.Fatalf("DiffEntries(%s): %v", cfg.Mode, err)
	}
	return cs
}
