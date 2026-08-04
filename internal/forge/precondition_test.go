package forge

import "testing"

func premiumCaps() CapabilityFlags {
	return CapabilityFlags{
		Tier:                        TierPremium,
		HasApprovalRulesAPI:         true,
		DiscussionsResolvedGate:     true,
		ProtectedPipelineExternal:   true,
		MergeResultDigestRecordable: true,
		MergeTrainAvailable:         true,
	}
}

func hasRefusal(probe PreconditionProbe, code PreconditionRefusalCode) bool {
	for _, r := range probe.Refusals {
		if r.Code == code {
			return true
		}
	}
	return false
}

// Eligible Premium fixture with C3/C6/C7/C17 satisfied → arms.
func TestPreconditionFromCapabilitiesEligible(t *testing.T) {
	probe := PreconditionFromCapabilities(premiumCaps())

	if !probe.ArmEligible {
		t.Fatalf("eligible Premium caps must arm; refusals=%+v", probe.Refusals)
	}
	if !probe.AutoMergeEligible {
		t.Error("AutoMergeEligible must be true when approval-rules API is present")
	}
	if !probe.ProtectedConfigVerified {
		t.Error("ProtectedConfigVerified must be true with external CI config (C17)")
	}
	if len(probe.Refusals) != 0 {
		t.Errorf("eligible probe must carry no refusals; got %+v", probe.Refusals)
	}
	if len(probe.CapabilityGaps) != 0 {
		t.Errorf("eligible probe must carry no capability gaps; got %+v", probe.CapabilityGaps)
	}
	if probe.DuplicatePrevention != DuplicatePreventionBestEffort {
		t.Errorf("DuplicatePrevention = %q, want safe default %q",
			probe.DuplicatePrevention, DuplicatePreventionBestEffort)
	}
}

// C17: author-editable in-repo CI without external config → default-deny.
func TestPreconditionFromCapabilitiesC17Insecure(t *testing.T) {
	caps := premiumCaps()
	caps.ProtectedPipelineExternal = false

	probe := PreconditionFromCapabilities(caps)

	if probe.ArmEligible {
		t.Fatal("author-editable-only CI must not arm (C17)")
	}
	if !hasRefusal(probe, RefusalInsecureTopology) {
		t.Errorf("must refuse with %q; got %+v", RefusalInsecureTopology, probe.Refusals)
	}
	if probe.ProtectedConfigVerified {
		t.Error("ProtectedConfigVerified must be false without external CI config")
	}
}

// C3: missing discussions-resolved merge gate → default-deny.
func TestPreconditionFromCapabilitiesC3Missing(t *testing.T) {
	caps := premiumCaps()
	caps.DiscussionsResolvedGate = false

	probe := PreconditionFromCapabilities(caps)

	if probe.ArmEligible {
		t.Fatal("missing C3 gate must not arm")
	}
	if !hasRefusal(probe, RefusalDiscussionsGateMissing) {
		t.Errorf("must refuse with %q; got %+v", RefusalDiscussionsGateMissing, probe.Refusals)
	}
}

// C6/C7: Free tier / absent approval-rules API → typed tier gap, never arms.
func TestPreconditionFromCapabilitiesC6C7TierGap(t *testing.T) {
	t.Run("free tier", func(t *testing.T) {
		caps := premiumCaps()
		caps.Tier = TierFree
		caps.HasApprovalRulesAPI = false

		probe := PreconditionFromCapabilities(caps)
		assertTierGap(t, probe)
	})

	t.Run("approval rules API absent on premium-shaped caps", func(t *testing.T) {
		caps := premiumCaps()
		caps.HasApprovalRulesAPI = false

		probe := PreconditionFromCapabilities(caps)
		assertTierGap(t, probe)
	})
}

func assertTierGap(t *testing.T, probe PreconditionProbe) {
	t.Helper()
	if probe.ArmEligible {
		t.Fatal("tier gap must refuse arming")
	}
	if probe.AutoMergeEligible {
		t.Error("AutoMergeEligible must be false on tier gap")
	}
	if !hasRefusal(probe, RefusalTierCapabilityGap) {
		t.Errorf("must refuse with %q; got %+v", RefusalTierCapabilityGap, probe.Refusals)
	}
	if len(probe.CapabilityGaps) != 1 || probe.CapabilityGaps[0] != GapFreeTierRequireReview {
		t.Errorf("CapabilityGaps = %v, want [%q]", probe.CapabilityGaps, GapFreeTierRequireReview)
	}
}
