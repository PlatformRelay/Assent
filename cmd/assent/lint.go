package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/PlatformRelay/assent/internal/lint"
)

// lint.go is the thin filesystem+command shell for `assent lint <dir>` (E3-S01).
// It discovers the repo's `.assent/**` tree, reads every YAML document into
// in-memory lint.Source values, and hands them to the PURE internal/lint check
// library. The directory walk is the ONLY I/O — the checks themselves are pure
// (no clock/env/net/random), so the impurity stays here at the sanctioned
// boundary, exactly as `assent run` keeps env/clock in cmd/assent.
//
// Exit code: 0 when lint is clean, non-zero when any error diagnostic is present
// (or the tree cannot be discovered) — an `assent lint` failure fails CI.

// runLint is the testable entry point for `assent lint`. args[0] is the repo
// directory to lint (the tree containing `.assent/**`). It returns a process exit
// code: 0 clean, 1 on any error diagnostic, 2 on a usage/discovery error.
func runLint(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] == "" {
		_, _ = fmt.Fprintln(stderr, "assent lint: a directory argument is required (usage: assent lint <dir>)")
		return 2
	}
	dir := args[0]

	sources, err := discoverAssentTree(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent lint:", err)
		return 2
	}

	report := lint.Lint(sources)

	// Diagnostics go to stderr in canonical order; a clean run prints nothing.
	if out := report.Render(); out != "" {
		_, _ = fmt.Fprint(stderr, out)
	}
	if report.HasErrors() {
		_, _ = fmt.Fprintf(stdout, "assent lint: %d error(s) — see diagnostics above\n", errorCount(report))
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "assent lint: clean")
	return 0
}

// errorCount reports how many error-severity diagnostics the report carries, for
// the one-line summary. (HasErrors is the gate; this is presentation only.)
func errorCount(report *lint.Report) int {
	n := 0
	for _, d := range report.Diagnostics() {
		if d.Severity == lint.SeverityError {
			n++
		}
	}
	return n
}

// discoverAssentTree walks <dir>/.assent and returns every YAML document as a
// lint.Source, its Path made repo-relative (to <dir>) and slash-separated so the
// diagnostic Location and the pack-name derivation are stable across platforms.
// A missing `.assent` tree is an error (there is nothing to lint).
func discoverAssentTree(dir string) ([]lint.Source, error) {
	root := filepath.Join(dir, ".assent")
	// #nosec G703 -- dir is an operator-supplied CLI path to their own repo, not
	// remote/attacker input; lint reads it read-only. Symlink hardening belongs to
	// a checkout-provisioning story, not this read.
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("no .assent/** tree found under %q", dir)
	}
	var sources []lint.Source
	// #nosec G703 -- see the os.Stat note above: operator-supplied local path, read-only walk.
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isYAML(path) {
			return nil
		}
		raw, rerr := os.ReadFile(path) // #nosec G304,G122 -- path comes from walking a fixed in-repo .assent tree, not user-controlled input; no symlink surface.
		if rerr != nil {
			return rerr
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			rel = path
		}
		sources = append(sources, lint.Source{Path: filepath.ToSlash(rel), Bytes: raw})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk .assent tree: %w", walkErr)
	}
	return sources, nil
}

// isYAML reports whether a path is a YAML document lint should ingest.
func isYAML(path string) bool {
	return strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")
}
