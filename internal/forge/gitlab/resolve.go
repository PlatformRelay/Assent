package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/schemas"
)

// Resolve implements forge.Resolver per forge dossier §4 / P1-E3-S02: fetch
// approval rules/state, map eligible approvers, collect actual approvals, exclude
// MR author and bot client-side, pin SHAs on the evidence, and validate against
// the frozen ApprovalEvidence schema. Free/missing approval-rules → typed gap
// (never fabricated evidence). Stale evaluation pins → expired evidence (re-evaluate).
func (c *Client) Resolve(req forge.ResolveRequest) (forge.ResolveResult, error) {
	hasRules, err := c.hasApprovalRulesAPI(req.Project, req.MR)
	if err != nil {
		return forge.ResolveResult{}, err
	}
	if !hasRules {
		return forge.ResolveWithGap(forge.CapabilityGap{
			Reason:  forge.GapFreeTierRequireReview,
			Subject: req.Subject,
		}), nil
	}

	info, authorID, authorUsername, err := c.mrHeadsWithAuthor(req.Project, req.MR)
	if err != nil {
		return forge.ResolveResult{}, err
	}
	if req.MRAuthor == "" {
		req.MRAuthor = authorUsername
	}

	liveDigest := SyntheticDigest(info.SourceSHA, info.TargetSHA)
	pinsStale := req.SourceSha != info.SourceSHA ||
		req.TargetSha != info.TargetSHA ||
		req.MergeResultDigest != liveDigest

	state, err := c.approvalState(req.Project, req.MR)
	if err != nil {
		return forge.ResolveResult{}, err
	}
	rule, ok := pickApprovalRule(state.Rules)
	if !ok {
		return forge.ResolveWithGap(forge.CapabilityGap{
			Reason:  forge.GapApprovalRulesUnavailable,
			Subject: req.Subject,
		}), nil
	}

	botID, err := c.currentUserID()
	if err != nil {
		return forge.ResolveResult{}, err
	}

	eligible := filterEligible(rule.EligibleApprovers, authorID, authorUsername, req.MRAuthor, botID, c.botAuthor)
	approved := filterApproved(rule.ApprovedBy, eligible, authorID, authorUsername, req.MRAuthor, botID, c.botAuthor)

	if len(eligible) == 0 || len(approved) == 0 || len(approved) < rule.ApprovalsRequired {
		return forge.ResolveWithGap(forge.CapabilityGap{
			Reason:  forge.GapApprovalRulesUnavailable,
			Subject: req.Subject,
		}), nil
	}

	srcPin := req.SourceSha
	tgtPin := req.TargetSha
	digPin := req.MergeResultDigest
	expired := pinsStale
	if !pinsStale {
		srcPin = info.SourceSHA
		tgtPin = info.TargetSHA
		digPin = liveDigest
	}

	ev := aggregate.ApprovalEvidence{
		VerifyingCapability: "approval-rules-api",
		ApprovalsRequired:   rule.ApprovalsRequired,
		ApprovedBy:          approved,
		Eligibility:         eligibleIDs(eligible),
		Pins:                aggregate.ApprovalPins{SourceSha: srcPin},
		Expired:             expired,
	}

	doc, err := c.evidenceDocument(&ev, req, rule, srcPin, tgtPin, digPin)
	if err != nil {
		return forge.ResolveResult{}, err
	}
	if err := schemas.ValidateApprovalEvidence(doc); err != nil {
		return forge.ResolveResult{}, fmt.Errorf("gitlab: resolved evidence fails schema: %w", err)
	}

	result := forge.ResolveWithEvidence(ev)
	if err := result.WellFormed(); err != nil {
		return forge.ResolveResult{}, err
	}
	return result, nil
}

type gitlabUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type approvalStateRule struct {
	ID                 int           `json:"id"`
	Name               string        `json:"name"`
	RuleType           string        `json:"rule_type"`
	ApprovalsRequired  int           `json:"approvals_required"`
	Approved           bool          `json:"approved"`
	EligibleApprovers  []gitlabUser  `json:"eligible_approvers"`
	ApprovedBy         []approvedRow `json:"approved_by"`
}

type approvedRow struct {
	User gitlabUser `json:"user"`
}

type approvalStateResp struct {
	Rules []approvalStateRule `json:"rules"`
}

