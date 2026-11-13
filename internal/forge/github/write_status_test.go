package github

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// write_status_test.go pins the non-happy status/decode mappings of the Forge
// writes: each must fail closed with a distinguishable error, never succeed
// or report absence.

// prServer serves the PR read and the merge ref, and delegates every other
// route to other.
func prServer(t *testing.T, other http.HandlerFunc) *Client {
	t.Helper()
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls/7"):
			_, _ = io.WriteString(w, sameRepoPR)
		case r.Method == http.MethodGet && strings.Contains(r.URL.RawPath+r.URL.Path, "/git/ref/"):
			_, _ = io.WriteString(w, `{"object":{"sha":"mergeSHA"}}`)
		default:
			other(w, r)
		}
	})
	return c
}

func replyWith(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func TestApproveReviewResponseMapping(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		want       string
		wantTarget error
		wantID     string
	}{
		{name: "created", status: 201, body: `{"id":42}`, wantID: "review/42"},
		{name: "ok", status: 200, body: `{"id":43}`, wantID: "review/43"},
		{name: "forbidden is unauthorized", status: 403, body: `{}`, wantTarget: forge.ErrUnauthorized},
		{name: "unauthenticated is unauthorized", status: 401, body: `{}`, wantTarget: forge.ErrUnauthorized},
		{name: "unexpected status", status: 422, body: `{}`, want: "unexpected status 422"},
		{name: "undecodable body", status: 200, body: `not json`, want: "decode approval review"},
		{name: "missing review id", status: 200, body: `{}`, want: "no review id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := prServer(t, replyWith(tt.status, tt.body))
			id, err := c.Approve(baseRepo, "7")
			switch {
			case tt.wantID != "":
				if err != nil || id != tt.wantID {
					t.Fatalf("got %q, %v; want %q", id, err, tt.wantID)
				}
			case tt.wantTarget != nil:
				if !errors.Is(err, tt.wantTarget) {
					t.Fatalf("err = %v, want %v", err, tt.wantTarget)
				}
			default:
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("err = %v, want containing %q", err, tt.want)
				}
			}
			if tt.wantID == "" && id != "" {
				t.Fatalf("a failed approval must return no id, got %q", id)
			}
		})
	}
}

func TestApproveFailsWhenPinUnavailable(t *testing.T) {
	c, _ := newServer(t, replyWith(http.StatusNotFound, `{}`))
	if _, err := c.Approve(baseRepo, "7"); err == nil || !strings.Contains(err.Error(), "pin approval head") {
		t.Fatalf("err = %v, want the pin failure", err)
	}
	if _, err := c.Approve("not-a-repo", "7"); err == nil {
		t.Fatal("an unparseable project must fail before any request")
	}
}

func TestMergeCASRefusals(t *testing.T) {
	pinned := forge.DesiredMerge{SourceSha: "srcSHA", TargetSha: "tgtTIP", MergeResultDigest: "mergeSHA"}
	tests := []struct {
		name    string
		mutate  func(*forge.DesiredMerge)
		status  int
		body    string
		wantMsg string
		sha     bool // wants ErrSHAMoved
		wantID  string
	}{
		{name: "merged", status: 200, body: `{"merge_commit_sha":"abc"}`, wantID: "merge/abc"},
		{name: "target moved", mutate: func(m *forge.DesiredMerge) { m.TargetSha = "old" }, sha: true, wantMsg: "target tip moved"},
		{name: "source moved", mutate: func(m *forge.DesiredMerge) { m.SourceSha = "old" }, sha: true, wantMsg: "source head moved"},
		{name: "digest moved", mutate: func(m *forge.DesiredMerge) { m.MergeResultDigest = "old" }, sha: true, wantMsg: "merge result moved"},
		{name: "409 is a moved source", status: 409, body: `{}`, sha: true, wantMsg: "409"},
		{name: "405 is a refusal", status: 405, body: `{}`, sha: true, wantMsg: "405"},
		{name: "other status", status: 422, body: `{}`, wantMsg: "unexpected status 422"},
		{name: "undecodable body", status: 200, body: `nope`, wantMsg: "decode merge response"},
		{name: "no merge commit sha", status: 200, body: `{}`, wantMsg: "without a merge_commit_sha"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := pinned
			if tt.mutate != nil {
				tt.mutate(&m)
			}
			c := prServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut {
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				}
				replyWith(tt.status, tt.body)(w, r)
			})
			id, err := c.MergeCAS(baseRepo, "7", m)
			if tt.wantID != "" {
				if err != nil || id != tt.wantID {
					t.Fatalf("got %q, %v; want %q", id, err, tt.wantID)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantMsg)
			}
			if errors.Is(err, forge.ErrSHAMoved) != tt.sha {
				t.Fatalf("ErrSHAMoved = %v, want %v (err %v)", !tt.sha, tt.sha, err)
			}
			if id != "" {
				t.Fatalf("a refused merge must return no id, got %q", id)
			}
		})
	}
}

