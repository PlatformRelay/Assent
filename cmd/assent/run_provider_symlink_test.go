package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// requireSymlinks skips loudly (never silently) where the runner cannot create a
// symlink — unprivileged Windows, exotic filesystems. Everywhere else these cases
// MUST run: they are the production-path containment proof for D-129.
func requireSymlinks(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "target"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable on %s (%v) — checkout containment cannot be proven here", runtime.GOOS, err)
	}
}

// hostSecretFile writes an off-tree host file shaped like a quota document. It is
// the exfiltration target: a YAML mapping carrying a declared output name, which
// is the only shape `builtin/repo-file` will hand to the decision engine.
func hostSecretFile(t *testing.T, partitions string) string {
	t.Helper()
	dir := t.TempDir() // NOT under the checkout root
	p := filepath.Join(dir, "cluster-secrets.yaml")
	if err := os.WriteFile(p, []byte("max_partitions: "+partitions+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// symlinkBothSides plants the same symlink in base/ and head/ so the poisoned path
// is byte-identical on both sides and therefore NOT a changed file: the run then
// turns solely on the resolved fact, with no extra unclassified changed file
// nudging the verdict. (A head-only symlink is the real-world MR shape and is
// strictly *more* likely to be refused — this is the harder test.)
func symlinkBothSides(t *testing.T, checkout, rel, target string) {
	t.Helper()
	for _, side := range []string{"base", "head"} {
		link := filepath.Join(checkout, side, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("symlink %s -> %s: %v", link, target, err)
		}
	}
}

// runQuotaCheckout drives the PRODUCTION path — `assent run --checkout <dir>` with
// a builtin/repo-file provider — and returns the emitted output.
func runQuotaCheckout(t *testing.T, checkout, headPartitions string) string {
	t.Helper()
	f := newFakeGitLab(t)
	f.governedPath = "topics/prod/orders.yaml"
	f.mergePolicy = mergePolicyQuotaFromFact
	f.rulesetBinding = rulesetBindingBoundedChange
	f.config = configQuotaRepoFile()
	f.providerDecls = map[string]string{"quota": quotaDeclarationJSON}
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: " + headPartitions + "\n"

	var out bytes.Buffer
	code := runRun(
		runArgs("--config", ".assent/config.yaml", "--checkout", checkout, "--subject", "file:topics/prod/orders.yaml"),
		env("tok"), fixedClock(), &out, &out, f.factory(),
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (the run must complete and DECIDE, not crash — a crash would pass a containment assertion for the wrong reason)\n%s", code, out.String())
	}
	return out.String()
}

// TestRunCheckoutSymlinkFactEscapeRefused — D-129 (closes OQ-28) on the LIVE path.
//
// Threat model: assent evaluates merge requests, and `--checkout` points at the
// MR's head tree (cmd/assent/provider_host.go checkoutFS). Before D-129 the MR
// itself could ship `topics/prod/quota.yaml -> /abs/host/cluster-secrets.yaml`;
// os.DirFS followed it and the engine received max_partitions=31337 from OUTSIDE
// the checkout, approving a change no in-repo quota allows.
//
// Distinguishing assertion: a legitimate topics/quota.yaml (24) sits on the walk-up
// path and 20 <= 24, so a fix that merely SKIPPED the symlinked candidate and fell
// back would still APPROVE — silently masking the attack. Only a hard containment
// refusal makes the fact non-resolved and the decision non-APPROVE.
func TestRunCheckoutSymlinkFactEscapeRefused(t *testing.T) {
	requireSymlinks(t)
	secret := hostSecretFile(t, "31337")

	checkout := writeCheckout(t, map[string][2]string{
		"topics/quota.yaml":       {"max_partitions: 24\n", "max_partitions: 24\n"},
		"topics/prod/orders.yaml": {"partitions: 12\n", "partitions: 20\n"},
	})
	symlinkBothSides(t, checkout, "topics/prod/quota.yaml", secret)

	body := runQuotaCheckout(t, checkout, "20")

	if strings.Contains(body, "31337") {
		t.Fatalf("EXFILTRATION: an off-tree host value reached the forge-facing output:\n%s", body)
	}
	if strings.Contains(body, `"decision":"APPROVE"`) {
		t.Fatalf("a symlinked quota candidate must not prove bounded-change (fail closed):\n%s", body)
	}
	if !strings.Contains(body, `"decision":`) {
		t.Fatalf("run emitted no decision — assertion above would be vacuous:\n%s", body)
	}
}

// TestRunCheckoutLegitimateQuotaStillResolves — the other polarity, on the same
// production route: with no symlink in the tree the walk-up still resolves and the
// decision still APPROVEs. Without this, "refuse everything" would pass the test
// above.
func TestRunCheckoutLegitimateQuotaStillResolves(t *testing.T) {
	checkout := writeCheckout(t, map[string][2]string{
		"topics/quota.yaml":       {"max_partitions: 24\n", "max_partitions: 24\n"},
		"topics/prod/orders.yaml": {"partitions: 12\n", "partitions: 20\n"},
	})

	body := runQuotaCheckout(t, checkout, "20")

	if !strings.Contains(body, `"decision":"APPROVE"`) {
		t.Fatalf("legitimate in-root walk-up (20 <= 24) must still resolve and APPROVE:\n%s", body)
	}
	if strings.Contains(body, `"factsResolvedAt":{}`) {
		t.Fatalf("quota fact must still resolve through the checkout route:\n%s", body)
	}
}
