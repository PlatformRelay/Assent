package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PlatformRelay/assent/internal/adoptertest"
	"github.com/PlatformRelay/assent/internal/catalogue"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// test.go is the thin filesystem+command shell for `assent test <repo>` (E6-S01).
// The FS walk (discovering `.assent/tests/**` directory cases and reading each
// case's files into memory) is the ONLY I/O; the case loader/assembler/decision-
// assertion is the PURE internal/adoptertest library. It loads the repo's pack the
// same way `assent catalogue` does (loadCatalogueInput + selectBinding), runs each
// discovered case, prints pass/fail, and exits 0 when every case's produced Decision
// matched its expect.yaml, non-zero on any mismatch or load error.
//
// S01 scope: single-rule/single-file directory cases (the anchor). Whole multi-rule
// packs, entry-scoped rules, the finding matcher, and inline cases.yaml are S02/S03/S06.

// runTest is the testable entry point for `assent test`. It accepts an optional
// `--update` flag (in any position) plus the repo directory (the tree containing
// `.assent/**`). It returns a process exit code: 0 = every case's decision matched (or,
// under --update, every failing case was refreshed), 1 = a mismatch/write/load error,
// 2 = usage/discovery/CI-guard refusal.
func runTest(args []string, stdout, stderr io.Writer) int {
	update := false
	coverage := false
	var positional []string
	for _, a := range args {
		switch a {
		case "--update":
			update = true
		case "--coverage":
			coverage = true
		default:
			positional = append(positional, a)
		}
	}
	// --coverage is a READ-ONLY completeness gate (S07): it never writes goldens, so
	// it supersedes --update (and its CI write-guard) when both are passed.
	if coverage {
		update = false
	}
	if len(positional) < 1 || positional[0] == "" {
		_, _ = fmt.Fprintln(stderr, "assent test: a directory argument is required (usage: assent test [--update] [--coverage] <repo>)")
		return 2
	}
	dir := positional[0]

	// CI guard (D-058): --update auto-accepts the produced actuals as the new golden,
	// so running it in CI would silently ratify a regression instead of failing the
	// build (the classic golden-update footgun). Refuse under a CI environment. The env
	// read lives ONLY here in cmd/assent — never in internal/core or the pure library.
	if update && os.Getenv("CI") != "" {
		_, _ = fmt.Fprintln(stderr, "assent test: --update refused: a CI environment is set (auto-accepting actuals in CI would ratify a regression); run --update locally and review the diff")
		return 2
	}

	in, err := catalogue.LoadFromDir(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent test:", err)
		return 2
	}
	policies := map[string]*policy.MergePolicy{}
	for _, p := range in.Packs {
		combined, cerr := catalogue.CombinePolicies(p.Policies)
		if cerr != nil {
			_, _ = fmt.Fprintf(stderr, "assent test: pack %q: %v\n", p.Name, cerr)
			return 2
		}
		if combined != nil {
			policies[p.Name] = combined
		}
	}
	if len(in.Bindings) == 0 {
		_, _ = fmt.Fprintln(stderr, "assent test: no RulesetBinding found under", dir)
		return 2
	}
	bind, err := selectBindingForTest(in.Bindings[0])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent test:", err)
		return 2
	}

	cases, err := discoverCases(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent test:", err)
		return 2
	}
	if len(cases) == 0 {
		_, _ = fmt.Fprintln(stderr, "assent test: no .assent/tests/** cases found under", dir)
		return 2
	}

	// --coverage (S07): the both-polarity completeness gate runs its own case walk
	// (RunCaseCoverage) so it can attribute each PASSING case's polarities, then gates
	// on every enforce obligation rule being tested in BOTH polarities. Read-only.
	if coverage {
		return runCoverage(dir, policies, bind, cases, in, stdout, stderr)
	}

	anyFail := false
	for _, cf := range cases {
		pol, ok := policies[cf.pack]
		if !ok {
			_, _ = fmt.Fprintf(stderr, "assent test: %s: no pack %q loaded under .assent/packs/**\n", cf.name, cf.pack)
			anyFail = true
			continue
		}
		facts, ferr := adoptertest.MapFacts(cf.factsRaw)
		if ferr != nil {
			_, _ = fmt.Fprintf(stderr, "assent test: %s: %v\n", cf.name, ferr)
			anyFail = true
			continue
		}
		out, rerr := adoptertest.RunCase(adoptertest.Case{
			Name:   cf.name,
			Policy: pol,
			Bind:   bind,
			File:   cf.file,
			Base:   cf.base,
			Head:   cf.head,
			Facts:  facts,
			Expect: cf.expect,
		})
		if rerr != nil {
			_, _ = fmt.Fprintf(stderr, "assent test: %s: %v\n", cf.name, rerr)
			anyFail = true
			continue
		}
		if out.Pass {
			// A PASSING case is left byte-identical under --update — the writer is
			// never invoked for it (no spurious churn, REQ-E6-S05-03).
			_, _ = fmt.Fprintf(stdout, "PASS %s (%s)\n", out.Name, out.Actual)
			continue
		}
		if update {
			// S05 golden-refresh: rewrite ONLY this failing case's expect.yaml with the
			// produced actuals, preserving the author's comments (pure Node surgery in
			// internal/adoptertest; the FS write — the only I/O — lives here). Fail-closed:
			// a rewrite that would not re-validate against the frozen schema is an error,
			// never a written golden.
			newBytes, uerr := adoptertest.UpdateExpectationBlock(cf.expectRaw, out.ActualExpect)
			if uerr != nil {
				_, _ = fmt.Fprintf(stderr, "assent test: %s: --update: %v\n", out.Name, uerr)
				anyFail = true
				continue
			}
			if werr := os.WriteFile(cf.expectPath, newBytes, 0o600); werr != nil { // #nosec G306 -- 0600 golden; path is the discovered case's own expect.yaml.
				_, _ = fmt.Fprintf(stderr, "assent test: %s: --update write: %v\n", out.Name, werr)
				anyFail = true
				continue
			}
			_, _ = fmt.Fprintf(stdout, "UPDATED %s (%s)\n", out.Name, out.Actual)
			continue
		}
		anyFail = true
		// S04 failure UX: the located expected/actual diff + a ready-to-copy
		// actual block. The pure formatting lives in internal/adoptertest; this
		// shell only prints it. A serialization failure (the actual block cannot
		// be made schema-valid) is itself a fail-closed error, never a silent pass.
		report, rerr := adoptertest.RenderFailure(cf.expect, out)
		if rerr != nil {
			_, _ = fmt.Fprintf(stderr, "assent test: %s: render failure: %v\n", out.Name, rerr)
			continue
		}
		_, _ = fmt.Fprint(stdout, report)
	}

	// Inline `cases.yaml` shorthand (E6-S06): a `.assent/tests/<pack>/cases.yaml`
	// holds many one-file cases whose base/head live INLINE. It is an ALTERNATE
	// front-end — each inline case is assembled into the SAME adoptertest.Case a
	// directory case is and run through the identical pipeline. The FS read (the only
	// I/O) lives here; the strict-decode + assembly is the pure library. --update is a
	// directory-case flow only (S05); an inline case simply reports pass/fail here.
	inlineFiles, err := discoverInlineCasesFiles(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent test:", err)
		return 2
	}
	for _, icf := range inlineFiles {
		pol, ok := policies[icf.pack]
		if !ok {
			_, _ = fmt.Fprintf(stderr, "assent test: %s/cases.yaml: no pack %q loaded under .assent/packs/**\n", icf.pack, icf.pack)
			anyFail = true
			continue
		}
		inlineCases, lerr := adoptertest.LoadInlineCases(icf.raw, pol, bind)
		if lerr != nil {
			_, _ = fmt.Fprintf(stderr, "assent test: %s/cases.yaml: %v\n", icf.pack, lerr)
			anyFail = true
			continue
		}
		for _, c := range inlineCases {
			c.Name = icf.pack + "/" + c.Name
			out, rerr := adoptertest.RunCase(c)
			if rerr != nil {
				_, _ = fmt.Fprintf(stderr, "assent test: %s: %v\n", c.Name, rerr)
				anyFail = true
				continue
			}
			if out.Pass {
				_, _ = fmt.Fprintf(stdout, "PASS %s (%s)\n", out.Name, out.Actual)
				continue
			}
			anyFail = true
			report, rerr := adoptertest.RenderFailure(c.Expect, out)
			if rerr != nil {
				_, _ = fmt.Fprintf(stderr, "assent test: %s: render failure: %v\n", out.Name, rerr)
				continue
			}
			_, _ = fmt.Fprint(stdout, report)
		}
	}

	if anyFail {
		return 1
	}
	return 0
}

