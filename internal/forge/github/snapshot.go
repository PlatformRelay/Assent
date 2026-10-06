package github

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge"
)

// snapshot.go — the read half of the port (E10-S07): GetMR, Snapshot, the
// MR-relative governed-subject accessors FileAtBase/FileAtHead, and FileAtRef
// for the ref-addressed decision-input loads the port keeps ref-addressed.
//
// The addressing model is S00 Q1's: the governed subject is addressed relative
// to the merge request, never by (project, branch-name). On GitHub a fork PR's
// head content is reachable in the BASE repo at the head SHA (refs/pull/N/head
// carries the same commit), so FileAtHead reads at the PINNED head SHA — the
// same value the record pins into pins.sourceSha, so the record and the read
// cannot disagree. A branch-name read inside the base repo is forbidden: on a
// fork it 404s and the orAbsent mapping would mint a fabricated whole-file
// DELETE (S00 Q1).

// mergeRefState is the adapter's recorded outcome of the merge-ref probe
// (mergeResultDigest), the state the merge-result-pinning capability entry
// grades from. Empty means never probed on this run.
const (
	mergeRefUnprobed    = ""
	mergeRefReadable    = "readable"
	mergeRefUnreadable  = "unreadable"
	mergeRefUnmergeable = "unmergeable"
)

// cleanMergeableState is the PR mergeable_state under which the merge ref
// refs/pull/{n}/merge is readable and the digest axis is live (dossier C16).
// Any other state — blocked, dirty, draft, queued — means the PR is not
// mergeable now, which is exactly the merge-queue shape (dossier C14): the
// merge ref is absent and the digest axis is honestly unavailable.
const cleanMergeableState = "clean"

// setMergeRefState records the merge-ref probe outcome for the capability
// report (probeCapabilities grades merge-result-pinning from it; the state is
// adapter-internal, guarded by the same small-state mutex as the pin caches).
func (c *Client) setMergeRefState(state string) {
	c.scopeMu.Lock()
	c.mergeRefState = state
	c.scopeMu.Unlock()
}

// mergeRefStateOf returns the recorded merge-ref probe outcome, "" when never
// probed.
func (c *Client) mergeRefStateOf() string {
	c.scopeMu.Lock()
	defer c.scopeMu.Unlock()
	return c.mergeRefState
}

// recordMergeableState records the PR's mergeable_state from the latest PR
// read (freshPR). mergeResultDigest consults it so the digest axis and the
// merge-result-pinning grading both derive from the SAME PR read. It carries
// its own mutex: the PR read chain (mrPinned) holds scopeMu, so recording
// under scopeMu would re-enter a held mutex.
func (c *Client) recordMergeableState(state string) {
	c.stateMu.Lock()
	c.lastMergeState = state
	c.stateMu.Unlock()
}

// lastMergeableState returns the mergeable_state of the latest PR read, ""
// when this client has read no PR yet on this run.
func (c *Client) lastMergeableState() string {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.lastMergeState
}

// recordPRChangedFiles records the PR object's changed_files count from the
// latest PR read (freshPR) — the enumeration count mrChangedFiles's
// completeness decision cross-checks (ADR-0020 §2). A nil pointer is the
// forge REPORTING NO COUNT (the JSON field was absent): nil is information,
// not a 0 count. Same mutex discipline as recordMergeableState: the PR read
// chain holds pinMu, so this state carries its own mutex.
func (c *Client) recordPRChangedFiles(n *int) {
	c.stateMu.Lock()
	c.lastChangedFiles = n
	c.stateMu.Unlock()
}

// lastPRChangedFiles returns the changed_files count of the latest PR read;
// nil when this client has read no PR yet or the forge reported no count.
func (c *Client) lastPRChangedFiles() *int {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.lastChangedFiles
}

// ghLabel is one label of a PR.
type ghLabel struct {
	Name string `json:"name"`
}

// labelNames maps the PR label objects to the port's string list.
func labelNames(labels []ghLabel) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, l.Name)
	}
	return out
}

