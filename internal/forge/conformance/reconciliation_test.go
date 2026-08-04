package conformance

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

// TestConformanceRerunIdempotence replays the frozen P3-E5 rerun-idempotence and
// crash-then-rerun fixtures (REQ-E4-S09-01). A plain rerun creates zero new bot
// threads; a crash-then-rerun fills exactly the one gap slot without duplicating
// partial work.
func TestConformanceRerunIdempotence(t *testing.T) {
	t.Run("fake/rerun-idempotence", func(t *testing.T) {
		// Pre-state = rerun-idempotence.yaml run2.step2ExistingArtifacts (lines 58-66).
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/9001", botID, rerunChallengeMarker(), true)  // reviewer-resolved
		f.SeedThread("note/9002", botID, rerunCommentMarker(), false)

		created, err := replayRerunIdempotence(f)
		if err != nil {
			t.Fatal(err)
		}
		// expected.newArtifactsCreated: 0 (rerun-idempotence.yaml line 82).
		if created != 0 {
			t.Fatalf("rerun must create zero new artifacts, created %d", created)
		}
		if got := f.BotThreadCount(); got != 2 {
			t.Fatalf("rerun must leave exactly 2 bot threads, got %d", got)
		}
		if !f.IsResolved("note/9001") {
			t.Fatal("rerun must preserve reviewer resolution of note/9001")
		}
	})

	t.Run("fake/crash-then-rerun", func(t *testing.T) {
		// Pre-state = crash-then-rerun.yaml step2ExistingArtifacts (lines 55-61).
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/7001", botID, crashChallengeMarker(), false)

		created, err := replayCrashThenRerun(f)
		if err != nil {
			t.Fatal(err)
		}
		// expected.newArtifactsCreated: 1 (crash-then-rerun.yaml line 81).
		if created != 1 {
			t.Fatalf("crash-then-rerun must create exactly one gap artifact, created %d", created)
		}
		if got := f.BotThreadCount(); got != 2 {
			t.Fatalf("expected 2 bot threads after gap-fill, got %d", got)
		}
	})

	t.Run("gitlab/rerun-idempotence", func(t *testing.T) {
		h := newGitLabHarness(proj, mrIID)
		if err := h.seed("note/9001", botID, rerunChallengeMarker(), true); err != nil {
			t.Fatal(err)
		}
		if err := h.seed("note/9002", botID, rerunCommentMarker(), false); err != nil {
			t.Fatal(err)
		}
		c := h.client(t)

		created, err := replayRerunIdempotence(c)
		if err != nil {
			t.Fatal(err)
		}
		if created != 0 {
			t.Fatalf("gitlab rerun must create zero new artifacts, created %d", created)
		}
		if h.createCalls != 0 {
			t.Fatalf("gitlab rerun must not POST new discussions, createCalls=%d", h.createCalls)
		}
	})

	t.Run("gitlab/crash-then-rerun", func(t *testing.T) {
		h := newGitLabHarness("platform/orders-service", "551")
		if err := h.seed("note/7001", botID, crashChallengeMarker(), false); err != nil {
			t.Fatal(err)
		}
		c := h.client(t)

		created, err := replayCrashThenRerun(c)
		if err != nil {
			t.Fatal(err)
		}
		if created != 1 {
			t.Fatalf("gitlab crash-then-rerun must create one gap artifact, created %d", created)
		}
		if h.createCalls != 1 {
			t.Fatalf("gitlab crash-then-rerun must POST exactly one discussion, createCalls=%d", h.createCalls)
		}
	})
}

// TestConformanceDuplicateRepair replays duplicate-repair.yaml (REQ-E4-S09-02):
// lowest forge id is canonical, non-canonical duplicates are resolved, and
// PublicationReceipt.repairs records each repair deterministically.
func TestConformanceDuplicateRepair(t *testing.T) {
	fixtureOrder := []string{"note/8005", "note/8001", "note/8003"}
	reversedOrder := []string{"note/8003", "note/8001", "note/8005"}
	wantRepairs := []forge.Repair{
		{RepairedForgeID: "note/8003", CanonicalForgeID: "note/8001", Action: "resolve"},
		{RepairedForgeID: "note/8005", CanonicalForgeID: "note/8001", Action: "resolve"},
	}

	for _, tc := range []struct {
		name string
		seed []string
	}{
		{"fake/fixture-pagination-order", fixtureOrder},
		{"fake/reversed-scan-order", reversedOrder},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fake.New(botID, "src", "tgt", "sha256:merge")
			for _, id := range tc.seed {
				f.SeedThread(id, botID, dupMarker(), false)
			}

			receipt, err := replayDuplicateRepair(f)
			if err != nil {
				t.Fatal(err)
			}

			// expected.canonicalForgeId: note/8001 (duplicate-repair.yaml line 102).
			if len(receipt.Operations) != 1 || receipt.Operations[0].TargetID != "note/8001" {
				t.Fatalf("canonical must be note/8001, got %+v", receipt.Operations)
			}
			if len(receipt.Repairs) != len(wantRepairs) {
				t.Fatalf("expected %d repairs, got %+v", len(wantRepairs), receipt.Repairs)
			}
			for i, want := range wantRepairs {
				if receipt.Repairs[i] != want {
					t.Fatalf("repair[%d]: got %+v want %+v", i, receipt.Repairs[i], want)
				}
			}
			if got := f.OpenBotThreadCount(); got != 1 {
				t.Fatalf("after repair exactly one open bot thread must remain, got %d", got)
			}
		})
	}

	t.Run("gitlab/fixture-pagination-order", func(t *testing.T) {
		h := newGitLabHarness("platform/orders-service", "612")
		for _, id := range fixtureOrder {
			if err := h.seed(id, botID, dupMarker(), false); err != nil {
				t.Fatal(err)
			}
		}
		c := h.client(t)

		receipt, err := replayDuplicateRepair(c)
		if err != nil {
			t.Fatal(err)
		}
		if receipt.Operations[0].TargetID != "note/8001" {
			t.Fatalf("gitlab canonical must be note/8001, got %q", receipt.Operations[0].TargetID)
		}
		if len(receipt.Repairs) != 2 {
			t.Fatalf("gitlab repair must record 2 repairs, got %+v", receipt.Repairs)
		}
		if h.createCalls != 0 {
			t.Fatalf("duplicate repair must not create threads, createCalls=%d", h.createCalls)
		}
		if h.resolveCalls != 2 {
			t.Fatalf("duplicate repair must resolve 2 duplicates, resolveCalls=%d", h.resolveCalls)
		}
		open := 0
		for _, d := range h.discussions {
			if d.author == botID && !d.resolved {
				open++
			}
		}
		if open != 1 {
			t.Fatalf("after repair exactly one open bot discussion must remain, got %d", open)
		}
	})
}