// runCoverage is the `assent test --coverage` both-polarity completeness gate (S07,
// the run-time counterpart to E3-S06's static tests-per-rule presence check). It
// walks the same directory + inline cases as the plain run (RunCaseCoverage), and
// for every PASSING case accumulates which enforce obligation rules it witnessed
// firing (failing polarity) and proven-silent (proving polarity). It then reports,
// per rule, whether BOTH polarities were exercised, and exits non-zero if any rule
// is missing one (or if any case failed / errored). The gate is fail-safe toward
// NOT-covered (D-059): an unwitnessed polarity is never assumed covered.
func runCoverage(dir string, policies map[string]*policy.MergePolicy, bind *policy.Binding, cases []discoveredCase, in catalogue.Input, stdout, stderr io.Writer) int {
	// Enforce obligation rule UNIVERSE from the catalogue (D-059): only an
	// enforce-effective-phase obligation rule can both drive a finding and satisfy an
	// obligation, so both-polarity applies to it; off/observe/non-obligation rules are
	// excluded (requiring both-polarity for a rule that cannot fire enforcing would make
	// the gate un-satisfiable). enforceOblByPack scopes the universe per pack for the
	// per-case witness.
	cat := catalogue.Build(in)
	var universe []adoptertest.CoverageRule
	enforceOblByPack := map[string]map[string]bool{}
	for _, re := range cat.Rules {
		if re.EffectivePhase != "enforce" || re.Obligation == "" {
			continue
		}
		universe = append(universe, adoptertest.CoverageRule{Pack: re.Pack, Rule: re.Rule})
		if enforceOblByPack[re.Pack] == nil {
			enforceOblByPack[re.Pack] = map[string]bool{}
		}
		enforceOblByPack[re.Pack][re.Rule] = true
	}

	anyFail := false
	var credits []adoptertest.CoverageCredit
	accumulate := func(pack string, out adoptertest.Outcome, w adoptertest.CaseWitness, exp adoptertest.Expectation) {
		if !out.Pass {
			// A case whose own assertions did not hold is not valid coverage evidence
			// (and fails the run in its own right); print the located diff UX.
			anyFail = true
			report, rerr := adoptertest.RenderFailure(exp, out)
			if rerr != nil {
				_, _ = fmt.Fprintf(stderr, "assent test: %s: render failure: %v\n", out.Name, rerr)
				return
			}
			_, _ = fmt.Fprint(stdout, report)
			return
		}
		_, _ = fmt.Fprintf(stdout, "PASS %s (%s)\n", out.Name, out.Actual)
		for _, r := range w.Failing {
			credits = append(credits, adoptertest.CoverageCredit{Pack: pack, Rule: r, Failing: true})
		}
		for _, r := range w.Proving {
			credits = append(credits, adoptertest.CoverageCredit{Pack: pack, Rule: r, Proving: true})
		}
	}

	for _, cf := range cases {
		pol, ok := policies[cf.pack]
		if !ok {
			_, _ = fmt.Fprintf(stderr, "assent test: %s: no pack %q loaded under .assent/packs/**\n", cf.name, cf.pack)
			anyFail = true
			continue
		}
		facts, ferr := adoptertest.MapFacts(cf.factsRaw)
		if ferr != nil {
			_, _ = fmt.Fprintf(stderr, "assent test: %s: %v\n", cf.name, ferr)
			anyFail = true
			continue
		}
		out, w, rerr := adoptertest.RunCaseCoverage(adoptertest.Case{
			Name:   cf.name,
			Policy: pol,
			Bind:   bind,
			File:   cf.file,
			Base:   cf.base,
			Head:   cf.head,
			Facts:  facts,
			Expect: cf.expect,
		}, enforceOblByPack[cf.pack])
		if rerr != nil {
			_, _ = fmt.Fprintf(stderr, "assent test: %s: %v\n", cf.name, rerr)
			anyFail = true
			continue
		}
		accumulate(cf.pack, out, w, cf.expect)
	}

	inlineFiles, err := discoverInlineCasesFiles(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent test:", err)
		return 2
	}
	for _, icf := range inlineFiles {
		pol, ok := policies[icf.pack]
		if !ok {
			_, _ = fmt.Fprintf(stderr, "assent test: %s/cases.yaml: no pack %q loaded under .assent/packs/**\n", icf.pack, icf.pack)
			anyFail = true
			continue
		}
		inlineCases, lerr := adoptertest.LoadInlineCases(icf.raw, pol, bind)
		if lerr != nil {
			_, _ = fmt.Fprintf(stderr, "assent test: %s/cases.yaml: %v\n", icf.pack, lerr)
			anyFail = true
			continue
		}
		for _, c := range inlineCases {
			c.Name = icf.pack + "/" + c.Name
			out, w, rerr := adoptertest.RunCaseCoverage(c, enforceOblByPack[icf.pack])
			if rerr != nil {
				_, _ = fmt.Fprintf(stderr, "assent test: %s: %v\n", c.Name, rerr)
				anyFail = true
				continue
			}
			accumulate(icf.pack, out, w, c.Expect)
		}
	}

	rep := adoptertest.BuildCoverageReport(universe, credits)
	_, _ = fmt.Fprint(stdout, adoptertest.RenderCoverage(rep))
	if !rep.Complete() {
		anyFail = true
	}
	if anyFail {
		return 1
	}
	return 0
}

