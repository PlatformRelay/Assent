package decision

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/schemas"
)

// fixturePins is a caller-supplied Pins with the out-of-band build/policy values
// (toolVersion/toolDigest/policySha) and the S01-derived SHAs, plus a merge-gap
// (the S04-only slice has no merge path). Package tests use fixture values —
// this pure package never reads build info or env.
func fixturePins() Pins {
	return Pins{
		ToolVersion: "0.1.0-test",
		ToolDigest:  "sha256:tool",
		PolicySha:   "sha256:policy",
		SourceSha:   "cccc", // <- S01 Pins.SourceSHA
		TargetSha:   "dddd", // <- S01 Pins.TargetSHA
		MergeResult: SkeletonMergeGap(),
		// provider-less skeleton: empty facts map.
	}
}

// sampleFindings returns findings across the fail-safe + unsatisfied shapes S03
// produces, exercising optional obligation/code presence.
func sampleFindings() []aggregate.Finding {
	return []aggregate.Finding{
		{Rule: "non-destructive-rule", Obligation: "non-destructive", Effect: aggregate.EffectBlock, Subject: "file:topics/a.yaml", Points: 0, Code: "destructive.change"},
		{Rule: "aggregate.uncovered", Obligation: "orphan", Effect: aggregate.EffectRequireReview, Subject: "file:topics/a.yaml", Points: 0, Code: "obligation.uncovered"},
	}
}

// validateRecord validates arbitrary JSON bytes against the frozen
// DecisionRecord schema, mirroring aggregate_test.go's pattern.
func validateRecord(t *testing.T, raw []byte) error {
	t.Helper()
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("unmarshal for validation: %v", err)
	}
	return schemas.DecisionRecordSchema.Validate(parsed)
}

func validatePresentation(t *testing.T, raw []byte) error {
	t.Helper()
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("unmarshal for validation: %v", err)
	}
	return schemas.PresentationModelSchema.Validate(parsed)
}

// TestDecisionRecordValidates proves the serialized DecisionRecord AND its
// companion PresentationModel validate against the frozen schemas across the
// three decisions and both merge-result shapes (gap and pinned), and that the
// PresentationModel carries no raw fact value / no rendered markdown
// (REQ-P4-E1-S04-01, REQ-P4-E1-S04-02).
func TestDecisionRecordValidates(t *testing.T) {
	pinnedDigest, err := PinnedMergeResult("sha256:merge")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		decision aggregate.Decision
		findings []aggregate.Finding
		pins     Pins
	}{
		{"review-with-findings-gap", aggregate.DecisionReview, sampleFindings(), fixturePins()},
		{"block-with-findings-gap", aggregate.DecisionBlock, sampleFindings(), fixturePins()},
		{"approve-empty-findings-gap", aggregate.DecisionApprove, nil, fixturePins()},
		{"approve-empty-findings-pinned", aggregate.DecisionApprove, nil, func() Pins {
			p := fixturePins()
			p.MergeResult = pinnedDigest
			return p
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := Build(aggregate.Result{Decision: tc.decision, Findings: tc.findings}, tc.pins)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			recBytes, err := rep.MarshalRecord()
			if err != nil {
				t.Fatalf("marshal record: %v", err)
			}
			if err := validateRecord(t, recBytes); err != nil {
				t.Fatalf("DecisionRecord does not validate: %v\n%s", err, recBytes)
			}

			pmBytes, err := rep.MarshalPresentation()
			if err != nil {
				t.Fatalf("marshal presentation: %v", err)
			}
			if err := validatePresentation(t, pmBytes); err != nil {
				t.Fatalf("PresentationModel does not validate: %v\n%s", err, pmBytes)
			}

			// PresentationModel redaction by construction: no "value" (raw fact)
			// and no "markdown"/"rendered" property (ADR-0016 §3/§4).
			var pm map[string]any
			if err := json.Unmarshal(pmBytes, &pm); err != nil {
				t.Fatal(err)
			}
			for _, banned := range []string{"value", "markdown", "rendered"} {
				if _, present := pm[banned]; present {
					t.Errorf("PresentationModel must not carry %q (redaction by construction): %s", banned, pmBytes)
				}
			}
			// findings must be an ARRAY in the PresentationModel (vs object in the record).
			if _, ok := pm["findings"].([]any); !ok {
				t.Errorf("PresentationModel.findings must be an array, got %T", pm["findings"])
			}

			// DecisionRecord.findings must be the phase-split OBJECT {observed, enforcing}.
			var rec map[string]any
			if err := json.Unmarshal(recBytes, &rec); err != nil {
				t.Fatal(err)
			}
			fo, ok := rec["findings"].(map[string]any)
			if !ok {
				t.Fatalf("DecisionRecord.findings must be an object, got %T", rec["findings"])
			}
			obs, ok := fo["observed"].([]any)
			if !ok || len(obs) != 0 {
				t.Errorf("DecisionRecord.findings.observed must be an empty array (no observe phase), got %v", fo["observed"])
			}
			if _, ok := fo["enforcing"].([]any); !ok {
				t.Errorf("DecisionRecord.findings.enforcing must be an array, got %T", fo["enforcing"])
			}
		})
	}
}

