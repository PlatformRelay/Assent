package forge

import "fmt"

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

// PreconditionFromReport evaluates arming preconditions from the forge's
// capability report (E10-S04, ADR-0021 item 3). Default-deny: any consultation
// point not PROVEN `supported` refuses arming with a typed reason; `unknown`
// and `absent` refuse identically with distinguishable detail strings
// (REQ-E10-S04-02 — unprobed is not proof).
//
// The arming consultation set is STATED (S00 Q2's scope rule): exactly the three
// ADR-0015 §4/§8 gates precondition.go has always consulted —
// protected-pipeline-source, threads-block-merge, eligible-approval-evidence.
// Every other capability is consulted by its own path or carried to doctor.
func PreconditionFromReport(report CapabilityReport) PreconditionProbe {
	probe := PreconditionProbe{
		// Snapshot does not yet probe per-MR resource_group serialization — safe
		// default on ambiguity (P3-E5 / spike-secure-setup D15).
		DuplicatePrevention: DuplicatePreventionBestEffort,
	}

	if report.ArmedBlocks(CapabilityProtectedPipelineSource) {
		probe.Refusals = append(probe.Refusals, PreconditionRefusal{
			Code:   RefusalInsecureTopology,
			Detail: protectionTopologyRefusalDetail(report),
		})
	} else {
		probe.ProtectedConfigVerified = true
	}

	if report.ArmedBlocks(CapabilityThreadsBlockMerge) {
		probe.Refusals = append(probe.Refusals, PreconditionRefusal{
			Code:   RefusalDiscussionsGateMissing,
			Detail: discussionsGateRefusalDetail(report),
		})
	}

	if report.ArmedBlocks(CapabilityEligibleApprovalEvidence) {
		probe.AutoMergeEligible = false
		probe.CapabilityGaps = append(probe.CapabilityGaps, GapFreeTierRequireReview)
		probe.Refusals = append(probe.Refusals, PreconditionRefusal{
			Code:   RefusalTierCapabilityGap,
			Detail: approvalEvidenceRefusalDetail(report),
		})
	} else {
		probe.AutoMergeEligible = true
	}

	probe.ArmEligible = len(probe.Refusals) == 0
	return probe
}

// protectionTopologyRefusalDetail renders the protected-pipeline-source refusal
// with the state (absent vs unknown) and the adapter's own reason — the two
// states refuse identically but must read differently (REQ-E10-S04-02).
func protectionTopologyRefusalDetail(report CapabilityReport) string {
	return capabilityRefusalDetail(report, CapabilityProtectedPipelineSource,
		"the protected-pipeline-source capability is not proven — the CI configuration that drives assent may be author-editable (ADR-0015 §4)")
}

func discussionsGateRefusalDetail(report CapabilityReport) string {
	return capabilityRefusalDetail(report, CapabilityThreadsBlockMerge,
		"unresolved discussion resolution gate is not proven to block the merge (forge dossier C3 / ADR-0009)")
}

func approvalEvidenceRefusalDetail(report CapabilityReport) string {
	return capabilityRefusalDetail(report, CapabilityEligibleApprovalEvidence,
		"forge-proven eligible approval evidence is unsatisfiable — require-review cannot be proven on this forge (ADR-0017 §3)")
}

// capabilityRefusalDetail composes state + reason into a refusal detail string.
func capabilityRefusalDetail(report CapabilityReport, c Capability, consequence string) string {
	return fmt.Sprintf("capability %q is %s — %s. %s",
		c, capabilityStatePhrase(report.State(c)), consequence, report.Reason(c))
}

// capabilityStatePhrase distinguishes unknown from absent in prose: the two
// states refuse arming identically, but a reader must be able to tell "the
// forge lacks it" from "nobody proved it".
func capabilityStatePhrase(s CapabilityState) string {
	switch s {
	case CapabilityUnknown:
		return "UNPROBED/UNKNOWN"
	default:
		return "ABSENT"
	}
}
