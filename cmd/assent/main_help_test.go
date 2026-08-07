package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// staleClaims are the pre-release phrases DOC-08 removed. They must not come back
// on any user-facing surface: the help text, the CLI sources, or the docs tree.
var staleClaims = []string{
	"no commands implemented yet",
	"pre-alpha",
}

// REQ-AUD-S05-01: the help listing is derived from the single dispatch table, so a
// subcommand cannot be dispatched without appearing in help.
func TestHelpListsAllSubcommands(t *testing.T) {
	table := subcommands()
	if len(table) == 0 {
		t.Fatal("subcommands() is empty — the dispatch table is the help source of truth")
	}
	usage := usageText()
	if missing := missingHelpEntries(usage, table); len(missing) != 0 {
		t.Fatalf("help output omits dispatched subcommands %v\n---\n%s", missing, usage)
	}
	for _, sc := range table {
		if strings.TrimSpace(sc.synopsis) == "" {
			t.Errorf("subcommand %q has no synopsis", sc.name)
		}
		if strings.TrimSpace(sc.usage) == "" {
			t.Errorf("subcommand %q has no usage line", sc.name)
		}
		if sc.run == nil {
			t.Errorf("subcommand %q is listed but not dispatched", sc.name)
		}
	}
}

// Negative polarity for the same requirement: the listing check must actually fail
// when a dispatched subcommand is absent from the help text.
func TestHelpListingDetectsAnUnlistedSubcommand(t *testing.T) {
	table := subcommands()
	augmented := make([]subcommand, 0, len(table)+1)
	augmented = append(augmented, table...)
	augmented = append(augmented, subcommand{
		name:     "totally-not-dispatched",
		synopsis: "a subcommand the help text has never heard of",
		usage:    "assent totally-not-dispatched",
		run:      func([]string) int { return 0 },
	})
	missing := missingHelpEntries(usageText(), augmented)
	if len(missing) != 1 || missing[0] != "totally-not-dispatched" {
		t.Fatalf("missingHelpEntries = %v, want exactly [totally-not-dispatched] — the drift check is not load-bearing", missing)
	}
}

// REQ-AUD-S05-01: help exits 0; bare invocation and unknown subcommands keep the
// exit-2 contract and write to stderr. Asserted against the real built binary, per
// stream, so a stdout/stderr swap cannot pass.
func TestHelpExitCodesOnBuiltBinary(t *testing.T) {
	bin := buildAssent(t, "")

	for _, arg := range []string{"--help", "-h", "-help", "help"} {
		stdout, stderr, code := runAssent(t, bin, arg)
		if code != 0 {
			t.Errorf("assent %s: exit = %d, want 0 (stderr: %s)", arg, code, stderr)
		}
		if missing := missingHelpEntries(stdout, subcommands()); len(missing) != 0 {
			t.Errorf("assent %s: stdout omits %v", arg, missing)
		}
		if strings.TrimSpace(stderr) != "" {
			t.Errorf("assent %s: help must go to stdout, got stderr: %s", arg, stderr)
		}
	}

	for _, args := range [][]string{{}, {"definitely-not-a-command"}} {
		stdout, stderr, code := runAssent(t, bin, args...)
		if code != 2 {
			t.Errorf("assent %v: exit = %d, want 2", args, code)
		}
		if missing := missingHelpEntries(stderr, subcommands()); len(missing) != 0 {
			t.Errorf("assent %v: stderr omits %v", args, missing)
		}
		if strings.TrimSpace(stdout) != "" {
			t.Errorf("assent %v: usage must go to stderr, got stdout: %s", args, stdout)
		}
	}
}

// REQ-AUD-S05-01: the shipped help text must not resurrect the pre-release claims,
// and neither may the CLI sources or the docs tree (DOC-08 grep pin). The story
// spec under openspec/ quotes the old string by design and is out of the walk.
func TestNoStaleProductClaims(t *testing.T) {
	usage := strings.ToLower(usageText())
	for _, claim := range staleClaims {
		if strings.Contains(usage, claim) {
			t.Errorf("help text still claims %q\n---\n%s", claim, usageText())
		}
	}

	for _, root := range []string{".", "../../internal", "../../docs"} {
		walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !isPinnedSource(path) {
				return nil
			}
			data, readErr := os.ReadFile(path) // #nosec G304 G122 -- test-walked repository sources, read-only
			if readErr != nil {
				return readErr
			}
			body := strings.ToLower(string(data))
			for _, claim := range staleClaims {
				if strings.Contains(body, claim) {
					t.Errorf("%s still contains the pre-release claim %q", path, claim)
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", root, walkErr)
		}
	}
}

// isPinnedSource reports whether a walked file is a user-facing surface covered by
// the DOC-08 grep pin: Go sources and markdown docs. Two exclusions, both because
// naming the phrase is their job: this test, and the append-only decision log,
// which records superseded states (D-104's revert note) as history.
func isPinnedSource(path string) bool {
	if filepath.Base(path) == "main_help_test.go" {
		return false
	}
	if strings.Contains(filepath.ToSlash(path), "/decisions/") {
		return false
	}
	ext := filepath.Ext(path)
	return ext == ".go" || ext == ".md"
}

// REQ-AUD-S05-01: each table usage line is the same string the subcommand itself
// prints when invoked without arguments, so the two cannot drift apart. Only the
// commands that own a canonical `(usage: …)` error string are covered; run, doctor,
// version, eval-input and help have no such string to pin against.
func TestTableUsageMatchesSubcommandErrors(t *testing.T) {
	byName := map[string]func(io.Writer) int{
		"lint":      func(w io.Writer) int { return runLint(nil, io.Discard, w) },
		"catalogue": func(w io.Writer) int { return runCatalogue(nil, io.Discard, w) },
		"test":      func(w io.Writer) int { return runTest(nil, io.Discard, w) },
		"compare":   func(w io.Writer) int { return runCompare(nil, io.Discard, w) },
		"render":    func(w io.Writer) int { return runRender(nil, io.Discard, w) },
	}
	covered := 0
	for _, sc := range subcommands() {
		invoke, ok := byName[sc.name]
		if !ok {
			continue
		}
		covered++
		var stderr bytes.Buffer
		invoke(&stderr)
		if !strings.Contains(stderr.String(), sc.usage) {
			t.Errorf("%s: help table usage %q is not what the command prints:\n%s", sc.name, sc.usage, stderr.String())
		}
	}
	if covered != len(byName) {
		t.Fatalf("covered %d subcommands, want %d — a pinned command left the dispatch table", covered, len(byName))
	}
}

// missingHelpEntries returns the subcommands absent from a rendered help text.
func missingHelpEntries(help string, table []subcommand) []string {
	var missing []string
	for _, sc := range table {
		if !strings.Contains(help, "  "+sc.name+"\n") {
			missing = append(missing, sc.name)
		}
	}
	return missing
}

// runAssent executes the built binary and returns stdout, stderr and the exit code
// as separate streams — a combined capture would hide a stdout/stderr swap.
func runAssent(t *testing.T, bin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...) // #nosec G204 -- test-built binary path
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %s %v: %v", bin, args, err)
		}
		return stdout.String(), stderr.String(), exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), 0
}