// selectBindingForTest picks the single binding a whole-pack `assent test` replay
// evaluates under. A single-binding RulesetBinding is used directly (exactly the
// pre-S08 behaviour). A MULTI-binding document (e.g. a dev/prod environment split that
// differs ONLY in environment + risk.threshold) is decision-safe to collapse to its
// STRICTEST (lowest-threshold) binding: a lower threshold can only make a points sum
// MORE likely to escalate to REVIEW, never less — the fail-safe direction (D-060).
//
// If two bindings differ in anything BEYOND environment + threshold (class, packs, or
// require[]), collapsing could silently FLIP a decision, so it fails CLOSED here — the
// full (class, environment) routing that a real `assent run` needs is not wired for
// `assent test` in this lane (cf. run.go selectBinding). Fail-closed, never a guess.
func selectBindingForTest(rb *policy.RulesetBinding) (*policy.Binding, error) {
	if rb == nil || len(rb.Bindings) == 0 {
		return nil, fmt.Errorf("ruleset-binding declares no bindings")
	}
	if len(rb.Bindings) == 1 {
		return &rb.Bindings[0], nil
	}
	first := rb.Bindings[0]
	strictest := 0
	for i := 1; i < len(rb.Bindings); i++ {
		b := rb.Bindings[i]
		if b.Class != first.Class || !sameStrings(b.Packs, first.Packs) || !sameStrings(b.Require, first.Require) {
			return nil, fmt.Errorf("ruleset-binding declares %d bindings differing beyond environment+threshold — (class, environment) routing is not wired for `assent test` (D-060); supply a single-binding RulesetBinding or align the bindings", len(rb.Bindings))
		}
		if b.Risk.Threshold < rb.Bindings[strictest].Risk.Threshold {
			strictest = i
		}
	}
	return &rb.Bindings[strictest], nil
}

