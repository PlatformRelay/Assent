// Package gitlab is the REAL GitLab REST v4 adapter for the P4-E1 walking
// skeleton (P4-E1-S10). It implements the forge.Forge port (ListBotThreads,
// CurrentHeads, CreateThread, ResolveThread, Approve, MergeCAS) plus the reads
// the `assent run` orchestration needs (GetMR, FileAtRef) against a live GitLab
// instance's `/api/v4` surface.
//
// The exact API call shapes it mirrors are the ones the product-surface spike
// exercised against a booted GitLab (hack/spikes/e2e/smoke.sh): discussions →
// resolve → approve → merge?sha=. This adapter is the side-effecting write edge
// (it lives OUTSIDE internal/core and may use net/http); it stays FAIL-CLOSED —
// a 409/406 on the SHA-pinned merge maps to forge.ErrSHAMoved (no merge), and a
// target tip that moved since evaluation is rejected before the merge PUT.
//
// AUTHOR-IDENTITY filter (ADR-0019): ListBotThreads returns a discussion as a
// bot thread ONLY when its first note's author username equals the configured
// botAuthor. A contributor (non-bot) note carrying a syntactically perfect
// marker is EXCLUDED — invisible to reconciliation — regardless of the marker's
// well-formedness. Filtering is by author identity, never by marker content.
//
// SECRET REDACTION: the PAT is sent only as the PRIVATE-TOKEN request header. It
// is never logged, never placed in a URL, an error message, or a thread body.
package gitlab

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/render"
)

// Client is a GitLab REST v4 adapter bound to one endpoint + PAT + bot identity.
// It implements forge.Forge. The zero value is not usable — construct with New.
type Client struct {
	endpoint   string           // e.g. https://gitlab.com (no trailing slash, no /api/v4).
	token      string           // PAT, sent as the PRIVATE-TOKEN header. NEVER logged.
	botAuthor  string           // username whose notes count as bot-authored (ADR-0019 filter).
	http       *http.Client     // injected transport; tests point it at an httptest.Server.
	observedAt func() time.Time // optional clock hook (tests); nil → time.Now().UTC in Resolve.

	// ctx is the parent context every request derives from (AUD-S11 / REL-04).
	// It lives on the struct rather than on each method because forge.Forge is
	// a frozen port with no context parameters (ADR-0011/ADR-0017); the CLI owns
	// one client per run, so one parent context per client is the right grain.
	ctx   context.Context
	retry RetryPolicy
}

// Retry defaults (AUD-S11 / REL-04). A run that would have decided correctly
// must not be lost to one transient 5xx, but the budget is BOUNDED: retries buy
// availability, never a different decision. Exhausting the budget is still the
// same hard error an un-retried failure produces today.
const (
	// defaultMaxAttempts is the TOTAL attempt budget for one idempotent read
	// (1 initial + 2 retries), not the number of retries.
	defaultMaxAttempts = 3

	// defaultBaseBackoff is the first backoff window. Doubling per attempt, the
	// worst-case added latency for one read is well under a second.
	defaultBaseBackoff = 200 * time.Millisecond

	// defaultMaxBackoff clamps the exponential so a longer budget can never
	// stall a CI job.
	defaultMaxBackoff = 2 * time.Second

	// defaultRequestTimeout is the per-request context deadline. A hung
	// connection consumes one attempt, not the whole run.
	defaultRequestTimeout = 30 * time.Second
)

// RetryPolicy is the bounded retry/backoff configuration for idempotent reads.
// Sleep and Jitter are injected seams: tests supply a recording sleeper and a
// fixed jitter source so the backoff SCHEDULE is asserted deterministically,
// with no wall-clock or math/rand dependence in any assertion.
type RetryPolicy struct {
	MaxAttempts    int
	BaseBackoff    time.Duration
	MaxBackoff     time.Duration
	RequestTimeout time.Duration
	Sleep          func(time.Duration)
	Jitter         func() float64 // returns [0,1)
}

