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
