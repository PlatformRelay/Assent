package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

const snapshotProject = "42"
const snapshotMR = "7"

// diffEntry is one entry of the GitLab MR /diffs response as the cassettes model
// it: the two path fields the fold reads (D-076) plus the optional overflow
// marker ADR-0020 §2 requires the adapter to decode.
type diffEntry struct {
	OldPath  string `json:"old_path"`
	NewPath  string `json:"new_path"`
	Overflow bool   `json:"overflow,omitempty"`
}

// diffsCassette configures the paginated /diffs half of a Snapshot cassette.
// pages[i] is served for page=i+1; any page beyond the slice serves an empty
// array (the terminating short page). status, when non-zero, is served for
// EVERY diffs request instead of 200 (the ADR-0020 §3 hard-error axis).
type diffsCassette struct {
	changesCount string
	pages        [][]diffEntry
	status       int
}

// renameCassette is the happy-path enumeration: four diff ENTRIES yielding five
// deduped PATHS (the rename contributes two). changes_count cross-checks the
// ENTRY count — the path count would be 5 and must never be what is compared,
// or every renaming MR would fail-safe-degrade forever (ADR-0020 §2).
func renameCassette() diffsCassette {
	return diffsCassette{
		changesCount: "4",
		pages: [][]diffEntry{{
			{OldPath: "README.md", NewPath: "README.md"},
			{OldPath: "removed.go", NewPath: ""},
			{OldPath: "", NewPath: "added.go"},
			{OldPath: "old/name.go", NewPath: "new/name.go"},
		}},
	}
}

// fullPagesCassette serves n FULL pages (diffsPerPage entries each) and never a
// short page — the pagination-ceiling shape.
func fullPagesCassette(n int) diffsCassette {
	pages := make([][]diffEntry, n)
	for p := range pages {
		page := make([]diffEntry, diffsPerPage)
		for i := range page {
			name := fmt.Sprintf("pad/f%d_%d.txt", p, i)
			page[i] = diffEntry{OldPath: name, NewPath: name}
		}
		pages[p] = page
	}
	return diffsCassette{changesCount: fmt.Sprintf("%d", n*diffsPerPage), pages: pages}
}

// premiumSnapshotHandler serves the multi-endpoint cassette for Premium-tier Snapshot tests.
func premiumSnapshotHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return snapshotHandler(t, renameCassette())
}

// snapshotHandler serves the Premium-tier Snapshot cassette with a configurable
// /diffs half.
func snapshotHandler(t *testing.T, diffs diffsCassette) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7":
			_, _ = fmt.Fprintf(w, `{
				"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main",
				"author":{"username":"alice"},
				"changes_count":%q,
				"diff_refs":{"base_sha":"mergeBaseNOTused"}
			}`, diffs.changesCount)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			_, _ = w.Write([]byte(`{"commit":{"id":"tgtTIP"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7/diffs":
			serveDiffsPage(t, w, r, diffs)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42":
			_, _ = w.Write([]byte(`{
				"only_allow_merge_if_all_discussions_are_resolved":true,
				"merge_trains_enabled":true,
				"ci_config_path":".gitlab-ci.yml@group/external-ci"
			}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7/approval_rules":
			if r.URL.Query().Get("page") == "2" {
				_, _ = w.Write([]byte(`[{"id":1,"name":"default","approvals_required":1}]`))
				return
			}
			var full []map[string]any
			for i := 0; i < 100; i++ {
				full = append(full, map[string]any{"id": i, "name": "noise", "approvals_required": 0})
			}
			_ = json.NewEncoder(w).Encode(full)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/merge_requests/7/discussions"):
			if r.URL.Query().Get("page") == "1" {
				_, _ = w.Write([]byte(`[]`))
			}
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}
}

