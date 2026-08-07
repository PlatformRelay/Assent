package fake_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

func TestSnapshotInPackage(t *testing.T) {
	f := fake.New(bot, "src", "tgt", "dig")
	f.MRAuthor = "alice"
	f.SourceBranch = "feature/x"
	f.TargetBranch = "main"
	f.ChangedFiles = []string{"b.yaml", "a.go"}
	f.Capabilities = forge.CapabilityFlags{Tier: forge.TierPremium, HasApprovalRulesAPI: true}
	f.SeedThread("note/9001", bot, marker(), false)
	f.SeedThread("note/6660", "contributor", marker(), false)

	snap, err := f.Snapshot("p", "1")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.Heads.SourceSHA != "src" || snap.Heads.Author != "alice" {
		t.Fatalf("Heads = %+v", snap.Heads)
	}
	wantFiles := []string{"a.go", "b.yaml"}
	if !reflect.DeepEqual(snap.ChangedFiles, wantFiles) {
		t.Fatalf("ChangedFiles = %v, want sorted %v", snap.ChangedFiles, wantFiles)
	}
	if len(snap.BotThreads) != 1 || snap.BotThreads[0].ID != "note/9001" {
		t.Fatalf("BotThreads = %+v", snap.BotThreads)
	}
	// ADR-0020 §1/§6: the fake must set completeness EXPLICITLY. The default
	// (no truncation knob) is a provably complete enumeration.
	if !snap.ChangedFilesComplete || snap.ChangedFilesGap != "" {
		t.Fatalf("default fake snapshot must be complete: complete=%v gap=%q", snap.ChangedFilesComplete, snap.ChangedFilesGap)
	}
	if snap.EnumerationOpaqueReason() != "" {
		t.Fatalf("EnumerationOpaqueReason = %q, want empty when complete", snap.EnumerationOpaqueReason())
	}
}

// ADR-0020 §6 / REQ-AUD-S01-01: the fake models a TRUNCATED enumeration — the
// partial path list is still reported (so a visible `.assent/**` path still
// dominates to BLOCK) but completeness is denied with a specific gap.
func TestSnapshotTruncationKnob(t *testing.T) {
	f := fake.New(bot, "src", "tgt", "dig")
	f.ChangedFiles = []string{"a.go"}
	f.ChangedFilesGap = "instance diff limit reached"

	snap, err := f.Snapshot("p", "1")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap.ChangedFilesComplete {
		t.Fatal("truncation knob must deny completeness")
	}
	if snap.ChangedFilesGap != "instance diff limit reached" {
		t.Fatalf("ChangedFilesGap = %q, want the configured reason", snap.ChangedFilesGap)
	}
	if len(snap.ChangedFiles) != 1 {
		t.Fatalf("ChangedFiles = %v, want the partial list still reported", snap.ChangedFiles)
	}
	want := forge.EnumerationIncompletePrefix + "instance diff limit reached"
	if got := snap.EnumerationOpaqueReason(); got != want {
		t.Fatalf("EnumerationOpaqueReason = %q, want %q", got, want)
	}
}

// ADR-0020 §3/§6 / REQ-AUD-S01-03: the fake models a diff-endpoint 404/5xx —
// Snapshot fails HARD with no snapshot at all, never an empty change set.
func TestSnapshotDiffEndpointErrorKnob(t *testing.T) {
	boom := errors.New("gitlab: get MR diffs p!1 page 1: unexpected status 404")
	f := fake.New(bot, "src", "tgt", "dig")
	f.ChangedFiles = []string{"a.go"}
	f.ChangedFilesErr = boom

	snap, err := f.Snapshot("p", "1")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the configured diff-endpoint error", err)
	}
	if len(snap.ChangedFiles) != 0 || snap.ChangedFilesComplete {
		t.Fatalf("a hard error must yield NO usable snapshot, got %+v", snap)
	}
}

func TestResolveEligibleInPackage(t *testing.T) {
	f := fake.New(bot, "src", "tgt", "dig")
	f.Capabilities = forge.CapabilityFlags{Tier: forge.TierPremium, HasApprovalRulesAPI: true}
	req := forge.ResolveRequest{
		Project: "p", MR: "1", Subject: "topic",
		SourceSha: "src", TargetSha: "tgt", MergeResultDigest: "dig",
	}
	got, err := f.Resolve(req)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !got.HasEvidence() || got.HasGap() {
		t.Fatalf("want evidence-only, got %+v", got)
	}
	if got.Evidence.Pins.SourceSha != "src" {
		t.Fatalf("Pins.SourceSha = %q", got.Evidence.Pins.SourceSha)
	}
}

func TestResolveGapFreeTierAutoMode(t *testing.T) {
	f := fake.New(bot, "src", "tgt", "dig")
	f.Capabilities = forge.CapabilityFlags{Tier: forge.TierFree, HasApprovalRulesAPI: false}
	req := forge.ResolveRequest{Project: "p", MR: "1", Subject: "topic"}
	got, err := f.Resolve(req)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.HasEvidence() || !got.HasGap() {
		t.Fatalf("want gap-only, got %+v", got)
	}
	if got.Gap.Reason != forge.GapFreeTierRequireReview {
		t.Fatalf("Gap.Reason = %q", got.Gap.Reason)
	}
}

func TestListBotThreadsRescanHook(t *testing.T) {
	f := fake.New(bot, "src", "tgt", "dig")
	f.SeedThread("note/1", bot, marker(), false)
	f.RescanListBotThreads = func(_ []forge.Thread) ([]forge.Thread, error) {
		return nil, errors.New("rescan failed")
	}
	if _, err := f.CreateThread("p", "1", marker(), "body"); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	_, err := f.ListBotThreads("p", "1")
	if err == nil {
		t.Fatal("expected rescan hook error")
	}
}
