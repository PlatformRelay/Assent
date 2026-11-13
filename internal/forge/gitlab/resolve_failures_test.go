package gitlab

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
)

// override replaces the response for one path of the premium fixture; every
// other route keeps serving the healthy answer.
type override struct {
	path   string
	status int
	body   string
}

func resolveWithOverride(t *testing.T, ov override) http.HandlerFunc {
	healthy := premiumEligibleHandler(t, false)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == ov.path {
			w.WriteHeader(ov.status)
			_, _ = w.Write([]byte(ov.body))
			return
		}
		healthy(w, r)
	}
}

// Resolve must fail closed — an error, never fabricated evidence or a gap that
// reads as "no approvals" — when any read in the chain fails or is garbled.
func TestResolveFailsClosedOnReadFailures(t *testing.T) {
	const mrPath = "/api/v4/projects/42/merge_requests/7"
	tests := []struct {
		name    string
		ov      override
		wantErr string
	}{
		{"mr read unexpected status", override{mrPath, 418, `{}`}, "get MR"},
		{"mr read undecodable", override{mrPath, 200, `not json`}, "decode MR"},
		{"target branch unexpected status", override{"/api/v4/projects/42/repository/branches/main", 418, `{}`}, "get branch"},
		{"approval_state unexpected status", override{mrPath + "/approval_state", 418, `{}`}, "approval_state"},
		{"approval_state undecodable", override{mrPath + "/approval_state", 200, `[`}, "decode approval_state"},
		{"user read unexpected status", override{"/api/v4/user", 418, `{}`}, "get user"},
		{"user read undecodable", override{"/api/v4/user", 200, `nope`}, "decode user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newServer(t, resolveWithOverride(t, tt.ov))
			got, err := c.Resolve(baseResolveRequest())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
			if got.HasEvidence() || got.HasGap() {
				t.Fatalf("a failed read must yield an empty result, got %+v", got)
			}
		})
	}
}

func TestResolveWithoutPickableRuleIsAGap(t *testing.T) {
	// approval_state reports only rules that require zero approvals: there is
	// nothing to verify, which is a typed capability gap, not evidence.
	c, _ := newServer(t, resolveWithOverride(t, override{
		"/api/v4/projects/42/merge_requests/7/approval_state", 200,
		`{"rules":[{"id":1,"name":"informational","rule_type":"regular","approvals_required":0}]}`,
	}))
	got, err := c.Resolve(baseResolveRequest())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !got.HasGap() || got.HasEvidence() || got.Gap.Reason != forge.GapApprovalRulesUnavailable {
		t.Fatalf("want an approval-rules-unavailable gap, got %+v", got)
	}
}

func TestIsExcludedApprover(t *testing.T) {
	user := gitlabUser{ID: 5, Username: "carol"}
	tests := []struct {
		name           string
		authorID       int
		authorUsername string
		mrAuthor       string
		botID          int
		botUsername    string
		want           bool
	}{
		{name: "nobody excluded"},
		{name: "author by id", authorID: 5, want: true},
		{name: "mr author by username", mrAuthor: "carol", want: true},
		{name: "mr author by numeric id string", mrAuthor: "5", want: true},
		{name: "author by username", authorUsername: "carol", want: true},
		{name: "bot by id", botID: 5, want: true},
		{name: "bot by username", botUsername: "carol", want: true},
		{name: "unrelated identities", authorID: 6, authorUsername: "x", mrAuthor: "y", botID: 7, botUsername: "z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExcludedApprover(user, tt.authorID, tt.authorUsername, tt.mrAuthor, tt.botID, tt.botUsername)
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterApprovedDedupesAndRequiresEligibility(t *testing.T) {
	eligible := []gitlabUser{{ID: 2, Username: "bob"}, {ID: 1, Username: "ann"}}
	rows := []approvedRow{
		{User: gitlabUser{ID: 2, Username: "bob"}},
		{User: gitlabUser{ID: 2, Username: "bob"}},     // duplicate approval
		{User: gitlabUser{ID: 9, Username: "mallory"}}, // not eligible
		{User: gitlabUser{ID: 1, Username: "ann"}},
	}
	got := filterApproved(rows, eligible, 0, "", "", 0, "")
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("want ann,bob once each sorted by id, got %+v", got)
	}
}

func TestObservedNowUsesInjectedClock(t *testing.T) {
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	c, _ := newServer(t, unexpectedHandler)
	c.observedAt = func() time.Time { return fixed }
	if got := c.observedNow(); !got.Equal(fixed) {
		t.Fatalf("got %v, want %v", got, fixed)
	}
	c.observedAt = nil
	if got := c.observedNow(); got.Location() != time.UTC || time.Since(got) > time.Minute {
		t.Fatalf("default clock must be current UTC time, got %v", got)
	}
}

func TestEvidenceDocumentRejectsNil(t *testing.T) {
	c, _ := newServer(t, unexpectedHandler)
	if _, err := c.evidenceDocument(nil, forge.ResolveRequest{}, approvalStateRule{}, "", "", ""); err == nil {
		t.Fatal("nil evidence must be rejected")
	}
}

func unexpectedHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
}