func serveDiffsPage(t *testing.T, w http.ResponseWriter, r *http.Request, diffs diffsCassette) {
	t.Helper()
	if diffs.status != 0 && diffs.status != http.StatusOK {
		http.Error(w, "diffs unavailable", diffs.status)
		return
	}
	if got := r.URL.Query().Get("per_page"); got != fmt.Sprintf("%d", diffsPerPage) {
		t.Errorf("per_page = %q, want %d (ADR-0020 §2 page size)", got, diffsPerPage)
	}
	page := r.URL.Query().Get("page")
	var idx int
	if _, err := fmt.Sscanf(page, "%d", &idx); err != nil || idx < 1 {
		t.Errorf("page query = %q, want a 1-based page number", page)
		idx = 1
	}
	if idx > len(diffs.pages) {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	_ = json.NewEncoder(w).Encode(diffs.pages[idx-1])
}

// REQ-E4-S02-01: Snapshot MR heads match GetMR semantics (target = branch tip).
func TestSnapshotMRHeads(t *testing.T) {
	c, _ := newServer(t, premiumSnapshotHandler(t))

	snap, err := c.Snapshot(snapshotProject, snapshotMR)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	info, err := c.GetMR(snapshotProject, snapshotMR)
	if err != nil {
		t.Fatalf("GetMR: %v", err)
	}
	if snap.Heads.SourceSHA != info.SourceSHA || snap.Heads.TargetSHA != info.TargetSHA {
		t.Errorf("SHAs mismatch Snapshot vs GetMR: snap=%q/%q getMR=%q/%q",
			snap.Heads.SourceSHA, snap.Heads.TargetSHA, info.SourceSHA, info.TargetSHA)
	}
	if snap.Heads.TargetSHA != "tgtTIP" {
		t.Errorf("TargetSHA = %q, want tgtTIP (branch tip, not merge-base)", snap.Heads.TargetSHA)
	}
	if snap.Heads.SourceBranch != "feature" || snap.Heads.TargetBranch != "main" {
		t.Errorf("branches = %q/%q, want feature/main", snap.Heads.SourceBranch, snap.Heads.TargetBranch)
	}
	if snap.Heads.Author != "alice" {
		t.Errorf("Author = %q, want alice", snap.Heads.Author)
	}
	wantDig := SyntheticDigest("srcSHA", "tgtTIP")
	if snap.Heads.MergeResultDigest != wantDig {
		t.Errorf("MergeResultDigest = %q, want %q", snap.Heads.MergeResultDigest, wantDig)
	}
}

// REQ-E4-S02-02: changed-file enumeration from the MR diffs API (D-076), now
// paginated (ADR-0020 §2). REQ-AUD-S01-01/02 happy polarity: a terminating short
// page plus a changes_count equal to the ENTRY count proves completeness, and a
// rename (2 paths from 1 entry) must not disturb that.
func TestSnapshotChangedFiles(t *testing.T) {
	c, _ := newServer(t, premiumSnapshotHandler(t))

	snap, err := c.Snapshot(snapshotProject, snapshotMR)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	want := []string{"README.md", "added.go", "new/name.go", "old/name.go", "removed.go"}
	if !reflect.DeepEqual(snap.ChangedFiles, want) {
		t.Errorf("ChangedFiles = %v, want sorted %v", snap.ChangedFiles, want)
	}
	if !snap.ChangedFilesComplete {
		t.Errorf("ChangedFilesComplete = false on a proven-complete cassette (gap %q); the count cross-check must compare diff ENTRIES (4), not deduped paths (5)", snap.ChangedFilesGap)
	}
	if snap.ChangedFilesGap != "" {
		t.Errorf("ChangedFilesGap = %q, want empty when complete (ADR-0020 §1)", snap.ChangedFilesGap)
	}
}

// TestSnapshotChangedFilesPaginates proves multi-page enumeration unions every
// page — a second page is never silently dropped.
func TestSnapshotChangedFilesPaginates(t *testing.T) {
	cassette := fullPagesCassette(1)
	cassette.pages = append(cassette.pages, []diffEntry{{OldPath: "tail.go", NewPath: "tail.go"}})
	cassette.changesCount = fmt.Sprintf("%d", diffsPerPage+1)

	c, _ := newServer(t, snapshotHandler(t, cassette))
	snap, err := c.Snapshot(snapshotProject, snapshotMR)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap.ChangedFiles) != diffsPerPage+1 {
		t.Fatalf("ChangedFiles = %d paths, want %d (page 2 must be fetched)", len(snap.ChangedFiles), diffsPerPage+1)
	}
	if !snap.ChangedFilesComplete {
		t.Errorf("ChangedFilesComplete = false, want true (gap %q)", snap.ChangedFilesGap)
	}
}