func TestMergeCASHeadReadFailureIsNotASHAMove(t *testing.T) {
	c, _ := newServer(t, replyWith(http.StatusNotFound, `{}`))
	_, err := c.MergeCAS(baseRepo, "7", forge.DesiredMerge{})
	if err == nil || errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("err = %v, want a read error that is not ErrSHAMoved", err)
	}
	if _, err := c.MergeCAS("bad", "7", forge.DesiredMerge{}); err == nil {
		t.Fatal("an unparseable project must fail")
	}
}

func TestCurrentHeadsMergeRefStatus(t *testing.T) {
	tests := []struct {
		name       string
		refStatus  int
		refBody    string
		wantDigest string
		wantErr    string
	}{
		{name: "readable", refStatus: 200, refBody: `{"object":{"sha":"m1"}}`, wantDigest: "m1"},
		{name: "absent ref leaves digest empty", refStatus: 404, refBody: `{}`},
		{name: "undecodable ref", refStatus: 200, refBody: `x`, wantErr: "decode merge ref"},
		{name: "ref without sha", refStatus: 200, refBody: `{"object":{}}`, wantErr: "no object sha"},
		{name: "unexpected status", refStatus: 403, refBody: `{}`, wantErr: "unexpected status 403"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/pulls/7") {
					_, _ = io.WriteString(w, sameRepoPR)
					return
				}
				replyWith(tt.refStatus, tt.refBody)(w, r)
			})
			src, tgt, digest, err := c.CurrentHeads(baseRepo, "7")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || src != "srcSHA" || tgt != "tgtTIP" || digest != tt.wantDigest {
				t.Fatalf("got %q %q %q %v", src, tgt, digest, err)
			}
		})
	}
}

func TestNoteWriteDecodeAndStatusFailures(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{name: "create unexpected status", status: 422, body: `{}`, wantMsg: "unexpected status 422"},
		{name: "create undecodable", status: 201, body: `nope`, wantMsg: "decode created comment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newServer(t, replyWith(tt.status, tt.body))
			_, err := c.createNote(baseRepo, "7", summaryMarker(), "body")
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantMsg)
			}
		})
	}
	c, _ := newServer(t, replyWith(http.StatusNotFound, `{}`))
	if _, err := c.editNote(baseRepo, "7", "issue-comment/9", summaryMarker(), "b"); err == nil || !strings.Contains(err.Error(), "unexpected status 404") {
		t.Fatalf("edit err = %v", err)
	}
	for _, bad := range []string{"bad", ""} {
		if _, err := c.createNote(bad, "7", summaryMarker(), "b"); err == nil {
			t.Fatalf("createNote(%q) must reject an unparseable project", bad)
		}
		if _, err := c.editNote(bad, "7", "issue-comment/9", summaryMarker(), "b"); err == nil {
			t.Fatalf("editNote(%q) must reject an unparseable project", bad)
		}
	}
}

func TestCreateThreadStatusFailures(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{name: "unexpected status", status: 422, body: `{}`, wantMsg: "unexpected status 422"},
		{name: "undecodable", status: 201, body: `nope`, wantMsg: "decode created thread"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := prServer(t, replyWith(tt.status, tt.body))
			_, err := c.CreateThread(baseRepo, "7", threadMarker(), "body")
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantMsg)
			}
		})
	}
	c, _ := newServer(t, replyWith(http.StatusNotFound, `{}`))
	if _, err := c.CreateThread(baseRepo, "7", threadMarker(), "b"); err == nil || !strings.Contains(err.Error(), "pin thread head") {
		t.Fatalf("err = %v, want the pin failure", err)
	}
	if _, err := c.CreateThread("bad", "7", threadMarker(), "b"); err == nil {
		t.Fatal("an unparseable project must fail")
	}
}

func TestListingFailuresFailClosed(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{name: "undecodable", status: 200, body: `nope`, wantMsg: "decode"},
		{name: "unexpected status", status: 418, body: `{}`, wantMsg: "unexpected status 418"},
		{name: "not found is unauthorized", status: 404, body: `{}`, wantMsg: "unauthorized"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newServer(t, replyWith(tt.status, tt.body))
			if _, err := c.ListBotNotes(baseRepo, "7"); err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("notes err = %v, want containing %q", err, tt.wantMsg)
			}
			if _, err := c.ListBotThreads(baseRepo, "7"); err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("threads err = %v, want containing %q", err, tt.wantMsg)
			}
		})
	}
	c, _ := newServer(t, unexpectedEndpoint)
	if _, err := c.ListBotNotes("bad", "7"); err == nil {
		t.Fatal("notes: unparseable project must fail")
	}
	if _, err := c.ListBotThreads("bad", "7"); err == nil {
		t.Fatal("threads: unparseable project must fail")
	}
}

func TestListBotNotesPaginationCapFailsClosed(t *testing.T) {
	// A paginator that always returns a full page of foreign comments must hit
	// the cap and error, never return a partial list.
	var rows []string
	for i := 0; i < listPerPage; i++ {
		rows = append(rows, commentRow(i+1, "someone-else", "hi"))
	}
	page := listingPage(rows...)
	c, _ := newServer(t, replyWith(http.StatusOK, page))
	if _, err := c.ListBotNotes(baseRepo, "7"); err == nil || !strings.Contains(err.Error(), "pagination cap") {
		t.Fatalf("err = %v, want the pagination-cap failure", err)
	}
}
