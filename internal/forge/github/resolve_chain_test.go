package github

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// resolve_chain_test.go covers the review-chain reduction the Resolve gap
// declaration rests on (E10-S06 / OQ-34): the latest-non-dismissed-per-reviewer
// reduction, author/bot exclusion, and the pagination-cap fail-closed shape.
// The VALUE is never minted into evidence (the gap is always returned), but
// the reduction must still be correct — it is what the OQ-34 promotion route
// would consume.

func TestReviewChainReduction(t *testing.T) {
	rows := "[" +
		reviewRow(1, "alice", "APPROVED") + "," +
		reviewRow(2, "bob", "CHANGES_REQUESTED") + "," +
		reviewRow(3, "alice", "APPROVED") + "," + // alice's latest is APPROVED
		reviewRow(4, "carol", "APPROVED") + "," +
		reviewRow(5, "carol", "DISMISSED") + "," + // carol's latest is DISMISSED: not counted
		reviewRow(6, "octocat", "APPROVED") + "," + // the PR author: excluded
		reviewRow(7, "dependabot[bot]", "APPROVED") + "]"
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octo-org/base-repo/pulls/7/reviews" {
			_, _ = io.WriteString(w, rows)
			return
		}
		_, _ = io.WriteString(w, sameRepoPR)
	})
	got, err := c.reviewChainApprovals("octo-org/base-repo", "7", "octocat")
	if err != nil {
		t.Fatalf("reviewChainApprovals: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("the reduction must keep only alice's latest undismissed approval, got %v", got)
	}
	if got[0] != "alice" {
		t.Fatalf("the surviving approver is %q, want alice", got[0])
	}
}

func TestReviewChainPaginationCapFailsClosed(t *testing.T) {
	requests := 0
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octo-org/base-repo/pulls/7/reviews" {
			requests++
			// A page that NEVER shortens: the cap must refuse, never spin or
			// silently truncate.
			page := make([]string, 0, listPerPage)
			for i := 0; i < listPerPage; i++ {
				page = append(page, reviewRow(int64(i+1), "alice", "APPROVED"))
			}
			_, _ = io.WriteString(w, "["+strings.Join(page, ",")+"]")
			return
		}
		_, _ = io.WriteString(w, sameRepoPR)
	})
	if _, err := c.reviewChainApprovals("octo-org/base-repo", "7", "octocat"); err == nil {
		t.Fatal("a never-shortening review pagination must fail closed at the cap")
	}
	if requests < maxListPages {
		t.Fatalf("the cap did not bind: %d requests, want >= %d", requests, maxListPages)
	}
}

func TestResolveAuthorFetchFailClosed(t *testing.T) {
	// Resolve with a request that carries no author: the PR read must fail
	// closed (never resolve against an unknown author), and the transport
	// error must propagate rather than degrade to the gap.
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, err := c.Resolve(forge.ResolveRequest{Project: "octo-org/base-repo", MR: "7", Subject: "topic"})
	if err == nil {
		t.Fatal("Resolve without a readable PR must error (a broken forge is never a silent gap)")
	}
}

func TestReviewRowHelper(t *testing.T) {
	row := reviewRow(3, "alice", "APPROVED")
	if !strings.Contains(row, `"login":"alice"`) || !strings.Contains(row, "APPROVED") {
		t.Fatalf("reviewRow = %s", row)
	}
}
