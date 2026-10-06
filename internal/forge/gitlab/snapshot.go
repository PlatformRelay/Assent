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
			Labels:            info.Labels,
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
		IID             int      `json:"iid"`
		ProjectID       int      `json:"project_id"`
		SourceProjectID int      `json:"source_project_id"`
		SHA             string   `json:"sha"`
		SourceBranch    string   `json:"source_branch"`
		TargetBranch    string   `json:"target_branch"`
		ChangesCount    string   `json:"changes_count"`
		Labels          []string `json:"labels"`
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
			IID:             fmt.Sprintf("%d", mrResp.IID),
			ProjectID:       fmt.Sprintf("%d", mrResp.ProjectID),
			SourceProjectID: fmt.Sprintf("%d", mrResp.SourceProjectID),
			SourceBranch:    mrResp.SourceBranch,
			TargetBranch:    mrResp.TargetBranch,
			SourceSHA:       mrResp.SHA,
			TargetSHA:       targetSHA,
			ForkMR:          mrResp.SourceProjectID != 0 && mrResp.SourceProjectID != mrResp.ProjectID,
			Labels:          mrResp.Labels,
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

func (c *Client) probeCapabilities(project, mr string) (forge.CapabilityReport, error) {
	// E10-S04 (ADR-0021 item 3): the probe TRANSPORT failure is a hard process
	// error — it propagates as this function's error, never a silent downgrade
	// to `unknown` (REQ-E10-S04-05: a 5xx on one endpoint must not flip
	// APPROVE→REVIEW as an "unknown" capability; a broken forge aborts the run).
	caps := map[forge.Capability]forge.CapabilityEntry{
		// C constants licensed by named conformance cases (S00 Q2 rows 1/5/6/7):
		// the p3e5-* reconciliation cases license resolvable-threads; the
		// sha-guard-* cases license sha-guarded-merge; the dossier records MWPS
		// as tier-independent deferred arming with revoke-on-push.
		forge.CapabilityResolvableThreads: {State: forge.CapabilitySupported, Reason: "constant supported — licensed by the p3e5-* reconciliation conformance cases (S00 Q2 row 1)"},
		forge.CapabilitySHAGuardedMerge:   {State: forge.CapabilitySupported, Reason: "constant supported — licensed by the sha-guard-* conformance cases (PUT /merge?sha= CAS)"},
		forge.CapabilityDeferredMergeArming: {
			State:  forge.CapabilitySupported,
			Reason: "constant supported — merge-when-pipelines-succeed is tier-independent (dossier C11)",
		},
		forge.CapabilityArmingRevokedOnPush: {
			State:  forge.CapabilitySupported,
			Reason: "constant supported — any new commit cancels MWPS arming (dossier C11)",
		},
		// Honestly absent on GitLab (no REQUEST_CHANGES primitive, no analogue;
		// ADR-0017 §3 uses threads — dossier C4/C8).
		forge.CapabilityBlockingReview: {State: forge.CapabilityAbsent, Reason: "GitLab has no REQUEST_CHANGES review primitive; blocking review is carried by resolvable threads (ADR-0017 §3)"},
		forge.CapabilityReviewDismissalRestrictions: {
			State:  forge.CapabilityAbsent,
			Reason: "no GitLab analogue (dossier row 4)",
		},
		// Retired SEC-04 heuristic (S00 Q2 row 11): a ci_config_path substring
		// test is a heuristic, not a probe — it proves neither that the
		// referenced CI config sits on a protected branch nor that the MR author
		// cannot push to it. Until a decidable predicate exists (OQ-33), this is
		// unknown, and unknown refuses to arm (ADR-0021 §3).
		forge.CapabilityProtectedPipelineSource: {
			State:  forge.CapabilityUnknown,
			Reason: "no operationally decidable predicate is probed — the retired ci_config_path '@' substring heuristic was the SEC-04 shape (audit 2026-08-09); OQ-33 records the candidate probes",
		},
		// Never probed by any adapter version (audit RELI-03): unknown, not
		// assumed-true. The arming path does not consult it in v1, but doctor
		// must state it honestly.
		forge.CapabilityApprovalResetOnPush: {
			State:  forge.CapabilityUnknown,
			Reason: "not probed — reset_approvals_on_push is not read by any probe today (audit RELI-03); an unprobed setting may never be cited as a safety argument",
		},
	}

	status, raw, err := c.do(http.MethodGet,
		fmt.Sprintf("/api/v4/projects/%s", url.PathEscape(project)), nil, "")
	if err != nil {
		return forge.CapabilityReport{}, err
	}
	if status == http.StatusNotFound {
		return forge.NewCapabilityReport(caps)
	}
	if status != http.StatusOK {
		return forge.CapabilityReport{}, fmt.Errorf("gitlab: get project %s: unexpected status %d", project, status)
	}
	var proj struct {
		OnlyAllowMergeIfAllDiscussionsAreResolved bool   `json:"only_allow_merge_if_all_discussions_are_resolved"`
		MergeTrainsEnabled                        bool   `json:"merge_trains_enabled"`
		CIConfigPath                              string `json:"ci_config_path"`
	}
	if err := json.Unmarshal(raw, &proj); err != nil {
		return forge.CapabilityReport{}, fmt.Errorf("gitlab: decode project %s: %w", project, err)
	}
	caps[forge.CapabilityThreadsBlockMerge] = probedEntry(proj.OnlyAllowMergeIfAllDiscussionsAreResolved,
		"only_allow_merge_if_all_discussions_are_resolved is true (forge dossier C3 / ADR-0009)",
		"only_allow_merge_if_all_discussions_are_resolved is false — unresolved discussions do not block the merge (forge dossier C3 / ADR-0009)")
	caps[forge.CapabilityMergeResultPinning] = probedEntry(proj.MergeTrainsEnabled,
		"merge_trains_enabled is true — merge trains publish a real merge-result digest (S00 Q2 row 8)",
		"merge_trains_enabled is false — plain merge exposes no merge-result digest; the CAS pins the adapter-synthesised digest (dossier C16)")

	hasRules, err := c.hasApprovalRulesAPI(project, mr)
	if err != nil {
		return forge.CapabilityReport{}, err
	}
	if hasRules {
		caps[forge.CapabilityEligibleApprovalEvidence] = forge.CapabilityEntry{
			State:  forge.CapabilitySupported,
			Reason: "approval-rules API present with required approvals (Premium tier); eligible_approvers is forge-computed (S00 Q2 row 9, graded full)",
		}
	} else {
		caps[forge.CapabilityEligibleApprovalEvidence] = forge.CapabilityEntry{
			State:  forge.CapabilityAbsent,
			Reason: "approval-rules API absent (GitLab Free) — no forge-computed eligible approver set, require-review unsatisfiable (dossier C6/C7)",
		}
	}
	return forge.NewCapabilityReport(caps)
}