// sameStrings reports order-sensitive slice equality (the binding fields it compares
// are authored lists whose order is stable).
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// inlineCasesFile is one discovered `.assent/tests/<pack>/cases.yaml`: its pack (the
// first path segment under tests/) and its raw bytes. The pure library strict-decodes
// the bytes and assembles the cases.
type inlineCasesFile struct {
	pack string
	raw  []byte
}

// discoverInlineCasesFiles walks <dir>/.assent/tests and returns, in deterministic
// pack order, every `cases.yaml` file (the inline shorthand, E6-S06). A cases.yaml
// carries no expect.yaml, so discoverCases (which keys on expect.yaml) never picks it
// up — the two discovery paths are disjoint.
func discoverInlineCasesFiles(dir string) ([]inlineCasesFile, error) {
	root := filepath.Join(dir, ".assent", "tests")
	info, err := os.Stat(root) // #nosec G703 -- dir is an operator-supplied CLI path to their own repo, read-only.
	if err != nil || !info.IsDir() {
		return nil, nil // no tests tree ⇒ no inline cases (the directory path already reported it)
	}

	// Collect the (pack, path) pairs in the walk; read the bytes AFTER it, so the file
	// read is not done inside the WalkDir callback.
	type located struct{ pack, path string }
	var found []located
	// #nosec G703 -- operator-supplied local path, read-only walk.
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || d.Name() != "cases.yaml" {
			return nil
		}
		rel, rerr := filepath.Rel(root, filepath.Dir(path))
		if rerr != nil {
			return rerr
		}
		pack := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		found = append(found, located{pack: pack, path: path})
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk .assent/tests tree for cases.yaml: %w", walkErr)
	}

	files := make([]inlineCasesFile, 0, len(found))
	for _, f := range found {
		raw, rerr := os.ReadFile(f.path) // #nosec G304 -- fixed .assent/tests cases.yaml path from the walk.
		if rerr != nil {
			return nil, fmt.Errorf("read %s/cases.yaml: %w", f.pack, rerr)
		}
		files = append(files, inlineCasesFile{pack: f.pack, raw: raw})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].pack < files[j].pack })
	return files, nil
}

