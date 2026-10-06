package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/render"
)

// write.go — the write half of the port (E10-S10): the forge.Forge write
// surface the shared internal/forge Reconcile engine drives. The adapter
// supplies PRIMITIVES only — the engine owns the ADR-0019 marker protocol,
// idempotence, duplicate repair and the fail-closed SHA-guard.
//
// The marker filter matches the AUTHENTICATED identity (REQ-E10-S10-02):
// markers authored by the token identity's own login are ours; a second app's
// or a contributor's marker-carrying artifact is invisible (this repo runs
// Renovate, so any second app causes marker collision, and anyone who can
// install an app gets marker spoofing — dossier C9). A malformed marker on an
// OWN artifact is skipped with a warning (RELI-06 precedent), never a
// reconciliation brick.

// markerRe extracts the JSON payload of the marker HTML comment from a comment
// body — the same wire grammar both adapters agree on (the marker bytes are
// owned by internal/render, D-094; each adapter owns its extraction). It is
// anchored on the exact sentinel so a stray HTML comment in a human body is
// never mistaken for a marker.
var markerRe = regexp.MustCompile(`<!--\s*` + regexp.QuoteMeta(render.MarkerSentinel) + `\s*(\{.*?\})\s*-->`)

// markerJSON is the wire shape of the ADR-0019 marker payload (the gitlab
// marker.go shape; four top-level concepts, schema-frozen).
type markerJSON struct {
	Slot struct {
		Project  string `json:"project"`
		MR       string `json:"mr"`
		Rule     string `json:"rule"`
		EntryRef string `json:"entryRef,omitempty"`
		Effect   string `json:"effect"`
	} `json:"slot"`
	Occurrence string `json:"occurrence"`
	Decision   string `json:"decision"`
	Artifact   struct {
		Kind          string `json:"kind"`
		SchemaVersion string `json:"schemaVersion"`
	} `json:"artifact"`
}

// markerOf extracts and decodes the ADR-0019 marker from a comment body (the
// gitlab parseMarker mirror): (marker, true, nil) for a well-formed marker,
// (_, false, nil) when the body carries no marker, and an error only when a
// marker sentinel is present but its payload is malformed JSON — the case the
// RELI-06 skip-with-warning channel exists for.
func markerOf(body string) (forge.Marker, bool, error) {
	sub := markerRe.FindStringSubmatch(body)
	if sub == nil {
		return forge.Marker{}, false, nil
	}
	var mj markerJSON
	if err := json.Unmarshal([]byte(sub[1]), &mj); err != nil {
		return forge.Marker{}, false, fmt.Errorf("github: parse marker payload: %w", err)
	}
	return forge.Marker{
		Slot: forge.Slot{
			Project:  mj.Slot.Project,
			MR:       mj.Slot.MR,
			Rule:     mj.Slot.Rule,
			EntryRef: mj.Slot.EntryRef,
			Effect:   mj.Slot.Effect,
		},
		Occurrence: mj.Occurrence,
		Decision:   mj.Decision,
		Artifact: forge.Artifact{
			Kind:          mj.Artifact.Kind,
			SchemaVersion: mj.Artifact.SchemaVersion,
		},
	}, true, nil
}

// markerSkipWarning is the operator-facing sentence recorded on the receipt
// when a bot-authored artifact's marker payload cannot be decoded (AUD-S12 /
// REL-06). It embeds the DECODER's error only — never the raw payload — so a
// corrupted marker cannot smuggle arbitrary text into the run summary.
func markerSkipWarning(kind, id string, err error) string {
	return fmt.Sprintf(
		"github: skipped bot %s %s — malformed marker payload (%v); reconcile continued and the artifact was left in place",
		kind, id, err)
}

