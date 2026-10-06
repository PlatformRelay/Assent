package github

import (
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// failclosed_test.go — capability gaps fail closed on GitHub (E10-S12,
// REQ-E10-S12-01/02). Fail-closed claims are worthless untested, so this file
// is the epic's named table proving the POLARITY: for each of the three known
// GitHub deltas — review-dismissal-restrictions, arming-revoked-on-push and
// merge-result-pinning — the armed merge path performs ZERO merges when the
// delta is simulated absent (or unknown), and — the mandatory positive
// control, without which every refusal row passes vacuously on an adapter
// that never arms at all — the all-capabilities-present case yields exactly
// one merge.
//
// The consultation mechanics, stated exactly: the deltas are the ADAPTER's
// arming-path capabilities (merge.go's armingConsultSet), consulted ONLY at
// the adapter's arming verb (EnablePullRequestAutoMerge) — which v1
// production does NOT call: the verb is not on forge.RunPort, the run path's
// arming comes solely from forge.PreconditionFromReport (the ADR-0015 §4/§8
// forge-neutral set), and that run is refused upstream anyway because
// protected-pipeline-source and eligible-approval-evidence grade UNKNOWN
// (arming is refused upstream by the capability report; D-188). So this
// table guards the DELTAS' consultation at the verb they live on — it is not
// a table over the production run path. A promotion of OQ-33/OQ-34 must wire
// the delta consult into the run path or re-derive the armed-path gate
// (D-188), or these rows become a table over dead code while the live path
// merges unconsulted. The test composes the consult outcomes by hand (the
// D-034 seam shape buildDesired would carry), which is what it drives: the
// armed path with the consult's outcome as the precondition. A refused
// consult is carried in as ArmEligible=false, so Reconcile refuses with
// forge.ErrArmingRefused BEFORE any write and MergeCAS is never reached —
// the count this table reads is the transport count (each real MergeCAS puts
// exactly once; a refused one puts never), the only count an in-package
// httptest test can take without the conformance package's countingPort.

// TestDeltasFailClosed is REQ-E10-S12-02's named table.
func TestDeltasFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		delta forge.Capability
		state forge.CapabilityState
	}{
		{"review-dismissal-restrictions-absent", forge.CapabilityReviewDismissalRestrictions, forge.CapabilityAbsent},
		{"arming-revoked-on-push-absent", forge.CapabilityArmingRevokedOnPush, forge.CapabilityAbsent},
		{"arming-revoked-on-push-unknown", forge.CapabilityArmingRevokedOnPush, forge.CapabilityUnknown},
		{"merge-result-pinning-absent", forge.CapabilityMergeResultPinning, forge.CapabilityAbsent},
		{"merge-result-pinning-unknown", forge.CapabilityMergeResultPinning, forge.CapabilityUnknown},
		{"all-capabilities-present", "", ""}, // the mandatory positive control
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &armingHandler{prBody: armingPR, mergeRefSHA: "mCommit", armEnabled: true}
			c, _ := newServer(t, h.ServeHTTP)

			var report forge.CapabilityReport
			if tc.delta == "" {
				report = allSupportedReport()
			} else {
				report = reportWithAllSupportedExcept(tc.delta, tc.state)
			}

			// (1) The deltas' consultation point — the adapter's arming verb.
			//     A refused consult performs ZERO arming mutations, whatever
			//     the repository setting would allow.
			armErr := c.EnablePullRequestAutoMerge(baseRepo, "7", report, MergeMethodMerge)

			// (2) The engine consult, for fidelity with the run path — the
			//     SAME report the arming verb just consulted.
			probe := forge.PreconditionFromReport(report)
			armEligible := probe.ArmEligible && armErr == nil

			// (3) The armed merge path with the consult's outcome injected —
			// the shape the run path passes to Reconcile (buildDesired's
			// D-034 seam). The pins are the forge's current heads.
			source, target, digest, err := c.CurrentHeads(baseRepo, "7")
			if err != nil {
				t.Fatalf("CurrentHeads: %v", err)
			}
			pins := forge.DesiredMerge{
				SourceSha:         source,
				TargetSha:         target,
				MergeResultDigest: digest,
			}
			_, recErr := forge.Reconcile(c, fixedTestClock{}, forge.DesiredReviewState{
				Project: baseRepo,
				MR:      "7",
				Approve: true,
				Merge:   &pins,
			}, forge.Preconditions{
				ArmEligible:       armEligible,
				SourceSha:         pins.SourceSha,
				TargetSha:         pins.TargetSha,
				MergeResultDigest: pins.MergeResultDigest,
			})

			if tc.delta == "" {
				// The positive control: the all-supported consult arms, and
				// the armed merge path approves and merges exactly once —
				// the proof that the refusal rows above are not satisfied by
				// an adapter that never arms at all.
				if armErr != nil {
					t.Fatalf("the all-supported report must arm, got %v", armErr)
				}
				if recErr != nil {
					t.Fatalf("the armed merge must succeed, got %v", recErr)
				}
				if got := h.mergePUTs; got != 1 {
					t.Fatalf("merges = %d, want exactly 1", got)
				}
				if got := h.approvePOSTs; got != 1 {
					t.Fatalf("approvals = %d, want exactly 1", got)
				}
				return
			}

			// The refusal rows: the consult refused, so the armed path is
			// advisory-only with ZERO writes — no approval, and MergeCAS
			// never reached (zero merge PUTs).
			if !errors.Is(armErr, forge.ErrArmingRefused) {
				t.Fatalf("the refused consult must wrap forge.ErrArmingRefused, got %v", armErr)
			}
			if !errors.Is(recErr, forge.ErrArmingRefused) {
				t.Fatalf("the armed path must refuse with the consult's outcome, got %v", recErr)
			}
			if got := h.mergePUTs; got != 0 {
				t.Fatalf("merges = %d, want 0 — an unproven delta must never merge", got)
			}
			if got := h.approvePOSTs; got != 0 {
				t.Fatalf("approvals = %d, want 0 — a refused run writes nothing", got)
			}
			if got := h.armMutations; got != 0 {
				t.Fatalf("arm mutations = %d, want 0 — a refused consult never arms", got)
			}
		})
	}
}

// TestDeltaReportsBuildIsolated is the table's own can-fail control: each
// delta report the rows simulate differs from the all-supported report in
// exactly the delta — so a row that passes on a refusal is refusing on THAT
// delta, not on a blanket.
func TestDeltaReportsBuildIsolated(t *testing.T) {
	for _, delta := range []forge.Capability{
		forge.CapabilityReviewDismissalRestrictions,
		forge.CapabilityArmingRevokedOnPush,
		forge.CapabilityMergeResultPinning,
	} {
		row := reportWithAllSupportedExcept(delta, forge.CapabilityAbsent)
		if got := row.State(delta); got != forge.CapabilityAbsent {
			t.Fatalf("delta %s state = %q, want absent", delta, got)
		}
		for _, c := range forge.AllCapabilities() {
			if c == delta {
				continue
			}
			if row.State(c) != forge.CapabilitySupported {
				t.Fatalf("delta %s's report must leave %s supported, got %q", delta, c, row.State(c))
			}
		}
		// The engine consult still arms on every isolated report: the refusal
		// the rows prove is the ADAPTER's, not a blanket engine refusal.
		if probe := forge.PreconditionFromReport(row); !probe.ArmEligible {
			t.Fatalf("the isolated report for %s must still arm at the engine consult (the deltas are the ADAPTER's gates); refusals=%+v", delta, probe.Refusals)
		}
	}
}
