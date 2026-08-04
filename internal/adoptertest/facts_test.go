package adoptertest_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
)

// TestAuthoredFactsMapToResolvedEnvelope (REQ-E6-S01-02) proves the load-bearing
// fact-shape translation the E3-S08 exit gate documented as deferred: an authored
// facts.yaml (provider -> name -> value) is lifted into the resolved-fact envelope
// {State:"resolved", Value:...} the engine binds — and an ABSENT provider/name yields
// NO Fact (never a fabricated resolved value that could APPROVE on an unresolved fact).
func TestAuthoredFactsMapToResolvedEnvelope(t *testing.T) {
	const authored = `
limits:
  maxPartitions: 32
author:
  groups: [orders-team]
`
	facts, err := adoptertest.MapFacts([]byte(authored))
	if err != nil {
		t.Fatalf("MapFacts: %v", err)
	}

	// A present fact becomes a RESOLVED envelope exposing its value.
	f, ok := facts["limits"]["maxPartitions"]
	if !ok {
		t.Fatal("expected a Fact for limits.maxPartitions")
	}
	if f.State != "resolved" {
		t.Fatalf("limits.maxPartitions state = %q, want \"resolved\"", f.State)
	}
	// Numerics are carried as json.Number so aggregate.toCEL binds int64 (a numeric
	// compare), not a lexical string.
	if got, ok := f.Value.(json.Number); !ok || got.String() != "32" {
		t.Fatalf("limits.maxPartitions value = %#v (%T), want json.Number(\"32\")", f.Value, f.Value)
	}

	// A nested/list authored value is carried through faithfully.
	if g, ok := facts["author"]["groups"]; !ok || g.State != "resolved" || g.Value == nil {
		t.Fatalf("author.groups = %#v, want a resolved fact with a value", facts["author"]["groups"])
	}

	// An ABSENT provider yields NO Fact entry — the engine then errors on any
	// predicate reading its .value and fails safe, never fabricating a resolved value.
	if _, ok := facts["band"]; ok {
		t.Fatal("provider \"band\" was never authored — it must not appear in the envelope")
	}
	// An absent NAME within a present provider likewise yields no Fact.
	if _, ok := facts["limits"]["minPartitions"]; ok {
		t.Fatal("fact \"minPartitions\" was never authored — it must not appear in the envelope")
	}
}

// TestMapFactsMalformedRejected proves a facts.yaml that is not a provider ->
// name -> value map is a located rejection rather than a silently-empty (and thus
// fail-open) envelope.
func TestMapFactsMalformedRejected(t *testing.T) {
	t.Run("a non-map top level is rejected", func(t *testing.T) {
		if _, err := adoptertest.MapFacts([]byte("- a\n- b\n")); err == nil {
			t.Fatal("expected a sequence top level to be rejected")
		}
	})
	t.Run("a provider whose value is not a name map is rejected", func(t *testing.T) {
		_, err := adoptertest.MapFacts([]byte("limits: 5\n"))
		if err == nil {
			t.Fatal("expected a scalar provider value to be rejected")
		}
		if !strings.Contains(err.Error(), "limits") {
			t.Fatalf("error does not locate the offending provider: %v", err)
		}
	})
}

// TestAuthoredFactsEmptyIsEmptyEnvelope proves an empty/whitespace facts.yaml lifts to
// an empty envelope (no fabricated facts) rather than an error — the "provider
// resolved nothing" case the unresolved-fact directory case exercises end-to-end.
func TestAuthoredFactsEmptyIsEmptyEnvelope(t *testing.T) {
	for _, raw := range []string{"", "   \n", "{}\n"} {
		facts, err := adoptertest.MapFacts([]byte(raw))
		if err != nil {
			t.Fatalf("MapFacts(%q): %v", raw, err)
		}
		if len(facts) != 0 {
			t.Fatalf("MapFacts(%q) = %v, want an empty envelope", raw, facts)
		}
	}
}