// REQ-AUD-S01-01 (ADR-0020 §2, D-119): a pagination ceiling hit or a decoded
// per-entry overflow marker degrades to ChangedFilesComplete=false with a
// specific gap — never a silently short list.
func TestChangedFilesOverflowFailsClosed(t *testing.T) {
	t.Run("pagination_ceiling_hit", func(t *testing.T) {
		// maxDiffPages FULL pages and no terminating short page: the enumeration
		// cannot prove there is no page maxDiffPages+1.
		c, _ := newServer(t, snapshotHandler(t, fullPagesCassette(maxDiffPages)))
		snap, err := c.Snapshot(snapshotProject, snapshotMR)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if snap.ChangedFilesComplete {
			t.Fatal("ceiling hit must set ChangedFilesComplete=false (ADR-0020 §2)")
		}
		if !strings.Contains(snap.ChangedFilesGap, "ceiling") {
			t.Errorf("ChangedFilesGap = %q, want a specific ceiling reason", snap.ChangedFilesGap)
		}
		if len(snap.ChangedFiles) == 0 {
			t.Error("the partial list must still be reported (a visible `.assent/**` path must still dominate to BLOCK)")
		}
	})

	t.Run("per_entry_overflow_marker", func(t *testing.T) {
		cassette := renameCassette()
		cassette.pages[0] = append(cassette.pages[0], diffEntry{OldPath: "huge.bin", NewPath: "huge.bin", Overflow: true})
		cassette.changesCount = "5"

		c, _ := newServer(t, snapshotHandler(t, cassette))
		snap, err := c.Snapshot(snapshotProject, snapshotMR)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if snap.ChangedFilesComplete {
			t.Fatal("a decoded overflow marker must set ChangedFilesComplete=false (ADR-0020 §2)")
		}
		if !strings.Contains(snap.ChangedFilesGap, "overflow") {
			t.Errorf("ChangedFilesGap = %q, want a specific overflow reason", snap.ChangedFilesGap)
		}
	})

	t.Run("below_ceiling_is_complete", func(t *testing.T) {
		// Both polarities: a full page followed by a SHORT page terminates below
		// the ceiling and must stay complete.
		cassette := fullPagesCassette(1)
		cassette.pages = append(cassette.pages, []diffEntry{{OldPath: "tail.go", NewPath: "tail.go"}})
		cassette.changesCount = fmt.Sprintf("%d", diffsPerPage+1)

		c, _ := newServer(t, snapshotHandler(t, cassette))
		snap, err := c.Snapshot(snapshotProject, snapshotMR)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if !snap.ChangedFilesComplete || snap.ChangedFilesGap != "" {
			t.Errorf("terminating short page must prove completeness: complete=%v gap=%q", snap.ChangedFilesComplete, snap.ChangedFilesGap)
		}
	})
}

// REQ-AUD-S01-02 (ADR-0020 §2): the changes_count cross-check. A "+"-suffixed
// (capped) count, a mismatch in EITHER direction, an absent count and a
// non-numeric count all degrade to incomplete.
func TestChangedFilesCountMismatchFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name         string
		changesCount string
		wantGapPart  string
	}{
		{"plus_suffix_capped", "1000+", "1000+"},
		{"count_greater_than_enumerated", "9", "9"},
		{"count_less_than_enumerated", "2", "2"},
		{"count_absent", "", "absent"},
		{"count_not_an_integer", "many", "many"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cassette := renameCassette()
			cassette.changesCount = tc.changesCount

			c, _ := newServer(t, snapshotHandler(t, cassette))
			snap, err := c.Snapshot(snapshotProject, snapshotMR)
			if err != nil {
				t.Fatalf("Snapshot: %v", err)
			}
			if snap.ChangedFilesComplete {
				t.Fatalf("changes_count %q must degrade to ChangedFilesComplete=false", tc.changesCount)
			}
			if !strings.Contains(snap.ChangedFilesGap, "changes_count") {
				t.Errorf("ChangedFilesGap = %q, want a changes_count reason", snap.ChangedFilesGap)
			}
			if !strings.Contains(snap.ChangedFilesGap, tc.wantGapPart) {
				t.Errorf("ChangedFilesGap = %q, want it to name %q", snap.ChangedFilesGap, tc.wantGapPart)
			}
			if len(snap.ChangedFiles) == 0 {
				t.Error("the partial list must still be reported for the `.assent/**` dominance check")
			}
		})
	}

	t.Run("matching_count_is_complete", func(t *testing.T) {
		c, _ := newServer(t, snapshotHandler(t, renameCassette()))
		snap, err := c.Snapshot(snapshotProject, snapshotMR)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if !snap.ChangedFilesComplete || snap.ChangedFilesGap != "" {
			t.Errorf("matching changes_count must prove completeness: complete=%v gap=%q", snap.ChangedFilesComplete, snap.ChangedFilesGap)
		}
	})
}

