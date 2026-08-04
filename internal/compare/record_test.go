package compare

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/schemas"
)

const testBundleSubject = "topic-registry:orders.events.v1"

func validateRecordJSON(t *testing.T, rec ComparisonRecord) {
	t.Helper()
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	var doc any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}
	if err := schemas.ComparisonRecordSchema.Validate(doc); err != nil {
		t.Fatalf("ComparisonRecordSchema.Validate: %v\njson=%s", err, string(raw))
	}
}

// REQ-PCS-S04-01: a classified widening case produces schema-valid JSON with
// per-delta identity fields (kind, rule, subject, baseline/candidate sides).
func TestComparisonRecordValidates(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod-strict@6", "new >= old", "", policy.EffectBlock)
	candidate := mkProfile("prod-strict@7", "true", "", policy.EffectBlock)
	baseRes := evaluateMust(t, in, baseline)
	candRes := evaluateMust(t, in, candidate)

	rec, err := BuildComparisonRecord("partition-shrink-widen", baseline.Name, candidate.Name, baseRes, candRes, []string{testBundleSubject})
	if err != nil {
		t.Fatalf("BuildComparisonRecord: %v", err)
	}
	if rec.BaselineProfile != baseline.Name || rec.CandidateProfile != candidate.Name {
		t.Fatalf("profiles = %q / %q", rec.BaselineProfile, rec.CandidateProfile)
	}
	if rec.CaseID != "partition-shrink-widen" {
		t.Fatalf("caseId = %q, want partition-shrink-widen", rec.CaseID)
	}
	if len(rec.Deltas) != 1 {
		t.Fatalf("len(deltas) = %d, want 1", len(rec.Deltas))
	}
	d := rec.Deltas[0]
	if d.Kind != KindNewlyAutoMergeable {
		t.Fatalf("delta kind = %q, want %q", d.Kind, KindNewlyAutoMergeable)
	}
	if d.Rule != "non-destructive" || d.Subject != "topic-registry:orders.events.v1" {
		t.Fatalf("delta identity = rule=%q subject=%q", d.Rule, d.Subject)
	}
	if !d.Baseline.Present || d.Baseline.Decision != string(aggregate.DecisionBlock) {
		t.Fatalf("baseline side = %+v, want present BLOCK", d.Baseline)
	}
	if d.Candidate.Present {
		t.Fatalf("candidate side should be absent (present=false), got %+v", d.Candidate)
	}
	if d.Candidate.Decision != string(aggregate.DecisionApprove) {
		t.Fatalf("candidate aggregate decision = %q, want APPROVE", d.Candidate.Decision)
	}
	validateRecordJSON(t, rec)
}

// REQ-PCS-S04-02: explanation-only deltas appear with shared decision/effect on
// both sides; the record carries no gate-failure signal (gates are S05).
func TestComparisonRecordExplanationOnly(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod@6", "new >= old", "partitions must not shrink below the baseline", policy.EffectBlock)
	candidate := mkProfile("prod@7", "new >= old", "partitions may not be reduced", policy.EffectBlock)
	baseRes := evaluateMust(t, in, baseline)
	candRes := evaluateMust(t, in, candidate)

	rec, err := BuildComparisonRecord("wording-only", baseline.Name, candidate.Name, baseRes, candRes, []string{testBundleSubject})
	if err != nil {
		t.Fatalf("BuildComparisonRecord: %v", err)
	}
	if len(rec.Deltas) != 1 {
		t.Fatalf("len(deltas) = %d, want 1", len(rec.Deltas))
	}
	d := rec.Deltas[0]
	if d.Kind != KindExplanationOnly {
		t.Fatalf("kind = %q, want explanation-only", d.Kind)
	}
	if d.Baseline.Decision != d.Candidate.Decision || d.Baseline.Effect != d.Candidate.Effect {
		t.Fatalf("baseline/candidate must share decision/effect: %+v vs %+v", d.Baseline, d.Candidate)
	}
	if !d.Baseline.Present || !d.Candidate.Present {
		t.Fatalf("both sides must be present for explanation-only")
	}
	validateRecordJSON(t, rec)
}

