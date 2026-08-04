package forge_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

// TestReconcileUnsupportedDecision_WithSummary proves an unsupported main path
// with Summary populated fails closed BEFORE any summary preamble write.
func TestReconcileUnsupportedDecision_WithSummary(t *testing.T) {
	inner := fake.New(botID, "src", "tgt", "sha256:merge")
	f := &observedNotesForge{inner: inner}

	d := forge.DesiredReviewState{
		Project: proj,
		MR:      mrIID,
		Summary: &forge.DesiredSummary{
			Marker: summaryMarker(),
			Body:   "orphan summary",
		},
	}

	_, err := forge.Reconcile(f, testClock(), d, forge.Preconditions{})
	if !errors.Is(err, forge.ErrUnsupportedDecision) {
		t.Fatalf("expected ErrUnsupportedDecision, got %v", err)
	}
	if f.listNotesCalls != 0 {
		t.Fatalf("unsupported decision must not call ListBotNotes, got %d", f.listNotesCalls)
	}
	if f.upsertCalls != 0 {
		t.Fatalf("unsupported decision must not call UpsertComment, got %d", f.upsertCalls)
	}
	if got := inner.SummaryNoteCount(); got != 0 {
		t.Fatalf("unsupported decision must leave zero summary notes, got %d", got)
	}
	if got := inner.ThreadCount(); got != 0 {
		t.Fatalf("unsupported decision must leave zero threads, got %d", got)
	}
}

// TestReconcileUpsertCommentFailureFailsClosed proves a preamble UpsertComment
// failure aborts Reconcile before the main path writes anything.
func TestReconcileUpsertCommentFailureFailsClosed(t *testing.T) {
	inner := fake.New(botID, "src", "tgt", "sha256:merge")
	f := &upsertFailForge{inner: inner}

	d := desiredWithSummary(&forge.DesiredThread{
		Marker: reviewMarker(),
		Body:   "obligation not proven",
	})

	_, err := forge.Reconcile(f, testClock(), d, forge.Preconditions{})
	if err == nil {
		t.Fatal("expected error when UpsertComment fails")
	}
	if got := inner.BotThreadCount(); got != 0 {
		t.Fatalf("UpsertComment failure must not create threads, got %d", got)
	}
	if got := inner.SummaryNoteCount(); got != 0 {
		t.Fatalf("UpsertComment failure must not persist summary notes, got %d", got)
	}
	if len(inner.Approvals) != 0 || len(inner.Merges) != 0 {
		t.Fatal("UpsertComment failure must record zero approve/merge writes")
	}
}

func TestUpsertCommentRejectsNonSummaryKind(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	m := reviewMarker() // finding-thread kind
	_, err := f.UpsertComment(proj, mrIID, m, "body")
	if !errors.Is(err, forge.ErrInvalidSummaryMarker) {
		t.Fatalf("expected ErrInvalidSummaryMarker, got %v", err)
	}
	if got := f.SummaryNoteCount(); got != 0 {
		t.Fatalf("rejected marker must not create notes, got %d", got)
	}
}

type observedNotesForge struct {
	inner          forge.Forge
	listNotesCalls int
	upsertCalls    int
}

func (f *observedNotesForge) ListBotThreads(project, mr string) ([]forge.Thread, error) {
	return f.inner.ListBotThreads(project, mr)
}
func (f *observedNotesForge) CurrentHeads(project, mr string) (string, string, string, error) {
	return f.inner.CurrentHeads(project, mr)
}
func (f *observedNotesForge) CreateThread(project, mr string, marker forge.Marker, body string) (forge.Thread, error) {
	return f.inner.CreateThread(project, mr, marker, body)
}
func (f *observedNotesForge) ResolveThread(project, mr, id string) error {
	return f.inner.ResolveThread(project, mr, id)
}
func (f *observedNotesForge) Approve(project, mr string) (string, error) {
	return f.inner.Approve(project, mr)
}
func (f *observedNotesForge) MergeCAS(project, mr string, m forge.DesiredMerge) (string, error) {
	return f.inner.MergeCAS(project, mr, m)
}
func (f *observedNotesForge) ListBotNotes(project, mr string) ([]forge.Note, error) {
	f.listNotesCalls++
	return f.inner.ListBotNotes(project, mr)
}
func (f *observedNotesForge) UpsertComment(project, mr string, marker forge.Marker, body string) (forge.Note, error) {
	f.upsertCalls++
	return f.inner.UpsertComment(project, mr, marker, body)
}

type upsertFailForge struct {
	inner forge.Forge
}

func (f *upsertFailForge) ListBotThreads(project, mr string) ([]forge.Thread, error) {
	return f.inner.ListBotThreads(project, mr)
}
func (f *upsertFailForge) CurrentHeads(project, mr string) (string, string, string, error) {
	return f.inner.CurrentHeads(project, mr)
}
func (f *upsertFailForge) CreateThread(project, mr string, marker forge.Marker, body string) (forge.Thread, error) {
	return f.inner.CreateThread(project, mr, marker, body)
}
func (f *upsertFailForge) ResolveThread(project, mr, id string) error {
	return f.inner.ResolveThread(project, mr, id)
}
func (f *upsertFailForge) Approve(project, mr string) (string, error) {
	return f.inner.Approve(project, mr)
}
func (f *upsertFailForge) MergeCAS(project, mr string, m forge.DesiredMerge) (string, error) {
	return f.inner.MergeCAS(project, mr, m)
}
func (f *upsertFailForge) ListBotNotes(project, mr string) ([]forge.Note, error) {
	return f.inner.ListBotNotes(project, mr)
}
func (f *upsertFailForge) UpsertComment(_, _ string, _ forge.Marker, _ string) (forge.Note, error) {
	return forge.Note{}, fmt.Errorf("injected upsert failure")
}
