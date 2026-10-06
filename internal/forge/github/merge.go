package github

import (
	"encoding/json"
	"fmt"

	"github.com/PlatformRelay/assent/internal/forge"
)

// merge.go — deferred merge arming (E10-S11, REQ-E10-S11-02/03/04).
//
// GitHub's deferred arming is the GraphQL mutation enablePullRequestAutoMerge
// (dossier C11): the pull request node id comes from the PR GET (ghPR.NodeID),
// and the forge merges when its own gates (conversation resolution, checks,
// reviews) clear. The arming verb is a WRITE — one attempt, never retried —
// and it consults the arming-path capabilities BEFORE anything runs:
//
//	deferred-merge-arming          the repo allows auto-merge at all (dossier C11)
//	arming-revoked-on-push         delta 2 (dossier §3): a write-access push does
//	                               NOT auto-disarm GitHub auto-merge, so arming is
//	                               GitLab-equivalent only with a proven revocation
//	                               signal (stale-approval dismissal + required
//	                               checks); unproven ⇒ refuse
//	review-dismissal-restrictions  delta 1 (dossier §3): the block device's
//	                               protection must be restricted so the MR author
//	                               cannot dismiss the bot's blocking review
//	merge-result-pinning           delta 3 (dossier §3): deferred arming without
//	                               a merge queue on a busy target is a capability
//	                               gap — fail closed (dossier C14)
//
// The engine's own arming consult (forge.PreconditionFromReport) is the
// ADR-0015 §4/§8 forge-neutral set and is deliberately NOT restated here: the
// deltas are GitHub's arming-path capabilities, consulted at the adapter's
// arming verb, and each refusal names the capability and its state so the
// operator can tell "the forge lacks it" from "nobody proved it"
// (REQ-E10-S04-02). In the v1 posture every one of these is unknown ⇒ arming
// is refused — the product limitation S12 pins, never silently armed.

// MergeMethod is GitHub's merge method for the arming mutation. Values mirror
// the GraphQL enum PullRequestMergeMethod.
type MergeMethod string

const (
	MergeMethodMerge  MergeMethod = "MERGE"
	MergeMethodSquash MergeMethod = "SQUASH"
	MergeMethodRebase MergeMethod = "REBASE"
)

// parseMergeMethod decodes a merge method. An unknown value is an error, not
// a silent default: a default would arm a merge method the caller never asked
// for.
func parseMergeMethod(m MergeMethod) (MergeMethod, error) {
	switch m {
	case MergeMethodMerge, MergeMethodSquash, MergeMethodRebase:
		return m, nil
	default:
		return "", fmt.Errorf("github: merge method %q is not one of MERGE|SQUASH|REBASE", m)
	}
}

// armingConsultSet is the arming consultation set for the GitHub deferred
// arming verb: every member must be PROVEN supported before arming (unknown
// and absent refuse identically, with distinguishable reasons — REQ-E10-S04-02).
var armingConsultSet = []forge.Capability{
	forge.CapabilityDeferredMergeArming,
	forge.CapabilityArmingRevokedOnPush,
	forge.CapabilityReviewDismissalRestrictions,
	forge.CapabilityMergeResultPinning,
}

// armingRefusal returns the arming consult's refusal: the first arming-path
// capability whose state is not supported, named with its state and the
// adapter's own reason — the contributor-legible detail REQ-E10-S12-01 renders.
// Nil when every member is supported.
func armingRefusal(report forge.CapabilityReport) error {
	for _, c := range armingConsultSet {
		if report.State(c) == forge.CapabilitySupported {
			continue
		}
		return fmt.Errorf(
			"github: %w: capability %q is %s, not proven supported — refusing to arm deferred auto-merge. %s",
			forge.ErrArmingRefused, c, report.State(c), report.Reason(c))
	}
	return nil
}

// gqlEnableAutoMerge arms deferred auto-merge on a pull request (dossier C11).
// The pull request is addressed by its GraphQL node id (ghPR.NodeID); the
// response must report the arming enabled — a forge that answers the mutation
// without confirming it is an error, never success (the resolve-verb's
// contract).
const gqlEnableAutoMerge = `mutation($id: ID!, $method: PullRequestMergeMethod!) {
  enablePullRequestAutoMerge(input: {pullRequestId: $id, mergeMethod: $method}) {
    pullRequest {
      autoMergeRequest {
        enabled
        mergeMethod
      }
    }
  }
}`

// EnablePullRequestAutoMerge arms deferred auto-merge on the pull request,
// fail-closed on the arming-path capabilities (REQ-E10-S11-02): every member
// of armingConsultSet must be supported, or the call refuses BEFORE any
// network write, wrapping forge.ErrArmingRefused and naming the capability and
// its state. The refusal is the product posture for v1 — GitHub's auto-merge
// is not revoked by a write-access push (dossier §3 delta 2), so arming with
// an unproven revocation signal is refused even when the repo setting itself
// would allow it.
//
// report is the forge-probed capability report the caller consulted (the
// run path's Snapshot report). The adapter never fabricates a report: an
// unknown capability refuses, and the reason strings ride the error.
//
// The mutation itself carries no SHA pin: the armed merge is re-validated by
// the forge's own gates at merge time, which is precisely why arming demands
// the merge-result delta (a queue) and the revocation signal to be proven
// first (dossier §3 deltas 2/3) — without them, arming is not equivalent to
// the GitLab contract and is refused.
func (c *Client) EnablePullRequestAutoMerge(project, mr string, report forge.CapabilityReport, method MergeMethod) error {
	if err := armingRefusal(report); err != nil {
		return err
	}
	if _, err := parseMergeMethod(method); err != nil {
		return err
	}
	repo, err := repoParts(project)
	if err != nil {
		return err
	}
	pr, err := c.freshPR(repo, mr)
	if err != nil {
		return err
	}
	if pr.NodeID == "" {
		// Fail closed: without the node id the mutation cannot address the PR,
		// and an adapter that guesses would arm a different pull request.
		return fmt.Errorf("github: PR %s#%s reports no node id — refusing to arm a pull request the forge did not address", repo, mr)
	}
	data, err := c.gqlDo(c.ctx, gqlEnableAutoMerge, map[string]any{"id": pr.NodeID, "method": string(method)})
	if err != nil {
		return fmt.Errorf("github: enablePullRequestAutoMerge %s#%s: %w", project, mr, err)
	}
	var payload struct {
		EnablePullRequestAutoMerge struct {
			PullRequest struct {
				AutoMergeRequest struct {
					Enabled     bool   `json:"enabled"`
					MergeMethod string `json:"mergeMethod"`
				} `json:"autoMergeRequest"`
			} `json:"pullRequest"`
		} `json:"enablePullRequestAutoMerge"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("github: decode enablePullRequestAutoMerge %s#%s: %w", project, mr, err)
	}
	if !payload.EnablePullRequestAutoMerge.PullRequest.AutoMergeRequest.Enabled {
		return fmt.Errorf("github: arming %s#%s did not report enabled:true — fail-closed", project, mr)
	}
	return nil
}