// ghPR is the subset of the PR object the port reads. base.SHA is the base
// BRANCH TIP (not the merge base); head.SHA is the PR head; head.Ref is the
// PR's own head BRANCH NAME (S00 Q1 forbids smuggling refs/pull/N/head into
// MRInfo.SourceBranch — it corrupts a documented field and leaks into
// rendering, ADR-0021 item 5). NodeID is the PR's GraphQL node id, which
// enablePullRequestAutoMerge addresses the pull request by (dossier C11).
type ghPR struct {
	Number int       `json:"number"`
	SHA    string    `json:"sha"`
	NodeID string    `json:"node_id"`
	Labels []ghLabel `json:"labels"`
	// ChangedFiles is the PR object's own changed_files count — the
	// enumeration count the changed-file completeness cross-check consults
	// (ADR-0020 §2, condition (2)'s GitHub column). It is a POINTER: an
	// ABSENT count is unreported (nil), and the completeness verdict grades
	// incomplete on it — a silently-zero count would read an unreportable
	// enumeration as "matches the count 0" (fail-open, E10 branch-review
	// round 2, finding 7).
	ChangedFiles *int `json:"changed_files"`
	Base         struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo *struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
	Head struct {
		Ref  string `json:"ref"`
		SHA  string `json:"sha"`
		Repo *struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	MergeableState string `json:"mergeable_state"`
}

// mrInfoFromPR maps a decoded PR object to the port's MRInfo, applying the two
// fail-closed metadata guards both GetMR and the fresh Snapshot read owe:
// unpinned refs are refused, and the null-head-repo trap (below) is an ERROR.
func mrInfoFromPR(repo string, pr ghPR) (forge.MRInfo, error) {
	if pr.Head.SHA == "" || pr.Head.Ref == "" || pr.Base.Ref == "" {
		return forge.MRInfo{}, fmt.Errorf("github: PR %s is missing head/base refs — refusing to evaluate unpinned metadata", repo)
	}
	// The absent-means-trusted trap (REQ-E10-S07-01): a null head.repo is an
	// ERROR, never ForkMR=false; a fork PR's head lives in the fork, and a
	// deleted fork's head repo is exactly the state a wrong "not a fork"
	// reading would evaluate as trusted.
	if pr.Base.Repo == nil || pr.Head.Repo == nil {
		return forge.MRInfo{}, fmt.Errorf("github: PR %s#%d reports no head or base repository — fork identity unprovable, fail-closed", repo, pr.Number)
	}
	return forge.MRInfo{
		IID:             fmt.Sprintf("%d", pr.Number),
		ProjectID:       repo,
		SourceProjectID: pr.Head.Repo.FullName,
		// SourceBranch is the PR's own HEAD branch name (head.ref) — NOT
		// base.ref, and never the PR head ref refs/pull/N/head (S00 Q1 forbids
		// smuggling the ref into a documented field; it corrupts the field and
		// leaks into rendering).
		SourceBranch: pr.Head.Ref,
		TargetBranch: pr.Base.Ref,
		SourceSHA:    pr.SHA,
		TargetSHA:    pr.Base.SHA,
		ForkMR:       pr.Head.Repo.FullName != pr.Base.Repo.FullName,
		Labels:       labelNames(pr.Labels),
	}, nil
}

// repoParts splits an "owner/repo" project address. Anything else is a hard
// error — the adapter never guesses a repo shape.
func repoParts(project string) (string, error) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(project), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("github: project %q is not an owner/repo pair", project)
	}
	return parts[0] + "/" + parts[1], nil
}

// freshPR performs the raw PR GET GetMR and the CAS re-reads share: one GET
// that reports the head SHA (pr.sha), the PR's own head branch (head.ref), the
// base branch tip (base.sha, NOT the merge-base), the fork identities, and the
// PR's mergeable_state, which the adapter records for the merge-ref probe.
func (c *Client) freshPR(repo, mr string) (ghPR, error) {
	status, _, raw, err := c.do(http.MethodGet, "/repos/"+repo+"/pulls/"+mr, nil, "")
	if err != nil {
		return ghPR{}, err
	}
	if status != http.StatusOK {
		return ghPR{}, fmt.Errorf("github: get PR %s#%s: unexpected status %d", repo, mr, status)
	}
	var pr ghPR
	if err := json.Unmarshal(raw, &pr); err != nil {
		return ghPR{}, fmt.Errorf("github: decode PR %s#%s: %w", repo, mr, err)
	}
	c.recordMergeableState(pr.MergeableState)
	c.recordPRChangedFiles(pr.ChangedFiles)
	return pr, nil
}

