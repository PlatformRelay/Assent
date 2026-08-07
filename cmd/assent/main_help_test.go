package main

import (
	"bytes"
	"errors"
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
// and neither may the surfaces this pin walks (DOC-08).
//
// Scope, stated exactly: the walk roots are .. (the whole cmd/ tree, so a second
// binary added later is covered too), ../../internal and ../../docs; within them
// it reads .go and .md files, minus the two exclusions in isPinnedSource. It does NOT
// cover repo-root markdown (README.md, API_STABILITY.md — AUD-S06's files),
// examples/, openspec/, hack/, schemas/, .github/ or test/. The AC's "nowhere in
// the repo" is therefore narrowed on purpose: openspec/ and docs/decisions/ quote
// the phrases as history by design. Known live hit outside this scope:
// examples/README.md — reported to the coordinator, owned by no story yet.
func TestNoStaleProductClaims(t *testing.T) {
	usage := strings.ToLower(usageText())
	for _, claim := range staleClaims {
		if strings.Contains(usage, claim) {
			t.Errorf("help text still claims %q\n---\n%s", claim, usageText())
		}
	}

	for _, root := range []string{"..", "../../internal", "../../docs"} {
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

// isPinnedSource reports whether a file reached by the walk roots is in scope for
// the DOC-08 grep pin: .go and .md files, minus two exclusions where naming the
// phrase is the file's job — this test, and the append-only decision log, which
// records superseded states (D-104's revert note) as history.
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

// dispatchMarkers returns, per subcommand name, a string that ONLY that command's
// own handler can produce when the built binary is invoked with the bare name and a
// scrubbed environment.
//
// Every marker is an `assent <name>:` diagnostic prefix (or, for version, the
// stamped semver). None of them can be produced by usageText(), which renders the
// name as `usage: assent <name>` with no colon — that matters, because usageText()
// is emitted by three paths (the help alias, the help handler, usageError), so any
// marker it can render would be satisfied by the usage listing rather than by the
// handler actually running. TestDispatchMarkersAreUnique enforces both that
// property and mutual exclusivity.
//
// `help` is the one legitimate exception: printing usageText() IS its handler's
// job, so its marker is necessarily listing content. Its binding is pinned instead
// by TestHelpExitCodesOnBuiltBinary, which requires the listing on stdout with an
// empty stderr and exit 0 — usageError writes to stderr and exits 2, so it cannot
// satisfy that.
func dispatchMarkers() map[string]string {
	return map[string]string{
		"run":        "assent run:",
		"doctor":     "assent doctor:",
		"lint":       "assent lint:",
		"test":       "assent test:",
		"compare":    "assent compare:",
		"catalogue":  "assent catalogue:",
		"render":     "assent render:",
		"eval-input": "assent eval-input:",
		"version":    "assent " + version,
		"help":       tagline,
	}
}

// usagePinnedCommands are the commands whose no-argument diagnostic ends in their
// canonical `(usage: …)` string. For those the binding probe additionally asserts
// the table's usage line is what the command really prints, so the two cannot
// drift. The remaining commands have no such string to pin against.
var usagePinnedCommands = map[string]bool{
	"lint": true, "test": true, "compare": true, "catalogue": true, "render": true,
}

// REQ-AUD-S05-01: every name in the dispatch table reaches ITS OWN handler in the
// built binary. Asserted end-to-end rather than by calling runLint/runCompare/… in
// process, because an in-process call bypasses the table and would stay green under
// a hand transposition (`lint` wired to runCatalogue) that ships a binary answering
// the wrong command behind a name.
func TestDispatchTableBindsEachNameToItsHandler(t *testing.T) {
	bin := buildAssent(t, "")
	table := subcommands()
	markers := dispatchMarkers()

	if len(markers) != len(table) {
		t.Fatalf("dispatchMarkers covers %d names, the table has %d — every subcommand needs a binding probe", len(markers), len(table))
	}
	for _, sc := range table {
		marker, ok := markers[sc.name]
		if !ok {
			t.Errorf("subcommand %q has no binding probe", sc.name)
			continue
		}
		stdout, stderr, _ := runAssent(t, bin, sc.name)
		out := stdout + stderr
		if !strings.Contains(out, marker) {
			t.Errorf("assent %s is not bound to its own handler: output lacks %q\nstdout:\n%s\nstderr:\n%s",
				sc.name, marker, stdout, stderr)
		}
		if usagePinnedCommands[sc.name] && !strings.Contains(out, sc.usage) {
			t.Errorf("assent %s: help table usage %q is not what the command prints:\n%s", sc.name, sc.usage, out)
		}
	}
}

// Negative polarity for the binding probe. Two ways it could be satisfied without
// the handler having run, both closed here:
//   - a marker the usage listing itself renders (usageText() reaches stdout on the
//     help paths and stderr on every usage error, so such a marker proves nothing);
//   - a marker that is a substring of another command's, which would let a
//     two-name transposition satisfy both sides.
func TestDispatchMarkersAreUnique(t *testing.T) {
	markers := dispatchMarkers()
	usage := usageText()
	for name, marker := range markers {
		if strings.TrimSpace(marker) == "" {
			t.Errorf("subcommand %q has an empty binding marker", name)
			continue
		}
		// `help`'s handler exists to print the listing; see dispatchMarkers.
		if name != "help" && strings.Contains(usage, marker) {
			t.Errorf("marker for %q (%q) is rendered by usageText() — the usage listing alone would satisfy the binding probe", name, marker)
		}
		for other, otherMarker := range markers {
			if other == name {
				continue
			}
			if strings.Contains(otherMarker, marker) {
				t.Errorf("marker for %q (%q) also matches %q — a %s/%s transposition would pass undetected",
					name, marker, other, name, other)
			}
		}
	}
}

// REQ-AUD-S05-01: each entry's usage line must name its own command. Without this,
// transposing two `name:` fields while leaving `usage:` and `run:` in place leaves
// the binding probe satisfied — the listing changes, but regenerating the embedded
// block in docs/usage/cli.md is the documented loop for a help-text edit, so the
// whole suite would go green on a binary answering the wrong command by name.
func TestEachUsageLineNamesItsOwnCommand(t *testing.T) {
	for _, sc := range subcommands() {
		if want := "assent " + sc.name; !strings.Contains(sc.usage, want) {
			t.Errorf("subcommand %q has usage %q, which does not invoke %q", sc.name, sc.usage, want)
		}
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
// as separate streams — a combined capture would hide a stdout/stderr swap. The
// environment is scrubbed: GITLAB_TOKEN, CI and CI_* would otherwise steer run,
// doctor, test and eval-input down different paths depending on where the suite runs.
func runAssent(t *testing.T, bin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...) // #nosec G204 -- test-built binary path
	cmd.Env = []string{}
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
