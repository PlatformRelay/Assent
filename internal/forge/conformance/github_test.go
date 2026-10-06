package conformance

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
	github "github.com/PlatformRelay/assent/internal/forge/github"
	"github.com/PlatformRelay/assent/internal/render"
)

// github_test.go wires the GitHub adapter into the shared suite (E10-S10,
// REQ-E10-S10-01): an httptest harness serving GitHub's REST routes — PR GET,
// pull-files, contents (probe + specific path), review comments, issue
// comments, approve, merge — plus the GraphQL routes thread resolution and
// arming need. The Backend is a Factory and nothing more, exactly like the
// fake and GitLab backends in backends_test.go: the case bodies live in
// importable non-test Go, and a fourth adapter needs a file this size and no
// copied cases.
//
// The harness models the addressing model S00 pinned (docs/planning/
// github-addressing-model.md): the fork PR's head identity is carried by the
// PR JSON's head.repo, while the head CONTENT is reachable in the base repo at
// the head SHA (refs/pull/N/head carries the same commit) — which is the
// addressing model the adapter reads, so the harness serves content by ref,
// never by repository.

const ghToken = "test-token"

type ghThreadRow struct {
	id       int64
	nodeID   string
	body     string
	author   string
	resolved bool
}

type ghNoteRow struct {
	id     int64
	body   string
	author string
}

type githubHarness struct {
	project   string
	mr        string
	botAuthor string

	// MR heads and the merge-result state. mergeRefSHA is the object SHA the
	// merge ref refs/pull/{n}/merge carries; it is what the adapter's
	// mergeResultDigest reads and what Pins() reports — one honest digest.
	sourceSHA   string
	targetSHA   string
	mergeRefSHA string

	mergeableState string // served in the PR JSON; "clean" by default
	allowAutoMerge bool   // served in the repo settings probe; true by default

	forkMR       bool
	forkRepo     string // head.repo.full_name when the MR is a fork
	governedPath string

	baseFile []byte
	baseSet  bool
	headFile []byte
	headSet  bool

	refusedPath string

	threads []ghThreadRow
	notes   []ghNoteRow
	nextID  int64

	// Transport knobs (E10-S05), the mirror of the GitLab harness's: a page
	// that never shortens, an injected write status, and a response slower
	// than a short per-request deadline.
	pageStorm   bool
	writeStatus int
	slowAfter   time.Duration

	// afterPRRead fires once a PR read has been SERVED — the TOCTOU seam the
	// source-moved case drives, mirroring the GitLab harness's afterMRRead.
	afterPRRead func(h *githubHarness)

	// write counters, read through the Observer surface.
	noteCreateCalls int
	noteUpdateCalls int
	reviewPOSTs     int
	approvePOSTs    int
	mergePUTs       int
	gqlResolveCalls int
	armMutations    int

	taken map[int64]bool
}

func newGitHubHarness(project, mr string) *githubHarness {
	return &githubHarness{
		project:        project,
		mr:             mr,
		botAuthor:      botID,
		mergeableState: "clean",
		allowAutoMerge: true,
		governedPath:   "topics/orders.yaml",
		mergeRefSHA:    "mrgSHA",
		nextID:         10_000,
		taken:          map[int64]bool{},
	}
}

// rowID derives the numeric REST id a seeded artifact is served under: the
// numeric suffix of the seed id (the seeded id's number is the row's number,
// so the adapter's "comment/<n>" forge id round-trips the seed), or a fresh
// id for seeds whose id carries no numeric suffix. Collapsing two seeds onto
// one row is never allowed — a collision is served with a fresh id.
func (h *githubHarness) rowID(seedID string) int64 {
	numeric := numericSuffixOf(seedID)
	if numeric > 0 && !h.taken[numeric] {
		h.taken[numeric] = true
		return numeric
	}
	for {
		h.nextID++
		if !h.taken[h.nextID] {
			h.taken[h.nextID] = true
			return h.nextID
		}
	}
}