// GetMR reads the PR metadata: head SHA, base branch tip (NOT the merge base),
// branches, fork detection and labels (REQ-E10-S07-01).
//
// Fork detection closes the absent-means-trusted trap (audit SEC-05's class): a
// null `head.repo` is an ERROR, never ForkMR=false — GitHub nulls head.repo for
// a deleted fork, exactly the state a wrong "not a fork" reading would evaluate
// as trusted.
func (c *Client) GetMR(project, mr string) (forge.MRInfo, error) {
	repo, err := repoParts(project)
	if err != nil {
		return forge.MRInfo{}, err
	}
	pr, err := c.freshPR(repo, mr)
	if err != nil {
		return forge.MRInfo{}, err
	}
	info, err := mrInfoFromPR(repo, pr)
	if err != nil {
		return forge.MRInfo{}, err
	}
	// E10 branch-review fix: GetMR IS the run's own pinned read — it populates
	// the pin so FileAtBase/FileAtHead judge bytes at the SAME SHAs this read
	// reported (REV1-S01's one-read-chain rule). A third, separate PR read
	// would let a head move between the record's pin (this read's SHAs) and
	// the governed read (its bytes) — the move-and-restore merge of
	// un-evaluated bytes the branch review's CRITICAL names. pinMu, not
	// scopeMu: mrPinned consults the pin while holding pinMu, and a mutex may
	// never be re-entered.
	//
	// FIRST WRITE WINS (E10 branch-review round 2, findings 2/6): the pin is
	// written only when the cache is empty or pinned to a DIFFERENT
	// (project, mr). A same-MR re-read — the CAS's CurrentHeads read — must NOT
	// move the evaluation pin: Approve's commit_id and the governed reads
	// follow the pin, and the pin must stay at the SHAs the evaluation judged,
	// whatever the forge reports later. This is what makes CurrentHeads' "goes
	// FRESH, deliberately not through the mrPinned cache" comment true: the
	// re-read never rewrites the pin. A different (project, mr) evicts the pin
	// (a stale pin would be worse than none).
	key := project + "/" + mr
	c.pinMu.Lock()
	if c.mrPinnedKey != key {
		c.mrPinnedKey = key
		c.mrPinnedInfo = info
	}
	c.pinMu.Unlock()
	return info, nil
}

// mrPinned returns the pinned MR read for (project, mr), populating the cache
// on first touch — the same read chain the GitLab adapter uses, so judged bytes
// and record pins cannot disagree (REV1-S01). The pin is the CONTENT accessors'
// read chain only: the CAS reads (CurrentHeads) deliberately go FRESH, so the
// SHA-guard sees the forge's current heads, not the cached evaluation pin.
func (c *Client) mrPinned(project, mr string) (forge.MRInfo, error) {
	key := project + "/" + mr
	c.pinMu.Lock()
	if c.mrPinnedKey == key && c.mrPinnedInfo.IID != "" {
		info := c.mrPinnedInfo
		c.pinMu.Unlock()
		return info, nil
	}
	c.pinMu.Unlock()
	// GetMR IS the pin: it fills the cache itself (the run's own pinned read),
	// so this re-read only happens for an MR the run never described. GetMR
	// takes pinMu for the write; never call it while holding pinMu.
	return c.GetMR(project, mr)
}

