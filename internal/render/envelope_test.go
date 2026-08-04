package render

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

func sampleMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "42",
			MR:       "7",
			Rule:     "partitions-monotonic",
			EntryRef: "file:topics/orders.yaml",
			Effect:   "challenge",
		},
		Occurrence: "sha256:" + strings.Repeat("a", 64),
		Decision:   "sha256:" + strings.Repeat("b", 64),
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

var markerCommentRe = regexp.MustCompile(`<!--\s*` + regexp.QuoteMeta(MarkerSentinel) + `\s*\{.*?\}\s*-->`)

// TestEnvelopeMarkerOutsideBody (REQ-E8-S04-01) proves Envelope emits exactly one
// well-formed hidden-HTML marker outside the markdown body region.
func TestEnvelopeMarkerOutsideBody(t *testing.T) {
	t.Parallel()
	body := "## Headline\n\nSome finding details."
	out, err := Envelope(sampleMarker(), body)
	if err != nil {
		t.Fatalf("Envelope: %v", err)
	}
	matches := markerCommentRe.FindAllString(out, -1)
	if len(matches) != 1 {
		t.Fatalf("want exactly one marker comment, got %d in %q", len(matches), out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "<!-- "+MarkerSentinel) {
		t.Errorf("marker must precede body; got %q", out)
	}
	afterMarker := strings.TrimPrefix(out, matches[0])
	afterMarker = strings.TrimLeft(afterMarker, "\n")
	if afterMarker != body {
		t.Errorf("body region mismatch:\n got %q\nwant %q", afterMarker, body)
	}
	if strings.Contains(body, MarkerSentinel) {
		t.Fatal("test setup: body must not contain sentinel")
	}
	commentOnly, err := FormatMarker(sampleMarker())
	if err != nil {
		t.Fatal(err)
	}
	if matches[0] != commentOnly {
		t.Errorf("marker comment not well-formed:\n got %q\nwant %q", matches[0], commentOnly)
	}
}

// TestEnvelopeRejectsEmbeddedSentinel (REQ-E8-S04-02) proves bodies carrying the
// marker sentinel or a premature HTML comment close fail closed at envelope time.
func TestEnvelopeRejectsEmbeddedSentinel(t *testing.T) {
	t.Parallel()
	m := sampleMarker()
	cases := []struct {
		name    string
		body    string
		wantErr error
	}{
		{name: "sentinel substring", body: "do not forge <!-- assent:marker {} -->", wantErr: ErrEmbeddedMarkerSentinel},
		{name: "bare sentinel", body: "contains assent:marker token", wantErr: ErrEmbeddedMarkerSentinel},
		{name: "premature close", body: "broken <!-- not a marker -->", wantErr: ErrPrematureCommentClose},
		{name: "renderer details close allowed", body: "<details>\n<summary>x</summary>\n\ny\n\n</details>", wantErr: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Envelope(m, tc.body)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Envelope(%q): got %v, want nil", tc.body, err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Envelope(%q): got %v, want %v", tc.body, err, tc.wantErr)
			}
		})
	}
}
