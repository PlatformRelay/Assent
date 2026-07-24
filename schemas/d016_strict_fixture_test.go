package schemas

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// d016FixtureDir is the strict §8 exit-gate fixture directory (P3-E1-S07).
// It holds the one strict, versioned end-to-end fixture ADR-0017 §8 requires.
const d016FixtureDir = "../examples/contracts/d016-strict-fixture"

// readD016Fixture loads a fixture file and parses it into the any-tree shape
// jsonschema.Schema.Validate expects (json.Number for numbers).
func readD016Fixture(t *testing.T, name string) any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(d016FixtureDir, name)) //nolint:gosec // fixed test-fixture tree
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return doc
}

// TestD016StrictFixture is REQ-P3-E1-S07-01: every document of the strict
// exit-gate fixture validates against its owning S01–S05 schema. The fixture
// embodies ADR-0017 §8's six required elements:
//
//	(a) pinned target + merge-result digest (decision-record.json pins),
//	(b) a multi-entry document with one EntryRef renamed base→head while
//	    identity.pointer proves stable identity (evaluation-input.json +
//	    merge-policy.json entries.identity),
//	(c) exactly two obligations independently required by the binding, each
//	    proved by a distinct rule (ruleset-binding.json require + merge-policy
//	    two prove rules),
//	(d) one typed fact in state "expired" (evaluation-input.json facts),
//	(e) a missing required ApprovalEvidence for a require-review binding
//	    (decision-record.json require-review finding, no ApprovalEvidence).
func TestD016StrictFixture(t *testing.T) {
	cases := map[string]struct {
		file   string
		schema *jsonschema.Schema
	}{
		"RulesetBinding":     {"ruleset-binding.json", RulesetBindingSchema},
		"MergePolicy":        {"merge-policy.json", MergePolicySchema},
		"EvaluationInput":    {"evaluation-input.json", EvaluationInputSchema},
		"DecisionRecord":     {"decision-record.json", DecisionRecordSchema},
		"PresentationModel":  {"presentation-model.json", PresentationModelSchema},
		"PublicationReceipt": {"publication-receipt.json", PublicationReceiptSchema},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			doc := readD016Fixture(t, tc.file)
			if err := tc.schema.Validate(doc); err != nil {
				t.Fatalf("%s fixture fails %s schema validation: %v", tc.file, name, err)
			}
		})
	}

	t.Run("(a) pinned target + merge-result digest", func(t *testing.T) {
		pins := fixtureObj(t, "decision-record.json")["pins"].(map[string]any)
		if s, _ := pins["targetSha"].(string); s == "" {
			t.Error("pins.targetSha must be populated (ADR-0017 §1 target pin)")
		}
		if _, ok := pins["mergeResultDigest"].(string); !ok {
			t.Error("pins.mergeResultDigest must be a populated string (pinned merge result), not null")
		}
		if _, hasGap := pins["capabilityGap"]; hasGap {
			t.Error("pins.capabilityGap must be absent when mergeResultDigest is pinned (never a silent widening)")
		}
	})

	t.Run("(b) renamed entry with stable identity in a multi-entry document", func(t *testing.T) {
		changes := changeSetChanges(t)
		if len(changes) < 2 {
			t.Fatalf("multi-entry document requires >= 2 changes, got %d", len(changes))
		}
		var renamed map[string]any
		for _, c := range changes {
			cm := c.(map[string]any)
			if cm["kind"] == "rename" {
				renamed = cm
			}
		}
		if renamed == nil {
			t.Fatal("expected one change with kind: rename (base->head rename)")
		}

		// The pack declares identity is content-derived: entries.identity.pointer
		// points INTO each document, not at its filename (ADR-0017 §5 — why
		// filename-derived identity is wrong).
		entries := mergePolicyEntries(t)
		if len(entries) == 0 {
			t.Fatal("merge-policy must declare entries with identity.pointer proving stable identity")
		}
		var idPointer string
		for _, spec := range entries {
			sm := spec.(map[string]any)
			if id, ok := sm["identity"].(map[string]any); ok {
				if ptr, _ := id["pointer"].(string); ptr != "" {
					idPointer = ptr
				}
			}
		}
		if idPointer == "" {
			t.Fatal("merge-policy entries must declare identity.pointer (content-derived stable identity)")
		}

		// The rename change carries whole-file (path: "") base/head document
		// content in old/new. BOTH the filename/path AND the head content change,
		// yet the value at identity.pointer is the SAME in base and head — that is
		// what makes the governed subject stable across the rename.
		oldDoc, okOld := renamed["old"].(map[string]any)
		newDoc, okNew := renamed["new"].(map[string]any)
		if !okOld || !okNew {
			t.Fatal("rename change must carry whole-file base/head document content in old/new")
		}
		oldName := valueAtPointer(oldDoc, idPointer)
		newName := valueAtPointer(newDoc, idPointer)
		if oldName == "" || newName == "" {
			t.Fatalf("identity.pointer %q must resolve in both base and head docs, got base=%q head=%q", idPointer, oldName, newName)
		}
		if oldName != newName {
			t.Errorf("identity at %q must be stable across the rename, got base=%q head=%q", idPointer, oldName, newName)
		}

		// The resolved subject EntryRef's identity part must equal the
		// content-derived name — NOT either filename stem. Both stems here
		// (legacy-orders, orders-events) deliberately differ from the
		// /metadata/name (orders.events.v1), so a filename-derived identity
		// would produce a different EntryRef and this assertion would fail.
		subject, _ := renamed["subject"].(string)
		wantSubject := "topic-registry:" + newName
		if subject != wantSubject {
			t.Errorf("subject must track content identity %q, got %q", wantSubject, subject)
		}
		oldStem := fileStem(renamed, oldDoc)
		newStem := fileStem(renamed, newDoc)
		if oldStem == newName || newStem == newName {
			t.Errorf("test setup weak: a filename stem equals the content identity %q (base stem=%q head stem=%q) — identity would be indistinguishable from filename-derived", newName, oldStem, newStem)
		}
	})

	t.Run("(c) exactly two obligations independently required, two distinct proving rules", func(t *testing.T) {
		binding := fixtureObj(t, "ruleset-binding.json")
		bindings := binding["bindings"].([]any)
		var require []any
		for _, b := range bindings {
			bm := b.(map[string]any)
			if r, ok := bm["require"].([]any); ok && len(r) > 0 {
				require = r
			}
		}
		if len(require) != 2 {
			t.Fatalf("binding must independently require exactly two obligations, got %v", require)
		}
		// Two distinct proving rules — one prove.obligation per required name.
		proved := map[string]int{}
		for _, rule := range mergePolicyRules(t) {
			rm := rule.(map[string]any)
			if p, ok := rm["prove"].(map[string]any); ok {
				if ob, _ := p["obligation"].(string); ob != "" {
					proved[ob]++
				}
			}
		}
		for _, req := range require {
			name := req.(string)
			if proved[name] == 0 {
				t.Errorf("required obligation %q has no proving rule (lint hard error)", name)
			}
		}
		if len(proved) < 2 {
			t.Errorf("expected two distinct proving rules, got %d obligation-proving rules", len(proved))
		}
	})

	t.Run("(d) one typed fact in state expired", func(t *testing.T) {
		facts := fixtureObj(t, "evaluation-input.json")["facts"].(map[string]any)
		expiredCount := 0
		for _, provider := range facts {
			for _, fact := range provider.(map[string]any) {
				fm := fact.(map[string]any)
				if fm["state"] == "expired" {
					expiredCount++
					if _, hasVal := fm["value"]; hasVal {
						t.Error("an expired fact must omit value (schema else-branch)")
					}
					obs, _ := fm["observedAt"].(string)
					exp, _ := fm["expiresAt"].(string)
					if exp != "" && obs != "" && exp >= obs {
						t.Errorf("expired fact expiresAt (%s) must be before observedAt (%s)", exp, obs)
					}
				}
			}
		}
		if expiredCount != 1 {
			t.Errorf("expected exactly one fact in state expired, got %d", expiredCount)
		}
	})

	t.Run("(e) missing required ApprovalEvidence: a require-review finding, no ApprovalEvidence file", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(d016FixtureDir, "approval-evidence.json")); err == nil {
			t.Fatal("strict fixture must NOT carry an ApprovalEvidence — the required approval is missing by construction")
		}
		findingsObj := fixtureObj(t, "decision-record.json")["findings"].(map[string]any)
		findings := findingsObj["enforcing"].([]any)
		found := false
		for _, f := range findings {
			if f.(map[string]any)["effect"] == "require-review" {
				found = true
			}
		}
		if !found {
			t.Error("expected a require-review finding representing the unproven required approval")
		}
	})
}

