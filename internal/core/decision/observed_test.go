package decision

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

// TestObservedFindingsThreadedIntoRecord — REQ-E2-S08-05. record.go threads the
// aggregator's Observed bucket into DecisionRecord findings.observed (it was
// hardcoded []). An observe finding must appear ONLY in observed and never in
// enforcing; the record still validates against the frozen schema; and a Result
// with no Observed still serializes observed as [] (not null), keeping the D-016
// golden byte-identical.
func TestObservedFindingsThreadedIntoRecord(t *testing.T) {
	res := aggregate.Result{
		Decision: aggregate.DecisionApprove, // observe never lowers the decision
		Findings: []aggregate.Finding{
			{Rule: "enforce-owner", Obligation: "ownership", Effect: aggregate.EffectRequireReview, Subject: "topic-registry:orders.events.v1", Points: 0, Code: "owner-missing"},
		},
		Observed: []aggregate.Finding{
			{Rule: "would-block", Obligation: "signal", Effect: aggregate.EffectBlock, Subject: "topic-registry:orders.events.v1", Points: 7, Code: "would-block"},
		},
	}

	rep, err := Build(res, fixturePins())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	recBytes, err := rep.MarshalRecord()
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	if err := validateRecord(t, recBytes); err != nil {
		t.Fatalf("record with observed findings must validate: %v\n%s", err, recBytes)
	}

	if got := len(rep.Record.Findings.Observed); got != 1 {
		t.Fatalf("record.findings.observed must carry the one observe finding, got %d: %+v", got, rep.Record.Findings.Observed)
	}
	if code := rep.Record.Findings.Observed[0].Code; code != "would-block" {
		t.Errorf("observed finding code = %q, want would-block", code)
	}
	// The observe finding must NOT leak into the enforcing bucket.
	for _, f := range rep.Record.Findings.Enforcing {
		if f.Code == "would-block" {
			t.Errorf("observe finding leaked into enforcing: %+v", f)
		}
	}

	// A Result with NO observed findings still serializes observed as [] (not null).
	empty, err := Build(aggregate.Result{Decision: aggregate.DecisionBlock, Findings: sampleFindings()}, fixturePins())
	if err != nil {
		t.Fatal(err)
	}
	emptyBytes, _ := empty.MarshalRecord()
	var rec map[string]any
	if err := json.Unmarshal(emptyBytes, &rec); err != nil {
		t.Fatal(err)
	}
	obs, ok := rec["findings"].(map[string]any)["observed"].([]any)
	if !ok || len(obs) != 0 {
		t.Errorf("no-observe run must serialize observed as [], got %v", rec["findings"].(map[string]any)["observed"])
	}
	if bytes.Contains(emptyBytes, []byte(`"observed":null`)) {
		t.Error("observed must never serialize as null")
	}
}
