package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge"
)

// resolve.go — forge.Resolver for GitHub (E10-S06 over S00 Q2 row 9).
//
// The honest v1 posture: GitHub exposes no API that returns the computed
// per-PR eligible approver set (the required code owners), so the
// eligible-approver set is UNPROVABLE (OQ-34) and require-review evidence can
// never be minted. Resolve therefore ALWAYS returns the
// GapEligibilityUnprovable gap — never evidence, never silent APPROVE.
//
// The review-chain logic is nonetheless REAL and unit-tested: the adapter
// fetches the PR's review chain (paginated, capped, fail-closed), excludes the
// PR author and bots, and tracks each reviewer's LATEST non-dismissed state —
// exactly the computation OQ-34's promotion route would consume if the forge
// ever exposes an eligible set to prove equality against. The chain fetch
// failing is a transport/permission error that propagates — the gap is an
// honest declaration, never a blanket that hides a broken forge.

// ghReview is the subset of a PR review the adapter reads. State is one of
// GitHub's review states: APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED,
// PENDING. Only an undismissed APPROVED as the reviewer's LATEST state counts.
type ghReview struct {
	ID   int64 `json:"id"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	State string `json:"state"`
}

// Resolve implements forge.Resolver. It fetches and evaluates the review chain
// (fail-closed transport), then returns the capability gap — ALWAYS, in v1
// (OQ-34: the eligible-approver set is unprovable, so no evidence may be
// minted; forge.GapEligibilityUnprovable documents the reason).
func (c *Client) Resolve(req forge.ResolveRequest) (forge.ResolveResult, error) {
	// The PR author must be excluded from the approvals even when the caller
	// did not carry it (REQ-E10-S10-02): the author comes from the PR read
	// itself — a fresh read, the same chain gitlab.Resolve's mrHeadsWithAuthor
	// uses. A missing author from a broken forge is a decode error there,
	// never a silently-unexcluded approval.
	author := req.MRAuthor
	repo, err := repoParts(req.Project)
	if err != nil {
		return forge.ResolveResult{}, err
	}
	if author == "" {
		pr, err := c.freshPR(repo, req.MR)
		if err != nil {
			return forge.ResolveResult{}, err
		}
		author = pr.User.Login
	}
	// Fetch and evaluate the review chain. The VALUE is deliberately not minted
	// into evidence (OQ-34); the fetch itself is the fail-closed transport
	// check — an unreadable chain is a hard error, never a silent gap.
	if _, err := c.reviewChainApprovals(req.Project, req.MR, author); err != nil {
		return forge.ResolveResult{}, err
	}
	return forge.ResolveWithGap(forge.CapabilityGap{
		Reason:  forge.GapEligibilityUnprovable,
		Subject: req.Subject,
	}), nil
}

// listReviews reads the PR's review chain via the PAGINATED GET
// /repos/{repo}/pulls/{mr}/reviews (oldest-first, the order GitHub returns),
// CAPPED at maxListPages like every listing. A 404/403 wraps
// forge.ErrUnauthorized — a review chain the caller cannot read is a
// permission failure, never "no reviews" (S00 Q4).
func (c *Client) listReviews(project, mr string) ([]ghReview, error) {
	repo, err := repoParts(project)
	if err != nil {
		return nil, err
	}
	var reviews []ghReview
	for page := 1; ; page++ {
		if page > maxListPages {
			return nil, fmt.Errorf(
				"github: list PR reviews %s#%s: pagination cap of %d pages reached without a short page — refusing to resolve against a partial review chain",
				project, mr, maxListPages)
		}
		status, _, raw, err := c.do(http.MethodGet,
			fmt.Sprintf("/repos/%s/pulls/%s/reviews?per_page=%d&page=%d", repo, mr, listPerPage, page), nil, "")
		if err != nil {
			return nil, err
		}
		switch status {
		case http.StatusOK:
			var rows []ghReview
			if err := json.Unmarshal(raw, &rows); err != nil {
				return nil, fmt.Errorf("github: decode reviews %s#%s: %w", repo, mr, err)
			}
			if len(rows) == 0 {
				return reviews, nil
			}
			reviews = append(reviews, rows...)
			if len(rows) < listPerPage {
				return reviews, nil
			}
		case http.StatusNotFound, http.StatusForbidden:
			return nil, fmt.Errorf("github: %w: list PR reviews %s#%s (status %d)", forge.ErrUnauthorized, repo, mr, status)
		default:
			return nil, fmt.Errorf("github: list reviews %s#%s: unexpected status %d", repo, mr, status)
		}
	}
}

// latestNonDismissedApprovals reduces a PR's review chain to the set of
// logins whose LATEST review is an undismissed APPROVED.
//
// The chain is read oldest→newest (GitHub returns reviews chronologically); a
// later review by the same login REPLACES the earlier state — so a newer
// CHANGES_REQUESTED or a DISMISSED event always supersedes an older approval
// (a dismissed approval is never counted, a re-requested-changes reviewer is
// never counted). Exclusions are the AUTHOR-IDENTITY axes (ADR-0019 /
// REQ-E10-S10-02): the PR author, the configured bot identity, and GitHub's
// app-bot logins ("*[bot]") are never counted as approvals. The result is
// sorted for determinism.
func latestNonDismissedApprovals(reviews []ghReview, mrAuthor, botName string) []string {
	latest := make(map[string]string, len(reviews))
	for _, r := range reviews {
		login := r.User.Login
		if login == "" {
			continue
		}
		latest[login] = r.State
	}
	out := make([]string, 0, len(latest))
	for login, state := range latest {
		if state != "APPROVED" {
			continue
		}
		if mrAuthor != "" && login == mrAuthor {
			continue
		}
		if botName != "" && login == botName {
			continue
		}
		if isBotLogin(login) {
			continue
		}
		out = append(out, login)
	}
	sort.Strings(out)
	return out
}

// isBotLogin reports whether a GitHub login is an app-bot identity (the
// "<app>[bot]" shape GitHub assigns to GitHub App invocations).
func isBotLogin(login string) bool {
	return strings.HasSuffix(login, "[bot]")
}

// reviewChainApprovals fetches the review chain and reduces it to the latest
// non-dismissed approval set (the exclusion axes: PR author, configured bot
// identity, app bots).
func (c *Client) reviewChainApprovals(project, mr, mrAuthor string) ([]string, error) {
	reviews, err := c.listReviews(project, mr)
	if err != nil {
		return nil, err
	}
	return latestNonDismissedApprovals(reviews, mrAuthor, c.botName), nil
}

var _ forge.Resolver = (*Client)(nil)