// discoveredCase is one directory case's raw contents: the pure library maps the
// facts, so the shell carries facts.yaml as raw bytes.
type discoveredCase struct {
	pack string
	name string
	file string
	base []byte
	head []byte
	// expectPath is the case's expect.yaml path and expectRaw its original bytes —
	// carried so `--update` can rewrite the golden in place, preserving the authored
	// comments in expectRaw (the pure surgery reads the original bytes).
	expectPath string
	expectRaw  []byte
	factsRaw   []byte
	expect     adoptertest.Expectation
}

// discoverCases walks <dir>/.assent/tests and returns, in deterministic name order,
// every directory case (a directory that directly contains an expect.yaml). Each
// case's pack is the first path segment under tests/; its single changed file is the
// one regular file present under base/ (and head/).
func discoverCases(dir string) ([]discoveredCase, error) {
	root := filepath.Join(dir, ".assent", "tests")
	// #nosec G703 -- dir is an operator-supplied CLI path to their own repo, read-only.
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("no .assent/tests/** tree found under %q", dir)
	}

	var cases []discoveredCase
	// #nosec G703 -- operator-supplied local path, read-only walk.
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if !d.IsDir() {
			return nil
		}
		expectPath := filepath.Join(path, "expect.yaml")
		if _, serr := os.Stat(expectPath); serr != nil {
			return nil // not a case directory
		}
		dc, cerr := loadDiscoveredCase(root, path)
		if cerr != nil {
			return cerr
		}
		cases = append(cases, dc)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk .assent/tests tree: %w", walkErr)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].name < cases[j].name })
	return cases, nil
}

// loadDiscoveredCase reads one case directory's files. name is the case's slash path
// relative to the tests root (e.g. "capped/within-cap"); pack is its first segment.
func loadDiscoveredCase(root, caseDir string) (discoveredCase, error) {
	rel, err := filepath.Rel(root, caseDir)
	if err != nil {
		return discoveredCase{}, fmt.Errorf("relativize case dir: %w", err)
	}
	name := filepath.ToSlash(rel)
	pack := strings.SplitN(name, "/", 2)[0]

	expectPath := filepath.Join(caseDir, "expect.yaml")
	expectRaw, err := os.ReadFile(expectPath) // #nosec G304 -- fixed .assent/tests case path.
	if err != nil {
		return discoveredCase{}, fmt.Errorf("%s: read expect.yaml: %w", name, err)
	}
	expect, err := adoptertest.LoadExpectation(expectRaw)
	if err != nil {
		return discoveredCase{}, fmt.Errorf("%s: %w", name, err)
	}

	var factsRaw []byte
	if b, ferr := os.ReadFile(filepath.Join(caseDir, "facts.yaml")); ferr == nil { // #nosec G304 -- fixed case path.
		factsRaw = b
	}

	file, base, head, err := readSingleFilePair(caseDir)
	if err != nil {
		return discoveredCase{}, fmt.Errorf("%s: %w", name, err)
	}

	return discoveredCase{
		pack:       pack,
		name:       name,
		file:       file,
		base:       base,
		head:       head,
		expectPath: expectPath,
		expectRaw:  expectRaw,
		factsRaw:   factsRaw,
		expect:     expect,
	}, nil
}

// readSingleFilePair finds the single governed file of an S01 case (the one regular
// file under base/) and reads its base and head bytes. A case with zero or multiple
// files, or one whose head side is missing, is beyond S01's single-file scope and is
// reported as an error rather than silently mis-evaluated.
func readSingleFilePair(caseDir string) (file string, base, head []byte, err error) {
	baseRoot := filepath.Join(caseDir, "base")
	var rels []string
	walkErr := filepath.WalkDir(baseRoot, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(baseRoot, path)
		if rerr != nil {
			return rerr
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if walkErr != nil {
		return "", nil, nil, fmt.Errorf("walk base/: %w", walkErr)
	}
	if len(rels) != 1 {
		return "", nil, nil, fmt.Errorf("expected exactly one file under base/, found %d (multi-file/whole-pack cases are E6-S02)", len(rels))
	}
	file = rels[0]

	base, err = os.ReadFile(filepath.Join(baseRoot, filepath.FromSlash(file))) // #nosec G304 -- fixed case path.
	if err != nil {
		return "", nil, nil, fmt.Errorf("read base/%s: %w", file, err)
	}
	head, err = os.ReadFile(filepath.Join(caseDir, "head", filepath.FromSlash(file))) // #nosec G304 -- fixed case path.
	if err != nil {
		return "", nil, nil, fmt.Errorf("read head/%s: %w (new/deleted-file cases are E6-S06)", file, err)
	}
	return file, base, head, nil
}
