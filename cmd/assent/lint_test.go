package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestLintCommand — REQ-E3-S01-04: `assent lint <dir>` discovers `.assent/**`,
// runs the check library, prints located diagnostics, and returns the correct
// exit code (0 clean, non-zero on any error).
func TestLintCommand(t *testing.T) {
	t.Run("covered pack exits 0 with no output", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runLint([]string{"testdata/lint/covered"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("clean lint must print no diagnostics, got:\n%s", stderr.String())
		}
	})

	t.Run("uncovered obligation exits non-zero with a located diagnostic", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runLint([]string{"testdata/lint/uncovered"}, &stdout, &stderr)
		if code == 0 {
			t.Fatalf("exit code = 0, want non-zero for an uncovered obligation")
		}
		out := stderr.String()
		if !strings.Contains(out, "obligation-coverage") {
			t.Errorf("stderr must name the obligation-coverage code, got:\n%s", out)
		}
		if !strings.Contains(out, "freshness") {
			t.Errorf("stderr must name the uncovered obligation, got:\n%s", out)
		}
		if !strings.Contains(out, "bindings.yaml") {
			t.Errorf("stderr must locate the diagnostic to the binding file, got:\n%s", out)
		}
	})

	t.Run("missing directory argument exits non-zero", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runLint(nil, &stdout, &stderr)
		if code == 0 {
			t.Fatal("missing <dir> argument must exit non-zero")
		}
	})

	t.Run("directory without an .assent tree exits non-zero", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runLint([]string{"testdata/lint"}, &stdout, &stderr)
		if code == 0 {
			t.Fatal("a directory with no .assent/** tree must exit non-zero")
		}
	})
}

// TestLintCommandDoubleRunStable — the command-level determinism guard: the same
// tree double-lints to byte-identical stderr (REQ-E3-S01-05, end to end).
func TestLintCommandDoubleRunStable(t *testing.T) {
	run := func() string {
		var stdout, stderr bytes.Buffer
		runLint([]string{"testdata/lint/uncovered"}, &stdout, &stderr)
		return stderr.String()
	}
	if first, second := run(), run(); first != second {
		t.Fatalf("double run not byte-identical:\n--- first ---\n%s--- second ---\n%s", first, second)
	}
}