func numericSuffixOf(id string) int64 {
	i := strings.LastIndex(id, "/")
	if i < 0 {
		return 0
	}
	n, err := strconv.ParseInt(id[i+1:], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func (h *githubHarness) seedThread(id, author string, marker forge.Marker, resolved bool) error {
	body, err := render.Envelope(marker, "seed")
	if err != nil {
		return err
	}
	row := ghThreadRow{id: h.rowID(id), body: body, author: author, resolved: resolved}
	row.nodeID = fmt.Sprintf("PRRC_node%d", row.id)
	h.threads = append(h.threads, row)
	return nil
}

func (h *githubHarness) seedNote(id, author string, marker forge.Marker, body string) error {
	fullBody, err := render.Envelope(marker, body)
	if err != nil {
		return err
	}
	h.notes = append(h.notes, ghNoteRow{id: h.rowID(id), body: fullBody, author: author})
	return nil
}

func (h *githubHarness) servesMergeRef() bool {
	return h.mergeableState == "clean"
}

// client builds the harness's GitHub adapter pointed at the httptest server.
func (h *githubHarness) client(t interface {
	Helper()
	Cleanup(func())
}) *github.Client {
	t.Helper()
	return h.clientWith(t, github.WithSleeper(func(time.Duration) {}))
}

// clientWith builds the harness's client with explicit options, for the
// transport cases that must override the retry/deadline policy.
func (h *githubHarness) clientWith(t interface {
	Helper()
	Cleanup(func())
}, opts ...github.Option) *github.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(h.handle))
	t.Cleanup(srv.Close)
	return github.New(srv.URL, ghToken, h.botAuthor, opts...)
}

func (h *githubHarness) handle(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get("Authorization"); got != "Bearer "+ghToken {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	prBase := fmt.Sprintf("/repos/%s/pulls/%s", h.project, h.mr)
	repoBase := "/repos/" + h.project
	issueCommentsBase := fmt.Sprintf("/repos/%s/issues/%s/comments", h.project, h.mr)
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == prBase:
		h.servePR(w, r)
	case r.Method == http.MethodGet && path == repoBase:
		h.serveRepo(w, r)
	case r.Method == http.MethodGet && strings.Contains(path, "/git/ref/refs/pull/"+h.mr+"/merge"):
		h.serveMergeRef(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(path, prBase+"/files"):
		h.serveFiles(w, r)
	case r.Method == http.MethodGet && path == prBase+"/comments":
		h.serveThreadListing(w, r)
	case r.Method == http.MethodPost && path == prBase+"/comments":
		h.createThreadRow(w, r)
	case r.Method == http.MethodGet && path == issueCommentsBase:
		h.serveNotes(w, r)
	case r.Method == http.MethodPost && path == issueCommentsBase:
		h.createNoteRow(w, r)
	case r.Method == http.MethodPatch && strings.HasPrefix(path, repoBase+"/issues/comments/"):
		h.updateNoteRow(w, r)
	case r.Method == http.MethodPost && path == prBase+"/reviews":
		h.approve(w, r)
	case r.Method == http.MethodPut && path == prBase+"/merge":
		h.merge(w, r)
	case r.Method == http.MethodPost && path == "/graphql":
		h.serveGraphQL(w, r)
	case r.Method == http.MethodGet && path == repoBase+"/contents":
		// The content-scope probe (S00 Q4): the root listing at the asked
		// ref. The harness models a repo whose contents READ is granted, so
		// the probe answers 200 and a 404 on the specific path is genuine
		// absence; a refused path is refused on the SPECIFIC path instead,
		// which keeps the forbidden≠absent seam exercised through the same
		// status mapping the real forge's refusal produces.
		_, _ = io.WriteString(w, "[]")
	case r.Method == http.MethodGet && strings.HasPrefix(path, repoBase+"/contents/"):
		h.serveContent(w, r)
	case r.Method == http.MethodGet && strings.Contains(path, "/branches/") && strings.HasSuffix(path, "/protection"):
		// The branch-protection shape (dossier C3) the capability probes
		// enumerate. Nothing in the honest v1 report grades from it yet —
		// the field names are unverified (S00 Q2 row 2) — so the shape is
		// served, not asserted on.
		_, _ = io.WriteString(w, `{"required_pull_request_reviews":{"required_approving_review_count":1},"required_conversation_resolution":{"enabled":true}}`)
	default:
		http.Error(w, "unexpected "+r.Method+" "+path, http.StatusInternalServerError)
	}
}

// ---- read routes ----

func (h *githubHarness) servePR(w http.ResponseWriter, r *http.Request) {
	h.slow()
	headRepo := h.project
	if h.forkMR {
		headRepo = h.forkRepo
	}
	number, _ := strconv.Atoi(h.mr)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"number":          number,
		"node_id":         fmt.Sprintf("PR_node%d", number),
		"sha":             h.sourceSHA,
		"user":            map[string]any{"login": "octocat"},
		"labels":          []ghLabelView{},
		"mergeable_state": h.mergeableState,
		"base": map[string]any{
			"ref":  "main",
			"sha":  h.targetSHA,
			"repo": map[string]any{"full_name": h.project},
		},
		"head": map[string]any{
			"ref":  "feature",
			"sha":  h.sourceSHA,
			"repo": map[string]any{"full_name": headRepo},
		},
	})
	// Fire AFTER the response is written, so this read returns the PRE-move
	// value and only the NEXT one sees the drift (the gitlab harness's
	// afterMRRead seam).
	if h.afterPRRead != nil {
		h.afterPRRead(h)
	}
}

