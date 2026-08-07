// Command assent is the CLI entry point for the deterministic auto-merge gate.
//
// The binary evaluates a merge request against the policy tree committed in the
// target repository, produces a DecisionRecord, and reconciles the verdict on the
// forge. Alongside the gate it ships the policy-authoring surface: `lint` and
// `catalogue` over a `.assent/**` tree, `test` for adopter cases, `compare` for
// baseline/candidate promotion gates, `render` for local finding previews, and
// `doctor` for an arming pre-flight.
//
// Every subcommand is declared once in the dispatch table below; the help listing
// is rendered from that table, so a command cannot be dispatched without being
// documented (REQ-AUD-S05-01).
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/gitlab"
)

// version is the CLI semver surfaced by `assent version`. Release builds inject it via
// -ldflags "-X main.version=…" (Taskfile.yml ASSENT_VERSION, goreleaser D-099); dev
// builds keep this default with no git dirty suffix — the tag is source of truth (E9-S01).
var version = "0.0.0-dev"

// tagline is the one-line product description printed above the command listing.
const tagline = "assent — deterministic, policy-driven auto-merge for self-service repositories"

// helpAliases are the FLAG spellings that print the usage listing on stdout and
// exit 0. The bare word `help` is deliberately absent: it is a real dispatch-table
// entry, so its handler is live code reached the same way every other command is,
// rather than a branch that shadows the table.
var helpAliases = map[string]bool{"--help": true, "-help": true, "-h": true}

// subcommand is one dispatched command. The same record supplies the dispatch, the
// help listing and the drift assertions against docs/usage/cli.md — there is no
// second list to keep in step.
type subcommand struct {
	// name is the argv[1] token that selects the command.
	name string
	// synopsis is the one-line description shown in the help listing.
	synopsis string
	// usage is the invocation form; for the commands that print their own
	// `(usage: …)` on a missing argument it is that exact string.
	usage string
	// run executes the command with the arguments that follow its name and
	// returns the process exit code.
	run func(args []string) int
}

// subcommands returns the dispatch table in help-listing order: the gate first,
// then the authoring surface, then the auxiliary commands.
func subcommands() []subcommand {
	return []subcommand{
		{
			name:     "run",
			synopsis: "Evaluate a merge request against its policy and reconcile the decision on the forge",
			usage:    "GITLAB_TOKEN=<pat> assent run --project <id> --mr <iid> --subject file:<path> --bot-author <user> [flags]",
			// The walking-skeleton end-to-end path (P4-E1-S10): read the MR, load the
			// policy from the TARGET ref, diff → classify → aggregate → build+validate
			// the DecisionRecord → Reconcile against the live GitLab, emit the record.
			// The GitLab PAT is read from GITLAB_TOKEN at this boundary (never a flag),
			// and the clock is bound to time.Now here and threaded down as data.
			run: func(args []string) int {
				return runRun(args, os.Getenv, time.Now, os.Stdout, os.Stderr,
					func(endpoint, token, botAuthor string) forgePort {
						return gitlab.New(endpoint, token, botAuthor)
					})
			},
		},
		{
			name:     "doctor",
			synopsis: "Report whether this environment can arm auto-merge, and why not when it cannot",
			usage:    "assent doctor",
			run: func([]string) int {
				return runDoctor(os.Getenv, os.Stdout, os.Stderr,
					func(endpoint, token, botAuthor string) forge.Snapshotter {
						return gitlab.New(endpoint, token, botAuthor)
					})
			},
		},
		{
			name:     "lint",
			synopsis: "Check a repository's .assent/** policy tree for hard errors",
			usage:    "assent lint <dir>",
			// `assent lint <dir>` (E3-S01): discover the repo's `.assent/**` tree and
			// run the pure internal/lint hard-error checks over it. The directory walk
			// (the only I/O) lives in runLint; the checks are pure. Exit non-zero on any
			// error diagnostic so a policy defect fails CI before it reaches an MR.
			run: func(args []string) int { return runLint(args, os.Stdout, os.Stderr) },
		},
		{
			name:     "test",
			synopsis: "Run a repository's .assent/tests/** adopter cases against the real engine",
			usage:    "assent test [--update] [--coverage] <repo>",
			// `assent test <repo>` (E6-S01): discover the repo's `.assent/tests/**`
			// directory cases, diff each case's base/↔head/ with the production differ,
			// stub its facts.yaml into the resolved-fact envelope, and evaluate the pack
			// via aggregate.Cover — asserting the produced Decision equals the case's
			// expect.yaml. The directory walk + file reads (the only I/O) live in runTest;
			// the case loader/assembler/assertion is the pure internal/adoptertest library.
			// Exit 0 when every case matched, non-zero on any mismatch or load error, so a
			// policy regression fails CI before it reaches an MR (ADR-0006 dogfooding).
			run: func(args []string) int { return runTest(args, os.Stdout, os.Stderr) },
		},
		{
			name:     "compare",
			synopsis: "Replay a comparison suite of baseline vs candidate policy and apply the promotion gates",
			usage:    "assent compare <dir> | assent compare --suite <dir>",
			// `assent compare <dir>` (E6-S09 seed) or `assent compare --suite <dir>` (PCS-S07):
			// load immutable ReplayBundle(s), baseline/candidate activations + binding,
			// evaluate through the reused engine, classify deltas, and apply promotion
			// gates. FS reads live in runCompare; internal/compare is pure. Exit codes
			// follow ADR-0018 / D-115: 0 pass, 1–5 first failing gate, 6 fail-closed/load.
			run: func(args []string) int { return runCompare(args, os.Stdout, os.Stderr) },
		},
		{
			name:     "catalogue",
			synopsis: "Emit the generated rule catalogue for a policy tree as JSON on stdout",
			usage:    "assent catalogue <dir>",
			// `assent catalogue <dir>` (E3-S07): discover the repo's `.assent/**` tree,
			// load every policy document via the E2 strict loader, and emit the pure
			// internal/catalogue generated rule catalogue (D-017 B10) as JSON on stdout
			// for the docs pipeline. The directory walk + loader calls (the only I/O)
			// live in runCatalogue; the generator is pure. A distinct subcommand, NOT
			// `assent lint --catalogue` — generation is a docs artifact, not a gate
			// (D-048).
			run: func(args []string) int { return runCatalogue(args, os.Stdout, os.Stderr) },
		},
		{
			name:     "render",
			synopsis: "Render a committed finding fixture as markdown for local preview",
			usage:    "assent render --finding examples/render/<case> [--artifact finding-thread|summary] [--presentation-minimal|--presentation-full]",
			// `assent render --finding examples/render/<case>` (E8-S10): load a committed
			// render fixture, validate via LoadPresentationModel/LoadRenderContext, and
			// emit markdown on stdout for local preview without a live MR (ADR-0016 §4).
			run: func(args []string) int { return runRender(args, os.Stdout, os.Stderr) },
		},
		{
			name:     "eval-input",
			synopsis: "Assemble the EvaluationInput and pinned SHAs from the CI environment",
			usage:    "assent eval-input",
			run:      func([]string) int { return runEvalInput(os.Stdout, os.Stderr) },
		},
		{
			name:     "version",
			synopsis: "Print the assent version",
			usage:    "assent version",
			run: func([]string) int {
				fmt.Println("assent " + version)
				return 0
			},
		},
		{
			name:     "help",
			synopsis: "Print this help listing",
			usage:    "assent help",
			run: func([]string) int {
				_, _ = fmt.Fprint(os.Stdout, usageText())
				return 0
			},
		},
	}
}

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is the testable entry point; it returns a process exit code. Help goes to
// stdout with exit 0; a bare or unknown invocation prints the same listing on
// stderr and keeps the exit-2 contract scripts depend on (REQ-AUD-S05-01).
func run(args []string) int {
	if len(args) == 0 {
		return usageError(os.Stderr, "")
	}
	if helpAliases[args[0]] {
		_, _ = fmt.Fprint(os.Stdout, usageText())
		return 0
	}
	for _, sc := range subcommands() {
		if sc.name == args[0] {
			return sc.run(args[1:])
		}
	}
	return usageError(os.Stderr, args[0])
}

