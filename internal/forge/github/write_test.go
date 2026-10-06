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
	"github.com/PlatformRelay/assent/internal/render"
)

// threadMarker is the fixture marker for finding threads. EntryRef is
// file-shaped: the GitHub review thread anchors on the governed FILE
// (commit_id + path), which only a file:<path> entryRef can express — a
// non-file marker fails closed in CreateThread.
func threadMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  baseRepo,
			MR:       "7",
			Rule:     "assent/policy-missing",
			EntryRef: "file:topics/orders.yaml",
			Effect:   "require-review",
		},
		Occurrence: "sha256:occurrence",
		Decision:   "sha256:decision",
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

// summaryMarker is the fixture marker for the summary-comment slot.
func summaryMarker() forge.Marker {
	m := threadMarker()
	m.Artifact.Kind = "summary-comment"
	return m
}

// envelopeBody renders a marker into the envelope form a listing row carries.
func envelopeBody(t *testing.T, m forge.Marker, body string) string {
	t.Helper()
	full, err := render.Envelope(m, body)
	if err != nil {
		t.Fatalf("render envelope fixture: %v", err)
	}
	return full
}

// commentRow builds one PR review/issue comment listing row.
func commentRow(id int, login, body string) string {
	bodyJSON, _ := json.Marshal(body)
	return fmt.Sprintf(`{"id":%d,"node_id":"PRRC_node%d","body":%s,"user":{"login":%q}}`,
		id, id, bodyJSON, login)
}

// listingPage builds a paginated listing body from rows.
func listingPage(rows ...string) string {
	out := "["
	for i, r := range rows {
		if i > 0 {
			out += ","
		}
		out += r
	}
	return out + "]"
}

// unexpectedEndpoint is the harness's 500 for a route no test in this file
// planned to serve.
func unexpectedEndpoint(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
}

