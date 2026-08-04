package forge_test

import (
	"encoding/json"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
	"github.com/PlatformRelay/assent/internal/render"
)

// This file gates the P4-E1-S12 rerun-idempotence + deterministic
// duplicate-repair invariants against the in-memory fake forge, replaying the
// three FROZEN P3-E5 publication-protocol fixtures:
//
//   docs/contracts/p3-e5-publication-protocol/fixtures/rerun-idempotence.yaml
//   docs/contracts/p3-e5-publication-protocol/fixtures/crash-then-rerun.yaml
//   docs/contracts/p3-e5-publication-protocol/fixtures/duplicate-repair.yaml
//
// The fixtures are Level:doc (illustrative, not schema-validated), so their
// load-bearing values (markers, forgeIds, expected.newArtifactsCreated,
// expected.canonicalForgeId, expected.repairs) are hand-encoded here WITH a
// per-value citation to the frozen fixture line. Reconcile is a PER-SLOT port
// (DesiredReviewState carries exactly one Thread), so a multi-slot fixture is
// replayed one finding-thread slot at a time and the artifact deltas are summed;
// the summary-comment slot is published via the additive Summary preamble (E8-S12).

// ---- rerun-idempotence.yaml markers (run1.artifactsCreated) ----

// occChallenge is rerun-idempotence.yaml's challenge-slot occurrence (line 23).
const occChallenge = "sha256:c6957a516c95532386bed08f56441dfbb8d18efda24f5abdab1e48437aa3357d"

// occComment is rerun-idempotence.yaml's comment-slot occurrence (line 25).
const occComment = "sha256:1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaaa"

// rerunChallengeMarker mirrors rerun-idempotence.yaml's challenge slot
// (topic-safety/retention-shrink-challenge, effect challenge, note/9001).
func rerunChallengeMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  proj,
			MR:       mrIID,
			Rule:     "topic-safety/retention-shrink-challenge",
			Effect:   "challenge",
			EntryRef: "topic-registry:orders.events.v1",
		},
		Occurrence: occChallenge,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

// rerunCommentMarker mirrors rerun-idempotence.yaml's comment slot
// (ownership/entry-owner-required, effect comment, note/9002). It shares the
// slot fields of reviewMarker() (helpers) but pins occComment explicitly.
func rerunCommentMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  proj,
			MR:       mrIID,
			Rule:     "ownership/entry-owner-required",
			Effect:   "comment",
			EntryRef: "topic-registry:orders.events.v1",
		},
		Occurrence: occComment,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

// desiredThreadFor builds a per-slot DesiredReviewState for the given marker —
// one call to Reconcile publishes one slot (the port is per-slot). When summary
// is non-nil the E8-S12 publication preamble upserts the summary slot first.
func desiredThreadFor(m forge.Marker, summary *forge.DesiredSummary) forge.DesiredReviewState {
	return forge.DesiredReviewState{
		Project: proj,
		MR:      mrIID,
		Thread:  &forge.DesiredThread{Marker: m, Body: "obligation not proven"},
		Summary: summary,
	}
}

func rerunSummary() *forge.DesiredSummary {
	return &forge.DesiredSummary{
		Marker: summaryMarker(),
		Body:   "placeholder summary",
	}
}

