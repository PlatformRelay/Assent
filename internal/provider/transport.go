package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// secretName flags environment variable / argv names that must never pass
// through to a provider process, even when explicitly configured (ADR-0015 §7).
var secretName = regexp.MustCompile(`(?i)(TOKEN|SECRET)`)

// ExecOpts configures one digest-pinned exec provider invocation.
// Digest is required (D-065 / REQ-E5-S03-02); missing or mismatched pin refuses
// before the child is spawned. Env/Args are scrubbed — the host environment is
// never inherited.
type ExecOpts struct {
	Binary  string
	Digest  string // sha256:<hex>; required
	Env     []string
	Args    []string
	Timeout time.Duration
}

// ScrubEnv builds a provider environment from scratch: the host process
// environment (which holds the forge write token) is never inherited. Only
// PATH plus explicitly configured, non-credential-looking entries survive.
func ScrubEnv(configured []string) []string {
	env := []string{"PATH=" + os.Getenv("PATH")}
	for _, kv := range configured {
		name, _, _ := strings.Cut(kv, "=")
		if secretName.MatchString(name) {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// ScrubArgv drops credential-looking argv entries and any arg equal to a host
// env value whose name matches TOKEN|SECRET (forge write-token canaries).
// Non-credential operator flags pass through (REQ-E5-S03-03).
func ScrubArgv(configured []string) []string {
	secretValues := hostSecretValues()
	out := make([]string, 0, len(configured))
	for _, arg := range configured {
		name, val, hasEq := strings.Cut(arg, "=")
		checkName := strings.TrimLeft(name, "-")
		if secretName.MatchString(checkName) {
			continue
		}
		if secretValues[arg] {
			continue
		}
		if hasEq && secretValues[val] {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func hostSecretValues() map[string]bool {
	out := map[string]bool{}
	for _, kv := range os.Environ() {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || val == "" {
			continue
		}
		if secretName.MatchString(name) {
			out[val] = true
		}
	}
	return out
}

// CallHTTP posts the FactQuery to an HTTP provider and returns the raw body.
// Caller-enforced timeout via context; errors (incl. deadline) surface to
// ResolveFacts as unavailable (REQ-E5-S03-04).
func CallHTTP(ctx context.Context, url string, q FactQuery, timeout time.Duration) ([]byte, error) {
	body, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// CallExec runs an exec provider with the FactQuery on stdin, a scrubbed
// environment/argv, and a verified digest pin. Refuses before spawn when the
// pin is missing or does not match the binary bytes (REQ-E5-S03-02).
func CallExec(ctx context.Context, opts ExecOpts, q FactQuery) ([]byte, error) {
	if err := VerifyExecDigest(opts.Binary, opts.Digest); err != nil {
		return nil, err
	}
	body, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	args := ScrubArgv(opts.Args)
	// #nosec G204 -- operator-pinned provider binary; digest verified above.
	cmd := exec.CommandContext(ctx, opts.Binary, args...)
	cmd.Env = ScrubEnv(opts.Env)
	cmd.Stdin = bytes.NewReader(body)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return out.Bytes(), err
	}
	return out.Bytes(), nil
}

// FileDigestSHA256 returns the sha256:<hex> digest of the file at path.
func FileDigestSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("exec digest: read %q: %w", path, err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// VerifyExecDigest refuses when pin is empty or does not match the binary.
func VerifyExecDigest(path, pin string) error {
	if strings.TrimSpace(pin) == "" {
		return fmt.Errorf("exec digest pin missing — refuse to spawn provider %q", path)
	}
	got, err := FileDigestSHA256(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, pin) {
		return fmt.Errorf("exec digest mismatch for %q: pin %s != actual %s", path, pin, got)
	}
	return nil
}
