package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge"
)

// Snapshot implements forge.Snapshotter. It reads MR heads (via GetMR semantics),
// changed-file paths from the MR diffs API (D-076), project merge settings and
// approval-rules presence for capability flags, and bot threads for reconciliation.
// Optional endpoints that return 404/403 fail safe to honest tier gaps — never
// invented Premium capabilities.
func (c *Client) Snapshot(project, mr string) (forge.Snapshot, error) {
	info, author, err := c.mrWithAuthor(project, mr)
	if err != nil {
		return forge.Snapshot{}, err
	}

	files, err := c.mrChangedFiles(project, mr)
	if err != nil {
		return forge.Snapshot{}, err
	}
	sort.Strings(files)

	caps, err := c.probeCapabilities(project, mr)
	if err != nil {
		return forge.Snapshot{}, err
	}

	threads, err := c.ListBotThreads(project, mr)
	if err != nil {
		return forge.Snapshot{}, err
	}

	return forge.Snapshot{
		Heads: forge.MRHeads{
			SourceSHA:         info.SourceSHA,
			TargetSHA:         info.TargetSHA,
			SourceBranch:      info.SourceBranch,
			TargetBranch:      info.TargetBranch,
			MergeResultDigest: SyntheticDigest(info.SourceSHA, info.TargetSHA),
			Author:            author,
			ForkMR:            info.ForkMR,
		},
		ChangedFiles: files,
		Capabilities: caps,
		BotThreads:   threads,
	}, nil
}

func (c *Client) mrWithAuthor(project, mr string) (MRInfo, string, error) {
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s", url.PathEscape(project), url.PathEscape(mr))
	status, raw, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return MRInfo{}, "", err
	}
	if status != http.StatusOK {
		return MRInfo{}, "", fmt.Errorf("gitlab: get MR %s!%s: unexpected status %d", project, mr, status)
	}
	var mrResp struct {
		IID             int    `json:"iid"`
		ProjectID       int    `json:"project_id"`
		SourceProjectID int    `json:"source_project_id"`
		SHA             string `json:"sha"`
		SourceBranch    string `json:"source_branch"`
		TargetBranch    string `json:"target_branch"`
		Author          struct {
			Username string `json:"username"`
		} `json:"author"`
	}
	if err := json.Unmarshal(raw, &mrResp); err != nil {
		return MRInfo{}, "", fmt.Errorf("gitlab: decode MR %s!%s: %w", project, mr, err)
	}

	targetSHA, err := c.branchTip(project, mrResp.TargetBranch)
	if err != nil {
		return MRInfo{}, "", err
	}

	return MRInfo{
		IID:          fmt.Sprintf("%d", mrResp.IID),
		ProjectID:    fmt.Sprintf("%d", mrResp.ProjectID),
		SourceBranch: mrResp.SourceBranch,
		TargetBranch: mrResp.TargetBranch,
		SourceSHA:    mrResp.SHA,
		TargetSHA:    targetSHA,
		ForkMR:       mrResp.SourceProjectID != 0 && mrResp.SourceProjectID != mrResp.ProjectID,
	}, mrResp.Author.Username, nil
}

// mrChangedFiles enumerates every path touched by the MR via
// GET .../merge_requests/:iid/changes (new_path and old_path per D-076).
func (c *Client) mrChangedFiles(project, mr string) ([]string, error) {
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/changes",
		url.PathEscape(project), url.PathEscape(mr))
	status, raw, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("gitlab: get MR changes %s!%s: unexpected status %d", project, mr, status)
	}
	var resp struct {
		Changes []struct {
			OldPath string `json:"old_path"`
			NewPath string `json:"new_path"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("gitlab: decode MR changes %s!%s: %w", project, mr, err)
	}

	seen := make(map[string]struct{})
	var paths []string
	for _, ch := range resp.Changes {
		for _, p := range []string{ch.OldPath, ch.NewPath} {
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			paths = append(paths, p)
		}
	}
	return paths, nil
}

func (c *Client) probeCapabilities(project, mr string) (forge.CapabilityFlags, error) {
	caps := forge.CapabilityFlags{
		// merge_ref (C16) is available on all tiers — record-only digest axis.
		MergeResultDigestRecordable: true,
	}

	status, raw, err := c.do(http.MethodGet,
		fmt.Sprintf("/api/v4/projects/%s", url.PathEscape(project)), nil, "")
	if err != nil {
		return forge.CapabilityFlags{}, err
	}
	if status == http.StatusNotFound {
		return caps, nil
	}
	if status != http.StatusOK {
		return forge.CapabilityFlags{}, fmt.Errorf("gitlab: get project %s: unexpected status %d", project, status)
	}
	var proj struct {
		OnlyAllowMergeIfAllDiscussionsAreResolved bool   `json:"only_allow_merge_if_all_discussions_are_resolved"`
		MergeTrainsEnabled                        bool   `json:"merge_trains_enabled"`
		CIConfigPath                              string `json:"ci_config_path"`
	}
	if err := json.Unmarshal(raw, &proj); err != nil {
		return forge.CapabilityFlags{}, fmt.Errorf("gitlab: decode project %s: %w", project, err)
	}
	caps.DiscussionsResolvedGate = proj.OnlyAllowMergeIfAllDiscussionsAreResolved
	caps.MergeTrainAvailable = proj.MergeTrainsEnabled
	caps.ProtectedPipelineExternal = strings.Contains(proj.CIConfigPath, "@")

	hasRules, err := c.hasApprovalRulesAPI(project, mr)
	if err != nil {
		return forge.CapabilityFlags{}, err
	}
	caps.HasApprovalRulesAPI = hasRules
	if hasRules {
		caps.Tier = forge.TierPremium
	} else {
		caps.Tier = forge.TierFree
	}
	return caps, nil
}

// hasApprovalRulesAPI probes GET .../approval_rules with pagination. A 404 or 403
// fail-safes to false (Free tier — no invented Premium features).
func (c *Client) hasApprovalRulesAPI(project, mr string) (bool, error) {
	page := 1
	for {
		path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/approval_rules?per_page=100&page=%d",
			url.PathEscape(project), url.PathEscape(mr), page)
		status, raw, err := c.do(http.MethodGet, path, nil, "")
		if err != nil {
			return false, err
		}
		switch status {
		case http.StatusNotFound, http.StatusForbidden:
			return false, nil
		case http.StatusOK:
			var rules []json.RawMessage
			if err := json.Unmarshal(raw, &rules); err != nil {
				return false, fmt.Errorf("gitlab: decode approval rules %s!%s: %w", project, mr, err)
			}
			if len(rules) == 0 {
				return page > 1, nil
			}
			for _, r := range rules {
				var rule struct {
					ApprovalsRequired int `json:"approvals_required"`
				}
				if err := json.Unmarshal(r, &rule); err != nil {
					return false, fmt.Errorf("gitlab: decode approval rule %s!%s: %w", project, mr, err)
				}
				if rule.ApprovalsRequired > 0 {
					return true, nil
				}
			}
			if len(rules) < 100 {
				return false, nil
			}
			page++
		default:
			return false, fmt.Errorf("gitlab: get approval rules %s!%s: unexpected status %d", project, mr, status)
		}
	}
}

var _ forge.Snapshotter = (*Client)(nil)
