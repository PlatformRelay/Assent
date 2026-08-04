// Command assent is the CLI entry point for the auto-merge gate.
//
// Pre-alpha: the walking skeleton (meta-plan Phase 4) is being assembled
// slice by slice. P4-E1-S01 adds the CI-environment adapter — the ONLY place
// env/CI variables are read — that assembles a schema-valid EvaluationInput
// and the out-of-band pinned SHAs (ADR-0015 §1/§2). Later slices wire the
// differ, aggregation, report, and Reconcile behind subcommands.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/PlatformRelay/assent/internal/forge/gitlab"
)

var version = "0.0.0-dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is the testable entry point; it returns a process exit code.
func run(args []string) int {
	if len(args) > 0 && args[0] == "version" {
		fmt.Println("assent " + version)
		return 0
	}
	if len(args) > 0 && args[0] == "eval-input" {
		// Assemble the pinned EvaluationInput + Pins from the CI environment.
		// The env boundary lives here (readCIEnv) and the clock is bound to the
		// wall clock at the process edge, then threaded down as data — never
		// time.Now() inside internal/core (ADR-0017 §4, GUIDELINES §5).
		clock := Clock(time.Now)
		env := readCIEnv()
		// The differ (S02) fills the ChangeSet; until it lands, this path only
		// proves the adapter wiring end-to-end from the real environment.
		_, pins, err := AssembleEvaluationInput(env, AssemblyInputs{}, clock)
		if err != nil {
			fmt.Fprintln(os.Stderr, "assent eval-input:", err)
			return 1
		}
		fmt.Printf("assembled EvaluationInput for project=%s mr=%s (source=%s target=%s)\n",
			pins.ProjectID, pins.MergeRequestIID, pins.SourceSHA, pins.TargetSHA)
		return 0
	}
	if len(args) > 0 && args[0] == "run" {
		// The walking-skeleton end-to-end path (P4-E1-S10): read the MR, load the
		// policy from the TARGET ref, diff → classify → aggregate → build+validate
		// the DecisionRecord → Reconcile against the live GitLab, emit the record.
		// The GitLab PAT is read from GITLAB_TOKEN at this boundary (never a flag),
		// and the clock is bound to time.Now here and threaded down as data.
		return runRun(args[1:], os.Getenv, time.Now, os.Stdout, os.Stderr,
			func(endpoint, token, botAuthor string) forgePort {
				return gitlab.New(endpoint, token, botAuthor)
			})
	}
	if len(args) > 0 && args[0] == "lint" {
		// `assent lint <dir>` (E3-S01): discover the repo's `.assent/**` tree and
		// run the pure internal/lint hard-error checks over it. The directory walk
		// (the only I/O) lives in runLint; the checks are pure. Exit non-zero on any
		// error diagnostic so a policy defect fails CI before it reaches an MR.
		return runLint(args[1:], os.Stdout, os.Stderr)
	}
	if len(args) > 0 && args[0] == "catalogue" {
		// `assent catalogue <dir>` (E3-S07): discover the repo's `.assent/**` tree,
		// load every policy document via the E2 strict loader, and emit the pure
		// internal/catalogue generated rule catalogue (D-017 B10) as JSON on stdout
		// for the docs pipeline. The directory walk + loader calls (the only I/O)
		// live in runCatalogue; the generator is pure. A distinct subcommand, NOT
		// `assent lint --catalogue` — generation is a docs artifact, not a gate
		// (D-048).
		return runCatalogue(args[1:], os.Stdout, os.Stderr)
	}
	if len(args) > 0 && args[0] == "test" {
		// `assent test <repo>` (E6-S01): discover the repo's `.assent/tests/**`
		// directory cases, diff each case's base/↔head/ with the production differ,
		// stub its facts.yaml into the resolved-fact envelope, and evaluate the pack
		// via aggregate.Cover — asserting the produced Decision equals the case's
		// expect.yaml. The directory walk + file reads (the only I/O) live in runTest;
		// the case loader/assembler/assertion is the pure internal/adoptertest library.
		// Exit 0 when every case matched, non-zero on any mismatch or load error, so a
		// policy regression fails CI before it reaches an MR (ADR-0006 dogfooding).
		return runTest(args[1:], os.Stdout, os.Stderr)
	}
	if len(args) > 0 && args[0] == "compare" {
		// `assent compare <dir>` (E6-S09 seed): load one immutable ReplayBundle plus
		// the baseline/candidate policy activations + their shared binding, evaluate
		// both through the reused engine, classify the decision delta into one frozen
		// taxonomy kind, and apply the bounded-auto-merge-widening promotion gate. The
		// dir reads (the only I/O) live in runCompare; internal/compare is pure. Exit
		// 0 = gate pass, 1 = gate fail (a widening), 2 = load/fail-closed error.
		return runCompare(args[1:], os.Stdout, os.Stderr)
	}
	if len(args) > 0 && args[0] == "doctor" {
		// Precondition/arming report (ADR-0015 §4/§8, ADR-0017 §9). The env
		// boundary lives here (readPipelineDescription); Doctor is pure and
		// receives the resolved description as data. Default-deny: the run is
		// advisory-only unless a protected-source config is verified — no
		// approve/merge write path exists yet (S06/S08), and it will gate on
		// report.ArmEligible.
		report := Doctor(readPipelineDescription())
		if report.ArmEligible {
			fmt.Println("assent doctor: arming precondition MET — auto-merge may be armed")
			return 0
		}
		// Exit 1 = "not armed" (arming-gate semantics): a not-armed/advisory run
		// is a non-zero gate result, not a soft warning. A bare informational run
		// on an unprotected pipeline therefore exits non-zero by design.
		fmt.Fprintln(os.Stderr, "assent doctor: advisory-only — auto-merge NOT armed:")
		for _, r := range report.Reasons {
			fmt.Fprintf(os.Stderr, "  - [%s] %s\n", r.Code, r.Detail)
		}
		return 1
	}
	fmt.Fprintln(os.Stderr, "assent (pre-alpha): no commands implemented yet — see docs/planning/meta-plan.md")
	return 2
}
