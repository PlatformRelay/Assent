package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// REQ-AUD-S16-01 (production entry point, both polarities): `assent compare --suite`
// accepts the D-121 domain-separated digest and REFUSES the old-format undomained
// `sha256:<hex>` pin, fail-closed before evaluation. Driving runCompare — not just
// compare.RunSuite — proves the branch is reachable through the shipped CLI.
//
// Both pins are literals taken from the real corpus: the legacy one is the exact
// value examples/comparison/promotion-gates/suite.yaml carried before this migration.
func TestReplayBundleDigestCompareCLIRejectsOldFormatPin(t *testing.T) {
	const (
		legacyUndomainedPin = "sha256:c1223668c50254d873361347ea6e30deec8baf75c010c855a9cc697a44011000"
		domainSeparatedPin  = "7243562c4614e4bebe14f10db4d29ba64765ef1041a5c820518be852d0a0aef5"
	)

	bundlePath := filepath.Clean("../../examples/comparison/promotion-gates/cases/partition-grow-agree/bundle.json")
	bundle, err := os.ReadFile(bundlePath) //nolint:gosec // fixed, committed corpus path
	if err != nil {
		t.Fatalf("read corpus bundle: %v", err)
	}

	// Guard the premise: the two pins must really disagree, and the production
	// digest must be the domain-separated one.
	if got := suiteDigest(t, string(bundle)); got != domainSeparatedPin {
		t.Fatalf("compare.ReplayBundleDigest = %q, want the D-121 pin %q", got, domainSeparatedPin)
	}

	run := func(t *testing.T, pin string) (int, string, string) {
		t.Helper()
		dir := writeSuiteDir(t,
			mkSuiteJSON([]struct{ id, digest string }{{"partition-grow-agree", pin}}),
			map[string]string{"partition-grow-agree": string(bundle)},
			mergePolicyYAML("prod-strict-6", "new >= old", "must not shrink"),
			mergePolicyYAML("prod-strict-7", "new >= old", "must not shrink"),
		)
		var out, errb bytes.Buffer
		code := runCompare([]string{"--suite", dir}, &out, &errb)
		return code, out.String(), errb.String()
	}

	t.Run("accepted/domain-separated", func(t *testing.T) {
		code, out, errb := run(t, domainSeparatedPin)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stdout=%q stderr=%q", code, out, errb)
		}
	})

	t.Run("rejected/old-format-undomained", func(t *testing.T) {
		code, out, errb := run(t, legacyUndomainedPin)
		if code == 0 {
			t.Fatalf("assent compare --suite accepted a pre-D-121 pin; stdout=%q", out)
		}
		if !bytes.Contains([]byte(errb+out), []byte("digest mismatch")) {
			t.Fatalf("stderr=%q stdout=%q, want a fail-closed replay bundle digest mismatch", errb, out)
		}
	})
}
