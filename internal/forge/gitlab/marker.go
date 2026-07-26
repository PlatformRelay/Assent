package gitlab

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/PlatformRelay/assent/internal/forge"
)

// markerSentinel is the opening token of the hidden-HTML marker (ADR-0016 §1,
// docs/contracts/p3-e5-publication-protocol/marker-grammar.md): a bot artifact
// carries the ADR-0019 correlation marker as an HTML comment
// `<!-- assent:marker {json} -->` outside any user-customisable slot. The
// Reconcile port reads it ONLY to answer "which slot/occurrence/decision
// produced this artifact"; it is never presentation and never decision input.
const markerSentinel = "assent:marker"

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

// renderMarker serialises a forge.Marker to the hidden-HTML comment form. The
// JSON is marshalled deterministically (encoding/json emits struct fields in
// declaration order), so render/parse round-trips byte-for-byte — the
// (slot, occurrence) idempotence key depends on that stability.
func renderMarker(m forge.Marker) (string, error) {
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
		return "", fmt.Errorf("gitlab: render marker: %w", err)
	}
	return fmt.Sprintf("<!-- %s %s -->", markerSentinel, payload), nil
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