// ghLabelView is the label object the PR JSON carries.
type ghLabelView struct {
	Name string `json:"name"`
}

func (h *githubHarness) serveRepo(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"allow_auto_merge": h.allowAutoMerge,
	})
}

func (h *githubHarness) serveMergeRef(w http.ResponseWriter, _ *http.Request) {
	if !h.servesMergeRef() {
		http.Error(w, "no merge ref: not mergeable or merge queue in use", http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ref": fmt.Sprintf("refs/pull/%s/merge", h.mr),
		"object": map[string]any{
			"sha":  h.mergeRefSHA,
			"type": "commit",
		},
	})
}

func (h *githubHarness) serveFiles(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if h.pageStorm {
		// E10-S05 transport knob: a paginator that never shortens — a FULL
		// page on every request, so the cap must fail closed.
		storm := make([]map[string]any, 100)
		for i := range storm {
			storm[i] = map[string]any{"filename": fmt.Sprintf("storm-%d", i)}
		}
		_ = json.NewEncoder(w).Encode(storm)
		return
	}
	if page > 1 {
		_, _ = io.WriteString(w, "[]")
		return
	}
	_ = json.NewEncoder(w).Encode([]map[string]any{
		{"filename": h.governedPath},
	})
}

func (h *githubHarness) threadViews() []string {
	out := make([]string, 0, len(h.threads))
	for _, row := range h.threads {
		out = append(out, fmt.Sprintf(
			`{"id":%d,"node_id":%q,"body":%s,"user":{"login":%q}}`,
			row.id, row.nodeID, quoteJSON(row.body), row.author))
	}
	return out
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (h *githubHarness) serveThreadListing(w http.ResponseWriter, r *http.Request) {
	if h.pageStorm {
		// E10-S05 transport knob: a paginator that never shortens — the storm
		// serves a FULL page on every request, before any page arithmetic.
		storm := make([]string, 100)
		for i := range storm {
			storm[i] = fmt.Sprintf(`{"id":%d,"node_id":"PRRC_storm%d","body":"storm","user":{"login":"storm"}}`, i, i)
		}
		_, _ = io.WriteString(w, "["+strings.Join(storm, ",")+"]")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page > 1 {
		_, _ = io.WriteString(w, "[]")
		return
	}
	views := h.threadViews()
	_, _ = io.WriteString(w, "["+strings.Join(views, ",")+"]")
}

func (h *githubHarness) createThreadRow(w http.ResponseWriter, r *http.Request) {
	h.reviewPOSTs++
	if h.writeStatus != 0 {
		http.Error(w, "injected transport failure", h.writeStatus)
		return
	}
	var posted struct {
		Body string `json:"body"`
	}
	_ = json.NewDecoder(r.Body).Decode(&posted)
	row := ghThreadRow{id: h.rowID("comment/"), body: posted.Body, author: h.botAuthor}
	row.nodeID = fmt.Sprintf("PRRC_node%d", row.id)
	h.threads = append(h.threads, row)
	w.WriteHeader(http.StatusCreated)
	_, _ = io.WriteString(w, fmt.Sprintf(`{"id":%d,"node_id":%q,"user":{"login":%q}}`, row.id, row.nodeID, row.author))
}

func (h *githubHarness) noteViews() []string {
	out := make([]string, 0, len(h.notes))
	for _, row := range h.notes {
		out = append(out, fmt.Sprintf(
			`{"id":%d,"node_id":"IC_node%d","body":%s,"user":{"login":%q}}`,
			row.id, row.id, quoteJSON(row.body), row.author))
	}
	return out
}

func (h *githubHarness) serveNotes(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page > 1 {
		_, _ = io.WriteString(w, "[]")
		return
	}
	views := h.noteViews()
	_, _ = io.WriteString(w, "["+strings.Join(views, ",")+"]")
}

func (h *githubHarness) createNoteRow(w http.ResponseWriter, r *http.Request) {
	h.noteCreateCalls++
	if h.writeStatus != 0 {
		http.Error(w, "injected transport failure", h.writeStatus)
		return
	}
	var posted struct {
		Body string `json:"body"`
	}
	_ = json.NewDecoder(r.Body).Decode(&posted)
	row := ghNoteRow{id: h.rowID("issue-comment/"), body: posted.Body, author: h.botAuthor}
	h.notes = append(h.notes, row)
	w.WriteHeader(http.StatusCreated)
	_, _ = io.WriteString(w, fmt.Sprintf(`{"id":%d,"node_id":"IC_node%d"}`, row.id, row.id))
}

func (h *githubHarness) updateNoteRow(w http.ResponseWriter, r *http.Request) {
	h.noteUpdateCalls++
	if h.writeStatus != 0 {
		http.Error(w, "injected transport failure", h.writeStatus)
		return
	}
	var posted struct {
		Body string `json:"body"`
	}
	_ = json.NewDecoder(r.Body).Decode(&posted)
	numeric := numericSuffixOf(r.URL.Path)
	for i := range h.notes {
		if h.notes[i].id == numeric {
			h.notes[i].body = posted.Body
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, fmt.Sprintf(`{"id":%d}`, numeric))
			return
		}
	}
	http.NotFound(w, r)
}

func (h *githubHarness) approve(w http.ResponseWriter, _ *http.Request) {
	h.approvePOSTs++
	if h.writeStatus != 0 {
		http.Error(w, "injected transport failure", h.writeStatus)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "state": "APPROVED"})
}

