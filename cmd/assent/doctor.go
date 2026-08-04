package main

import (
	"os"

	"github.com/PlatformRelay/assent/internal/forge"
)

// PipelineDescription is the trust-relevant description of the pipeline running
// assent: whether its configuration comes from a PROTECTED source outside the
// MR branch's control, whether the author can edit that configuration, and
// whether the job holds a privileged (merge/write) token. It is injected as
// data — Doctor never probes a live pipeline — mirroring the S01 CI-env adapter
// discipline that keeps env reading at cmd/assent's edge (ADR-0015 §1,
// GUIDELINES §5). readPipelineDescription (below) is the single os.Getenv
// boundary for these signals; Doctor takes the resolved struct by value.
type PipelineDescription struct {
	// ConfigProtected is true only when the pipeline configuration is VERIFIED
	// to come from a protected/included file outside the MR branch's control
	// (compliance pipeline / instance template / merge-request-approved
	// pipeline; ADR-0015 §4). When the signal is absent or cannot be verified,
	// this is false — the fail-safe default is NOT-protected.
	ConfigProtected bool
	// ConfigAuthorEdit is true when the assent job definition (e.g. an
	// author-editable .gitlab-ci.yml) is under the MR author's control. Such a
	// job can grant itself write authority — the F5 topology ADR-0015 §4
	// forbids.
	ConfigAuthorEdit bool
	// TokenPrivileged is true when the job holds a privileged approve/merge
	// (write) token. Combined with an author-editable config it is the exact
	// insecure topology ADR-0015 §4 calls out as unsupported.
	TokenPrivileged bool
}

// ArmingReasonCode is a typed, machine-readable reason an arming precondition
// failed. Typed rather than free-text so S06/S08 (and the presentation layer)
// can branch on the specific failure without string matching.
type ArmingReasonCode string

const (
	// ReasonProtectedConfigUnverified: the protected-config precondition could
	// not be verified. Fail-safe default-deny (ADR-0015 §4/§8) — unverifiable
	// is treated as NOT-protected, never optimistically armed.
	ReasonProtectedConfigUnverified ArmingReasonCode = "protected-config-unverified"
	// ReasonInsecureTopology: the assent job runs from an author-editable job
	// definition — the F5 topology ADR-0015 §4 forbids (an author-editable CI
	// job can grant itself write authority). Reported unsupported/insecure and
	// never armed INDEPENDENT of the token; distinct from the generic unverified
	// code so the adversarial path is explicit.
	ReasonInsecureTopology ArmingReasonCode = "insecure-topology"
	// ReasonDiscussionsGateMissing: forge probe reports C3 discussions-resolved
	// merge gate absent (E4-S05 / ADR-0009).
	ReasonDiscussionsGateMissing ArmingReasonCode = "discussions-gate-missing"
	// ReasonTierCapabilityGap: forge probe detects a tier gap (C6/C7) — auto-
	// merge and require-review proof are unsatisfiable on this tier.
	ReasonTierCapabilityGap ArmingReasonCode = "tier-capability-gap"
)

// ArmingReason is one typed refusal reason with a human-readable detail.
type ArmingReason struct {
	Code   ArmingReasonCode
	Detail string
}

// Capabilities are the boolean precondition/capability flags doctor verified
// (ADR-0017 §9: a typed capability/precondition report). Additive: later slices
// add flags (e.g. SHA-guard, merge-result pinning) without breaking this shape.
type Capabilities struct {
	// ProtectedConfigVerified mirrors the verified protected-source precondition
	// (ADR-0015 §4). Only true when the pipeline config is proven to come from a
	// protected source that the MR branch cannot edit.
	ProtectedConfigVerified bool
}

// DuplicatePrevention values mirror P3-E5 / ADR-0019 doctor reporting.
const (
	DuplicatePreventionSerialized = "single-writer-serialized"
	DuplicatePreventionBestEffort = "unserialized-best-effort"
)

// PreconditionReport is doctor's typed capability/precondition report and the
// arming DECISION derived from it (ADR-0017 §9, ADR-0015 §8 execution-authority
// matrix). ArmEligible is the single gate S06/S08 will consult before any
// approve/merge WRITE: when false the run is advisory-only (report, no writes).
// The write path does not exist yet; expressing "performs no write" as
// ArmEligible=false is deliberate — it is the decision the write path gates on.
type PreconditionReport struct {
	// ArmEligible is true only when EVERY arming precondition is met. Default is
	// false (advisory-only) — arming is opt-in, refusal is the fail-safe.
	ArmEligible bool
	// AutoMergeEligible is false when forge tier gaps block require-review proof
	// (C6/C7). Additive field for S06/S08 capability honesty (D-075).
	AutoMergeEligible bool
	// DuplicatePrevention reports the P3-E5 duplicate-thread guarantee level.
	DuplicatePrevention string
	// Reasons lists the typed refusals when not arm-eligible; empty when armed.
	Reasons []ArmingReason
	// CapabilityGaps lists typed tier gaps from forge probe (never silent APPROVE).
	CapabilityGaps []forge.CapabilityGapReason
	// Capabilities are the verified precondition flags (typed report, §9).
	Capabilities Capabilities
}