// contentScopeOK probes the content-scope permission the read it licenses needs
// (S00 Q4): a sibling content read in the same repo at the same ref — the root
// listing, GET /repos/{repo}/contents?ref={ref} — returning 200 proves
// contents-read is granted THERE, and only then is a 404 on the specific path
// genuine absence. The probe is cached per (repo, ref) for the run, so it costs
// one request, not one per governed read. A probe that does not answer 200 is a
// permission failure (ErrUnauthorized) or a transport failure — NEVER absence:
// "the ref does not exist" is correctly not path absence, and the wording says
// exactly that.
func (c *Client) contentScopeOK(repo, ref string) error {
	key := repo + "@" + ref
	c.scopeMu.Lock()
	ok, cached := c.scopeOK[key]
	c.scopeMu.Unlock()
	if cached && ok {
		return nil
	}
	status, _, _, err := c.do(http.MethodGet, "/repos/"+repo+"/contents?ref="+urlQueryEscape(ref), nil, "")
	if err != nil {
		return fmt.Errorf("github: content-scope probe %s@%s: %w", repo, ref, err)
	}
	granted := status == http.StatusOK
	c.scopeMu.Lock()
	if c.scopeOK == nil {
		c.scopeOK = map[string]bool{}
	}
	c.scopeOK[key] = granted
	c.scopeMu.Unlock()
	if granted {
		return nil
	}
	return fmt.Errorf("github: content unreadable at ref %q: absent and forbidden could not be distinguished (probe answered %d) — %w",
		ref, status, forge.ErrUnauthorized)
}

// urlQueryEscape percent-encodes a query parameter value.
func urlQueryEscape(s string) string { return url.QueryEscape(s) }

// pathEsc percent-encodes a path segment for a GitHub REST URL (the Contents
// API requires URL-encoded paths; the same discipline the GitLab adapter
// applies to its file paths).
func pathEsc(path string) string {
	return url.PathEscape(path)
}

// ghContent is the Contents API response for a single file. GitHub serves files
// ≤ 1 MiB inline as base64; larger ones come back with truncated: true and
// empty content — never judged as absence.
type ghContent struct {
	Content   string `json:"content"`
	Encoding  string `json:"encoding"`
	Truncated bool   `json:"truncated"`
	Type      string `json:"type"`
	Size      int    `json:"size"`
}

// ghContentOf decodes a Contents API response into file bytes. A truncated
// content body is NEVER absence: the adapter fails closed with an error naming
// the limit, because a silently empty governed file would mint a fabricated
// whole-file lifecycle event.
func ghContentOf(raw []byte) ([]byte, error) {
	var payload ghContent
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("github: decode contents response: %w", err)
	}
	if payload.Truncated {
		return nil, errors.New("github: contents response is truncated (file over 1 MiB via this endpoint) — refusing to judge truncated bytes")
	}
	switch payload.Encoding {
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(strings.NewReplacer("\n", "", "\r", "").Replace(payload.Content))
		if err != nil {
			return nil, fmt.Errorf("github: decode base64 contents: %w", err)
		}
		return decoded, nil
	case "":
		return []byte{}, nil
	default:
		return nil, fmt.Errorf("github: contents encoding %q is not supported by this adapter", payload.Encoding)
	}
}

// contentRead reads a file's bytes at a ref of a repo through the Q4
// content-scope probe: a 404 is only claimed as ABSENT after the probe proved
// contents-read there.
func (c *Client) contentRead(repo, path, ref string) ([]byte, error) {
	if err := c.contentScopeOK(repo, ref); err != nil {
		return nil, err
	}
	status, _, raw, err := c.do(http.MethodGet, "/repos/"+repo+"/contents/"+pathEsc(path)+"?ref="+urlQueryEscape(ref), nil, "")
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusOK:
		return ghContentOf(raw)
	case status == http.StatusNotFound:
		// The probe proved contents-read at this (repo, ref), so this 404 is
		// genuine absence (S00 Q4's disambiguation, now earned).
		return nil, fmt.Errorf("github: %w: file %q at ref %q", forge.ErrNotFound, path, ref)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return nil, fmt.Errorf("github: %w: file %q at ref %q (status %d)", forge.ErrUnauthorized, path, ref, status)
	default:
		return nil, fmt.Errorf("github: get file %q at ref %q: unexpected status %d", path, ref, status)
	}
}

// FileAtBase reads the governed subject's content on the BASE side of the PR
// (REQ-E10-S07-01): the base side is the TARGET repo's pinned base commit by
// definition, so this reads the pinned target SHA the adapter's own MR read
// reported.
func (c *Client) FileAtBase(project, mr, path string) ([]byte, error) {
	info, err := c.mrPinned(project, mr)
	if err != nil {
		return nil, err
	}
	return c.contentRead(info.ProjectID, path, info.TargetSHA)
}