// probedEntry builds an entry whose state is the probe's boolean outcome and
// whose reason names the probe and its outcome, so doctor can show WHY the
// state is what it is.
func probedEntry(v bool, trueReason, falseReason string) forge.CapabilityEntry {
	if v {
		return forge.CapabilityEntry{State: forge.CapabilitySupported, Reason: "probe: " + trueReason}
	}
	return forge.CapabilityEntry{State: forge.CapabilityAbsent, Reason: "probe: " + falseReason}
}

// hasApprovalRulesAPI probes GET .../approval_rules with pagination. A 404 or 403
// fail-safes to false (Free tier — no invented Premium features).
//
// The loop is CAPPED at maxListPages (AUD-S10 / REL-03). The cap is an ERROR,
// deliberately NOT the 404/403 Free-tier fail-safe: a paginator that never
// shortens is a forge anomaly, not evidence that the instance lacks the
// approval-rules API. Erroring aborts the run with zero forge writes, which is
// strictly safer than the previous unbounded spin.
func (c *Client) hasApprovalRulesAPI(project, mr string) (bool, error) {
	for page := 1; ; page++ {
		if page > maxListPages {
			return false, fmt.Errorf(
				"gitlab: probe approval rules %s!%s: pagination cap of %d pages reached without a short page",
				project, mr, maxListPages)
		}
		path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/approval_rules?per_page=%d&page=%d",
			url.PathEscape(project), url.PathEscape(mr), listPerPage, page)
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
			if len(rules) < listPerPage {
				return false, nil
			}
		default:
			return false, fmt.Errorf("gitlab: get approval rules %s!%s: unexpected status %d", project, mr, status)
		}
	}
}

var _ forge.Snapshotter = (*Client)(nil)