func (h *githubHarness) merge(w http.ResponseWriter, r *http.Request) {
	h.mergePUTs++
	if h.writeStatus != 0 {
		http.Error(w, "injected transport failure", h.writeStatus)
		return
	}
	var body struct {
		SHA         string `json:"sha"`
		MergeMethod string `json:"merge_method"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	// The sha body parameter IS the compare-and-swap on the source head
	// (dossier C10): a moved source is 409, no merge. Modelled faithfully so
	// the sha-guard cases prove the guard, not the fake.
	if body.SHA != h.sourceSHA {
		http.Error(w, "sha mismatch", http.StatusConflict)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"merged":           true,
		"merge_commit_sha": h.mergeRefSHA,
	})
}

// ---- content routes ----

// serveContent serves the governed subject's content by ref: the pinned
// target SHA is the base side, the pinned source SHA the head side — the fork
// PR's head commit is reachable in the base repo at the head SHA (S00 Q1's
// addressing model, which the adapter reads). A side that was never seeded is
// ABSENT (404 → the adapter's ErrNotFound); a refused path answers 403 — the
// forbidden≠absent seam (the probe stays 200: the probe is a listing, the
// refusal is exercised on the same permission the specific read uses).
func (h *githubHarness) serveContent(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if h.refusedPath != "" && strings.Contains(r.URL.Path, h.refusedPath) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch {
	case ref == h.targetSHA:
		if !h.baseSet {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		h.serveContentBase64(w, h.baseFile)
	case ref == h.sourceSHA:
		if !h.headSet {
			http.Error(w, "absent", http.StatusNotFound)
			return
		}
		h.serveContentBase64(w, h.headFile)
	default:
		http.Error(w, "unexpected ref "+ref, http.StatusNotFound)
	}
}

func (h *githubHarness) serveContentBase64(w http.ResponseWriter, content []byte) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"content":   base64.StdEncoding.EncodeToString(content),
		"encoding":  "base64",
		"truncated": false,
		"size":      len(content),
	})
}

func (h *githubHarness) slow() {
	if h.slowAfter > 0 {
		time.Sleep(h.slowAfter)
	}
}

// ---- GraphQL routes ----

// serveGraphQL serves the two GraphQL operations the adapter uses: the
// reviewThreads listing (thread resolution state, dossier C1) and the
// resolveReviewThread mutation (dossier C2). A thread row IS a review comment
// in this harness; the thread's node id is the comment's node id, which is the
// handle resolveReviewThread addresses threads by (dossier §4, adapter-internal
// seam — the node id is remembered by every listing and creation).
func (h *githubHarness) serveGraphQL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad graphql request", http.StatusBadRequest)
		return
	}
	switch {
	case strings.Contains(req.Query, "resolveReviewThread"):
		h.gqlResolveCalls++
		if h.writeStatus != 0 {
			http.Error(w, "injected transport failure", h.writeStatus)
			return
		}
		threadID, _ := req.Variables["threadId"].(string)
		for i := range h.threads {
			if h.threads[i].nodeID == threadID {
				if h.threads[i].resolved {
					// Resolving an already-resolved thread is a no-op: the
					// forge's "already resolved" rejection IS success for this
					// verb (the adapter's resolve contract).
					_, _ = io.WriteString(w, `{"data":null,"errors":[{"message":"Thread is already resolved."}]}`)
					return
				}
				h.threads[i].resolved = true
				_, _ = io.WriteString(w, fmt.Sprintf(
					`{"data":{"resolveReviewThread":{"thread":{"id":%q,"isResolved":true}}}}`, threadID))
				return
			}
		}
		_, _ = io.WriteString(w, `{"data":null,"errors":[{"message":"Could not resolve to a node with the global id."}]}`)
	case strings.Contains(req.Query, "reviewThreads"):
		nodes := make([]string, 0, len(h.threads))
		for _, row := range h.threads {
			nodes = append(nodes, fmt.Sprintf(
				`{"id":%q,"isResolved":%t,"comments":{"nodes":[{"databaseId":%d}]}}`,
				row.nodeID, row.resolved, row.id))
		}
		_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[`+strings.Join(nodes, ",")+`]}}}}}`)
	default:
		http.Error(w, "unexpected graphql operation", http.StatusInternalServerError)
	}
}