// TestConformanceSpoofedMarkerIgnored proves contributor marker spoofing has zero
// reconciliation effect (REQ-E4-S09-03 / P3-E5-S01-04): the author-identity
// filter on ListBotThreads excludes non-bot markers.
func TestConformanceSpoofedMarkerIgnored(t *testing.T) {
	m := rerunCommentMarker()

	t.Run("fake/contributor-marker-invisible-on-rerun", func(t *testing.T) {
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/9002", botID, m, false)
		f.SeedThread("note/6660", "contributor-mallory", m, false)

		before := f.ThreadCount()
		receipt, err := forge.Reconcile(f, testClock(), desiredThreadFor(m), forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if after := f.ThreadCount(); after != before {
			t.Fatalf("spoofed marker must not create threads on rerun: before=%d after=%d", before, after)
		}
		if got := receipt.Operations[0].TargetID; got != "note/9002" {
			t.Fatalf("receipt must target bot thread note/9002, got %q (contributor would be note/6660)", got)
		}
	})

	t.Run("fake/contributor-only-creates-bot-thread", func(t *testing.T) {
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedThread("note/6660", "contributor-mallory", m, false)

		_, err := forge.Reconcile(f, testClock(), desiredThreadFor(m), forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if got := f.BotThreadCount(); got != 1 {
			t.Fatalf("contributor marker alone must not satisfy slot; expected 1 bot thread, got %d", got)
		}
	})

	t.Run("gitlab/contributor-marker-excluded", func(t *testing.T) {
		h := newGitLabHarness(m.Slot.Project, m.Slot.MR)
		if err := h.seed("note/9002", botID, m, false); err != nil {
			t.Fatal(err)
		}
		if err := h.seed("note/6660", "contributor-mallory", m, false); err != nil {
			t.Fatal(err)
		}
		c := h.client(t)

		before, err := botThreadCount(c, m.Slot.Project, m.Slot.MR)
		if err != nil {
			t.Fatal(err)
		}
		receipt, err := forge.Reconcile(c, testClock(), desiredThreadFor(m), forge.Preconditions{})
		if err != nil {
			t.Fatal(err)
		}
		after, err := botThreadCount(c, m.Slot.Project, m.Slot.MR)
		if err != nil {
			t.Fatal(err)
		}
		if after-before != 0 {
			t.Fatalf("gitlab spoof rerun must create zero artifacts, created %d", after-before)
		}
		if h.createCalls != 0 {
			t.Fatalf("contributor marker must not prevent idempotent reuse; createCalls=%d", h.createCalls)
		}
		if got := receipt.Operations[0].TargetID; got != "note/9002" {
			t.Fatalf("receipt must target bot thread note/9002, got %q", got)
		}
		threads, err := c.ListBotThreads(m.Slot.Project, m.Slot.MR)
		if err != nil {
			t.Fatal(err)
		}
		if len(threads) != 1 || threads[0].ID != "note/9002" {
			t.Fatalf("ListBotThreads must return only bot thread, got %+v", threads)
		}
	})

	t.Run("gitlab/contributor-only-creates-bot-thread", func(t *testing.T) {
		h := newGitLabHarness(m.Slot.Project, m.Slot.MR)
		if err := h.seed("note/6660", "contributor-mallory", m, false); err != nil {
			t.Fatal(err)
		}
		c := h.client(t)

		_, err := forge.Reconcile(c, testClock(), desiredThreadFor(m), forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if h.createCalls != 1 {
			t.Fatalf("contributor marker invisible — must create one bot thread, createCalls=%d", h.createCalls)
		}
		threads, err := c.ListBotThreads(m.Slot.Project, m.Slot.MR)
		if err != nil {
			t.Fatal(err)
		}
		if len(threads) != 1 || threads[0].Author != botID {
			t.Fatalf("only bot-authored thread counts, got %+v", threads)
		}
	})
}
