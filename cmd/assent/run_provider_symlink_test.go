package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// requireSymlinks skips loudly (never silently) where the runner cannot create a
// symlink — unprivileged Windows, exotic filesystems. Everywhere else these cases
// MUST run: they are the production-path containment proof for D-129 (provider
// reads) and D-133 (checkout reads). It is the package's single symlink guard —
// the checkout-containment cases in run_checkout_containment_test.go call it too.
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

// quotaCheckoutFake builds the forge double every case here shares: a quota
// policy whose bounded-change assert reads `facts.quota.max_partitions`, with the
// `builtin/repo-file` provider declared.
func quotaCheckoutFake(t *testing.T, headPartitions string) *fakeGitLab {
	t.Helper()
	f := newFakeGitLab(t)
	f.governedPath = "topics/prod/orders.yaml"
	f.mergePolicy = mergePolicyQuotaFromFact
	f.rulesetBinding = rulesetBindingBoundedChange
	f.config = configQuotaRepoFile()
	f.providerDecls = map[string]string{"quota": quotaDeclarationJSON}
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: " + headPartitions + "\n"
	return f
}

// runQuotaCheckoutRaw drives the PRODUCTION path — `assent run --checkout <dir>`
// with a builtin/repo-file provider — and returns the exit code and the emitted
// output, asserting nothing. Callers that expect a decision use runQuotaCheckout.
func runQuotaCheckoutRaw(t *testing.T, f *fakeGitLab, checkout string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := runRun(
		runArgs("--config", ".assent/config.yaml", "--checkout", checkout, "--subject", "file:topics/prod/orders.yaml"),
		env("tok"), fixedClock(), &out, &out, f.factory(),
	)
	return code, out.String()
}

// runQuotaCheckout is runQuotaCheckoutRaw for the cases that must DECIDE.
func runQuotaCheckout(t *testing.T, checkout, headPartitions string) string {
	t.Helper()
	code, body := runQuotaCheckoutRaw(t, quotaCheckoutFake(t, headPartitions), checkout)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (the run must complete and DECIDE, not crash — a crash would pass a containment assertion for the wrong reason)\n%s", code, body)
	}
	return body
}

// resolveQuotaFact drives the PRODUCTION fact resolver — the very function
// `runRun` step 5c calls — over a checkout tree, and returns the bound fact.
//
// This is the deepest slice of the live path that a planted symlink can still
// reach (see TestRunCheckoutSymlinkRefusedAtEnumeration): it traverses
// `checkoutFS` → `providerCallFor` → the SOLE production `builtin.RepoFileOpts`
// construction site (`provider_host.go:213`) → `CallRepoFile`'s walk-up →
// `ResolveFactsChecked` → `ToAggregateFact`. Nothing here is hand-wired: the FS,
// the roots, the anchor and the declaration all come from the same code the run
// uses.
func resolveQuotaFact(t *testing.T, checkout string) aggregate.Fact {
	t.Helper()
	f := newFakeGitLab(t)
	f.config = configQuotaRepoFile()
	f.providerDecls = map[string]string{"quota": quotaDeclarationJSON}
	conf, err := policy.LoadConfig([]byte(configQuotaRepoFile()))
	if err != nil {
		t.Fatal(err)
	}
	facts, resolvedAt, err := resolveRunFacts(
		context.Background(), conf, ".assent/config.yaml", f.factory()("", "tok", "assent-bot"),
		"42", "main", checkout, "file:topics/prod/orders.yaml", time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("resolveRunFacts: %v", err)
	}
	if _, ok := resolvedAt["quota"]; !ok {
		t.Fatalf("the quota provider must have been CALLED, else every assertion below is vacuous: %v", resolvedAt)
	}
	fact, ok := facts["quota"]["max_partitions"]
	if !ok {
		t.Fatalf("no facts.quota.max_partitions envelope bound; got %v", facts["quota"])
	}
	return fact
}

