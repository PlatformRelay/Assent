package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/core/policy"
	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/provider/builtin"
)

// registryStub answers FileAtRef for the ownership registry only, recording the
// ref it was asked for — the trust-boundary evidence (GUIDELINES §Safety 3).
type registryStub struct {
	raw     []byte
	err     error
	gotRefs []string
}

func (s *registryStub) FileAtRef(_, _, ref string) ([]byte, error) {
	s.gotRefs = append(s.gotRefs, ref)
	if s.err != nil {
		return nil, s.err
	}
	return s.raw, nil
}

func ownersTree(t *testing.T, owner string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "governance"), 0o750); err != nil {
		t.Fatal(err)
	}
	body := "owners:\n  kafka-topic:payments.settled.v2: " + owner + "\n"
	if err := os.WriteFile(filepath.Join(dir, "governance", "owners.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// absentAtRef is the error shape the real adapter returns for a file that is
// genuinely missing at the ref: the neutral port sentinel wrapped with the
// adapter's own prefix (`internal/forge/gitlab.FileAtRef`, 404 branch). The
// stubs use the WRAPPED form on purpose — matching must survive the wrap chain,
// which a bare sentinel would not prove.
func absentAtRef(regPath, ref string) error {
	return fmt.Errorf("gitlab: %w: file %q at ref %q", forge.ErrNotFound, regPath, ref)
}

// brokenForge is the error shape the real adapter returns for a NON-404 status
// — a forge that is down, throttled, or misconfigured (`gitlab.FileAtRef`,
// default branch). It is NOT absence, and it must never be read as absence.
func brokenForge(regPath, ref string, status int) error {
	return fmt.Errorf("gitlab: get file %q at ref %q: unexpected status %d", regPath, ref, status)
}

func ownerOf(t *testing.T, client builtin.ResourceOwnerClient) string {
	t.Helper()
	owner, err := client.Owner(context.Background(), "kafka-topic:payments.settled.v2")
	if err != nil {
		t.Fatalf("owner lookup: %v", err)
	}
	return owner
}

// TestResourceOwnerRegistryLoadsFromTargetRef — D-130 (GUIDELINES §Safety 3).
//
// The ownership registry DECIDES WHO MAY APPROVE, so it is a decision input and
// must load from the TARGET ref. loadResourceOwnerRegistry used to prefer the
// checkout tree — which under `--checkout` is the merge request's own head — so an
// MR could ship `governance/owners.yaml` naming its author as owner of the
// resource it is changing and satisfy an ownership obligation with it.
func TestResourceOwnerRegistryLoadsFromTargetRef(t *testing.T) {
	poisoned := ownersTree(t, "attacker") // the MR head tree
	repoFS, closer, err := checkoutFS(poisoned)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	stub := &registryStub{raw: []byte("owners:\n  kafka-topic:payments.settled.v2: team-payments\n")}
	client, err := loadResourceOwnerRegistry(context.Background(), stub, "42", "main", repoFS, "governance/owners.yaml")
	if err != nil {
		t.Fatalf("registry load: %v", err)
	}
	if got := ownerOf(t, client); got != "team-payments" {
		t.Fatalf("owner = %q, want team-payments — the MR head tree must NOT shadow the target-ref registry", got)
	}
	if len(stub.gotRefs) == 0 || stub.gotRefs[0] != "main" {
		t.Fatalf("registry must be fetched from the target ref first; refs asked = %v", stub.gotRefs)
	}
}

// TestResourceOwnerRegistryFallsBackToCheckout — compat: when the target ref has
// no registry (hermetic/local runs, or a repo that keeps it only in the checkout),
// the checkout copy is still used. Fallback direction only — never a shadow.
func TestResourceOwnerRegistryFallsBackToCheckout(t *testing.T) {
	local := ownersTree(t, "team-payments")
	repoFS, closer, err := checkoutFS(local)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	stub := &registryStub{err: absentAtRef("governance/owners.yaml", "main")}
	client, err := loadResourceOwnerRegistry(context.Background(), stub, "42", "main", repoFS, "governance/owners.yaml")
	if err != nil {
		t.Fatalf("registry load: %v", err)
	}
	if got := ownerOf(t, client); got != "team-payments" {
		t.Fatalf("owner = %q, want team-payments from the checkout fallback", got)
	}
}

// TestResourceOwnerRegistryTransientForgeErrorNeverFallsBackToCheckout — the
// fallback is gated on ABSENCE, not on "the forge answered badly".
//
// The registry decides WHO MAY APPROVE (D-130 / GUIDELINES §Safety 2+3). If any
// FileAtRef error opened the fallback, a forge that is merely DOWN — a 503, a
// throttle, a proxy hiccup — would hand the who-may-approve document to the
// merge request's own head tree, with no error surfaced and no trace in the
// decision: exactly the shadow D-130 claims is impossible. A broken forge is
// not an absent file, so it degrades to an error — no client, and the error
// propagates out of resolveRunFacts and aborts the run (no DecisionRecord, no
// forge write; see TestResourceOwnerRegistryForgeErrorAbortsResolveRunFacts) —
// never to a contributor-authored registry.
//
// Shape note: the 404 variant of this attack is separately mitigated (a
// whole-file add of the registry folds opaque → REVIEW). The unmitigated shape
// is MODIFY-plus-transient-error — a plain value diff on an existing file —
// which is what this test drives: the checkout carries a registry the MR
// rewrote, and the target-ref read fails with a non-404.
func TestResourceOwnerRegistryTransientForgeErrorNeverFallsBackToCheckout(t *testing.T) {
	poisoned := ownersTree(t, "attacker") // the MR head tree, registry MODIFIED
	repoFS, closer, err := checkoutFS(poisoned)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	for _, status := range []int{500, 502, 503, 429, 401} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			stub := &registryStub{err: brokenForge("governance/owners.yaml", "main", status)}
			client, err := loadResourceOwnerRegistry(
				context.Background(), stub, "42", "main", repoFS, "governance/owners.yaml")
			if err == nil {
				t.Fatalf("OWNERSHIP SHADOW: a %d from the forge yielded a registry; owner = %q "+
					"— a transient forge error must never promote the MR head tree to the "+
					"who-may-approve authority", status, ownerOf(t, client))
			}
			if client != nil {
				t.Fatalf("error path must yield no client; got %#v", client)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%d", status)) {
				t.Fatalf("error %q must carry the forge's own failure so the operator can see WHY", err)
			}
		})
	}
}

