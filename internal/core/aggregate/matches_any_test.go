package aggregate_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// TestMatchesAnyDomains covers the shared exported matcher's four domains and its
// two fail-closed error branches, so the predicate the harness now shares with the
// engine (D3) is exercised in the engine's own package too.
func TestMatchesAnyDomains(t *testing.T) {
	modify := aggregate.EvalChange{Subject: "config.json", File: "config.json", Path: "/partitions", Kind: "modify"}
	add := aggregate.EvalChange{Subject: "other.json", File: "other.json", Path: "/name", Kind: "add"}
	wholeFileAdd := aggregate.EvalChange{Subject: "file:topics/orders.yaml", File: "topics/orders.yaml", Path: "", Kind: "add"}
	both := []aggregate.EvalChange{modify, add}

	cases := []struct {
		name      string
		match     policy.Match
		changes   []aggregate.EvalChange
		wantMatch bool
		wantErr   bool
	}{
		{"files match", policy.Match{Files: &policy.FilesMatch{Paths: []string{"*.json"}}}, both, true, false},
		{"files no match", policy.Match{Files: &policy.FilesMatch{Paths: []string{"*.yaml"}}}, both, false, false},
		{"valueChanges match", policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/partitions"}, Kinds: []string{"modify"}}}, both, true, false},
		{"valueChanges kind excludes", policy.Match{ValueChanges: &policy.ValueChangesMatch{Pointers: []string{"/name"}, Kinds: []string{"modify"}}}, both, false, false},
		{"values match", policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"/partitions"}}}, both, true, false},
		{"values excludes add", policy.Match{Values: &policy.ValuesMatch{Pointers: []string{"/name"}}}, both, false, false},
		{"fileEvents match", policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"topics/*.yaml"}, Kinds: []string{"add", "delete"}}}, []aggregate.EvalChange{wholeFileAdd}, true, false},
		{"fileEvents disjoint from value change", policy.Match{FileEvents: &policy.FileEventsMatch{Paths: []string{"topics/*.yaml"}, Kinds: []string{"add"}}}, []aggregate.EvalChange{modify}, false, false},
		{"files disjoint from whole-file event", policy.Match{Files: &policy.FilesMatch{Paths: []string{"topics/*.yaml"}}}, []aggregate.EvalChange{wholeFileAdd}, false, false},
		{"values without selectors errors", policy.Match{Values: &policy.ValuesMatch{}}, both, false, true},
		{"no domain errors", policy.Match{}, both, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := aggregate.MatchesAny(tc.match, tc.changes)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected a fail-closed error, got match=%v", got)
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

// TestDecideNilInputAndBinding covers the guarded entry's defensive branches.
func TestDecideNilInputAndBinding(t *testing.T) {
	if _, err := aggregate.Decide(aggregate.DecideRequest{Subject: "file:x", Binding: &policy.Binding{Require: []string{"o"}}}); err == nil {
		t.Fatal("Decide with a nil input must error")
	}
	if _, err := aggregate.Decide(aggregate.DecideRequest{Subject: "file:x", Input: oneChange()}); err == nil {
		t.Fatal("Decide with a nil binding must error")
	}
}
