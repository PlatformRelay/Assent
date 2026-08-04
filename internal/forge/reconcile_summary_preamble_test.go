package forge_test

import (
	"fmt"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
	"github.com/PlatformRelay/assent/internal/render"
)

func desiredWithSummary(thread *forge.DesiredThread) forge.DesiredReviewState {
	d := forge.DesiredReviewState{
		Project: proj,
		MR:      mrIID,
		Summary: &forge.DesiredSummary{
			Marker: summaryMarker(),
			Body:   fixtureSummaryBody(),
		},
	}
	if thread != nil {
		d.Thread = thread
	}
	return d
}

// TestReconcileSummaryPreamble proves REQ-E8-S12-02: when desired.Summary is
// populated, Reconcile runs the publication preamble (list notes → upsert summary)
// before the existing thread path, without breaking thread-only behaviour.
func TestReconcileSummaryPreamble(t *testing.T) {
	t.Run("creates-summary-then-thread", func(t *testing.T) {
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		d := desiredWithSummary(&forge.DesiredThread{
			Marker: reviewMarker(),
			Body:   "obligation not proven",
		})

		receipt, err := forge.Reconcile(f, testClock(), d, forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if got := f.SummaryNoteCount(); got != 1 {
			t.Fatalf("expected one summary note, got %d", got)
		}
		if got := f.BotThreadCount(); got != 1 {
			t.Fatalf("expected one bot thread, got %d", got)
		}
		if len(receipt.Operations) != 2 {
			t.Fatalf("expected summary+thread ops in receipt, got %d: %+v", len(receipt.Operations), receipt.Operations)
		}
	})

	t.Run("updates-summary-in-place-on-rerun", func(t *testing.T) {
		f := fake.New(botID, "src", "tgt", "sha256:merge")
		f.SeedNote("note/9000", botID, summaryMarker(), "old summary")
		f.SeedThread("note/9002", botID, reviewMarker(), false)

		d := desiredWithSummary(&forge.DesiredThread{
			Marker: reviewMarker(),
			Body:   "obligation not proven",
		})

		beforeSummaries := f.SummaryNoteCount()
		_, err := forge.Reconcile(f, testClock(), d, forge.Preconditions{})
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if got := f.SummaryNoteCount() - beforeSummaries; got != 0 {
			t.Fatalf("rerun must not create a new summary note, created %d", got)
		}
		wantSummary, err := render.Envelope(summaryMarker(), fixtureSummaryBody())
		if err != nil {
			t.Fatal(err)
		}
		if got := f.NoteBody("note/9000"); got != wantSummary {
			t.Fatalf("summary must be updated in place, got %q", got)
		}
	})

	t.Run("preamble-list-error-fails-closed", func(t *testing.T) {
		inner := fake.New(botID, "src", "tgt", "sha256:merge")
		f := &listNotesFailForge{inner: inner}
		d := desiredWithSummary(&forge.DesiredThread{Marker: reviewMarker(), Body: "x"})
		_, err := forge.Reconcile(f, testClock(), d, forge.Preconditions{})
		if err == nil {
			t.Fatal("expected error when ListBotNotes fails")
		}
		if got := inner.BotThreadCount(); got != 0 {
			t.Fatalf("ListBotNotes failure must not create threads, got %d", got)
		}
		if got := inner.SummaryNoteCount(); got != 0 {
			t.Fatalf("ListBotNotes failure must not create summary notes, got %d", got)
		}
	})
}

// listNotesFailForge wraps a Forge and injects a ListBotNotes failure for the
// summary preamble fail-closed path.
type listNotesFailForge struct {
	inner forge.Forge
}

func (f *listNotesFailForge) ListBotThreads(project, mr string) ([]forge.Thread, error) {
	return f.inner.ListBotThreads(project, mr)
}
func (f *listNotesFailForge) CurrentHeads(project, mr string) (string, string, string, error) {
	return f.inner.CurrentHeads(project, mr)
}
func (f *listNotesFailForge) CreateThread(project, mr string, marker forge.Marker, body string) (forge.Thread, error) {
	return f.inner.CreateThread(project, mr, marker, body)
}
func (f *listNotesFailForge) ResolveThread(project, mr, id string) error {
	return f.inner.ResolveThread(project, mr, id)
}
func (f *listNotesFailForge) Approve(project, mr string) (string, error) {
	return f.inner.Approve(project, mr)
}
func (f *listNotesFailForge) MergeCAS(project, mr string, m forge.DesiredMerge) (string, error) {
	return f.inner.MergeCAS(project, mr, m)
}
func (f *listNotesFailForge) ListBotNotes(_, _ string) ([]forge.Note, error) {
	return nil, fmt.Errorf("injected list notes failure")
}
func (f *listNotesFailForge) UpsertComment(project, mr string, marker forge.Marker, body string) (forge.Note, error) {
	return f.inner.UpsertComment(project, mr, marker, body)
}
