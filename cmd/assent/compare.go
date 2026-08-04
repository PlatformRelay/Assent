package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/PlatformRelay/assent/internal/compare"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// compare.go is the thin filesystem+command shell for `assent compare <dir>`
// (E6-S09 seed). The FS reads (the ONLY I/O) load an immutable ReplayBundle plus
// the baseline/candidate policy activations and their shared binding; the pure
// internal/compare library does the engine evaluation, delta classification, and
// promotion-gate verdict. It maps the verdict to a process exit code:
//
//	0 = gate PASS (candidate promotable — no widening),
//	1 = gate FAIL (a newly-auto-mergeable widening — do NOT promote),
//	2 = usage / load / fail-closed classification error (never a silent 0).
//
// SEED SCOPE (ADR-0018 / decisions.md D-055): one bundle, one baseline vs one
// candidate, the newly-auto-mergeable + explanation-only classifiers, and the
// single bounded-auto-merge-widening gate. The full PolicyComparisonSuite runner
// (all six kinds, all five gates, acceptedDeltas) is its own epic.
//
// Layout of <dir> (flat, four files):
//   - bundle.json      the immutable ReplayBundle (pre-built evaluationInput + pins)
//   - binding.yaml     the shared RulesetBinding (require/environment/class)
//   - baseline.yaml    the baseline MergePolicy activation
//   - candidate.yaml   the candidate MergePolicy activation
func runCompare(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] == "" {
		_, _ = fmt.Fprintln(stderr, "assent compare: a directory argument is required (usage: assent compare <dir>)")
		return 2
	}
	dir := args[0]

	bundleRaw, err := os.ReadFile(filepath.Join(dir, "bundle.json")) // #nosec G703 G304 -- operator-supplied local compare dir, read-only.
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare: read bundle.json:", err)
		return 2
	}
	in, err := compare.LoadBundle(bundleRaw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return 2
	}

	bind, err := loadCompareBinding(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return 2
	}

	baseline, err := loadCompareProfile(dir, "baseline.yaml", bind)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return 2
	}
	candidate, err := loadCompareProfile(dir, "candidate.yaml", bind)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return 2
	}

	cmp, err := compare.Compare(in, baseline, candidate)
	if err != nil {
		// A fail-closed classification error (or a load/eval error) is NEVER a
		// silent gate-pass — it exits non-zero (2), distinct from a gate FAIL (1).
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return 2
	}

	kind := string(cmp.Kind)
	if kind == "" {
		kind = "(no delta)"
	}
	_, _ = fmt.Fprintf(stdout, "compare %s -> %s: baseline=%s candidate=%s delta=%s gate=%s verdict=%s\n",
		cmp.BaselineProfile, cmp.CandidateProfile, cmp.Baseline, cmp.Candidate, kind, cmp.Gate, cmp.Verdict)

	if cmp.Verdict == compare.VerdictFail {
		return 1
	}
	return 0
}

// loadCompareBinding reads and strict-loads the shared RulesetBinding, routing to
// its single covering binding (fail-closed on zero/many, like the other subcommands).
func loadCompareBinding(dir string) (*policy.Binding, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "binding.yaml")) // #nosec G703 G304 -- operator-supplied local compare dir, read-only.
	if err != nil {
		return nil, fmt.Errorf("read binding.yaml: %w", err)
	}
	rb, err := policy.LoadRulesetBinding(raw)
	if err != nil {
		return nil, err
	}
	return selectBinding(rb)
}

// loadCompareProfile reads and strict-loads one side's MergePolicy activation and
// wires it into a compare.Profile over the shared binding. The profile identity is
// the loaded policy's metadata.name (the promotion-comparison label). Ceiling is
// enforce (no cap) — a per-profile ceiling lever is the full runner's concern.
func loadCompareProfile(dir, file string, bind *policy.Binding) (compare.Profile, error) {
	raw, err := os.ReadFile(filepath.Join(dir, file)) // #nosec G703 G304 -- operator-supplied local compare dir, read-only.
	if err != nil {
		return compare.Profile{}, fmt.Errorf("read %s: %w", file, err)
	}
	mp, err := policy.LoadMergePolicy(raw)
	if err != nil {
		return compare.Profile{}, fmt.Errorf("%s: %w", file, err)
	}
	return compare.Profile{
		Name:    mp.Metadata.Name,
		Policy:  mp,
		Bind:    bind,
		Ceiling: policy.PhaseEnforce,
	}, nil
}
