package github

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
)

// snapshotHandler builds a handler serving the full read chain one Snapshot
// consumes: the PR read, the repo settings read (the capability probe), the
// merge-ref route (200 with mergeRefSHA, or a 404), the pull-files pages, and
// an empty review-comment listing. A requested files page beyond the served
// list repeats the LAST page — which is how the never-shortens cap fixture is
// built from one full page.
func snapshotHandler(t *testing.T, mergeRefStatus int, mergeRefSHA string, filePages []string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
			_, _ = io.WriteString(w, sameRepoPR)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo":
			_, _ = io.WriteString(w, `{"allow_auto_merge":true}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/git/ref/refs/pull/7/merge":
			if mergeRefStatus != http.StatusOK {
				http.Error(w, "no merge ref", mergeRefStatus)
				return
			}
			_, _ = io.WriteString(w, `{"ref":"refs/pull/7/merge","object":{"sha":"`+mergeRefSHA+`","type":"commit"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/files":
			page := pageParam(t, r)
			if page < 1 {
				page = 1
			}
			idx := page - 1
			if idx >= len(filePages) {
				idx = len(filePages) - 1
			}
			_, _ = io.WriteString(w, filePages[idx])
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7/comments":
			_, _ = io.WriteString(w, "[]")
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}
}

// pageParam extracts the ?page= query value.
func pageParam(t *testing.T, r *http.Request) int {
	t.Helper()
	p := 0
	_, _ = fmt.Sscanf(r.URL.Query().Get("page"), "%d", &p)
	return p
}

// filePage builds a files page of n file entries.
func filePage(n int) string {
	out := "["
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf(`{"filename":"pkg/file%03d.go"}`, i)
	}
	return out + "]"
}

// filePageOf builds a literal files page from explicit entries (including
// rename rows with previous_filename).
func filePageOf(entries ...string) string {
	out := "["
	for i, e := range entries {
		if i > 0 {
			out += ","
		}
		out += e
	}
	return out + "]"
}

// TestSnapshotMRInfo proves the snapshot's heads block: head/base pins, the
// PR's own head branch, the author, labels, fork flag, and the merge-result
// digest from the readable merge ref.
func TestSnapshotMRInfo(t *testing.T) {
	c, _ := newServer(t, snapshotHandler(t, http.StatusOK, "mrgSHA", []string{
		filePageOf(`{"filename":"pkg/a.go"}`, `{"filename":"b.go","previous_filename":"pkg/old-b.go"}`),
	}))

	snap, err := c.Snapshot(baseRepo, "7")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.Heads.SourceSHA != "srcSHA" || snap.Heads.TargetSHA != "tgtTIP" {
		t.Errorf("pins = %q/%q, want srcSHA/tgtTIP", snap.Heads.SourceSHA, snap.Heads.TargetSHA)
	}
	if snap.Heads.SourceBranch != "feature" || snap.Heads.TargetBranch != "main" {
		t.Errorf("branches = %q/%q, want feature/main", snap.Heads.SourceBranch, snap.Heads.TargetBranch)
	}
	if snap.Heads.Author != "octocat" {
		t.Errorf("Author = %q, want octocat", snap.Heads.Author)
	}
	if len(snap.Heads.Labels) != 1 || snap.Heads.Labels[0] != "security-hold" {
		t.Errorf("Labels = %#v, want [security-hold]", snap.Heads.Labels)
	}
	if snap.Heads.MergeResultDigest != "mrgSHA" {
		t.Errorf("MergeResultDigest = %q, want the merge ref's object SHA", snap.Heads.MergeResultDigest)
	}
	if snap.Heads.ForkMR {
		t.Error("same-repo PR must not report ForkMR")
	}
}

// TestChangedFilesCompleteness proves the ADR-0020 §1 completeness verdict on
// the pull-files enumeration: a terminating short page proves completeness; a
// never-shortening paginator degrades to an HONEST incomplete enumeration
// (forge.EnumerationIncompletePrefix + ChangedFilesGap) — never a silently
// truncated list.
func TestChangedFilesCompleteness(t *testing.T) {
	t.Run("short page is complete", func(t *testing.T) {
		c, _ := newServer(t, snapshotHandler(t, http.StatusOK, "mrgSHA", []string{
			filePageOf(`{"filename":".assent/config.yaml"}`, `{"filename":"pkg/main.go"}`),
		}))
		snap, err := c.Snapshot(baseRepo, "7")
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if !snap.ChangedFilesComplete {
			t.Errorf("ChangedFilesComplete = false on a terminating short page (gap %q)", snap.ChangedFilesGap)
		}
		if snap.ChangedFilesGap != "" {
			t.Errorf("ChangedFilesGap = %q, want empty when complete", snap.ChangedFilesGap)
		}
		if got := snap.EnumerationOpaqueReason(); got != "" {
			t.Errorf("EnumerationOpaqueReason = %q, want empty", got)
		}
		if len(snap.ChangedFiles) != 2 {
			t.Errorf("ChangedFiles = %#v, want the two enumerated paths", snap.ChangedFiles)
		}
	})

	t.Run("pagination ceiling fails closed", func(t *testing.T) {
		requests := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/repos/octo-org/base-repo/pulls/7/files":
				requests++
				// A page that NEVER shortens: 100 entries every time.
				_, _ = io.WriteString(w, filePage(listPerPage))
			case r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
				_, _ = io.WriteString(w, sameRepoPR)
			case r.URL.Path == "/repos/octo-org/base-repo/git/ref/refs/pull/7/merge":
				_, _ = io.WriteString(w, `{"object":{"sha":"mrgSHA","type":"commit"}}`)
			case r.URL.Path == "/repos/octo-org/base-repo/pulls/7/comments":
				_, _ = io.WriteString(w, "[]")
			case r.URL.Path == "/repos/octo-org/base-repo":
				_, _ = io.WriteString(w, `{"allow_auto_merge":false}`)
			default:
				http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
			}
		}))
		t.Cleanup(srv.Close)
		c := New(srv.URL, patToken, botUser, WithSleeper(func(time.Duration) {}))

		snap, err := c.Snapshot(baseRepo, "7")
		if err != nil {
			t.Fatalf("Snapshot with a never-shortening files paginator: %v", err)
		}
		if requests != maxListPages {
			t.Fatalf("paginator must stop at exactly %d pages, made %d requests", maxListPages, requests)
		}
		if snap.ChangedFilesComplete {
			t.Error("ChangedFilesComplete = true on an unverifiable enumeration — must fail closed")
		}
		if !strings.Contains(snap.ChangedFilesGap, "pagination ceiling") {
			t.Errorf("ChangedFilesGap = %q, want a specific ceiling reason", snap.ChangedFilesGap)
		}
		if got := snap.EnumerationOpaqueReason(); !strings.HasPrefix(got, forge.EnumerationIncompletePrefix) {
			t.Errorf("EnumerationOpaqueReason = %q, want the %q prefix", got, forge.EnumerationIncompletePrefix)
		}
	})
}

