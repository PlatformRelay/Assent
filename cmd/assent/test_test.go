package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// adopterRepo is the minimal single-rule fixture repo the pure library owns; the
// command test drives `assent test` end-to-end over it.
const adopterRepo = "../../internal/adoptertest/testdata/repo"

// TestTestCommand (REQ-E6-S01-04) proves `assent test <repo>` is wired into main.go's
// dispatch: it discovers `.assent/tests/**`, runs each case, and exits 0 when every
// case's decision matched its expect.yaml, non-zero on any mismatch, and 2 on a usage
// error.
func TestTestCommand(t *testing.T) {
	t.Run("all cases matching exits 0 and prints PASS", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runTest([]string{adopterRepo}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (all cases match)\nstdout:%s\nstderr:%s", code, stdout.String(), stderr.String())
		}
		out := stdout.String()
		for _, name := range []string{"capped/within-cap", "capped/over-cap", "capped/unresolved-fact"} {
			if !strings.Contains(out, "PASS "+name) {
				t.Fatalf("stdout missing PASS for %q:\n%s", name, out)
			}
		}
	})

	t.Run("a decision mismatch exits non-zero and prints FAIL", func(t *testing.T) {
		repo := copyTree(t, adopterRepo)
		// Tamper one case's expectation so the produced APPROVE no longer matches.
		expect := filepath.Join(repo, ".assent", "tests", "capped", "within-cap", "expect.yaml")
		writeFile(t, expect, "decision: BLOCK\n")

		var stdout, stderr bytes.Buffer
		code := runTest([]string{repo}, &stdout, &stderr)
		if code == 0 {
			t.Fatalf("exit = 0, want non-zero on a decision mismatch\nstdout:%s", stdout.String())
		}
		if !strings.Contains(stdout.String(), "FAIL capped/within-cap") {
			t.Fatalf("stdout missing FAIL for the tampered case:\n%s", stdout.String())
		}
	})

	t.Run("a missing directory argument exits 2", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := runTest(nil, &stdout, &stderr); code != 2 {
			t.Fatalf("exit = %d, want 2 on a usage error", code)
		}
	})
}

// TestTestCommandFailureUX (REQ-E6-S04-03) proves the S04 diff UX end-to-end through
// `assent test`: a PASSING run prints NO diff (no ready-to-copy block), and a FAILING
// case exits non-zero AND prints the located diff + a ready-to-copy actual block.
func TestTestCommandFailureUX(t *testing.T) {
	t.Run("a passing run prints no diff", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := runTest([]string{adopterRepo}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit = %d, want 0 (all cases pass)\nstderr:%s", code, stderr.String())
		}
		out := stdout.String()
		if strings.Contains(out, "ready to copy") || strings.Contains(out, "FAIL ") {
			t.Fatalf("passing run must print no diff, got:\n%s", out)
		}
	})

	t.Run("a failing case exits non-zero and prints the diff + copyable block", func(t *testing.T) {
		repo := copyTree(t, adopterRepo)
		expect := filepath.Join(repo, ".assent", "tests", "capped", "within-cap", "expect.yaml")
		writeFile(t, expect, "decision: BLOCK\n")

		var stdout, stderr bytes.Buffer
		code := runTest([]string{repo}, &stdout, &stderr)
		if code == 0 {
			t.Fatalf("exit = 0, want non-zero on a decision mismatch\nstdout:%s", stdout.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "FAIL capped/within-cap") {
			t.Fatalf("stdout missing FAIL header:\n%s", out)
		}
		if !strings.Contains(out, "expected BLOCK, got APPROVE") {
			t.Fatalf("stdout missing decision diff:\n%s", out)
		}
		if !strings.Contains(out, "ready to copy into expect.yaml") {
			t.Fatalf("stdout missing ready-to-copy actual block:\n%s", out)
		}
		if !strings.Contains(out, "decision: APPROVE") {
			t.Fatalf("copyable block missing the actual decision:\n%s", out)
		}
	})
}

