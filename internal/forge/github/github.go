// Package github is the GitHub forge adapter (E10-S06). It implements
// forge.RunPort over GitHub's REST v3 API and — for thread resolution only —
// the GraphQL v4 API, under PAT and GitHub App installation credentials.
//
// Design inputs, in force: ADR-0021 (the seam this adapter plugs into), S00's
// addressing model (docs/planning/github-addressing-model.md — MR-relative
// governed addressing, the status→sentinel mapping, the capability predicates),
// and the GitHub dossier (docs/planning/forge-dossier-github.md). The package
// imports no cmd/assent symbol. Bounded reads, pagination caps,
// idempotent-GET-only retry, context deadlines and writes-never-retried are
// PORT requirements (E10-S05) this adapter satisfies the same way the GitLab
// adapter does.
//
// THE v1 POSTURE, stated up front and fail-closed: the honest capability report
// marks `protected-pipeline-source` and `eligible-approval-evidence` UNKNOWN
// (OQ-33/OQ-34), and unknown never arms (ADR-0021 §3) — so v1 GitHub runs
// decide, comment and resolve threads, but NEVER arm auto-merge. That is the
// product limitation E10-S12 pins, not a defect to paper over.
//
// SECRET HYGIENE (REQ-E10-S06-03): the token is never logged and never embedded
// in an error; every request error names the method, the path and the status,
// and nothing else.
package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
)

// Client is the GitHub adapter: one concrete type satisfying forge.RunPort.
// Thread resolution uses the GraphQL transport (dossier §4: resolution is
// GraphQL-only); everything else is REST. The port never names a transport, so
// the split is adapter-internal freedom (S00 Q2 judgment call (c)).
type Client struct {
	endpoint string // REST base URL, e.g. https://api.github.com (no trailing slash).
	token    string // credential sent as the Bearer header. NEVER logged.
	botName  string // the login whose artifacts count as bot-authored (ADR-0019 filter).

	// appAuth is non-nil when the credentials are a GitHub App installation:
	// requests carry the refreshed installation token (auth.go), not the JWT.
	appAuth *appAuth

	http  *http.Client
	ctx   context.Context // parent context every request derives from (AUD-S11 / REL-04)
	retry RetryPolicy

	// scopeOK caches the content-scope permission probe per (repo, ref)
	// (S00 Q4: a bare 404 is not evidence of absence — the probe must exercise
	// the SAME permission as the read it licenses; one request per (repo, ref)).
	scopeMu sync.Mutex
	scopeOK map[string]bool

	// mrPinnedKey/mrPinnedInfo cache the LAST PR read (E10-S07), the GitHub
	// mirror of the GitLab adapter's pin: the MR-relative accessors read at the
	// SHAs this cache carries so judged bytes and record pins share one read
	// chain. A fresh (project, mr) evicts it.
	mrPinnedKey  string
	mrPinnedInfo forge.MRInfo

	// mergeRefState records the LAST merge-ref probe outcome for the pinned
	// MR (snapshot.go's mergeResultDigest), which the merge-result-pinning
	// capability entry grades from. lastMergeState records the mergeable_state
	// of the latest PR read, which gates whether the merge-ref probe runs at
	// all (REQ-E10-S11-03: a non-clean PR is not mergeable now — the merge
	// queue shape — and the digest axis is honestly unavailable). mergeRefState
	// is guarded with the same small-state mutex as the pin caches;
	// lastMergeState carries its OWN mutex because it is recorded from the PR
	// read chain, which mrPinned holds scopeMu through (a mutex may never be
	// re-entered).
	mergeRefState  string
	stateMu        sync.Mutex
	lastMergeState string

	// nodeIDs remembers each review comment's GraphQL node id (REST `node_id`)
	// keyed by its numeric REST id. Thread resolution is GraphQL-only on
	// GitHub (dossier §4) and addresses threads by node id, while the port's
	// ResolveThread carries the REST-derived Thread.ID ("comment/<id>") — the
	// map, populated by every listing and creation, is how the adapter keeps
	// the port's signature without losing the GraphQL handle. Guarded because
	// a client may be shared across goroutines, like the warnings set.
	nodeMu  sync.Mutex
	nodeIDs map[int64]string

	warnMu   sync.Mutex
	warnings map[string]struct{}
}