// TestApproveRequiresMergeResultPin proves the silent-widening state is (a) NOT
// representable by the frozen schema — a DecisionRecord claiming APPROVE with
// mergeResultDigest:null and NO capabilityGap FAILS validation (ADR-0017 §1) —
// and (b) NOT producible by this serializer: every gap MergeResult carries a
// non-empty capabilityGap, and a zero-value MergeResult is rejected
// (REQ-P4-E1-S04-02).
func TestApproveRequiresMergeResultPin(t *testing.T) {
	// (a) Hand-built adversarial JSON: APPROVE + null digest + no capabilityGap
	// must be REJECTED by the frozen schema (silent widening not representable).
	adversarial := map[string]any{
		"apiVersion": "assent.dev/v1alpha1",
		"kind":       "DecisionRecord",
		"decision":   "APPROVE",
		"findings":   map[string]any{"observed": []any{}, "enforcing": []any{}},
		"pins": map[string]any{
			"toolVersion": "0.1.0", "toolDigest": "sha256:t",
			"policySha": "p", "sourceSha": "s", "targetSha": "tg",
			"mergeResultDigest": nil, // null...
			// ...and NO capabilityGap -> must fail the allOf if/then.
			"factsResolvedAt": map[string]any{},
		},
	}
	raw, err := json.Marshal(adversarial)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRecord(t, raw); err == nil {
		t.Fatal("schema MUST reject APPROVE with mergeResultDigest:null and no capabilityGap (silent widening not representable, ADR-0017 §1)")
	}

	// Control: the SAME instance with a capabilityGap added must validate — the
	// rule is null<->capabilityGap coupling, decision-independent.
	pinsMap := adversarial["pins"].(map[string]any)
	pinsMap["capabilityGap"] = "no merge path in this slice"
	rawOK, err := json.Marshal(adversarial)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRecord(t, rawOK); err != nil {
		t.Fatalf("APPROVE + null digest + capabilityGap must VALIDATE (coupling is decision-independent): %v", err)
	}

	// (b) The serializer CANNOT produce the rejected state. A zero-value
	// MergeResult (never built by a constructor) is refused by marshalPins.
	if _, err := Build(aggregate.Result{Decision: aggregate.DecisionApprove}, Pins{
		ToolVersion: "0.1.0", ToolDigest: "sha256:t", PolicySha: "p",
		SourceSha: "s", TargetSha: "tg",
		MergeResult: MergeResult{}, // zero value: neither pinned nor a valid gap.
	}); err == nil {
		t.Fatal("Build MUST reject a zero-value MergeResult (would serialize to null-without-capabilityGap)")
	}

	// And a legitimately-constructed gap always yields a serialization that
	// validates: null digest + non-empty capabilityGap, for APPROVE too.
	rep, err := Build(aggregate.Result{Decision: aggregate.DecisionApprove}, fixturePins())
	if err != nil {
		t.Fatalf("Build with SkeletonMergeGap: %v", err)
	}
	recBytes, err := rep.MarshalRecord()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRecord(t, recBytes); err != nil {
		t.Fatalf("APPROVE + SkeletonMergeGap must validate: %v\n%s", err, recBytes)
	}
	// Confirm the produced bytes actually carry null + a capabilityGap.
	var rec map[string]any
	if err := json.Unmarshal(recBytes, &rec); err != nil {
		t.Fatal(err)
	}
	p := rec["pins"].(map[string]any)
	if p["mergeResultDigest"] != nil {
		t.Errorf("gap must serialize mergeResultDigest as null, got %v", p["mergeResultDigest"])
	}
	if gap, _ := p["capabilityGap"].(string); gap == "" {
		t.Error("gap must serialize a non-empty capabilityGap")
	}

	// Pinned digest must OMIT capabilityGap (the else branch of the allOf).
	pinned, err := PinnedMergeResult("sha256:m")
	if err != nil {
		t.Fatal(err)
	}
	pp := fixturePins()
	pp.MergeResult = pinned
	rep2, err := Build(aggregate.Result{Decision: aggregate.DecisionApprove}, pp)
	if err != nil {
		t.Fatal(err)
	}
	rec2Bytes, _ := rep2.MarshalRecord()
	var rec2 map[string]any
	if err := json.Unmarshal(rec2Bytes, &rec2); err != nil {
		t.Fatal(err)
	}
	p2 := rec2["pins"].(map[string]any)
	if p2["mergeResultDigest"] != "sha256:m" {
		t.Errorf("pinned digest must serialize the string, got %v", p2["mergeResultDigest"])
	}
	if _, present := p2["capabilityGap"]; present {
		t.Errorf("pinned digest must OMIT capabilityGap (allOf else), got %v", p2["capabilityGap"])
	}
	if err := validateRecord(t, rec2Bytes); err != nil {
		t.Fatalf("pinned-digest record must validate: %v\n%s", err, rec2Bytes)
	}
}

