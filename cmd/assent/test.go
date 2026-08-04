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

// runTest is the testable entry point for `assent test`. args[0] is the repo
// directory (the tree containing `.assent/**`). It returns a process exit code:
// 0 = every case's decision matched, 1 = a mismatch/load error, 2 = usage/discovery.
func runTest(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] == "" {
		_, _ = fmt.Fprintln(stderr, "assent test: a directory argument is required (usage: assent test <repo>)")
		return 2
	}
	dir := args[0]

	in, err := loadCatalogueInput(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent test:", err)
		return 2
	}
	policies := map[string]*policy.MergePolicy{}
	for _, p := range in.Packs {
		if len(p.Policies) > 0 {
			policies[p.Name] = p.Policies[0]
		}
	}
	if len(in.Bindings) == 0 {
		_, _ = fmt.Fprintln(stderr, "assent test: no RulesetBinding found under", dir)
		return 2
	}
	bind, err := selectBinding(in.Bindings[0])
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
			_, _ = fmt.Fprintf(stdout, "PASS %s (%s)\n", out.Name, out.Actual)
		} else {
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
	}
	if anyFail {
		return 1
	}
	return 0
}

// discoveredCase is one directory case's raw contents: the pure library maps the
// facts, so the shell carries facts.yaml as raw bytes.
type discoveredCase struct {
	pack     string
	name     string
	file     string
	base     []byte
	head     []byte
	factsRaw []byte
	expect   adoptertest.Expectation
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

	expectRaw, err := os.ReadFile(filepath.Join(caseDir, "expect.yaml")) // #nosec G304 -- fixed .assent/tests case path.
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
		pack:     pack,
		name:     name,
		file:     file,
		base:     base,
		head:     head,
		factsRaw: factsRaw,
		expect:   expect,
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