// Bounds mirror the GitLab adapter's (AUD-S10/S11, E10-S05): the same
// fail-closed availability posture at the port — a hostile or broken endpoint
// must not exhaust the runner's memory or spin a pagination loop forever, and
// only idempotent GETs retry.
const (
	// maxResponseBytes caps a single response body (the GitLab adapter's bound).
	maxResponseBytes = 8 << 20
	// maxListPages caps every paginated listing; exhaustion fails CLOSED.
	maxListPages = 100
	// listPerPage is the page size every listing read uses.
	listPerPage = 100

	defaultMaxAttempts    = 4
	defaultBaseBackoff    = 200 * time.Millisecond
	defaultMaxBackoff     = 4 * time.Second
	defaultRequestTimeout = 30 * time.Second
)

// RetryPolicy is the bounded retry/backoff configuration for idempotent reads.
// Sleep and Jitter are injected seams (determinism: assertions carry no
// wall-clock or randomness).
type RetryPolicy struct {
	MaxAttempts    int
	BaseBackoff    time.Duration
	MaxBackoff     time.Duration
	RequestTimeout time.Duration
	Sleep          func(time.Duration)
	Jitter         func() float64
}

// Option configures the client.
type Option func(*Client)

// WithRetry replaces the whole retry policy. Zero-valued fields keep their
// defaults.
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

// WithSleeper replaces the backoff sleep seam (tests spend no wall-clock).
func WithSleeper(sleep func(time.Duration)) Option {
	return func(c *Client) { c.retry.Sleep = sleep }
}

// WithContext sets the parent context every request derives from.
func WithContext(ctx context.Context) Option {
	return func(c *Client) { c.ctx = ctx }
}