// Doctor evaluates the arming preconditions from an injected pipeline
// description and returns a typed precondition report (ADR-0015 §4/§8,
// ADR-0017 §9). It is pure: no os.Getenv, no live probe, no clock, no network —
// the description carries every signal it needs, so it is testable against a
// fake environment. The policy is DEFAULT-DENY:
//
//   - An author-editable job definition is the insecure F5 topology (ADR-0015
//     §4) — reported unsupported/insecure and NEVER armed, INDEPENDENT of the
//     token (an author who controls the job can escalate the token; a
//     least-privilege token today is no guarantee). A privileged token adds a
//     further escalation detail but is not required to refuse.
//   - A protected-config signal that cannot be verified is treated as
//     NOT-protected — advisory-only, never optimistically armed.
//   - Only a VERIFIED protected-source config arms auto-merge (§8 matrix:
//     CI-from-protected-config MAY auto-merge).
func Doctor(desc PipelineDescription) PreconditionReport {
	report := PreconditionReport{
		Capabilities: Capabilities{
			ProtectedConfigVerified: desc.ConfigProtected,
		},
	}

	// Adversarial topology first: an author-editable job definition is
	// unsupported/insecure on its own, INDEPENDENT of the token — an author who
	// controls the job that runs assent can grant it merge authority, which
	// ADR-0015 §4 forbids. REQ-S05-02 names author-editable as an independent
	// advisory trigger. A privileged token is an additional escalation detail,
	// never a precondition for refusing.
	if desc.ConfigAuthorEdit {
		detail := "author-editable job definition; an author-editable CI job must not grant itself merge authority (ADR-0015 §4)"
		if desc.TokenPrivileged {
			detail += " — and it holds a privileged write token (escalation)"
		}
		report.Reasons = append(report.Reasons, ArmingReason{
			Code:   ReasonInsecureTopology,
			Detail: detail,
		})
	}

	// Fail-safe default-deny: without a VERIFIED protected-source config the
	// run is advisory-only. Unverifiable is treated as NOT-protected.
	if !desc.ConfigProtected {
		report.Reasons = append(report.Reasons, ArmingReason{
			Code:   ReasonProtectedConfigUnverified,
			Detail: "pipeline configuration is not verified to come from a protected source outside the MR branch's control (ADR-0015 §4/§8)",
		})
	}

	// Arm only when nothing refused it.
	report.ArmEligible = len(report.Reasons) == 0
	return report
}

// readPipelineDescription is the single os.Getenv boundary for the trust
// signals Doctor consumes, mirroring adapter.go's readCIEnv (the env boundary
// stays in cmd/assent; internal/core and internal/change never read env,
// ADR-0015 §1). The two L1 tests inject a PipelineDescription directly and do
// not exercise this reader.
//
// INSECURE PLACEHOLDER — this reader trusts env vars an author-editable CI can
// set itself (an author-editable .gitlab-ci.yml can `export
// ASSENT_PIPELINE_CONFIG_PROTECTED=true`); it is NOT real protected-source
// verification and MUST NOT be relied on for arming in production — that is the
// exact self-assertion ADR-0015 §4 forbids. Real protected-source verification
// (GitLab forge-capability probing) is deferred to a later slice (E4/E5) and
// MUST replace this reader before any approve/merge write is gated on
// ArmEligible. See INBOX OQ 2026-07-26.
func readPipelineDescription() PipelineDescription {
	return PipelineDescription{
		ConfigProtected: os.Getenv("ASSENT_PIPELINE_CONFIG_PROTECTED") == "true",
		// NOTE (unsafe-direction default): absent -> false. Contained today
		// (arming also requires ConfigProtected=true, and this whole reader is
		// under the INSECURE-PLACEHOLDER banner above). The real verifier (E4/E5)
		// must default absent/unknown to DENY (author-editable) — fail-safe.
		ConfigAuthorEdit: os.Getenv("ASSENT_PIPELINE_CONFIG_AUTHOR_EDITABLE") == "true",
		TokenPrivileged:  os.Getenv("ASSENT_PIPELINE_TOKEN_PRIVILEGED") == "true",
	}
}
