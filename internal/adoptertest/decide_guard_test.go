package adoptertest

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// TestEvaluateEmptyRequireFailsClosed — REQ-XREV-S01-01 / D2. The adopter
// harness's decidable path routes through aggregate.Decide, so a binding with an
// empty require[] is refused fail-closed rather than vacuously APPROVEd. This is
// the adoptertest half of the S01 Definition of Done.
func TestEvaluateEmptyRequireFailsClosed(t *testing.T) {
	pol := &policy.MergePolicy{}
	bind := &policy.Binding{Environment: "prod"} // the vacuity: no required obligations
	c := Case{
		Name:   "empty-require",
		Policy: pol,
		Bind:   bind,
		File:   "topics/orders.yaml",
		Base:   []byte("enabled: true\n"),
		Head:   []byte("enabled: false\n"),
	}
	if _, err := Evaluate(c); err == nil {
		t.Fatal("Evaluate with an empty require[] must error (D2), not vacuously APPROVE")
	} else if !strings.Contains(err.Error(), "require is empty") {
		t.Fatalf("expected the empty-require guard to fire, got: %v", err)
	}
}