// TestResourceOwnerRegistryBothMissingFailsClosed — neither side has a registry:
// an error (no client → the owner fact never resolves), never an empty map that
// would make every resource unowned.
func TestResourceOwnerRegistryBothMissingFailsClosed(t *testing.T) {
	empty := t.TempDir()
	repoFS, closer, err := checkoutFS(empty)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	stub := &registryStub{err: absentAtRef("governance/owners.yaml", "main")}
	client, err := loadResourceOwnerRegistry(context.Background(), stub, "42", "main", repoFS, "governance/owners.yaml")
	if err == nil {
		t.Fatalf("missing registry must be an error, got client %#v", client)
	}
}

// TestResourceOwnerRegistrySymlinkInCheckoutRefused — D-129 at the cmd edge: the
// checkout fallback must not read a registry through a symlink either.
//
// WHAT THIS PINS, precisely: the OUTCOME of the two D-129 layers TOGETHER, not
// the builtin guard on its own. With the symlink-safe root that `checkoutFS`
// injects, `fs.Stat(repoFS, regPath)` already errors on the escaping link, so
// the fallback is never entered and `LoadResourceOwnerMap`'s own refusal is
// never reached — this test stays green if you delete the builtin guard but
// keep the root, and green if you keep the guard but the root refuses first. It
// reds only when BOTH layers are gone, which is the honest reading: it is the
// end-to-end "no registry through a symlink at the cmd edge" assertion.
// Layer (b) in isolation — the builtin's own refusal, which is the only layer
// that survives a non-root FS — is pinned by
// `internal/provider/builtin/resource_owner_symlink_test.go`.
func TestResourceOwnerRegistrySymlinkInCheckoutRefused(t *testing.T) {
	requireSymlinks(t)

	secret := t.TempDir()
	secretFile := filepath.Join(secret, "owners.yaml")
	if err := os.WriteFile(secretFile, []byte("owners:\n  kafka-topic:payments.settled.v2: attacker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tree := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tree, "governance"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secretFile, filepath.Join(tree, "governance", "owners.yaml")); err != nil {
		t.Fatal(err)
	}
	repoFS, closer, err := checkoutFS(tree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	stub := &registryStub{err: absentAtRef("governance/owners.yaml", "main")}
	client, err := loadResourceOwnerRegistry(context.Background(), stub, "42", "main", repoFS, "governance/owners.yaml")
	if err == nil {
		t.Fatalf("OWNERSHIP ESCAPE: symlinked registry accepted; owner = %q", ownerOf(t, client))
	}
}

// TestResolveRunFactsFailsLoudlyOnUnopenableCheckout — D-129 behaviour change:
// a --checkout that cannot be opened as a containment root is a HARD error out of
// resolveRunFacts, not a silent degrade to "no facts". Nothing upstream catches it
// first (run.go's governed read and foldCheckout both tolerate a missing checkout),
// so without this the run would evaluate a provider-configured policy with an empty
// Facts map and never say why.
func TestResolveRunFactsFailsLoudlyOnUnopenableCheckout(t *testing.T) {
	f := newFakeGitLab(t)
	f.config = configQuotaRepoFile()
	f.providerDecls = map[string]string{"quota": quotaDeclarationJSON}
	client := f.factory()("", "tok", "assent-bot")

	conf, err := policy.LoadConfig([]byte(configQuotaRepoFile()))
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "no-such-checkout")

	facts, resolvedAt, err := resolveRunFacts(
		context.Background(), conf, ".assent/config.yaml", client,
		"42", "main", missing, "file:topics/prod/orders.yaml", time.Now().UTC(),
	)
	if err == nil {
		t.Fatalf("an unopenable --checkout must be a hard error; got facts=%v resolvedAt=%v", facts, resolvedAt)
	}
	if !strings.Contains(err.Error(), "checkout root") {
		t.Fatalf("error %q must name the checkout root it could not open", err)
	}
	// Fail-closed shape: no partial fact map escapes alongside the error.
	if facts != nil || resolvedAt != nil {
		t.Fatalf("error path must return no facts; got %v / %v", facts, resolvedAt)
	}
}

// resourceOwnerDeclarationJSON is the host-owned declaration (D-065) that wires
// the `builtin/resource-owner` provider to a registry path — the only shape that
// reaches loadResourceOwnerRegistry from resolveRunFacts.
//
// The outputs block MIRRORS `builtin.OwnerDeclaration()` field for field, and it
// has to: `provider.ResolveFactsChecked` compares the provider's echoed
// declaration against the host's with `DeclarationsEqual` (type, cardinality,
// subject, sensitive, maxAge — `internal/provider/resolve.go`) and synthesizes
// `state:"invalid"` / "provider echoed declaration does not match host config" on
// any difference, whatever the registry says. This fixture used to declare output
// `team` at `maxAge 1h` while the builtin emits `owner` at 24h, so the fact was
// PERMANENTLY invalid — copy this shape, not an invented one.
// Pinned by TestResourceOwnerDeclarationResolvesOwnerFact.
const resourceOwnerDeclarationJSON = `{
  "name": "owner",
  "requests": {"values": {"pointers": []}},
  "resourceOwner": {"registry": "governance/owners.yaml"},
  "outputs": {
    "owner": {
      "type": "string",
      "cardinality": "single",
      "subject": "entry",
      "sensitive": false,
      "maxAge": "24h"
    }
  }
}`

// configOwnerResourceOwner configures the controlling `owner` provider as the
// resource-owner builtin. No example, exit gate or run test configured this type
// before, so nothing in the tree entered providerCallFor's resource-owner arm at
// all; the tests below are the first, and they enter it on both the error path
// (TestResourceOwnerRegistryForgeErrorAbortsResolveRunFacts) and the healthy
// resolve (TestResourceOwnerDeclarationResolvesOwnerFact).
const configOwnerResourceOwner = `apiVersion: assent.dev/v1alpha1
kind: Config
environments:
  - name: prod
    match: { paths: ["topics/**"] }
classes:
  - name: topic-registry
    match: { paths: ["topics/**.yaml"] }
providers:
  owner:
    type: builtin/resource-owner
    failure: closed
`

// registry503Port is the whole forge with ONE read broken: the ownership
// registry answers 503, everything else (the host declaration included) answers
// normally. That is the live shape of a flaky forge, which a stub covering only
// loadResourceOwnerRegistry cannot express.
type registry503Port struct {
	forgePort
	regPath string
}

func (c registry503Port) FileAtRef(project, p, ref string) ([]byte, error) {
	if p == c.regPath {
		return nil, brokenForge(p, ref, 503)
	}
	return c.forgePort.FileAtRef(project, p, ref)
}

// TestResourceOwnerRegistryForgeErrorAbortsResolveRunFacts — the wiring the unit
// tests cannot see: providerCallFor → loadResourceOwnerRegistry → resolveRunFacts.
//
// TestResourceOwnerRegistryTransientForgeErrorNeverFallsBackToCheckout pins the
// LOADER's refusal; nothing pinned that the refusal actually leaves the host, and
// providerCallFor's resource-owner branch is the one arm no cmd-level test ever
// entered. Drop the error there (`return nil, err` → `return nil, nil`) and the
// provider is silently skipped: the run continues with an empty fact map and the
// forge failure is never mentioned. This test reds on exactly that mutation.
func TestResourceOwnerRegistryForgeErrorAbortsResolveRunFacts(t *testing.T) {
	f := newFakeGitLab(t)
	f.config = configOwnerResourceOwner
	f.providerDecls = map[string]string{"owner": resourceOwnerDeclarationJSON}
	client := registry503Port{
		forgePort: f.factory()("", "tok", "assent-bot"),
		regPath:   "governance/owners.yaml",
	}

	conf, err := policy.LoadConfig([]byte(configOwnerResourceOwner))
	if err != nil {
		t.Fatal(err)
	}
	// The checkout carries a registry the MR rewrote: if the 503 opened the
	// fallback here, `attacker` would become the who-may-approve authority.
	poisoned := ownersTree(t, "attacker")

	facts, resolvedAt, err := resolveRunFacts(
		context.Background(), conf, ".assent/config.yaml", client,
		"42", "main", poisoned, "file:topics/orders.yaml", time.Now().UTC(),
	)
	if err == nil {
		t.Fatalf("a 503 on the ownership registry must abort fact resolution; "+
			"got facts=%v resolvedAt=%v", facts, resolvedAt)
	}
	if !strings.Contains(err.Error(), `provider "owner"`) || !strings.Contains(err.Error(), "503") {
		t.Fatalf("error %q must name the provider and carry the forge's own failure", err)
	}
	// Fail-closed shape: no partial fact map escapes beside the error, so no
	// caller can evaluate a provider-configured policy on an empty Facts map.
	if facts != nil || resolvedAt != nil {
		t.Fatalf("error path must return no facts; got %v / %v", facts, resolvedAt)
	}
}

// registryServingPort is the whole forge with the ownership registry answering
// normally from the target ref; every other read falls through to the fake.
type registryServingPort struct {
	forgePort
	regPath string
	body    string
}

func (c registryServingPort) FileAtRef(project, p, ref string) ([]byte, error) {
	if p == c.regPath {
		return []byte(c.body), nil
	}
	return c.forgePort.FileAtRef(project, p, ref)
}

// TestResourceOwnerDeclarationResolvesOwnerFact — the fixture guard.
//
// Every other cmd-level resource-owner test here asserts a REFUSAL, and a refusal
// looks identical whether the wiring is healthy or the declaration is nonsense. So
// nothing pinned the HEALTHY resolve, and the declaration fixture above was in fact
// broken: it named output `team` at `maxAge 1h` while `builtin/resource-owner`
// emits `owner` at 24h, so ResolveFactsChecked's DeclarationsEqual check
// synthesized `state:"invalid"` ("provider echoed declaration does not match host
// config") on every run — a fact that can never satisfy an ownership obligation,
// in a fixture the next author copies. This test reds on either mutation (the
// output name or the maxAge) and on any regression that stops the target-ref
// registry from reaching the builtin.
func TestResourceOwnerDeclarationResolvesOwnerFact(t *testing.T) {
	f := newFakeGitLab(t)
	f.config = configOwnerResourceOwner
	f.providerDecls = map[string]string{"owner": resourceOwnerDeclarationJSON}
	client := registryServingPort{
		forgePort: f.factory()("", "tok", "assent-bot"),
		regPath:   "governance/owners.yaml",
		body:      "owners:\n  topics/orders.yaml: team-payments\n",
	}

	conf, err := policy.LoadConfig([]byte(configOwnerResourceOwner))
	if err != nil {
		t.Fatal(err)
	}
	facts, resolvedAt, err := resolveRunFacts(
		context.Background(), conf, ".assent/config.yaml", client,
		"42", "main", "", "file:topics/orders.yaml", time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("resolveRunFacts: %v", err)
	}
	if _, ok := resolvedAt["owner"]; !ok {
		t.Fatalf("pins.factsResolvedAt must carry the owner provider; got %v", resolvedAt)
	}
	fact, ok := facts["owner"][builtin.OutputOwner]
	if !ok {
		t.Fatalf("no facts.owner.%s bound; the declaration must name the builtin's output (got %v)",
			builtin.OutputOwner, facts["owner"])
	}
	if fact.State != "resolved" {
		t.Fatalf("facts.owner.%s state = %q (reason %q), want resolved — the host declaration must "+
			"mirror builtin.OwnerDeclaration()", builtin.OutputOwner, fact.State, fact.Reason)
	}
	if fact.Value != "team-payments" {
		t.Fatalf("facts.owner.%s value = %v, want team-payments from the target-ref registry",
			builtin.OutputOwner, fact.Value)
	}
}

// TestProviderHostRepoFileContractDocumented pins that the FS the cmd edge injects
// really is a symlink-safe root and not a bare os.DirFS: an escaping symlink read
// through it must fail, whatever the builtin does on top.
func TestProviderHostInjectsSymlinkSafeRoot(t *testing.T) {
	requireSymlinks(t)

	secret := hostSecretFile(t, "31337")
	tree := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tree, "head", "topics"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(tree, "head", "topics", "quota.yaml")); err != nil {
		t.Fatal(err)
	}
	repoFS, closer, err := checkoutFS(tree)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closer.Close() })

	if _, err := fs.ReadFile(repoFS, "topics/quota.yaml"); err == nil {
		t.Fatal("checkoutFS must hand out a symlink-safe root: reading an escaping symlink succeeded")
	}
	// Sanity: the same path read through a bare os.DirFS DOES escape — the guard
	// above is not vacuous.
	if _, err := fs.ReadFile(os.DirFS(filepath.Join(tree, "head")), "topics/quota.yaml"); err != nil {
		t.Fatalf("control: os.DirFS should still follow the symlink (else this test proves nothing): %v", err)
	}
}
