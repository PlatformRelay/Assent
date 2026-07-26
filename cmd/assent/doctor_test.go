package main

import "testing"

// REQ-P4-E1-S05-01: Given a fixture environment declaring the assent job comes
// from a protected/included config outside the MR branch's control, when doctor
// runs, then it reports the arming precondition MET and the run is permitted to
// arm auto-merge (ADR-0015 §8 matrix: CI-from-protected-config -> may auto-merge).
//
// The description is injected as data (a fake environment), never probed live —
// Doctor takes no os.Getenv (the env boundary stays at cmd/assent's edge; this
// mirrors the S01 adapter discipline of receiving pinned values as data).
func TestDoctorProtectedPipelineArms(t *testing.T) {
	// Fixture: the pipeline configuration is sourced from a protected/included
	// file outside the MR branch's control (compliance pipeline / instance
	// template), and the write token is least-privilege scoped to this project.
	desc := PipelineDescription{
		ConfigProtected:  true,
		ConfigAuthorEdit: false,
		TokenPrivileged:  false,
	}

	report := Doctor(desc)

	if !report.ArmEligible {
		t.Fatalf("protected-config pipeline must be arm-eligible; got advisory-only with reasons %+v", report.Reasons)
	}
	if !report.Capabilities.ProtectedConfigVerified {
		t.Errorf("ProtectedConfigVerified capability = false, want true for a protected-config pipeline")
	}
	// A permitted-to-arm report carries no refusal reasons.
	if len(report.Reasons) != 0 {
		t.Errorf("arm-eligible report must carry no refusal reasons, got %+v", report.Reasons)
	}
}

// REQ-P4-E1-S05-02: Given a fixture environment where the protected-config
// precondition CANNOT be verified (or is author-editable), when doctor runs,
// then it reports advisory-only and the run performs NO approve/merge write —
// doctor refuses to arm (ADR-0015 §4/§8). The write path does not exist yet
// (S06/S08); "performs no write" is expressed as ArmEligible=false / advisory-
// only, the decision S06/S08 will gate their writes on. Fail-safe direction:
// UNVERIFIABLE is treated as NOT-protected, never optimistically armed.
func TestDoctorRefusesArmWhenUnprotected(t *testing.T) {
	// --- Sub-case A: protected-config simply cannot be verified. Default-deny. ---
	unverifiable := PipelineDescription{
		ConfigProtected:  false, // no protected-source signal available
		ConfigAuthorEdit: false,
		TokenPrivileged:  false,
	}
	rA := Doctor(unverifiable)
	if rA.ArmEligible {
		t.Fatalf("unverifiable protected-config must NOT arm (fail-safe default-deny); got arm-eligible")
	}
	if rA.Capabilities.ProtectedConfigVerified {
		t.Errorf("ProtectedConfigVerified must be false when the precondition is unverifiable")
	}
	if !hasReason(rA.Reasons, ReasonProtectedConfigUnverified) {
		t.Errorf("unverifiable case must report %q; got %+v", ReasonProtectedConfigUnverified, rA.Reasons)
	}

	// --- Sub-case B (adversarial): an author-editable .gitlab-ci.yml topology
	// carrying a privileged token. ADR-0015 §4 forbids this exact shape — an
	// author-editable CI job granting itself write authority. It must be
	// reported unsupported/insecure and never arm, with a reason DISTINCT from
	// the generic unverifiable code so the adversarial path is actually modeled. ---
	insecure := PipelineDescription{
		ConfigProtected:  false,
		ConfigAuthorEdit: true, // author controls the .gitlab-ci.yml that runs assent
		TokenPrivileged:  true, // and holds a privileged merge/write token
	}
	rB := Doctor(insecure)
	if rB.ArmEligible {
		t.Fatalf("author-editable config + privileged token must NEVER arm (ADR-0015 §4); got arm-eligible")
	}
	if !hasReason(rB.Reasons, ReasonInsecureTopology) {
		t.Errorf("adversarial author-editable + privileged-token case must report %q (distinct from unverifiable); got %+v",
			ReasonInsecureTopology, rB.Reasons)
	}
	if rB.Capabilities.ProtectedConfigVerified {
		t.Errorf("ProtectedConfigVerified must be false for an author-editable insecure topology")
	}
}

// REQ-P4-E1-S05-02 (author-editable is an INDEPENDENT advisory trigger):
// ADR-0015 §4 makes an author-editable job definition unsupported/insecure
// REGARDLESS of the token — an author who controls the job can escalate its
// authority, so a least-privilege token today is no guarantee. This isolates
// the case that a PROTECTED-but-author-editable pipeline with a NON-privileged
// token must STILL refuse to arm. It fails against a gate that conjoins the
// insecure-topology check on TokenPrivileged.
func TestDoctorRefusesArmWhenAuthorEditableEvenWithoutPrivilegedToken(t *testing.T) {
	desc := PipelineDescription{
		ConfigProtected:  true,  // even a verified protected-source signal ...
		ConfigAuthorEdit: true,  // ... cannot save an author-editable job def ...
		TokenPrivileged:  false, // ... and no privileged token is needed to refuse.
	}
	report := Doctor(desc)
	if report.ArmEligible {
		t.Fatalf("author-editable job must NEVER arm, independent of token (ADR-0015 §4); got arm-eligible")
	}
	if !hasReason(report.Reasons, ReasonInsecureTopology) {
		t.Errorf("author-editable-alone must report %q; got %+v", ReasonInsecureTopology, report.Reasons)
	}
}

// hasReason reports whether the typed reason code appears in the report.
func hasReason(reasons []ArmingReason, code ArmingReasonCode) bool {
	for _, r := range reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}
