package adoptertest_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
	"github.com/PlatformRelay/assent/internal/change"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// loadEntriesCase assembles a directory case for the multi-rule list-mode pack
// (testdata/entries/**). It reads the pack, binding, base/head config.json, facts,
// and expect off disk (the test's I/O boundary) and hands the bytes to the pure
// library — exactly like loadCase, but for the S02 whole-pack replay.
func loadEntriesCase(t *testing.T, name string) adoptertest.Case {
	t.Helper()
	const root = "testdata/entries"
	mp, err := policy.LoadMergePolicy(readFile(t, filepath.Join(root, "pack", "rules", "multi.yaml")))
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	rb, err := policy.LoadRulesetBinding(readFile(t, filepath.Join(root, "bindings.yaml")))
	if err != nil {
		t.Fatalf("load binding: %v", err)
	}
	caseDir := filepath.Join(root, "cases", name)
	expect, err := adoptertest.LoadExpectation(readFile(t, filepath.Join(caseDir, "expect.yaml")))
	if err != nil {
		t.Fatalf("load expect: %v", err)
	}
	facts, err := adoptertest.MapFacts(readFile(t, filepath.Join(caseDir, "facts.yaml")))
	if err != nil {
		t.Fatalf("map facts: %v", err)
	}
	const file = "config.json"
	return adoptertest.Case{
		Name:   name,
		Policy: mp,
		Bind:   &rb.Bindings[0],
		File:   file,
		Base:   readFile(t, filepath.Join(caseDir, "base", file)),
		Head:   readFile(t, filepath.Join(caseDir, "head", file)),
		Facts:  facts,
		Expect: expect,
	}
}

// TestWholePackReplayBindsEntryAndScalar (REQ-E6-S02-03) drives the whole multi-rule
// pack over ONE base/↔head/ input: an ENTRY-scoped ownership rule (`entry.owner in
// facts…`) and a SCALAR bounded-change leaf rule (`new >= old`) must BOTH bind
// correctly simultaneously. The APPROVE case can only pass if `entry` bound the
// reconstructed OBJECT (a scalar binding would make `entry.owner` error → REVIEW);
// the negative cases prove each rule is genuinely active (the scalar rule fires on a
// replicas shrink; the entry rule fires on an unauthorized owner).
func TestWholePackReplayBindsEntryAndScalar(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"approve", "APPROVE"},
		{"shrink", "REVIEW"},       // the SCALAR bounded rule fired
		{"unauthorized", "REVIEW"}, // the ENTRY-scoped ownership rule fired
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := adoptertest.RunCase(loadEntriesCase(t, tc.name))
			if err != nil {
				t.Fatalf("RunCase: %v", err)
			}
			if out.Actual != tc.want {
				t.Fatalf("decision = %q, want %q", out.Actual, tc.want)
			}
		})
	}
}

// TestEntryTreeReconstructionKeyedByPointer (REQ-E6-S02-04) proves the harness
// reconstructs list-mode entries keyed by identity.pointer and that `entry` binds
// the reconstructed OBJECT, not the raw change scalar. The reconstructed EntryRef
// keys must match exactly what change.DiffEntries tags a change with; and the
// APPROVE outcome (ownership proven via entry.owner) is only reachable when the
// object — not the scalar — is bound.
func TestEntryTreeReconstructionKeyedByPointer(t *testing.T) {
	c := loadEntriesCase(t, "approve")
	cfg := change.EntryConfig{Mode: change.ModeList, Root: "/services", Identity: "/name", Label: "svc"}

	// Reconstructed keys line up with the differ's EntryRefs (single source of truth).
	entries, err := change.Entries(c.File, c.Head, cfg)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	cs, err := change.DiffEntries(c.File, c.Base, c.Head, cfg)
	if err != nil {
		t.Fatalf("DiffEntries: %v", err)
	}
	if len(cs.Changes) == 0 {
		t.Fatal("expected at least one entry-tagged change")
	}
	for _, ch := range cs.Changes {
		if _, ok := entries[ch.EntryRef]; !ok {
			t.Fatalf("change EntryRef %q not reconstructed (keys: %v)", ch.EntryRef, entries)
		}
	}

	// The whole-pack replay APPROVEs — only possible if entry bound the object.
	out, err := adoptertest.RunCase(c)
	if err != nil {
		t.Fatalf("RunCase: %v", err)
	}
	if out.Actual != "APPROVE" {
		t.Fatalf("entry-object binding decision = %q, want APPROVE", out.Actual)
	}
}

// TestS02WholePackDoubleRunStable proves the S02 replay path (entry reconstruction +
// approval stub + MR threading) is deterministic: each case evaluates twice
// byte-identical (ADR-0014 golden L0), across the entries, review, and mr fixtures.
func TestS02WholePackDoubleRunStable(t *testing.T) {
	cases := []adoptertest.Case{
		loadEntriesCase(t, "approve"),
		loadEntriesCase(t, "unauthorized"),
	}
	rev := loadReviewCase(t, nil)
	appr, err := adoptertest.MapApproval(readFile(t, "testdata/reviewpack/approval.yaml"))
	if err != nil {
		t.Fatalf("MapApproval: %v", err)
	}
	rev.Approval = appr
	cases = append(cases, rev)

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			first, err := adoptertest.Evaluate(c)
			if err != nil {
				t.Fatalf("Evaluate #1: %v", err)
			}
			second, err := adoptertest.Evaluate(c)
			if err != nil {
				t.Fatalf("Evaluate #2: %v", err)
			}
			if !bytes.Equal(mustJSON(t, first), mustJSON(t, second)) {
				t.Fatalf("double run not byte-identical:\n#1 %s\n#2 %s", mustJSON(t, first), mustJSON(t, second))
			}
		})
	}
}
