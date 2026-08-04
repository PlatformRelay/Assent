package main

import (
	"encoding/json"
	"errors"
	"flag"
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

// compare.go is the thin filesystem+command shell for `assent compare` (E6-S09 seed,
// PCS-S07 suite mode + ADR-0018 exit codes). FS reads load ReplayBundles, suite
// corpora, repo catalogues, and baseline/candidate activations; internal/compare
// evaluates, classifies, and gates. Exit codes (D-115 / ADR-0018):
//
//	0 = all promotion gates PASS,
//	1–5 = first failing gate (destructive → authorization → obligation → widening → accepted),
//	6 = load / schema / digest / fail-closed classification error.
//
// Single-dir seed layout (<dir>):
//   - bundle.json, binding.yaml, baseline.yaml, candidate.yaml
//   - optional .assent/** for PolicyProfile pack activation
//
// Suite layout (--suite <dir>):
//   - suite.yaml or suite.json (PolicyComparisonSuite)
//   - cases/<caseId>/bundle.json per suite case
//   - binding.yaml, baseline.yaml, candidate.yaml (profile activation)
func runCompare(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var suitePath, baselineProfile, candidateProfile, recordDir string
	fs.StringVar(&suitePath, "suite", "", "PolicyComparisonSuite directory or suite.yaml/json path")
	fs.StringVar(&baselineProfile, "baseline-profile", "", "baseline PolicyProfile name (overrides suite default)")
	fs.StringVar(&candidateProfile, "candidate-profile", "", "candidate PolicyProfile name (overrides suite default)")
	fs.StringVar(&recordDir, "record", "", "directory to write ComparisonRecord JSON per caseId")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return compare.ExitCodeFailClosed
	}
	rest := fs.Args()
	if suitePath != "" {
		return runCompareSuite(suitePath, baselineProfile, candidateProfile, recordDir, stdout, stderr)
	}
	if len(rest) < 1 || rest[0] == "" {
		_, _ = fmt.Fprintln(stderr, "assent compare: a directory argument is required (usage: assent compare <dir> | assent compare --suite <dir>)")
		return compare.ExitCodeFailClosed
	}
	return runCompareDir(rest[0], stdout, stderr)
}

func runCompareDir(dir string, stdout, stderr io.Writer) int {
	bundleRaw, err := os.ReadFile(filepath.Join(dir, "bundle.json")) // #nosec G703 G304 -- operator-supplied local compare dir, read-only.
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare: read bundle.json:", err)
		return compare.ExitCodeFailClosed
	}
	in, err := compare.LoadBundle(bundleRaw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	bind, err := loadCompareBinding(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	baselineSide, err := readCompareSide(dir, "baseline.yaml")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}
	candidateSide, err := readCompareSide(dir, "candidate.yaml")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	var catIn catalogue.Input
	if baselineSide.profile != nil || candidateSide.profile != nil {
		catIn, err = catalogue.LoadFromDir(dir)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "assent compare:", err)
			return compare.ExitCodeFailClosed
		}
	}

	baseline, err := resolveCompareProfile(baselineSide, catIn, bind)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}
	candidate, err := resolveCompareProfile(candidateSide, catIn, bind)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	cmp, err := compare.Compare(in, baseline, candidate)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	kind := string(cmp.Kind)
	if kind == "" {
		kind = "(no delta)"
	}
	_, _ = fmt.Fprintf(stdout, "compare %s -> %s: baseline=%s candidate=%s delta=%s gate=%s verdict=%s\n",
		cmp.BaselineProfile, cmp.CandidateProfile, cmp.Baseline, cmp.Candidate, kind, cmp.Gate, cmp.Verdict)

	if cmp.Verdict == compare.VerdictFail {
		return compare.ExitCodeForGate(cmp.Gate)
	}
	return 0
}

