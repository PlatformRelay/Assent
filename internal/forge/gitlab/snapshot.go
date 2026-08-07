package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge"
)

// Snapshot implements forge.Snapshotter. It reads MR heads (via GetMR semantics),
// changed-file paths from the MR diffs API (D-076), project merge settings and
// approval-rules presence for capability flags, and bot threads for reconciliation.
// Optional endpoints that return 404/403 fail safe to honest tier gaps — never
// invented Premium capabilities.
func (c *Client) Snapshot(project, mr string) (forge.Snapshot, error) {
	meta, err := c.mrWithAuthor(project, mr)
	if err != nil {
		return forge.Snapshot{}, err
	}
	info := meta.info

	changed, err := c.mrChangedFiles(project, mr, meta.changesCount)
	if err != nil {
		return forge.Snapshot{}, err
	}
	sort.Strings(changed.paths)

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
			Author:            meta.author,
			ForkMR:            info.ForkMR,
		},
		ChangedFiles: changed.paths,
		// Set EXPLICITLY on the success path (ADR-0020 §1): the zero value
		// would fail safe to REVIEW, but an adapter must never rely on that.
		ChangedFilesComplete: changed.complete,
		ChangedFilesGap:      changed.gap,
		Capabilities:         caps,
		BotThreads:           threads,
	}, nil
}

// mrMeta is the decoded MR GET view Snapshot needs: the pinned heads/author plus
// the RAW changes_count string, which ADR-0020 §2 uses as the independent
// cross-check on changed-file enumeration completeness. GitLab reports it as a
// STRING and caps it with a "+" suffix (commonly at 1000 files), so it is kept
// unparsed here and interpreted by the completeness check.
type mrMeta struct {
	info         MRInfo
	author       string
	changesCount string
}

func (c *Client) mrWithAuthor(project, mr string) (mrMeta, error) {
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s", url.PathEscape(project), url.PathEscape(mr))
	status, raw, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return mrMeta{}, err
	}
	if status != http.StatusOK {
		return mrMeta{}, fmt.Errorf("gitlab: get MR %s!%s: unexpected status %d", project, mr, status)
	}
	var mrResp struct {
		IID             int    `json:"iid"`
		ProjectID       int    `json:"project_id"`
		SourceProjectID int    `json:"source_project_id"`
		SHA             string `json:"sha"`
		SourceBranch    string `json:"source_branch"`
		TargetBranch    string `json:"target_branch"`
		ChangesCount    string `json:"changes_count"`
		Author          struct {
			Username string `json:"username"`
		} `json:"author"`
	}
	if err := json.Unmarshal(raw, &mrResp); err != nil {
		return mrMeta{}, fmt.Errorf("gitlab: decode MR %s!%s: %w", project, mr, err)
	}

	targetSHA, err := c.branchTip(project, mrResp.TargetBranch)
	if err != nil {
		return mrMeta{}, err
	}

	return mrMeta{
		info: MRInfo{
			IID:          fmt.Sprintf("%d", mrResp.IID),
			ProjectID:    fmt.Sprintf("%d", mrResp.ProjectID),
			SourceBranch: mrResp.SourceBranch,
			TargetBranch: mrResp.TargetBranch,
			SourceSHA:    mrResp.SHA,
			TargetSHA:    targetSHA,
			ForkMR:       mrResp.SourceProjectID != 0 && mrResp.SourceProjectID != mrResp.ProjectID,
		},
		author:       mrResp.Author.Username,
		changesCount: mrResp.ChangesCount,
	}, nil
}

const (
	// diffsPerPage is the page size for GET .../merge_requests/:iid/diffs
	// (ADR-0020 §2).
	diffsPerPage = 100

	// maxDiffPages is the HARD CEILING on paginated diff requests (ADR-0020 §2):
	// maxDiffPages * diffsPerPage = 10,000 diff entries. Reaching it without a
	// terminating short page means completeness cannot be proven — the
	// enumeration is then declared incomplete, never silently truncated. It is a
	// named constant so an instance with higher diff limits can be accommodated
	// by a future flag without a contract change.
	maxDiffPages = 100
)

// changedFileSet is the enumeration result: the deduped path set plus the
// ADR-0020 §1 completeness verdict. complete and gap are two halves of ONE
// honest statement — gap is non-empty IFF complete is false.
type changedFileSet struct {
	paths    []string
	complete bool
	gap      string
}