// TestConstructorsRejectEmpty proves the sum-type constructors refuse the
// degenerate inputs that would let a silent-widening or empty-pin state exist.
func TestConstructorsRejectEmpty(t *testing.T) {
	if _, err := PinnedMergeResult(""); err == nil {
		t.Error("PinnedMergeResult(\"\") must error (a pinned nothing is not a pin)")
	}
	if _, err := MergeResultGap(""); err == nil {
		t.Error("MergeResultGap(\"\") must error (null digest requires a non-empty capabilityGap)")
	}
	if _, err := PinnedMergeResult("sha256:x"); err != nil {
		t.Errorf("PinnedMergeResult with a digest must succeed: %v", err)
	}
	if _, err := MergeResultGap("no forge merge-train"); err != nil {
		t.Errorf("MergeResultGap with a reason must succeed: %v", err)
	}
}

// TestReportDoubleRunStable proves the serializer is deterministic: building +
// marshalling twice from the SAME inputs, and from a SHUFFLED finding order,
// yields byte-identical DecisionRecord and PresentationModel bytes (ADR-0013
// double-run gate; ADR-0017 §9 order-independence).
func TestReportDoubleRunStable(t *testing.T) {
	findings := sampleFindings()
	shuffled := []aggregate.Finding{findings[1], findings[0]} // reversed input order.

	build := func(fs []aggregate.Finding) (rec, pm []byte) {
		rep, err := Build(aggregate.Result{Decision: aggregate.DecisionReview, Findings: fs}, fixturePins())
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		rec, err = rep.MarshalRecord()
		if err != nil {
			t.Fatal(err)
		}
		pm, err = rep.MarshalPresentation()
		if err != nil {
			t.Fatal(err)
		}
		return rec, pm
	}

	rec1, pm1 := build(findings)
	rec2, pm2 := build(findings) // same input, second run.
	rec3, pm3 := build(shuffled) // shuffled input.

	if !bytes.Equal(rec1, rec2) {
		t.Errorf("DecisionRecord not byte-stable across a double-run:\n%s\n%s", rec1, rec2)
	}
	if !bytes.Equal(pm1, pm2) {
		t.Errorf("PresentationModel not byte-stable across a double-run:\n%s\n%s", pm1, pm2)
	}
	if !bytes.Equal(rec1, rec3) {
		t.Errorf("DecisionRecord not order-independent (shuffled input differs):\n%s\n%s", rec1, rec3)
	}
	if !bytes.Equal(pm1, pm3) {
		t.Errorf("PresentationModel not order-independent (shuffled input differs):\n%s\n%s", pm1, pm3)
	}
}

