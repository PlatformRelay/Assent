package forge

// DuplicatePrevention records the doctor-reported duplicate-thread guarantee
// per P3-E5 / ADR-0019 (single-writer-serialized vs unserialized-best-effort).
type DuplicatePrevention string

const (
	// DuplicatePreventionSerialized — per-MR serialization mechanism verified.
	DuplicatePreventionSerialized DuplicatePrevention = "single-writer-serialized"
	// DuplicatePreventionBestEffort — safe default when serialization is absent or
	// unverifiable from forge probe data.
	DuplicatePreventionBestEffort DuplicatePrevention = "unserialized-best-effort"
)

// PreconditionRefusalCode is a typed forge-probed arming refusal.
type PreconditionRefusalCode string

const (
	// RefusalInsecureTopology — author-editable in-repo CI with no C17 external config.
	RefusalInsecureTopology PreconditionRefusalCode = "insecure-topology"
	// RefusalDiscussionsGateMissing — C3 merge gate absent.
	RefusalDiscussionsGateMissing PreconditionRefusalCode = "discussions-gate-missing"
	// RefusalTierCapabilityGap — C6/C7 tier lacks enforceable approval rules.
	RefusalTierCapabilityGap PreconditionRefusalCode = "tier-capability-gap"
)

// PreconditionRefusal is one typed refusal with human detail.
type PreconditionRefusal struct {
	Code   PreconditionRefusalCode
	Detail string
}

// PreconditionProbe is the forge-probed capability/precondition report derived
// from Snapshot capability flags (E4-S05). Pure — no network, no env.
type PreconditionProbe struct {
	ArmEligible             bool
	AutoMergeEligible       bool
	DuplicatePrevention     DuplicatePrevention
	ProtectedConfigVerified bool
	Refusals                []PreconditionRefusal
	CapabilityGaps          []CapabilityGapReason
}

// PreconditionFromCapabilities evaluates arming preconditions from forge Snapshot
// capability flags (D-034 forge-probe path). Default-deny: any missing gate or
// tier gap refuses arming with typed reasons.
func PreconditionFromCapabilities(caps CapabilityFlags) PreconditionProbe {
	probe := PreconditionProbe{
		// Snapshot does not yet probe per-MR resource_group serialization — safe
		// default on ambiguity (P3-E5 / spike-secure-setup D15).
		DuplicatePrevention: DuplicatePreventionBestEffort,
	}

	if caps.ProtectedPipelineExternal {
		probe.ProtectedConfigVerified = true
	} else {
		probe.Refusals = append(probe.Refusals, PreconditionRefusal{
			Code:   RefusalInsecureTopology,
			Detail: "CI configuration is author-editable in-repo (.gitlab-ci.yml) with no external/protected config file (forge dossier C17)",
		})
	}

	if !caps.DiscussionsResolvedGate {
		probe.Refusals = append(probe.Refusals, PreconditionRefusal{
			Code:   RefusalDiscussionsGateMissing,
			Detail: "only_allow_merge_if_all_discussions_are_resolved is not enabled (forge dossier C3 / ADR-0009)",
		})
	}

	if !caps.HasApprovalRulesAPI || caps.Tier == TierFree {
		probe.AutoMergeEligible = false
		probe.CapabilityGaps = append(probe.CapabilityGaps, GapFreeTierRequireReview)
		probe.Refusals = append(probe.Refusals, PreconditionRefusal{
			Code:   RefusalTierCapabilityGap,
			Detail: "GitLab tier lacks enforced approval-rules API — require-review evidence is unsatisfiable (forge dossier C6/C7)",
		})
	} else {
		probe.AutoMergeEligible = true
	}

	probe.ArmEligible = len(probe.Refusals) == 0
	return probe
}
