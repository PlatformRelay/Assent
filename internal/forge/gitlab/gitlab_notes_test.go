package gitlab

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

func summaryMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project: "42",
			MR:      "7",
			Rule:    "assent/summary",
			Effect:  "comment",
		},
		Occurrence: "sha256:" + strings.Repeat("b", 64),
		Decision:   "sha256:" + strings.Repeat("c", 64),
		Artifact:   forge.Artifact{Kind: "summary-comment", SchemaVersion: "v1alpha1"},
	}
}

func TestListBotNotesAuthorIdentityFilter(t *testing.T) {
	m := summaryMarker()
	rendered, err := renderMarker(m)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		if r.URL.Path != "/api/v4/projects/42/merge_requests/7/notes" {
			http.Error(w, "unexpected", http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("page") != "1" {
			_, _ = io.WriteString(w, `[]`)
			return
		}
		resp := []map[string]any{
			{"id": 9000, "body": rendered + "\n\nbot summary", "author": map[string]any{"username": botUser}},
			{"id": 6660, "body": rendered + "\n\nspoofed", "author": map[string]any{"username": "mallory"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	notes, err := c.ListBotNotes("42", "7")
	if err != nil {
		t.Fatalf("ListBotNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("got %d notes, want 1 (contributor marker excluded)", len(notes))
	}
	if notes[0].ID != "note/9000" {
		t.Errorf("note id = %q, want note/9000", notes[0].ID)
	}
}

func TestUpsertCommentRejectsNonSummaryKind(t *testing.T) {
	c, _ := newServer(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("must not call GitLab when marker kind is invalid")
	})
	m := botMarker() // finding-thread
	_, err := c.UpsertComment("42", "7", m, "body")
	if !errors.Is(err, forge.ErrInvalidSummaryMarker) {
		t.Fatalf("expected ErrInvalidSummaryMarker, got %v", err)
	}
}

func TestUpsertCommentCreateThenUpdate(t *testing.T) {
	m := summaryMarker()
	rendered, err := renderMarker(m)
	if err != nil {
		t.Fatal(err)
	}
	var created bool
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/merge_requests/7/notes":
			if created {
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": 9000, "body": rendered + "\n\nupdated", "author": map[string]any{"username": botUser}},
				})
			} else {
				_, _ = io.WriteString(w, `[]`)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/merge_requests/7/notes":
			created = true
			_, _ = io.WriteString(w, `{"id":9000,"body":"created"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/projects/42/merge_requests/7/notes/9000":
			_, _ = io.WriteString(w, `{"id":9000,"body":"updated"}`)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	first, err := c.UpsertComment("42", "7", m, "initial")
	if err != nil {
		t.Fatalf("UpsertComment create: %v", err)
	}
	if first.ID != "note/9000" {
		t.Fatalf("create id = %q, want note/9000", first.ID)
	}

	second, err := c.UpsertComment("42", "7", m, "updated")
	if err != nil {
		t.Fatalf("UpsertComment update: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("update must reuse id %q, got %q", first.ID, second.ID)
	}
}

func TestListBotNotesPagination(t *testing.T) {
	m := summaryMarker()
	rendered, err := renderMarker(m)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantToken(t, r)
		switch r.URL.Query().Get("page") {
		case "1":
			var page []map[string]any
			for i := 0; i < 100; i++ {
				page = append(page, map[string]any{
					"id": i, "body": "no marker", "author": map[string]any{"username": botUser},
				})
			}
			_ = json.NewEncoder(w).Encode(page)
		case "2":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 9000, "body": rendered + "\n\nsummary", "author": map[string]any{"username": botUser}},
			})
		default:
			_, _ = io.WriteString(w, `[]`)
		}
	})

	notes, err := c.ListBotNotes("42", "7")
	if err != nil {
		t.Fatalf("ListBotNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != "note/9000" {
		t.Fatalf("pagination: got %+v, want single note/9000", notes)
	}
}

func TestListBotNotesUnexpectedStatus(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	_, err := c.ListBotNotes("42", "7")
	if err == nil {
		t.Fatal("expected error on unexpected status")
	}
}