// TestChangedFilesHardErrorOnNon200 proves the files endpoint's 404 is a HARD
// error, never an empty change set (the PR provably exists by this point).
func TestChangedFilesHardErrorOnNon200(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/octo-org/base-repo/pulls/7":
			_, _ = io.WriteString(w, sameRepoPR)
		case "/repos/octo-org/base-repo/git/ref/refs/pull/7/merge":
			_, _ = io.WriteString(w, `{"object":{"sha":"mrgSHA","type":"commit"}}`)
		case "/repos/octo-org/base-repo/pulls/7/files":
			http.Error(w, "gone", http.StatusNotFound)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	})
	_, err := c.Snapshot(baseRepo, "7")
	if err == nil {
		t.Fatal("non-200 on the files endpoint must be a hard error, not a gap")
	}
}

// TestMergeResultPin proves the adapter-owned merge-result scheme (dossier
// C16): a readable merge ref yields its object SHA as the digest and grades
// merge-result-pinning supported; an unavailable merge ref yields an empty
// digest (nil error) and an absent capability — the CAS then fails closed.
func TestMergeResultPin(t *testing.T) {
	t.Run("readable merge ref pins the digest", func(t *testing.T) {
		c, _ := newServer(t, snapshotHandler(t, http.StatusOK, "mrgSHA", []string{"[]"}))

		source, target, digest, err := c.CurrentHeads(baseRepo, "7")
		if err != nil {
			t.Fatalf("CurrentHeads: %v", err)
		}
		if source != "srcSHA" || target != "tgtTIP" {
			t.Errorf("heads = %q/%q, want srcSHA/tgtTIP", source, target)
		}
		if digest != "mrgSHA" {
			t.Errorf("digest = %q, want the raw merge-ref object SHA", digest)
		}

		snap, err := c.Snapshot(baseRepo, "7")
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if snap.Heads.MergeResultDigest != "mrgSHA" {
			t.Errorf("Snapshot.MergeResultDigest = %q, want mrgSHA", snap.Heads.MergeResultDigest)
		}
		if state := snap.Capabilities.State(forge.CapabilityMergeResultPinning); state != forge.CapabilitySupported {
			t.Errorf("merge-result-pinning state = %q, want supported", state)
		}
	})

	t.Run("unreadable merge ref is honestly empty", func(t *testing.T) {
		c, _ := newServer(t, snapshotHandler(t, http.StatusNotFound, "", []string{"[]"}))

		source, target, digest, err := c.CurrentHeads(baseRepo, "7")
		if err != nil {
			t.Fatalf("CurrentHeads with an absent merge ref: %v", err)
		}
		if source != "srcSHA" || target != "tgtTIP" {
			t.Errorf("heads = %q/%q, want srcSHA/tgtTIP", source, target)
		}
		if digest != "" {
			t.Errorf("digest = %q, want empty (the forge exposes no merge result)", digest)
		}

		snap, err := c.Snapshot(baseRepo, "7")
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if snap.Heads.MergeResultDigest != "" {
			t.Errorf("MergeResultDigest = %q, want empty", snap.Heads.MergeResultDigest)
		}
		if state := snap.Capabilities.State(forge.CapabilityMergeResultPinning); state != forge.CapabilityAbsent {
			t.Errorf("merge-result-pinning state = %q, want absent", state)
		}
		if !strings.Contains(snap.Capabilities.Reason(forge.CapabilityMergeResultPinning), "merge ref unreadable") {
			t.Errorf("capability reason = %q, want the unmergeable/merge-queue wording", snap.Capabilities.Reason(forge.CapabilityMergeResultPinning))
		}
	})
}