// REQ-PCS-S04-03: duplicate (kind, rule, subject) identities fail closed at build.
func TestComparisonRecordDuplicateDeltaRejected(t *testing.T) {
	rec := ComparisonRecord{
		APIVersion:       recordAPIVersion,
		Kind:             recordKind,
		BaselineProfile:  "b@1",
		CandidateProfile: "c@1",
		CaseID:           "dup",
		Deltas: []Delta{
			{
				Kind:    KindNewlyAutoMergeable,
				Rule:    "r",
				Subject: "topic-registry:orders.events.v1",
				Baseline: OutcomeSide{
					Present:  true,
					Decision: string(aggregate.DecisionBlock),
					Effect:   string(aggregate.EffectBlock),
				},
				Candidate: OutcomeSide{Present: false, Decision: string(aggregate.DecisionApprove)},
			},
			{
				Kind:    KindNewlyAutoMergeable,
				Rule:    "r",
				Subject: "topic-registry:orders.events.v1",
				Baseline: OutcomeSide{
					Present:  true,
					Decision: string(aggregate.DecisionBlock),
					Effect:   string(aggregate.EffectBlock),
				},
				Candidate: OutcomeSide{Present: false, Decision: string(aggregate.DecisionApprove)},
			},
		},
	}
	if err := rec.Validate(); err == nil {
		t.Fatal("expected duplicate delta keys to be rejected at build time")
	}
}

// Empty deltas when baseline and candidate fully agree.
func TestComparisonRecordEmptyWhenAgree(t *testing.T) {
	in := loadBundle(t)
	p := mkProfile("prod@6", "new >= old", "same message", policy.EffectBlock)
	res := evaluateMust(t, in, p)

	rec, err := BuildComparisonRecord("agree", p.Name, p.Name, res, res, []string{testBundleSubject})
	if err != nil {
		t.Fatalf("BuildComparisonRecord: %v", err)
	}
	if len(rec.Deltas) != 0 {
		t.Fatalf("deltas = %+v, want empty", rec.Deltas)
	}
	validateRecordJSON(t, rec)
}

func TestComparisonRecordMissedIntervention(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod-strict@6", "new >= old", "", policy.EffectBlock)
	candidate := Profile{
		Name:    "prod-permissive@7",
		Policy:  &policy.MergePolicy{Spec: policy.MergePolicySpec{Rules: nil}},
		Bind:    baseline.Bind,
		Ceiling: policy.PhaseEnforce,
	}
	baseRes := evaluateMust(t, in, baseline)
	candRes := evaluateMust(t, in, candidate)

	rec, err := BuildComparisonRecord("missed", baseline.Name, candidate.Name, baseRes, candRes, []string{testBundleSubject})
	if err != nil {
		t.Fatalf("BuildComparisonRecord: %v", err)
	}
	if len(rec.Deltas) != 1 || rec.Deltas[0].Kind != KindDestructiveOrAuthorizationInterventionMissed {
		t.Fatalf("deltas = %+v, want one missed-intervention delta", rec.Deltas)
	}
	validateRecordJSON(t, rec)
}

func TestComparisonRecordStricterIntervention(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod-permissive@6", "true", "", policy.EffectBlock)
	candidate := mkProfile("prod-strict@7", "new >= old", "", policy.EffectBlock)
	baseRes := evaluateMust(t, in, baseline)
	candRes := evaluateMust(t, in, candidate)

	rec, err := BuildComparisonRecord("stricter", baseline.Name, candidate.Name, baseRes, candRes, []string{testBundleSubject})
	if err != nil {
		t.Fatalf("BuildComparisonRecord: %v", err)
	}
	if len(rec.Deltas) != 1 || rec.Deltas[0].Kind != KindStricterInterventionAdded {
		t.Fatalf("deltas = %+v, want one stricter-intervention delta", rec.Deltas)
	}
	validateRecordJSON(t, rec)
}

