package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge"
)

const doctorEnvInsecureBanner = "INSECURE: env self-assertion — protected-source signals are spoofable by an author-editable CI job; forge probe unavailable (no GITLAB_TOKEN)"

// DoctorFromForgeProbe maps a forge PreconditionProbe into cmd/assent's
// PreconditionReport (ADR-0017 §9 additive fields).
func DoctorFromForgeProbe(probe forge.PreconditionProbe) PreconditionReport {
	report := PreconditionReport{
		ArmEligible:         probe.ArmEligible,
		AutoMergeEligible:   probe.AutoMergeEligible,
		DuplicatePrevention: string(probe.DuplicatePrevention),
		Capabilities: Capabilities{
			ProtectedConfigVerified: probe.ProtectedConfigVerified,
		},
		CapabilityGaps: probe.CapabilityGaps,
	}
	for _, r := range probe.Refusals {
		report.Reasons = append(report.Reasons, ArmingReason{
			Code:   ArmingReasonCode(r.Code),
			Detail: r.Detail,
		})
	}
	return report
}

// DoctorEnvOnly evaluates the env diagnostic path. It mirrors Doctor(desc) but
// never claims forge-verified protected-source (D-034 env placeholder honesty).
func DoctorEnvOnly(desc PipelineDescription) PreconditionReport {
	report := Doctor(desc)
	report.Capabilities.ProtectedConfigVerified = false
	return report
}

// runDoctor is the testable entry point for `assent doctor`. When GITLAB_TOKEN is
// present it forge-probes via Snapshot; otherwise it runs the env-only diagnostic
// path with an explicit INSECURE banner.
func runDoctor(getenv func(string) string, stdout, stderr io.Writer,
	snapshotFactory func(endpoint, token, botAuthor string) forge.Snapshotter) int {
	token := getenv("GITLAB_TOKEN")
	if token != "" {
		project := getenv("CI_PROJECT_ID")
		mr := getenv("CI_MERGE_REQUEST_IID")
		if project == "" || mr == "" {
			_, _ = fmt.Fprintln(stderr, "assent doctor: GITLAB_TOKEN set but CI_PROJECT_ID and CI_MERGE_REQUEST_IID are required for forge probe")
			return 2
		}
		endpoint := normalizeGitLabEndpoint(getenv("CI_API_V4_URL"))
		bot := getenv("ASSENT_BOT_AUTHOR")
		if bot == "" {
			bot = "assent-bot"
		}
		snap := snapshotFactory(endpoint, token, bot)
		snapshot, err := snap.Snapshot(project, mr)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "assent doctor:", err)
			return 2
		}
		report := DoctorFromForgeProbe(forge.PreconditionFromCapabilities(snapshot.Capabilities))
		return emitDoctorReport(report, stdout, stderr, false)
	}

	_, _ = fmt.Fprintln(stderr, doctorEnvInsecureBanner)
	report := DoctorEnvOnly(readPipelineDescription())
	return emitDoctorReport(report, stdout, stderr, true)
}

func normalizeGitLabEndpoint(apiV4URL string) string {
	endpoint := strings.TrimSpace(apiV4URL)
	if endpoint == "" {
		return "https://gitlab.com"
	}
	endpoint = strings.TrimSuffix(endpoint, "/")
	endpoint = strings.TrimSuffix(endpoint, "/api/v4")
	return endpoint
}

func emitDoctorReport(report PreconditionReport, stdout, stderr io.Writer, envOnly bool) int {
	if report.ArmEligible {
		if envOnly {
			_, _ = fmt.Fprintln(stdout, "assent doctor: arming precondition MET (env self-assertion — INSECURE, unverified)")
		} else {
			_, _ = fmt.Fprintln(stdout, "assent doctor: arming precondition MET — forge-probed, auto-merge may be armed")
		}
		return 0
	}
	_, _ = fmt.Fprintln(stderr, "assent doctor: advisory-only — auto-merge NOT armed:")
	for _, r := range report.Reasons {
		_, _ = fmt.Fprintf(stderr, "  - [%s] %s\n", r.Code, r.Detail)
	}
	return 1
}
