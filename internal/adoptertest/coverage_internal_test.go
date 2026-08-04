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

// TestRuleMatchesAnyDomains covers the four match domains (files, values,
// valueChanges, fileEvents) — with both a matching and a non-matching change where
// applicable — and the THREE fail-closed error branches (fileEvents → error,
// values with no selector → error, no-domain → error), each asserting an ERROR
// (never a silent false), per D-059's "a match-derivation defect errors the gate".
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
			name:    "fileEvents domain errors (deferred, unsupported)",
			match:   policy.Match{FileEvents: &policy.FileEventsMatch{Kinds: []string{"delete"}}},
			changes: both,
			wantErr: true,
		},
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