func TestComparisonRecordObligationUncovered(t *testing.T) {
	in := loadBundleJSON(t, obligationBundleJSON)
	baseline := mkObligationProfile("prod@6", []policy.Rule{ownershipRule(), nonDestructiveRule()})
	candidate := mkObligationProfile("prod@7", []policy.Rule{nonDestructiveRule()})
	baseRes := evaluateMust(t, in, baseline)
	candRes := evaluateMust(t, in, candidate)

	rec, err := BuildComparisonRecord("uncovered", baseline.Name, candidate.Name, baseRes, candRes, []string{testBundleSubject})
	if err != nil {
		t.Fatalf("BuildComparisonRecord: %v", err)
	}
	if len(rec.Deltas) != 1 || rec.Deltas[0].Kind != KindSubjectOrObligationUncovered {
		t.Fatalf("deltas = %+v, want one obligation-uncovered delta", rec.Deltas)
	}
	validateRecordJSON(t, rec)
}

func TestComparisonRecordScoreThresholdChange(t *testing.T) {
	in := loadBundleJSON(t, scoreBundleJSON)
	baseline := mkScoreProfile("prod@6", 4)
	candidate := mkScoreProfile("prod@7", 10)
	baseRes := evaluateMust(t, in, baseline)
	candRes := evaluateMust(t, in, candidate)

	rec, err := BuildComparisonRecord("score", baseline.Name, candidate.Name, baseRes, candRes, []string{testBundleSubject})
	if err != nil {
		t.Fatalf("BuildComparisonRecord: %v", err)
	}
	if len(rec.Deltas) != 1 || rec.Deltas[0].Kind != KindScoreThresholdChange {
		t.Fatalf("deltas = %+v, want one score-threshold delta", rec.Deltas)
	}
	validateRecordJSON(t, rec)
}

func TestBuildComparisonRecordRejectsEmptyCaseID(t *testing.T) {
	in := loadBundle(t)
	p := mkProfile("prod@6", "new >= old", "", policy.EffectBlock)
	res := evaluateMust(t, in, p)
	if _, err := BuildComparisonRecord("", p.Name, p.Name, res, res, []string{testBundleSubject}); err == nil {
		t.Fatal("expected error for empty caseId")
	}
}

func TestBuildComparisonRecordRejectsEmptyProfiles(t *testing.T) {
	in := loadBundle(t)
	p := mkProfile("prod@6", "new >= old", "", policy.EffectBlock)
	res := evaluateMust(t, in, p)
	if _, err := BuildComparisonRecord("c", "", p.Name, res, res, nil); err == nil {
		t.Fatal("expected error for empty baselineProfile")
	}
	if _, err := BuildComparisonRecord("c", p.Name, "", res, res, nil); err == nil {
		t.Fatal("expected error for empty candidateProfile")
	}
}

func TestComparisonRecordScoreThresholdPointsOnly(t *testing.T) {
	in := loadBundleJSON(t, scoreBundleJSON)
	baseline := mkScoreProfileWithPoints("prod@6", 5, 2)
	candidate := mkScoreProfileWithPoints("prod@7", 5, 1)
	baseRes := evaluateMust(t, in, baseline)
	candRes := evaluateMust(t, in, candidate)

	rec, err := BuildComparisonRecord("points", baseline.Name, candidate.Name, baseRes, candRes, []string{testBundleSubject})
	if err != nil {
		t.Fatalf("BuildComparisonRecord: %v", err)
	}
	if len(rec.Deltas) != 1 || rec.Deltas[0].Kind != KindScoreThresholdChange {
		t.Fatalf("deltas = %+v", rec.Deltas)
	}
	if rec.Deltas[0].Baseline.Points == nil || rec.Deltas[0].Candidate.Points == nil {
		t.Fatalf("points-only delta must carry points on both sides: %+v", rec.Deltas[0])
	}
	validateRecordJSON(t, rec)
}

