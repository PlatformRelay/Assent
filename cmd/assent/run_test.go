package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge/gitlab"
)

// fakeGitLab is an in-process GitLab REST v4 mock for the `run` end-to-end tests.
// It records the mutating calls (discussions POSTed, approvals, merges) so a test
// can assert EXACT write behaviour — the fail-closed teeth (zero approve/merge
// when a write must not happen). No live network: httptest only.
type fakeGitLab struct {
	srv *httptest.Server

	sourceSHA, targetTip string
	sourceBranch, target string
	mergePolicy          string            // MergePolicy served from the TARGET ref.
	rulesetBinding       string            // RulesetBinding served from the TARGET ref.
	config               string            // optional Config served from the TARGET ref.
	providerDecls        map[string]string // optional host declarations (.assent/providers/<name>.json)
	baseFile, headFile   string            // governed-file content at target/source ref.
	governedPath         string

	// recorded writes
	discussionsPosted int
	notesPosted       int
	notesUpdated      int
	approvals         int
	merges            int
	lastMergeSHA      string
	lastThreadBody    string
	lastSummaryBody   string

	// stateful discussions — POST appends; GET lists; PUT ?resolved=true marks resolved.
	botAuthor   string
	discussions []fakeDiscussion
	notes       []fakeNote

	// E4-S06 forge Snapshot/Resolve probe configuration.
	projectJSON  string
	changedFiles []string

	// AUD-S01 / ADR-0020 changed-file completeness knobs. changesCount overrides
	// the MR GET's changes_count (e.g. "1000+" — GitLab's capped form — to model
	// an unprovable enumeration); diffsStatus, when non-200, models a
	// diff-endpoint 404/5xx hard error.
	changesCount string
	diffsStatus  int

	freeTier            bool
	mrAuthor            string
	approvalEligible    bool
	approvalRulesStatus int

	// E4-S08 adversarial source-ref policy bytes (target ref stays trusted).
	sourceMergePolicy    string
	sourceRulesetBinding string

	// forkMR models a fork workflow (source_project_id != target project_id).
	forkMR bool

	// policyLoads records FileAtRef calls for `.assent/**` policy documents.
	policyLoads []policyLoad
}

type policyLoad struct {
	path string
	ref  string
}

type fakeDiscussion struct {
	id       string
	body     string
	resolved bool
}

type fakeNote struct {
	id   int
	body string
}

const fakeForgePremiumProjectJSON = `{
	"only_allow_merge_if_all_discussions_are_resolved":true,
	"merge_trains_enabled":true,
	"ci_config_path":".gitlab-ci.yml@group/external-ci"
}`

const fakeForgeIneligibleProjectJSON = `{
	"only_allow_merge_if_all_discussions_are_resolved":false,
	"merge_trains_enabled":true,
	"ci_config_path":".gitlab-ci.yml@group/external-ci"
}`

const fakeForgeInsecureProjectJSON = `{
	"only_allow_merge_if_all_discussions_are_resolved":true,
	"merge_trains_enabled":true,
	"ci_config_path":".gitlab-ci.yml"
}`

