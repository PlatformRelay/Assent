package aggregate_test

// d016_reproduce_test.go is the E2-S10 EXIT GATE: it wires the E2-S01..S08 engine
// end-to-end over the FROZEN D-016 strict fixture and proves the real decision
// path REPRODUCES the frozen DecisionRecord — not merely that the fixture is
// shape-valid (schemas/d016_strict_fixture_test.go already checks that), but that
// the engine BEHAVIOURALLY reconstructs decision + findings from its own
// MergePolicy + RulesetBinding + EvaluationInput. This closes the contracts↔engine
// loop (spec §E2-S10, REQ-E2-S10-01..04).
//
// External test package (aggregate_test) — NOT internal `package aggregate` —
// because this file imports internal/core/decision, and decision imports
// internal/core/aggregate; an internal test would be an import cycle. Everything
// consumed here is exported.
//
// PINS handling (spec S10 "THE PINS WRINKLE", option (a)). The golden's pins carry
// values the ENGINE tier cannot deterministically reproduce: toolVersion/toolDigest
// are cmd/assent build values, policySha/mergeResultDigest/source+targetSha are
// hand-authored fixture placeholders (e.g. policySha "sha256:aabb0011…" repeats a
// pattern — it is NOT a real hash of merge-policy.json), and factsResolvedAt is a
// provider-tier pass-through (E5). S10's scope is the DECISION + FINDINGS
// reproduction; pins are "passed through". So we inject the golden's DECLARED pins
// as fixed test inputs (option (a) — legitimate per the spec), constructed as
// LITERALS (not parsed back from the golden, which would make the pins comparison
// vacuously circular). The golden's mergeResultDigest is a STRING with no
// capabilityGap, so we use decision.PinnedMergeResult (run.go uses MergeResultGap
// for GitLab plain-merge — that divergence is the deferred REQ-06/run.go lane, out
// of S10 scope).
//
// COMPARISON semantics. Raw-file byte-equality is IMPOSSIBLE and not what the spec
// asks: (1) the golden is pretty-printed while MarshalRecord emits compact JSON,
// and (2) the golden's AUTHORED finding order (orders require-review, orders block,
// payments) differs from the engine's canonical sort key (subject, rule,
// obligation, code, effect → orders block, orders require-review, payments). "Equals
// the frozen decision-record.json after CANONICAL serialization" means: normalize
// BOTH sides identically (sort the findings arrays + key-sort every object via
// json.Marshal) then compare bytes. canonicalize() below is that normalization; it
// is applied to golden and produced alike, so the golden's authored order does not
// matter. The double-run test asserts RAW MarshalRecord equality across two runs
// (stronger; deterministic because findings are pre-sorted and map keys sort).

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/decision"
	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/schemas"
)

// fixtureDir is the frozen D-016 strict fixture directory, relative to this
// package (internal/core/aggregate → repo root is ../../../).
const fixtureDir = "../../../examples/contracts/d016-strict-fixture"

// goldenPins are the D-016 decision-record.json pins, injected verbatim as fixed
// test inputs (option (a)): they are cmd/assent build values, hand-authored
// fixture placeholders, or provider-tier pass-throughs — none reproducible at the
// engine tier. Kept as literals (not parsed from the golden) so the pins do not
// trivially/circularly match; the DECISION + FINDINGS are what the engine genuinely
// reproduces.
var goldenPins = decision.Pins{
	ToolVersion: "0.1.0",
	ToolDigest:  "sha256:00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
	PolicySha:   "sha256:aabb00112233445566778899ccddeeff00112233445566778899aabbccddeeff",
	SourceSha:   "1f0c3ab9d2e4c5a6b7089a1b2c3d4e5f60718293",
	TargetSha:   "a4b5c6d7e8f9001122334455667788990011aabb",
	MergeResult: mustPinnedDigest("sha256:9f8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a39281706f5e4d3c2b1a0"),
	FactsResolvedAt: map[string]string{
		"owner": "2026-07-24T09:00:00Z",
		"quota": "2026-07-24T09:00:00Z",
	},
}

func mustPinnedDigest(d string) decision.MergeResult {
	mr, err := decision.PinnedMergeResult(d)
	if err != nil {
		panic(err)
	}
	return mr
}