// TestRunCheckoutSymlinkRefusedAtEnumeration — D-129's original live-path
// scenario, now SUBSUMED by D-133 (this is the reshape, see the D-129 amendment).
//
// Threat model unchanged: `--checkout` points at the MR's head tree
// (cmd/assent/provider_host.go checkoutFS). Before D-129 the MR could ship
// `topics/prod/quota.yaml -> /abs/host/cluster-secrets.yaml`; os.DirFS followed
// it and the engine received max_partitions=31337 from OUTSIDE the checkout,
// approving a change no in-repo quota allows.
//
// WHAT CHANGED, and why this test no longer asserts a decision. D-133 refuses ANY
// symlink anywhere under base/ or head/ at changed-file ENUMERATION (run.go step
// 5b), which runs BEFORE providers resolve (step 5c). The run therefore never
// reaches the provider, and the scenario is unreachable on the live path: the
// outer layer fires first. Asserting "the run completes and decides" here would
// now be asserting a behaviour the tool deliberately no longer has.
//
// WHY THIS IS A TRIPWIRE, NOT A DEMOTION. D-133 does not make D-129's provider
// guard redundant — it makes it defence in depth. ADR-0008 Amendment 2 names the
// revisit direction explicitly: loosen the refusal by folding it OPAQUE
// (fail-safe REVIEW with a resolvable thread) rather than by following the link.
// The moment that fold lands, this run CONTINUES with a symlink present, the
// provider resolves, and D-129's guard is the live barrier again. So: when this
// assertion reds because the exit code became 0, that is not a stale test — it is
// this file telling you the fold landed and that
// TestResolveRunFactsRefusesSymlinkedQuotaCandidate (below) has just become the
// live-path proof. Re-point this case at the decision, keep that one.
//
// The exfiltration assertion is polarity-free and survives the subsumption: no
// off-tree byte may reach stdout or the forge, whichever layer refuses.
func TestRunCheckoutSymlinkRefusedAtEnumeration(t *testing.T) {
	requireSymlinks(t)
	secret := hostSecretFile(t, "31337")

	checkout := writeCheckout(t, map[string][2]string{
		"topics/quota.yaml":       {"max_partitions: 24\n", "max_partitions: 24\n"},
		"topics/prod/orders.yaml": {"partitions: 12\n", "partitions: 20\n"},
	})
	symlinkBothSides(t, checkout, "topics/prod/quota.yaml", secret)

	f := quotaCheckoutFake(t, "20")
	code, body := runQuotaCheckoutRaw(t, f, checkout)

	if code == 0 {
		t.Fatalf("exit = 0: the D-133 enumeration refusal no longer fires. If you landed ADR-0008 "+
			"Amendment 2 (fold the refusal opaque), the provider guard is the live barrier again — "+
			"re-point this case at the decision and keep "+
			"TestResolveRunFactsRefusesSymlinkedQuotaCandidate:\n%s", body)
	}
	// The message contract, not just the exit code: an unrelated hard error must
	// not be able to satisfy this tripwire. The refused path, not the side
	// directory — which side is walked first is behaviourally irrelevant.
	if !strings.Contains(body, "reached through a symlink") || !strings.Contains(body, "topics/prod/quota.yaml") {
		t.Fatalf("exit %d, but not the containment refusal — this test would pass for the wrong reason:\n%s", code, body)
	}
	if strings.Contains(body, "31337") {
		t.Fatalf("EXFILTRATION: an off-tree host value reached the operator-facing output:\n%s", body)
	}
	if strings.Contains(f.lastThreadBody, "31337") {
		t.Fatalf("EXFILTRATION: an off-tree host value reached the posted thread:\n%s", f.lastThreadBody)
	}
	if f.discussionsPosted != 0 || f.notesPosted != 0 || f.approvals != 0 || f.merges != 0 {
		t.Fatalf("a refused run must write nothing to the forge: discussions=%d notes=%d approvals=%d merges=%d",
			f.discussionsPosted, f.notesPosted, f.approvals, f.merges)
	}
}