// ---- Backend wiring (the mirror of backends_test.go's gitlabBackend) ----

type githubBackend struct {
	h  *githubHarness
	cp *countingPort
}

func (b githubBackend) MergeAttempts() int   { return b.cp.mergeAttempts }
func (b githubBackend) MergesPerformed() int { return b.cp.mergesPerformed }
func (b githubBackend) Approvals() int       { return b.cp.approvals }
func (b githubBackend) ThreadsCreated() int  { return b.cp.threadsCreated }
func (b githubBackend) ThreadsResolved() int { return b.cp.threadsResolved }
func (b githubBackend) NotesCreated() int    { return b.h.noteCreateCalls }
func (b githubBackend) NotesUpdated() int    { return b.h.noteUpdateCalls }

func (b githubBackend) NoteBody(id string) string {
	numeric := numericSuffixOf(id)
	if numeric == 0 {
		return ""
	}
	for _, row := range b.h.notes {
		if row.id == numeric {
			return row.body
		}
	}
	return ""
}

func (b githubBackend) IsResolved(id string) bool {
	numeric := numericSuffixOf(id)
	for _, row := range b.h.threads {
		if row.id == numeric {
			return row.resolved
		}
	}
	return false
}

func (b githubBackend) ThreadCount() int { return len(b.h.threads) }