// New builds the GitHub adapter client. token is a PAT, or (when WithApp
// installs the App credentials) a private key whose installation token is
// minted and refreshed automatically.
//
// Without options the client ships the default bounded-retry policy (E10-S05).
func New(endpoint, token, botName string, opts ...Option) *Client {
	c := &Client{
		endpoint: strings.TrimRight(endpoint, "/"),
		token:    token,
		botName:  botName,
		http:     &http.Client{Timeout: 30 * time.Second},
		ctx:      context.Background(),
		retry: RetryPolicy{
			MaxAttempts:    defaultMaxAttempts,
			BaseBackoff:    defaultBaseBackoff,
			MaxBackoff:     defaultMaxBackoff,
			RequestTimeout: defaultRequestTimeout,
			Sleep:          time.Sleep,
			// #nosec G404 -- backoff jitter spreads retry storms; it is not a
			// security or decision input, and the decision path (internal/core)
			// remains free of randomness by construction.
			Jitter: func() float64 { return 0.5 },
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ErrNotFound is the adapter's absent sentinel — the PORT sentinel wrapped with
// the adapter's prefix at the call sites (the transitional-alias pattern the
// GitLab adapter records). Absent means ABSENT: see fileRead for the
// discrimination that licenses it (S00 Q4).
var ErrNotFound = forge.ErrNotFound

// ErrUnauthorized is the port sentinel for a permission failure. GitHub returns
// 404 for permission-denied resources too (to avoid leaking existence), so this
// adapter discriminates with the content-scope probe before it ever claims
// absence (S00 Q4).
var ErrUnauthorized = forge.ErrUnauthorized

// errRateLimited is the transport failure a rate-limited 403 maps to: never a
// sentinel, never absence (S00 Q4).
var errRateLimited = errors.New("github: rate limited (primary/secondary limit) — transport failure")

// errBodyTooLarge marks the bounded-read failure as DETERMINISTIC so the retry
// budget is not spent on it (the AUD-S10 pattern).
var errBodyTooLarge = errors.New("github: response body over limit")

// Warn records a non-fatal anomaly (the AUD-S12 / REL-06 warning channel).
// Duplicates collapse; the set makes a double run byte-identical.
func (c *Client) warn(msg string) {
	c.warnMu.Lock()
	defer c.warnMu.Unlock()
	if c.warnings == nil {
		c.warnings = map[string]struct{}{}
	}
	c.warnings[msg] = struct{}{}
}

// Warnings returns the deduplicated, sorted anomalies observed so far (the
// forge.Warner capability; AUD-S12 / REL-06).
func (c *Client) Warnings() []string {
	c.warnMu.Lock()
	defer c.warnMu.Unlock()
	out := make([]string, 0, len(c.warnings))
	for w := range c.warnings {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// readBounded reads at most limit bytes from r and errors when the source has
// MORE than limit bytes to give (fail-closed: the prefix is discarded, never
// parsed as a truncated document — AUD-S10).
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%w: response body exceeds the %d-byte limit — refusing to parse a truncated response",
			errBodyTooLarge, limit)
	}
	return raw, nil
}

// retryableMethod reports whether an HTTP method is safe to replay
// automatically: ONLY idempotent reads (writes are never retried,
// REQ-E10-S05-02 — replaying a POST duplicates a comment; replaying a merge PUT
// re-runs a compare-and-swap the caller believed it had lost).
func retryableMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// transientStatus reports whether a status is worth another try.
func transientStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// backoff returns the wait before the attempt AFTER the given 1-based attempt
// number: an exponential window clamped at MaxBackoff, spread over its lower
// half by the injected jitter source (determinism: injected, never math/rand).
func (c *Client) backoff(attempt int) time.Duration {
	window := c.retry.BaseBackoff << (attempt - 1)
	if window <= 0 || window > c.retry.MaxBackoff {
		window = c.retry.MaxBackoff
	}
	half := window / 2
	return half + time.Duration(c.retry.Jitter()*float64(half))
}

// do issues one authenticated REST call under the port's transport policy:
// idempotent GETs retry within a bounded, jittered budget under a per-request
// deadline; writes get exactly one attempt; every read is byte-bounded. The
// status, headers and body are returned to callers, which own the
// status→sentinel mapping (S00 Q4).
func (c *Client) do(method, path string, body io.Reader, contentType string) (int, http.Header, []byte, error) {
	attempts := 1
	if retryableMethod(method) && body == nil {
		attempts = c.retry.MaxAttempts
	}
	for attempt := 1; ; attempt++ {
		if err := c.ctx.Err(); err != nil {
			return 0, nil, nil, fmt.Errorf("github: %s %s: %w", method, path, err)
		}
		status, headers, raw, err := c.doOnce(method, path, body, contentType)
		if attempt >= attempts || !transient(status, err) {
			return status, headers, raw, err
		}
		c.retry.Sleep(c.backoff(attempt))
	}
}

// transient reports whether an attempt's outcome is worth another try: a
// transport-level failure, a 429, or any 5xx. Deterministic 4xxes — 401/403/404
// included — surface at once, and an over-limit body is excluded (deterministic).
func transient(status int, err error) bool {
	if errors.Is(err, errBodyTooLarge) {
		return false
	}
	if err != nil {
		return true
	}
	return transientStatus(status)
}

// doOnce performs exactly one attempt under a per-request context deadline.
func (c *Client) doOnce(method, path string, body io.Reader, contentType string) (int, http.Header, []byte, error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.retry.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("github: build request %s %s: %w", method, path, err)
	}
	bearer, err := c.bearer(ctx)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("github: %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.http.Do(req)
	if err != nil {
		// The credential never reaches an error message: only method/path/status.
		return 0, nil, nil, fmt.Errorf("github: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if isRateLimited(resp.Header) {
		return resp.StatusCode, resp.Header, nil, errRateLimited
	}
	raw, err := readBounded(resp.Body, maxResponseBytes)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, fmt.Errorf("github: read body %s %s: %w", method, path, err)
	}
	return resp.StatusCode, resp.Header, raw, nil
}

// isRateLimited reports a response carrying a primary/secondary rate-limit
// marker (S00 Q4: a rate-limited 403 is a transport failure — retried like a
// 429, never mapped to a sentinel, never absence). GitHub conveys it via the
// Retry-After header; the X-RateLimit-Remaining: 0 marker is the other shape.
func isRateLimited(h http.Header) bool {
	return h.Get("Retry-After") != "" || h.Get("X-RateLimit-Remaining") == "0"
}

// static assertion that *Client implements the full forge.RunPort composite
// (Forge + Snapshotter + Resolver + the MR/identity reads). The compiler, not a
// test, proves the port is satisfied.
var _ forge.RunPort = (*Client)(nil)
