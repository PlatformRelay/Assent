package main

import (
	"os"
	"strings"
	"testing"
)

// guards_source_test.go holds the SOURCE-LEVEL guards E10-S02 (REQ-E10-S02-05)
// calls for explicitly: a green behaviour test against a fake that happens to
// serve the right bytes does not prove the call was rewritten, so the
// governed-subject migration is pinned by reading run.go's and
// provider_host.go's own text.
//
// A source-level guard is a predicate over text — which is why the shapes it
// asserts are single, exact literals: a guard loose enough to survive any
// refactor is a guard that proves nothing.

// TestGovernedSubjectReadsAreMRRelative asserts the migration is a CALL-SITE
// rewrite, not a signature that a refactor can quietly re-collapse: the
// governed-subject path must call FileAtBase/FileAtHead (MR-relative) and the
// ref-addressed helper must be GONE from the governed path, while the policy
// loads keep their exact FileAtRef shape.
//
// Scope (per the epic spec): this binds the forge-sourced path only. Under
// --checkout, run.go overrides base/head from the local tree via
// dirCheckout.FileContents, which carries no forge call at all and is invisible
// here — that is EFE-S03's intended existing behaviour, not this guard's
// business.
func TestGovernedSubjectReadsAreMRRelative(t *testing.T) {
	raw, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatal(err)
	}
	src := codeLines(string(raw))

	if !strings.Contains(src, "fileAtBaseOrAbsent(client, cfg.project, cfg.mr, governed)") {
		t.Fatal("run.go must read the governed BASE via the MR-relative FileAtBase accessor " +
			"(ADR-0021 item 5) — a ref-inside-one-project read mints a fabricated whole-file DELETE on fork PRs")
	}
	if !strings.Contains(src, "fileAtHeadOrAbsent(client, cfg.project, cfg.mr, governed)") {
		t.Fatal("run.go must read the governed HEAD via the MR-relative FileAtHead accessor " +
			"(S00 Q1: the fork's head lives in the source repository, never addressable by a branch name in the target project)")
	}
	if strings.Contains(src, "fileAtRefOrAbsent(client") {
		t.Fatal("the governed-subject path must not call FileAtRef at all — fileAtRefOrAbsent was retired with E10-S02")
	}
	// The six policy loads stay ref-addressed, deliberately (ADR-0015 §1): four
	// in run.go (MergePolicy, RulesetBinding, Config, pack) and the registry read
	// in provider_host.go (D-130: it decides WHO MAY APPROVE, so it must never
	// move onto an MR-relative accessor). Counting the exact shapes is the
	// polarity that catches a well-meaning "consistency" refactor onto an
	// MR-relative accessor — which would let a fork's head reach a policy load.
	if got := strings.Count(src, "client.FileAtRef(cfg.project"); got != 4 {
		t.Fatalf("run.go must keep exactly the four ref-addressed POLICY loads on FileAtRef, found %d", got)
	}
}

// TestProviderHostRegistryStaysRefAddressed is the provider_host.go half of the
// same invariant: BOTH decision-input reads there stay on FileAtRef at the
// target ref — the provider-host declaration (D-065) and the resource-owner
// registry, which decides WHO MAY APPROVE (D-130: it once preferred the
// checkout, letting an MR ship its own owner registry).
func TestProviderHostRegistryStaysRefAddressed(t *testing.T) {
	raw, err := os.ReadFile("provider_host.go")
	if err != nil {
		t.Fatal(err)
	}
	src := codeLines(string(raw))
	if got := strings.Count(src, "client.FileAtRef(project"); got != 2 {
		t.Fatalf("provider_host.go must keep both ref-addressed decision-input reads (host declaration + D-130 registry) on FileAtRef, found %d", got)
	}
	if strings.Contains(src, "FileAtBase(") || strings.Contains(src, "FileAtHead(") {
		t.Fatal("provider_host.go must not reach governed MR content — its reads are target-ref decision inputs, never MR-relative")
	}
}

// codeLines blanks whole-line `//` comments but keeps their line count (the
// same scan shape hack/lint/depguard_test.sh's scanners use), so guards read
// code, not prose. Block comments are refused by the lint gate, so this helper
// is only ever fed files the gate already vetted.
func codeLines(src string) string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			out = append(out, "")
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
