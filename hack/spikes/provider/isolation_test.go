package provider

import (
	"strings"
	"testing"
)

// REQ-P2-E3-S02-01: the harness holds ASSENT_FORGE_TOKEN; a deliberately
// malicious exec provider dumps its entire environment and stdin. The dump
// must contain neither the token value nor any passed-through variable whose
// name matches *TOKEN*/*SECRET* — the exec.Cmd environment is scrubbed, never
// inherited.
func TestIsolation(t *testing.T) {
	const forgeToken = "assent-test-canary-not-a-forge-token" // #nosec G101 -- deliberate canary, not a real credential
	t.Setenv("ASSENT_FORGE_TOKEN", forgeToken)
	t.Setenv("CI_JOB_SECRET", "job-secret-canary")

	// An operator explicitly configures env for the provider — credential-
	// looking names must still be refused by the scrubber.
	configuredEnv := []string{
		"PROVIDER_MODE=spike",
		"UPSTREAM_TOKEN=configured-token-canary",
		"LDAP_SECRET=configured-secret-canary",
	}

	q := groupQuery(t)
	raw, err := CallExec(t.Context(), maliciousExecBin, configuredEnv, q, execTestTimeout)
	if err != nil {
		// Name the deadline: `signal: killed` on its own reads like a crash.
		t.Fatalf("malicious provider run (timeout %s): %v", execTestTimeout, err)
	}
	dump := string(raw)
	if !strings.Contains(dump, "PROVIDER_MODE=spike") {
		t.Fatalf("sanity: declared non-secret env did not reach the provider; dump:\n%s", dump)
	}
	if !strings.Contains(dump, q.QueryID) {
		t.Fatalf("sanity: stdin did not reach the provider; dump:\n%s", dump)
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
		upper := strings.ToUpper(line)
		if name, _, ok := strings.Cut(upper, "="); ok &&
			(strings.Contains(name, "TOKEN") || strings.Contains(name, "SECRET")) {
			t.Errorf("credential-looking variable reached the provider: %s", line)
		}
	}
}
