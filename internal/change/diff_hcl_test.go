package change

import (
	"bytes"
	"errors"
	"testing"
)

// Fixtures modelled on examples/repos/infra-vars/envs/prod/compute.tfvars: a `workloads` map of
// keyed objects. Trimmed to one workload so the change is unambiguous.
const hclBase = `workloads = {
  orders-api = {
    owner        = "orders-team"
    min_replicas = 3
  }
}
`

const hclHeadMinReplicas = `workloads = {
  orders-api = {
    owner        = "orders-team"
    min_replicas = 4
  }
}
`

const hclHeadOwner = `workloads = {
  orders-api = {
    owner        = "platform-team"
    min_replicas = 3
  }
}
`

// REQ-E1-S04-01 — a tfvars literal value changed inside a nested object produces the SAME Change
// shape (Kind:modify, pointer Path, tag-qualified Old/New, positions) the YAML/JSON adapters do.
func TestHCLAdapterModify(t *testing.T) {
	t.Run("nested number min_replicas 3->4", func(t *testing.T) {
		cs, err := Diff("compute.tfvars", []byte(hclBase), []byte(hclHeadMinReplicas))
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
		if c.File != "compute.tfvars" || c.Path != "/workloads/orders-api/min_replicas" || c.Kind != KindModify {
			t.Errorf("File/Path/Kind = %q/%q/%q, want compute.tfvars//workloads/orders-api/min_replicas/modify", c.File, c.Path, c.Kind)
		}
		if c.Old != "3" || c.New != "4" {
			t.Errorf("Old/New = %q/%q, want raw literals 3 -> 4", c.Old, c.New)
		}
		if c.OldPos == nil || c.NewPos == nil {
			t.Fatalf("modify must carry positions on both sides, got old=%v new=%v", c.OldPos, c.NewPos)
		}
		if c.OldPos.Line != 4 || c.NewPos.Line != 4 {
			t.Errorf("min_replicas value is on line 4, got old=%+v new=%+v", c.OldPos, c.NewPos)
		}
	})

	t.Run("nested string owner", func(t *testing.T) {
		cs, err := Diff("compute.tfvars", []byte(hclBase), []byte(hclHeadOwner))
		if err != nil || cs.Opaque {
			t.Fatalf("diff failed: err=%v opaque=%v", err, cs.OpaqueReason)
		}
		if len(cs.Changes) != 1 {
			t.Fatalf("expected 1 change, got %+v", cs.Changes)
		}
		c := cs.Changes[0]
		if c.Path != "/workloads/orders-api/owner" || c.Old != `"orders-team"` || c.New != `"platform-team"` {
			t.Errorf("owner change wrong: %+v (want /workloads/orders-api/owner, quoted strings)", c)
		}
	})
}

// REQ-E1-S04-02 — a tfvars value expressed as a NON-LITERAL HCL expression (interpolation,
// function call, variable reference) is opaque with a reason naming the construct — never silently
// evaluated (which could hide the real value) and never silently dropped (fail-safe).
func TestHCLNonLiteralFailsSafe(t *testing.T) {
	cases := []struct{ name, head string }{
		{"interpolation", `workloads = {` + "\n" + `  orders-api = { owner = "${var.team}" }` + "\n" + `}` + "\n"},
		{"variable reference", `workloads = {` + "\n" + `  orders-api = { owner = var.team }` + "\n" + `}` + "\n"},
		{"function call", `workloads = {` + "\n" + `  orders-api = { owner = upper("x") }` + "\n" + `}` + "\n"},
		{"template wrap", `region = "${var.region}"` + "\n"},
		{"arithmetic", `count = 1 + 2` + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Diff a valid literal base against the non-literal head.
			cs, err := Diff("f.tfvars", []byte(hclBase), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("non-literal %q must be opaque (never silently evaluated), got %d changes: %+v", tc.name, len(cs.Changes), cs.Changes)
			}
			if cs.OpaqueReason == "" {
				t.Errorf("opaque must carry a reason naming the unsupported construct")
			}
			if len(cs.Changes) != 0 {
				t.Errorf("opaque result must carry no partial changes, got %d", len(cs.Changes))
			}
			if err == nil || !errors.Is(err, ErrOpaque) {
				t.Errorf("expected error wrapping ErrOpaque, got %v", err)
			}
		})
	}
}

// REQ-E1-S04-03 — malformed HCL bytes fail opaque with a non-empty reason and zero partial changes.
func TestHCLOpaqueFailsSafe(t *testing.T) {
	cases := []struct{ name, head string }{
		{"unterminated object", `workloads = {` + "\n" + `  orders-api = {` + "\n"},
		{"garbage tokens", `= = = @@@`},
		{"bare value no attribute", `just_a_bareword_no_equals`},
		{"empty", ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.tfvars", []byte(hclBase), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("malformed %q must be opaque, got %d changes", tc.name, len(cs.Changes))
			}
			if len(cs.Changes) != 0 {
				t.Errorf("opaque must carry no changes, got %d", len(cs.Changes))
			}
			if err == nil || !errors.Is(err, ErrOpaque) {
				t.Errorf("expected error wrapping ErrOpaque, got %v", err)
			}
		})
	}
}

// A tfvars list/tuple value is a vSequence leaf -> opaque (list walking is E1-S05), mirroring the
// YAML/JSON adapters' sequence handling.
func TestHCLTupleIsOpaque(t *testing.T) {
	base := `tags = "a"` + "\n"
	head := `tags = ["a", "b"]` + "\n"
	cs, err := Diff("f.tfvars", []byte(base), []byte(head))
	if !cs.Opaque {
		t.Fatalf("a tfvars tuple must be opaque (E1-S05 territory), got %+v", cs.Changes)
	}
	if err == nil || !errors.Is(err, ErrOpaque) {
		t.Errorf("expected ErrOpaque, got %v", err)
	}
}