// TestUpdateLeavesPassingCasesUntouched (REQ-E6-S05-03) proves the `--update`
// orchestration: only FAILING cases are rewritten (the golden refreshed, its authored
// comment preserved, and a subsequent normal run passes), while a PASSING case's
// expect.yaml is left byte-identical (no spurious churn). The FS write lives here in
// cmd/assent; the pure comment-preserving surgery is internal/adoptertest.
func TestUpdateLeavesPassingCasesUntouched(t *testing.T) {
	t.Setenv("CI", "") // hermetic: GitHub Actions sets CI=true, which the D-058 --update guard refuses (exit 2)
	repo := copyTree(t, adopterRepo)
	within := filepath.Join(repo, ".assent", "tests", "capped", "within-cap", "expect.yaml")
	overCap := filepath.Join(repo, ".assent", "tests", "capped", "over-cap", "expect.yaml")

	// Tamper the passing within-cap case into a FAILING one, keeping an authored comment
	// that must survive the refresh.
	writeFile(t, within, "# keep this comment across --update\ndecision: BLOCK\nfindings: []\n")

	// Snapshot a genuinely PASSING case (over-cap) before the update.
	overBefore := readCaseFile(t, overCap)

	var stdout, stderr bytes.Buffer
	code := runTest([]string{"--update", repo}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 after --update rewrites the failing case\nstdout:%s\nstderr:%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "UPDATED capped/within-cap") {
		t.Fatalf("stdout missing UPDATED for the refreshed case:\n%s", stdout.String())
	}

	// The passing case is byte-identical (no spurious churn).
	if overAfter := readCaseFile(t, overCap); !bytes.Equal(overBefore, overAfter) {
		t.Fatalf("passing case was rewritten under --update:\nbefore:\n%s\nafter:\n%s", overBefore, overAfter)
	}

	// The failing case was refreshed to the actual (APPROVE), its comment survived...
	refreshed := string(readCaseFile(t, within))
	if !strings.Contains(refreshed, "decision: APPROVE") {
		t.Fatalf("refreshed within-cap did not take the actual decision:\n%s", refreshed)
	}
	if !strings.Contains(refreshed, "# keep this comment across --update") {
		t.Fatalf("authored comment did not survive --update:\n%s", refreshed)
	}

	// ...and a subsequent NORMAL run now passes (exit 0, no failures).
	var stdout2, stderr2 bytes.Buffer
	if code := runTest([]string{repo}, &stdout2, &stderr2); code != 0 {
		t.Fatalf("re-run after --update exit = %d, want 0\nstdout:%s\nstderr:%s", code, stdout2.String(), stderr2.String())
	}
}

// TestUpdateRefusedInCI proves the logged CI guard (D-058): `--update` under a CI
// environment refuses (exit 2) and writes nothing — auto-accepting actuals in CI would
// silently ratify a regression (the classic golden-update footgun). The env read lives
// only in cmd/assent, never in internal/core or the pure library.
func TestUpdateRefusedInCI(t *testing.T) {
	t.Setenv("CI", "true")
	repo := copyTree(t, adopterRepo)
	within := filepath.Join(repo, ".assent", "tests", "capped", "within-cap", "expect.yaml")
	writeFile(t, within, "# keep\ndecision: BLOCK\nfindings: []\n")
	before := readCaseFile(t, within)

	var stdout, stderr bytes.Buffer
	code := runTest([]string{"--update", repo}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (--update refused in CI)\nstdout:%s\nstderr:%s", code, stdout.String(), stderr.String())
	}
	if after := readCaseFile(t, within); !bytes.Equal(before, after) {
		t.Fatalf("--update wrote a file despite the CI refusal:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// readCaseFile reads a case file's bytes for a byte-identity assertion.
func readCaseFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // path is under t.TempDir(), a fresh test dir.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(src, path)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		b, rerr := os.ReadFile(path) //nolint:gosec // fixed in-repo test fixture path.
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(target, b, 0o600) //nolint:gosec // target is under t.TempDir(), a fresh test dir.
	})
	if err != nil {
		t.Fatalf("copy fixture tree: %v", err)
	}
	return dst
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