// loadD016 loads the frozen fixture's MergePolicy, RulesetBinding[0], and
// EvaluationInput through the real E2-S01/S02 loaders.
func loadD016(t *testing.T) (*policy.MergePolicy, *policy.Binding, *aggregate.EvaluationInput) {
	t.Helper()
	mp, err := policy.LoadMergePolicy(readFixture(t, "merge-policy.json"))
	if err != nil {
		t.Fatalf("load merge-policy: %v", err)
	}
	rb, err := policy.LoadRulesetBinding(readFixture(t, "ruleset-binding.json"))
	if err != nil {
		t.Fatalf("load ruleset-binding: %v", err)
	}
	if len(rb.Bindings) == 0 {
		t.Fatal("ruleset-binding has no bindings")
	}
	in, err := aggregate.LoadEvaluationInput(readFixture(t, "evaluation-input.json"))
	if err != nil {
		t.Fatalf("load evaluation-input: %v", err)
	}
	return mp, &rb.Bindings[0], in
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixtureDir, name)) //nolint:gosec // hardcoded fixture path
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

// buildD016Record runs the full engine (nil ApprovalEvidence — D-016 case (e)) and
// serializes the DecisionRecord bytes. Returns the aggregate Result too so the
// per-finding REQs can inspect it.
func buildD016Record(t *testing.T) (aggregate.Result, []byte) {
	t.Helper()
	mp, bind, in := loadD016(t)
	res, err := aggregate.CoverWithApproval(mp, bind, in, nil)
	if err != nil {
		t.Fatalf("CoverWithApproval: %v", err)
	}
	report, err := decision.Build(res, goldenPins)
	if err != nil {
		t.Fatalf("decision.Build: %v", err)
	}
	recBytes, err := report.MarshalRecord()
	if err != nil {
		t.Fatalf("MarshalRecord: %v", err)
	}
	return res, recBytes
}

// TestReproduceD016DecisionRecord (REQ-E2-S10-01) is the exit gate: the full engine
// with NO injected ApprovalEvidence reproduces the frozen decision-record.json after
// canonical serialization — decision BLOCK, findings.observed [], and exactly the
// three enforcing findings with their {rule, obligation, effect, subject, points,
// code} values. It also confirms the Result invariants directly before the
// canonical compare so a mismatch is diagnosable at the engine layer, not only the
// serializer.
func TestReproduceD016DecisionRecord(t *testing.T) {
	res, recBytes := buildD016Record(t)

	// Engine-layer invariants (diagnose before the byte compare).
	if res.Decision != aggregate.DecisionBlock {
		t.Fatalf("decision = %q, want BLOCK", res.Decision)
	}
	if len(res.Observed) != 0 {
		t.Fatalf("observed = %+v, want empty (no observe-phase rules in D-016)", res.Observed)
	}
	if len(res.CapabilityGaps) != 0 {
		t.Fatalf("capabilityGaps = %+v, want none (nil ApprovalEvidence is a missing approval, not a capability gap)", res.CapabilityGaps)
	}
	assertD016EnforcingFindings(t, res.Findings)

	// Canonical reproduction: normalize BOTH the produced record and the frozen
	// golden identically (sort findings arrays + key-sort objects) then compare.
	golden := readFixture(t, "decision-record.json")
	gotCanon := canonicalize(t, recBytes)
	wantCanon := canonicalize(t, golden)
	if !bytes.Equal(gotCanon, wantCanon) {
		t.Fatalf("reproduced DecisionRecord != frozen golden after canonical serialization\n got: %s\nwant: %s", gotCanon, wantCanon)
	}
}

// TestReproducedRecordValidatesSchema (REQ-E2-S10-02) proves the reproduced record
// validates against the frozen DecisionRecordSchema — so S10 can never reproduce
// the golden VALUES while drifting the record out of the frozen SHAPE.
func TestReproducedRecordValidatesSchema(t *testing.T) {
	_, recBytes := buildD016Record(t)
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(recBytes))
	if err != nil {
		t.Fatalf("reproduced record is not JSON: %v", err)
	}
	if err := schemas.DecisionRecordSchema.Validate(parsed); err != nil {
		t.Fatalf("reproduced DecisionRecord fails DecisionRecordSchema: %v", err)
	}
}