// FileAtHead reads the governed subject's content on the HEAD side of the PR.
// MR-relative (S00 Q1): the head is read at the PINNED head SHA in the TARGET
// repo — the PR refs make the fork's head commit reachable there; addressing by
// the head SHA is S00 Q1's preferred form ("the same value pinned into
// pins.sourceSha, so the record and the read cannot disagree"). A branch-name
// read inside the target repo is forbidden: on a fork it 404s and the orAbsent
// mapping would mint a fabricated whole-file DELETE.
func (c *Client) FileAtHead(project, mr, path string) ([]byte, error) {
	info, err := c.mrPinned(project, mr)
	if err != nil {
		return nil, err
	}
	if info.ForkMR && (info.SourceProjectID == "" || info.SourceProjectID == "0") {
		return nil, fmt.Errorf("github: fork PR %s#%s has no source repository — head content cannot be addressed (fail-closed)", project, mr)
	}
	return c.contentRead(info.ProjectID, path, info.SourceSHA)
}

// FileAtRef reads a file at an explicit ref of an explicit project — the
// ref-addressed decision-input loads (ADR-0015 §1: policy, binding, config,
// pack, provider declarations, resource-owner registry; the pinned target
// SHA), never the governed subject.
func (c *Client) FileAtRef(project, path, ref string) ([]byte, error) {
	repo, err := repoParts(project)
	if err != nil {
		return nil, err
	}
	return c.contentRead(repo, path, ref)
}

// mergeResultDigest reads the adapter-owned merge-result digest (dossier C16).
// GitHub exposes no plain-merge merge-result digest API; what it DOES mint is
// the merge ref refs/pull/{n}/merge — a real merge-result commit, present only
// while the PR is mergeable and not queued. The digest is therefore that
// commit's SHA, read raw (the adapter-owned merge-result scheme, playing the
// role gitlab.SyntheticDigest plays for GitLab); it is empty when the forge
// exposes none.
//
// REQ-E10-S11-03: the merge ref is consulted ONLY when the PR's recorded
// mergeable_state is "clean" — the state under which the ref exists and the
// merge result it mints is the one the forge will produce. Any other state
// (blocked, dirty, draft, queued) is the merge-queue-or-not-mergeable shape:
// the digest is empty (no pin is fabricated) and merge-result-pinning is
// graded from that same outcome, so the capability state and the digest the
// CAS pins come from ONE read chain.
//
//   - mergeable_state "clean" + 200 → digest = the merge ref's object SHA
//     (raw, no prefix).
//   - mergeable_state not "clean" → "" with a nil error, no ref probe; the
//     unavailability is recorded so the capability report grades
//     merge-result-pinning ABSENT with the merge-queue reason.
//   - 404 → "" with a nil error, and the unavailability is recorded so the
//     capability report grades merge-result-pinning ABSENT ("merge ref
//     unreadable: not mergeable or merge queue in use"). A permission-denied
//     read renders as the same "" here, and the CAS still fails closed: the
//     port's completeForMerge refuses an empty digest, so a non-mergeable PR
//     never arms (the correct v1 posture — digest axis honestly unavailable).
//   - 401/403 (rate-limited ones are transport failures at the transport
//     layer) and any other status → a hard error, never a silent empty digest.
//
// The probe outcome rides the adapter-internal mergeRefState, which
// probeCapabilities consults — the capability grading and the digest the CAS
// pins come from the SAME read.
func (c *Client) mergeResultDigest(project, mr string) (string, error) {
	repo, err := repoParts(project)
	if err != nil {
		return "", err
	}
	// REQ-E10-S11-03: a PR that is not mergeable NOW never gets a digest. The
	// recorded state comes from the PR read the caller just made (freshPR in
	// the same Snapshot/CurrentHeads chain), so the digest axis and the PR
	// cannot disagree. No pin is fabricated for the merge-queue shape — the
	// digest stays empty and arming is refused on the capability gap.
	if state := c.lastMergeableState(); state != "" && state != cleanMergeableState {
		c.setMergeRefState(mergeRefUnmergeable)
		return "", nil
	}
	// The ref name carries slashes, so it travels as a URL-encoded path
	// segment: refs%2Fpull%2F{n}%2Fmerge (dossier C16's route).
	status, _, raw, err := c.do(http.MethodGet,
		fmt.Sprintf("/repos/%s/git/ref/refs/pull%%2F%s%%2Fmerge", repo, mr), nil, "")
	if err != nil {
		return "", err
	}
	switch {
	case status == http.StatusOK:
		var ref struct {
			Object struct {
				SHA string `json:"sha"`
			} `json:"object"`
		}
		if err := json.Unmarshal(raw, &ref); err != nil {
			return "", fmt.Errorf("github: decode merge ref %s#%s: %w", repo, mr, err)
		}
		if ref.Object.SHA == "" {
			return "", fmt.Errorf("github: merge ref refs/pull/%s/merge on %s reports no object sha", mr, repo)
		}
		c.setMergeRefState(mergeRefReadable)
		return ref.Object.SHA, nil
	case status == http.StatusNotFound:
		// Absent merge ref = the PR is not mergeable now (conflict, merge
		// queue, draft) or the forge refuses the read; both leave the digest
		// axis honestly UNAVAILABLE, and completeForMerge fails closed on it.
		c.setMergeRefState(mergeRefUnreadable)
		return "", nil
	default:
		return "", fmt.Errorf("github: get merge ref %s#%s: unexpected status %d", repo, mr, status)
	}
}

