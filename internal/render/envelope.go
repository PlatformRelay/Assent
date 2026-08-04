package render

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/PlatformRelay/assent/internal/forge"
)

// MarkerSentinel is the opening token of the hidden-HTML marker comment
// (docs/contracts/p3-e5-publication-protocol/marker-grammar.md).
const MarkerSentinel = "assent:marker"

var (
	// ErrEmbeddedMarkerSentinel is returned when the body region carries the marker
	// sentinel and could forge or confuse reconciliation parsing.
	ErrEmbeddedMarkerSentinel = errors.New("render: body contains assent:marker sentinel")
	// ErrPrematureCommentClose is returned when the body region closes an HTML
	// comment prematurely and could truncate the envelope marker.
	ErrPrematureCommentClose = errors.New("render: body contains premature HTML comment close")
)

// markerJSON is the wire shape of the ADR-0019 marker payload, matching
// docs/contracts/p3-e5-publication-protocol/marker-grammar.schema.json.
type markerJSON struct {
	Slot struct {
		Project  string `json:"project"`
		MR       string `json:"mr"`
		Rule     string `json:"rule"`
		EntryRef string `json:"entryRef,omitempty"`
		Effect   string `json:"effect"`
	} `json:"slot"`
	Occurrence string `json:"occurrence"`
	Decision   string `json:"decision"`
	Artifact   struct {
		Kind          string `json:"kind"`
		SchemaVersion string `json:"schemaVersion"`
	} `json:"artifact"`
}

// FormatMarker serialises a forge.Marker to the hidden-HTML comment form. JSON
// field order follows struct declaration for deterministic round-trips.
func FormatMarker(m forge.Marker) (string, error) {
	var mj markerJSON
	mj.Slot.Project = m.Slot.Project
	mj.Slot.MR = m.Slot.MR
	mj.Slot.Rule = m.Slot.Rule
	mj.Slot.EntryRef = m.Slot.EntryRef
	mj.Slot.Effect = m.Slot.Effect
	mj.Occurrence = m.Occurrence
	mj.Decision = m.Decision
	mj.Artifact.Kind = m.Artifact.Kind
	mj.Artifact.SchemaVersion = m.Artifact.SchemaVersion

	payload, err := json.Marshal(mj)
	if err != nil {
		return "", fmt.Errorf("render: format marker: %w", err)
	}
	return fmt.Sprintf("<!-- %s %s -->", MarkerSentinel, payload), nil
}

// Envelope wraps markdown body content with exactly one renderer-owned marker
// comment outside the user/content region (ADR-0016 §1, D-094).
func Envelope(m forge.Marker, body string) (string, error) {
	if err := validateEnvelopeBody(body); err != nil {
		return "", err
	}
	marker, err := FormatMarker(m)
	if err != nil {
		return "", err
	}
	if body == "" {
		return marker, nil
	}
	return marker + "\n\n" + body, nil
}

func validateEnvelopeBody(body string) error {
	if strings.Contains(body, MarkerSentinel) {
		return ErrEmbeddedMarkerSentinel
	}
	if strings.Contains(body, "-->") {
		return ErrPrematureCommentClose
	}
	return nil
}
