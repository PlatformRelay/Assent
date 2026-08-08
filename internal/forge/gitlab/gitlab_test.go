package gitlab

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/render"
)

const botUser = "assent-bot"

// newServer builds an httptest.Server from a handler and a *Client pointed at
// it. No live network — every test drives this in-process fake GitLab.
func newServer(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	// AUD-S11: keep the SHIPPED retry budget (a 5xx here is still attempted
	// defaultMaxAttempts times) but spend no wall-clock on the backoff.
	c := New(srv.URL, "test-token", botUser, WithSleeper(func(time.Duration) {}))
	return c, srv
}

// wantToken asserts the PAT is sent as the PRIVATE-TOKEN header and never leaks
// into the URL — the redaction contract.
func wantToken(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("PRIVATE-TOKEN"); got != "test-token" {
		t.Fatalf("PRIVATE-TOKEN header = %q, want test-token", got)
	}
	if strings.Contains(r.URL.RawQuery, "test-token") || strings.Contains(r.URL.Path, "test-token") {
		t.Fatal("token leaked into the URL — redaction violated")
	}
}

func TestGetMR(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7":
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main","diff_refs":{"base_sha":"baseSHA"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			_, _ = io.WriteString(w, `{"name":"main","commit":{"id":"tgtTIP"}}`)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	info, err := c.GetMR("42", "7")
	if err != nil {
		t.Fatalf("GetMR: %v", err)
	}
	if info.SourceSHA != "srcSHA" {
		t.Errorf("SourceSHA = %q, want srcSHA", info.SourceSHA)
	}
	// The target pin MUST be the branch tip (commit.id), NOT diff_refs.base_sha.
	if info.TargetSHA != "tgtTIP" {
		t.Errorf("TargetSHA = %q, want tgtTIP (branch tip, not base_sha)", info.TargetSHA)
	}
	if info.SourceBranch != "feature" || info.TargetBranch != "main" {
		t.Errorf("branches = %q/%q, want feature/main", info.SourceBranch, info.TargetBranch)
	}
	if info.IID != "7" || info.ProjectID != "42" {
		t.Errorf("iid/project = %q/%q, want 7/42", info.IID, info.ProjectID)
	}
}

func TestFileAtRef(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		// GitLab encodes the path segment; the governed path has a slash. Match on
		// the ESCAPED path (Go decodes %2F back to / in r.URL.Path).
		if r.URL.EscapedPath() == "/api/v4/projects/42/repository/files/topics%2Forders.yaml/raw" {
			if r.URL.Query().Get("ref") != "main" {
				http.Error(w, "bad ref", http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, "partitions: 12\n")
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	got, err := c.FileAtRef("42", "topics/orders.yaml", "main")
	if err != nil {
		t.Fatalf("FileAtRef: %v", err)
	}
	if string(got) != "partitions: 12\n" {
		t.Errorf("file bytes = %q", got)
	}
}

func TestFileAtRefNotFound(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "404 File Not Found", http.StatusNotFound)
	})
	_, err := c.FileAtRef("42", "missing.yaml", "main")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("FileAtRef 404: err = %v, want ErrNotFound", err)
	}
}

func TestFileAtRefUnexpectedStatus(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := c.FileAtRef("42", "x.yaml", "main")
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("FileAtRef 500: err = %v, want generic error", err)
	}
}