// serveThreadsQueryOpen answers the reviewThreads listing query the bot-thread
// listing consults (dossier §4) with every named comment unresolved — the
// handlers whose thread states are not the subject of the test. The query body
// is already consumed by the caller.
func serveThreadsQueryOpen(w http.ResponseWriter, query string, ids ...int) {
	if !strings.Contains(query, "reviewThreads") {
		http.Error(w, "unexpected graphql operation", http.StatusInternalServerError)
		return
	}
	nodes := make([]string, 0, len(ids))
	for _, id := range ids {
		nodes = append(nodes, fmt.Sprintf(`{"id":"PRRC_node%d","isResolved":false,"comments":{"nodes":[{"databaseId":%d}]}}`, id, id))
	}
	_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[`+strings.Join(nodes, ",")+`]}}}}}`)
}

// TestMarkerOfParsesEnvelope proves the marker round-trip: the
// render.MarkerSentinel JSON payload decodes back to the four frozen concepts,
// a markerless body is (false, nil), and a malformed payload is an error.
func TestMarkerOfParsesEnvelope(t *testing.T) {
	m := threadMarker()
	got, ok, err := markerOf(envelopeBody(t, m, "a finding"))
	if err != nil {
		t.Fatalf("markerOf: %v", err)
	}
	if !ok {
		t.Fatal("marker not found in envelope body")
	}
	if got != m {
		t.Errorf("marker round-trip mismatch: got %+v want %+v", got, m)
	}

	if _, ok, err := markerOf("just a human comment"); err != nil || ok {
		t.Errorf("markerOf on a markerless body = (%v, %v), want (false, nil)", ok, err)
	}

	if _, _, err := markerOf("<!-- assent:marker {broken} -->"); err == nil {
		t.Error("malformed marker payload must error, got nil")
	}
}

// TestMarkerSpoofAndMalformed proves the two listing filter axes (ADR-0019 /
// REQ-E10-S10-02, RELI-06): a contributor's well-formed marker is INVISIBLE
// (and never even parsed — no warning); the bot's own malformed marker is
// skipped WITH A WARNING; the well-formed own marker becomes the thread.
func TestMarkerSpoofAndMalformed(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/comments":
			page := pageParam(t, r)
			if page > 1 {
				_, _ = io.WriteString(w, "[]")
				return
			}
			body := listingPage(
				commentRow(601, "octocat", envelopeBody(t, threadMarker(), "spoofed")),   // contributor: invisible
				commentRow(602, botUser, "<!-- assent:marker {broken} -->"),              // own: malformed
				commentRow(603, botUser, envelopeBody(t, threadMarker(), "the finding")), // own: valid
				commentRow(604, botUser, "a bot comment with no marker"),                 // bot: no marker
			)
			_, _ = io.WriteString(w, body)
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			var gqlReq struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&gqlReq); err != nil {
				t.Fatalf("decode graphql request: %v", err)
			}
			serveThreadsQueryOpen(w, gqlReq.Query, 601, 602, 603, 604)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	threads, err := c.ListBotThreads(baseRepo, "7")
	if err != nil {
		t.Fatalf("ListBotThreads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("threads = %#v, want exactly the one well-formed bot thread", threads)
	}
	if threads[0].ID != "comment/603" || threads[0].Author != botUser {
		t.Errorf("thread = %+v, want comment/603 authored by %q", threads[0], botUser)
	}

	// The contributor's marker was never even parsed: invisible, and silent.
	for _, w := range c.Warnings() {
		if strings.Contains(w, "601") {
			t.Errorf("a contributor artifact must be invisible without a warning: %q", w)
		}
	}
	// The bot's malformed marker is skipped WITH a warning naming the artifact.
	found := false
	for _, w := range c.Warnings() {
		if strings.Contains(w, "comment/602") && strings.Contains(w, "malformed marker payload") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v, want the malformed own-marker skip warning", c.Warnings())
	}
}

// TestListBotThreads404FailsClosed proves S00 Q4 on the listing: a
// permission-denied or missing listing wraps forge.ErrUnauthorized — never an
// empty list (absence is not permission failure).
// TestListBotThreadsUnauthorizedFailsClosed proves S00 Q4 on the listing: a
// permission-denied or missing listing wraps forge.ErrUnauthorized — never an
// empty list (absence is not permission failure).
func TestListBotThreadsUnauthorizedFailsClosed(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden} {
		c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/comments") {
				http.Error(w, "denied", status)
				return
			}
			unexpectedEndpoint(w, r)
		})
		threads, err := c.ListBotThreads(baseRepo, "7")
		if err == nil {
			t.Fatalf("status %d must fail closed, got threads %#v", status, threads)
		}
		if !errors.Is(err, forge.ErrUnauthorized) {
			t.Errorf("error must wrap forge.ErrUnauthorized, got %v", err)
		}
	}
}

// TestListBotThreadsPaginationCap proves the listing cap is FAIL-CLOSED
// (AUD-S10): a paginator that never shortens errors at the cap — never a
// silent partial thread list.
func TestListBotThreadsPaginationCap(t *testing.T) {
	requests := 0
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/comments":
			requests++
			// A page that never shortens: 100 bot rows with no markers.
			_, _ = io.WriteString(w, fullBotRows())
		default:
			unexpectedEndpoint(w, r)
		}
	})

	_, err := c.ListBotThreads(baseRepo, "7")
	if err == nil {
		t.Fatal("pagination cap must fail closed, got nil error")
	}
	if requests != maxListPages {
		t.Fatalf("paginator must stop at exactly %d pages, made %d requests", maxListPages, requests)
	}
	if !strings.Contains(err.Error(), "pagination cap") {
		t.Errorf("error must name the pagination cap, got %v", err)
	}
}

// fullBotRows builds one full page of bot-authored comments.
func fullBotRows() string {
	var rows []string
	for i := 0; i < listPerPage; i++ {
		rows = append(rows, commentRow(1000+i, botUser, "no marker here"))
	}
	return listingPage(rows...)
}

// TestListBotNotesFiltersByAuthor — same filter as threads; a contributor's
// well-formed summary marker is invisible, so UpsertComment would post fresh.
func TestListBotNotesFiltersByAuthor(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/issues/7/comments":
			page := pageParam(t, r)
			if page > 1 {
				_, _ = io.WriteString(w, "[]")
				return
			}
			body := "[" +
				commentRow(501, "octocat", envelopeBody(t, summaryMarker(), "spoofed summary")) + "," +
				commentRow(502, botUser, "a bot note with no marker") + "," +
				commentRow(503, botUser, envelopeBody(t, summaryMarker(), "the summary")) +
				"]"
			_, _ = io.WriteString(w, body)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	notes, err := c.ListBotNotes(baseRepo, "7")
	if err != nil {
		t.Fatalf("ListBotNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != "issue-comment/503" {
		t.Fatalf("notes = %#v, want exactly issue-comment/503", notes)
	}
	if notes[0].Marker.Artifact.Kind != "summary-comment" {
		t.Errorf("note marker kind = %q, want summary-comment", notes[0].Marker.Artifact.Kind)
	}
}

// TestUpsertCommentCreatesWhenAbsent — POST /repos/{repo}/issues/{n}/comments
// with the envelope body; expect 201; never retried.
func TestUpsertCommentCreatesWhenAbsent(t *testing.T) {
	var postedBody string
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/issues/7/comments":
			_, _ = io.WriteString(w, "[]")
		case r.Method == http.MethodPost && r.URL.Path == "/repos/octo-org/base-repo/issues/7/comments":
			var posted struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&posted)
			postedBody = posted.Body
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":501,"node_id":"IC_node1"}`)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	note, err := c.UpsertComment(baseRepo, "7", summaryMarker(), "the summary")
	if err != nil {
		t.Fatalf("UpsertComment: %v", err)
	}
	if note.ID != "issue-comment/501" {
		t.Errorf("note ID = %q, want issue-comment/501", note.ID)
	}
	if !strings.Contains(postedBody, render.MarkerSentinel) {
		t.Errorf("posted body carries no marker envelope: %q", postedBody)
	}
}

