package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// resolve_test.go — the review-chain logic (E10-S06 over S00 Q2 row 9 / OQ-34):
// the latest-non-dismissed-per-reviewer reduction, the author/bot exclusion
// axes, and the fail-closed gap Resolve ALWAYS returns in v1 (eligible set
// unprovable — OQ-34). The gap means require-review is UNSATISFIABLE on
// GitHub v1 — never evidence, never silent APPROVE.

// reviewRow builds one PR review listing row.
func reviewRow(id int64, login, state string) string {
	return fmt.Sprintf(`{"id":%d,"user":{"login":%q},"state":%q}`, id, login, state)
}

// decodeReviews parses a reviews page exactly the way the listing does.
func decodeReviews(t *testing.T, raw string) []ghReview {
	t.Helper()
	var rows []ghReview
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		t.Fatalf("decode reviews fixture: %v", err)
	}
	return rows
}

// TestResolveApprovalEvidence proves the review-chain reduction the Resolver
// evaluates (the logic OQ-34's promotion route consumes): latest
// non-dismissed APPROVED per reviewer, author/bot exclusion, and
// CHANGES_REQUESTED never counted.
func TestResolveApprovalEvidence(t *testing.T) {
	// The chain is chronological (GitHub returns reviews oldest-first): a
	// later review by the same login replaces the earlier state.
	rows := listingPage(
		reviewRow(1, "octocat", "APPROVED"),       // the PR author — excluded
		reviewRow(2, "alice", "APPROVED"),         // stands
		reviewRow(3, "bob", "CHANGES_REQUESTED"),  // bob
		reviewRow(3, "assent-bot", "APPROVED"),    // the configured bot — excluded
		reviewRow(4, "renovate[bot]", "APPROVED"), // an app bot — excluded
		reviewRow(5, "carol", "APPROVED"),         // stands
		reviewRow(6, "bob", "APPROVED"),           // supersedes his earlier request-for-changes
	)
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/reviews":
			_, _ = io.WriteString(w, rows)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	approvals, err := c.reviewChainApprovals(baseRepo, "7", "octocat")
	if err != nil {
		t.Fatalf("reviewChainApprovals: %v", err)
	}
	want := []string{"alice", "bob", "carol"}
	if len(approvals) != len(want) {
		t.Fatalf("approvals = %v, want %v", approvals, want)
	}
	for i := range want {
		if approvals[i] != want[i] {
			t.Errorf("approvals[%d] = %q, want %q", i, approvals[i], want[i])
		}
	}
}

// TestDismissedApprovalNotCounted proves the state-replacement rule: a
// DISMISSED event (and any newer non-approved review) supersedes an older
// approval — a dismissed approval is never counted.
func TestDismissedApprovalNotCounted(t *testing.T) {
	t.Run("dismissed approval not counted", func(t *testing.T) {
		rows := listingPage(
			reviewRow(1, "alice", "APPROVED"),
			reviewRow(2, "alice", "DISMISSED"),
		)
		got := latestNonDismissedApprovals(decodeReviews(t, rows), "octocat", botUser)
		if len(got) != 0 {
			t.Errorf("approvals = %#v, want none (dismissed)", got)
		}
	})

	t.Run("newer request-for-changes supersedes", func(t *testing.T) {
		rows := listingPage(
			reviewRow(1, "bob", "APPROVED"),
			reviewRow(2, "bob", "CHANGES_REQUESTED"),
		)
		got := latestNonDismissedApprovals(decodeReviews(t, rows), "octocat", botUser)
		if len(got) != 0 {
			t.Errorf("approvals = %#v, want none (CHANGES_REQUESTED supersedes)", got)
		}
	})

	t.Run("re-approval after dismissal counts", func(t *testing.T) {
		rows := listingPage(
			reviewRow(1, "carol", "DISMISSED"),
			reviewRow(2, "carol", "APPROVED"),
		)
		got := latestNonDismissedApprovals(decodeReviews(t, rows), "octocat", botUser)
		if len(got) != 1 || got[0] != "carol" {
			t.Errorf("approvals = %#v, want [carol] (latest state is APPROVED)", got)
		}
	})

	t.Run("empty chain", func(t *testing.T) {
		got := latestNonDismissedApprovals(nil, "octocat", botUser)
		if len(got) != 0 {
			t.Errorf("approvals = %#v, want none", got)
		}
	})
}

// TestUnprovableEligibilityFailsClosed proves OQ-34's fail-closed shape: the
// eligible-approver set is unprovable on GitHub, so Resolve returns the
// explicit gap — never evidence, never silent approval — while the review
// chain fetch itself still runs fail-closed.
func TestUnprovableEligibilityFailsClosed(t *testing.T) {
	reviewsFetched := 0
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/octo-org/base-repo/pulls/7/reviews":
			reviewsFetched++
			_, _ = io.WriteString(w, "["+reviewRow(1, "alice", "APPROVED")+"]")
		default:
			unexpectedEndpoint(w, r)
		}
	})

	req := forge.ResolveRequest{
		Project:   baseRepo,
		MR:        "7",
		Subject:   ".assent/config.yaml",
		SourceSha: "srcSHA",
		TargetSha: "tgtTIP",
		MRAuthor:  "octocat",
	}
	result, err := c.Resolve(req)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := result.WellFormed(); err != nil {
		t.Fatalf("result must be well-formed (exactly one of evidence/gap): %v", err)
	}
	if result.HasEvidence() {
		t.Fatal("v1 GitHub must NEVER mint evidence (eligible set unprovable, OQ-34)")
	}
	if result.Gap == nil || result.Gap.Reason != forge.GapEligibilityUnprovable {
		t.Fatalf("gap = %+v, want forge.GapEligibilityUnprovable", result.Gap)
	}
	if result.Gap.Subject != req.Subject {
		t.Errorf("gap subject = %q, want %q", result.Gap.Subject, req.Subject)
	}
	if reviewsFetched == 0 {
		t.Error("the review chain must actually be fetched (fail-closed transport), made 0 requests")
	}
}

// TestResolveExcludesAuthorAndBots — REQ-E10-S10-02's exclusion axes on the
// chain: the PR author, the configured bot identity, and app-bot logins are
// never approvals.
func TestResolveExcludesAuthorAndBots(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/reviews") {
			_, _ = io.WriteString(w, listingPage(
				reviewRow(1, "octocat", "APPROVED"),        // the PR author
				reviewRow(2, botUser, "APPROVED"),          // our own bot identity
				reviewRow(3, "other-app[bot]", "APPROVED"), // a GitHub app bot
				reviewRow(4, "real-person", "APPROVED"),
			))
			return
		}
		unexpectedEndpoint(w, r)
	})

	approvals, err := c.reviewChainApprovals(baseRepo, "7", "octocat")
	if err != nil {
		t.Fatalf("reviewChainApprovals: %v", err)
	}
	if len(approvals) != 1 || approvals[0] != "real-person" {
		t.Errorf("approvals = %v, want only [real-person] (author and bots excluded)", approvals)
	}
}

// TestResolveReviewsUnauthorizedFailsClosed — a review chain the caller
// cannot read is ErrUnauthorized, never "no approvals".
func TestResolveReviewsUnauthorizedFailsClosed(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/reviews") {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		unexpectedEndpoint(w, r)
	})
	req := forge.ResolveRequest{Project: baseRepo, MR: "7", Subject: ".assent/config.yaml", MRAuthor: "octocat"}
	_, err := c.Resolve(req)
	if !errors.Is(err, forge.ErrUnauthorized) {
		t.Errorf("error = %v, want forge.ErrUnauthorized (an unreadable chain is a hard error)", err)
	}
}