// botMarker builds a well-formed marker for the bot's slot.
func botMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "42",
			MR:       "7",
			Rule:     "partitions-monotonic",
			EntryRef: "file:topics/orders.yaml",
			Effect:   "challenge",
		},
		Occurrence: "sha256:" + strings.Repeat("a", 64),
		Decision:   "sha256:" + strings.Repeat("b", 64),
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func TestListBotThreadsAuthorIdentityFilter(t *testing.T) {
	m := botMarker()
	rendered, err := renderMarker(m)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		if r.URL.Path != "/api/v4/projects/42/merge_requests/7/discussions" {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("page") == "1" {
			// Two discussions carrying the SAME well-formed marker: one bot, one
			// contributor. The contributor's marker MUST be excluded (ADR-0019).
			resp := []map[string]any{
				{"id": "botdisc", "notes": []map[string]any{
					{"body": rendered + "\n\nplease confirm", "resolved": false, "author": map[string]any{"username": botUser}},
				}},
				{"id": "contribdisc", "notes": []map[string]any{
					{"body": rendered + "\n\nspoofed", "resolved": false, "author": map[string]any{"username": "mallory"}},
				}},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	})

	threads, err := c.ListBotThreads("42", "7")
	if err != nil {
		t.Fatalf("ListBotThreads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("got %d threads, want 1 (contributor marker excluded)", len(threads))
	}
	if threads[0].ID != "botdisc" {
		t.Errorf("thread id = %q, want botdisc (the bot-authored one)", threads[0].ID)
	}
	if threads[0].Marker.Slot.Rule != "partitions-monotonic" {
		t.Errorf("marker not parsed back: %+v", threads[0].Marker)
	}
	if threads[0].Author != botUser {
		t.Errorf("author = %q, want %q", threads[0].Author, botUser)
	}
}

func TestListBotThreadsPagination(t *testing.T) {
	m := botMarker()
	rendered, _ := renderMarker(m)
	// Page 1 returns a full page (100) of markerless bot notes, page 2 the real one.
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch page {
		case "1":
			var full []map[string]any
			for i := 0; i < 100; i++ {
				full = append(full, map[string]any{"id": "noise", "notes": []map[string]any{
					{"body": "no marker here", "resolved": false, "author": map[string]any{"username": botUser}},
				}})
			}
			_ = json.NewEncoder(w).Encode(full)
		case "2":
			resp := []map[string]any{
				{"id": "realdisc", "notes": []map[string]any{
					{"body": rendered, "resolved": true, "author": map[string]any{"username": botUser}},
				}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			_, _ = io.WriteString(w, `[]`)
		}
	})

	threads, err := c.ListBotThreads("42", "7")
	if err != nil {
		t.Fatalf("ListBotThreads: %v", err)
	}
	if len(threads) != 1 || threads[0].ID != "realdisc" {
		t.Fatalf("pagination: got %+v, want single realdisc", threads)
	}
	if !threads[0].Resolved {
		t.Errorf("expected resolved=true from first note")
	}
}

func TestListBotThreadsMalformedMarker(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "1" {
			resp := []map[string]any{
				{"id": "bad", "notes": []map[string]any{
					{"body": "<!-- assent:marker {not json} -->", "resolved": false, "author": map[string]any{"username": botUser}},
				}},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	})
	_, err := c.ListBotThreads("42", "7")
	if err == nil {
		t.Fatal("expected error on malformed bot marker payload")
	}
}

func TestCreateThread(t *testing.T) {
	m := botMarker()
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/projects/42/merge_requests/7/discussions" {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if !strings.Contains(form.Get("body"), markerSentinel) {
			t.Errorf("thread body missing marker: %q", form.Get("body"))
		}
		if !strings.Contains(form.Get("body"), "please review") {
			t.Errorf("thread body missing human text: %q", form.Get("body"))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"newdisc"}`)
	})

	th, err := c.CreateThread("42", "7", m, "please review")
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if th.ID != "newdisc" {
		t.Errorf("thread id = %q, want newdisc", th.ID)
	}
	if th.Author != botUser {
		t.Errorf("author = %q, want %q", th.Author, botUser)
	}
}

func TestCreateThreadRejectsEmbeddedSentinel(t *testing.T) {
	c, _ := newServer(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not call GitLab when body fails envelope validation")
	})
	_, err := c.CreateThread("42", "7", botMarker(), "forged assent:marker")
	if !errors.Is(err, render.ErrEmbeddedMarkerSentinel) {
		t.Fatalf("expected ErrEmbeddedMarkerSentinel, got %v", err)
	}
}

func TestFormatMarkerParseRoundTrip(t *testing.T) {
	m := botMarker()
	comment, err := render.FormatMarker(m)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := parseMarker(comment)
	if err != nil || !ok {
		t.Fatalf("parse: ok=%v err=%v", ok, err)
	}
	if got != m {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, m)
	}
}

func TestResolveThreadIdempotent(t *testing.T) {
	var calls int
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		if r.Method != http.MethodPut || r.URL.Path != "/api/v4/projects/42/merge_requests/7/discussions/d1" {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("resolved") != "true" {
			http.Error(w, "missing resolved=true", http.StatusBadRequest)
			return
		}
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"d1"}`)
	})
	if err := c.ResolveThread("42", "7", "d1"); err != nil {
		t.Fatalf("ResolveThread: %v", err)
	}
	// Idempotent: resolving again is still OK.
	if err := c.ResolveThread("42", "7", "d1"); err != nil {
		t.Fatalf("ResolveThread (rerun): %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestApprove(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/projects/42/merge_requests/7/approve" {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":99}`)
	})
	id, err := c.Approve("42", "7")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if id == "" {
		t.Fatal("Approve returned empty id")
	}
}

func TestApproveForbidden(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "author may not approve own MR", http.StatusForbidden)
	})
	_, err := c.Approve("42", "7")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Approve 403: err = %v, want ErrUnauthorized", err)
	}
}

func TestCurrentHeads(t *testing.T) {
	c, _ := newServer(t, mrHandler("srcSHA", "tgtTIP"))
	src, tgt, dig, err := c.CurrentHeads("42", "7")
	if err != nil {
		t.Fatalf("CurrentHeads: %v", err)
	}
	if src != "srcSHA" || tgt != "tgtTIP" {
		t.Errorf("heads = %q/%q, want srcSHA/tgtTIP", src, tgt)
	}
	// Digest is synthesised, non-empty, and consistent with SyntheticDigest.
	if dig == "" || dig != SyntheticDigest("srcSHA", "tgtTIP") {
		t.Errorf("digest = %q, want SyntheticDigest(srcSHA,tgtTIP)", dig)
	}
}