// TestUpsertCommentEditsInPlace proves P3-E5 step 3: an existing bot summary
// note is edited in place (PATCH /repos/{repo}/issues/comments/{id}), never a
// second summary posted.
func TestUpsertCommentEditsInPlace(t *testing.T) {
	m := summaryMarker()
	var patched bool
	var postedToCreate bool
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/issues/7/comments":
			_, _ = io.WriteString(w, listingPage(
				commentRow(501, botUser, envelopeBody(t, m, "old summary")),
				commentRow(502, botUser, envelopeBody(t, threadMarker(), "not a summary")),
			))
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/octo-org/base-repo/issues/comments/501":
			patched = true
			_, _ = io.WriteString(w, `{"id":501,"user":{"login":"assent-bot"}}`)
		case r.Method == http.MethodPost:
			postedToCreate = true
			unexpectedEndpoint(w, r)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	note, err := c.UpsertComment(baseRepo, "7", m, "new summary")
	if err != nil {
		t.Fatalf("UpsertComment: %v", err)
	}
	if !patched {
		t.Fatal("the existing summary note must be edited in place (PATCH)")
	}
	if postedToCreate {
		t.Error("a second summary must never be posted (create)")
	}
	if note.ID != "issue-comment/501" {
		t.Errorf("note ID = %q, want issue-comment/501", note.ID)
	}
}

// TestUpsertCommentRejectsNonSummary proves the guard on the marker kind.
func TestUpsertCommentRejectsNonSummary(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		unexpectedEndpoint(w, r)
	})
	_, err := c.UpsertComment(baseRepo, "7", threadMarker(), "body")
	if !errors.Is(err, forge.ErrInvalidSummaryMarker) {
		t.Errorf("error = %v, want forge.ErrInvalidSummaryMarker", err)
	}
}

