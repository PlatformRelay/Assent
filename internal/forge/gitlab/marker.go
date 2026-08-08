package gitlab

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/render"
)

// markerSentinel is the opening token of the hidden-HTML marker (ADR-0016 §1,
// docs/contracts/p3-e5-publication-protocol/marker-grammar.md): a bot artifact
// carries the ADR-0019 correlation marker as an HTML comment
// `<!-- assent:marker {json} -->` outside any user-customisable slot. The
// Reconcile port reads it ONLY to answer "which slot/occurrence/decision
// produced this artifact"; it is never presentation and never decision input.
const markerSentinel = render.MarkerSentinel

// markerRe extracts the JSON payload of the marker HTML comment from a note
// body. It is anchored on the exact sentinel so a stray HTML comment in a
// human body is never mistaken for a marker. The payload is captured lazily up
// to the closing `-->`.
var markerRe = regexp.MustCompile(`<!--\s*` + regexp.QuoteMeta(markerSentinel) + `\s*(\{.*?\})\s*-->`)

// markerJSON is the wire shape of the ADR-0019 marker payload, matching
// docs/contracts/p3-e5-publication-protocol/marker-grammar.schema.json EXACTLY:
// four top-level concepts (slot, occurrence, decision, artifact). The forge.Slot
// carries only Project/MR/Rule/Effect/EntryRef (no obligation/anchor), so only
// those slot fields are rendered — all are schema-optional except
// project/mr/rule/effect, so the payload stays schema-valid. entryRef is omitted
// when empty (never an empty string standing in for "none").
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

// renderMarker serialises a forge.Marker to the hidden-HTML comment form. Marker
// bytes are owned by internal/render (D-094); gitlab delegates assembly here.
func renderMarker(m forge.Marker) (string, error) {
	comment, err := render.FormatMarker(m)
	if err != nil {
		return "", fmt.Errorf("gitlab: render marker: %w", err)
	}
	return comment, nil
}

// markerSkipWarning is the operator-facing sentence recorded on the
// PublicationReceipt when a BOT-authored artifact's marker payload cannot be
// decoded (AUD-S12 / REL-06). It names the artifact so the operator can find
// and delete it, and states plainly that reconcile continued.
//
// It embeds the DECODER's error only — never the raw payload — so a corrupted
// marker cannot smuggle arbitrary text into the run summary, and the string
// stays deterministic across runs (the receipt is compared byte-for-byte by the
// determinism gate).
func markerSkipWarning(kind, id string, err error) string {
	return fmt.Sprintf(
		"gitlab: skipped bot %s %s — malformed marker payload (%v); reconcile continued and the artifact was left in place",
		kind, id, err)
}

// parseMarker extracts and decodes the ADR-0019 marker from a note body. It
// returns (marker, true, nil) when a well-formed marker is present, (_, false,
// nil) when the body carries no marker, and an error only when a marker sentinel
// is present but its payload is malformed JSON. A missing marker is NOT an error
// (a bot note may legitimately carry none in this slice — the caller treats a
// markerless discussion as not-a-finding-thread).
func parseMarker(body string) (forge.Marker, bool, error) {
	sub := markerRe.FindStringSubmatch(body)
	if sub == nil {
		return forge.Marker{}, false, nil
	}
	var mj markerJSON
	if err := json.Unmarshal([]byte(sub[1]), &mj); err != nil {
		return forge.Marker{}, false, fmt.Errorf("gitlab: parse marker payload: %w", err)
	}
	m := forge.Marker{
		Slot: forge.Slot{
			Project:  mj.Slot.Project,
			MR:       mj.Slot.MR,
			Rule:     mj.Slot.Rule,
			EntryRef: mj.Slot.EntryRef,
			Effect:   mj.Slot.Effect,
		},
		Occurrence: mj.Occurrence,
		Decision:   mj.Decision,
		Artifact: forge.Artifact{
			Kind:          mj.Artifact.Kind,
			SchemaVersion: mj.Artifact.SchemaVersion,
		},
	}
	return m, true, nil
}