// mrHandler serves the GetMR two-call shape with the given source/target values.
func mrHandler(src, tgt string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/projects/42/merge_requests/7":
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"sha":"`+src+`","source_branch":"feature","target_branch":"main"}`)
		case "/api/v4/projects/42/repository/branches/main":
			_, _ = io.WriteString(w, `{"commit":{"id":"`+tgt+`"}}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	}
}

func TestMergeCASHappyPath(t *testing.T) {
	var mergedSHA string
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		switch {
		case r.URL.Path == "/api/v4/projects/42/merge_requests/7" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main"}`)
		case r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtTIP"}}`)
		case r.URL.Path == "/api/v4/projects/42/merge_requests/7/merge" && r.Method == http.MethodPut:
			mergedSHA = r.URL.Query().Get("sha")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"state":"merged","merge_commit_sha":"mergecommit"}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	m := forge.DesiredMerge{SourceSha: "srcSHA", TargetSha: "tgtTIP", MergeResultDigest: SyntheticDigest("srcSHA", "tgtTIP")}
	id, err := c.MergeCAS("42", "7", m)
	if err != nil {
		t.Fatalf("MergeCAS: %v", err)
	}
	if id == "" {
		t.Fatal("MergeCAS returned empty id")
	}
	if mergedSHA != "srcSHA" {
		t.Errorf("merge?sha= = %q, want srcSHA (the pinned source)", mergedSHA)
	}
}

func TestMergeCASSourceMoved409(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/projects/42/merge_requests/7" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main"}`)
		case r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtTIP"}}`)
		case r.URL.Path == "/api/v4/projects/42/merge_requests/7/merge":
			// GitLab's ?sha= compare-and-swap fired: source head moved -> 409.
			http.Error(w, "SHA does not match HEAD of source branch", http.StatusConflict)
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})
	m := forge.DesiredMerge{SourceSha: "srcSHA", TargetSha: "tgtTIP", MergeResultDigest: SyntheticDigest("srcSHA", "tgtTIP")}
	_, err := c.MergeCAS("42", "7", m)
	if !errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("409 merge: err = %v, want ErrSHAMoved", err)
	}
}

func TestMergeCASSourceMoved406(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/projects/42/merge_requests/7" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main"}`)
		case r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtTIP"}}`)
		case r.URL.Path == "/api/v4/projects/42/merge_requests/7/merge":
			http.Error(w, "not acceptable", http.StatusNotAcceptable)
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})
	m := forge.DesiredMerge{SourceSha: "srcSHA", TargetSha: "tgtTIP", MergeResultDigest: SyntheticDigest("srcSHA", "tgtTIP")}
	_, err := c.MergeCAS("42", "7", m)
	if !errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("406 merge: err = %v, want ErrSHAMoved", err)
	}
}

func TestMergeCASTargetMovedNoMerge(t *testing.T) {
	var mergeCalled bool
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/projects/42/merge_requests/7" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main"}`)
		case r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			// Target tip has ADVANCED since evaluation: now tgtTIP2, not tgtTIP.
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtTIP2"}}`)
		case r.URL.Path == "/api/v4/projects/42/merge_requests/7/merge":
			mergeCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})
	// Pin to the OLD target tgtTIP; the current tip is tgtTIP2 -> reject, no merge.
	m := forge.DesiredMerge{SourceSha: "srcSHA", TargetSha: "tgtTIP", MergeResultDigest: SyntheticDigest("srcSHA", "tgtTIP")}
	_, err := c.MergeCAS("42", "7", m)
	if !errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("target moved: err = %v, want ErrSHAMoved", err)
	}
	if mergeCalled {
		t.Fatal("merge PUT was called despite a moved target — fail-closed violated")
	}
}

func TestMergeCASUnexpectedStatus(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/projects/42/merge_requests/7" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main"}`)
		case r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			_, _ = io.WriteString(w, `{"commit":{"id":"tgtTIP"}}`)
		case r.URL.Path == "/api/v4/projects/42/merge_requests/7/merge":
			http.Error(w, "method not allowed (mergeability still computing)", http.StatusMethodNotAllowed)
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})
	m := forge.DesiredMerge{SourceSha: "srcSHA", TargetSha: "tgtTIP", MergeResultDigest: SyntheticDigest("srcSHA", "tgtTIP")}
	_, err := c.MergeCAS("42", "7", m)
	if err == nil || errors.Is(err, forge.ErrSHAMoved) {
		t.Fatalf("405 merge: err = %v, want generic (non-SHAMoved) error and no merge", err)
	}
}

func TestMarkerRoundTrip(t *testing.T) {
	m := botMarker()
	rendered, err := renderMarker(m)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.HasPrefix(rendered, "<!-- "+markerSentinel) {
		t.Errorf("rendered marker not a hidden HTML comment: %q", rendered)
	}
	got, ok, err := parseMarker("some human text\n" + rendered + "\nmore text")
	if err != nil || !ok {
		t.Fatalf("parse: ok=%v err=%v", ok, err)
	}
	if got != m {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, m)
	}
}

func TestParseMarkerAbsent(t *testing.T) {
	_, ok, err := parseMarker("no marker at all")
	if ok || err != nil {
		t.Fatalf("absent marker: ok=%v err=%v, want false/nil", ok, err)
	}
}