// ghIssueComment is the subset of a PR review/issue comment the adapter reads.
// NodeID is the GraphQL node id the ResolveThread mutation needs (dossier §4:
// thread resolution is GraphQL-only); the adapter remembers it in nodeIDs.
type ghIssueComment struct {
	ID     int64  `json:"id"`
	NodeID string `json:"node_id"`
	Body   string `json:"body"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
}

// ghNoteID splits an adapter forge id ("comment/123", "issue-comment/123")
// into its kind prefix and numeric suffix. Callers downstream use the numeric
// suffix for the REST write routes.
func ghNoteID(id string) (kind, numeric string) {
	i := strings.LastIndex(id, "/")
	if i < 0 {
		return "", ""
	}
	return id[:i], id[i+1:]
}

// rememberNodeID records a review comment's GraphQL node id under its numeric
// REST id (the ResolveThread lookup).
func (c *Client) rememberNodeID(commentID int64, nodeID string) {
	if nodeID == "" {
		return
	}
	c.nodeMu.Lock()
	defer c.nodeMu.Unlock()
	if c.nodeIDs == nil {
		c.nodeIDs = map[int64]string{}
	}
	c.nodeIDs[commentID] = nodeID
}

// nodeIDFor returns the GraphQL node id remembered for a review comment, ""
// when this client has never seen it.
func (c *Client) nodeIDFor(commentID int64) string {
	c.nodeMu.Lock()
	defer c.nodeMu.Unlock()
	return c.nodeIDs[commentID]
}

// ListBotThreads returns the PR's review comments authored by the configured
// identity, filtered by AUTHOR IDENTITY (ADR-0019): a contributor comment
// carrying a well-formed marker is EXCLUDED — invisible to reconciliation.
// GitHub review threads are PR review comments: GET
// /repos/{repo}/pulls/{n}/comments.
//
// The listing loop is CAPPED at maxListPages and the cap is FAIL-CLOSED: a
// paginator that never returns a short page yields an error, not a silent
// partial — an incomplete thread list would read as "no thread yet" to
// reconcile, which duplicates findings. A 404/403 wraps forge.ErrUnauthorized
// (S00 Q4: never an empty list — permission failure is not absence).
//
// Thread.ID carries the REST id as "comment/<id>"; the comment's GraphQL
// node_id is remembered in nodeIDs, because resolution is GraphQL-only and
// addresses threads by node id (the port's ResolveThread signature keeps
// (project, mr, id)).
func (c *Client) ListBotThreads(project, mr string) ([]forge.Thread, error) {
	repo, err := repoParts(project)
	if err != nil {
		return nil, err
	}
	var out []forge.Thread
	for page := 1; ; page++ {
		if page > maxListPages {
			return nil, fmt.Errorf(
				"github: list PR review comments %s#%s: pagination cap of %d pages reached without a short page — refusing to reconcile against a partial thread list",
				project, mr, maxListPages)
		}
		status, _, raw, err := c.do(http.MethodGet,
			fmt.Sprintf("/repos/%s/pulls/%s/comments?per_page=%d&page=%d", repo, mr, listPerPage, page), nil, "")
		if err != nil {
			return nil, err
		}
		switch status {
		case http.StatusOK:
			var rows []ghIssueComment
			if err := json.Unmarshal(raw, &rows); err != nil {
				return nil, fmt.Errorf("github: decode review comments %s#%s: %w", repo, mr, err)
			}
			for _, row := range rows {
				c.rememberNodeID(row.ID, row.NodeID)
				// AUTHOR-IDENTITY filter (ADR-0019): only the bot's own comments
				// count. A contributor's well-formed marker is invisible here —
				// it is never even parsed (the spoof-resistance axis).
				if row.User.Login != c.botName {
					continue
				}
				marker, ok, err := markerOf(row.Body)
				if err != nil {
					// AUD-S12 / REL-06: SKIP-WITH-WARNING. One corrupted marker
					// never bricks reconcile; the artifact is left in place and
					// the anomaly rides the receipt's warnings.
					c.warn(markerSkipWarning("review comment", fmt.Sprintf("comment/%d", row.ID), err))
					continue
				}
				if !ok {
					// A bot comment without a marker is not a finding thread.
					continue
				}
				out = append(out, forge.Thread{
					// The REST id is the Thread.ID; the GraphQL node id rides
					// in nodeIDs for ResolveThread (dossier §4).
					ID:     fmt.Sprintf("comment/%d", row.ID),
					Marker: marker,
					Author: row.User.Login,
					// GitHub's REST listing carries no resolution state
					// (resolution is GraphQL-only, dossier §4); an open
					// reading is the conservative one.
					Resolved: false,
				})
			}
			if len(rows) < listPerPage {
				return out, nil
			}
		case http.StatusNotFound, http.StatusForbidden:
			// Per S00 Q4: a permission-denied listing must NOT render as an
			// empty list — fail closed (never absence).
			return nil, fmt.Errorf("github: %w: list PR review comments %s#%s (status %d)", forge.ErrUnauthorized, repo, mr, status)
		default:
			return nil, fmt.Errorf("github: list PR review comments %s#%s: unexpected status %d", repo, mr, status)
		}
	}
}

// ListBotNotes returns bot-authored issue comments (the summary-comment slot),
// filtered by AUTHOR IDENTITY like ListBotThreads (ADR-0019). The loop is
// CAPPED at maxListPages, FAIL-CLOSED for the same reason: UpsertComment reads
// this list to decide edit-in-place vs. create, so a silent partial would post
// a duplicate summary.
func (c *Client) ListBotNotes(project, mr string) ([]forge.Note, error) {
	repo, err := repoParts(project)
	if err != nil {
		return nil, err
	}
	var notes []forge.Note
	for page := 1; ; page++ {
		if page > maxListPages {
			return nil, fmt.Errorf(
				"github: list issue comments %s#%s: pagination cap of %d pages reached without a short page — refusing to reconcile against a partial note list",
				project, mr, maxListPages)
		}
		status, _, raw, err := c.do(http.MethodGet, fmt.Sprintf("/repos/%s/issues/%s/comments?per_page=%d&page=%d", repo, mr, listPerPage, page), nil, "")
		if err != nil {
			return nil, err
		}
		switch status {
		case http.StatusOK:
			var rows []ghIssueComment
			if err := json.Unmarshal(raw, &rows); err != nil {
				return nil, fmt.Errorf("github: decode issue comments %s#%s: %w", repo, mr, err)
			}
			if len(rows) == 0 {
				return notes, nil
			}
			for _, row := range rows {
				// AUTHOR-IDENTITY filter (ADR-0019): only the bot's own comments
				// count; a contributor's well-formed marker is invisible.
				if row.User.Login != c.botName {
					continue
				}
				marker, ok, err := markerOf(row.Body)
				if err != nil {
					// AUD-S12 / REL-06: SKIP-WITH-WARNING, as in
					// ListBotThreads. A corrupted summary note is invisible to
					// UpsertComment, which posts one healthy replacement; the
					// next run edits THAT in place (write minimisation — the
					// corrupt note is never auto-deleted).
					c.warn(markerSkipWarning("issue comment", fmt.Sprintf("issue-comment/%d", row.ID), err))
					continue
				}
				if !ok {
					continue
				}
				notes = append(notes, forge.Note{
					ID:     fmt.Sprintf("issue-comment/%d", row.ID),
					Marker: marker,
					Author: row.User.Login,
					Body:   row.Body,
				})
			}
			if len(rows) < listPerPage {
				return notes, nil
			}
		case http.StatusNotFound, http.StatusForbidden:
			// Per S00 Q4: a permission-denied listing must NOT render as an
			// empty list — fail closed (an empty list would post a duplicate
			// summary).
			return nil, fmt.Errorf("github: %w: list issue comments %s#%s (status %d)", forge.ErrUnauthorized, repo, mr, status)
		default:
			return nil, fmt.Errorf("github: list issue comments %s#%s: unexpected status %d", repo, mr, status)
		}
	}
}

// UpsertComment creates OR edits-in-place exactly one summary-comment note per
// PR (P3-E5 step 3): an existing bot summary note is edited in place; a second
// one is never posted. The edit goes through PATCH
// /repos/{repo}/issues/comments/{id} (a repo-level id, NOT issue-scoped).
func (c *Client) UpsertComment(project, mr string, marker forge.Marker, body string) (forge.Note, error) {
	if marker.Artifact.Kind != "summary-comment" {
		return forge.Note{}, forge.ErrInvalidSummaryMarker
	}
	if _, err := render.Envelope(marker, body); err != nil {
		return forge.Note{}, err
	}
	existing, err := c.ListBotNotes(project, mr)
	if err != nil {
		return forge.Note{}, err
	}
	for _, n := range existing {
		if n.Marker.Artifact.Kind == marker.Artifact.Kind && marker.Artifact.Kind == "summary-comment" {
			return c.editNote(project, mr, n.ID, marker, body)
		}
	}
	return c.createNote(project, mr, marker, body)
}

// createNote posts a new issue comment carrying the marker (POST
// /repos/{repo}/issues/{n}/comments → 201; writes are NEVER retried — the
// transport layer gives one attempt).
func (c *Client) createNote(project, mr string, marker forge.Marker, body string) (forge.Note, error) {
	repo, err := repoParts(project)
	if err != nil {
		return forge.Note{}, err
	}
	fullBody, err := render.Envelope(marker, body)
	if err != nil {
		return forge.Note{}, err
	}
	payload, err := json.Marshal(map[string]any{"body": fullBody})
	if err != nil {
		return forge.Note{}, errors.New("github: encode comment body")
	}
	status, _, raw, err := c.do(http.MethodPost, "/repos/"+repo+"/issues/"+mr+"/comments", strings.NewReader(string(payload)), "application/json")
	if err != nil {
		return forge.Note{}, err
	}
	if status != http.StatusCreated {
		return forge.Note{}, fmt.Errorf("github: create issue comment %s#%s: unexpected status %d", repo, mr, status)
	}
	var created ghIssueComment
	if err := json.Unmarshal(raw, &created); err != nil {
		return forge.Note{}, fmt.Errorf("github: decode created comment %s#%s: %w", repo, mr, err)
	}
	c.rememberNodeID(created.ID, created.NodeID)
	return forge.Note{
		ID:     fmt.Sprintf("issue-comment/%d", created.ID),
		Marker: marker,
		Author: c.botName,
		Body:   fullBody,
	}, nil
}

// editNote edits an existing issue comment in place (PATCH
// /repos/{repo}/issues/comments/{id} — repo-level id space).
func (c *Client) editNote(project, mr, id string, marker forge.Marker, body string) (forge.Note, error) {
	repo, err := repoParts(project)
	if err != nil {
		return forge.Note{}, err
	}
	fullBody, err := render.Envelope(marker, body)
	if err != nil {
		return forge.Note{}, err
	}
	_, numeric := ghNoteID(id)
	payload, err := json.Marshal(map[string]any{"body": fullBody})
	if err != nil {
		return forge.Note{}, errors.New("github: encode comment body")
	}
	status, _, _, err := c.do(http.MethodPatch, "/repos/"+repo+"/issues/comments/"+numeric, strings.NewReader(string(payload)), "application/json")
	if err != nil {
		return forge.Note{}, err
	}
	if status != http.StatusOK {
		return forge.Note{}, fmt.Errorf("github: edit issue comment %s on %s#%s: unexpected status %d", id, repo, mr, status)
	}
	return forge.Note{
		ID:     fmt.Sprintf("issue-comment/%s", numeric),
		Marker: marker,
		Author: c.botName,
		Body:   fullBody,
	}, nil
}

// CreateThread posts a new PR review comment whose body is the marker envelope
// followed by the human body, and returns the created forge.Thread. POST
// /repos/{repo}/pulls/{n}/comments → 201, body only — the PR comment body
// carries the marker envelope via render.Envelope; no commit anchor is needed
// in this slice (the ADR-0019 envelope makes the artifact correlatable without
// a side anchor).
func (c *Client) CreateThread(project, mr string, marker forge.Marker, body string) (forge.Thread, error) {
	repo, err := repoParts(project)
	if err != nil {
		return forge.Thread{}, err
	}
	fullBody, err := render.Envelope(marker, body)
	if err != nil {
		return forge.Thread{}, err
	}
	payload, err := json.Marshal(map[string]any{"body": fullBody})
	if err != nil {
		return forge.Thread{}, errors.New("github: encode thread body")
	}
	status, _, raw, err := c.do(http.MethodPost, fmt.Sprintf("/repos/%s/pulls/%s/comments", repo, mr), strings.NewReader(string(payload)), "application/json")
	if err != nil {
		return forge.Thread{}, err
	}
	if status != http.StatusCreated {
		return forge.Thread{}, fmt.Errorf("github: create thread %s#%s: unexpected status %d", repo, mr, status)
	}
	var created ghIssueComment
	if err := json.Unmarshal(raw, &created); err != nil {
		return forge.Thread{}, fmt.Errorf("github: decode created thread %s#%s: %w", repo, mr, err)
	}
	// Remember the created comment's GraphQL node id so ResolveThread can
	// address it without a re-listing.
	c.rememberNodeID(created.ID, created.NodeID)
	return forge.Thread{
		ID:     fmt.Sprintf("comment/%d", created.ID),
		Marker: marker,
		Author: c.botName,
	}, nil
}

// ResolveThread marks the bot review thread resolved in place (the
// duplicate-repair and supersede action). Thread resolution is GraphQL-only on
// GitHub (dossier §4): resolveReviewThread(input:{threadId}) — the REST API
// has no resolution verb, so the GraphQL node id (remembered by the listing
// and creation) is REQUIRED; an unknown id fails closed rather than guessing.
// The response must report isResolved:true — a forge that answers the mutation
// but not the resolution is an error, never success. Resolving is idempotent:
// an "already resolved" GraphQL error is success; an unparseable/unknown id is
// an error.
func (c *Client) ResolveThread(project, mr, id string) error {
	_, numeric := ghNoteID(id)
	num, err := strconv.ParseInt(numeric, 10, 64)
	if err != nil {
		return fmt.Errorf("github: thread id %q carries no parseable numeric comment id — refusing to resolve an unaddressable thread", id)
	}
	nodeID := c.nodeIDFor(num)
	if nodeID == "" {
		return fmt.Errorf("github: thread %s is unknown to this client — its GraphQL node id was never observed; list bot threads (or create the thread) before resolving", id)
	}
	data, err := c.gqlDo(c.ctx, gqlResolveThread, map[string]any{"threadId": nodeID})
	if err != nil {
		// Idempotence: resolving an already-resolved thread is a no-op — the
		// forge's "already resolved" rejection IS success for this verb.
		if strings.Contains(strings.ToLower(err.Error()), "already resolved") {
			return nil
		}
		return fmt.Errorf("github: resolve thread %s: %w", id, err)
	}
	var payload struct {
		ResolveReviewThread struct {
			Thread struct {
				ID         string `json:"id"`
				IsResolved bool   `json:"isResolved"`
			} `json:"thread"`
		} `json:"resolveReviewThread"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("github: decode resolveReviewThread: %w", err)
	}
	if !payload.ResolveReviewThread.Thread.IsResolved {
		return fmt.Errorf("github: thread %s did not report isResolved:true after resolution — fail-closed", id)
	}
	return nil
}

