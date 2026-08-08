package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/hash"
)

// mergePolicySchemaID is the MergePolicy `$id`. It is spelled out here only so the
// test can compute the digest the code MUST NOT emit.
const mergePolicySchemaID = "https://assent.dev/schemas/policy/v1alpha1/merge-policy.schema.json"

// REQ-AUD-S16-02 (guard · byte artifacts stay raw) — D-121 byte-vs-document split.
//
// AUD-S16 moved compare.ReplayBundleDigest onto the domain-separated `assent-jcs-v1`
// digest because a ReplayBundle is a schema-owned DOCUMENT. `pins.policySha` is the
// other half of the split: it is a digest over the BYTE artifact loaded from the
// target ref, where byte identity is the whole point — two policy files that
// canonicalize identically but differ in bytes are DIFFERENT policies, and the
// released contract is `sha256:` + sha256(raw policy bytes).
//
// This test exists to fail if a future change ever "helpfully" canonicalizes the
// policy before hashing it.
func TestPolicyShaStaysRawBytes(t *testing.T) {
	f := newFakeGitLab(t)
	f.mergePolicy = mergePolicyChallenge
	f.baseFile = "partitions: 12\n"
	f.headFile = "partitions: 24\n" // governed grow + --arm => APPROVE, record emitted
	raw := []byte(f.mergePolicy)

	// Premise guard: this fixture can only discriminate raw bytes from canonical
	// JSON if the two actually differ. Without this, the test could pass vacuously.
	canon, err := hash.Canonicalize(raw)
	if err != nil {
		t.Fatalf("Canonicalize(policy): %v — the fixture must be JSON for this test to discriminate", err)
	}
	if bytes.Equal(canon, raw) {
		t.Fatal("fixture cannot detect canonicalization: raw policy bytes already equal their canonical form")
	}

	var out bytes.Buffer
	if code := runRun(runArgs("--arm"), env("tok"), fixedClock(), &out, &out, f.factory()); code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	got := out.String()

	// Positive: policySha is the raw-byte digest, tag included.
	wantRaw := `"policySha":"` + sha256Prefix + sha256Hex(raw) + `"`
	if !strings.Contains(got, wantRaw) {
		t.Fatalf("policySha must be sha256(raw policy bytes); missing %s in:\n%s", wantRaw, got)
	}

	// Negative 1: it must NOT be sha256 over the canonicalized document.
	if canonSha := sha256Hex(canon); strings.Contains(got, canonSha) {
		t.Fatalf("policySha digests CANONICALIZED policy bytes (%s) — D-121 keeps byte artifacts raw:\n%s", canonSha, got)
	}

	// Negative 2: it must NOT be the domain-separated assent-jcs-v1 digest either.
	domained, err := hash.Digest(mergePolicySchemaID, raw)
	if err != nil {
		t.Fatalf("hash.Digest(mergePolicy): %v", err)
	}
	if strings.Contains(got, domained) {
		t.Fatalf("policySha is the domain-separated document digest (%s) — D-121 assigns that to "+
			"schema-owned documents (ReplayBundle), not to the policy BYTE artifact:\n%s", domained, got)
	}
}