// TestReconcileReplaysP3E5Fixtures proves REQ-P4-E1-S12-02: replaying the frozen
// rerun-idempotence.yaml and crash-then-rerun.yaml fixtures against the fake
// produces the fixture's expected.newArtifactsCreated — 0 for a rerun, 1 for a
// crash-then-rerun (the single gap-fill slot), with ZERO duplicates in both.
func TestReconcileReplaysP3E5Fixtures(t *testing.T) {
	t.Run("rerun-idempotence/zero-new-artifacts", func(t *testing.T) {
		// Pre-state = rerun-idempotence.yaml run2.step2ExistingArtifacts: both
		// finding-thread slots already have a bot thread with matching marker.
		// Between run1 and run2 a reviewer RESOLVED note/9001 via the forge UI
		// (fixture `between.reviewerAction`, line 49; step2ExistingArtifacts marks
		// note/9001 `resolved: true`, line 62) — so this run must exercise the
		// state-table row-3 PRESERVE-RESOLUTION branch (line 71), not re-open or
		// re-derive it. note/9002 stays unresolved (row 2, leave-untouched). The
		// summary-comment note/9000 is seeded and updated in place via Summary preamble.
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedNote("note/9000", botID, summaryMarker(), "old summary")
		f.SeedThread("note/9001", botID, rerunChallengeMarker(), true) // reviewer-resolved
		f.SeedThread("note/9002", botID, rerunCommentMarker(), false)

		beforeThreads := f.ThreadCount()
		beforeSummaries := f.SummaryNoteCount()

		// run2: re-publish BOTH slots with the identical DesiredReviewState.
		slots := []forge.Marker{rerunChallengeMarker(), rerunCommentMarker()}
		for _, m := range slots {
			r, err := forge.Reconcile(f, testClock(), desiredThreadFor(m, rerunSummary()), forge.Preconditions{})
			if err != nil {
				t.Fatalf("Reconcile(%s): %v", m.Slot.Rule, err)
			}
			if len(r.Repairs) != 0 {
				t.Fatalf("rerun must record no repairs, got %+v", r.Repairs)
			}
		}

		// expected.newArtifactsCreated: 0 (line 82) — every slot already occupied.
		newArtifacts := f.ThreadCount() - beforeThreads
		if newArtifacts != 0 {
			t.Fatalf("rerun must create zero new thread artifacts, created %d", newArtifacts)
		}
		if got := f.SummaryNoteCount() - beforeSummaries; got != 0 {
			t.Fatalf("rerun must not create a new summary note, created %d", got)
		}
		wantSummary, err := render.Envelope(summaryMarker(), "placeholder summary")
		if err != nil {
			t.Fatal(err)
		}
		if got := f.NoteBody("note/9000"); got != wantSummary {
			t.Fatalf("summary must be updated in place (summaryUpdated: true), got %q", got)
		}
		// expected.duplicateSlotOccupancy: 0 (line 83) — exactly the two seeded
		// bot threads remain, one per slot.
		if got := f.BotThreadCount(); got != 2 {
			t.Fatalf("rerun must leave exactly the 2 seeded bot threads, got %d", got)
		}
		// PRESERVE-RESOLUTION (state-table row 3, line 71): the reviewer's
		// resolution of note/9001 is READ, never overridden — the rerun must leave
		// note/9001 resolved. This is the fixture's one adversarial state: a rerun
		// that re-opened a reviewer-resolved thread would be a silent regression.
		if !f.IsResolved("note/9001") {
			t.Fatal("rerun must PRESERVE the reviewer's resolution of note/9001, not re-open it")
		}
	})

	t.Run("crash-then-rerun/gap-fill-one-slot", func(t *testing.T) {
		// Pre-state = crash-then-rerun.yaml step2ExistingArtifacts: run1 crashed
		// after creating the challenge finding-thread (note/7001) and the summary
		// (note/7000). The comment slot was never reached, so no artifact exists for it.
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedNote("note/7000", botID, crashSummaryMarker(), "crash summary")
		f.SeedThread("note/7001", botID, crashChallengeMarker(), false)

		before := f.ThreadCount()

		// run2: re-publish BOTH slots. Step-2 listing finds note/7001 (challenge)
		// -> idempotent, zero new; the comment slot has no occupant -> one create.
		var created int
		for _, m := range []forge.Marker{crashChallengeMarker(), crashCommentMarker()} {
			r, err := forge.Reconcile(f, testClock(), desiredCrashThreadFor(m), forge.Preconditions{})
			if err != nil {
				t.Fatalf("Reconcile(%s): %v", m.Slot.Rule, err)
			}
			if len(r.Repairs) != 0 {
				t.Fatalf("crash-then-rerun must record no repairs, got %+v", r.Repairs)
			}
		}
		created = f.ThreadCount() - before

		// expected.newArtifactsCreated: 1 (line 81) — exactly the slot run1 never
		// reached; duplicateSlotOccupancy: 0 (line 82) — the challenge slot is not
		// re-created.
		if created != 1 {
			t.Fatalf("crash-then-rerun must create exactly one new artifact (the gap slot), created %d", created)
		}
		// Two bot threads total: the reused note/7001 + the one gap-fill create.
		if got := f.BotThreadCount(); got != 2 {
			t.Fatalf("expected 2 bot threads after gap-fill, got %d", got)
		}
	})
}

// ---- crash-then-rerun.yaml markers ----

// occCrashChallenge is crash-then-rerun.yaml's challenge occurrence (line 23).
const occCrashChallenge = "sha256:aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"

// occCrashComment is crash-then-rerun.yaml's comment occurrence (line 25).
const occCrashComment = "sha256:bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"

func crashChallengeMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "platform/orders-service",
			MR:       "551",
			Rule:     "topic-safety/retention-shrink-challenge",
			Effect:   "challenge",
			EntryRef: "topic-registry:payments.events.v1",
		},
		Occurrence: occCrashChallenge,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func crashCommentMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "platform/orders-service",
			MR:       "551",
			Rule:     "ownership/entry-owner-required",
			Effect:   "comment",
			EntryRef: "topic-registry:payments.events.v1",
		},
		Occurrence: occCrashComment,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func crashSummaryMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project: "platform/orders-service",
			MR:      "551",
			Rule:    "assent/summary",
			Effect:  "comment",
		},
		Occurrence: decHex,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "summary-comment", SchemaVersion: "v1alpha1"},
	}
}

func desiredCrashThreadFor(m forge.Marker) forge.DesiredReviewState {
	return forge.DesiredReviewState{
		Project: "platform/orders-service",
		MR:      "551",
		Thread:  &forge.DesiredThread{Marker: m, Body: "obligation not proven"},
		Summary: &forge.DesiredSummary{
			Marker: crashSummaryMarker(),
			Body:   "placeholder summary",
		},
	}
}

// ---- duplicate-repair.yaml (S12-03) ----

// occDup is duplicate-repair.yaml's challenge-slot occurrence (line 38): the
// occurrence all three raced duplicates share.
const occDup = "sha256:dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444"

// dupMarker is the marker the three raced challenge-slot duplicates all carry
// (duplicate-repair.yaml step2ExistingArtifacts note/8005, note/8001, note/8003).
func dupMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "platform/orders-service",
			MR:       "612",
			Rule:     "topic-safety/retention-shrink-challenge",
			Effect:   "challenge",
			EntryRef: "topic-registry:orders.events.v1",
		},
		Occurrence: occDup,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func dupDesired() forge.DesiredReviewState {
	return forge.DesiredReviewState{
		Project: "platform/orders-service",
		MR:      "612",
		Thread:  &forge.DesiredThread{Marker: dupMarker(), Body: "duplicate repair"},
	}
}

// TestDuplicateRepairDeterministic proves REQ-P4-E1-S12-03: two+ bot artifacts
// on ONE slot are repaired by the FIXED rule "lowest-forge-id canonical" —
// numeric, independent of scan/pagination order — every other duplicate is
// resolved against the canonical, and PublicationReceipt.repairs records each
// repair. It replays duplicate-repair.yaml in BOTH the fixture's (non-ascending)
// order and its REVERSAL, asserting the SAME canonical (note/8001) both times.
func TestDuplicateRepairDeterministic(t *testing.T) {
	// duplicate-repair.yaml step2ExistingArtifacts order (lines 35-49):
	// deliberately NOT ascending by forge id.
	fixtureOrder := []string{"note/8005", "note/8001", "note/8003"}
	reversedOrder := []string{"note/8003", "note/8001", "note/8005"}

	cases := []struct {
		name string
		seed []string
	}{
		{"fixture-pagination-order", fixtureOrder},
		{"reversed-scan-order", reversedOrder},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fake.New(botID, "src", "tgt", "sha256:merge")
			for _, id := range tc.seed {
				f.SeedThread(id, botID, dupMarker(), false)
			}
			before := f.ThreadCount()

			receipt, err := forge.Reconcile(f, testClock(), dupDesired(), forge.Preconditions{})
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}

			// expected.newArtifactsCreated: 0 (line 108) — repair creates nothing.
			if newArtifacts := f.ThreadCount() - before; newArtifacts != 0 {
				t.Fatalf("repair must create zero artifacts, created %d", newArtifacts)
			}

			// expected.canonicalForgeId: note/8001 (line 102) — the LOWEST forge id,
			// NOT the first-seen (note/8005 in fixture order). The receipt's single
			// thread op targets the canonical.
			if len(receipt.Operations) != 1 {
				t.Fatalf("repair receipt must carry exactly one thread op (canonical), got %d", len(receipt.Operations))
			}
			if got := receipt.Operations[0].TargetID; got != "note/8001" {
				t.Fatalf("canonical must be the lowest forge id note/8001, got %q", got)
			}

			// expected.repairs (lines 104-106): note/8003 and note/8005 each
			// resolved against note/8001, sorted numeric-ascending by repaired id
			// so the receipt is scan-order-independent.
			wantRepairs := []forge.Repair{
				{RepairedForgeID: "note/8003", CanonicalForgeID: "note/8001", Action: "resolve"},
				{RepairedForgeID: "note/8005", CanonicalForgeID: "note/8001", Action: "resolve"},
			}
			if len(receipt.Repairs) != len(wantRepairs) {
				t.Fatalf("expected %d repairs, got %+v", len(wantRepairs), receipt.Repairs)
			}
			for i, want := range wantRepairs {
				if receipt.Repairs[i] != want {
					t.Fatalf("repair[%d]: got %+v want %+v", i, receipt.Repairs[i], want)
				}
			}

			// duplicateSlotOccupancyAfter: 0 (line 107) — exactly ONE open bot
			// thread (the canonical) occupies the slot; the two duplicates are
			// resolved.
			if got := f.OpenBotThreadCount(); got != 1 {
				t.Fatalf("after repair exactly one OPEN bot thread must remain, got %d", got)
			}
			if !f.IsResolved("note/8003") || !f.IsResolved("note/8005") {
				t.Fatal("both non-canonical duplicates must be resolved")
			}
			if f.IsResolved("note/8001") {
				t.Fatal("the canonical (note/8001) must remain OPEN, not resolved")
			}

			// The receipt WITH repairs still validates against the FROZEN schema
			// (publication-receipt.schema.json is additionalProperties:true at top
			// level, so `repairs` validates with no schema change — D-016).
			raw, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateReceipt(t, raw); err != nil {
				t.Fatalf("receipt with repairs must validate against frozen schema: %v\n%s", err, raw)
			}
		})
	}
}

