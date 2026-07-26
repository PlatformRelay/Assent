package forge_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/schemas"
)

// fixedClock is the injected, deterministic time source. Reconcile never calls
// time.Now — a fixed clock makes performedAt byte-stable so receipts double-run
// identically (ADR-0013).
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// testClock is a single frozen instant used across the receipt goldens.
func testClock() fixedClock {
	return fixedClock{t: time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)}
}

const (
	botID  = "assent-bot"
	proj   = "platform/orders-service"
	mrIID  = "482"
	occOne = "sha256:c6957a516c95532386bed08f56441dfbb8d18efda24f5abdab1e48437aa3357d"
	decHex = "sha256:1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaaa"
)

// reviewMarker builds the ADR-0019 marker for the REVIEW slot used across the
// thread tests. Tests construct the marker with literal sha256 values (like
// rerun-idempotence.yaml) — Reconcile consumes the marker, it never derives it.
func reviewMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  proj,
			MR:       mrIID,
			Rule:     "ownership/entry-owner-required",
			Effect:   "comment",
			EntryRef: "topic-registry:orders.events.v1",
		},
		Occurrence: occOne,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

// reviewState is the DesiredReviewState for a REVIEW decision: exactly one
// desired resolvable thread carrying the marker.
func reviewState() forge.DesiredReviewState {
	return forge.DesiredReviewState{
		Project: proj,
		MR:      mrIID,
		Thread:  &forge.DesiredThread{Marker: reviewMarker(), Body: "obligation not proven"},
	}
}

// validateReceipt validates a PublicationReceipt's JSON bytes against the frozen
// publication-receipt schema, mirroring the decision package's validation
// pattern.
func validateReceipt(t *testing.T, raw []byte) error {
	t.Helper()
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("unmarshal receipt for validation: %v", err)
	}
	return schemas.PublicationReceiptSchema.Validate(parsed)
}
