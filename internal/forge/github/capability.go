package github

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/PlatformRelay/assent/internal/forge"
)

// capability.go — the GitHub capability report (E10-S09 over S04's model).
//
// Every flag of the closed enum carries a state and a reason; a capability the
// adapter does not actually probe is UNKNOWN, never supported (REQ-E10-S04-03),
// and a probe TRANSPORT failure is a hard error, never a silent unknown
// (REQ-E10-S04-05 — it propagates as Snapshot's error).
//
// The predicates follow S00 Q2's GitHub column. Where the dossier's field name
// is `unverified`, the flag stays UNKNOWN until a live response confirms it —
// an unverified probe may never arm anything (the SEC-04 rule). That is why
// several rows below are UNKNOWN: it is the honest grading of hermetic-only
// evidence, and the S12/S15 product-limitation note carries it. A cell that
// turns out to be right is promoted at S09/S18; a cell that turns out to be
// wrong never armed anything.

// probeCapabilities reads the forge settings the predicates need and returns
// the report. Probe transport failures propagate as errors (never unknown).
func (c *Client) probeCapabilities(project, mr string) (forge.CapabilityReport, error) {
	repo, err := repoParts(project)
	if err != nil {
		return forge.CapabilityReport{}, err
	}
	caps := map[forge.Capability]forge.CapabilityEntry{
		// C constants licensed by named cases (S00 Q2 rows 1/5): the GraphQL
		// resolve round-trip licenses resolvable-threads; the GitHub-factory
		// sha-guard cases license sha-guarded-merge (PUT /pulls/{n}/merge with
		// the sha pin, 409 on mismatch — dossier C10).
		forge.CapabilityResolvableThreads: forge.SupportedCapabilityEntry(
			"constant supported — licensed by the GraphQL thread-resolve round-trip (resolveReviewThread / isResolved, dossier C1/C2)"),
		forge.CapabilitySHAGuardedMerge: forge.SupportedCapabilityEntry(
			"constant supported — PUT /pulls/{n}/merge honours the sha pin (409 on mismatch, dossier C10); licensed by the sha-guard-* conformance cases"),

		// NOT probed: the branch-protection read (dossier C3) has no probe in
		// this package, and required_conversation_resolution.enabled's shape is
		// UNVERIFIED against a live API (S00 Q2 row 2). No phantom probe may be
		// claimed — the grading stays unknown until a live response confirms
		// the field (S09/S18), and a wrong cell never armed anything.
		forge.CapabilityThreadsBlockMerge: forge.UnknownCapabilityEntry(
			"no probe is implemented for the branch-protection read (dossier C3); required_conversation_resolution's shape is unverified against a live API — unknown until confirmed (S00 Q2 row 2)"),

		// Unknown rows (S00 Q2 rows 3/4/7/9/10/11) — each reason names the open
		// verification item, exactly the honest grading the model demands.
		forge.CapabilityBlockingReview: forge.UnknownCapabilityEntry(
			"required_pull_request_reviews.required_approving_review_count is enumerated (dossier §2 row 3) but the branch-protection read's shape is UNVERIFIED against a live API — unknown until confirmed"),
		forge.CapabilityReviewDismissalRestrictions: forge.UnknownCapabilityEntry(
			"dismissal_restrictions probe (dossier C8') is UNVERIFIED against a live response — unknown until S09 confirms (S00 Q2 row 4)"),
		forge.CapabilityArmingRevokedOnPush: forge.UnknownCapabilityEntry(
			"the substitute arming-revoke signal (stale-approval dismissal AND at least one required status check) is UNVERIFIED — a write-access push does NOT auto-disarm GitHub auto-merge (dossier §3 delta 2); unknown until verified (S00 Q2 row 7)"),
		forge.CapabilityApprovalResetOnPush: forge.UnknownCapabilityEntry(
			"stale-approval dismissal and require-most-recent-reviewable-push field names are UNVERIFIED (dossier C19/C8) — unknown until a live response confirms them (S00 Q2 row 10)"),

		// The two standing unknowns (ADR-0021 Consequences; S00 rows 9 and 11):
		forge.CapabilityProtectedPipelineSource: forge.UnknownCapabilityEntry(
			"no operationally decidable predicate is probed — ADR-0015 §4's GitHub condition spans the workflow trust model (pull_request runs the base-ref workflow for forks; same-repo branches need branch protection on workflow paths) and its probeability is UNVERIFIED; OQ-33 records the candidate routes and no heuristic may stand in (the SEC-04 shape)"),
		forge.CapabilityEligibleApprovalEvidence: forge.UnknownCapabilityEntry(
			"no API returns the computed per-PR eligible code owners (dossier §2 step (b), graded partial) — the adapter-computed CODEOWNERS set cannot prove set-equality with the forge (OQ-34); unknown per S00 Q2 row 9"),
	}

	// Probe 2 (S00 Q2 row 8 / dossier C16): the merge-result digest axis. The
	// grading comes from the adapter's OWN merge-ref probe (mergeResultDigest
	// ran earlier in the same Snapshot read), so the capability state and the
	// digest the CAS pins come from one read. Never probed this run → unknown,
	// never absent-by-default.
	switch c.mergeRefStateOf() {
	case mergeRefReadable:
		caps[forge.CapabilityMergeResultPinning] = forge.SupportedCapabilityEntry(
			"probe: refs/pull/{n}/merge is readable — a real merge-result commit backs the digest axis (dossier C16)")
	case mergeRefUnmergeable:
		// REQ-E10-S11-03: the PR was not mergeable when the probe ran — the
		// merge-queue-or-blocked shape. The digest axis is honestly
		// unavailable, and completeForMerge refuses to arm on it.
		caps[forge.CapabilityMergeResultPinning] = forge.AbsentCapabilityEntry(fmt.Sprintf(
			"merge result unavailable: mergeable_state %q — the PR is not mergeable now (merge queue in use or blocked, dossier C14/C16), so the digest axis is unavailable and the merge is not armed",
			c.lastMergeableState()))
	case mergeRefUnreadable:
		caps[forge.CapabilityMergeResultPinning] = forge.AbsentCapabilityEntry(
			"merge ref unreadable: not mergeable or merge queue in use (dossier C16) — the digest axis is honestly unavailable and completeForMerge refuses to arm")
	default:
		caps[forge.CapabilityMergeResultPinning] = forge.UnknownCapabilityEntry(
			"merge ref not probed on this run — unprobed is not proof (dossier C16)")
	}

	// Probe 1 (S00 Q2 row 6 / dossier C11): allow_auto_merge on the repo.
	// The probe runs, but the field name is UNVERIFIED against a live response
	// (S00 Q2 row 6) and the enablePullRequestAutoMerge preconditions are
	// dossier "open verification items" (REQ-E10-S09-02) — so the grading
	// stays UNKNOWN with the open item cited, in BOTH directions of the
	// observed value: an unconfirmed field name licenses neither support nor
	// absence, and the arming consult refuses on unknown either way.
	status, _, raw, err := c.do(http.MethodGet, "/repos/"+repo, nil, "")
	if err != nil {
		return forge.CapabilityReport{}, err
	}
	if status == http.StatusOK {
		var repoMeta struct {
			AllowAutoMerge bool `json:"allow_auto_merge"`
		}
		if err := json.Unmarshal(raw, &repoMeta); err != nil {
			return forge.CapabilityReport{}, fmt.Errorf("github: decode repo %s: %w", repo, err)
		}
		if repoMeta.AllowAutoMerge {
			caps[forge.CapabilityDeferredMergeArming] = forge.UnknownCapabilityEntry(
				"probe: allow_auto_merge is true (dossier C11) — but the field name is UNVERIFIED against a live response (S00 Q2 row 6) and the enablePullRequestAutoMerge preconditions are dossier open verification items (REQ-E10-S09-02): unknown until a live response confirms it")
		} else {
			caps[forge.CapabilityDeferredMergeArming] = forge.UnknownCapabilityEntry(
				"probe: allow_auto_merge is false (dossier C11) — the field name is UNVERIFIED against a live response (S00 Q2 row 6), so the false reading proves neither support nor absence (REQ-E10-S09-02): unknown until confirmed")
		}
	} else if status != http.StatusNotFound {
		// A non-404 error on the repo probe (an unauthorized metadata read)
		// means every setting below it is unprovable — a hard error, never a
		// silent unknown (REQ-E10-S04-05).
		return forge.CapabilityReport{}, fmt.Errorf("github: get repo %s: unexpected status %d", repo, status)
	}
	// A 404 repo read leaves deferred-merge-arming UNKNOWN (not probed).
	if caps[forge.CapabilityDeferredMergeArming].State == "" {
		caps[forge.CapabilityDeferredMergeArming] = forge.UnknownCapabilityEntry(
			fmt.Sprintf("repo settings unreadable (probe answered %d) — unprobed is not proof", status))
	}

	return forge.NewCapabilityReport(caps)
}
