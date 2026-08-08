package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

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

	stub := &registryStub{err: errors.New("404 file not found")}
	client, err := loadResourceOwnerRegistry(context.Background(), stub, "42", "main", repoFS, "governance/owners.yaml")
	if err != nil {
		t.Fatalf("registry load: %v", err)
	}
	if got := ownerOf(t, client); got != "team-payments" {
		t.Fatalf("owner = %q, want team-payments from the checkout fallback", got)
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

	stub := &registryStub{err: errors.New("404 file not found")}
	client, err := loadResourceOwnerRegistry(context.Background(), stub, "42", "main", repoFS, "governance/owners.yaml")
	if err == nil {
		t.Fatalf("missing registry must be an error, got client %#v", client)
	}
}

// TestResourceOwnerRegistrySymlinkInCheckoutRefused — D-129 at the cmd edge: the
// checkout fallback must not read a registry through a symlink either.
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

	stub := &registryStub{err: errors.New("404 file not found")}
	client, err := loadResourceOwnerRegistry(context.Background(), stub, "42", "main", repoFS, "governance/owners.yaml")
	if err == nil {
		t.Fatalf("OWNERSHIP ESCAPE: symlinked registry accepted; owner = %q", ownerOf(t, client))
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