// Snapshot implements forge.Snapshotter. It reads the PR heads (a fresh read
// carrying the author, the gitlab.Snapshot mirror), the changed-file paths from
// the pull-files API (ADR-0020 §1/D-119), the merge-result digest axis and the
// capability report, plus bot threads for reconciliation. Optional endpoints
// that cannot answer fail closed to honest gaps — never invented capabilities.
func (c *Client) Snapshot(project, mr string) (forge.Snapshot, error) {
	repo, err := repoParts(project)
	if err != nil {
		return forge.Snapshot{}, err
	}
	pr, err := c.freshPR(repo, mr)
	if err != nil {
		return forge.Snapshot{}, err
	}
	info, err := mrInfoFromPR(repo, pr)
	if err != nil {
		return forge.Snapshot{}, err
	}

	// The merge-ref probe runs BEFORE the capability report so the
	// merge-result-pinning entry is graded from the probe that actually ran
	// (same read, one honest statement).
	digest, err := c.mergeResultDigest(project, mr)
	if err != nil {
		return forge.Snapshot{}, err
	}

	changed, err := c.mrChangedFiles(project, mr)
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
			MergeResultDigest: digest,
			Author:            pr.User.Login,
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

// changedFileSet is the enumeration result: the deduped path set plus the
// ADR-0020 §1 completeness verdict. complete and gap are two halves of ONE
// honest statement — gap is non-empty IFF complete is false.
type changedFileSet struct {
	paths    []string
	complete bool
	gap      string
}

// ghFileDiff is one entry of the PR files listing (GET
// /repos/{repo}/pulls/{n}/files): the new path plus, for a rename, the old one
// (both enumerated, D-076's two-paths-per-rename discipline).
type ghFileDiff struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
}

// pullFilesCap is the pull-files endpoint's file cap (dossier): a PR with
// changed_files at or above it can never be enumerated completely through this
// endpoint.
const pullFilesCap = 3000