// TestResolveRunFactsRefusesSymlinkedQuotaCandidate — D-129 (closes OQ-28) at the
// deepest seam the live path still reaches, and the coverage the case above hands
// over rather than drops.
//
// The DISTINGUISHING ASSERTION is preserved intact, which is the whole point of
// keeping this: a legitimate `topics/quota.yaml` (24) sits on the walk-up path and
// 20 <= 24, so a fix that merely SKIPPED the symlinked candidate and fell back
// would resolve 24 — a perfectly ordinary-looking success that silently masks the
// attack, and end-to-end an APPROVE. Only a refusal that STOPS the walk-up leaves
// the fact unresolved. "state != resolved" is therefore the only assertion that
// discriminates, and the control subtest keeps "refuse everything" from passing.
//
// The two poisoned subtests pin DIFFERENT layers, and say so rather than blurring
// them (D-133 was corrected once for exactly this conjunction-gate blur):
//
//   - absolute off-tree link — refused by EITHER layer (the os.Root FS blocks the
//     escape at the syscall level; the builtin refuses the link). It pins the
//     combined OUTCOME plus the no-31337 exfiltration property.
//   - relative in-root link — the os.Root FS follows it happily, because it never
//     leaves the root. The builtin's refusal is the ONLY thing standing between
//     the run and a resolved 24. That is layer (b) in isolation.
func TestResolveRunFactsRefusesSymlinkedQuotaCandidate(t *testing.T) {
	cleanTree := func(t *testing.T) string {
		t.Helper()
		return writeCheckout(t, map[string][2]string{
			"topics/quota.yaml":       {"max_partitions: 24\n", "max_partitions: 24\n"},
			"topics/prod/orders.yaml": {"partitions: 12\n", "partitions: 20\n"},
		})
	}

	t.Run("absolute off-tree link: both layers, and nothing leaks", func(t *testing.T) {
		requireSymlinks(t)
		secret := hostSecretFile(t, "31337")
		checkout := cleanTree(t)
		symlinkBothSides(t, checkout, "topics/prod/quota.yaml", secret)

		fact := resolveQuotaFact(t, checkout)
		if fact.State == "resolved" {
			t.Fatalf("FACT ESCAPE: a symlinked candidate resolved to %v — a refusal must stop the "+
				"walk-up, not skip to the next candidate (reason %q)", fact.Value, fact.Reason)
		}
		if fact.Value != nil {
			t.Fatalf("an unresolved fact must carry no value; got %v", fact.Value)
		}
		if strings.Contains(fact.Reason, "31337") {
			t.Fatalf("EXFILTRATION: the off-tree value reached the fact reason (a rendered surface): %q", fact.Reason)
		}
	})

	t.Run("relative in-root link: the builtin guard alone", func(t *testing.T) {
		requireSymlinks(t)
		checkout := cleanTree(t)
		// Resolves back INSIDE the root, so (*os.Root).FS() follows it: without the
		// builtin refusal this reads topics/quota.yaml and resolves 24.
		symlinkBothSides(t, checkout, "topics/prod/quota.yaml", "../quota.yaml")

		fact := resolveQuotaFact(t, checkout)
		if fact.State == "resolved" {
			t.Fatalf("FACT ESCAPE: an in-root symlinked candidate resolved to %v — the symlink-safe "+
				"root cannot see this one, so the builtin refusal is the only barrier and it is gone "+
				"(reason %q)", fact.Value, fact.Reason)
		}
		if !strings.Contains(fact.Reason, "symlink") {
			t.Fatalf("the refusal must tell the contributor WHY, naming the symlink; got %q", fact.Reason)
		}
	})

	t.Run("control: an ordinary tree still resolves", func(t *testing.T) {
		fact := resolveQuotaFact(t, cleanTree(t))
		if fact.State != "resolved" {
			t.Fatalf("a symlink-free walk-up must still resolve; got state %q reason %q", fact.State, fact.Reason)
		}
		// Compared as text: the envelope carries the decoded YAML scalar, and its
		// Go numeric type is not what this case is about.
		if got := fmt.Sprint(fact.Value); got != "24" {
			t.Fatalf("walk-up must reach topics/quota.yaml (max_partitions: 24); got %q", got)
		}
	})
}

// TestRunCheckoutLegitimateQuotaStillResolves — the other polarity, on the
// production route: with no symlink in the tree the walk-up still resolves and the
// decision still APPROVEs. Without this, "refuse everything" would pass the
// enumeration-refusal test above.
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
