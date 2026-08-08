package provider_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/provider"
)

// Paths to isolation harness binaries, built once in TestMain.
var maliciousExecBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "provider-e5-s03")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	maliciousExecBin = filepath.Join(tmp, "maliciousexec")
	// #nosec G204 -- test fixture build with fixed package path
	cmd := exec.Command("go", "build", "-o", maliciousExecBin, "./testdata/maliciousexec")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "building maliciousexec: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func maliciousDigest(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(maliciousExecBin) //nolint:gosec // TestMain-built binary under testdata/; not remote input.
	if err != nil {
		t.Fatalf("read malicious bin: %v", err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// redactDumpValues masks the value half of every NAME=VALUE line in a child
// process dump so a failure message can never carry credential material into a
// CI log. Assertions always run against the raw dump — only the printed form
// changes — so this cannot weaken a check. It matters because the dump is
// printed precisely when the child received something unexpected, which is also
// the scenario in which a regressed scrubber would have handed it the host
// environment.
func redactDumpValues(dump string) string {
	lines := strings.Split(dump, "\n")
	for i, line := range lines {
		// name == "" is the dump's own `=== SECTION ===` banner, not an
		// assignment — keep those so the redacted dump is still readable.
		if name, _, ok := strings.Cut(line, "="); ok && name != "" {
			lines[i] = name + "=<redacted>"
		}
	}
	return strings.Join(lines, "\n")
}

func isolationQuery() provider.FactQuery {
	return provider.FactQuery{
		APIVersion: provider.APIVersion,
		Kind:       provider.KindFactQuery,
		QueryID:    "q-isolation-1",
		AsOf:       fixedAsOf,
		Subject:    provider.Subject{Kind: "user", ID: "alice"},
		Outputs:    []string{"groups"},
	}
}

// TestIsolationNoWriteToken — REQ-E5-S03-01: exec child env is scrubbed from
// scratch; forge write-token canaries and TOKEN|SECRET names never appear.
func TestIsolationNoWriteToken(t *testing.T) {
	const forgeToken = "assent-test-canary-not-a-forge-token" // #nosec G101 -- deliberate canary
	t.Setenv("ASSENT_FORGE_TOKEN", forgeToken)
	t.Setenv("CI_JOB_SECRET", "job-secret-canary")

	configuredEnv := []string{
		"PROVIDER_MODE=spike",
		"UPSTREAM_TOKEN=configured-token-canary",
		"LDAP_SECRET=configured-secret-canary",
	}

	q := isolationQuery()
	raw, err := provider.CallExec(t.Context(), provider.ExecOpts{
		Binary:  maliciousExecBin,
		Digest:  maliciousDigest(t),
		Env:     configuredEnv,
		Timeout: execTestTimeout,
	}, q)
	if err != nil {
		// Name the deadline: `signal: killed` on its own reads like a crash.
		t.Fatalf("malicious provider run (timeout %s): %v", execTestTimeout, err)
	}
	dump := string(raw)
	if !strings.Contains(dump, "PROVIDER_MODE=spike") {
		t.Fatalf("sanity: declared non-secret env did not reach the provider; dump:\n%s", redactDumpValues(dump))
	}
	if !strings.Contains(dump, q.QueryID) {
		t.Fatalf("sanity: stdin did not reach the provider; dump:\n%s", redactDumpValues(dump))
	}

	for _, leaked := range []string{
		forgeToken,
		"job-secret-canary",
		"configured-token-canary",
		"configured-secret-canary",
		"ASSENT_FORGE_TOKEN",
		"UPSTREAM_TOKEN",
		"LDAP_SECRET",
		"CI_JOB_SECRET",
	} {
		if strings.Contains(dump, leaked) {
			t.Errorf("provider dump leaked %q", leaked)
		}
	}
	for _, line := range strings.Split(dump, "\n") {
		if !strings.Contains(line, "=") {
			continue
		}
		// Only inspect the ENV DUMP section for credential-looking names.
		upper := strings.ToUpper(line)
		name, _, ok := strings.Cut(upper, "=")
		if !ok {
			continue
		}
		if strings.Contains(name, "TOKEN") || strings.Contains(name, "SECRET") {
			// Name only, never the value: this branch fires exactly when the
			// scrubber has regressed, i.e. exactly when `line` holds a real host
			// credential. Reporting the value would copy it into the CI log.
			shown, _, _ := strings.Cut(line, "=")
			t.Errorf("credential-looking variable reached the provider: %s", shown)
		}
	}
}

// TestIsolationNoCredentialInArgv — REQ-E5-S03-03: adversarial canaries must
// not appear in the spawned process argv (argv hygiene, Spike C residual).
func TestIsolationNoCredentialInArgv(t *testing.T) {
	const forgeToken = "assent-test-canary-not-a-forge-token" // #nosec G101 -- deliberate canary
	t.Setenv("ASSENT_FORGE_TOKEN", forgeToken)

	configuredArgs := []string{
		"--mode=spike",
		"--token=" + forgeToken,
		"UPSTREAM_TOKEN=" + forgeToken,
		"LDAP_SECRET=configured-secret-canary",
		forgeToken, // bare canary value
	}

	q := isolationQuery()
	raw, err := provider.CallExec(t.Context(), provider.ExecOpts{
		Binary:  maliciousExecBin,
		Digest:  maliciousDigest(t),
		Args:    configuredArgs,
		Timeout: execTestTimeout,
	}, q)
	if err != nil {
		// Name the deadline: `signal: killed` on its own reads like a crash.
		t.Fatalf("malicious provider run (timeout %s): %v", execTestTimeout, err)
	}
	dump := string(raw)

	// Sanity: a non-credential operator flag may reach argv.
	if !strings.Contains(dump, "--mode=spike") {
		t.Fatalf("sanity: non-credential argv did not reach the provider; dump:\n%s", redactDumpValues(dump))
	}

	for _, leaked := range []string{
		forgeToken,
		"configured-secret-canary",
		"--token=",
		"UPSTREAM_TOKEN=",
		"LDAP_SECRET=",
	} {
		if strings.Contains(dump, leaked) {
			t.Errorf("provider argv/env dump leaked credential material %q", leaked)
		}
	}
}
