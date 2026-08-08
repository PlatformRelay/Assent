package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/render"
)

// corruptBotMarkerBody is a bot-authored note carrying the real marker SENTINEL
// with a payload the extraction regexp matches but the decoder rejects — the
// AUD-S12 / REL-06 corruption.
func corruptBotMarkerBody() string {
	return "<!-- " + render.MarkerSentinel + ` {"slot": 12345} -->` + "\n\nstale finding"
}

// TestRunSurfacesForgeWarningOnUnarmedRefusal — AUD-S12 / REL-06, review
// finding F1.
//
// `Reconcile` returns a typed REFUSAL (ErrArmingRefused) for an APPROVE
// decision the forge will not arm. That is an expected, exit-0, advisory-only
// outcome — and UNARMED IS THE DEFAULT ADOPTER POSTURE. If the warning is
// attached only on the success return, a run that skips a corrupted bot marker
// is completely silent for most adopters, which is exactly the invisibility
// this story exists to remove.
//
// This drives the real CLI entry point (`runRun` → the real gitlab.Client →
// forge.Reconcile → summarize → stdout), not the seam: an unwired warning fails
// it.
func TestRunSurfacesForgeWarningOnUnarmedRefusal(t *testing.T) {
	f := newFakeGitLab(t)
	f.projectJSON = fakeForgeIneligibleProjectJSON // forge refuses to arm
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n" // an increase → APPROVE decision
	f.discussions = append(f.discussions, fakeDiscussion{id: "disc-corrupt", body: corruptBotMarkerBody()})

	var out bytes.Buffer
	code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory())
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (advisory refusal)\n%s", code, out.String())
	}

	summary := lastLine(out.String())

	// The refusal itself is unchanged — this must remain an advisory no-write run.
	if !strings.Contains(summary, "advisory-only") {
		t.Fatalf("the arming refusal must still be reported, got %q", summary)
	}
	if f.approvals != 0 || f.merges != 0 {
		t.Fatalf("a refusal must write nothing: approvals=%d merges=%d", f.approvals, f.merges)
	}

	// ... and the skipped marker reaches the operator ANYWAY.
	if !strings.Contains(summary, "forge warning") {
		t.Fatalf("a refusal must not swallow the malformed-marker warning, got %q", summary)
	}
	if !strings.Contains(summary, "disc-corrupt") {
		t.Fatalf("the summary must name the artifact the operator has to repair, got %q", summary)
	}
}

// TestRunEmitsNoWarningSuffixOnCleanRefusal is the POSITIVE CONTROL for the
// above: with no corrupted marker the SAME refusal path prints the SAME line it
// printed before this change — byte-identical, no empty "[0 forge warning(s)]"
// noise. Without it, unconditionally appending a suffix would pass.
func TestRunEmitsNoWarningSuffixOnCleanRefusal(t *testing.T) {
	f := newFakeGitLab(t)
	f.projectJSON = fakeForgeIneligibleProjectJSON
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n"

	var out bytes.Buffer
	if code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory()); code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}

	summary := lastLine(out.String())
	if !strings.Contains(summary, "advisory-only") {
		t.Fatalf("expected the advisory refusal, got %q", summary)
	}
	if strings.Contains(summary, "warning") {
		t.Fatalf("a clean refusal summary must be unchanged, got %q", summary)
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}
