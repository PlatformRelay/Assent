package builtin_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/provider"
	"github.com/PlatformRelay/assent/internal/provider/builtin"
)

// TestResourceOwnerRegistrySymlinkRefused — D-129 sibling assessment.
//
// LoadResourceOwnerMap reads the SAME filesystem as builtin/repo-file and with NO
// declared roots at all, and the registry it loads decides WHO MAY APPROVE. Read
// through a bare os.DirFS a symlinked registry path resolves an off-tree host file
// into the ownership map; the containment rule must be identical here — a registry
// reached through a symlink is refused, and refusal is an ERROR (the caller then
// has no client and the owner fact never resolves), never a partial load.
func TestResourceOwnerRegistrySymlinkRefused(t *testing.T) {
	requireSymlinks(t)

	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	outside := filepath.Join(tmp, "outside")
	for _, dir := range []string{filepath.Join(repo, "governance"), outside} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	// The attacker-chosen registry: it names the MR author as owner of everything.
	if err := os.WriteFile(filepath.Join(outside, "owners.yaml"),
		[]byte("owners:\n  kafka-topic:payments.settled.v2: attacker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, filepath.Join(outside, "owners.yaml"), filepath.Join(repo, "governance", "owners.yaml"))

	for _, flavour := range []string{"dirfs", "rootfs"} {
		t.Run(flavour, func(t *testing.T) {
			client, err := builtin.LoadResourceOwnerMap(openFlavour(t, flavour, repo), "governance/owners.yaml")
			if err == nil {
				fact := builtin.ResolveResourceOwner(context.Background(), client,
					resourceOwnerQuery("kafka-topic:payments.settled.v2")).Facts[builtin.OutputOwner]
				t.Fatalf("OWNERSHIP ESCAPE: symlinked registry loaded; owner=%#v state=%q", fact.Value, fact.State)
			}
			if !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("error %q must name the symlink refusal", err)
			}
		})
	}
}

// TestResourceOwnerRegistryRealFileStillLoads — other polarity: a plain in-tree
// registry keeps loading unchanged under both filesystem flavours.
func TestResourceOwnerRegistryRealFileStillLoads(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "governance"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "governance", "owners.yaml"),
		[]byte("owners:\n  kafka-topic:payments.settled.v2: team-payments\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, flavour := range []string{"dirfs", "rootfs"} {
		t.Run(flavour, func(t *testing.T) {
			client, err := builtin.LoadResourceOwnerMap(openFlavour(t, flavour, tmp), "governance/owners.yaml")
			if err != nil {
				t.Fatalf("legitimate registry must still load: %v", err)
			}
			fact := builtin.ResolveResourceOwner(context.Background(), client,
				resourceOwnerQuery("kafka-topic:payments.settled.v2")).Facts[builtin.OutputOwner]
			if fact.State != provider.StateResolved || fact.Value != "team-payments" {
				t.Fatalf("owner fact = %q/%#v, want resolved/team-payments", fact.State, fact.Value)
			}
		})
	}
}
