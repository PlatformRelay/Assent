package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/catalogue"
	"github.com/PlatformRelay/assent/internal/compare"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// compare.go is the thin filesystem+command shell for `assent compare <dir>`
// (E6-S09 seed, PCS-S01 profile activation). The FS reads (the ONLY I/O) load an
// immutable ReplayBundle, the repo catalogue via catalogue.LoadFromDir, baseline/
// candidate PolicyProfile activations (or legacy MergePolicy seed fixtures), and
// their shared binding; the pure internal/compare library evaluates, classifies,
// and gates. Exit codes:
//
//	0 = gate PASS (candidate promotable — no widening),
//	1 = gate FAIL (a newly-auto-mergeable widening — do NOT promote),
//	2 = usage / load / fail-closed classification error (never a silent 0).
//
// Layout of <dir> for profile activation:
//   - bundle.json, binding.yaml, baseline.yaml, candidate.yaml (flat seed files)
//   - .assent/** packs the profiles activate via spec.packs[]
//
// Legacy seed fixtures may still supply baseline.yaml/candidate.yaml as MergePolicy
// docs (no .assent tree required) — behaviour unchanged until PCS-S07 migrates them.
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

	baselineSide, err := readCompareSide(dir, "baseline.yaml")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return 2
	}
	candidateSide, err := readCompareSide(dir, "candidate.yaml")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return 2
	}

	var catIn catalogue.Input
	if baselineSide.profile != nil || candidateSide.profile != nil {
		catIn, err = catalogue.LoadFromDir(dir)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "assent compare:", err)
			return 2
		}
	}

	baseline, err := resolveCompareProfile(baselineSide, catIn, bind)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return 2
	}
	candidate, err := resolveCompareProfile(candidateSide, catIn, bind)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return 2
	}

	cmp, err := compare.Compare(in, baseline, candidate)
	if err != nil {
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

type compareSide struct {
	profile *policy.Profile
	policy  *policy.MergePolicy
}

func readCompareSide(dir, file string) (compareSide, error) {
	raw, err := os.ReadFile(filepath.Join(dir, file)) // #nosec G703 G304 -- operator-supplied local compare dir, read-only.
	if err != nil {
		return compareSide{}, fmt.Errorf("read %s: %w", file, err)
	}
	kind, err := compareDocKind(raw)
	if err != nil {
		return compareSide{}, fmt.Errorf("%s: %w", file, err)
	}
	switch kind {
	case "PolicyProfile":
		p, lerr := policy.LoadProfile(raw)
		if lerr != nil {
			return compareSide{}, fmt.Errorf("%s: %w", file, lerr)
		}
		return compareSide{profile: p}, nil
	case "MergePolicy":
		mp, lerr := policy.LoadMergePolicy(raw)
		if lerr != nil {
			return compareSide{}, fmt.Errorf("%s: %w", file, lerr)
		}
		return compareSide{policy: mp}, nil
	default:
		return compareSide{}, fmt.Errorf("%s: unsupported kind %q (want PolicyProfile or legacy MergePolicy seed)", file, kind)
	}
}

func resolveCompareProfile(side compareSide, catIn catalogue.Input, bind *policy.Binding) (compare.Profile, error) {
	if side.profile != nil {
		mp, err := catalogue.MergePolicyForProfile(side.profile, catIn)
		if err != nil {
			return compare.Profile{}, err
		}
		return compare.Profile{
			Name:    side.profile.Metadata.Name,
			Policy:  mp,
			Bind:    bind,
			Ceiling: catalogue.PhaseCeilingForProfile(side.profile, catIn),
		}, nil
	}
	if side.policy != nil {
		return compare.Profile{
			Name:    side.policy.Metadata.Name,
			Policy:  side.policy,
			Bind:    bind,
			Ceiling: policy.PhaseEnforce,
		}, nil
	}
	return compare.Profile{}, fmt.Errorf("compare side declares neither PolicyProfile nor MergePolicy")
}

func compareDocKind(raw []byte) (string, error) {
	var header struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(raw, &header); err != nil {
		return "", err
	}
	return strings.TrimSpace(header.Kind), nil
}

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
