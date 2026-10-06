package github

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
)

// merge_test.go — deferred arming and the merge-queue pin (E10-S11,
// REQ-E10-S11-02/03). The arming verb consults the arming-path capabilities
// BEFORE any write; the merge-result digest axis is read only under a clean
// mergeable_state, so a merge-queue PR never gets a fabricated pin.

// queuedPR is the merge-queue shape (dossier C14): the PR is queued —
// mergeable_state "queued" — and the merge ref refs/pull/7/merge is absent,
// because the queue's temporary gh-readonly-queue branch, not the PR, holds
// the merge result.
const queuedPR = `{"number":7,"sha":"srcSHA","node_id":"PR_node7","user":{"login":"octocat"},"labels":[],` +
	`"base":{"ref":"main","sha":"tgtTIP","repo":{"full_name":"octo-org/base-repo"}},` +
	`"head":{"ref":"feature","sha":"srcSHA","repo":{"full_name":"octo-org/base-repo"}},"mergeable_state":"queued"}`

// prWithoutNodeID is a PR object stripped of its GraphQL node id — the forge
// did not address the pull request, so the arming mutation cannot either.
const prWithoutNodeID = `{"number":7,"sha":"srcSHA","user":{"login":"octocat"},"labels":[],` +
	`"base":{"ref":"main","sha":"tgtTIP","repo":{"full_name":"octo-org/base-repo"}},` +
	`"head":{"ref":"feature","sha":"srcSHA","repo":{"full_name":"octo-org/base-repo"}},"mergeable_state":"clean"}`

// armingPR is the same-repo PR with the GraphQL node id the arming mutation
// addresses the pull request by (dossier C11).
const armingPR = `{"number":7,"sha":"srcSHA","node_id":"PR_node7","user":{"login":"octocat"},"labels":[],` +
	`"base":{"ref":"main","sha":"tgtTIP","repo":{"full_name":"octo-org/base-repo"}},` +
	`"head":{"ref":"feature","sha":"srcSHA","repo":{"full_name":"octo-org/base-repo"}},"mergeable_state":"clean"}`

// armingHandler serves the route set the arming verb and the merge-queue pin
// tests drive: the PR read (with the arming node id and the mergeable state),
// the repo settings probe, the pull-files pages, the review-comment listing,
// the merge ref, the merge PUT with its CAS, and the GraphQL endpoint (the
// arming mutation, counted — a refused consult must never reach it).
type armingHandler struct {
	prBody      string
	mergeRefSHA string
	armEnabled  bool

	prReads      int
	armMutations int
	mergePUTs    int
	approvePOSTs int
	armReqQuery  string
}

