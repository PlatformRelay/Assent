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
	policy               string // policy.yaml content served from the TARGET ref.
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
		sourceSHA:    "srcSHA",
		targetTip:    "tgtTIP",
		sourceBranch: "feature",
		target:       "main",
		governedPath: "topics/orders.yaml",
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

func (f *fakeGitLab) serveFile(w http.ResponseWriter, r *http.Request, p string) {
	ref := r.URL.Query().Get("ref")
	// The policy path (loaded from the TARGET ref).
	if strings.Contains(p, "policy.yaml") {
		if ref != f.target {
			http.Error(w, "policy MUST load from the target ref, got "+ref, http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(f.policy))
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

const policyOrders = `subject: "file:topics/orders.yaml"
require: [partitions-not-decreased]
rules:
  - name: partitions-monotonic
    obligation: partitions-not-decreased
    when: "int(new) >= int(old)"
    onFailure: { effect: challenge, code: PARTITIONS_DECREASED }
`

func runArgs(extra ...string) []string {
	return append([]string{
		"--project", "42", "--mr", "7", "--bot-author", "assent-bot",
		"--policy", ".assent/policy.yaml",
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
	f.policy = policyOrders
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
	// The posted marker must carry the GOVERNED-SUBJECT entryRef (not a branch)
	// and a content-derived occurrence = sha256(head content) — the grammar's
	// stable-across-tool/policy-bump occurrence, so a rerun recognises the thread.
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
	f.policy = policyOrders
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
	f.policy = policyOrders
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

func TestRunMissingFlags(t *testing.T) {
	var out bytes.Buffer
	// Missing --project.
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
	f.policy = "\tthis: is: not: valid: yaml: ["
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code == 0 {
		t.Fatalf("unparseable policy should fail (non-zero), got 0\n%s", out.String())
	}
	if f.approvals != 0 || f.merges != 0 || f.discussionsPosted != 0 {
		t.Errorf("unparseable policy must produce NO writes: a=%d m=%d d=%d", f.approvals, f.merges, f.discussionsPosted)
	}
}

func TestRunEmitToFile(t *testing.T) {
	f := newFakeGitLab(t)
	f.policy = policyOrders
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
