package conformance

import (
	"github.com/PlatformRelay/assent/internal/forge"
)

// cases_replay.go holds the three ADR-0019 publication-protocol replay cases.
//
// Each one previously existed TWICE — once as a `fake/...` subtest and once as a
// `gitlab/...` subtest — with assertions that did not match: the fake subtests
// asserted on thread COUNTS, the GitLab subtests on write-CALL counts
// (`createCalls`, `noteCreateCalls`, `noteUpdateCalls`, `resolveCalls`), because
// each backend only exposed one of the two. Neither backend was checked against
// the other's assertions.
//
// Extraction merges them into ONE body per case asserting the UNION, which every
// backend must now satisfy. No assertion was dropped to make that work
// (REQ-E10-S01-04); the counters the fake lacked were added to it instead.

func replayConfig(project, mr string) Config {
	return Config{
		Project:   project,
		MR:        mr,
		BotAuthor: botID,
		// The replay cases never merge, so the heads only have to be internally
		// consistent — but they are still stated explicitly rather than defaulted,
		// because a Factory that invents SHAs makes the SHA-guard cases unreadable.
		CurrentSourceSHA:         "src",
		CurrentTargetSHA:         "tgt",
		CurrentMergeResultDigest: "sha256:merge",
	}
}

// caseRerunIdempotence is REQ-E4-S09-01, replaying rerun-idempotence.yaml and
// crash-then-rerun.yaml: a plain rerun creates zero new bot threads and updates
// the summary IN PLACE, while a crash-then-rerun fills exactly the one gap slot
// without duplicating the partial work that already landed.
func caseRerunIdempotence(t TB, f Factory) {
	t.Helper()

	t.Run("rerun-idempotence", func(t TB) {
		// Pre-state = rerun-idempotence.yaml run2.step2ExistingArtifacts (lines 58-66).
		b := f(t, replayConfig(proj, mrIID))
		mustSeedNote(t, b, "note/9000", botID, rerunSummaryMarker(), "old summary")
		mustSeedThread(t, b, "note/9001", botID, rerunChallengeMarker(), true) // reviewer-resolved
		mustSeedThread(t, b, "note/9002", botID, rerunCommentMarker(), false)

		created, err := replayRerunIdempotence(b.Port, b.Observer)
		if err != nil {
			t.Fatal(err)
		}
		// expected.newArtifactsCreated: 0 (rerun-idempotence.yaml line 82).
		if created != 0 {
			t.Fatalf("rerun must create zero new artifacts, created %d", created)
		}
		if got := b.Observer.BotThreadCount(); got != 2 {
			t.Fatalf("rerun must leave exactly 2 bot threads, got %d", got)
		}
		if !b.Observer.IsResolved("note/9001") {
			t.Fatal("rerun must preserve reviewer resolution of note/9001")
		}
		// Was GitLab-only before extraction; now required of every backend.
		if got := b.Observer.ThreadsCreated(); got != 0 {
			t.Fatalf("rerun must not create discussions, ThreadsCreated=%d", got)
		}
		if got := b.Observer.NotesCreated(); got != 0 {
			t.Fatalf("rerun must not create summary notes, NotesCreated=%d", got)
		}
		// EXACT, not ">0". The replay reconciles two markers, each carrying the same
		// summary, so the summary must be edited in place once per reconcile and never
		// re-created. The ">0" form here was DEAD WEIGHT: any positive count satisfied
		// it, including a runaway one, so no corruption of the value could flip the
		// verdict. TestEveryObservationIsLoadBearing caught that, not review.
		if got := b.Observer.NotesUpdated(); got != 2 {
			t.Fatalf("rerun must update the summary in place once per reconcile (want 2), got %d", got)
		}
	})

	t.Run("crash-then-rerun", func(t TB) {
		// Pre-state = crash-then-rerun.yaml step2ExistingArtifacts (lines 55-61).
		m := crashChallengeMarker()
		b := f(t, replayConfig(m.Slot.Project, m.Slot.MR))
		mustSeedNote(t, b, "note/7000", botID, crashSummaryMarker(), "crash summary")
		mustSeedThread(t, b, "note/7001", botID, m, false)

		created, err := replayCrashThenRerun(b.Port)
		if err != nil {
			t.Fatal(err)
		}
		// expected.newArtifactsCreated: 1 (crash-then-rerun.yaml line 81).
		if created != 1 {
			t.Fatalf("crash-then-rerun must create exactly one gap artifact, created %d", created)
		}
		if got := b.Observer.BotThreadCount(); got != 2 {
			t.Fatalf("expected 2 bot threads after gap-fill, got %d", got)
		}
		if got := b.Observer.ThreadsCreated(); got != 1 {
			t.Fatalf("crash-then-rerun must create exactly one discussion, ThreadsCreated=%d", got)
		}
	})
}

