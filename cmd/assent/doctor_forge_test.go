package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/gitlab"
)

const (
	forgeDoctorProject = "42"
	forgeDoctorMR      = "7"
	forgeDoctorToken   = "doctor-forge-test-token" //nolint:gosec // test fixture
)

const premiumProjectJSON = `{
	"only_allow_merge_if_all_discussions_are_resolved":true,
	"merge_trains_enabled":true,
	"ci_config_path":".gitlab-ci.yml@group/external-ci"
}`

func forgeDoctorHandler(t *testing.T, projectJSON string, approvalRulesStatus int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("PRIVATE-TOKEN"); got != forgeDoctorToken {
			t.Errorf("PRIVATE-TOKEN = %q, want test token", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7":
			_, _ = w.Write([]byte(`{"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main","author":{"username":"alice"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			_, _ = w.Write([]byte(`{"commit":{"id":"tgtTIP"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7/changes":
			_, _ = w.Write([]byte(`{"changes":[{"old_path":"a.go","new_path":"a.go"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42":
			_, _ = w.Write([]byte(projectJSON))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7/approval_rules":
			if approvalRulesStatus == http.StatusNotFound {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`[{"id":1,"name":"default","approvals_required":1}]`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/merge_requests/7/discussions"):
			_, _ = w.Write([]byte(`[]`))
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}
}

func doctorReportFromForgeHandler(t *testing.T, h http.HandlerFunc) PreconditionReport {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	t.Setenv("GITLAB_TOKEN", forgeDoctorToken)
	t.Setenv("CI_PROJECT_ID", forgeDoctorProject)
	t.Setenv("CI_MERGE_REQUEST_IID", forgeDoctorMR)
	t.Setenv("CI_API_V4_URL", srv.URL+"/api/v4")

	client := gitlab.New(srv.URL, forgeDoctorToken, "assent-bot")
	snap, err := client.Snapshot(forgeDoctorProject, forgeDoctorMR)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	return DoctorFromForgeProbe(forge.PreconditionFromCapabilities(snap.Capabilities))
}

// REQ-E4-S05-01: missing discussions-resolved gate → ArmEligible=false.
func TestDoctorForgeMissingDiscussionGate(t *testing.T) {
	projectJSON := `{
		"only_allow_merge_if_all_discussions_are_resolved":false,
		"merge_trains_enabled":true,
		"ci_config_path":".gitlab-ci.yml@group/external-ci"
	}`
	report := doctorReportFromForgeHandler(t, forgeDoctorHandler(t, projectJSON, http.StatusOK))

	if report.ArmEligible {
		t.Fatal("missing C3 discussions gate must refuse arming (ArmEligible=false)")
	}
	if !hasReason(report.Reasons, ReasonDiscussionsGateMissing) {
		t.Errorf("must report %q; got %+v", ReasonDiscussionsGateMissing, report.Reasons)
	}
}

// REQ-E4-S05-02: tier gap → typed capability gap, never arms.
func TestDoctorForgeTierGap(t *testing.T) {
	report := doctorReportFromForgeHandler(t, forgeDoctorHandler(t, premiumProjectJSON, http.StatusNotFound))

	if report.ArmEligible {
		t.Fatal("Free-tier gap must refuse arming (ArmEligible=false)")
	}
	if report.AutoMergeEligible {
		t.Error("AutoMergeEligible must be false on tier gap")
	}
	if !hasReason(report.Reasons, ReasonTierCapabilityGap) {
		t.Errorf("must report typed tier gap %q; got %+v", ReasonTierCapabilityGap, report.Reasons)
	}
	if len(report.CapabilityGaps) == 0 || report.CapabilityGaps[0] != forge.GapFreeTierRequireReview {
		t.Errorf("CapabilityGaps = %v, want [%q]", report.CapabilityGaps, forge.GapFreeTierRequireReview)
	}
}

// REQ-E4-S05-03: env-only doctor without forge prints INSECURE banner.
func TestDoctorEnvOnlyInsecureBanner(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("ASSENT_PIPELINE_CONFIG_PROTECTED", "true")
	t.Setenv("ASSENT_PIPELINE_CONFIG_AUTHOR_EDITABLE", "false")
	t.Setenv("ASSENT_PIPELINE_TOKEN_PRIVILEGED", "false")

	code, stdout, stderr := captureRun(t, []string{"doctor"})
	if code != 0 {
		t.Fatalf("env-only diagnostic may still report arm-eligible; exit = %d stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "INSECURE") {
		t.Errorf("env-only doctor must print INSECURE banner; stderr=%q", stderr)
	}
	if strings.Contains(stdout, "forge-probed") {
		t.Errorf("env-only must not claim forge-probed verification; stdout=%q", stdout)
	}
	if strings.Contains(strings.ToLower(stdout+stderr), "verified protected") {
		t.Errorf("env-only must not claim verified protected-source; out=%q err=%q", stdout, stderr)
	}
	report := DoctorEnvOnly(PipelineDescription{ConfigProtected: true, ConfigAuthorEdit: false})
	if report.Capabilities.ProtectedConfigVerified {
		t.Error("env-only path must not set ProtectedConfigVerified=true")
	}
}

// REQ-E4-S05-04: duplicate_prevention guarantee field populated per P3-E5.
func TestDoctorDuplicatePreventionReport(t *testing.T) {
	report := doctorReportFromForgeHandler(t, forgeDoctorHandler(t, premiumProjectJSON, http.StatusNotFound))
	if report.DuplicatePrevention != DuplicatePreventionBestEffort {
		t.Errorf("DuplicatePrevention = %q, want %q (safe default when serialization unverifiable)",
			report.DuplicatePrevention, DuplicatePreventionBestEffort)
	}

	eligible := doctorReportFromForgeHandler(t, forgeDoctorHandler(t, premiumProjectJSON, http.StatusOK))
	if eligible.DuplicatePrevention != DuplicatePreventionBestEffort {
		t.Errorf("DuplicatePrevention = %q, want %q on eligible fixture too",
			eligible.DuplicatePrevention, DuplicatePreventionBestEffort)
	}
}

// REQ-E4-S05-05: author-editable-only CI (no C17 external config) → insecure topology.
func TestDoctorForgeInsecureCITopology(t *testing.T) {
	projectJSON := `{
		"only_allow_merge_if_all_discussions_are_resolved":true,
		"merge_trains_enabled":true,
		"ci_config_path":".gitlab-ci.yml"
	}`
	report := doctorReportFromForgeHandler(t, forgeDoctorHandler(t, projectJSON, http.StatusOK))

	if report.ArmEligible {
		t.Fatal("author-editable-only CI must refuse arming (ArmEligible=false)")
	}
	if !hasReason(report.Reasons, ReasonInsecureTopology) {
		t.Errorf("must report %q for in-repo CI; got %+v", ReasonInsecureTopology, report.Reasons)
	}
	if report.Capabilities.ProtectedConfigVerified {
		t.Error("ProtectedConfigVerified must be false for author-editable-only CI")
	}
}