func newFakeGitLab(t *testing.T) *fakeGitLab {
	t.Helper()
	f := &fakeGitLab{
		sourceSHA:           "srcSHA",
		targetTip:           "tgtTIP",
		sourceBranch:        "feature",
		target:              "main",
		governedPath:        "topics/orders.yaml",
		mergePolicy:         mergePolicyChallenge,
		rulesetBinding:      rulesetBindingNonDestructive,
		botAuthor:           "assent-bot",
		projectJSON:         fakeForgePremiumProjectJSON,
		mrAuthor:            "alice",
		approvalEligible:    true,
		approvalRulesStatus: http.StatusOK,
	}
	f.changedFiles = []string{f.governedPath}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitLab) handle(w http.ResponseWriter, r *http.Request) {
	p := r.URL.EscapedPath()
	switch {
	case p == "/api/v4/projects/42/merge_requests/7" && r.Method == http.MethodGet:
		sourceProjectID := 42
		if f.forkMR {
			sourceProjectID = 99
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"iid": 7, "project_id": 42, "source_project_id": sourceProjectID,
			"sha":           f.sourceSHA,
			"source_branch": f.sourceBranch, "target_branch": f.target,
			"changes_count": f.changesCountBody(),
			"author":        map[string]any{"id": 101, "username": f.mrAuthor},
		})
	case p == "/api/v4/projects/42/repository/branches/main" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{"commit": map[string]any{"id": f.targetTip}})
	case p == "/api/v4/projects/42" && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(f.projectJSONBody()))
	case p == "/api/v4/projects/42/merge_requests/7/diffs" && r.Method == http.MethodGet:
		f.serveMRDiffs(w, r)
	case strings.HasPrefix(p, "/api/v4/projects/42/merge_requests/7/approval_rules") && r.Method == http.MethodGet:
		f.serveApprovalRules(w)
	case p == "/api/v4/projects/42/merge_requests/7/approval_state" && r.Method == http.MethodGet:
		f.serveApprovalState(w)
	case p == "/api/v4/user" && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(`{"id":999,"username":"` + f.botAuthor + `"}`))
	case strings.HasPrefix(p, "/api/v4/projects/42/repository/files/") && strings.HasSuffix(p, "/raw"):
		f.serveFile(w, r, p)
	case p == "/api/v4/projects/42/merge_requests/7/discussions" && r.Method == http.MethodGet:
		f.serveDiscussions(w, r)
	case p == "/api/v4/projects/42/merge_requests/7/discussions" && r.Method == http.MethodPost:
		f.discussionsPosted++
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		f.lastThreadBody = form.Get("body")
		id := fmt.Sprintf("disc-%d", len(f.discussions)+1)
		f.discussions = append(f.discussions, fakeDiscussion{id: id, body: f.lastThreadBody})
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
	case strings.HasPrefix(p, "/api/v4/projects/42/merge_requests/7/discussions/") && r.Method == http.MethodPut:
		f.resolveDiscussion(w, r, p)
	case p == "/api/v4/projects/42/merge_requests/7/notes" && r.Method == http.MethodGet:
		f.serveNotes(w, r)
	case p == "/api/v4/projects/42/merge_requests/7/notes" && r.Method == http.MethodPost:
		f.createNote(w, r)
	case strings.HasPrefix(p, "/api/v4/projects/42/merge_requests/7/notes/") && r.Method == http.MethodPut:
		f.updateNote(w, r, p)
	case p == "/api/v4/projects/42/merge_requests/7/approve" && r.Method == http.MethodPost:
		f.approvals++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	case p == "/api/v4/projects/42/merge_requests/7/merge" && r.Method == http.MethodPut:
		f.merges++
		f.lastMergeSHA = r.URL.Query().Get("sha")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"state":"merged","merge_commit_sha":"mc"}`))
	default:
		http.Error(w, "unexpected "+r.Method+" "+p, http.StatusInternalServerError)
	}
}

func (f *fakeGitLab) projectJSONBody() string {
	if f.projectJSON != "" {
		return f.projectJSON
	}
	return fakeForgePremiumProjectJSON
}

// enumeratedFiles is the changed-file set the /diffs cassette serves — one diff
// ENTRY per path (old_path == new_path), so the entry count equals the path
// count for this fixture.
func (f *fakeGitLab) enumeratedFiles() []string {
	if len(f.changedFiles) == 0 {
		return []string{f.governedPath}
	}
	return f.changedFiles
}

// changesCountBody is the MR GET's changes_count string — the ADR-0020 §2
// completeness cross-check. It reports the TRUE entry count unless a test
// overrides it (e.g. "1000+", GitLab's capped form) to model an unprovable
// enumeration.
func (f *fakeGitLab) changesCountBody() string {
	if f.changesCount != "" {
		return f.changesCount
	}
	return strconv.Itoa(len(f.enumeratedFiles()))
}

// serveMRDiffs is the paginated GET .../merge_requests/:iid/diffs cassette
// (ADR-0020 §2). Page 1 carries every entry (a short page, which terminates the
// enumeration below the ceiling); later pages are empty. diffsStatus models the
// §3 hard-error axis.
func (f *fakeGitLab) serveMRDiffs(w http.ResponseWriter, r *http.Request) {
	if f.diffsStatus != 0 && f.diffsStatus != http.StatusOK {
		http.Error(w, "diffs unavailable", f.diffsStatus)
		return
	}
	if r.URL.Query().Get("page") != "1" {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	files := f.enumeratedFiles()
	entries := make([]map[string]string, len(files))
	for i, path := range files {
		entries[i] = map[string]string{"old_path": path, "new_path": path}
	}
	_ = json.NewEncoder(w).Encode(entries)
}

func (f *fakeGitLab) serveApprovalRules(w http.ResponseWriter) {
	if f.freeTier || f.approvalRulesStatus == http.StatusNotFound {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if f.approvalRulesStatus != 0 && f.approvalRulesStatus != http.StatusOK {
		http.Error(w, "unavailable", f.approvalRulesStatus)
		return
	}
	_, _ = w.Write([]byte(`[{"id":1,"name":"security-review","rule_type":"regular","approvals_required":1}]`))
}

func (f *fakeGitLab) serveApprovalState(w http.ResponseWriter) {
	if f.freeTier {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	approvedBy := `[]`
	if f.approvalEligible {
		approvedBy = `[{"user":{"id":202,"username":"bob"}}]`
	}
	body := `{"rules":[{
		"id":1,"name":"security-review","rule_type":"regular","approvals_required":1,"approved":true,
		"eligible_approvers":[{"id":101,"username":"alice"},{"id":202,"username":"bob"},{"id":999,"username":"` + f.botAuthor + `"}],
		"approved_by":` + approvedBy + `
	}]}`
	_, _ = w.Write([]byte(body))
}

// serveDiscussions returns stored MR discussions in GitLab REST shape so
// gitlab.Client.ListBotThreads can filter by bot author and parse markers.
func (f *fakeGitLab) serveDiscussions(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	perPage := 100
	start := (page - 1) * perPage
	if start >= len(f.discussions) {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	end := start + perPage
	if end > len(f.discussions) {
		end = len(f.discussions)
	}
	slice := f.discussions[start:end]
	out := make([]map[string]any, len(slice))
	for i, d := range slice {
		out[i] = map[string]any{
			"id": d.id,
			"notes": []map[string]any{{
				"body":     d.body,
				"resolved": d.resolved,
				"author":   map[string]any{"username": f.botAuthor},
			}},
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

// resolveDiscussion handles PUT …/discussions/{id}?resolved=true (ResolveThread).
func (f *fakeGitLab) resolveDiscussion(w http.ResponseWriter, r *http.Request, p string) {
	if r.URL.Query().Get("resolved") != "true" {
		http.Error(w, "missing resolved=true", http.StatusBadRequest)
		return
	}
	id := strings.TrimPrefix(p, "/api/v4/projects/42/merge_requests/7/discussions/")
	for i := range f.discussions {
		if f.discussions[i].id == id {
			f.discussions[i].resolved = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
			return
		}
	}
	http.Error(w, "discussion not found", http.StatusNotFound)
}

func (f *fakeGitLab) serveNotes(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	perPage := 100
	start := (page - 1) * perPage
	if start >= len(f.notes) {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	end := start + perPage
	if end > len(f.notes) {
		end = len(f.notes)
	}
	out := make([]map[string]any, 0, end-start)
	for _, n := range f.notes[start:end] {
		out = append(out, map[string]any{
			"id":     n.id,
			"body":   n.body,
			"author": map[string]any{"username": f.botAuthor},
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (f *fakeGitLab) createNote(w http.ResponseWriter, r *http.Request) {
	f.notesPosted++
	body, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(body))
	f.lastSummaryBody = form.Get("body")
	id := 8000 + len(f.notes) + 1
	f.notes = append(f.notes, fakeNote{id: id, body: f.lastSummaryBody})
	w.WriteHeader(http.StatusCreated)
	_, _ = io.WriteString(w, fmt.Sprintf(`{"id":%d}`, id))
}

func (f *fakeGitLab) updateNote(w http.ResponseWriter, r *http.Request, p string) {
	f.notesUpdated++
	body, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(body))
	f.lastSummaryBody = form.Get("body")
	rawID := strings.TrimPrefix(p, "/api/v4/projects/42/merge_requests/7/notes/")
	id, err := strconv.Atoi(rawID)
	if err != nil {
		http.Error(w, "bad note id", http.StatusBadRequest)
		return
	}
	for i := range f.notes {
		if f.notes[i].id == id {
			f.notes[i].body = f.lastSummaryBody
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, fmt.Sprintf(`{"id":%d}`, id))
			return
		}
	}
	http.Error(w, "note not found", http.StatusNotFound)
}

// serveFile routes the three loaded documents (MergePolicy, RulesetBinding, the
// optional Config) — all from the TARGET ref (ADR-0015 §1) — plus the governed
// file (base from target, head from source). Routing is by path substring: the
// three document paths carry distinctive names, the governed path does not.
func (f *fakeGitLab) serveFile(w http.ResponseWriter, r *http.Request, p string) {
	ref := r.URL.Query().Get("ref")
	switch {
	case strings.Contains(p, "merge-policy"):
		f.recordPolicyLoad(p, ref)
		f.servePolicyDocument(w, ref, f.mergePolicy, f.sourceMergePolicy)
		return
	case strings.Contains(p, "ruleset-binding"):
		f.recordPolicyLoad(p, ref)
		f.servePolicyDocument(w, ref, f.rulesetBinding, f.sourceRulesetBinding)
		return
	case strings.Contains(p, "providers"):
		// Host-owned declaration docs (D-065): .assent/providers/<name>.json
		for name, raw := range f.providerDecls {
			if strings.Contains(p, name) {
				f.serveFromTarget(w, ref, raw)
				return
			}
		}
		http.Error(w, "unknown provider declaration "+p, http.StatusNotFound)
		return
	case strings.Contains(p, "config"):
		f.serveFromTarget(w, ref, f.config)
		return
	}
	// The governed file: base from target ref, head from source branch.
	switch ref {
	case f.target:
		_, _ = w.Write([]byte(f.baseFile))
	case f.sourceBranch:
		_, _ = w.Write([]byte(f.headFile))
	default:
		http.Error(w, "unexpected ref "+ref, http.StatusBadRequest)
	}
}

// serveFromTarget enforces the ADR-0015 §1 target-ref trust boundary: a policy
// document is loaded ONLY from the target ref, never the MR source branch.
func (f *fakeGitLab) serveFromTarget(w http.ResponseWriter, ref, content string) {
	if ref != f.target {
		http.Error(w, "policy documents MUST load from the target ref, got "+ref, http.StatusBadRequest)
		return
	}
	_, _ = w.Write([]byte(content))
}

// servePolicyDocument serves merge-policy / ruleset-binding at ref. The target
// ref carries the trusted bytes; the source ref may carry adversarial bytes
// (E4-S08) so tests can prove orchestrate never loads from the MR source branch.
func (f *fakeGitLab) servePolicyDocument(w http.ResponseWriter, ref, targetContent, sourceContent string) {
	switch ref {
	case f.target:
		_, _ = w.Write([]byte(targetContent))
	case f.sourceBranch:
		if sourceContent != "" {
			_, _ = w.Write([]byte(sourceContent))
			return
		}
		http.Error(w, "policy documents MUST load from the target ref, got "+ref, http.StatusBadRequest)
	default:
		http.Error(w, "policy documents MUST load from the target ref, got "+ref, http.StatusBadRequest)
	}
}

func (f *fakeGitLab) recordPolicyLoad(path, ref string) {
	f.policyLoads = append(f.policyLoads, policyLoad{path: path, ref: ref})
}

// factory builds a real *gitlab.Client pointed at the fake server — the exact
// production adapter, driven end-to-end over HTTP without a live network.
func (f *fakeGitLab) factory() func(string, string, string) forgePort {
	return func(_, token, botAuthor string) forgePort {
		if botAuthor != "" {
			f.botAuthor = botAuthor
		}
		return gitlab.New(f.srv.URL, token, botAuthor,
			gitlab.WithSleeper(func(time.Duration) {}))
	}
}

func fixedClock() runClock {
	t := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// mergePolicyChallenge is a frozen MergePolicy of the D-016 shape: a
// valueChanges /partitions modify rule proving `non-destructive` via the bare
// `new >= old`, CHALLENGE on failure (so a partition shrink -> REVIEW). Generic,
// no employer names (D-002).
const mergePolicyChallenge = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "MergePolicy",
  "metadata": { "name": "topic-safety" },
  "spec": {
    "entries": { "topic-registry": { "mode": "document", "root": "", "identity": { "pointer": "/metadata/name" } } },
    "rules": [
      {
        "name": "partitions-must-not-shrink",
        "phase": "enforce",
        "match": { "valueChanges": { "pointers": ["/partitions"], "kinds": ["modify"] } },
        "prove": { "obligation": "non-destructive", "when": "new >= old" },
        "onFailure": { "effect": "challenge", "code": "partition-count-shrunk" }
      }
    ]
  }
}`