// TestFactsResolvedAtNonEmptyValidates proves a provider-keyed factsResolvedAt
// map (the non-provider-less case) round-trips and validates, exercising the
// non-nil facts path.
func TestFactsResolvedAtNonEmptyValidates(t *testing.T) {
	p := fixturePins()
	p.FactsResolvedAt = map[string]string{"quota": "2026-07-21T10:00:00Z"}
	rep, err := Build(aggregate.Result{Decision: aggregate.DecisionReview, Findings: sampleFindings()}, p)
	if err != nil {
		t.Fatal(err)
	}
	recBytes, err := rep.MarshalRecord()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRecord(t, recBytes); err != nil {
		t.Fatalf("record with a provider-keyed factsResolvedAt must validate: %v\n%s", err, recBytes)
	}
	var rec map[string]any
	if err := json.Unmarshal(recBytes, &rec); err != nil {
		t.Fatal(err)
	}
	facts := rec["pins"].(map[string]any)["factsResolvedAt"].(map[string]any)
	if facts["quota"] != "2026-07-21T10:00:00Z" {
		t.Errorf("factsResolvedAt must carry the provider timestamp, got %v", facts)
	}
}

// TestSortFindingsTotalKey exercises every tie-break branch of the canonical
// finding sort (subject, then rule, then obligation, then code, then effect), so
// the order-independence guarantee is proven at each level of the total key.
func TestSortFindingsTotalKey(t *testing.T) {
	// Each pair below differs ONLY at one key level, the second sorting before
	// the first; after the sort the "b" element must precede the "a" element.
	cases := []struct{ a, b finding }{
		{finding{Subject: "file:z", Rule: "r"}, finding{Subject: "file:a", Rule: "r"}},                                                                         // subject
		{finding{Subject: "s", Rule: "z"}, finding{Subject: "s", Rule: "a"}},                                                                                   // rule
		{finding{Subject: "s", Rule: "r", Obligation: "z"}, finding{Subject: "s", Rule: "r", Obligation: "a"}},                                                 // obligation
		{finding{Subject: "s", Rule: "r", Obligation: "o", Code: "z"}, finding{Subject: "s", Rule: "r", Obligation: "o", Code: "a"}},                           // code
		{finding{Subject: "s", Rule: "r", Obligation: "o", Code: "c", Effect: "z"}, finding{Subject: "s", Rule: "r", Obligation: "o", Code: "c", Effect: "a"}}, // effect
	}
	for i, tc := range cases {
		fs := []finding{tc.a, tc.b}
		sortFindings(fs)
		if fs[0] != tc.b {
			t.Errorf("case %d: expected %+v to sort first, got %+v", i, tc.b, fs[0])
		}
	}
}

// TestFindingsMirrorBetweenRecordAndPresentation proves the DecisionRecord's
// enforcing findings and the PresentationModel's findings array are the SAME
// finding set (same shape, same order) — the two artifacts never disagree.
func TestFindingsMirrorBetweenRecordAndPresentation(t *testing.T) {
	rep, err := Build(aggregate.Result{Decision: aggregate.DecisionReview, Findings: sampleFindings()}, fixturePins())
	if err != nil {
		t.Fatal(err)
	}
	enfBytes, _ := json.Marshal(rep.Record.Findings.Enforcing)
	pmBytes, _ := json.Marshal(rep.Presentation.Findings)
	if !bytes.Equal(enfBytes, pmBytes) {
		t.Errorf("record.enforcing and presentation.findings must mirror:\n%s\n%s", enfBytes, pmBytes)
	}
	// Sanity: the optional obligation/code round-trip and are present where set.
	if !strings.Contains(string(enfBytes), `"obligation"`) {
		t.Error("expected obligation to appear in enforcing findings")
	}
}
