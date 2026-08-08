package provider_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
)

// execTestTimeout is the ExecOpts.Timeout every test in this package uses when
// it spawns the real child provider binary.
//
// It is deliberately generous, and it must stay that way. Nothing in
// TestExecDigestPin, TestIsolationNoWriteToken or TestIsolationNoCredentialInArgv
// asserts timeout behaviour — they pin digest verification and argv/env
// scrubbing, for which the deadline is pure incidental plumbing. The dedicated
// timeout assertion lives in TestTransportTimeoutUnavailable, which blocks its
// server until released and so cannot be made wrong by a slow machine.
//
// The old value was 1s (5s in the isolation tests). Measured under
// `go test -race ./...` with 36 competing CPU burners, one spawn of a
// freshly-built child binary takes 1.3s-5.3s wall clock (~200ms unloaded): the
// first exec of a just-written file pays page-in plus macOS code-signature
// validation while 18 sibling package binaries compile and run. Those deadlines
// therefore fired on a healthy child and surfaced as `signal: killed` — a flake
// that reproduced 6/6 under load and cost four agents a session to diagnose.
//
// 60s is ~11x the worst measured spawn and costs nothing when things are
// healthy (the call returns in milliseconds). Do not "optimise" it back down:
// a short deadline here buys no coverage and only re-arms the flake.
const execTestTimeout = 60 * time.Second

// TestExecDigestPin — REQ-E5-S03-02: missing pin or digest mismatch refuses
// exec; facts land unavailable (never resolved). Matching pin allows the call.
func TestExecDigestPin(t *testing.T) {
	q := isolationQuery()
	pin := maliciousDigest(t)

	t.Run("missing_pin_refuses", func(t *testing.T) {
		_, err := provider.CallExec(t.Context(), provider.ExecOpts{
			Binary:  maliciousExecBin,
			Digest:  "",
			Timeout: execTestTimeout,
		}, q)
		if err == nil {
			t.Fatal("CallExec with missing digest pin must refuse")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "digest") {
			t.Fatalf("error should mention digest pin; got: %v", err)
		}

		call := func(ctx context.Context) ([]byte, error) {
			return provider.CallExec(ctx, provider.ExecOpts{
				Binary:  maliciousExecBin,
				Digest:  "",
				Timeout: execTestTimeout,
			}, q)
		}
		got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
		f := got.Facts["groups"]
		if f.State != provider.StateUnavailable {
			t.Fatalf("missing pin: state = %q, want unavailable", f.State)
		}
		if f.State == provider.StateResolved {
			t.Fatal("missing pin must never resolve")
		}
	})

	t.Run("mismatch_refuses", func(t *testing.T) {
		_, err := provider.CallExec(t.Context(), provider.ExecOpts{
			Binary:  maliciousExecBin,
			Digest:  "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			Timeout: execTestTimeout,
		}, q)
		if err == nil {
			t.Fatal("CallExec with mismatched digest pin must refuse")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "digest") {
			t.Fatalf("error should mention digest; got: %v", err)
		}

		call := func(ctx context.Context) ([]byte, error) {
			return provider.CallExec(ctx, provider.ExecOpts{
				Binary:  maliciousExecBin,
				Digest:  "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
				Timeout: execTestTimeout,
			}, q)
		}
		got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
		f := got.Facts["groups"]
		if f.State != provider.StateUnavailable {
			t.Fatalf("mismatch: state = %q, want unavailable", f.State)
		}
	})

	t.Run("match_allows_exec", func(t *testing.T) {
		raw, err := provider.CallExec(t.Context(), provider.ExecOpts{
			Binary:  maliciousExecBin,
			Digest:  pin,
			Timeout: execTestTimeout,
		}, q)
		if err != nil {
			// Name the deadline: `signal: killed` on its own reads like a crash.
			t.Fatalf("matching pin must allow exec (timeout %s): %v", execTestTimeout, err)
		}
		if !strings.Contains(string(raw), "=== ARGV DUMP ===") {
			t.Fatalf("expected maliciousexec dump; got: %s", raw)
		}
	})

	t.Run("pin_via_host_config_field", func(t *testing.T) {
		// D-065: digest lives on the internal host declaration, not config.schema.json.
		cfg, err := provider.LoadProviderConfig([]byte(`{
			"name": "exec-toy",
			"requests": {"values": {"pointers": []}},
			"exec": {"binary": "unused-in-this-assert", "digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			"outputs": {
				"groups": {
					"type": "string",
					"cardinality": "set",
					"subject": "user",
					"sensitive": false,
					"maxAge": "1h"
				}
			}
		}`))
		if err != nil {
			t.Fatalf("host config with exec digest must load: %v", err)
		}
		if cfg.Exec == nil || cfg.Exec.Digest == "" {
			t.Fatal("Config.Exec.Digest must be populated from internal declaration")
		}
		if cfg.Exec.Digest != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
			t.Fatalf("digest = %q", cfg.Exec.Digest)
		}
	})

	t.Run("wrong_binary_path_still_checks_digest", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "no-such-provider")
		_, err := provider.CallExec(t.Context(), provider.ExecOpts{
			Binary:  missing,
			Digest:  pin,
			Timeout: execTestTimeout,
		}, q)
		if err == nil {
			t.Fatal("missing binary must refuse")
		}
	})
}