func (b githubBackend) BotThreadCount() int {
	n := 0
	for _, row := range b.h.threads {
		if row.author == b.h.botAuthor {
			n++
		}
	}
	return n
}

func (b githubBackend) OpenBotThreadCount() int {
	n := 0
	for _, row := range b.h.threads {
		if row.author == b.h.botAuthor && !row.resolved {
			n++
		}
	}
	return n
}

func (b githubBackend) SeedThread(id, author string, m forge.Marker, resolved bool) error {
	return b.h.seedThread(id, author, m, resolved)
}

func (b githubBackend) SeedNote(id, author string, m forge.Marker, body string) error {
	return b.h.seedNote(id, author, m, body)
}

func (b githubBackend) MoveTargetHead(sha string) { b.h.targetSHA = sha }

// MoveSourceHead moves the PR's source head immediately. The harness's merge
// route checks the sha body pin against h.sourceSHA, so a restored head merges
// and a moved-away head is refused — the same CAS the production adapter
// performs (dossier C10).
func (b githubBackend) MoveSourceHead(sha string) { b.h.sourceSHA = sha }

// SeedFile records the governed subject's content on one SIDE of the pull
// request: the base side is served at the pinned target SHA, the head side at
// the pinned source SHA — in the BASE repository, which is where the adapter
// reads a fork's head content (the head SHA is reachable there).
func (b githubBackend) SeedFile(path, side string, content []byte) {
	switch side {
	case FileSideBase:
		b.h.baseFile = append([]byte(nil), content...)
		b.h.baseSet = true
	case FileSideHead:
		b.h.headFile = append([]byte(nil), content...)
		b.h.headSet = true
	}
}

// RefuseFileRead makes the SPECIFIC content read of `path` answer 403 — the
// forge refusing the read it was asked for — while the content-scope probe
// stays 200, mirroring the GitLab harness's refused-path model. The case
// proves a refused read renders ErrUnauthorized, never absence, through the
// adapter's specific-path status mapping.
func (b githubBackend) RefuseFileRead(path string) { b.h.refusedPath = path }

// Pins reports the pins an evaluation would record against THIS backend: the
// heads the PR currently reports, and the merge-ref digest the adapter's own
// probe would read ("" when the PR is not mergeable — the pin is honestly
// absent, never fabricated).
func (b githubBackend) Pins() forge.DesiredMerge {
	digest := ""
	if b.h.servesMergeRef() {
		digest = b.h.mergeRefSHA
	}
	return forge.DesiredMerge{
		SourceSha:         b.h.sourceSHA,
		TargetSha:         b.h.targetSHA,
		MergeResultDigest: digest,
	}
}

// DriftSourceHeadAfterRead moves the PR source head once the pre-check's PR
// read has been served — the TOCTOU window. Reconcile reads the heads, then
// MergeCAS reads them again; arming on the FIRST read means the second sees
// the moved head, so the atomic guard is the thing that refuses.
func (b githubBackend) DriftSourceHeadAfterRead(sha string) {
	b.h.afterPRRead = func(h *githubHarness) { h.sourceSHA = sha }
}

func githubFactory(t TB, cfg Config) Backend {
	t.Helper()
	h := newGitHubHarness(cfg.Project, cfg.MR)
	h.botAuthor = cfg.BotAuthor
	h.sourceSHA = cfg.CurrentSourceSHA
	h.targetSHA = cfg.CurrentTargetSHA
	h.forkMR = cfg.ForkMR
	h.forkRepo = "octo-contrib/fork-999"
	h.governedPath = cfg.GovernedPath
	cp := newCountingPort(h.client(t))
	b := githubBackend{h: h, cp: cp}
	return Backend{Port: cp, Fixture: b, Observer: b}
}
