package adoptertest

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// coverage_internal_test.go is a WHITE-BOX table test of ruleMatchesAny — the
// hand-copied mirror of engine aggregate.matchChanges that is the load-bearing
// proving-silent detector (D-059). It is PURE (no internal/core evaluation; it
// exercises the mirror's OWN behaviour) and guards every domain arm AND every
// fail-closed error branch, so a regression flipping fail-closed → silent
// matched-nothing (a false "covered", the exact failure --coverage exists to
// prevent) goes RED instead of shipping green.

// TestRuleMatchesAnyDomains covers the value-level match domains (files, values,
// valueChanges) — with both a matching and a non-matching change where applicable
// — and the TWO fail-closed error branches (values with no selector → error,
// no-domain → error), each asserting an ERROR (never a silent false), per D-059's
// "a match-derivation defect errors the gate". The fileEvents domain (no longer a
// fail-closed error since EFE-S01) has its own selection + disjointness coverage
// in TestRuleMatchesAnyFileEventsDisjoint / TestRuleMatchesAnyMirrorsEngineFileEvents.
func TestRuleMatchesAnyDomains(t *testing.T) {
	// changes reused across cases: a modify of /partitions in config.json, and an add
	// of /name in other.json.
	modifyPartitions := aggregate.EvalChange{Subject: "config.json", File: "config.json", Path: "/partitions", Kind: "modify"}
	addName := aggregate.EvalChange{Subject: "other.json", File: "other.json", Path: "/name", Kind: "add"}
	both := []aggregate.EvalChange{modifyPartitions, addName}

	tests := []struct {
		name      string
		match     policy.Match
		changes   []aggregate.EvalChange
		wantMatch bool
		wantErr   bool
	}{
		// --- files (glob over ch.File) ---
		{
			name:      "files: glob matches a changed file",
			match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"*.json"}}},
			changes:   both,
			wantMatch: true,
		},
		{
			name:      "files: glob matches no changed file",
			match:     policy.Match{Files: &policy.FilesMatch{Paths: []string{"*.yaml"}}},
			changes:   both,
			wantMatch: false,
		},

		// --- valueChanges (pointer glob AND kind AND path glob) ---
		{
			name:      "valueChanges: pointer + kind match",
			match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}},
			changes:   both,
			wantMatch: true,
		},
		{
			name:      "valueChanges: pointer matches but kind excludes",
			match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/name"}, Kinds: []string{"modify"}}},
			changes:   both, // /name is an add, not a modify
			wantMatch: false,
		},
		{
			name:      "valueChanges: pointer glob but path glob narrows it out",
			match:     policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Paths: []string{"other.json"}}},
			changes:   both, // /partitions is in config.json, not other.json
			wantMatch: false,
		},

		// --- values (implied kind:modify, pointer glob AND path glob) ---
		{
			name:      "values: implied modify + pointer match",
			match:     policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"/partitions"}}},
			changes:   both,
			wantMatch: true,
		},
		{
			name:      "values: pointer matches but change is an add (not modify)",
			match:     policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"/name"}}},
			changes:   both, // /name is an add; values implies modify
			wantMatch: false,
		},
		{
			name:      "values: path selector only, matches by file",
			match:     policy.Match{Values: &policy.ValuesMatch{Paths: []string{"config.json"}}},
			changes:   both,
			wantMatch: true,
		},
		{
			name:      "values: modify change but path selector narrows it out",
			match:     policy.Match{Values: &policy.ValuesMatch{Paths: []string{"nomatch.json"}}},
			changes:   both, // config.json is a modify, but the path glob excludes it
			wantMatch: false,
		},

		// --- fail-closed error branches (D-059) ---
		{
			name:    "values with neither pointers nor paths errors (never a wildcard)",
			match:   policy.Match{Values: &policy.ValuesMatch{}},
			changes: both,
			wantErr: true,
		},
		{
			name:    "no declared domain errors",
			match:   policy.Match{},
			changes: both,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ruleMatchesAny(tc.match, tc.changes)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected a fail-closed error, got (match=%v, nil error)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantMatch {
				t.Fatalf("match = %v, want %v", got, tc.wantMatch)
			}
		})
	}
}

// fileEventsMirrorCase is one shared (match, changes, wantMatch) row driving both
// the mirror-only disjointness test and the engine-agreement test below.
type fileEventsMirrorCase struct {
	name      string
	match     policy.Match
	changes   []aggregate.EvalChange
	wantMatch bool
}

