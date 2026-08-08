package main

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/forge"
)

// AUD-S12 / REL-06: the malformed-bot-marker skip is only real if the operator
// SEES it. A warning that reaches the PublicationReceipt and stops there is a
// struct field, not a behaviour — this pins that `assent run`'s one-line
// summary surfaces it, and (the other polarity) that a clean run's summary is
// byte-identical to before, so no existing summary assertion moves.
func TestSummarySurfacesReconcileWarnings(t *testing.T) {
	clean := forge.PublicationReceipt{
		Operations: []forge.Operation{{Kind: "thread", TargetID: "note/1"}},
	}
	warned := clean
	warned.Warnings = []string{
		"gitlab: skipped bot discussion d-corrupt — malformed marker payload (boom)",
	}

	base := summarize(aggregate.DecisionReview, false, clean, nil)
	if strings.Contains(base, "warning") {
		t.Fatalf("a clean reconcile summary must not mention warnings, got %q", base)
	}

	got := summarize(aggregate.DecisionReview, false, warned, nil)
	if !strings.HasPrefix(got, base) {
		t.Fatalf("the warning must be a SUFFIX so existing summaries stay stable:\n got %q\nwant prefix %q", got, base)
	}
	if !strings.Contains(got, "1 forge warning") {
		t.Fatalf("summary must state the warning count, got %q", got)
	}
	if !strings.Contains(got, "d-corrupt") {
		t.Fatalf("summary must name the skipped artifact so the operator can repair it, got %q", got)
	}

	// Two warnings still produce ONE line (the summary is a single line by
	// contract) and both are named.
	warned.Warnings = append(warned.Warnings, "gitlab: skipped bot note note/900 — malformed marker payload (boom)")
	multi := summarize(aggregate.DecisionReview, false, warned, nil)
	if strings.Contains(multi, "\n") {
		t.Fatalf("the run summary must stay a single line, got %q", multi)
	}
	if !strings.Contains(multi, "2 forge warning") ||
		!strings.Contains(multi, "d-corrupt") || !strings.Contains(multi, "note/900") {
		t.Fatalf("both warnings must be named, got %q", multi)
	}
}