// changedFilesVerdict is mrChangedFiles's completeness decision (ADR-0020 §1/§2,
// D-119), as a pure function so the cap shapes are unit-tested without a
// forged forge fixture. complete and gap are two halves of ONE honest
// statement — gap is non-empty IFF complete is false:
//
//   - no terminating short page: the pagination ceiling was reached —
//     incomplete (the ceiling gap).
//   - the PR reported NO changed_files count (nil): the enumeration cannot
//     cross-check completeness at all — incomplete (the unreported gap).
//   - the PR's changed_files is at or above the endpoint cap: the pull-files
//     endpoint can never serve the full change set — incomplete (the cap gap),
//     however many entries the pages did yield.
//   - the entries do not EQUAL the reported count, in EITHER direction: fewer
//     entries means the listing is provably short of the PR's own count (the
//     short-fall gap); MORE entries means the PR changed between the PR read
//     and the files read (the drift gap). Equality is the only honest complete.
//   - otherwise: the enumeration terminated on a short page and matches the
//     PR's count — complete.
func changedFilesVerdict(entries int, reported *int, terminated bool) (bool, string) {
	if !terminated {
		return false, fmt.Sprintf("pull-files pagination ceiling of %d pages (%d entries) reached without a terminating short page",
			maxListPages, maxListPages*listPerPage)
	}
	if reported == nil {
		return false, "the PR object reported no changed_files count — the enumeration completeness cross-check is unavailable"
	}
	if *reported >= pullFilesCap {
		return false, fmt.Sprintf("pull-files endpoint reports changed_files=%d and the endpoint caps at %d files — the enumeration cannot prove completeness",
			*reported, pullFilesCap)
	}
	if entries != *reported {
		if entries < *reported {
			return false, fmt.Sprintf("pull-files endpoint returned %d of the PR's changed_files=%d entries (endpoint caps at %d files)",
				entries, *reported, pullFilesCap)
		}
		return false, fmt.Sprintf("pull-files endpoint returned %d entries but the PR's changed_files=%d — the PR moved between the PR read and the files read (endpoint caps at %d files)",
			entries, *reported, pullFilesCap)
	}
	return true, ""
}

// mrChangedFiles enumerates every path touched by the PR via the PAGINATED
// GET /repos/{repo}/pulls/{n}/files (mirror of gitlab/snapshot.go's
// mrChangedFiles) and decides whether the enumeration is PROVABLY COMPLETE
// (ADR-0020 §2, D-119).
//
// The completeness decision (changedFilesVerdict) rests on FOUR axes: the
// enumeration terminated on a SHORT page below the pagination ceiling (a full
// page at the ceiling proves nothing about the tail); the PR object REPORTS a
// changed_files count at all (an unreported count cannot cross-check — an
// absent count is information, not a zero, and is decoded as a pointer);
// the count EQUALS the enumerated entries in BOTH directions (fewer entries is
// the short-fall gap, more entries is the PR-moved-between-reads gap — the
// ADR-0020 §2 count cross-check's GitHub column, the PR GET already decoded
// it); and the PR's changed_files sits below the pull-files endpoint's
// 3000-file cap, above which the endpoint can never serve the full change set.
// A failing axis is declared incomplete with a SPECIFIC gap reason; the
// partial path list is still returned, because an `.assent/**` path that IS
// visible must still dominate to BLOCK (the fail-safe direction of D-119).
//
// A NON-200 on the files endpoint (INCLUDING 404) is a HARD ERROR, not a gap:
// the PR provably exists by this point (the PR GET succeeded), so a missing
// files resource is a forge anomaly, never evidence of an empty change set.
func (c *Client) mrChangedFiles(project, mr string) (changedFileSet, error) {
	repo, err := repoParts(project)
	if err != nil {
		return changedFileSet{}, err
	}
	seen := make(map[string]struct{})
	var paths []string
	entries := 0
	terminated := false

	for page := 1; page <= maxListPages; page++ {
		status, _, raw, err := c.do(http.MethodGet,
			fmt.Sprintf("/repos/%s/pulls/%s/files?per_page=%d&page=%d", repo, mr, listPerPage, page), nil, "")
		if err != nil {
			return changedFileSet{}, err
		}
		if status != http.StatusOK {
			return changedFileSet{}, fmt.Errorf("github: get PR files %s#%s page %d: unexpected status %d",
				project, mr, page, status)
		}
		var list []ghFileDiff
		if err := json.Unmarshal(raw, &list); err != nil {
			return changedFileSet{}, fmt.Errorf("github: decode PR files %s#%s page %d: %w", project, mr, page, err)
		}

		entries += len(list)
		for _, ch := range list {
			for _, p := range []string{ch.Filename, ch.PreviousFilename} {
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

		if len(list) < listPerPage {
			terminated = true
			break
		}
	}

	set := changedFileSet{paths: paths}
	set.complete, set.gap = changedFilesVerdict(entries, c.lastPRChangedFiles(), terminated)
	return set, nil
}

var _ forge.Snapshotter = (*Client)(nil)