// TestCreateThreadAndResolveRoundTrip proves the full thread lifecycle: the
// REST creation returns "comment/<id>", and ResolveThread maps that comment to
// its THREAD's GraphQL node id (a PullRequestReviewThread, served DISTINCT
// from the comment's own node id — the reviewThreads read is the mapping the
// dossier's resolution path rides) and resolves THAT — no REST re-listing
// needed (dossier §4).
func TestCreateThreadAndResolveRoundTrip(t *testing.T) {
	var gqlThreadID string
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
			// CreateThread reads the PINNED head via mrPinned — serve the PR
			// the evaluation read reported.
			_, _ = io.WriteString(w, sameRepoPR)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/comments":
			// Live GitHub refuses a body-only create with 422: commit_id and
			// path are required, and the shape this POST must carry is the
			// pinned head + the governed subject's file path (subject_type
			// "file" makes it a file-level thread — no line anchor).
			var posted struct {
				Body        string `json:"body"`
				CommitID    string `json:"commit_id"`
				Path        string `json:"path"`
				SubjectType string `json:"subject_type"`
			}
			_ = json.NewDecoder(r.Body).Decode(&posted)
			if !strings.Contains(posted.Body, render.MarkerSentinel) {
				t.Errorf("created thread body carries no marker envelope: %q", posted.Body)
			}
			if posted.CommitID != "srcSHA" {
				t.Errorf("create commit_id = %q, want the PINNED source SHA the adapter's own MR read reported", posted.CommitID)
			}
			if posted.Path != "topics/orders.yaml" {
				t.Errorf("create path = %q, want the governed subject (Slot.EntryRef minus the file: prefix)", posted.Path)
			}
			if posted.SubjectType != "file" {
				t.Errorf("create subject_type = %q, want file (file-level thread, no line anchor)", posted.SubjectType)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":401,"node_id":"PRRC_node1","body":"x","user":{"login":"assent-bot"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			var req struct {
				Query     string         `json:"query"`
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode graphql request: %v", err)
			}
			if strings.Contains(req.Query, "reviewThreads") {
				// The reviewThreads read maps the comment's databaseId to its
				// THREAD node id — a different object from the comment's own
				// node id.
				serveThreadsQueryOpen(w, req.Query, 401)
				return
			}
			if !strings.Contains(req.Query, "resolveReviewThread") {
				t.Errorf("graphql query = %q, want the resolveReviewThread mutation", req.Query)
			}
			threadID, _ := req.Variables["threadId"].(string)
			gqlThreadID = threadID
			if threadID == "PRRC_node401" {
				_, _ = io.WriteString(w, `{"data":{"resolveReviewThread":{"thread":{"id":"PRRC_thread401","isResolved":true}}}}`)
				return
			}
			http.Error(w, "unexpected thread id", http.StatusInternalServerError)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	created, err := c.CreateThread(baseRepo, "7", threadMarker(), "the finding body")
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if created.ID != "comment/401" {
		t.Errorf("thread ID = %q, want comment/401", created.ID)
	}

	if err := c.ResolveThread(baseRepo, "7", created.ID); err != nil {
		t.Fatalf("ResolveThread (no REST re-listing): %v", err)
	}
	if gqlThreadID != "PRRC_node401" {
		t.Errorf("GraphQL threadId = %q, want the THREAD node id the reviewThreads read reports for comment 401", gqlThreadID)
	}
}

// TestResolveThreadIdempotentAlreadyResolved proves the idempotence: the
// forge's "already resolved" rejection IS success for this verb.
func TestResolveThreadAlreadyResolved(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/comments":
			_, _ = io.WriteString(w, listingPage(
				commentRow(401, botUser, envelopeBody(t, threadMarker(), "the finding")),
			))
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			var req struct {
				Query     string         `json:"query"`
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode graphql request: %v", err)
			}
			switch {
			case strings.Contains(req.Query, "reviewThreads"):
				serveThreadsQueryOpen(w, req.Query, 401)
			default:
				_, _ = io.WriteString(w, `{"data":null,"errors":[{"message":"Thread is already resolved."}]}`)
			}
		default:
			unexpectedEndpoint(w, r)
		}
	})

	if _, err := c.ListBotThreads(baseRepo, "7"); err != nil {
		t.Fatalf("ListBotThreads: %v", err)
	}
	if err := c.ResolveThread(baseRepo, "7", "comment/401"); err != nil {
		t.Errorf("resolving an already-resolved thread must be idempotent success, got %v", err)
	}
}

// TestResolveThreadNotConfirmedFailsClosed proves the response contract: the
// mutation answering without isResolved:true is an error, never success.
func TestResolveThreadNotConfirmedFailsClosed(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/comments"):
			_, _ = io.WriteString(w, listingPage(
				commentRow(401, botUser, envelopeBody(t, threadMarker(), "the finding")),
			))
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			var req struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode graphql request: %v", err)
			}
			if strings.Contains(req.Query, "reviewThreads") {
				serveThreadsQueryOpen(w, req.Query, 401)
				return
			}
			_, _ = io.WriteString(w, `{"data":{"resolveReviewThread":{"thread":{"id":"x","isResolved":false}}}}`)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	if _, err := c.ListBotThreads(baseRepo, "7"); err != nil {
		t.Fatalf("ListBotThreads: %v", err)
	}
	if err := c.ResolveThread(baseRepo, "7", "comment/401"); err == nil {
		t.Fatal("an unconfirmed resolution must fail closed")
	}
}