// Option customises a Client at construction.
type Option func(*Client)

// WithRetry replaces the whole retry policy. Zero-valued fields keep their
// default, so a caller can tune one axis without restating the rest.
func WithRetry(p RetryPolicy) Option {
	return func(c *Client) {
		if p.MaxAttempts > 0 {
			c.retry.MaxAttempts = p.MaxAttempts
		}
		if p.BaseBackoff > 0 {
			c.retry.BaseBackoff = p.BaseBackoff
		}
		if p.MaxBackoff > 0 {
			c.retry.MaxBackoff = p.MaxBackoff
		}
		if p.RequestTimeout > 0 {
			c.retry.RequestTimeout = p.RequestTimeout
		}
		if p.Sleep != nil {
			c.retry.Sleep = p.Sleep
		}
		if p.Jitter != nil {
			c.retry.Jitter = p.Jitter
		}
	}
}

// WithSleeper injects the backoff sleeper.
func WithSleeper(sleep func(time.Duration)) Option {
	return func(c *Client) {
		if sleep != nil {
			c.retry.Sleep = sleep
		}
	}
}

// WithJitter injects the [0,1) jitter source used to spread the backoff window.
func WithJitter(j func() float64) Option {
	return func(c *Client) {
		if j != nil {
			c.retry.Jitter = j
		}
	}
}

// WithContext sets the parent context every request derives from. Cancelling it
// stops the client between attempts and fails the call closed.
func WithContext(ctx context.Context) Option {
	return func(c *Client) {
		if ctx != nil {
			c.ctx = ctx
		}
	}
}