func (c *Client) mrHeadsWithAuthor(project, mr string) (MRInfo, int, string, error) {
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s", url.PathEscape(project), url.PathEscape(mr))
	status, raw, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return MRInfo{}, 0, "", err
	}
	if status != http.StatusOK {
		return MRInfo{}, 0, "", fmt.Errorf("gitlab: get MR %s!%s: unexpected status %d", project, mr, status)
	}
	var mrResp struct {
		IID          int    `json:"iid"`
		ProjectID    int    `json:"project_id"`
		SHA          string `json:"sha"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		Author       struct {
			ID       int    `json:"id"`
			Username string `json:"username"`
		} `json:"author"`
	}
	if err := json.Unmarshal(raw, &mrResp); err != nil {
		return MRInfo{}, 0, "", fmt.Errorf("gitlab: decode MR %s!%s: %w", project, mr, err)
	}
	targetSHA, err := c.branchTip(project, mrResp.TargetBranch)
	if err != nil {
		return MRInfo{}, 0, "", err
	}
	info := MRInfo{
		IID:          strconv.Itoa(mrResp.IID),
		ProjectID:    strconv.Itoa(mrResp.ProjectID),
		SourceBranch: mrResp.SourceBranch,
		TargetBranch: mrResp.TargetBranch,
		SourceSHA:    mrResp.SHA,
		TargetSHA:    targetSHA,
	}
	return info, mrResp.Author.ID, mrResp.Author.Username, nil
}

func (c *Client) approvalState(project, mr string) (approvalStateResp, error) {
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/approval_state",
		url.PathEscape(project), url.PathEscape(mr))
	status, raw, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return approvalStateResp{}, err
	}
	if status == http.StatusNotFound || status == http.StatusForbidden {
		return approvalStateResp{}, nil
	}
	if status != http.StatusOK {
		return approvalStateResp{}, fmt.Errorf("gitlab: get approval_state %s!%s: unexpected status %d", project, mr, status)
	}
	var resp approvalStateResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return approvalStateResp{}, fmt.Errorf("gitlab: decode approval_state %s!%s: %w", project, mr, err)
	}
	return resp, nil
}

func (c *Client) currentUserID() (int, error) {
	status, raw, err := c.do(http.MethodGet, "/api/v4/user", nil, "")
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("gitlab: get user: unexpected status %d", status)
	}
	var u gitlabUser
	if err := json.Unmarshal(raw, &u); err != nil {
		return 0, fmt.Errorf("gitlab: decode user: %w", err)
	}
	return u.ID, nil
}

func pickApprovalRule(rules []approvalStateRule) (approvalStateRule, bool) {
	for _, r := range rules {
		if r.ApprovalsRequired > 0 {
			return r, true
		}
	}
	return approvalStateRule{}, false
}

func filterEligible(users []gitlabUser, authorID int, authorUsername, mrAuthor string, botID int, botUsername string) []gitlabUser {
	var out []gitlabUser
	for _, u := range users {
		if isExcludedApprover(u, authorID, authorUsername, mrAuthor, botID, botUsername) {
			continue
		}
		out = append(out, u)
	}
	return out
}

func filterApproved(rows []approvedRow, eligible []gitlabUser, authorID int, authorUsername, mrAuthor string, botID int, botUsername string) []aggregate.Approver {
	eligibleSet := make(map[string]struct{}, len(eligible))
	for _, u := range eligible {
		eligibleSet[strconv.Itoa(u.ID)] = struct{}{}
	}
	var out []aggregate.Approver
	seen := make(map[string]struct{})
	for _, row := range rows {
		u := row.User
		if isExcludedApprover(u, authorID, authorUsername, mrAuthor, botID, botUsername) {
			continue
		}
		id := strconv.Itoa(u.ID)
		if _, ok := eligibleSet[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, aggregate.Approver{ID: id, Username: u.Username, IsAuthor: false})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func isExcludedApprover(u gitlabUser, authorID int, authorUsername, mrAuthor string, botID int, botUsername string) bool {
	id := strconv.Itoa(u.ID)
	if authorID != 0 && u.ID == authorID {
		return true
	}
	if mrAuthor != "" && (u.Username == mrAuthor || id == mrAuthor) {
		return true
	}
	if authorUsername != "" && u.Username == authorUsername {
		return true
	}
	if botID != 0 && u.ID == botID {
		return true
	}
	if botUsername != "" && u.Username == botUsername {
		return true
	}
	return false
}

func eligibleIDs(users []gitlabUser) []string {
	ids := make([]string, len(users))
	for i, u := range users {
		ids[i] = strconv.Itoa(u.ID)
	}
	sort.Strings(ids)
	return ids
}

func (c *Client) observedNow() time.Time {
	if c.observedAt != nil {
		return c.observedAt()
	}
	return time.Now().UTC()
}

// evidenceDocument serializes aggregate evidence to the frozen schema shape (tests + validation).
func (c *Client) evidenceDocument(ev *aggregate.ApprovalEvidence, _ forge.ResolveRequest, rule approvalStateRule, srcPin, tgtPin, digPin string) ([]byte, error) {
	if ev == nil {
		return nil, fmt.Errorf("gitlab: nil evidence")
	}
	principal := ev.ApprovedBy[0]
	approvedBy := make([]map[string]any, len(ev.ApprovedBy))
	for i, a := range ev.ApprovedBy {
		approvedBy[i] = map[string]any{
			"id":       a.ID,
			"username": a.Username,
			"isAuthor": false,
		}
	}
	doc := map[string]any{
		"apiVersion":          "assent.dev/v1alpha1",
		"kind":                "ApprovalEvidence",
		"verifyingCapability": ev.VerifyingCapability,
		"approvalsRequired":   ev.ApprovalsRequired,
		"principal": map[string]any{
			"id":       principal.ID,
			"username": principal.Username,
			"isAuthor": false,
		},
		"source": map[string]any{
			"rule":     rule.Name,
			"ruleType": rule.RuleType,
		},
		"eligibility": map[string]any{
			"eligibleApproverIds": ev.Eligibility,
		},
		"approvedBy": approvedBy,
		"pins": map[string]any{
			"toolVersion":       resolveToolVersion,
			"toolDigest":        resolveToolDigest,
			"policySha":         resolvePolicySha,
			"sourceSha":         srcPin,
			"targetSha":         tgtPin,
			"mergeResultDigest": digPin,
			"factsResolvedAt":   map[string]any{},
		},
		"observedAt": c.observedNow().Format(time.RFC3339),
	}
	return json.Marshal(doc)
}

const (
	resolveToolVersion = "0.0.0-dev"
	resolveToolDigest  = "sha256:resolve-unpinned"
	resolvePolicySha   = "unpinned"
)

var _ forge.Resolver = (*Client)(nil)