// mergePolicyOwnership proves `ownership` from a controlling owner fact via
// require-review on failure — the D-016 owner-rule shape. With empty facts (E5
// not wired) the predicate errors -> require-review unsatisfied -> REVIEW.
const mergePolicyOwnership = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "MergePolicy",
  "metadata": { "name": "topic-safety" },
  "spec": {
    "entries": { "topic-registry": { "mode": "document", "root": "", "identity": { "pointer": "/metadata/name" } } },
    "rules": [
      {
        "name": "topic-owner-must-approve",
        "phase": "enforce",
        "match": { "files": { "paths": ["topics/orders.yaml"] } },
        "prove": { "obligation": "ownership", "when": "facts.owner.team.state == 'resolved'" },
        "onFailure": { "effect": "require-review", "code": "ownership-approval-missing" }
      }
    ]
  }
}`

const rulesetBindingNonDestructive = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "RulesetBinding",
  "bindings": [
    { "class": "topic-registry", "environment": "prod", "packs": ["topic-safety"], "risk": { "threshold": 10 }, "require": ["non-destructive"] }
  ]
}`

const mergePolicyRequireReviewOnFalse = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "MergePolicy",
  "metadata": { "name": "topic-safety" },
  "spec": {
    "entries": { "topic-registry": { "mode": "document", "root": "", "identity": { "pointer": "/metadata/name" } } },
    "rules": [
      {
        "name": "human-approval-required",
        "phase": "enforce",
        "match": { "files": { "paths": ["topics/orders.yaml"] } },
        "prove": { "obligation": "ownership", "when": "false" },
        "onFailure": { "effect": "require-review", "code": "needs-human-approval" }
      }
    ]
  }
}`

const rulesetBindingOwnership = `{
  "apiVersion": "assent.dev/v1alpha1",
  "kind": "RulesetBinding",
  "bindings": [
    { "class": "topic-registry", "environment": "prod", "packs": ["topic-safety"], "risk": { "threshold": 10 }, "require": ["ownership"] }
  ]
}`

// configOwnerFailOpen configures the controlling `owner` provider failure:open —
// a posture ValidateProviderPosture must REJECT (a controlling fact must fail closed).
const configOwnerFailOpen = `apiVersion: assent.dev/v1alpha1
kind: Config
environments:
  - name: prod
    match: { paths: ["topics/**"] }