// caseDuplicateRepair is REQ-E4-S09-02, replaying duplicate-repair.yaml: the
// lowest forge id is canonical, non-canonical duplicates are RESOLVED rather than
// deleted or re-created, and PublicationReceipt.Repairs records each repair
// deterministically.
//
// Both seed orders run against every backend. The reversed order is not
// decoration: it is what proves canonical selection is by forge ID and not by
// scan order, and before extraction it ran on the fake only, while the GitLab
// side ran the fixture order alone under the name `gitlab/fixture-pagination-order`.
func caseDuplicateRepair(t TB, f Factory) {
	t.Helper()

	wantRepairs := []forge.Repair{
		{RepairedForgeID: "note/8003", CanonicalForgeID: "note/8001", Action: "resolve"},
		{RepairedForgeID: "note/8005", CanonicalForgeID: "note/8001", Action: "resolve"},
	}

	for _, tc := range []struct {
		name string
		seed []string
	}{
		{"fixture-pagination-order", []string{"note/8005", "note/8001", "note/8003"}},
		{"reversed-scan-order", []string{"note/8003", "note/8001", "note/8005"}},
	} {
		t.Run(tc.name, func(t TB) {
			b := f(t, replayConfig("platform/orders-service", "612"))
			for _, id := range tc.seed {
				mustSeedThread(t, b, id, botID, dupMarker(), false)
			}

			receipt, err := replayDuplicateRepair(b.Port)
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
			if got := b.Observer.OpenBotThreadCount(); got != 1 {
				t.Fatalf("after repair exactly one open bot thread must remain, got %d", got)
			}
			// Both were GitLab-only before extraction.
			if got := b.Observer.ThreadsCreated(); got != 0 {
				t.Fatalf("duplicate repair must not create threads, ThreadsCreated=%d", got)
			}
			if got := b.Observer.ThreadsResolved(); got != 2 {
				t.Fatalf("duplicate repair must resolve 2 duplicates, ThreadsResolved=%d", got)
			}
		})
	}
}

// caseSpoofedMarkerIgnored is REQ-E4-S09-03 / P3-E5-S01-04: a contributor posting
// a WELL-FORMED marker has zero reconciliation effect, because the
// author-identity filter on ListBotThreads excludes it.
//
// The two sub-cases assert different things and both are load-bearing. The first
// proves the spoofed thread does not SATISFY an occupied slot; the second proves
// it does not satisfy an EMPTY one either — without it, a backend that ignored
// contributor threads by deleting them would pass.
func caseSpoofedMarkerIgnored(t TB, f Factory) {
	t.Helper()
	m := rerunCommentMarker()

	t.Run("contributor-marker-invisible-on-rerun", func(t TB) {
		b := f(t, replayConfig(m.Slot.Project, m.Slot.MR))
		mustSeedThread(t, b, "note/9002", botID, m, false)
		mustSeedThread(t, b, "note/6660", "contributor-mallory", m, false)

		before := b.Observer.ThreadCount()
		receipt, err := forge.Reconcile(b.Port, testClock(), desiredThreadFor(m, rerunSummary()), forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		// ThreadCount is deliberately the UNFILTERED count: "the contributor thread
		// still exists but is invisible to the bot filter" is a different claim from
		// "no contributor thread exists", and only the unfiltered count can tell a
		// backend that IGNORED the spoof from one that DELETED it.
		if after := b.Observer.ThreadCount(); after != before {
			t.Fatalf("spoofed marker must not create threads on rerun: before=%d after=%d", before, after)
		}
		if got := threadOpTarget(receipt, "note/9002"); got != "note/9002" {
			t.Fatalf("receipt must target bot thread note/9002, got %q (contributor would be note/6660)", got)
		}
	})

	t.Run("contributor-only-creates-bot-thread", func(t TB) {
		b := f(t, replayConfig(m.Slot.Project, m.Slot.MR))
		mustSeedThread(t, b, "note/6660", "contributor-mallory", m, false)

		if _, err := forge.Reconcile(b.Port, testClock(), desiredThreadFor(m, nil), forge.Preconditions{}); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if got := b.Observer.BotThreadCount(); got != 1 {
			t.Fatalf("contributor marker alone must not satisfy slot; expected 1 bot thread, got %d", got)
		}
	})
}

func mustSeedThread(t TB, b Backend, id, author string, m forge.Marker, resolved bool) {
	t.Helper()
	if err := b.Fixture.SeedThread(id, author, m, resolved); err != nil {
		t.Fatalf("seed thread %s: %v", id, err)
	}
}

func mustSeedNote(t TB, b Backend, id, author string, m forge.Marker, body string) {
	t.Helper()
	if err := b.Fixture.SeedNote(id, author, m, body); err != nil {
		t.Fatalf("seed note %s: %v", id, err)
	}
}