// A scalar KIND change (number -> quoted string) must not collapse, mirroring YAML/JSON.
func TestHCLScalarKindChangeNotCollapsed(t *testing.T) {
	base := `n = 3` + "\n"
	head := `n = "3"` + "\n"
	cs, err := Diff("f.tfvars", []byte(base), []byte(head))
	if err != nil || cs.Opaque {
		t.Fatalf("diff failed: err=%v opaque=%v", err, cs.OpaqueReason)
	}
	if len(cs.Changes) != 1 || cs.Changes[0].Old != "3" || cs.Changes[0].New != `"3"` {
		t.Fatalf("kind change must be detected: got %+v (want Old 3 -> New \"3\")", cs.Changes)
	}
}

// REQ-E1-S04-01 (bool + null literals) — bool and null are in-scope literal scalars (the story
// excludes only .tf blocks, not bool/null); each must render injectively and be detected as a
// change, never silently collapsed. These lock the hclScalar bool/null branches.
func TestHCLBoolAndNullLiterals(t *testing.T) {
	t.Run("bool true -> false", func(t *testing.T) {
		cs, err := Diff("f.tfvars", []byte("enabled = true\n"), []byte("enabled = false\n"))
		if err != nil || cs.Opaque {
			t.Fatalf("diff failed: err=%v opaque=%v", err, cs.OpaqueReason)
		}
		if len(cs.Changes) != 1 || cs.Changes[0].Old != "true" || cs.Changes[0].New != "false" {
			t.Fatalf("bool change wrong: %+v (want Old true -> New false)", cs.Changes)
		}
	})
	t.Run("null -> string", func(t *testing.T) {
		cs, err := Diff("f.tfvars", []byte("owner = null\n"), []byte(`owner = "team"`+"\n"))
		if err != nil || cs.Opaque {
			t.Fatalf("diff failed: err=%v opaque=%v", err, cs.OpaqueReason)
		}
		if len(cs.Changes) != 1 || cs.Changes[0].Old != "null" || cs.Changes[0].New != `"team"` {
			t.Fatalf("null change wrong: %+v (want Old null -> New \"team\")", cs.Changes)
		}
	})
	t.Run("null and quoted-null string do not collapse", func(t *testing.T) {
		cs, err := Diff("f.tfvars", []byte("owner = null\n"), []byte(`owner = "null"`+"\n"))
		if err != nil || cs.Opaque {
			t.Fatalf("diff failed: err=%v opaque=%v", err, cs.OpaqueReason)
		}
		if len(cs.Changes) != 1 || cs.Changes[0].Old != "null" || cs.Changes[0].New != `"null"` {
			t.Fatalf("null vs \"null\" must be a detected change (kind discrimination), got %+v", cs.Changes)
		}
	})
}

// REQ-E1-S04-03 (fail-safe guards) — HCL blocks (a .tf resource-block shape, not tfvars),
// duplicate object keys (nested AND top-level), and a computed/non-literal object key must all
// fail opaque with a reason and zero partial changes. Locks the parseHCL block guard, the
// hclExprToNode duplicate-key guard, and the hclKeyName computed-key branch.
func TestHCLStructuralGuardsFailSafe(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{
			name: "hcl block (a .tf shape, not tfvars)",
			base: hclBase,
			head: "resource \"aws_s3_bucket\" \"b\" {\n  bucket = \"x\"\n}\n",
		},
		{
			name: "duplicate nested object key",
			base: hclBase,
			head: "workloads = {\n  orders-api = {\n    owner = \"a\"\n    owner = \"b\"\n  }\n}\n",
		},
		{
			name: "duplicate top-level attribute",
			base: hclBase,
			head: "region = \"eu\"\nregion = \"us\"\n",
		},
		{
			name: "computed (non-literal) object key",
			base: hclBase,
			head: "m = {\n  (var.k) = 1\n}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := Diff("f.tfvars", []byte(tc.base), []byte(tc.head))
			if !cs.Opaque {
				t.Fatalf("%q must be opaque, got %d changes: %+v", tc.name, len(cs.Changes), cs.Changes)
			}
			if cs.OpaqueReason == "" {
				t.Errorf("opaque must carry a reason")
			}
			if len(cs.Changes) != 0 {
				t.Errorf("opaque must carry no partial changes, got %d", len(cs.Changes))
			}
			if err == nil || !errors.Is(err, ErrOpaque) {
				t.Errorf("expected ErrOpaque, got %v", err)
			}
		})
	}
}

// REQ-E1-S04-04 — the HCL path is deterministic: every fixture double-runs byte-identical.
func TestHCLAdapterDoubleRunStable(t *testing.T) {
	cases := []struct{ name, base, head string }{
		{"number modify", hclBase, hclHeadMinReplicas},
		{"string modify", hclBase, hclHeadOwner},
		{"opaque non-literal", hclBase, `workloads = { orders-api = { owner = var.team } }` + "\n"},
		{"opaque malformed", hclBase, `= = =`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs1, _ := Diff("f.tfvars", []byte(tc.base), []byte(tc.head))
			cs2, _ := Diff("f.tfvars", []byte(tc.base), []byte(tc.head))
			if !bytes.Equal(canonicalJSON(t, cs1), canonicalJSON(t, cs2)) {
				t.Errorf("double-run diverged:\n run1: %s\n run2: %s", canonicalJSON(t, cs1), canonicalJSON(t, cs2))
			}
			if cs1.OpaqueReason != cs2.OpaqueReason {
				t.Errorf("opaque reason not stable: %q vs %q", cs1.OpaqueReason, cs2.OpaqueReason)
			}
		})
	}
}