// helpers ---------------------------------------------------------------

func fixtureObj(t *testing.T, name string) map[string]any {
	t.Helper()
	return readD016Fixture(t, name).(map[string]any)
}

func changeSetChanges(t *testing.T) []any {
	t.Helper()
	cs := fixtureObj(t, "evaluation-input.json")["changeSet"].(map[string]any)
	return cs["changes"].([]any)
}

func mergePolicyEntries(t *testing.T) map[string]any {
	t.Helper()
	spec := fixtureObj(t, "merge-policy.json")["spec"].(map[string]any)
	if e, ok := spec["entries"].(map[string]any); ok {
		return e
	}
	return nil
}

func mergePolicyRules(t *testing.T) []any {
	t.Helper()
	spec := fixtureObj(t, "merge-policy.json")["spec"].(map[string]any)
	return spec["rules"].([]any)
}

// valueAtPointer resolves a simple RFC-6901 JSON Pointer (no ~0/~1 escapes
// needed for this fixture) against a document, returning the string value at
// that location or "" if it is absent or non-string.
func valueAtPointer(doc map[string]any, pointer string) string {
	node := any(doc)
	for _, tok := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		m, ok := node.(map[string]any)
		if !ok {
			return ""
		}
		node = m[tok]
	}
	s, _ := node.(string)
	return s
}

// fileStem returns the filename stem (basename without extension) for a rename
// side. The head stem comes from the change's file field; the base stem comes
// from the sourcePath the base document records.
func fileStem(change, doc map[string]any) string {
	path, _ := doc["sourcePath"].(string)
	if path == "" {
		path, _ = change["file"].(string)
	}
	base := path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	// Strip only the final extension (last dot) — the "stem" of
	// orders.events.v1.yaml is orders.events.v1, which is exactly the value
	// that could be mistaken for a content-derived /metadata/name.
	if i := strings.LastIndex(base, "."); i >= 0 {
		base = base[:i]
	}
	return base
}
