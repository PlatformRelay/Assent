package gitlab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

const snapshotProject = "42"
const snapshotMR = "7"

// premiumSnapshotHandler serves the multi-endpoint cassette for Premium-tier Snapshot tests.
func premiumSnapshotHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7":
			_, _ = w.Write([]byte(`{
			"iid":7,"project_id":42,"sha":"srcSHA","source_branch":"feature","target_branch":"main",
			"author":{"username":"alice"},
			"diff_refs":{"base_sha":"mergeBaseNOTused"}
		}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/repository/branches/main":
			_, _ = w.Write([]byte(`{"commit":{"id":"tgtTIP"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7/changes":
			_, _ = w.Write([]byte(`{"changes":[
			{"old_path":"README.md","new_path":"README.md"},
			{"old_path":"removed.go","new_path":""},
			{"old_path":"","new_path":"added.go"},
			{"old_path":"old/name.go","new_path":"new/name.go"}
		]}`))
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

// REQ-E4-S02-02: changed-file enumeration from MR diffs API (D-076).
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
				_, _ = w.Write([]byte(`{"iid":7,"project_id":42,"sha":"s","source_branch":"f","target_branch":"main","author":{"username":"bob"}}`))
			case "/api/v4/projects/42/repository/branches/main":
				_, _ = w.Write([]byte(`{"commit":{"id":"t"}}`))
			case "/api/v4/projects/42/merge_requests/7/changes":
				_, _ = w.Write([]byte(`{"changes":[{"old_path":"a.go","new_path":"a.go"}]}`))
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
