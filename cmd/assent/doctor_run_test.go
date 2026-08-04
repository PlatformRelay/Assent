package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// D1: run(["doctor"]) is the documented testable entry point and its exit code
// IS the security-relevant gate signal to CI: 0 = armed, 1 = advisory/not-armed,
// 2 = unknown command. This drives run() end-to-end through the env boundary
// (readPipelineDescription) with t.Setenv, asserting the behavioural contract —
// exit code plus the typed reason printed to stderr on refusal. A protected +
// non-author-editable env arms; an unprotected/unverifiable env refuses and
// surfaces the typed reason.
func TestRunDoctorExitCodeGate(t *testing.T) {
	t.Run("verified-protected non-author-editable pipeline arms (exit 0)", func(t *testing.T) {
		t.Setenv("GITLAB_TOKEN", "")
		t.Setenv("CI_PROJECT_ID", "")
		t.Setenv("CI_MERGE_REQUEST_IID", "")
		t.Setenv("ASSENT_PIPELINE_CONFIG_PROTECTED", "true")
		t.Setenv("ASSENT_PIPELINE_CONFIG_AUTHOR_EDITABLE", "false")
		t.Setenv("ASSENT_PIPELINE_TOKEN_PRIVILEGED", "false")

		code, stdout, stderr := captureRun(t, []string{"doctor"})
		if code != 0 {
			t.Fatalf("armed pipeline: run([\"doctor\"]) exit = %d, want 0 (armed); stderr=%q stdout=%q", code, stderr, stdout)
		}
		if !strings.Contains(stderr, "INSECURE") {
			t.Errorf("env-only armed path must still print INSECURE banner; stderr=%q", stderr)
		}
	})

	t.Run("unprotected/unverifiable pipeline refuses (exit 1) with typed reason on stderr", func(t *testing.T) {
		t.Setenv("GITLAB_TOKEN", "")
		t.Setenv("CI_PROJECT_ID", "")
		t.Setenv("CI_MERGE_REQUEST_IID", "")
		// PROTECTED unset -> reader reads false -> fail-safe not-protected.
		t.Setenv("ASSENT_PIPELINE_CONFIG_PROTECTED", "")
		t.Setenv("ASSENT_PIPELINE_CONFIG_AUTHOR_EDITABLE", "false")
		t.Setenv("ASSENT_PIPELINE_TOKEN_PRIVILEGED", "false")

		code, _, stderr := captureRun(t, []string{"doctor"})
		if code != 1 {
			t.Fatalf("unprotected pipeline: run([\"doctor\"]) exit = %d, want 1 (advisory/not-armed); stderr=%q", code, stderr)
		}
		// The typed reason code must be surfaced to CI on the refusal path.
		if !strings.Contains(stderr, string(ReasonProtectedConfigUnverified)) {
			t.Errorf("refusal stderr must carry the typed reason %q; got %q", ReasonProtectedConfigUnverified, stderr)
		}
	})

	t.Run("unknown subcommand (exit 2)", func(t *testing.T) {
		code, _, _ := captureRun(t, []string{"no-such-cmd"})
		if code != 2 {
			t.Fatalf("unknown subcommand exit = %d, want 2", code)
		}
	})
}

// captureRun invokes run(args) with os.Stdout/os.Stderr redirected to pipes so
// the behavioural output can be asserted. It restores the originals via t.Cleanup.
func captureRun(t *testing.T, args []string) (code int, stdout, stderr string) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stdout, os.Stderr = wOut, wErr
	t.Cleanup(func() { os.Stdout, os.Stderr = origOut, origErr })

	code = run(args)

	_ = wOut.Close()
	_ = wErr.Close()
	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)
	return code, string(outBytes), string(errBytes)
}