func (h *armingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
		h.prReads++
		body := h.prBody
		if body == "" {
			http.Error(w, "unexpected empty PR fixture", http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, body)
	case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo":
		_, _ = io.WriteString(w, `{"allow_auto_merge":true}`)
	case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/files":
		_, _ = io.WriteString(w, `[{"filename":"topics/orders.yaml"}]`)
	case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/comments":
		_, _ = io.WriteString(w, "[]")
	case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/git/ref/refs/pull/7/merge":
		if h.mergeRefSHA == "" {
			http.Error(w, "no merge ref", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"ref":"refs/pull/7/merge","object":{"sha":"`+h.mergeRefSHA+`","type":"commit"}}`)
	case r.Method == http.MethodPut && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/merge":
		h.mergePUTs++
		var body struct {
			SHA string `json:"sha"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.SHA != "srcSHA" {
			http.Error(w, "sha mismatch", http.StatusConflict)
			return
		}
		_, _ = io.WriteString(w, `{"merged":true,"merge_commit_sha":"`+h.mergeRefSHA+`"}`)
	case r.Method == http.MethodPost && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/reviews":
		h.approvePOSTs++
		_, _ = io.WriteString(w, `{"id":9001,"state":"APPROVED","user":{"login":"assent-bot"}}`)
	case r.Method == http.MethodPost && r.URL.Path == "/graphql":
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if strings.Contains(req.Query, "reviewThreads") {
			_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}}}`)
			return
		}
		if !strings.Contains(req.Query, "enablePullRequestAutoMerge") {
			http.Error(w, "unexpected graphql operation", http.StatusInternalServerError)
			return
		}
		h.armMutations++
		h.armReqQuery = req.Query
		if !h.armEnabled {
			_, _ = io.WriteString(w, `{"data":{"enablePullRequestAutoMerge":{"pullRequest":{"autoMergeRequest":{"enabled":false,"mergeMethod":"MERGE"}}}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"enablePullRequestAutoMerge":{"pullRequest":{"autoMergeRequest":{"enabled":true,"mergeMethod":"MERGE"}}}}}`)
	default:
		unexpectedEndpoint(w, r)
	}
}

// allSupportedReport is the fixture report the arming cassette and the
// fail-closed table's positive control run under: every member of the closed
// enum is probe-proven supported.
func allSupportedReport() forge.CapabilityReport {
	entries := map[forge.Capability]forge.CapabilityEntry{}
	for _, c := range forge.AllCapabilities() {
		entries[c] = forge.SupportedCapabilityEntry("fixture: probe proven")
	}
	report, err := forge.NewCapabilityReport(entries)
	if err != nil {
		panic(err)
	}
	return report
}

// reportWithAllSupportedExcept builds the report one delta row simulates:
// every capability supported EXCEPT `delta`, graded at `state` — so the delta
// under test is the only thing an arming consult can trip on.
func reportWithAllSupportedExcept(delta forge.Capability, state forge.CapabilityState) forge.CapabilityReport {
	entries := map[forge.Capability]forge.CapabilityEntry{}
	for _, c := range forge.AllCapabilities() {
		if c == delta {
			switch state {
			case forge.CapabilityUnknown:
				entries[c] = forge.UnknownCapabilityEntry("simulated: unverified against a live response (S00 Q2)")
			default:
				entries[c] = forge.AbsentCapabilityEntry("simulated absent for this row")
			}
			continue
		}
		entries[c] = forge.SupportedCapabilityEntry("fixture: probe proven")
	}
	built, err := forge.NewCapabilityReport(entries)
	if err != nil {
		panic(err)
	}
	return built
}

// TestArmingRevokeOnPush is REQ-E10-S11-02: arming uses
// enablePullRequestAutoMerge, and the arming is REFUSED unless every
// arming-path capability is proven supported — including arming-revoked-on-
// push, because a write-access push does NOT auto-disarm GitHub auto-merge
// (dossier §3 delta 2): without a proven revocation signal the armed merge
// could survive an unevaluated push, which is GitLab-equivalent safety lost.
// In the adapter's honest v1 report that capability is unknown ⇒ arming is
// refused — the product limitation, proven here rather than assumed.
func TestArmingRevokeOnPush(t *testing.T) {
	t.Run("v1_honest_report_refuses_arming", func(t *testing.T) {
		h := &armingHandler{prBody: armingPR, mergeRefSHA: "mrgSHA", armEnabled: true}
		c, _ := newServer(t, h.ServeHTTP)

		// The honest report comes from the adapter's own probe chain (the run
		// path consults the same report it renders).
		snap, err := c.Snapshot(baseRepo, "7")
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		err = c.EnablePullRequestAutoMerge(baseRepo, "7", snap.Capabilities, MergeMethodMerge)
		if !errors.Is(err, forge.ErrArmingRefused) {
			t.Fatalf("the v1 report (revocation signal unverified) must refuse arming, got %v", err)
		}
		if !strings.Contains(err.Error(), "arming-revoked-on-push") {
			t.Errorf("the refusal must name the revocation capability, got %v", err)
		}
		if !strings.Contains(err.Error(), "unknown") {
			t.Errorf("the refusal must carry the capability state, got %v", err)
		}
		if got := h.armMutations; got != 0 {
			t.Fatalf("a refused arming must never reach the mutation, got %d mutation(s)", got)
		}
	})

	t.Run("revocation_signal_absent_refuses_even_when_the_repo_allows", func(t *testing.T) {
		h := &armingHandler{prBody: armingPR, mergeRefSHA: "mrgSHA", armEnabled: true}
		c, _ := newServer(t, h.ServeHTTP)
		report := reportWithAllSupportedExcept(forge.CapabilityArmingRevokedOnPush, forge.CapabilityAbsent)

		err := c.EnablePullRequestAutoMerge(baseRepo, "7", report, MergeMethodMerge)
		if !errors.Is(err, forge.ErrArmingRefused) {
			t.Fatalf("an absent revocation signal must refuse arming, got %v", err)
		}
		if !strings.Contains(err.Error(), "arming-revoked-on-push") {
			t.Errorf("the refusal must name the refusing capability, got %v", err)
		}
		if got := h.armMutations; got != 0 {
			t.Fatalf("a refused arming must issue zero mutations, got %d", got)
		}
	})

	t.Run("all_supported_arms_the_pull_request", func(t *testing.T) {
		// The mandatory positive control: with every arming-path capability
		// supported, the GraphQL mutation runs and the forge confirms it.
		h := &armingHandler{prBody: armingPR, mergeRefSHA: "mrgSHA", armEnabled: true}
		c, _ := newServer(t, h.ServeHTTP)

		if err := c.EnablePullRequestAutoMerge(baseRepo, "7", allSupportedReport(), MergeMethodMerge); err != nil {
			t.Fatalf("the all-supported report must arm, got %v", err)
		}
		if got := h.armMutations; got != 1 {
			t.Fatalf("arming mutations = %d, want exactly the one confirmed mutation", got)
		}
	})

	t.Run("unconfirmed_arming_fails_closed", func(t *testing.T) {
		h := &armingHandler{prBody: armingPR, mergeRefSHA: "mrgSHA", armEnabled: false}
		c, _ := newServer(t, h.ServeHTTP)

		if err := c.EnablePullRequestAutoMerge(baseRepo, "7", allSupportedReport(), MergeMethodMerge); err == nil {
			t.Fatal("a mutation answering without enabled:true must fail closed")
		}
	})

	t.Run("unaddressed_pr_never_arms", func(t *testing.T) {
		h := &armingHandler{prBody: prWithoutNodeID, mergeRefSHA: "mrgSHA", armEnabled: true}
		c, _ := newServer(t, h.ServeHTTP)

		if err := c.EnablePullRequestAutoMerge(baseRepo, "7", allSupportedReport(), MergeMethodMerge); err == nil {
			t.Fatal("a PR the forge did not address (no node id) must fail closed")
		}
		if got := h.armMutations; got != 0 {
			t.Fatalf("no mutation may be issued without an addressable PR, got %d", got)
		}
	})

	t.Run("unknown_merge_method_refuses_before_any_request", func(t *testing.T) {
		h := &armingHandler{prBody: armingPR, mergeRefSHA: "mrgSHA", armEnabled: true}
		c, _ := newServer(t, h.ServeHTTP)

		if err := c.EnablePullRequestAutoMerge(baseRepo, "7", allSupportedReport(), MergeMethod("amend")); err == nil {
			t.Fatal("an unknown merge method must refuse")
		}
		if got := h.armMutations; got != 0 {
			t.Fatalf("a refused arming must issue zero mutations, got %d", got)
		}
	})
}

// TestDeltasRefuseArmingAtTheVerb is the per-delta refusal at the arming
// verb's own consultation point (each row: the delta simulated absent or
// unknown, everything else supported — the verb refuses naming the capability
// and its state, before any write).
func TestDeltasRefuseArmingAtTheVerb(t *testing.T) {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &armingHandler{prBody: armingPR, mergeRefSHA: "mrgSHA", armEnabled: true}
			c, _ := newServer(t, h.ServeHTTP)

			report := reportWithAllSupportedExcept(tc.delta, tc.state)
			err := c.EnablePullRequestAutoMerge(baseRepo, "7", report, MergeMethodMerge)
			if !errors.Is(err, forge.ErrArmingRefused) {
				t.Fatalf("delta %s at %s must refuse arming, got %v", tc.delta, tc.state, err)
			}
			if !strings.Contains(err.Error(), string(tc.delta)) {
				t.Errorf("the refusal must name the delta, got %v", err)
			}
			if got := h.armMutations; got != 0 {
				t.Fatalf("a refused arming must issue zero mutations, got %d", got)
			}
			if got := h.prReads; got != 0 {
				t.Fatalf("a refused arming must issue zero reads, got %d", got)
			}
		})
	}
}

// fixedTestClock is the frozen clock forge.Reconcile stamps receipts with
// (determinism: no wall-clock in assertions).
type fixedTestClock struct{}

func (fixedTestClock) Now() time.Time {
	return time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
}

// TestMergeQueuePinMatchesMergedCommit is REQ-E10-S11-03: the recorded pin
// corresponds to what the merge queue merges, OR the merge-result-pinning
// capability is reported as a gap and NOTHING is pinned — never a fabricated
// digest.
//
// Hermetic shape: under a clean mergeable_state the merge ref
// refs/pull/{n}/merge IS the merge result the forge will produce, so the
// recorded pin equals the merge commit the PUT returns (the same commit SHA);
// under the merge-queue shape the digest is empty and merge-result-pinning is
// reported ABSENT with the queue reason, so the pin stays nil and arming is
// refused.
func TestMergeQueuePinMatchesMergedCommit(t *testing.T) {
	t.Run("clean_pr_records_the_commit_the_queue_merges", func(t *testing.T) {
		h := &armingHandler{prBody: armingPR, mergeRefSHA: "mCommit", armEnabled: true}
		c, _ := newServer(t, h.ServeHTTP)

		snap, err := c.Snapshot(baseRepo, "7")
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if snap.Heads.MergeResultDigest != "mCommit" {
			t.Fatalf("recorded pin = %q, want the merge-ref commit mCommit", snap.Heads.MergeResultDigest)
		}
		if state := snap.Capabilities.State(forge.CapabilityMergeResultPinning); state != forge.CapabilitySupported {
			t.Fatalf("merge-result-pinning = %q, want supported on a clean PR", state)
		}

		// The merge the forge performs produces THE SAME commit the pin
		// recorded — the pin corresponds to what was merged, not to a shape
		// the forge would have discarded.
		mergeID, err := c.MergeCAS(baseRepo, "7", forge.DesiredMerge{
			SourceSha:         "srcSHA",
			TargetSha:         "tgtTIP",
			MergeResultDigest: "mCommit",
		})
		if err != nil {
			t.Fatalf("MergeCAS: %v", err)
		}
		if mergeID != "merge/mCommit" {
			t.Fatalf("merged commit %q, want merge/mCommit — the pin and the merge result must be one commit", mergeID)
		}
		if got := h.mergePUTs; got != 1 {
			t.Fatalf("merge PUTs = %d, want 1", got)
		}
	})

	t.Run("queued_pr_pins_nothing_and_reports_the_gap", func(t *testing.T) {
		h := &armingHandler{prBody: queuedPR, armEnabled: true}
		c, _ := newServer(t, h.ServeHTTP)

		_, _, digest, err := c.CurrentHeads(baseRepo, "7")
		if err != nil {
			t.Fatalf("CurrentHeads (queued): %v", err)
		}
		if digest != "" {
			t.Fatalf("a queued PR must pin NOTHING (the queue's merge result is not the PR's), got digest %q", digest)
		}

		snap, err := c.Snapshot(baseRepo, "7")
		if err != nil {
			t.Fatalf("Snapshot (queued): %v", err)
		}
		if snap.Heads.MergeResultDigest != "" {
			t.Fatalf("MergeResultDigest = %q, want empty — a merge-queue PR must never be pinned to a fabricated result", snap.Heads.MergeResultDigest)
		}
		if state := snap.Capabilities.State(forge.CapabilityMergeResultPinning); state != forge.CapabilityAbsent {
			t.Fatalf("merge-result-pinning = %q, want absent with the queue reason", state)
		}
		if reason := snap.Capabilities.Reason(forge.CapabilityMergeResultPinning); !strings.Contains(reason, "merge queue") {
			t.Fatalf("the absent reason must name the merge-queue shape, got %q", reason)
		}

		// The pinned nothing cannot arm: the engine's precondition
		// completeness refuses an empty digest pin before any merge write
		// (ADR-0017 §1 — all three pins are required). The pins come from the
		// SAME snapshot the record would be built from, so the recorded pin
		// is exactly what the forge reports: nothing.
		_, err = forge.Reconcile(c, fixedTestClock{}, forge.DesiredReviewState{
			Project: baseRepo,
			MR:      "7",
			Approve: true,
			Merge: &forge.DesiredMerge{
				SourceSha:         snap.Heads.SourceSHA,
				TargetSha:         snap.Heads.TargetSHA,
				MergeResultDigest: snap.Heads.MergeResultDigest,
			},
		}, forge.Preconditions{
			ArmEligible:       true,
			SourceSha:         snap.Heads.SourceSHA,
			TargetSha:         snap.Heads.TargetSHA,
			MergeResultDigest: snap.Heads.MergeResultDigest,
		})
		if !errors.Is(err, forge.ErrIncompletePreconditions) {
			t.Fatalf("merging against an unpinned digest must fail closed with ErrIncompletePreconditions, got %v", err)
		}
		if got := h.mergePUTs; got != 0 {
			t.Fatalf("an unpinned digest must record zero merges, got %d PUTs", got)
		}
	})

	t.Run("absent_merge_ref_is_honestly_unavailable", func(t *testing.T) {
		h := &armingHandler{prBody: armingPR, mergeRefSHA: "", armEnabled: true}
		c, _ := newServer(t, h.ServeHTTP)

		_, _, digest, err := c.CurrentHeads(baseRepo, "7")
		if err != nil {
			t.Fatalf("CurrentHeads with an absent merge ref: %v", err)
		}
		if digest != "" {
			t.Fatalf("digest = %q, want empty", digest)
		}
	})
}
