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
