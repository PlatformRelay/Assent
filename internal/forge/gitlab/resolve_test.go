package gitlab

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/schemas"
)

const (
	resolveProject = "42"
	resolveMR      = "7"
	resolveSubject = "topic-registry:orders.events.v1"
	resolveAuthor  = "alice"
	resolveBot     = botUser
)

func resolvePins() (src, tgt, dig string) {
	src, tgt = "srcSHA", "tgtTIP"
	dig = SyntheticDigest(src, tgt)
	return src, tgt, dig
}

func baseResolveRequest() forge.ResolveRequest {
	src, tgt, dig := resolvePins()
	return forge.ResolveRequest{
		Project:           resolveProject,
		MR:                resolveMR,
		Subject:           resolveSubject,
		SourceSha:         src,
		TargetSha:         tgt,
		MergeResultDigest: dig,
		MRAuthor:          resolveAuthor,
	}
}

func premiumEligibleHandler(t *testing.T, authorApproved bool) http.HandlerFunc {
	t.Helper()
	src, tgt, _ := resolvePins()
	approvedBy := `[{"user":{"id":202,"username":"bob"}}]`
	if authorApproved {
		approvedBy = `[{"user":{"id":101,"username":"alice"}},{"user":{"id":202,"username":"bob"}}]`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7":
			_, _ = w.Write([]byte(`{
				"iid":7,"project_id":42,"sha":"` + src + `","source_branch":"feature","target_branch":"main",
				"author":{"id":101,"username":"alice"}
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			_, _ = w.Write([]byte(`{"commit":{"id":"` + tgt + `"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7/approval_rules":
			_, _ = w.Write([]byte(`[{"id":1,"name":"security-review","rule_type":"regular","approvals_required":1}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7/approval_state":
			_, _ = w.Write([]byte(`{"rules":[{
				"id":1,"name":"security-review","rule_type":"regular","approvals_required":1,"approved":true,
				"eligible_approvers":[{"id":101,"username":"alice"},{"id":202,"username":"bob"}],
				"approved_by":` + approvedBy + `
			}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/user":
			_, _ = w.Write([]byte(`{"id":999,"username":"` + resolveBot + `"}`))
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}
}

func freeTierHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	src, tgt, _ := resolvePins()
	return func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		switch r.URL.Path {
		case "/api/v4/projects/42/merge_requests/7":
			_, _ = w.Write([]byte(`{"iid":7,"project_id":42,"sha":"` + src + `","source_branch":"f","target_branch":"main","author":{"id":101,"username":"alice"}}`))
		case "/api/v4/projects/42/repository/branches/main":
			_, _ = w.Write([]byte(`{"commit":{"id":"` + tgt + `"}}`))
		case "/api/v4/projects/42/merge_requests/7/approval_rules":
			http.Error(w, "not found", http.StatusNotFound)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}
}

func validateEvidenceSchema(t *testing.T, doc []byte) {
	t.Helper()
	if err := schemas.ValidateApprovalEvidence(doc); err != nil {
		t.Fatalf("ApprovalEvidence schema: %v", err)
	}
}

// REQ-E4-S03-01: eligible approval → schema-valid evidence with matching sha pins.
func TestResolveEligibleApproval(t *testing.T) {
	c, _ := newServer(t, premiumEligibleHandler(t, false))
	c.observedAt = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }

	got, err := c.Resolve(baseResolveRequest())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := got.WellFormed(); err != nil {
		t.Fatalf("WellFormed: %v", err)
	}
	if !got.HasEvidence() || got.HasGap() {
		t.Fatalf("want evidence-only result, got %+v", got)
	}

	doc, err := c.evidenceDocument(got.Evidence, baseResolveRequest(), approvalStateRule{
		Name:     "security-review",
		RuleType: "regular",
	}, "srcSHA", "tgtTIP", SyntheticDigest("srcSHA", "tgtTIP"))
	if err != nil {
		t.Fatalf("evidenceDocument: %v", err)
	}
	validateEvidenceSchema(t, doc)

	ev := got.Evidence
	if ev.VerifyingCapability != "approval-rules-api" {
		t.Errorf("VerifyingCapability = %q, want approval-rules-api", ev.VerifyingCapability)
	}
	if ev.Pins.SourceSha != "srcSHA" {
		t.Errorf("Pins.SourceSha = %q, want srcSHA", ev.Pins.SourceSha)
	}
	if ev.Expired {
		t.Error("eligible fixture evidence must not be expired")
	}
	if len(ev.ApprovedBy) == 0 {
		t.Fatal("ApprovedBy must be populated")
	}
}

// REQ-E4-S03-02: MR-author approval excluded even if forge accepted it.
func TestResolveExcludesAuthorApproval(t *testing.T) {
	c, _ := newServer(t, premiumEligibleHandler(t, true))
	c.observedAt = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }

	got, err := c.Resolve(baseResolveRequest())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !got.HasEvidence() {
		t.Fatalf("want evidence after excluding author-only noise, got %+v", got)
	}
	ev := got.Evidence
	for _, id := range ev.Eligibility {
		if id == "101" || id == resolveAuthor {
			t.Errorf("eligibility must exclude MR author, got id %q", id)
		}
	}
	for _, a := range ev.ApprovedBy {
		if a.Username == resolveAuthor || a.ID == "101" {
			t.Errorf("approvedBy must exclude MR author, got %+v", a)
		}
	}
	if len(ev.ApprovedBy) != 1 || ev.ApprovedBy[0].Username != "bob" {
		t.Errorf("ApprovedBy = %+v, want bob only", ev.ApprovedBy)
	}
}

// REQ-E4-S03-03: Free tier / missing approval-rules → explicit capability gap.
func TestResolveTierGapFailClosed(t *testing.T) {
	c, _ := newServer(t, freeTierHandler(t))

	got, err := c.Resolve(baseResolveRequest())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := got.WellFormed(); err != nil {
		t.Fatalf("WellFormed: %v", err)
	}
	if got.HasEvidence() || !got.HasGap() {
		t.Fatalf("want gap-only result, got evidence=%v gap=%v", got.Evidence, got.Gap)
	}
	if got.Gap.Reason != forge.GapFreeTierRequireReview {
		t.Errorf("Gap.Reason = %q, want %q", got.Gap.Reason, forge.GapFreeTierRequireReview)
	}
	if got.Gap.Subject != resolveSubject {
		t.Errorf("Gap.Subject = %q, want %q", got.Gap.Subject, resolveSubject)
	}
}

// REQ-E4-S03-04: stale sha pins → evidence rejected (re-evaluate, not APPROVE).
func TestResolveShaMismatch(t *testing.T) {
	c, _ := newServer(t, premiumEligibleHandler(t, false))
	c.observedAt = func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) }

	req := baseResolveRequest()
	req.SourceSha = "stale-source-sha"

	got, err := c.Resolve(req)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.HasGap() {
		t.Fatalf("sha mismatch must not fabricate capability gap, got %+v", got.Gap)
	}
	if !got.HasEvidence() {
		t.Fatal("want stale evidence marker, not absence")
	}
	if !got.Evidence.Expired {
		t.Error("stale pin mismatch must mark evidence expired (re-evaluate, not APPROVE)")
	}
	src, _, _ := resolvePins()
	if got.Evidence.Pins.SourceSha != req.SourceSha {
		t.Errorf("Pins.SourceSha = %q, want stale request pin %q", got.Evidence.Pins.SourceSha, req.SourceSha)
	}
	if got.Evidence.Pins.SourceSha == src {
		t.Error("stale evidence must not carry the live MR source sha")
	}
}

func TestResolvePATRedaction(t *testing.T) {
	const secret = "resolve-redaction-token" //nolint:gosec // REQ-E4-S02-04 PAT redaction contract
	srv := httptestNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("PRIVATE-TOKEN"); got != secret {
			t.Fatalf("PRIVATE-TOKEN = %q", got)
		}
		if strings.Contains(r.URL.String(), secret) {
			t.Fatal("token leaked into URL")
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := New(srv.URL, secret, resolveBot)
	_, err := c.Resolve(baseResolveRequest())
	if err == nil {
		t.Fatal("want error from failing server")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("PAT leaked into error: %v", err)
	}
}