// fileEventsMirrorCases is the shared EFE-S01 table: fileEvents selection plus
// domain disjointness in BOTH directions. Value-domain globs use "**" (which
// matches both the empty pointer and a real pointer), so the path=="" disjointness
// rows are guard-DEPENDENT — absent the ch.Path!="" guard they would leak.
func fileEventsMirrorCases() []fileEventsMirrorCase {
	fileAdd := aggregate.EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "add"}
	fileDelete := aggregate.EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "delete"}
	fileModify := aggregate.EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "modify"}
	valueAdd := aggregate.EvalChange{Subject: "topics/orders.yaml", File: "topics/orders.yaml", Path: "/partitions", Kind: "add"}

	feAddDelete := policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"topics/*.yaml"}, Kinds: []string{"add", "delete"}}}
	feDeleteOnly := policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"topics/*.yaml"}, Kinds: []string{"delete"}}}
	feWrongGlob := policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"schemas/*.yaml"}, Kinds: []string{"add", "delete"}}}
	files := policy.Match{Files: &policy.FilesMatch{Paths: []string{"topics/*.yaml"}}}
	values := policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"**"}}}
	valueChanges := policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"**"}, Kinds: []string{"add"}}}

	return []fileEventsMirrorCase{
		{"fileEvents selects whole-file add", feAddDelete, []aggregate.EvalChange{fileAdd}, true},
		{"fileEvents selects whole-file delete", feAddDelete, []aggregate.EvalChange{fileDelete}, true},
		{"fileEvents kind not in kinds", feDeleteOnly, []aggregate.EvalChange{fileAdd}, false},
		{"fileEvents glob no match", feWrongGlob, []aggregate.EvalChange{fileAdd}, false},
		{"fileEvents does not select value-level add", feAddDelete, []aggregate.EvalChange{valueAdd}, false},
		{"files does not select path=='' add", files, []aggregate.EvalChange{fileAdd}, false},
		{"files does not select path=='' delete", files, []aggregate.EvalChange{fileDelete}, false},
		{"values does not select path=='' modify", values, []aggregate.EvalChange{fileModify}, false},
		{"valueChanges does not select path=='' add", valueChanges, []aggregate.EvalChange{fileAdd}, false},
		{"files still selects value-level change", files, []aggregate.EvalChange{valueAdd}, true},
		{"valueChanges still selects value-level add", valueChanges, []aggregate.EvalChange{valueAdd}, true},
	}
}

// TestRuleMatchesAnyFileEventsDisjoint — REQ-EFE-S01-03 (mirror side). The E6
// mirror implements the identical fileEvents + path=="" selection and both-way
// disjointness the engine matcher does.
func TestRuleMatchesAnyFileEventsDisjoint(t *testing.T) {
	for _, tc := range fileEventsMirrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ruleMatchesAny(tc.match, tc.changes)
			if err != nil {
				t.Fatalf("ruleMatchesAny: %v", err)
			}
			if got != tc.wantMatch {
				t.Fatalf("mirror match = %v, want %v", got, tc.wantMatch)
			}
		})
	}
}

// TestRuleMatchesAnyMirrorsEngineFileEvents — REQ-EFE-S01-04. The load-bearing E6
// mirror (ruleMatchesAny) agrees with the real engine matcher on every fileEvents
// + disjointness case. The engine's selection is observed through aggregate.Cover:
// a matched change under a when:false / onFailure:block enforce rule yields a
// non-APPROVE decision; an unmatched rule leaves the (covered) obligation
// non-firing -> APPROVE. Any drift between mirror and engine (a false "covered",
// the exact failure --coverage exists to prevent) fails here.
func TestRuleMatchesAnyMirrorsEngineFileEvents(t *testing.T) {
	for _, tc := range fileEventsMirrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			mirror, err := ruleMatchesAny(tc.match, tc.changes)
			if err != nil {
				t.Fatalf("ruleMatchesAny: %v", err)
			}

			pol := &policy.MergePolicy{
				Spec: policy.MergePolicySpec{
					Rules: []policy.Rule{{
						Name:      "r",
						Phase:     policy.PhaseEnforce,
						Match:     tc.match,
						Prove:     &policy.Prove{Obligation: "o", When: policy.AssertTree{Leaf: &policy.Leaf{CEL: "false"}}},
						OnFailure: &policy.OnFailure{Effect: policy.EffectBlock, Code: "c"},
					}},
				},
			}
			bind := &policy.Binding{Require: []string{"o"}}
			in := &aggregate.EvaluationInput{ChangeSet: aggregate.ChangeSet{Changes: tc.changes}}
			res, err := aggregate.Cover(pol, bind, in)
			if err != nil {
				t.Fatalf("aggregate.Cover: %v", err)
			}
			// A matched when:false change fires block -> non-APPROVE; 0 matched leaves
			// the covered obligation non-firing -> APPROVE.
			engine := res.Decision != aggregate.DecisionApprove
			if engine != mirror {
				t.Fatalf("engine matched=%v but mirror matched=%v (decision=%q)", engine, mirror, res.Decision)
			}
			if mirror != tc.wantMatch {
				t.Fatalf("mirror match = %v, want %v", mirror, tc.wantMatch)
			}
		})
	}
}