// usageError reports an unusable invocation: the offending command (when there was
// one), then the usage listing, both on stderr, exit 2.
func usageError(stderr io.Writer, unknown string) int {
	if unknown != "" {
		_, _ = fmt.Fprintf(stderr, "assent: unknown command %q\n\n", unknown)
	}
	_, _ = fmt.Fprint(stderr, usageText())
	return 2
}

// usageText renders the help listing from the dispatch table. It is deliberately
// free of build-injected values (no version string) so the published reference in
// docs/usage/cli.md can embed it verbatim and be pinned byte-for-byte.
func usageText() string {
	var b strings.Builder
	b.WriteString(tagline + "\n\n")
	b.WriteString("Usage:\n  assent <command> [arguments]\n\nCommands:\n")
	for _, sc := range subcommands() {
		_, _ = fmt.Fprintf(&b, "  %s\n      %s\n      usage: %s\n", sc.name, sc.synopsis, sc.usage)
	}
	b.WriteString("\nassent run -h, assent compare -h and assent render -h list their flags.\n")
	b.WriteString("Full command reference: https://platformrelay.github.io/assent/usage/cli/\n")
	return b.String()
}

// runEvalInput assembles the pinned EvaluationInput + Pins from the CI environment.
// The env boundary lives here (readCIEnv) and the clock is bound to the wall clock
// at the process edge, then threaded down as data — never time.Now() inside
// internal/core (ADR-0017 §4, GUIDELINES §5).
func runEvalInput(stdout, stderr io.Writer) int {
	clock := Clock(time.Now)
	env := readCIEnv()
	// The differ fills the ChangeSet; this path proves the adapter wiring
	// end-to-end from the real environment.
	_, pins, err := AssembleEvaluationInput(env, AssemblyInputs{}, clock)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent eval-input:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "assembled EvaluationInput for project=%s mr=%s (source=%s target=%s)\n",
		pins.ProjectID, pins.MergeRequestIID, pins.SourceSHA, pins.TargetSHA)
	return 0
}