classes:
  - name: topic-registry
    match: { paths: ["topics/**.yaml"] }
providers:
  owner:
    type: builtin/gitlab-groups
    failure: open
`

func runArgs(extra ...string) []string {
	return append([]string{
		"--project", "42", "--mr", "7", "--bot-author", "assent-bot",
		"--subject", "file:topics/orders.yaml",
	}, extra...)
}

func env(token string) func(string) string {
	return func(k string) string {
		if k == "GITLAB_TOKEN" {
			return token
		}
		return ""
	}
}

// REVIEW path: a partitions DECREASE (12 -> 6) fails the assert → challenge →
// REVIEW. Exactly one thread is created; NO approve/merge; the emitted record is
// schema-valid (validated inside orchestrate before any write).
func TestRunReviewCreatesThread(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 6\n"

	var out bytes.Buffer
	code := runRun(runArgs(), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if f.discussionsPosted != 1 {
		t.Errorf("threads posted = %d, want 1", f.discussionsPosted)
	}
	if f.approvals != 0 || f.merges != 0 {
		t.Errorf("REVIEW must not approve/merge: approvals=%d merges=%d", f.approvals, f.merges)
	}
	if !strings.Contains(out.String(), "\"decision\":\"REVIEW\"") {
		t.Errorf("expected REVIEW record in output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "\"mergeResultDigest\":null") {
		t.Errorf("expected null mergeResultDigest + capabilityGap:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "capabilityGap") {
		t.Errorf("expected capabilityGap in record:\n%s", out.String())
	}
	// The posted marker must carry the GOVERNED-SUBJECT entryRef (the --subject
	// entryRef, not a branch) and a content-derived occurrence = sha256(head content).
	if !strings.Contains(f.lastThreadBody, `"entryRef":"file:topics/orders.yaml"`) {
		t.Errorf("marker entryRef not the governed subject:\n%s", f.lastThreadBody)
	}
	wantOcc := "sha256:" + sha256Hex([]byte(f.headFile))
	if !strings.Contains(f.lastThreadBody, `"occurrence":"`+wantOcc+`"`) {
		t.Errorf("marker occurrence not sha256(head content) %q:\n%s", wantOcc, f.lastThreadBody)
	}
}

// APPROVE + --arm: a partitions INCREASE (12 -> 24) proves the obligation →
// APPROVE. With --arm, approve + a SHA-pinned merge are called; the merge is
// pinned to the source SHA.
func TestRunApproveArmedMerges(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if f.approvals != 1 {
		t.Errorf("approvals = %d, want 1", f.approvals)
	}
	if f.merges != 1 {
		t.Errorf("merges = %d, want 1", f.merges)
	}
	if f.lastMergeSHA != "srcSHA" {
		t.Errorf("merge?sha = %q, want srcSHA (pinned source)", f.lastMergeSHA)
	}
	if !strings.Contains(out.String(), "\"decision\":\"APPROVE\"") {
		t.Errorf("expected APPROVE record:\n%s", out.String())
	}
	if f.discussionsPosted != 0 {
		t.Errorf("APPROVE must not post a thread: %d", f.discussionsPosted)
	}
}

// APPROVE with forge probe refusing: same increase → APPROVE decision, but
// forge-probed ArmEligible=false → Reconcile refuses (ErrArmingRefused) → ZERO
// approve/merge writes. --arm alone cannot override (E4-S06 / D-034).
func TestRunApproveUnarmedNoWrite(t *testing.T) {
	f := newFakeGitLab(t)
	f.projectJSON = fakeForgeIneligibleProjectJSON
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (advisory)\n%s", code, out.String())
	}
	if f.approvals != 0 || f.merges != 0 {
		t.Errorf("forge-ineligible APPROVE must not write even with --arm: approvals=%d merges=%d", f.approvals, f.merges)
	}
	if !strings.Contains(out.String(), "advisory-only") {
		t.Errorf("expected advisory-only summary:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "\"decision\":\"APPROVE\"") {
		t.Errorf("expected APPROVE record even when advisory:\n%s", out.String())
	}
}

// GUARD 3 (updated E4-S06): without forge-resolved ApprovalEvidence a
// require-review obligation stays UNSATISFIED → REVIEW even when forge arming
// would allow writes. No approve/merge on REVIEW.
func TestRunNilEvidenceRequiresReview(t *testing.T) {
	f := newFakeGitLab(t)
	f.mergePolicy = mergePolicyOwnership
	f.rulesetBinding = rulesetBindingOwnership
	f.approvalEligible = false
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n" // a change the ownership rule's match.files selects

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "\"decision\":\"REVIEW\"") {
		t.Errorf("nil ApprovalEvidence must leave require-review UNSATISFIED → REVIEW:\n%s", out.String())
	}
	if f.approvals != 0 || f.merges != 0 {
		t.Errorf("unsatisfied require-review must not approve/merge even with --arm: approvals=%d merges=%d", f.approvals, f.merges)
	}
}

// The provider posture wiring (ADR-0017 §6): a controlling owner provider
// configured failure:open is REJECTED at load (--config), failing CLOSED with no
// forge writes — a controlling fact may not fail open.
func TestRunControllingFactFailOpenRejected(t *testing.T) {
	f := newFakeGitLab(t)
	f.mergePolicy = mergePolicyOwnership
	f.rulesetBinding = rulesetBindingOwnership
	f.config = configOwnerFailOpen
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	code := runRun(runArgs("--arm", "--config", ".assent/config.yaml"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code == 0 {
		t.Fatalf("a controlling failure:open provider must fail closed (non-zero), got 0\n%s", out.String())
	}
	if f.approvals != 0 || f.merges != 0 || f.discussionsPosted != 0 {
		t.Errorf("rejected posture must produce NO writes: a=%d m=%d d=%d", f.approvals, f.merges, f.discussionsPosted)
	}
	if !strings.Contains(out.String(), "posture") {
		t.Errorf("expected a provider-posture error:\n%s", out.String())
	}
}

// N1: an uncovered required obligation (the challenge policy proves only
// `non-destructive`, but the binding requires `ownership`) yields a coverage
// finding with an empty subject inside the engine. orchestrate sanitizes it to a
// per-obligation sentinel so the produced DecisionRecord PASSES schema validation
// (validateRecord runs before any write — a code-0 run proves the record validated)
// → REVIEW. This exercises the N1 sentinel path end-to-end through the serializer.
func TestRunUncoveredObligationRecordValidates(t *testing.T) {
	f := newFakeGitLab(t)
	f.rulesetBinding = rulesetBindingOwnership // require [ownership]; challenge policy proves only non-destructive
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n" // a grow: the non-destructive rule proves, so only ownership is uncovered

	var out bytes.Buffer
	code := runRun(runArgs(), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (a valid REVIEW record) \n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "\"decision\":\"REVIEW\"") {
		t.Errorf("an uncovered required obligation must fail safe to REVIEW:\n%s", out.String())
	}
	// The sanitized per-obligation sentinel subject appears in the validated record.
	if !strings.Contains(out.String(), `"subject":"obligation:ownership"`) {
		t.Errorf("expected the N1 sentinel subject for the uncovered obligation:\n%s", out.String())
	}
	if f.approvals != 0 || f.merges != 0 {
		t.Errorf("uncovered obligation must not approve/merge: approvals=%d merges=%d", f.approvals, f.merges)
	}
}

func TestRunMissingFlags(t *testing.T) {
	var out bytes.Buffer
	// Missing --project (and --subject).
	code := runRun([]string{"--mr", "7", "--bot-author", "b"}, env("tok"), fixedClock(), &out, &out, newFakeGitLab(t).factory())
	if code == 0 {
		t.Fatalf("missing --project should be non-zero, got 0\n%s", out.String())
	}
}

func TestRunMissingToken(t *testing.T) {
	var out bytes.Buffer
	code := runRun(runArgs(), env(""), fixedClock(), &out, &out, newFakeGitLab(t).factory())
	if code == 0 {
		t.Fatal("missing GITLAB_TOKEN should be non-zero")
	}
	if !strings.Contains(out.String(), "GITLAB_TOKEN") {
		t.Errorf("expected token error message:\n%s", out.String())
	}
}

func TestRunUnparseablePolicyNoWrite(t *testing.T) {
	f := newFakeGitLab(t)
	f.mergePolicy = "\tthis: is: not: valid: yaml: ["
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code == 0 {
		t.Fatalf("unloadable merge-policy should fail (non-zero), got 0\n%s", out.String())
	}
	if f.approvals != 0 || f.merges != 0 || f.discussionsPosted != 0 {
		t.Errorf("unloadable policy must produce NO writes: a=%d m=%d d=%d", f.approvals, f.merges, f.discussionsPosted)
	}
}

func TestRunEmitToFile(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 6\n"

	emitPath := t.TempDir() + "/record.json"
	var out bytes.Buffer
	code := runRun(runArgs("--emit", emitPath), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	data, err := os.ReadFile(emitPath) // #nosec G304 -- test-controlled temp path, not user input.
	if err != nil {
		t.Fatalf("read emitted record: %v", err)
	}
	if !strings.Contains(string(data), "\"kind\":\"DecisionRecord\"") {
		t.Errorf("emitted file is not a DecisionRecord:\n%s", data)
	}
	// stdout carried only the summary (not the record) when --emit is a file.
	if strings.Contains(out.String(), "\"kind\":\"DecisionRecord\"") {
		t.Errorf("record should NOT be on stdout when --emit is a file:\n%s", out.String())
	}
}
