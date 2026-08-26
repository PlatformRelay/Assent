package conformance

import (
	"fmt"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/render"
)

// replay.go holds the backend-neutral REPLAY BODIES of the P3-E5 publication
// protocol. These already took a `forge.Forge` before extraction — the case
// bodies were neutral; only their fixtures and their assertions were not — so
// this file is a move, not a rewrite, with ONE deliberate change called out at
// its site: the `if ff, ok := f.(*fake.Forge); ok` guard around the
// summary-updated-in-place check is gone, because it made that assertion
// unreachable, and therefore unfailable, on every non-fake backend.

func replayRerunIdempotence(f forge.Forge, obs Observer) (newArtifacts int, err error) {
	before, err := botThreadCount(f, proj, mrIID)
	if err != nil {
		return 0, err
	}
	beforeSummaries, err := botSummaryCount(f, proj, mrIID)
	if err != nil {
		return 0, err
	}
	for _, m := range []forge.Marker{rerunChallengeMarker(), rerunCommentMarker()} {
		r, err := forge.Reconcile(f, testClock(), desiredThreadFor(m, rerunSummary()), forge.Preconditions{})
		if err != nil {
			return 0, fmt.Errorf("Reconcile(%s): %w", m.Slot.Rule, err)
		}
		if len(r.Repairs) != 0 {
			return 0, fmt.Errorf("rerun must record no repairs, got %+v", r.Repairs)
		}
	}
	after, err := botThreadCount(f, proj, mrIID)
	if err != nil {
		return 0, err
	}
	if got := botSummaryCountMust(f, proj, mrIID) - beforeSummaries; got != 0 {
		return 0, fmt.Errorf("rerun must not create a new summary note, created %d", got)
	}
	wantSummary, err := render.Envelope(rerunSummaryMarker(), fixtureSummaryBody())
	if err != nil {
		return 0, err
	}
	// Pre-extraction this read `if ff, ok := f.(*fake.Forge); ok` and skipped
	// silently on every other backend — an assertion that could not fail for half
	// the suite, which is the exact species REQ-E10-S01-04 exists to stop. The
	// Observer answers it on both adapters, so it is now unconditional.
	if got := obs.NoteBody("note/9000"); got != wantSummary {
		return 0, fmt.Errorf("summary must be updated in place (summaryUpdated: true), got %q", got)
	}
	return after - before, nil
}

func replayCrashThenRerun(f forge.Forge) (newArtifacts int, err error) {
	project := crashChallengeMarker().Slot.Project
	mr := crashChallengeMarker().Slot.MR
	before, err := botThreadCount(f, project, mr)
	if err != nil {
		return 0, err
	}
	for _, m := range []forge.Marker{crashChallengeMarker(), crashCommentMarker()} {
		r, err := forge.Reconcile(f, testClock(), desiredThreadFor(m, crashSummary()), forge.Preconditions{})
		if err != nil {
			return 0, fmt.Errorf("Reconcile(%s): %w", m.Slot.Rule, err)
		}
		if len(r.Repairs) != 0 {
			return 0, fmt.Errorf("crash-then-rerun must record no repairs, got %+v", r.Repairs)
		}
	}
	after, err := botThreadCount(f, project, mr)
	if err != nil {
		return 0, err
	}
	return after - before, nil
}

func replayDuplicateRepair(f forge.Forge) (forge.PublicationReceipt, error) {
	before, err := botThreadCount(f, "platform/orders-service", "612")
	if err != nil {
		return forge.PublicationReceipt{}, err
	}
	receipt, err := forge.Reconcile(f, testClock(), dupDesired(), forge.Preconditions{})
	if err != nil {
		return forge.PublicationReceipt{}, err
	}
	after, err := botThreadCount(f, "platform/orders-service", "612")
	if err != nil {
		return forge.PublicationReceipt{}, err
	}
	if newArtifacts := after - before; newArtifacts != 0 {
		return forge.PublicationReceipt{}, fmt.Errorf("repair must create zero artifacts, created %d", newArtifacts)
	}
	return receipt, nil
}

func botThreadCount(f forge.Forge, project, mr string) (int, error) {
	threads, err := f.ListBotThreads(project, mr)
	if err != nil {
		return 0, err
	}
	return len(threads), nil
}

func botSummaryCount(f forge.Forge, project, mr string) (int, error) {
	notes, err := f.ListBotNotes(project, mr)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, note := range notes {
		if note.Marker.Artifact.Kind == "summary-comment" {
			n++
		}
	}
	return n, nil
}

func botSummaryCountMust(f forge.Forge, project, mr string) int {
	n, err := botSummaryCount(f, project, mr)
	if err != nil {
		panic(err)
	}
	return n
}

func threadOpTarget(receipt forge.PublicationReceipt, wantID string) string {
	for _, op := range receipt.Operations {
		if op.TargetID == wantID {
			return op.TargetID
		}
	}
	if len(receipt.Operations) > 0 {
		return receipt.Operations[0].TargetID
	}
	return ""
}
