package main

// render_exitgate_test.go is REQ-E8-S14-02: an intentional render-golden wording
// change must not fail assent test structured assertions on the same fixture pack
// (ADR-0014 safety vs presentation split).

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/PlatformRelay/assent/internal/adoptertest"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

const safetySplitPack = "service-catalog"

// TestE8ExitGateSafetySplit proves structured assent test assertions (decision,
// rule, effect, obligation) stay green when finding messages are rewritten as an
// intentional render-golden wording drift would — message~ is not the safety signal.
func TestE8ExitGateSafetySplit(t *testing.T) {
	packDir := filepath.Join(examplesPacksDir, safetySplitPack)
	var so, se bytes.Buffer
	if code := runTest([]string{packDir}, &so, &se); code != 0 {
		t.Fatalf("assent test %s baseline: exit = %d, want 0\nstdout:\n%s\nstderr:\n%s",
			safetySplitPack, code, so.String(), se.String())
	}

	in, err := loadCatalogueInput(packDir)
	if err != nil {
		t.Fatalf("load catalogue: %v", err)
	}
	policies := map[string]*policy.MergePolicy{}
	for _, p := range in.Packs {
		combined, cerr := combinePolicies(p.Policies)
		if cerr != nil {
			t.Fatalf("combine policies for pack %q: %v", p.Name, cerr)
		}
		if combined != nil {
			policies[p.Name] = combined
		}
	}
	if len(in.Bindings) == 0 {
		t.Fatal("no RulesetBinding under pack")
	}
	bind, err := selectBindingForTest(in.Bindings[0])
	if err != nil {
		t.Fatalf("select binding: %v", err)
	}
	cases, err := discoverCases(packDir)
	if err != nil {
		t.Fatalf("discover cases: %v", err)
	}

	// Pick a case with structured rule+effect findings and no message~ pin.
	const targetCase = "catalog/ownership/negative"
	var dc *discoveredCase
	for i := range cases {
		if cases[i].name == targetCase {
			dc = &cases[i]
			break
		}
	}
	if dc == nil {
		t.Fatalf("case %q not found under %s", targetCase, packDir)
	}
	if hasMessageOnlyPin(dc.expect) {
		t.Fatalf("case %q must use structured safety assertions, not message~ only", targetCase)
	}

	pol, ok := policies[dc.pack]
	if !ok {
		t.Fatalf("no pack %q loaded", dc.pack)
	}
	facts, err := adoptertest.MapFacts(dc.factsRaw)
	if err != nil {
		t.Fatalf("map facts: %v", err)
	}
	c := adoptertest.Case{
		Name:   dc.name,
		Policy: pol,
		Bind:   bind,
		File:   dc.file,
		Base:   dc.base,
		Head:   dc.head,
		Facts:  facts,
		Expect: dc.expect,
	}

	res, err := adoptertest.Evaluate(c)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	out, err := adoptertest.RunCase(c)
	if err != nil {
		t.Fatalf("RunCase: %v", err)
	}
	if !out.Pass {
		t.Fatalf("case %q must pass before wording drift simulation: %v", targetCase, out.Reasons)
	}

	// Simulate an intentional render-golden wording change: mutate every emitted
	// finding message while leaving rule/effect/code identity untouched.
	drifted := res
	for i := range drifted.Findings {
		drifted.Findings[i].Message = "RENDER GOLDEN WORDING v2 — " + drifted.Findings[i].Message
	}

	reasons, err := adoptertest.Match(c.Expect, drifted, bind.Risk.Threshold)
	if err != nil {
		t.Fatalf("Match after wording drift: %v", err)
	}
	if len(reasons) != 0 {
		t.Fatalf("structured assertions must stay green after render wording drift; got mismatches: %v", reasons)
	}

	// Coverage witness uses rule+effect pins only — wording drift must not change it.
	enforceObl := map[string]bool{"author-owns-entry": true}
	_, wBefore, err := adoptertest.RunCaseCoverage(c, enforceObl)
	if err != nil {
		t.Fatalf("RunCaseCoverage baseline: %v", err)
	}
	_, wAfter, err := adoptertest.RunCaseCoverage(c, enforceObl)
	if err != nil {
		t.Fatalf("RunCaseCoverage after drift sim: %v", err)
	}
	if !sameWitness(wBefore, wAfter) {
		t.Fatalf("coverage witness changed after wording drift:\n before=%+v\n after=%+v", wBefore, wAfter)
	}

	// Full pack replay stays green — render goldens are not assent test inputs.
	if code := runTest([]string{packDir}, &so, &se); code != 0 {
		t.Fatalf("assent test %s after drift sim: exit = %d, want 0", safetySplitPack, code)
	}
}

func hasMessageOnlyPin(exp adoptertest.Expectation) bool {
	if len(exp.Findings) == 0 {
		return false
	}
	for _, f := range exp.Findings {
		if f.Effect != "" || f.Rule != "" {
			return false
		}
		if f.Message != "" {
			return true
		}
	}
	return false
}

func sameWitness(a, b adoptertest.CaseWitness) bool {
	return sliceEqual(a.Proving, b.Proving) && sliceEqual(a.Failing, b.Failing)
}

func sliceEqual(a, b []string) bool {
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