// mrChangedFiles enumerates every path touched by the MR via the PAGINATED
// GET .../merge_requests/:iid/diffs (new_path and old_path per D-076) and
// decides whether that enumeration is PROVABLY COMPLETE (ADR-0020 §2, D-119).
//
// The deprecated unpaginated .../changes endpoint is gone: it truncates at the
// instance diff limit with no way to tell a short list from a complete one, and
// its 404 → empty-list mapping turned a forge anomaly into "this MR changes
// nothing" — the fail-open that starves the D-042 self-vouch guard, because in
// checkout-less runs this list is the SOLE `.assent/**` detector.
//
// Completeness requires ALL THREE (ADR-0020 §2):
//
//  1. the enumeration terminated below the ceiling — a SHORT final page is the
//     only proof that no further page exists; a FULL page at maxDiffPages
//     proves nothing about the tail;
//  2. changesCount (the MR GET's changes_count string) parses as a plain
//     integer with no "+" suffix;
//  3. that integer equals the number of enumerated diff ENTRIES.
//
// (3) compares ENTRIES, not the returned path count: one rename entry yields
// two paths, so comparing paths would fail-safe-degrade every renaming MR
// forever.
//
// Any violation — plus a decoded per-entry overflow marker — yields
// complete=false with a SPECIFIC gap reason. The partial path list is still
// returned: an `.assent/**` path that IS visible must still dominate to BLOCK.
//
// (2)/(3) are kept even though GitLab caps changes_count with a "+" suffix well
// below the page ceiling: that skews conservative only (a capped count degrades
// to REVIEW, never fail-open) and must NOT be "fixed" by trusting the ceiling
// alone (ADR-0020 Consequences).
//
// A NON-200 on the diffs endpoint (INCLUDING 404) is a HARD ERROR, not a gap:
// the MR provably exists by this point (the MR GET succeeded), so a missing
// diff resource is forge anomaly, never evidence of an empty change set
// (ADR-0020 §3).
func (c *Client) mrChangedFiles(project, mr, changesCount string) (changedFileSet, error) {
	seen := make(map[string]struct{})
	var paths []string
	entries := 0
	overflow := false
	terminated := false

	for page := 1; page <= maxDiffPages; page++ {
		path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/diffs?per_page=%d&page=%d",
			url.PathEscape(project), url.PathEscape(mr), diffsPerPage, page)
		status, raw, err := c.do(http.MethodGet, path, nil, "")
		if err != nil {
			return changedFileSet{}, err
		}
		if status != http.StatusOK {
			return changedFileSet{}, fmt.Errorf("gitlab: get MR diffs %s!%s page %d: unexpected status %d",
				project, mr, page, status)
		}
		var list []struct {
			OldPath string `json:"old_path"`
			NewPath string `json:"new_path"`
			// Overflow is the forge's marker that the diff COLLECTION overflowed
			// the instance limit — an enumeration gap. (A per-file `too_large` is
			// a RENDERING limit: both paths are still enumerated, so it is
			// deliberately NOT treated as a gap.)
			Overflow bool `json:"overflow"`
		}
		if err := json.Unmarshal(raw, &list); err != nil {
			return changedFileSet{}, fmt.Errorf("gitlab: decode MR diffs %s!%s page %d: %w", project, mr, page, err)
		}

		entries += len(list)
		for _, ch := range list {
			if ch.Overflow {
				overflow = true
			}
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

		if len(list) < diffsPerPage {
			terminated = true
			break
		}
	}

	set := changedFileSet{paths: paths}
	set.gap = enumerationGap(entries, changesCount, terminated, overflow)
	set.complete = set.gap == ""
	return set, nil
}

// enumerationGap returns the SPECIFIC reason the enumeration cannot be proven
// complete, or "" when every ADR-0020 §2 condition holds. The checks are ordered
// so the reported reason is deterministic when several apply.
func enumerationGap(entries int, changesCount string, terminated, overflow bool) string {
	if !terminated {
		return fmt.Sprintf("diff pagination ceiling of %d pages (%d entries) reached without a terminating short page",
			maxDiffPages, maxDiffPages*diffsPerPage)
	}
	if overflow {
		return "forge reported a diff overflow marker on at least one enumerated entry"
	}
	if changesCount == "" {
		return "MR changes_count absent — the enumeration cross-check is unavailable"
	}
	if strings.HasSuffix(changesCount, "+") {
		return fmt.Sprintf("MR changes_count %q is capped (trailing \"+\") — the true change count is unknown", changesCount)
	}
	n, err := strconv.Atoi(changesCount)
	if err != nil {
		return fmt.Sprintf("MR changes_count %q is not a plain integer", changesCount)
	}
	if n != entries {
		return fmt.Sprintf("MR changes_count %d does not equal the %d enumerated diff entries", n, entries)
	}
	return ""
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