// REQ-AUD-S01-03 (ADR-0020 §3): 404 or any non-200 on the diffs endpoint is a
// HARD ERROR. The MR provably exists by then (the MR GET succeeded), so a
// missing diff resource is forge anomaly — never evidence of an empty change
// set. The 404 → empty-list mapping is removed.
func TestChangedFiles404IsError(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			cassette := renameCassette()
			cassette.status = status

			c, _ := newServer(t, snapshotHandler(t, cassette))
			snap, err := c.Snapshot(snapshotProject, snapshotMR)
			if err == nil {
				t.Fatalf("Snapshot must fail hard on diffs status %d; got snapshot with %d changed files (complete=%v)",
					status, len(snap.ChangedFiles), snap.ChangedFilesComplete)
			}
			if !strings.Contains(err.Error(), "diffs") {
				t.Errorf("error = %v, want it to name the diffs endpoint", err)
			}
		})
	}
}

// REQ-E4-S02-03: capability flags expose tier gaps honestly (Free vs Premium).
func TestSnapshotCapabilityFlags(t *testing.T) {
	t.Run("premium", func(t *testing.T) {
		c, _ := newServer(t, premiumSnapshotHandler(t))
		snap, err := c.Snapshot(snapshotProject, snapshotMR)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		want := forge.CapabilityFlags{
			Tier:                        forge.TierPremium,
			HasApprovalRulesAPI:         true,
			DiscussionsResolvedGate:     true,
			MergeResultDigestRecordable: true,
			MergeTrainAvailable:         true,
			ProtectedPipelineExternal:   true,
		}
		if snap.Capabilities != want {
			t.Errorf("Capabilities = %+v, want %+v", snap.Capabilities, want)
		}
	})

	t.Run("free tier gap", func(t *testing.T) {
		c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
			wantToken(t, r)
			switch r.URL.Path {
			case "/api/v4/projects/42/merge_requests/7":
				_, _ = w.Write([]byte(`{"iid":7,"project_id":42,"sha":"s","source_branch":"f","target_branch":"main","changes_count":"1","author":{"username":"bob"}}`))
			case "/api/v4/projects/42/repository/branches/main":
				_, _ = w.Write([]byte(`{"commit":{"id":"t"}}`))
			case "/api/v4/projects/42/merge_requests/7/diffs":
				if r.URL.Query().Get("page") == "1" {
					_, _ = w.Write([]byte(`[{"old_path":"a.go","new_path":"a.go"}]`))
					return
				}
				_, _ = w.Write([]byte(`[]`))
			case "/api/v4/projects/42":
				_, _ = w.Write([]byte(`{"only_allow_merge_if_all_discussions_are_resolved":false,"merge_trains_enabled":false}`))
			case "/api/v4/projects/42/merge_requests/7/approval_rules":
				http.Error(w, "not found", http.StatusNotFound)
			case "/api/v4/projects/42/merge_requests/7/discussions":
				_, _ = w.Write([]byte(`[]`))
			default:
				http.Error(w, "unexpected", http.StatusInternalServerError)
			}
		})
		snap, err := c.Snapshot(snapshotProject, snapshotMR)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if snap.Capabilities.Tier != forge.TierFree {
			t.Errorf("Tier = %q, want free", snap.Capabilities.Tier)
		}
		if snap.Capabilities.HasApprovalRulesAPI {
			t.Error("HasApprovalRulesAPI = true on Free fixture, want false")
		}
		if snap.Capabilities.MergeTrainAvailable {
			t.Error("MergeTrainAvailable = true on Free fixture, want false (no invented Premium features)")
		}
	})
}

// REQ-E4-S02-04: PAT never appears in URLs, bodies, or error messages.
func TestSnapshotPATRedaction(t *testing.T) {
	const secret = "test-token-redaction-fixture" //nolint:gosec // REQ-E4-S02-04 PAT redaction contract test
	srv := httptestNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("PRIVATE-TOKEN"); got != secret {
			t.Fatalf("PRIVATE-TOKEN = %q, want header-only PAT", got)
		}
		if strings.Contains(r.URL.String(), secret) {
			t.Fatal("token leaked into URL")
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	c := New(srv.URL, secret, botUser)

	_, err := c.Snapshot(snapshotProject, snapshotMR)
	if err == nil {
		t.Fatal("want error from failing server")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("PAT leaked into error: %v", err)
	}
}

func httptestNewServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}
