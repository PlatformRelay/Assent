package schemas

import "testing"

// TestD016MissingApprovalNeverApproves is REQ-P3-E1-S07-02: a missing required
// ApprovalEvidence must never produce APPROVE, and the PresentationModel finding
// for that obligation names the missing evidence WITHOUT fabricating a
// capabilityGap — missing-approval and capability-gap stay distinguishable
// (S04-03). This MR did not lack the capability; it lacked the actual approval.
func TestD016MissingApprovalNeverApproves(t *testing.T) {
	dr := fixtureObj(t, "decision-record.json")

	t.Run("decision is never APPROVE", func(t *testing.T) {
		decision, _ := dr["decision"].(string)
		if decision != "REVIEW" && decision != "BLOCK" {
			t.Fatalf("missing required approval must yield REVIEW or BLOCK, got %q", decision)
		}
	})

	t.Run("no fabricated capabilityGap (missing-approval != capability-gap)", func(t *testing.T) {
		pins := dr["pins"].(map[string]any)
		if _, has := pins["capabilityGap"]; has {
			t.Error("capabilityGap must be absent: the forge HAD the capability, the MR lacked the actual approval")
		}
		if _, ok := pins["mergeResultDigest"].(string); !ok {
			t.Error("mergeResultDigest must be a pinned string (capability present), not null")
		}
	})

	t.Run("a require-review finding names the missing approval via its code", func(t *testing.T) {
		findingsObj := dr["findings"].(map[string]any)
		findings := findingsObj["enforcing"].([]any)
		named := false
		for _, f := range findings {
			fm := f.(map[string]any)
			if fm["effect"] == "require-review" {
				if code, _ := fm["code"].(string); code != "" {
					named = true
				}
			}
		}
		if !named {
			t.Error("expected a require-review finding whose code names the missing approval")
		}
	})

	t.Run("PresentationModel mirrors the require-review finding without a capabilityGap field", func(t *testing.T) {
		// The PresentationModel finding shape has no capabilityGap field at all
		// (it reuses DecisionRecord.$defs.finding). A missing approval is a
		// require-review finding; a capability gap would live in pins, which
		// PresentationModel does not carry. The two failure modes therefore
		// cannot be conflated in the presentation layer.
		pm := fixtureObj(t, "presentation-model.json")
		findings := pm["findings"].([]any)
		named := false
		for _, f := range findings {
			fm := f.(map[string]any)
			if fm["effect"] == "require-review" {
				named = true
			}
			if _, has := fm["capabilityGap"]; has {
				t.Error("no finding may carry a capabilityGap field (missing-approval is not a capability gap)")
			}
		}
		if !named {
			t.Error("PresentationModel must surface the missing approval as a require-review finding")
		}
	})
}
