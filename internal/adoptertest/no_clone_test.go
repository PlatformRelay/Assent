package adoptertest

import (
	"os"
	"strings"
	"testing"
)

// TestHarnessUsesSharedMatcher pins the structural half of REQ-XREV-S02-02: the
// harness's proving-silent detector must call the engine's shared
// aggregate.MatchesAny and must not reintroduce a local match predicate. Without
// this, a future re-clone would pass every behavioural test (the parity test would
// simply exercise the clone) and drift would go undetected — the D3 defect at one
// remove.
func TestHarnessUsesSharedMatcher(t *testing.T) {
	raw, err := os.ReadFile("coverage.go")
	if err != nil {
		t.Fatalf("read coverage.go: %v", err)
	}
	src := string(raw)
	if !strings.Contains(src, "aggregate.MatchesAny(") {
		t.Fatal("adoptertest/coverage.go does not call aggregate.MatchesAny — the harness no longer shares the engine predicate (D3)")
	}
	for _, banned := range []string{"func ruleMatchesAny", "func matchesAnyGlob", "func containsStr"} {
		if strings.Contains(src, banned) {
			t.Fatalf("adoptertest/coverage.go reintroduced a local matcher clone %q (D3 regression)", banned)
		}
	}
}
