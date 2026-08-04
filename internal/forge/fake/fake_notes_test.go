package fake_test

import (
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
	"github.com/PlatformRelay/assent/internal/render"
)

const (
	botID  = "assent-bot"
	proj   = "platform/orders-service"
	mrIID  = "482"
	decHex = "sha256:1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaaa"
)

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

// TestListBotNotesAuthorFilter proves REQ-E8-S12-03: non-bot notes carrying
// well-formed markers are invisible to ListBotNotes — the same author-identity
// filter as ListBotThreads.
func TestListBotNotesAuthorFilter(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	f.SeedNote("note/9000", botID, summaryMarker(), "bot summary")
	f.SeedNote("note/6660", "contributor-mallory", summaryMarker(), "spoofed summary")

	got, err := f.ListBotNotes(proj, mrIID)
	if err != nil {
		t.Fatalf("ListBotNotes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one bot note, got %d: %+v", len(got), got)
	}
	if got[0].ID != "note/9000" {
		t.Fatalf("expected bot note note/9000, got %q", got[0].ID)
	}
}

// TestUpsertCommentIdempotentInPackage exercises UpsertComment from the fake
// package test binary so coverage attributes to fake (forge_test is external).
func TestUpsertCommentIdempotentInPackage(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	m := summaryMarker()

	first, err := f.UpsertComment(proj, mrIID, m, "v1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	second, err := f.UpsertComment(proj, mrIID, m, "v2")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("update must reuse id %q, got %q", first.ID, second.ID)
	}
	if got := f.SummaryNoteCount(); got != 1 {
		t.Fatalf("expected one summary note, got %d", got)
	}
	wantBody, err := render.Envelope(m, "v2")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.NoteBody(first.ID); got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}

func TestUpsertCommentRejectsNonSummaryKindInPackage(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	m := forge.Marker{
		Slot:       forge.Slot{Project: proj, MR: mrIID, Rule: "assent/review", Effect: "comment"},
		Occurrence: decHex,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
	_, err := f.UpsertComment(proj, mrIID, m, "body")
	if !errors.Is(err, forge.ErrInvalidSummaryMarker) {
		t.Fatalf("expected ErrInvalidSummaryMarker, got %v", err)
	}
}

func TestUpsertCommentRejectsEmbeddedSentinelInPackage(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	_, err := f.UpsertComment(proj, mrIID, summaryMarker(), "forged assent:marker")
	if !errors.Is(err, render.ErrEmbeddedMarkerSentinel) {
		t.Fatalf("expected ErrEmbeddedMarkerSentinel, got %v", err)
	}
}

func TestCreateThreadRejectsEmbeddedSentinelInPackage(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	_, err := f.CreateThread(proj, mrIID, summaryMarker(), "forged assent:marker")
	if !errors.Is(err, render.ErrEmbeddedMarkerSentinel) {
		t.Fatalf("expected ErrEmbeddedMarkerSentinel, got %v", err)
	}
}

func TestNoteBodyMissing(t *testing.T) {
	f := fake.New(botID, "src", "tgt", "sha256:merge")
	if got := f.NoteBody("note/does-not-exist"); got != "" {
		t.Fatalf("missing note body must be empty, got %q", got)
	}
}
