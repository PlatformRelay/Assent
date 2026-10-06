package github

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// capability_test.go — the honest-grading cases for the capability report's
// UNVERIFIED rows (S00 Q2's rule: a cell the dossier tags `unverified` resolves
// to UNKNOWN until a live response confirms it — REQ-E10-S09-02). A probe that
// runs may not license a grading S00 has not: unverified resolves to unknown,
// never to supported, and the reason must cite the open item instead of
// inventing a probe the package does not issue.

// honestSnapshot drives the adapter's own probe chain (PR GET, repo settings,
// merge ref, files, comments) and returns the capability report.
func honestSnapshot(t *testing.T, repoStatus int, repoBody string) forge.CapabilityReport {
	t.Helper()
	c, _ := newServer(t, snapshotHandlerWithRepo(t, repoStatus, repoBody))
	snap, err := c.Snapshot(baseRepo, "7")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	return snap.Capabilities
}

// snapshotHandlerWithRepo is snapshotHandler with the repo-settings probe
// answerable per test (status + body); everything else serves the default
// readable chain (clean PR, readable merge ref, one short files page).
func snapshotHandlerWithRepo(t *testing.T, repoStatus int, repoBody string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo":
			w.WriteHeader(repoStatus)
			_, _ = w.Write([]byte(repoBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
			_, _ = io.WriteString(w, sameRepoPR)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/git/ref/refs/pull/7/merge":
			_, _ = io.WriteString(w, `{"ref":"refs/pull/7/merge","object":{"sha":"mrgSHA","type":"commit"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/files":
			_, _ = io.WriteString(w, filePageOf(`{"filename":"pkg/a.go"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/comments":
			_, _ = io.WriteString(w, "[]")
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/branches/") && strings.HasSuffix(r.URL.Path, "/protection"):
			_, _ = io.WriteString(w, `{"required_pull_request_reviews":{"required_approving_review_count":1},"required_conversation_resolution":{"enabled":true}}`)
		default:
			unexpectedEndpoint(w, r)
		}
	}
}

// TestUnverifiedReportedUnknown is REQ-E10-S09-02's named Verify: capabilities
// whose real behaviour is UNVERIFIED against a live API are reported `unknown`
// with the open item cited — never optimistically `supported`.
func TestUnverifiedReportedUnknown(t *testing.T) {
	t.Run("deferred-merge-arming stays unknown while allow_auto_merge is unverified", func(t *testing.T) {
		// The probe RUNS and the repo setting is true — but the field name is
		// unverified against a live response (S00 Q2 row 6), so the grading
		// stays unknown with the open item cited. The arming consult refuses
		// on unknown, so nothing regresses; the adapter just stops claiming a
		// capability S00 has not licensed.
		caps := honestSnapshot(t, http.StatusOK, `{"allow_auto_merge":true}`)
		state := caps.State(forge.CapabilityDeferredMergeArming)
		if state != forge.CapabilityUnknown {
			t.Fatalf("deferred-merge-arming = %q, want unknown — allow_auto_merge is an UNVERIFIED field (S00 Q2 row 6), so a hermetic 200 licenses nothing", state)
		}
		reason := strings.ToLower(caps.Reason(forge.CapabilityDeferredMergeArming))
		for _, want := range []string{"allow_auto_merge", "unverified", "req-e10-s09-02"} {
			if !strings.Contains(reason, want) {
				t.Errorf("deferred-merge-arming reason must cite the unverified field and the open item %q", want)
			}
		}
		if strings.Contains(reason, "SUPPORTED") {
			t.Errorf("the reason must not claim enablePullRequestAutoMerge availability: %q", reason)
		}
	})

	t.Run("deferred-merge-arming stays unknown when the field reads false", func(t *testing.T) {
		// The same unverified cell resolves to unknown in BOTH directions: an
		// unconfirmed field name cannot prove absence any more than support.
		caps := honestSnapshot(t, http.StatusOK, `{"allow_auto_merge":false}`)
		if state := caps.State(forge.CapabilityDeferredMergeArming); state != forge.CapabilityUnknown {
			t.Fatalf("deferred-merge-arming = %q on an unverified false, want unknown", state)
		}
		if reason := strings.ToLower(caps.Reason(forge.CapabilityDeferredMergeArming)); !strings.Contains(reason, "unverified") {
			t.Errorf("reason must carry the unverified citation, got %q", reason)
		}
	})

	t.Run("deferred-merge-arming unknown with no repo-settings probe", func(t *testing.T) {
		// The repo-settings probe is retired (its observed value licensed
		// neither support nor absence): the grading is unknown unconditionally
		// — unprobed is not proof, and an unreadable repo can no longer abort
		// the Snapshot for a capability that grades unknown either way.
		caps := honestSnapshot(t, http.StatusNotFound, "no repo")
		if state := caps.State(forge.CapabilityDeferredMergeArming); state != forge.CapabilityUnknown {
			t.Fatalf("deferred-merge-arming = %q with the repo probe unreadable, want unknown (unprobed is not proof)", state)
		}
	})

	t.Run("threads-block-merge reason names no phantom probe", func(t *testing.T) {
		caps := honestSnapshot(t, http.StatusOK, `{"allow_auto_merge":true}`)
		if state := caps.State(forge.CapabilityThreadsBlockMerge); state != forge.CapabilityUnknown {
			t.Fatalf("threads-block-merge = %q, want unknown (S00 Q2 row 2)", state)
		}
		reason := caps.Reason(forge.CapabilityThreadsBlockMerge)
		if strings.Contains(reason, "probe attempted") {
			t.Fatalf("the reason must not claim a probe the package does not issue: %q", reason)
		}
		for _, want := range []string{"no probe is implemented", "unverified", "S00 Q2 row 2"} {
			if !strings.Contains(reason, want) {
				t.Errorf("threads-block-merge reason must say %q, got %q", want, reason)
			}
		}
	})
}

// TestCapabilityReportKeepsLicensedConstants is the polarity control for the
// honest-unknown rows: the hermetic-licensed constants (S00 Q2 rows 1/5) and
// the adapter's own merge-ref probe stay SUPPORTED, so the unknown rows above
// are not satisfied by an adapter that grades everything unknown.
func TestCapabilityReportKeepsProbedRowsGraded(t *testing.T) {
	caps := honestSnapshot(t, http.StatusOK, `{"allow_auto_merge":true}`)
	for _, tc := range []struct {
		cap  forge.Capability
		want forge.CapabilityState
	}{
		{forge.CapabilityResolvableThreads, forge.CapabilitySupported},
		{forge.CapabilitySHAGuardedMerge, forge.CapabilitySupported},
		{forge.CapabilityMergeResultPinning, forge.CapabilitySupported},
	} {
		if got := caps.State(tc.cap); got != tc.want {
			t.Errorf("%s = %q, want %q — the unknown rows above must not be satisfied by a blanket unknown report", tc.cap, got, tc.want)
		}
	}
}

// TestHonestReportRefusesArmingOnUnknownArmingField composes the grading with
// its consumer: the adapter's arming consult refuses on the UNKNOWN
// deferred-merge-arming entry (the arming path's first consult member), so a
// repo that allows auto-merge is still never armed on an unverified field.
func TestArmingConsultRefusesOnUnknownDeferredMergeArming(t *testing.T) {
	h := &armingHandler{prBody: armingPR, mergeRefSHA: "mrgSHA", armEnabled: true}
	c, _ := newServer(t, h.ServeHTTP)

	snap, err := c.Snapshot(baseRepo, "7")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	err = c.EnablePullRequestAutoMerge(baseRepo, "7", snap.Capabilities, MergeMethodMerge)
	if !errors.Is(err, forge.ErrArmingRefused) {
		t.Fatalf("the honest v1 report must refuse arming, got %v", err)
	}
	if !strings.Contains(err.Error(), "deferred-merge-arming") {
		t.Errorf("the refusal must name the unproven arming-path capability, got %v", err)
	}
	if got := h.armMutations; got != 0 {
		t.Fatalf("a refused arming must never reach the mutation, got %d mutation(s)", got)
	}
}