func TestComparisonRecordValidateRejectsInvalidKind(t *testing.T) {
	rec := ComparisonRecord{
		APIVersion:       recordAPIVersion,
		Kind:             recordKind,
		BaselineProfile:  "b@1",
		CandidateProfile: "c@1",
		CaseID:           "c1",
		Deltas: []Delta{{
			Kind:    Kind("bogus"),
			Rule:    "r",
			Subject: testBundleSubject,
			Baseline: OutcomeSide{
				Present:  true,
				Decision: string(aggregate.DecisionBlock),
				Effect:   string(aggregate.EffectBlock),
			},
			Candidate: OutcomeSide{Present: false, Decision: string(aggregate.DecisionApprove)},
		}},
	}
	if err := rec.Validate(); err == nil {
		t.Fatal("expected schema rejection for bogus delta kind")
	}
}

func TestBuildComparisonRecordRejectsUnclassifiable(t *testing.T) {
	in := loadBundle(t)
	baseline := mkProfile("prod@6", "new >= old", "", policy.EffectBlock)
	candidate := mkProfile("prod@7", "new >= old", "", policy.EffectRequireReview)
	baseRes := evaluateMust(t, in, baseline)
	candRes := evaluateMust(t, in, candidate)

	_, err := BuildComparisonRecord("bad", baseline.Name, candidate.Name, baseRes, candRes, []string{testBundleSubject})
	if err == nil {
		t.Fatal("expected fail-closed error for unclassifiable delta")
	}
}

func TestComparisonRecordMarshalSortsDeltasDeterministically(t *testing.T) {
	rec := ComparisonRecord{
		APIVersion:       recordAPIVersion,
		Kind:             recordKind,
		BaselineProfile:  "b@1",
		CandidateProfile: "c@1",
		CaseID:           "sort",
		Deltas: []Delta{
			{Kind: KindExplanationOnly, Rule: "z-rule", Subject: testBundleSubject, Baseline: OutcomeSide{Present: true, Decision: "BLOCK", Effect: "block"}, Candidate: OutcomeSide{Present: true, Decision: "BLOCK", Effect: "block"}},
			{Kind: KindExplanationOnly, Rule: "a-rule", Subject: testBundleSubject, Baseline: OutcomeSide{Present: true, Decision: "BLOCK", Effect: "block"}, Candidate: OutcomeSide{Present: true, Decision: "BLOCK", Effect: "block"}},
		},
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	a := bytes.Index(raw, []byte(`"a-rule"`))
	z := bytes.Index(raw, []byte(`"z-rule"`))
	if a < 0 || z < 0 || a > z {
		t.Fatalf("MarshalJSON must sort deltas by rule, got %s", raw)
	}
}

func TestComparisonRecordDuplicateIdentityDuringCollect(t *testing.T) {
	subject := testBundleSubject
	baseline := aggregate.Result{
		Decision: aggregate.DecisionBlock,
		Findings: []aggregate.Finding{
			{Rule: "wording", Obligation: "o1", Effect: aggregate.EffectBlock, Subject: subject, Code: "c", Message: "baseline-a"},
			{Rule: "wording", Obligation: "o2", Effect: aggregate.EffectBlock, Subject: subject, Code: "c", Message: "baseline-b"},
		},
	}
	candidate := aggregate.Result{
		Decision: aggregate.DecisionBlock,
		Findings: []aggregate.Finding{
			{Rule: "wording", Obligation: "o1", Effect: aggregate.EffectBlock, Subject: subject, Code: "c", Message: "candidate-a"},
			{Rule: "wording", Obligation: "o2", Effect: aggregate.EffectBlock, Subject: subject, Code: "c", Message: "candidate-b"},
		},
	}
	_, err := BuildComparisonRecord("dup-collect", "b@1", "c@1", baseline, candidate, nil)
	if !errors.Is(err, errDuplicateDelta) {
		t.Fatalf("expected duplicate delta error during collect, got %v", err)
	}
}

func TestComparisonRecordMarshalNilDeltasAsEmptyArray(t *testing.T) {
	rec := ComparisonRecord{
		APIVersion:       recordAPIVersion,
		Kind:             recordKind,
		BaselineProfile:  "b@1",
		CandidateProfile: "c@1",
		CaseID:           "c1",
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"deltas":[]`)) {
		t.Fatalf("nil deltas must marshal as empty array, got %s", raw)
	}
}
