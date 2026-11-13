package github

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// snapshot_failures_test.go pins the fail-closed edges of the PR reads: a
// broken or ambiguous read is an error, never an empty/trusted result.

func TestMrInfoFromPRRefusesUnpinnedOrAnonymousRepos(t *testing.T) {
	repoOf := func(name string) *struct {
		FullName string `json:"full_name"`
	} {
		return &struct {
			FullName string `json:"full_name"`
		}{FullName: name}
	}
	full := func() ghPR {
		var pr ghPR
		pr.Number, pr.SHA = 7, "src"
		pr.Base.Ref, pr.Base.SHA, pr.Base.Repo = "main", "tgt", repoOf(baseRepo)
		pr.Head.Ref, pr.Head.SHA, pr.Head.Repo = "feature", "src", repoOf(baseRepo)
		return pr
	}
	tests := []struct {
		name    string
		mutate  func(*ghPR)
		wantErr string
	}{
		{name: "complete", mutate: func(*ghPR) {}},
		{name: "no head sha", mutate: func(p *ghPR) { p.Head.SHA = "" }, wantErr: "missing head/base refs"},
		{name: "no head ref", mutate: func(p *ghPR) { p.Head.Ref = "" }, wantErr: "missing head/base refs"},
		{name: "no base ref", mutate: func(p *ghPR) { p.Base.Ref = "" }, wantErr: "missing head/base refs"},
		{name: "null head repo", mutate: func(p *ghPR) { p.Head.Repo = nil }, wantErr: "fork identity unprovable"},
		{name: "null base repo", mutate: func(p *ghPR) { p.Base.Repo = nil }, wantErr: "fork identity unprovable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := full()
			tt.mutate(&pr)
			info, err := mrInfoFromPR(baseRepo, pr)
			if tt.wantErr == "" {
				if err != nil || info.SourceSHA != "src" || info.TargetSHA != "tgt" || info.ForkMR {
					t.Fatalf("got %+v, %v", info, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestPRReadFailuresFailClosed(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "unexpected status", status: 418, body: `{}`, wantErr: "unexpected status 418"},
		{name: "undecodable", status: 200, body: `nope`, wantErr: "decode PR"},
		{name: "unpinned metadata", status: 200, body: `{"number":7}`, wantErr: "missing head/base refs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newServer(t, replyWith(tt.status, tt.body))
			if _, err := c.GetMR(baseRepo, "7"); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("GetMR err = %v, want containing %q", err, tt.wantErr)
			}
			if _, err := c.Snapshot(baseRepo, "7"); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Snapshot err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
	c, _ := newServer(t, unexpectedEndpoint)
	if _, err := c.GetMR("bad", "7"); err == nil {
		t.Fatal("GetMR: unparseable project must fail")
	}
	if _, err := c.Snapshot("bad", "7"); err == nil {
		t.Fatal("Snapshot: unparseable project must fail")
	}
}

func TestSnapshotFailsClosedOnLaterReads(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{name: "merge ref read fails", path: "/git/ref/", wantErr: "get merge ref"},
		{name: "changed files read fails", path: "/pulls/7/files", wantErr: "get PR files"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				p := r.URL.Path
				switch {
				case strings.Contains(p, tt.path):
					w.WriteHeader(http.StatusTeapot)
				case strings.HasSuffix(p, "/pulls/7"):
					_, _ = io.WriteString(w, sameRepoPR)
				case strings.Contains(p, "/git/ref/"):
					_, _ = io.WriteString(w, `{"object":{"sha":"mergeSHA"}}`)
				default:
					unexpectedEndpoint(w, r)
				}
			})
			if _, err := c.Snapshot(baseRepo, "7"); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestFileReadsFailClosedWithoutPin(t *testing.T) {
	c, _ := newServer(t, replyWith(http.StatusNotFound, `{}`))
	if _, err := c.FileAtBase(baseRepo, "7", "a.yaml"); err == nil {
		t.Fatal("FileAtBase must fail when the PR pin cannot be read")
	}
	if _, err := c.FileAtHead(baseRepo, "7", "a.yaml"); err == nil {
		t.Fatal("FileAtHead must fail when the PR pin cannot be read")
	}
	if _, err := c.FileAtRef("bad", "a.yaml", "ref"); err == nil {
		t.Fatal("FileAtRef must reject an unparseable project")
	}
}