// TestDuplicateRepairNumericNotLexical proves the canonical selection is NUMERIC,
// not lexical string comparison. The frozen fixture's ids are all 4 digits, so a
// lexical `<` would pass it and hide the bug; here ids of DIFFERENT widths
// (note/999 vs note/1000 vs note/1002) force the distinction: lexically
// "note/1000" < "note/999", but numerically 999 < 1000, so note/999 MUST win.
func TestDuplicateRepairNumericNotLexical(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	// Seed in an order where lexical-min ("note/1000") is NOT the numeric-min.
	for _, id := range []string{"note/1002", "note/1000", "note/999"} {
		f.SeedThread(id, botID, dupMarker(), false)
	}

	receipt, err := forge.Reconcile(f, testClock(), dupDesired(), forge.Preconditions{})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Numeric min is note/999 (999 < 1000 < 1002); a lexical compare would have
	// wrongly kept "note/1000".
	if got := receipt.Operations[0].TargetID; got != "note/999" {
		t.Fatalf("canonical must be numeric-min note/999, got %q (lexical compare would pick note/1000)", got)
	}
	wantRepairs := []forge.Repair{
		{RepairedForgeID: "note/1000", CanonicalForgeID: "note/999", Action: "resolve"},
		{RepairedForgeID: "note/1002", CanonicalForgeID: "note/999", Action: "resolve"},
	}
	if len(receipt.Repairs) != 2 {
		t.Fatalf("expected 2 repairs, got %+v", receipt.Repairs)
	}
	for i, want := range wantRepairs {
		if receipt.Repairs[i] != want {
			t.Fatalf("repair[%d]: got %+v want %+v", i, receipt.Repairs[i], want)
		}
	}
}

// TestDuplicateRepairUnparseableIdNeverCanonical proves the defensive fallback in
// forgeIDNum: an id whose suffix is not a parseable integer sorts as the MAXIMUM
// int, so it can never be made canonical over a well-formed numeric id. The
// well-formed lowest id wins; the malformed id is repaired against it.
func TestDuplicateRepairUnparseableIdNeverCanonical(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	// A malformed id ("note/oops") alongside two numeric ids; canonical must be
	// the numeric-min note/8001, never the malformed one.
	for _, id := range []string{"note/oops", "note/8001", "note/8004"} {
		f.SeedThread(id, botID, dupMarker(), false)
	}

	receipt, err := forge.Reconcile(f, testClock(), dupDesired(), forge.Preconditions{})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := receipt.Operations[0].TargetID; got != "note/8001" {
		t.Fatalf("canonical must be the well-formed numeric-min note/8001, got %q", got)
	}
	if len(receipt.Repairs) != 2 {
		t.Fatalf("both non-canonical ids (including the malformed one) must be repaired, got %+v", receipt.Repairs)
	}
	if f.IsResolved("note/8001") {
		t.Fatal("the well-formed canonical must stay open")
	}
	if !f.IsResolved("note/oops") || !f.IsResolved("note/8004") {
		t.Fatal("both non-canonical ids (malformed included) must be resolved")
	}
}
