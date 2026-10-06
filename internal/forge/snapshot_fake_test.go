package forge_test

import (
	"reflect"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

// TestSnapshotFake — REQ-E4-S01-01/03: typed Snapshot port + deterministic fake.
func TestSnapshotFake(t *testing.T) {
	t.Parallel()

	const (
		proj      = "platform/orders-service"
		mr        = "482"
		srcSHA    = "abc123deadbeef"
		tgtSHA    = "def456cafebabe"
		srcBranch = "feature/orders"
		tgtBranch = "main"
		mergeDig  = "sha256:merge-result-digest-fixed"
		author    = "alice"
		bot       = "assent-bot"
	)

	f := fake.New(bot, srcSHA, tgtSHA, mergeDig)
	f.MRAuthor = author
	f.SourceBranch = srcBranch
	f.TargetBranch = tgtBranch
	f.Labels = []string{"security-hold"}
	f.ChangedFiles = []string{"topics/prod/orders.yaml", "internal/handler.go", ".assent/policy.yaml"}
	f.Capabilities = supportedCapReport(t)
	f.SeedThread("note/9001", bot, reviewMarker(), false)
	f.SeedThread("note/9002", "contributor", reviewMarker(), false)

	var snap forge.Snapshot
	for run := 0; run < 2; run++ {
		got, err := f.Snapshot(proj, mr)
		if err != nil {
			t.Fatalf("run %d: Snapshot: %v", run, err)
		}
		if run == 0 {
			snap = got
		} else if !reflect.DeepEqual(snap, got) {
			t.Fatalf("run %d: non-deterministic snapshot:\nwant %+v\ngot  %+v", run, snap, got)
		}
	}

	wantHeads := forge.MRHeads{
		SourceSHA:         srcSHA,
		TargetSHA:         tgtSHA,
		SourceBranch:      srcBranch,
		TargetBranch:      tgtBranch,
		MergeResultDigest: mergeDig,
		Author:            author,
		Labels:            []string{"security-hold"},
	}
	if !reflect.DeepEqual(snap.Heads, wantHeads) {
		t.Errorf("Heads = %+v, want %+v", snap.Heads, wantHeads)
	}

	wantFiles := []string{".assent/policy.yaml", "internal/handler.go", "topics/prod/orders.yaml"}
	if !reflect.DeepEqual(snap.ChangedFiles, wantFiles) {
		t.Errorf("ChangedFiles = %v, want sorted %v", snap.ChangedFiles, wantFiles)
	}

	if got := snap.Capabilities.State(forge.CapabilityResolvableThreads); got != f.Capabilities.State(forge.CapabilityResolvableThreads) {
		t.Errorf("Capabilities.State(resolvable-threads) = %q, want the fixture's report state", got)
	}
	if got := snap.Capabilities.State(forge.CapabilityEligibleApprovalEvidence); got != forge.CapabilitySupported {
		t.Errorf("Capabilities.State(eligible-approval-evidence) = %q, want supported on the all-supported fixture", got)
	}

	if len(snap.BotThreads) != 1 || snap.BotThreads[0].ID != "note/9001" {
		t.Errorf("BotThreads = %+v, want single bot thread note/9001", snap.BotThreads)
	}

	var _ forge.Snapshotter = (*fake.Forge)(nil)
}
