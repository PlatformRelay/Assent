package forge_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
	"github.com/PlatformRelay/assent/internal/render"
)

// summaryMarker builds the ADR-0019 marker for the per-MR summary slot used
// across summary-port tests. Body may be a placeholder in S12 — full render wiring
// is E8-S13.
func summaryMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project: proj,
			MR:      mrIID,
			Rule:    "assent/summary",
			Effect:  "comment",
		},
		Occurrence: decHex,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "summary-comment", SchemaVersion: "v1alpha1"},
	}
}

// TestUpsertCommentIdempotent proves REQ-E8-S12-01: UpsertComment creates a
// summary note on first call, then updates it in place on subsequent calls —
// same forge id, zero second summary notes.
func TestUpsertCommentIdempotent(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	m := summaryMarker()

	first, err := f.UpsertComment(proj, mrIID, m, "summary v1")
	if err != nil {
		t.Fatalf("UpsertComment create: %v", err)
	}
	if first.ID == "" {
		t.Fatal("create must return a forge-assigned id")
	}
	if got := f.SummaryNoteCount(); got != 1 {
		t.Fatalf("first upsert must create exactly one summary note, got %d", got)
	}

	second, err := f.UpsertComment(proj, mrIID, m, "summary v2")
	if err != nil {
		t.Fatalf("UpsertComment update: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("update must reuse the same forge id, got %q want %q", second.ID, first.ID)
	}
	if got := f.SummaryNoteCount(); got != 1 {
		t.Fatalf("update must not create a second summary note, got %d", got)
	}
	wantBody, err := render.Envelope(m, "summary v2")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.NoteBody(first.ID); got != wantBody {
		t.Fatalf("body must be updated in place, got %q want %q", got, wantBody)
	}
}