// New builds a GitLab adapter. endpoint is the instance base URL
// (e.g. https://gitlab.com); token is a PAT sent as the PRIVATE-TOKEN header;
// botAuthor is the username whose discussion first-notes count as bot-authored
// for the ADR-0019 author-identity filter. A trailing slash on endpoint is
// trimmed so path joins are unambiguous.
//
// Without options the client ships the default bounded-retry policy (AUD-S11):
// idempotent GET/HEAD reads retry up to defaultMaxAttempts times on a transport
// error, a 429 or a 5xx, with jittered exponential backoff under a per-request
// deadline. Writes are NEVER auto-retried.
func New(endpoint, token, botAuthor string, opts ...Option) *Client {
	c := &Client{
		endpoint:  strings.TrimRight(endpoint, "/"),
		token:     token,
		botAuthor: botAuthor,
		http:      &http.Client{Timeout: 30 * time.Second},
		ctx:       context.Background(),
		retry: RetryPolicy{
			MaxAttempts:    defaultMaxAttempts,
			BaseBackoff:    defaultBaseBackoff,
			MaxBackoff:     defaultMaxBackoff,
			RequestTimeout: defaultRequestTimeout,
			Sleep:          time.Sleep,
			// #nosec G404 -- backoff jitter spreads retry storms; it is not a
			// security or decision input, and the decision path (internal/core)
			// remains free of randomness by construction.
			Jitter: rand.Float64,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ErrNotFound is the typed error FileAtRef returns for a 404 (file absent at the
// ref). The caller decides what a missing file means (e.g. an absent governed
// file is a fail-safe REVIEW, not a crash).
var ErrNotFound = errors.New("gitlab: resource not found (404)")

// ErrUnauthorized is the typed error returned on a 401/403 — e.g. an approval
// attempt with a token that GitLab forbids (an MR author may not approve their
// own MR; the caller supplies a different token). It is surfaced clearly so the
// caller does not mistake an authorization refusal for a transient error.
var ErrUnauthorized = errors.New("gitlab: unauthorized (401/403)")

// MRInfo is the merge-request metadata `assent run` pins its evaluation to. All
// SHAs are the exact values GitLab reports at read time; TargetSHA is the target
// BRANCH TIP (commit.id from the branches endpoint), NOT diff_refs.base_sha
// (which is the merge-base — a different commit).
type MRInfo struct {
	IID          string
	ProjectID    string
	SourceBranch string
	TargetBranch string
	SourceSHA    string // the MR's current source head (`sha`).
	TargetSHA    string // the target branch tip (commit.id).
	// ForkMR is true when source_project_id != project_id (fork workflow).
	ForkMR bool
}

// Bounds on what a single forge interaction may consume (AUD-S10, audit
// findings REL-03 / SEC-08). The GitLab instance sits across a network the
// runner does not control: a hostile or broken endpoint must not be able to
// exhaust the runner's memory with one response, nor spin a pagination loop
// forever. Both bounds are FAIL-CLOSED — see readBounded and the list loops.
const (
	// maxResponseBytes caps a single response body. 8 MiB is MB-order and
	// generously above legitimate traffic: a 100-item discussions page is
	// KB-order, and the largest read (a governed file at a ref) is a policy or
	// registry document, not a binary blob.
	maxResponseBytes = 8 << 20

	// listPerPage is the page size requested for the discussion/note/approval-rule
	// listings. It is also the short-page test: a page with FEWER entries is the
	// last one.
	listPerPage = 100

	// maxListPages caps those pagination loops. 100 pages × listPerPage = 10 000
	// artifacts on a single MR — orders of magnitude above anything
	// reconciliation legitimately produces (one summary note plus one thread per
	// open finding). Reaching it means the forge is not shortening its pages,
	// i.e. the listing cannot be proven complete.
	maxListPages = 100
)

// readBounded reads at most limit bytes from r and errors when the source has
// MORE than limit bytes to give. It reads limit+1 bytes precisely so a body of
// exactly limit stays legitimate traffic while limit+1 is refused.
//
// FAIL-CLOSED: the partially-read prefix is DISCARDED (nil is returned with the
// error). A truncated JSON array that happened to parse would silently shrink
// the bot-artifact list and make reconcile create duplicates; truncated bytes
// must therefore never reach a decoder.
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("response body exceeds the %d-byte limit — refusing to parse a truncated response", limit)
	}
	return raw, nil
}

// retryableMethod reports whether an HTTP method is safe to replay
// automatically (AUD-S11 / REL-04). ONLY the idempotent reads are: replaying a
// POST duplicates a thread, a summary note or an approval, and replaying the
// merge PUT re-runs a compare-and-swap the caller believed it had lost.
// Idempotence discipline for writes stays the caller's (ADR-0019).
func retryableMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// transient reports whether an attempt's outcome is worth another try: a
// transport-level failure (connection refused, reset, deadline), a 429, or any
// 5xx. Every deterministic 4xx — 401/403/404 included — is surfaced at once;
// retrying it would only burn the budget and delay the real error.
func transient(status int, err error) bool {
	if err != nil {
		return true
	}
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// backoff returns the wait before the attempt AFTER the given 1-based attempt
// number: an exponential window (base << (attempt-1)), clamped at MaxBackoff,
// then spread over its lower half by the injected jitter source. Jitter is what
// keeps many concurrent runners from re-hitting a recovering instance in
// lockstep; it is injected, so the schedule is deterministic under test.
func (c *Client) backoff(attempt int) time.Duration {
	window := c.retry.BaseBackoff << (attempt - 1)
	if window <= 0 || window > c.retry.MaxBackoff {
		window = c.retry.MaxBackoff
	}
	half := window / 2
	return half + time.Duration(c.retry.Jitter()*float64(half))
}

// do issues an authenticated request and returns the status code + body bytes.
// The PAT travels ONLY in the PRIVATE-TOKEN header (never the URL/body), so it
// cannot leak into a log line that prints the request path or an error.
//
// The response read is BOUNDED at maxResponseBytes (AUD-S10 / REL-03 / SEC-08):
// an over-limit body is an error, never a truncated parse.
//
// IDEMPOTENT reads are retried within a bounded, jittered budget (AUD-S11 /
// REL-04); writes get exactly one attempt. Exhausting the budget returns the
// LAST failure unchanged, so every caller's existing fail-closed handling of a
// non-200 (or of a transport error) applies verbatim — retries move
// availability only, never a decision.
func (c *Client) do(method, path string, body io.Reader, contentType string) (int, []byte, error) {
	attempts := 1
	if retryableMethod(method) {
		attempts = c.retry.MaxAttempts
	}
	for attempt := 1; ; attempt++ {
		// The parent deadline is checked BEFORE every attempt, so a context that
		// expired during a backoff fails closed instead of issuing a doomed
		// request.
		if err := c.ctx.Err(); err != nil {
			return 0, nil, fmt.Errorf("gitlab: %s %s: %w", method, path, err)
		}
		status, raw, err := c.doOnce(method, path, body, contentType)
		if attempt >= attempts || !transient(status, err) {
			return status, raw, err
		}
		c.retry.Sleep(c.backoff(attempt))
	}
}

// doOnce performs exactly one attempt under a per-request context deadline.
func (c *Client) doOnce(method, path string, body io.Reader, contentType string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.retry.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return 0, nil, fmt.Errorf("gitlab: build request %s %s: %w", method, path, err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("gitlab: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := readBounded(resp.Body, maxResponseBytes)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("gitlab: read body %s %s: %w", method, path, err)
	}
	return resp.StatusCode, raw, nil
}

// GetMR reads the MR metadata plus the target branch tip. It performs two GETs:
//   - GET /api/v4/projects/{project}/merge_requests/{mr} for source sha +
//     source/target branch names;
//   - GET /api/v4/projects/{project}/repository/branches/{target_branch} for the
//     target-branch tip (commit.id) — the value the SHA-guard pins the merge
//     target to (NOT diff_refs.base_sha, which is the merge-base).
func (c *Client) GetMR(project, mr string) (MRInfo, error) {
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s", url.PathEscape(project), url.PathEscape(mr))
	status, raw, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return MRInfo{}, err
	}
	if status != http.StatusOK {
		return MRInfo{}, fmt.Errorf("gitlab: get MR %s!%s: unexpected status %d", project, mr, status)
	}
	var mrResp struct {
		IID          int    `json:"iid"`
		ProjectID    int    `json:"project_id"`
		SHA          string `json:"sha"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
	}
	if err := json.Unmarshal(raw, &mrResp); err != nil {
		return MRInfo{}, fmt.Errorf("gitlab: decode MR %s!%s: %w", project, mr, err)
	}

	targetSHA, err := c.branchTip(project, mrResp.TargetBranch)
	if err != nil {
		return MRInfo{}, err
	}

	return MRInfo{
		IID:          strconv.Itoa(mrResp.IID),
		ProjectID:    strconv.Itoa(mrResp.ProjectID),
		SourceBranch: mrResp.SourceBranch,
		TargetBranch: mrResp.TargetBranch,
		SourceSHA:    mrResp.SHA,
		TargetSHA:    targetSHA,
	}, nil
}

// branchTip returns the tip commit id of a branch via
// GET /api/v4/projects/{project}/repository/branches/{branch}.
func (c *Client) branchTip(project, branch string) (string, error) {
	path := fmt.Sprintf("/api/v4/projects/%s/repository/branches/%s",
		url.PathEscape(project), url.PathEscape(branch))
	status, raw, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("gitlab: get branch %s/%s: unexpected status %d", project, branch, status)
	}
	var br struct {
		Commit struct {
			ID string `json:"id"`
		} `json:"commit"`
	}
	if err := json.Unmarshal(raw, &br); err != nil {
		return "", fmt.Errorf("gitlab: decode branch %s/%s: %w", project, branch, err)
	}
	return br.Commit.ID, nil
}

// FileAtRef returns the raw bytes of a file at a git ref via
// GET /api/v4/projects/{project}/repository/files/{urlencoded path}/raw?ref={ref}.
// A 404 maps to ErrNotFound (the file is absent at that ref — the caller
// decides). The path segment is percent-encoded because a governed file path
// contains slashes GitLab requires URL-encoded.
func (c *Client) FileAtRef(project, path, ref string) ([]byte, error) {
	encPath := url.PathEscape(path)
	reqPath := fmt.Sprintf("/api/v4/projects/%s/repository/files/%s/raw?ref=%s",
		url.PathEscape(project), encPath, url.QueryEscape(ref))
	status, raw, err := c.do(http.MethodGet, reqPath, nil, "")
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
		return raw, nil
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w: file %q at ref %q", ErrNotFound, path, ref)
	default:
		return nil, fmt.Errorf("gitlab: get file %q at ref %q: unexpected status %d", path, ref, status)
	}
}

// discussion is the subset of a GitLab discussion the adapter reads.
type discussion struct {
	ID    string `json:"id"`
	Notes []struct {
		Body     string `json:"body"`
		Resolved bool   `json:"resolved"`
		Author   struct {
			Username string `json:"username"`
		} `json:"author"`
	} `json:"notes"`
}

// ListBotThreads returns the bot-authored discussions on the MR as
// forge.Thread, filtered by AUTHOR IDENTITY (ADR-0019): a discussion is a bot
// thread iff its FIRST note's author username equals the configured botAuthor.
// A contributor note carrying a well-formed marker is EXCLUDED. It paginates the
// discussions endpoint until the last page. A discussion whose first note has no
// marker is skipped (not a finding thread in this slice).
//
// The loop is CAPPED at maxListPages (AUD-S10 / REL-03) and the cap is
// FAIL-CLOSED: a paginator that never returns a short page yields an error, not
// a silent partial. An incomplete bot-thread list is the dangerous outcome —
// reconcile would read it as "that finding has no thread yet" and duplicate it.
func (c *Client) ListBotThreads(project, mr string) ([]forge.Thread, error) {
	var out []forge.Thread
	for page := 1; ; page++ {
		if page > maxListPages {
			return nil, fmt.Errorf(
				"gitlab: list discussions %s!%s: pagination cap of %d pages reached without a short page — refusing to reconcile against a partial thread list",
				project, mr, maxListPages)
		}
		path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/discussions?per_page=%d&page=%d",
			url.PathEscape(project), url.PathEscape(mr), listPerPage, page)
		status, raw, err := c.do(http.MethodGet, path, nil, "")
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("gitlab: list discussions %s!%s: unexpected status %d", project, mr, status)
		}
		var discs []discussion
		if err := json.Unmarshal(raw, &discs); err != nil {
			return nil, fmt.Errorf("gitlab: decode discussions %s!%s: %w", project, mr, err)
		}
		if len(discs) == 0 {
			break
		}
		for _, d := range discs {
			if len(d.Notes) == 0 {
				continue
			}
			first := d.Notes[0]
			// AUTHOR-IDENTITY filter (ADR-0019): only the bot's own first note
			// counts. A contributor's well-formed marker is invisible here.
			if first.Author.Username != c.botAuthor {
				continue
			}
			marker, ok, err := parseMarker(first.Body)
			if err != nil {
				return nil, err
			}
			if !ok {
				// A bot note without a marker is not a finding thread in this slice.
				continue
			}
			out = append(out, forge.Thread{
				ID:       d.ID,
				Marker:   marker,
				Author:   first.Author.Username,
				Resolved: first.Resolved,
			})
		}
		// A short page (< per_page) is the last page.
		if len(discs) < listPerPage {
			break
		}
	}
	return out, nil
}

// mrNote is the subset of a GitLab MR note the adapter reads.
type mrNote struct {
	ID     int    `json:"id"`
	Body   string `json:"body"`
	Author struct {
		Username string `json:"username"`
	} `json:"author"`
}

// ListBotNotes returns bot-authored MR notes filtered by AUTHOR IDENTITY
// (ADR-0019): a note counts iff its author username equals the configured
// botAuthor. It paginates the notes endpoint until the last page.
//
// The loop is CAPPED at maxListPages (AUD-S10 / REL-03), FAIL-CLOSED for the
// same reason as ListBotThreads: UpsertComment reads this list to decide
// edit-in-place vs. create, so a silent partial would post a duplicate summary.
func (c *Client) ListBotNotes(project, mr string) ([]forge.Note, error) {
	var out []forge.Note
	for page := 1; ; page++ {
		if page > maxListPages {
			return nil, fmt.Errorf(
				"gitlab: list notes %s!%s: pagination cap of %d pages reached without a short page — refusing to reconcile against a partial note list",
				project, mr, maxListPages)
		}
		path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/notes?per_page=%d&page=%d",
			url.PathEscape(project), url.PathEscape(mr), listPerPage, page)
		status, raw, err := c.do(http.MethodGet, path, nil, "")
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("gitlab: list notes %s!%s: unexpected status %d", project, mr, status)
		}
		var notes []mrNote
		if err := json.Unmarshal(raw, &notes); err != nil {
			return nil, fmt.Errorf("gitlab: decode notes %s!%s: %w", project, mr, err)
		}
		if len(notes) == 0 {
			break
		}
		for _, n := range notes {
			if n.Author.Username != c.botAuthor {
				continue
			}
			marker, ok, err := parseMarker(n.Body)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			out = append(out, forge.Note{
				ID:     fmt.Sprintf("note/%d", n.ID),
				Marker: marker,
				Author: n.Author.Username,
				Body:   n.Body,
			})
		}
		if len(notes) < listPerPage {
			break
		}
	}
	return out, nil
}

// UpsertComment creates or edits-in-place exactly one summary-comment note on
// the MR. When a bot note with artifact.kind summary-comment already exists, it
// is updated via PUT; otherwise a new note is POSTed.
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
			return c.updateNote(project, mr, n.ID, marker, body)
		}
	}
	return c.createNote(project, mr, marker, body)
}

func (c *Client) createNote(project, mr string, marker forge.Marker, body string) (forge.Note, error) {
	fullBody, err := render.Envelope(marker, body)
	if err != nil {
		return forge.Note{}, err
	}
	form := url.Values{}
	form.Set("body", fullBody)
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/notes",
		url.PathEscape(project), url.PathEscape(mr))
	status, raw, err := c.do(http.MethodPost, path, strings.NewReader(form.Encode()),
		"application/x-www-form-urlencoded")
	if err != nil {
		return forge.Note{}, err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return forge.Note{}, fmt.Errorf("gitlab: create note %s!%s: unexpected status %d", project, mr, status)
	}
	var n mrNote
	if err := json.Unmarshal(raw, &n); err != nil {
		return forge.Note{}, fmt.Errorf("gitlab: decode created note %s!%s: %w", project, mr, err)
	}
	return forge.Note{
		ID:     fmt.Sprintf("note/%d", n.ID),
		Marker: marker,
		Author: c.botAuthor,
		Body:   fullBody,
	}, nil
}

func (c *Client) updateNote(project, mr, id string, marker forge.Marker, body string) (forge.Note, error) {
	fullBody, err := render.Envelope(marker, body)
	if err != nil {
		return forge.Note{}, err
	}
	form := url.Values{}
	form.Set("body", fullBody)
	noteID := strings.TrimPrefix(id, "note/")
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/notes/%s",
		url.PathEscape(project), url.PathEscape(mr), url.PathEscape(noteID))
	status, _, err := c.do(http.MethodPut, path, strings.NewReader(form.Encode()),
		"application/x-www-form-urlencoded")
	if err != nil {
		return forge.Note{}, err
	}
	if status != http.StatusOK {
		return forge.Note{}, fmt.Errorf("gitlab: update note %s on %s!%s: unexpected status %d", id, project, mr, status)
	}
	return forge.Note{
		ID:     id,
		Marker: marker,
		Author: c.botAuthor,
		Body:   fullBody,
	}, nil
}

// CreateThread posts a new resolvable discussion whose body is the marker
// (hidden HTML comment) followed by the human body, and returns the created
// forge.Thread. POST /api/v4/projects/{project}/merge_requests/{mr}/discussions
// with a form-encoded `body`.
func (c *Client) CreateThread(project, mr string, marker forge.Marker, body string) (forge.Thread, error) {
	fullBody, err := render.Envelope(marker, body)
	if err != nil {
		return forge.Thread{}, err
	}
	form := url.Values{}
	form.Set("body", fullBody)
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/discussions",
		url.PathEscape(project), url.PathEscape(mr))
	status, raw, err := c.do(http.MethodPost, path, strings.NewReader(form.Encode()),
		"application/x-www-form-urlencoded")
	if err != nil {
		return forge.Thread{}, err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return forge.Thread{}, fmt.Errorf("gitlab: create thread %s!%s: unexpected status %d", project, mr, status)
	}
	var d struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return forge.Thread{}, fmt.Errorf("gitlab: decode created thread %s!%s: %w", project, mr, err)
	}
	return forge.Thread{ID: d.ID, Marker: marker, Author: c.botAuthor}, nil
}

// ResolveThread resolves a discussion in place via
// PUT /api/v4/projects/{project}/merge_requests/{mr}/discussions/{id}?resolved=true.
// It is idempotent: a 200 (resolved, or already-resolved) is success.
func (c *Client) ResolveThread(project, mr, id string) error {
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/discussions/%s?resolved=true",
		url.PathEscape(project), url.PathEscape(mr), url.PathEscape(id))
	status, _, err := c.do(http.MethodPut, path, nil, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("gitlab: resolve thread %s on %s!%s: unexpected status %d", id, project, mr, status)
	}
	return nil
}

// Approve records an approval via
// POST /api/v4/projects/{project}/merge_requests/{mr}/approve and returns an
// approval id. GitLab forbids the MR author approving their own MR — the CALLER
// supplies a different token; this adapter just calls with whatever token it
// holds and surfaces a 401/403 as ErrUnauthorized. The returned id is the MR IID
// (GitLab's approve response carries no distinct approval id), so the receipt
// records a stable, non-empty approval target.
func (c *Client) Approve(project, mr string) (string, error) {
	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/approve",
		url.PathEscape(project), url.PathEscape(mr))
	status, _, err := c.do(http.MethodPost, path, nil, "")
	if err != nil {
		return "", err
	}
	switch status {
	case http.StatusCreated, http.StatusOK:
		return fmt.Sprintf("approval/%s!%s", project, mr), nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", fmt.Errorf("%w: approve %s!%s (status %d — author may not self-approve; supply a different token)",
			ErrUnauthorized, project, mr, status)
	default:
		return "", fmt.Errorf("gitlab: approve %s!%s: unexpected status %d", project, mr, status)
	}
}

// CurrentHeads reads the MR's current source SHA, target-branch tip, and a
// SYNTHESISED merge-result digest.
//
// GitLab (plain merge, no merge trains) exposes NO real merge-result digest, so
// there is nothing to read for that axis. The frozen forge.Reconcile CAS path
// nonetheless REQUIRES all three pins to be present and consistent
// (completeForMerge + the step-4 CurrentHeads pre-check compares the returned
// digest against DesiredMerge.MergeResultDigest). Returning "" for the digest
// would make the APPROVE path unreachable through Reconcile (either
// ErrIncompletePreconditions or a spurious ErrSHAMoved). So the adapter
// synthesises a NON-EMPTY digest deterministically from the source+target SHAs:
// it tracks head movement (if either head moves the digest moves too), which is
// exactly the belt-and-suspenders the three-pin contract asks for. The REAL
// merge protection is still the ?sha= PUT + target re-read in MergeCAS. The
// honest "gitlab has no merge-result digest" audit fact is recorded SEPARATELY
// in the DecisionRecord's capabilityGap by cmd/assent — never here.
func (c *Client) CurrentHeads(project, mr string) (source, target, digest string, err error) {
	info, err := c.GetMR(project, mr)
	if err != nil {
		return "", "", "", err
	}
	return info.SourceSHA, info.TargetSHA, SyntheticDigest(info.SourceSHA, info.TargetSHA), nil
}

// SyntheticDigest derives the non-empty merge-result digest the CAS pins carry
// from the source+target SHAs. It is deterministic and moves iff either head
// moves, so it is a genuine (belt-and-suspenders) drift signal on the digest
// axis — not a constant. It is NOT a real GitLab merge-result digest (GitLab
// exposes none); the DecisionRecord records that capability gap honestly.
func SyntheticDigest(source, target string) string {
	sum := sha256.Sum256([]byte(source + "\n" + target))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// MergeCAS performs the SHA-pinned compare-and-swap merge, fail-closed on both
// the source and target axes GitLab can honour:
//
//  1. Re-read the current heads. If the TARGET tip has moved from the pinned
//     target, return forge.ErrSHAMoved with NO merge — GitLab's ?sha= guards
//     only the SOURCE head, so the adapter guards the target itself rather than
//     merge an unevaluated target (ADR-0017 §1, keeping the three-pin contract as
//     strong as GitLab allows). A source drift is also caught here early, and
//     re-caught atomically by ?sha= below.
//  2. PUT .../merge?sha={pinnedSource}. GitLab's ?sha= is a compare-and-swap on
//     the SOURCE head: a moved source returns 409 (or 406) → forge.ErrSHAMoved,
//     no merge. A 200 is the merge; any other non-200 is a generic error.
func (c *Client) MergeCAS(project, mr string, m forge.DesiredMerge) (string, error) {
	// Target-axis guard: re-read heads and reject a moved target before the PUT.
	curSource, curTarget, _, err := c.CurrentHeads(project, mr)
	if err != nil {
		return "", err
	}
	if curTarget != m.TargetSha {
		return "", fmt.Errorf("%w: target tip moved (pinned %s, now %s) — refusing to merge an unevaluated target",
			forge.ErrSHAMoved, m.TargetSha, curTarget)
	}
	if curSource != m.SourceSha {
		return "", fmt.Errorf("%w: source head moved (pinned %s, now %s)",
			forge.ErrSHAMoved, m.SourceSha, curSource)
	}

	path := fmt.Sprintf("/api/v4/projects/%s/merge_requests/%s/merge?sha=%s",
		url.PathEscape(project), url.PathEscape(mr), url.QueryEscape(m.SourceSha))
	status, raw, err := c.do(http.MethodPut, path, nil, "")
	if err != nil {
		return "", err
	}
	switch status {
	case http.StatusOK:
		var mResp struct {
			ID     int    `json:"id"`
			SHA    string `json:"sha"`
			IID    int    `json:"iid"`
			State  string `json:"state"`
			MergeC string `json:"merge_commit_sha"`
		}
		_ = json.Unmarshal(raw, &mResp)
		if mResp.MergeC != "" {
			return "merge/" + mResp.MergeC, nil
		}
		return fmt.Sprintf("merge/%s!%s", project, mr), nil
	case http.StatusConflict, http.StatusNotAcceptable:
		// 409/406 = the SHA guard fired (source head moved) — fail closed, no merge.
		return "", fmt.Errorf("%w: merge?sha= rejected with status %d (source head moved)", forge.ErrSHAMoved, status)
	default:
		return "", fmt.Errorf("gitlab: merge %s!%s: unexpected status %d", project, mr, status)
	}
}

// static assertion that *Client implements the forge.Forge port.
var _ forge.Forge = (*Client)(nil)
