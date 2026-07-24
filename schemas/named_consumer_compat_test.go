package schemas

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const compatFixtureDir = "../examples/contracts/named-consumer-compat"

func readCompat(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(compatFixtureDir, name)) //nolint:gosec // fixed test-fixture tree
	if err != nil {
		t.Fatalf("read compat fixture %s: %v", name, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse compat fixture %s: %v", name, err)
	}
	return doc.(map[string]any)
}

// TestNamedConsumerCompatFixtureIsStructured is REQ-P3-E1-S07-03: the one
// sanitized named-consumer compatibility fixture (D-017/B5) represents six
// named-consumer signals as STRUCTURED FIELDS — never inferred from free-text
// message bodies, labels, or rule names:
//
//	1. a rollout phase-shaped value,
//	2. a profile-shaped value,
//	3. a comparison-delta-shaped value,
//	4. an ApprovalEvidence instance (validates against its owning schema),
//	5. a publication-marker-shaped correlation value,
//	6. a budget-reservation-shaped value.
//
// Fields owned by P3-E4/P3-E5 (phase, profile, comparison delta, marker,
// budget) live in a documented placeholder doc whose apiVersion is NOT the
// contract group — those epics own the final field definitions; this fixture
// only proves the fields exist as structured data.
func TestNamedConsumerCompatFixtureIsStructured(t *testing.T) {
	// (4) ApprovalEvidence validates against its owning schema.
	t.Run("ApprovalEvidence instance validates against its schema", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(compatFixtureDir, "approval-evidence.json")) //nolint:gosec // fixed tree
		if err != nil {
			t.Fatalf("read approval-evidence.json: %v", err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("parse approval-evidence.json: %v", err)
		}
		if err := ApprovalEvidenceSchema.Validate(doc); err != nil {
			t.Fatalf("ApprovalEvidence instance fails schema validation: %v", err)
		}
	})

	// The EvaluationInput + DecisionRecord validate against their contract
	// schemas (they are also swept by TestExampleContractsFixturesValidate).
	t.Run("EvaluationInput validates", func(t *testing.T) {
		if err := EvaluationInputSchema.Validate(readCompat(t, "evaluation-input.json")); err != nil {
			t.Fatalf("EvaluationInput fails schema validation: %v", err)
		}
	})
	t.Run("DecisionRecord validates", func(t *testing.T) {
		if err := DecisionRecordSchema.Validate(readCompat(t, "decision-record.json")); err != nil {
			t.Fatalf("DecisionRecord fails schema validation: %v", err)
		}
	})

	// (1,2,3,5,6) The five P3-E4/P3-E5-owned signals are STRUCTURED fields in
	// the placeholder doc — each a typed value under a named key, not prose.
	t.Run("phase/profile/comparison-delta/marker/budget are structured fields", func(t *testing.T) {
		ph := readCompat(t, "named-consumer-signals.json")
		signals, ok := ph["signals"].(map[string]any)
		if !ok {
			t.Fatal("named-consumer-signals.json must carry a structured signals object")
		}

		// (1) rollout phase: a structured string field constrained to the
		//     off/observe/enforce closed set (ADR-0018, P3-E4).
		phase, _ := signals["phase"].(string)
		switch phase {
		case "off", "observe", "enforce":
		default:
			t.Errorf("phase must be a structured off/observe/enforce field, got %q", phase)
		}

		// (2) profile: a structured named profile reference.
		if p, _ := signals["profile"].(map[string]any); p == nil || p["name"] == nil {
			t.Error("profile must be a structured object with a name field")
		}

		// (3) comparison delta: structured counts, not a rendered sentence.
		cd, _ := signals["comparisonDelta"].(map[string]any)
		if cd == nil {
			t.Fatal("comparisonDelta must be a structured object")
		}
		for _, k := range []string{"added", "removed", "changed"} {
			if _, ok := cd[k].(json.Number); !ok {
				if _, ok2 := cd[k].(float64); !ok2 {
					t.Errorf("comparisonDelta.%s must be a structured numeric count", k)
				}
			}
		}

		// (5) publication-marker correlation: a structured correlation id, the
		//     database-free marker protocol's key (ADR-0019, P3-E5).
		if m, _ := signals["publicationMarker"].(map[string]any); m == nil || m["correlationId"] == nil {
			t.Error("publicationMarker must be a structured object with a correlationId field")
		}

		// (6) budget reservation: a structured reservation, not free text.
		br, _ := signals["budgetReservation"].(map[string]any)
		if br == nil || br["reserved"] == nil || br["limit"] == nil {
			t.Error("budgetReservation must be a structured object with reserved/limit fields")
		}
	})

	// Adversarial: no banned inference pattern anywhere in the fixture — a
	// reviewer grepping for the six signals expressed as prose inside a
	// message body, label, or rule name finds none. Structured keys (e.g.
	// "phase": "enforce") are allowed; the token appearing inside a free-text
	// string VALUE for message/label/name is not.
	t.Run("adversarial: no banned inference patterns in free-text values", func(t *testing.T) {
		bannedInText := regexp.MustCompile(`(?i)phase\s*[:=]\s*(off|observe|enforce)|profile\s*[:=]|comparison[- ]?delta|budget\s*[:=]|reservation\s*[:=]`)
		freeTextKeys := map[string]struct{}{
			"message": {}, "label": {}, "labels": {}, "name": {},
			"rule": {}, "reason": {}, "description": {}, "text": {},
		}
		for _, f := range compatFixtureFiles(t) {
			raw, err := os.ReadFile(filepath.Join(compatFixtureDir, f)) //nolint:gosec // fixed tree
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("parse %s: %v", f, err)
			}
			walkFreeText(t, f, "", doc, freeTextKeys, bannedInText)
		}
	})
}