// Approve records an approval via POST /repos/{repo}/pulls/{mr}/reviews with
// {"event":"APPROVE"} and returns the review's forge id ("review/<id>") — the
// stable non-empty target the receipt records. A 401/403 is the
// ErrUnauthorized sentinel (an author/bot cannot self-approve, or the token
// lacks the pull-request-write scope), never a transport failure.
func (c *Client) Approve(project, mr string) (string, error) {
	repo, err := repoParts(project)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]any{"event": "APPROVE"})
	if err != nil {
		return "", errors.New("github: encode approve body")
	}
	status, _, raw, err := c.do(http.MethodPost, fmt.Sprintf("/repos/%s/pulls/%s/reviews", repo, mr), strings.NewReader(string(payload)), "application/json")
	if err != nil {
		return "", err
	}
	switch status {
	case http.StatusOK, http.StatusCreated:
		var rev ghReview
		if err := json.Unmarshal(raw, &rev); err != nil {
			return "", fmt.Errorf("github: decode approval review %s#%s: %w", repo, mr, err)
		}
		if rev.ID == 0 {
			return "", fmt.Errorf("github: approve %s#%s: forge returned no review id", repo, mr)
		}
		return fmt.Sprintf("review/%d", rev.ID), nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", fmt.Errorf("github: %w: approve %s#%s (status %d)", forge.ErrUnauthorized, repo, mr, status)
	default:
		return "", fmt.Errorf("github: approve %s#%s: unexpected status %d", repo, mr, status)
	}
}