// TestD016PerSubjectRequireReview (REQ-E2-S10-03) proves the two require-review
// findings are SUBJECT-SCOPED (one per governed subject) with code
// ownership-approval-missing and points 0 — proving E2-S04 per-subject coverage and
// the E2-S07 evidence-absent require-review path compose on the real fixture.
func TestD016PerSubjectRequireReview(t *testing.T) {
	res, _ := buildD016Record(t)

	got := map[string]bool{}
	for _, f := range res.Findings {
		if f.Effect != aggregate.EffectRequireReview {
			continue
		}
		if f.Rule != "topic-owner-must-approve" || f.Obligation != "ownership" {
			t.Errorf("require-review finding for %q has rule=%q obligation=%q, want topic-owner-must-approve/ownership", f.Subject, f.Rule, f.Obligation)
		}
		if f.Code != "ownership-approval-missing" {
			t.Errorf("require-review finding for %q has code=%q, want ownership-approval-missing", f.Subject, f.Code)
		}
		if f.Points != 0 {
			t.Errorf("require-review finding for %q has points=%d, want 0 (the ownership rule authors no points)", f.Subject, f.Points)
		}
		got[f.Subject] = true
	}

	want := []string{
		"topic-registry:orders.events.v1",
		"topic-registry:payments.settled.v2",
	}
	for _, subj := range want {
		if !got[subj] {
			t.Errorf("missing subject-scoped require-review finding for %q", subj)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got require-review findings for %d subjects, want %d (%v)", len(got), len(want), want)
	}
}

// TestD016ReproductionDoubleRunStable (REQ-E2-S10-04) proves the end-to-end
// reproduction is deterministic: two independent runs marshal to BYTE-IDENTICAL
// record bytes (raw, not canonicalized — the stronger check; findings are
// pre-sorted and json map keys sort). A green run also confirms the fixture-fix
// lane (F) landed: the pre-fix `input.new >= input.old` would fail the E2-S02
// undeclared-reference rejection at load, so a reproduced record proves the
// in-scope `new >= old` is present.
func TestD016ReproductionDoubleRunStable(t *testing.T) {
	_, first := buildD016Record(t)
	_, second := buildD016Record(t)
	if !bytes.Equal(first, second) {
		t.Fatalf("double-run not byte-identical\nfirst:  %s\nsecond: %s", first, second)
	}
}

// assertD016EnforcingFindings asserts the produced findings are EXACTLY the three
// frozen enforcing findings (order-independent — matched by their full field tuple).
func assertD016EnforcingFindings(t *testing.T, findings []aggregate.Finding) {
	t.Helper()
	type key struct {
		rule, obligation, effect, subject, code string
		points                                  int
	}
	want := map[key]bool{
		{"topic-owner-must-approve", "ownership", "require-review", "topic-registry:orders.events.v1", "ownership-approval-missing", 0}:    true,
		{"partitions-must-not-shrink", "non-destructive", "block", "topic-registry:orders.events.v1", "partition-count-shrunk", 10}:        true,
		{"topic-owner-must-approve", "ownership", "require-review", "topic-registry:payments.settled.v2", "ownership-approval-missing", 0}: true,
	}
	if len(findings) != len(want) {
		t.Fatalf("got %d enforcing findings, want %d: %+v", len(findings), len(want), findings)
	}
	for _, f := range findings {
		k := key{f.Rule, f.Obligation, string(f.Effect), f.Subject, f.Code, f.Points}
		if !want[k] {
			t.Errorf("unexpected enforcing finding: %+v", f)
			continue
		}
		delete(want, k)
	}
	for k := range want {
		t.Errorf("missing expected enforcing finding: %+v", k)
	}
}

// canonicalize normalizes a DecisionRecord JSON document to a stable byte form for
// order-independent comparison: it sorts the findings.observed / findings.enforcing
// arrays by each element's marshaled bytes, then re-marshals via json.Marshal
// (which sorts every object's keys). Applied identically to both the produced record
// and the frozen golden, so neither the golden's authored finding order nor its
// pretty-printing affects equality. Numbers round-trip through float64 (points 0/10
// only), on both sides, so no precision drift is introduced.
func canonicalize(t *testing.T, raw []byte) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("canonicalize: unmarshal: %v", err)
	}
	if f, ok := doc["findings"].(map[string]any); ok {
		for _, bucket := range []string{"observed", "enforcing"} {
			arr, ok := f[bucket].([]any)
			if !ok {
				continue
			}
			sort.Slice(arr, func(i, j int) bool {
				bi, _ := json.Marshal(arr[i])
				bj, _ := json.Marshal(arr[j])
				return bytes.Compare(bi, bj) < 0
			})
			f[bucket] = arr
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("canonicalize: marshal: %v", err)
	}
	return out
}
