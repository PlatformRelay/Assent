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
	fmt.Fprintln(os.Stderr, "assent (pre-alpha): no commands implemented yet — see docs/planning/meta-plan.md")
	return 2
}
