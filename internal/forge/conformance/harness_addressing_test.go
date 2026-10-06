package conformance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// harness_addressing_test.go exercises the GitLab httptest harness's FILE and
// FORK routes directly (E10-S02): the addressing cases drive them through the
// adapter, so a broken status mapping (404 for absent, 403 for refused, fork
// project-999 route) would otherwise only surface through a conformance case's
// own assertions. These unit tests pin the mapping itself.

func TestHarnessServeFileStatusMapping(t *testing.T) {
	h := newGitLabHarness("42", "7")
	h.sourceSHA = "src"
	h.targetSHA = "tgt"
	h.baseFile = []byte("base-bytes")
	h.baseSet = true
	h.headFile = []byte("head bytes")
	h.headSet = true

	srv := httptest.NewServer(http.HandlerFunc(h.handle))
	t.Cleanup(srv.Close)

	get := func(url string) int {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("PRIVATE-TOKEN", "test-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	if got := get(srv.URL + "/api/v4/projects/42/repository/files/topics%2Fx.yaml/raw?ref=src"); got != 200 {
		t.Fatalf("head side at pinned source SHA = %d, want 200", got)
	}
	if got := get(srv.URL + "/api/v4/projects/42/repository/files/topics%2Fx.yaml/raw?ref=src"); got != 200 {
		t.Fatalf("head side at pinned source SHA = %d, want 200", got)
	}
	// A side that was never seeded is ABSENT — 404, not present-empty content.
	h.headSet = false
	if got := get(srv.URL + "/api/v4/projects/42/repository/files/topics%2Fx.yaml/raw?ref=src"); got != 404 {
		t.Fatalf("an unseeded side must be 404, got %d", got)
	}
	h.headSet = true
	// An unexpected ref is 404 too.
	if got := get(srv.URL + "/api/v4/projects/42/repository/files/topics%2Fx.yaml/raw?ref=other"); got != 404 {
		t.Fatalf("an unexpected ref must be 404, got %d", got)
	}
}

func TestHarnessForkRouteServesSourceProject(t *testing.T) {
	h := newGitLabHarness("42", "7")
	h.forkMR = true
	h.sourceSHA = "src"
	h.targetSHA = "tgt"
	h.headFile = []byte("fork head bytes")
	h.headSet = true

	srv := httptest.NewServer(http.HandlerFunc(h.handle))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v4/projects/999/repository/files/topics%2Fx.yaml/raw?ref=src", nil)
	req.Header.Set("PRIVATE-TOKEN", "test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("the fork's source-project route must serve head content, got %d", resp.StatusCode)
	}

	// The MR JSON must carry the fork's source_project_id, or the adapter's
	// FileAtHead would read inside the wrong repository.
	if !isFileRawOf(h, "/api/v4/projects/999/repository/files/topics%2Fx.yaml/raw") {
		t.Fatal("isFileRawOf must recognise the fork source-project route")
	}
	if isFileRawOf(h, "/api/v4/projects/42/repository/files/topics%2Fx.yaml") {
		t.Fatal("a non-raw request is not a governed-file read")
	}
	if isFileRawOf(h, "/api/v4/projects/43/repository/files/topics%2Fx.yaml/raw") {
		t.Fatal("a foreign project id is not served")
	}
}

func TestHarnessRefusedPathIsForbidden(t *testing.T) {
	h := newGitLabHarness("42", "7")
	h.sourceSHA = "src"
	h.targetSHA = "tgt"
	h.refusedPath = "topics/x.yaml"
	h.baseFile = []byte("base bytes")
	h.baseSet = true

	srv := httptest.NewServer(http.HandlerFunc(h.handle))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v4/projects/42/repository/files/topics%2Fx.yaml/raw?ref=tgt", nil)
	req.Header.Set("PRIVATE-TOKEN", "test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 403 {
		t.Fatalf("a refused path must answer 403, got %d", resp.StatusCode)
	}
}

func TestHarnessServeMRCarriesForkFields(t *testing.T) {
	h := newGitLabHarness("42", "7")
	h.sourceSHA = "src"
	h.targetSHA = "tgt"
	h.forkMR = true
	srv := httptest.NewServer(http.HandlerFunc(h.handle))
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v4/projects/42/merge_requests/7", nil)
	req.Header.Set("PRIVATE-TOKEN", "test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), `"source_project_id":999`) {
		t.Fatalf("the fork MR JSON must carry source_project_id 999, got %s", string(buf[:n]))
	}
}