// CurrentHeads returns the forge's CURRENT source SHA, target tip and
// merge-result digest for the PR. Both reads go FRESH — deliberately NOT
// through the mrPinned cache: the CAS pre-check must see the forge's current
// heads so a drifted source/target/digest fails closed with ZERO writes (the
// gitlab.CurrentHeads mirror, which reads GetMR fresh for the same reason).
//
// The digest axis is the adapter-owned merge-result scheme (snapshot.go):
// the refs/pull/{n}/merge commit's SHA when the forge mints it, "" otherwise —
// and completeForMerge then refuses to arm (fail-closed for a non-mergeable
// PR, the correct v1 behaviour).
func (c *Client) CurrentHeads(project, mr string) (source, target, digest string, err error) {
	info, err := c.GetMR(project, mr)
	if err != nil {
		return "", "", "", err
	}
	digest, err = c.mergeResultDigest(project, mr)
	if err != nil {
		return "", "", "", err
	}
	return info.SourceSHA, info.TargetSHA, digest, nil
}

// MergeCAS performs the SHA-pinned compare-and-swap merge, fail-closed on the
// axes GitHub honours (the gitlab.MergeCAS mirror):
//
//  1. Re-read the CURRENT heads. If the source head, the target tip OR the
//     merge-result digest has moved from the pinned values, return
//     forge.ErrSHAMoved with NO merge — the approval must never merge an
//     unevaluated state (ADR-0017 §1, ADR-0015 §2; the three-pin contract is
//     checked BEFORE any write, and again atomically by the PUT below).
//  2. PUT /repos/{repo}/pulls/{n}/merge with {"sha": pinnedSource}: GitHub's
//     `sha` body parameter IS the atomic CAS guard on the source head. A
//     moved source is refused with 409 → ErrSHAMoved, no merge. 405 (not
//     mergeable / protected branch) is the forge refusing the merge outright —
//     also ErrSHAMoved, with a distinct message (a refusal to merge an
//     unevaluated result, not a generic failure). A 200 is the merge; any
//     other non-200 is a generic error.
//
// Writes are never retried (the transport layer gives one attempt), so the
// compare-and-swap is replayed never.
func (c *Client) MergeCAS(project, mr string, m forge.DesiredMerge) (string, error) {
	repo, err := repoParts(project)
	if err != nil {
		return "", err
	}
	// Three-axis guard: re-read heads FRESH and reject a moved source, target
	// or merge result BEFORE the PUT (the pre-write SHA-guard, ADR-0015 §2).
	curSource, curTarget, curDigest, err := c.CurrentHeads(project, mr)
	if curTarget != m.TargetSha {
		return "", fmt.Errorf("%w: target tip moved (pinned %s, now %s) — refusing to merge an unevaluated target",
			forge.ErrSHAMoved, m.TargetSha, curTarget)
	}
	if curSource != m.SourceSha {
		return "", fmt.Errorf("%w: source head moved (pinned %s, now %s)",
			forge.ErrSHAMoved, m.SourceSha, curSource)
	}
	if curDigest != m.MergeResultDigest {
		return "", fmt.Errorf("%w: merge result moved (pinned %s, now %s) — the merge ref was re-minted or became unavailable",
			forge.ErrSHAMoved, m.MergeResultDigest, curDigest)
	}

	payload, err := json.Marshal(map[string]any{
		"sha":          m.SourceSha,
		"merge_method": "merge",
	})
	if err != nil {
		return "", errors.New("github: encode merge body")
	}
	status, _, raw, err := c.do(http.MethodPut,
		fmt.Sprintf("/repos/%s/pulls/%s/merge", repo, mr),
		strings.NewReader(string(payload)), "application/json")
	if err != nil {
		return "", err
	}
	switch status {
	case http.StatusOK:
		var mResp struct {
			MergeCommitSHA string `json:"merge_commit_sha"`
		}
		if err := json.Unmarshal(raw, &mResp); err != nil {
			return "", fmt.Errorf("github: decode merge response %s#%s: %w", repo, mr, err)
		}
		if mResp.MergeCommitSHA == "" {
			return "", fmt.Errorf("github: merge %s#%s succeeded without a merge_commit_sha — receipt would record nothing, fail-closed", repo, mr)
		}
		return "merge/" + mResp.MergeCommitSHA, nil
	case http.StatusConflict:
		// 409 = the sha pin did not match the current source head — the CAS
		// guard fired (dossier C10). No merge; fail closed.
		return "", fmt.Errorf("%w: merge sha pin rejected with 409 (source head moved)", forge.ErrSHAMoved)
	case http.StatusMethodNotAllowed:
		// 405 = the PR is not mergeable (protected branch, required checks
		// unmet) — the forge refused the compare-and-swap merge outright.
		return "", fmt.Errorf("%w: merge refused with 405 — pull request not mergeable (protected branch or required checks)", forge.ErrSHAMoved)
	default:
		return "", fmt.Errorf("github: merge %s#%s: unexpected status %d", repo, mr, status)
	}
}

var _ forge.Forge = (*Client)(nil)
