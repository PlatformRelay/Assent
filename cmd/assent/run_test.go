package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
	mergePolicy          string // MergePolicy served from the TARGET ref.
	rulesetBinding       string // RulesetBinding served from the TARGET ref.
	config               string // optional Config served from the TARGET ref.
	baseFile, headFile   string // governed-file content at target/source ref.
	governedPath         string

	// recorded writes
	discussionsPosted int
	approvals         int
	merges            int
	lastMergeSHA      string
	lastThreadBody    string
}

func newFakeGitLab(t *testing.T) *fakeGitLab {
	t.Helper()
	f := &fakeGitLab{
		sourceSHA:      "srcSHA",
		targetTip:      "tgtTIP",
		sourceBranch:   "feature",
		target:         "main",
		governedPath:   "topics/orders.yaml",
		mergePolicy:    mergePolicyChallenge,
		rulesetBinding: rulesetBindingNonDestructive,
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGitLab) handle(w http.ResponseWriter, r *http.Request) {
	p := r.URL.EscapedPath()
	switch {
	case p == "/api/v4/projects/42/merge_requests/7" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{
			"iid": 7, "project_id": 42, "sha": f.sourceSHA,
			"source_branch": f.sourceBranch, "target_branch": f.target,
		})
	case p == "/api/v4/projects/42/repository/branches/main" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{"commit": map[string]any{"id": f.targetTip}})
	case strings.HasPrefix(p, "/api/v4/projects/42/repository/files/") && strings.HasSuffix(p, "/raw"):
		f.serveFile(w, r, p)
	case p == "/api/v4/projects/42/merge_requests/7/discussions" && r.Method == http.MethodGet:
		_, _ = w.Write([]byte(`[]`)) // no pre-existing bot threads.
	case p == "/api/v4/projects/42/merge_requests/7/discussions" && r.Method == http.MethodPost:
		f.discussionsPosted++
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		f.lastThreadBody = form.Get("body")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"disc-new"}`))
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

// serveFile routes the three loaded documents (MergePolicy, RulesetBinding, the
// optional Config) — all from the TARGET ref (ADR-0015 §1) — plus the governed
// file (base from target, head from source). Routing is by path substring: the
// three document paths carry distinctive names, the governed path does not.
func (f *fakeGitLab) serveFile(w http.ResponseWriter, r *http.Request, p string) {
	ref := r.URL.Query().Get("ref")
	switch {
	case strings.Contains(p, "merge-policy"):
		f.serveFromTarget(w, ref, f.mergePolicy)
		return
	case strings.Contains(p, "ruleset-binding"):
		f.serveFromTarget(w, ref, f.rulesetBinding)
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

// factory builds a real *gitlab.Client pointed at the fake server — the exact
// production adapter, driven end-to-end over HTTP without a live network.
func (f *fakeGitLab) factory() func(string, string, string) forgePort {
	return func(_, token, botAuthor string) forgePort {
		return gitlab.New(f.srv.URL, token, botAuthor)
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

// APPROVE WITHOUT --arm: same increase → APPROVE, but no --arm → ArmEligible
// false → Reconcile refuses (ErrArmingRefused) → ZERO approve/merge writes. The
// run still exits 0 (advisory) and emits the schema-valid record.
func TestRunApproveUnarmedNoWrite(t *testing.T) {
	f := newFakeGitLab(t)
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	code := runRun(runArgs(), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (advisory)\n%s", code, out.String())
	}
	if f.approvals != 0 || f.merges != 0 {
		t.Errorf("unarmed APPROVE must not write: approvals=%d merges=%d", f.approvals, f.merges)
	}
	if !strings.Contains(out.String(), "advisory-only") {
		t.Errorf("expected advisory-only summary:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "\"decision\":\"APPROVE\"") {
		t.Errorf("expected APPROVE record even when advisory:\n%s", out.String())
	}
}

// GUARD 3: the live path injects NIL ApprovalEvidence, so a require-review
// obligation is UNSATISFIED (nil-fail-safe, never default-satisfied) → REVIEW. The
// owner fact is unresolved (facts empty until E5), the predicate errors, and the
// require-review finding stands. No approve/merge.
func TestRunNilEvidenceRequiresReview(t *testing.T) {
	f := newFakeGitLab(t)
	f.mergePolicy = mergePolicyOwnership
	f.rulesetBinding = rulesetBindingOwnership
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