// walkFreeText descends the doc; whenever it is inside a free-text-valued key
// (message/label/name/etc.), it asserts the string value does not smuggle a
// named-consumer signal as prose.
func walkFreeText(t *testing.T, file, key string, node any, freeText map[string]struct{}, banned *regexp.Regexp) {
	t.Helper()
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			walkFreeText(t, file, k, child, freeText, banned)
		}
	case []any:
		for _, child := range v {
			walkFreeText(t, file, key, child, freeText, banned)
		}
	case string:
		if _, isFreeText := freeText[key]; isFreeText && banned.MatchString(v) {
			t.Errorf("%s: free-text %q value %q smuggles a named-consumer signal as prose (must be a structured field)", file, key, v)
		}
	}
}

func compatFixtureFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(compatFixtureDir)
	if err != nil {
		t.Fatalf("read compat dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			out = append(out, e.Name())
		}
	}
	return out
}

// TestNamedConsumerCompatP3E4S05Scenarios is REQ-P3-E4-S05-01..03: observe vs
// enforce twin (identical aggregation fields; findings split), refused
// newly-auto-mergeable when absent from acceptedDeltas, and typed phase/
// profile on evaluation-input.json (never prose-inferred).
func TestNamedConsumerCompatP3E4S05Scenarios(t *testing.T) {
	t.Run("REQ-P3-E4-S05-03: evaluation-input carries typed phase and profile", func(t *testing.T) {
		doc := readCompat(t, "evaluation-input.json")
		phase, _ := doc["phase"].(string)
		switch phase {
		case "off", "observe", "enforce":
		default:
			t.Fatalf("evaluation-input.json must carry typed phase off|observe|enforce, got %v", doc["phase"])
		}
		profile, _ := doc["profile"].(map[string]any)
		if profile == nil || profile["name"] == nil {
			t.Fatal("evaluation-input.json must carry typed profile with name")
		}
	})

	t.Run("REQ-P3-E4-S05-01: observe/enforce twin — identical aggregation, findings split", func(t *testing.T) {
		dr := readCompat(t, "decision-record.json")
		scenarios, _ := dr["compatScenarios"].(map[string]any)
		if scenarios == nil {
			t.Fatal("decision-record.json must carry compatScenarios")
		}
		pair, _ := scenarios["observeVsEnforce"].(map[string]any)
		if pair == nil {
			t.Fatal("compatScenarios.observeVsEnforce required")
		}
		observe, _ := pair["observe"].(map[string]any)
		enforce, _ := pair["enforce"].(map[string]any)
		if observe == nil || enforce == nil {
			t.Fatal("observeVsEnforce must carry observe and enforce case objects")
		}
		if observe["phase"] != "observe" || enforce["phase"] != "enforce" {
			t.Fatalf("twin phases must be observe/enforce, got observe=%v enforce=%v", observe["phase"], enforce["phase"])
		}
		for _, key := range []string{"decision", "blocks", "requiredReviews", "score"} {
			if !reflect.DeepEqual(observe[key], enforce[key]) {
				t.Errorf("%s must be byte-identical between observe and enforce twins: observe=%v enforce=%v", key, observe[key], enforce[key])
			}
		}
		obsFindings, _ := observe["findings"].(map[string]any)
		enfFindings, _ := enforce["findings"].(map[string]any)
		obsObserved, _ := obsFindings["observed"].([]any)
		obsEnforcing, _ := obsFindings["enforcing"].([]any)
		enfObserved, _ := enfFindings["observed"].([]any)
		enfEnforcing, _ := enfFindings["enforcing"].([]any)
		if len(obsObserved) == 0 || len(obsEnforcing) != 0 {
			t.Fatalf("observe case: findings.observed must be populated and findings.enforcing empty; got observed=%d enforcing=%d", len(obsObserved), len(obsEnforcing))
		}
		if len(enfEnforcing) == 0 || len(enfObserved) != 0 {
			t.Fatalf("enforce case: findings.enforcing must be populated and findings.observed empty; got observed=%d enforcing=%d", len(enfObserved), len(enfEnforcing))
		}
		// Adversarial: swap findings arrays without swapping phase — must break
		// the "observed only when phase=observe" invariant documented in the fixture.
		adversarial, _ := pair["adversarialSwapCheck"].(string)
		if adversarial == "" {
			t.Fatal("observeVsEnforce must document adversarialSwapCheck (swap without phase swap fails)")
		}
	})

	t.Run("REQ-P3-E4-S05-02: newly-auto-mergeable refused when not in acceptedDeltas", func(t *testing.T) {
		dr := readCompat(t, "decision-record.json")
		scenarios, _ := dr["compatScenarios"].(map[string]any)
		refused, _ := scenarios["refusedAutoMergeWidening"].(map[string]any)
		if refused == nil {
			t.Fatal("compatScenarios.refusedAutoMergeWidening required")
		}
		delta, _ := refused["delta"].(map[string]any)
		if delta == nil || delta["kind"] != "newly-auto-mergeable" {
			t.Fatalf("refused scenario must carry newly-auto-mergeable delta, got %v", refused["delta"])
		}
		accepted, ok := refused["acceptedDeltas"].([]any)
		if !ok {
			t.Fatal("refused scenario must carry acceptedDeltas array")
		}
		if len(accepted) != 0 {
			t.Fatalf("acceptedDeltas must be empty (widening not allowlisted); got %v", accepted)
		}
		verdict, _ := refused["promotionGateVerdict"].(string)
		if verdict != "fail" {
			t.Fatalf("promotionGateVerdict must be fail (bounded auto-merge widening), got %q", verdict)
		}
	})
}