// TestResolveThreadUnknownIDFailsClosed proves the unknown-id axis: a comment
// no review thread carries is an error, never a guess — the reviewThreads read
// may run (it is the id lookup), but the resolveReviewThread MUTATION must
// never be issued for an unaddressable comment, and an unparseable id must
// fail closed with no request at all.
func TestResolveThreadUnknownIDFailsClosed(t *testing.T) {
	resolveAttempts := 0
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/graphql":
			var req struct {
				Query string `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode graphql request: %v", err)
			}
			if strings.Contains(req.Query, "reviewThreads") {
				// A thread exists — but it carries comment 402, not the one
				// the resolve asks about.
				serveThreadsQueryOpen(w, req.Query, 402)
				return
			}
			resolveAttempts++
			_, _ = io.WriteString(w, `{"data":{"resolveReviewThread":{"thread":{"id":"x","isResolved":true}}}}`)
		default:
			unexpectedEndpoint(w, r)
		}
	})

	if err := c.ResolveThread(baseRepo, "7", "comment/999"); err == nil {
		t.Fatal("resolving a comment no review thread carries must fail closed")
	} else if !strings.Contains(err.Error(), "no review thread") {
		t.Errorf("the failure must name the mapping, got %v", err)
	}
	if err := c.ResolveThread(baseRepo, "7", "bogus"); err == nil {
		t.Fatal("an unparseable thread id must fail closed")
	}
	if got := resolveAttempts; got != 0 {
		t.Fatalf("no resolveReviewThread mutation may be issued for an unaddressable thread, got %d", got)
	}
}

// TestApproveRecordsReviewID proves the approval write: POST
// /repos/{repo}/pulls/{n}/reviews with {"event":"APPROVE","commit_id":<pin>} →
// 200 with a review id → "review/<id>"; a refusal is ErrUnauthorized. The
// commit_id is the PINNED source SHA the adapter's own MR read reported (the
// ADR-0015 §2 approve-then-merge pin — the approval lands on the evaluated
// head, never a moved one).
func TestApproveReturnsReviewID(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
			_, _ = io.WriteString(w, sameRepoPR)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/reviews":
			var posted map[string]string
			_ = json.NewDecoder(r.Body).Decode(&posted)
			if posted["event"] != "APPROVE" {
				t.Errorf("approve body event = %q, want APPROVE", posted["event"])
			}
			if posted["commit_id"] != "srcSHA" {
				t.Errorf("approve body commit_id = %q, want the PINNED source SHA the adapter's own MR read reported", posted["commit_id"])
			}
			_, _ = io.WriteString(w, `{"id":9001,"state":"APPROVED","user":{"login":"assent-bot"}}`)
			return
		default:
			unexpectedEndpoint(w, r)
		}
	})

	id, err := c.Approve(baseRepo, "7")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if id != "review/9001" {
		t.Errorf("approval id = %q, want review/9001", id)
	}
}

// TestApproveForbidden is the ErrUnauthorized case (author self-approval or a
// scope-less token).
func TestApproveUnauthorized(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
			_, _ = io.WriteString(w, sameRepoPR)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		default:
			unexpectedEndpoint(w, r)
		}
	})
	_, err := c.Approve(baseRepo, "7")
	if !errors.Is(err, forge.ErrUnauthorized) {
		t.Errorf("approve 403 must wrap ErrUnauthorized, got %v", err)
	}
}

// mergeHandler serves the read chain MergeCAS consumes (a fresh PR read plus
// the merge-ref read) and the merge PUT with the given status/body. It
// records the PUT request pins and the call count for the guard assertions.
type mergeHandler struct {
	mergeRefSHA string
	putStatus   int
	putResp     string

	putCalls  int
	putReqSHA string
	putMethod string
}

func (h *mergeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
		_, _ = io.WriteString(w, sameRepoPR)
	case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/git/ref/refs/pull/7/merge":
		_, _ = io.WriteString(w, `{"object":{"sha":"`+h.mergeRefSHA+`","type":"commit"}}`)
	case r.Method == http.MethodPut && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/merge":
		h.putCalls++
		var posted map[string]string
		_ = json.NewDecoder(r.Body).Decode(&posted)
		h.putReqSHA = posted["sha"]
		h.putMethod = posted["merge_method"]
		w.WriteHeader(h.putStatus)
		_, _ = io.WriteString(w, h.putResp)
	default:
		unexpectedEndpoint(w, r)
	}
}

// pinnedMerge is the pinned three-tuple a happy-path merge honours.
func pinnedMerge() forge.DesiredMerge {
	return forge.DesiredMerge{
		SourceSha:         "srcSHA",
		TargetSha:         "tgtTIP",
		MergeResultDigest: "mrgSHA",
	}
}

// TestMergeCASHappyPath — pre-check passes, PUT /pulls/{n}/merge with the sha
// pin merges, and the receipt id is the merge commit's SHA.
func TestMergeCASHappyPath(t *testing.T) {
	h := &mergeHandler{
		mergeRefSHA: "mrgSHA",
		putStatus:   http.StatusOK,
		putResp:     `{"merged":true,"merge_commit_sha":"mCommit"}`,
	}
	c, _ := newServer(t, h.ServeHTTP)

	mergeID, err := c.MergeCAS(baseRepo, "7", pinnedMerge())
	if err != nil {
		t.Fatalf("MergeCAS: %v", err)
	}
	if mergeID != "merge/mCommit" {
		t.Errorf("merge id = %q, want merge/mCommit", mergeID)
	}
	if h.putCalls != 1 {
		t.Fatalf("merge PUT calls = %d, want 1 (writes are never retried)", h.putCalls)
	}
	if h.putReqSHA != "srcSHA" || h.putMethod != "merge" {
		t.Errorf("merge PUT pin = sha %q method %q, want srcSHA/merge (GitHub's CAS guard)", h.putReqSHA, h.putMethod)
	}
}

// TestMergeCASRefusesMovedTarget proves the pre-write target-axis guard: a
// target tip that moved since evaluation is refused with ErrSHAMoved BEFORE
// the PUT — zero merge writes.
func TestMergeCASRefusesMovedTarget(t *testing.T) {
	h := &mergeHandler{
		mergeRefSHA: "mrgSHA",
		putStatus:   http.StatusOK,
		putResp:     `{"merged":true,"merge_commit_sha":"mCommit"}`,
	}
	c, _ := newServer(t, h.ServeHTTP)

	moved := pinnedMerge()
	moved.TargetSha = "tgtOLD"
	_, err := c.MergeCAS(baseRepo, "7", moved)
	if !errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("error = %v, want forge.ErrSHAMoved", err)
	}
	if h.putCalls != 0 {
		t.Errorf("moved target must record ZERO merges, made %d PUTs", h.putCalls)
	}
}

// TestMergeCASRefusesMovedDigest proves the digest axis of the pre-check: a
// merge ref re-minted since evaluation (pinned digest stale) refuses before
// the PUT.
func TestMergeCASRefusesMovedDigest(t *testing.T) {
	h := &mergeHandler{
		mergeRefSHA: "mrgSHA",
		putStatus:   http.StatusOK,
		putResp:     `{"merge_commit_sha":"mCommit"}`,
	}
	c, _ := newServer(t, h.ServeHTTP)

	stale := pinnedMerge()
	stale.MergeResultDigest = "staleDigest"
	_, err := c.MergeCAS(baseRepo, "7", stale)
	if !errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("moved digest must wrap ErrSHAMoved, got %v", err)
	}
	if h.putCalls != 0 {
		t.Errorf("moved digest must record ZERO merges, made %d PUTs", h.putCalls)
	}
}

// TestMergeCASConflictOnPut proves the atomic CAS guard: GitHub's own 409 on
// the sha pin (the head moved in the TOCTOU window) maps to ErrSHAMoved.
func TestMergeCASConflictOnPut(t *testing.T) {
	h := &mergeHandler{
		mergeRefSHA: "mrgSHA",
		putStatus:   http.StatusConflict,
		putResp:     `{"message":"pull request head sha does not match"}`,
	}
	c, _ := newServer(t, h.ServeHTTP)

	_, err := c.MergeCAS(baseRepo, "7", pinnedMerge())
	if !errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("merge 409 must wrap ErrSHAMoved, got %v", err)
	}
}

// TestMergeCASRefusalOn405 proves the 405 axis: the forge refusing the merge
// outright (not mergeable) is ErrSHAMoved with a distinct message — still no
// merge, never a generic transport error.
func TestMergeCASRefusalOn405(t *testing.T) {
	h := &mergeHandler{
		mergeRefSHA: "mrgSHA",
		putStatus:   http.StatusMethodNotAllowed,
		putResp:     `{"message":"not mergeable"}`,
	}
	c, _ := newServer(t, h.ServeHTTP)

	_, err := c.MergeCAS(baseRepo, "7", pinnedMerge())
	if !errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("merge 405 must wrap ErrSHAMoved, got %v", err)
	}
	if !strings.Contains(err.Error(), "not mergeable") {
		t.Errorf("405 message must distinguish the refusal, got %v", err)
	}
}