func runCompareSuite(suitePath, baselineProfileRef, candidateProfileRef, recordDir string, stdout, stderr io.Writer) int {
	root, suiteFile, err := resolveSuiteRoot(suitePath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	suiteRaw, err := readCompareSuiteDoc(suiteFile)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}
	suite, err := compare.LoadSuite(suiteRaw)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	if baselineProfileRef == "" {
		baselineProfileRef = suite.BaselineProfileRef
	}
	if candidateProfileRef == "" {
		candidateProfileRef = suite.CandidateProfileRef
	}

	bind, err := loadCompareBinding(root)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	baselineSide, err := readCompareSide(root, "baseline.yaml")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}
	candidateSide, err := readCompareSide(root, "candidate.yaml")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	var catIn catalogue.Input
	if baselineSide.profile != nil || candidateSide.profile != nil {
		catIn, err = catalogue.LoadFromDir(root)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "assent compare:", err)
			return compare.ExitCodeFailClosed
		}
	}

	baseline, err := resolveCompareProfile(baselineSide, catIn, bind)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}
	candidate, err := resolveCompareProfile(candidateSide, catIn, bind)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}
	if err := assertProfileRef("baseline", baselineProfileRef, baseline.Name); err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}
	if err := assertProfileRef("candidate", candidateProfileRef, candidate.Name); err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	cases, err := loadSuiteCaseBundles(root, suite)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	result, err := compare.RunSuite(suite, cases, baseline, candidate)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent compare:", err)
		return compare.ExitCodeFailClosed
	}

	if recordDir != "" {
		if err := writeComparisonRecords(recordDir, result.Records); err != nil {
			_, _ = fmt.Fprintln(stderr, "assent compare:", err)
			return compare.ExitCodeFailClosed
		}
	}

	printSuiteReport(stdout, result)
	return compare.ExitCodeForSuiteRun(result.Gates)
}

func resolveSuiteRoot(suitePath string) (root, suiteFile string, err error) {
	info, statErr := os.Stat(suitePath)
	if statErr != nil {
		return "", "", statErr
	}
	if info.IsDir() {
		root = suitePath
		for _, name := range []string{"suite.yaml", "suite.yml", "suite.json"} {
			candidate := filepath.Join(root, name)
			if _, err := os.Stat(candidate); err == nil {
				return root, candidate, nil
			}
		}
		return "", "", fmt.Errorf("suite directory %q has no suite.yaml or suite.json", suitePath)
	}
	root = filepath.Dir(suitePath)
	return root, suitePath, nil
}

func readCompareSuiteDoc(path string) ([]byte, error) {
	raw, err := os.ReadFile(path) // #nosec G703 G304 -- operator-supplied suite path, read-only.
	if err != nil {
		return nil, fmt.Errorf("read suite doc: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" {
		return raw, nil
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse suite doc: %w", err)
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("encode suite doc: %w", err)
	}
	return out, nil
}

func loadSuiteCaseBundles(root string, suite compare.PolicyComparisonSuite) (map[string][]byte, error) {
	out := make(map[string][]byte, len(suite.Cases))
	for _, sc := range suite.Cases {
		path := filepath.Join(root, "cases", sc.CaseID, "bundle.json")
		raw, err := os.ReadFile(path) // #nosec G703 G304 -- operator-supplied case bundle, read-only.
		if err != nil {
			return nil, fmt.Errorf("case %q: read bundle: %w", sc.CaseID, err)
		}
		out[sc.CaseID] = raw
	}
	return out, nil
}

func writeComparisonRecords(dir string, records []compare.ComparisonRecord) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("record dir: %w", err)
	}
	for _, rec := range records {
		raw, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("case %q record encode: %w", rec.CaseID, err)
		}
		path := filepath.Join(dir, rec.CaseID+".json")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			return fmt.Errorf("case %q record write: %w", rec.CaseID, err)
		}
	}
	return nil
}

func printSuiteReport(w io.Writer, result compare.SuiteRunResult) {
	for _, rec := range result.Records {
		delta := "(no delta)"
		if len(rec.Deltas) == 1 {
			delta = string(rec.Deltas[0].Kind)
		} else if len(rec.Deltas) > 1 {
			delta = fmt.Sprintf("%d deltas", len(rec.Deltas))
		}
		_, _ = fmt.Fprintf(w, "case %s: baseline=%s candidate=%s delta=%s\n",
			rec.CaseID, rec.BaselineProfile, rec.CandidateProfile, delta)
	}
	gateOrder := []compare.GateID{
		compare.GateZeroMissedDestructive,
		compare.GateZeroMissedAuthorizationOwnership,
		compare.GateNoUnexpectedObligationRemoval,
		compare.GateBoundedAutoMergeWidening,
		compare.GateExplicitlyAcceptedDeltas,
	}
	for _, id := range gateOrder {
		v := result.Gates.Results[id]
		if v == "" {
			v = compare.VerdictPass
		}
		_, _ = fmt.Fprintf(w, "gate %s=%s\n", id, v)
	}
	if result.Gates.FirstFailure != "" {
		_, _ = fmt.Fprintf(w, "first-failure=%s\n", result.Gates.FirstFailure)
	}
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

func assertProfileRef(side, ref, resolvedName string) error {
	if ref == "" {
		return nil
	}
	if resolvedName != ref {
		return fmt.Errorf("%s profile ref %q does not match loaded profile %q", side, ref, resolvedName)
	}
	return nil
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
