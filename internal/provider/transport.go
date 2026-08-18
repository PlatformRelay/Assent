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

// MaxResponseBytes bounds how much of an HTTP provider response is read
// (AUD-S10, audit findings REL-03 / SEC-08). A provider is off-box and only as
// trustworthy as the operator who declared it; an unbounded io.ReadAll lets a
// hostile or broken endpoint exhaust the runner's memory.
//
// 8 MiB is MB-order and generously above legitimate traffic: a FactSet is a
// handful of scalar/array fact values — KB-order in every shipped example and
// in the conformance corpus — so the bound is invisible to real providers.
//
// FAIL-CLOSED: exceeding the bound is an ERROR, never a truncated read. A
// truncated prefix that happened to parse would become a "resolved" fact
// derived from half a document; ResolveFacts instead classifies the error as
// unavailable, which keeps auto-merge disarmed.
const MaxResponseBytes = 8 << 20

// readBounded reads at most limit bytes from r and errors if the source has
// MORE than limit bytes to give. It reads limit+1 bytes precisely so that
// "exactly at the limit" stays legitimate traffic while limit+1 is refused; the
// partially-read bytes are DISCARDED (nil is returned with the error) so no
// caller can accidentally parse a truncated document.
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("response body exceeds the %d-byte limit — refusing to parse a truncated response", limit)
	}
	return raw, nil
}

// CallHTTP posts the FactQuery to an HTTP provider and returns the raw body.
// Caller-enforced timeout via context; errors (incl. deadline) surface to
// ResolveFacts as unavailable (REQ-E5-S03-04). The response read is BOUNDED at
// MaxResponseBytes (AUD-S10 / REL-03 / SEC-08) — an over-limit body is an
// error, never a truncated parse.
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
	return readBounded(resp.Body, MaxResponseBytes)
}

// maxStderrExcerptBytes bounds how much of an exec provider's stderr is kept to
// explain a failure (REL-07). It is an ERROR-LEGIBILITY bound, not a response
// bound: MaxResponseBytes above remains the single declared bound on the bytes a
// provider may answer with, and stderr never becomes an answer (see CallExec).
// The excerpt exists so an operator debugging a fail-closed REVIEW reads the
// provider's own diagnostic instead of a bare "exit status 1"; 4 KiB is a few
// dozen lines, past which more text stops helping and starts flooding CI logs.
const maxStderrExcerptBytes = 4 << 10

// boundedCapture is an io.Writer that accumulates a child process stream in
// memory under a HARD cap, and then applies the readBounded verdict to what it
// kept (AUD2-S01, finding REL-01). It exists because a plain bytes.Buffer as
// cmd.Stdout is unbounded: opts.Timeout bounds wall clock, not memory, so a
// runaway provider could exhaust the runner before any deadline fired.
//
// The cap and the verdict are deliberately the same object: removing the exec
// bound means deleting this type's use, not silently loosening one half of it
// while the other still looks correct.
type boundedCapture struct {
	limit int64 // bytes ALLOWED through; limit+1 is refused
	buf   bytes.Buffer
}

func newBoundedCapture(limit int64) *boundedCapture { return &boundedCapture{limit: limit} }

// Write keeps at most limit+1 bytes — exactly what readBounded needs to tell
// "at the limit" (legitimate) from "over the limit" (refused) — and DISCARDS
// the rest. It reports a full write for the discarded remainder on purpose: an
// io.ErrShortWrite here would abort os/exec's copier and surface as a confusing
// I/O error instead of the limit error the caller must see.
func (c *boundedCapture) Write(p []byte) (int, error) {
	n := len(p)
	if room := c.limit + 1 - int64(c.buf.Len()); room > 0 {
		if int64(n) > room {
			p = p[:room]
		}
		c.buf.Write(p) // bytes.Buffer.Write never returns an error
	}
	return n, nil
}

// overflowed reports whether the stream had more bytes to give than the cap.
func (c *boundedCapture) overflowed() bool { return int64(c.buf.Len()) > c.limit }

// bytesOrError applies the shared bound semantics: at-limit is returned intact,
// over-limit is an error with NO bytes so nothing can parse a truncated
// document. Identical treatment to CallHTTP's response read, by construction.
func (c *boundedCapture) bytesOrError() ([]byte, error) {
	return readBounded(bytes.NewReader(c.buf.Bytes()), c.limit)
}

// excerpt renders the captured stream for an error message, marking truncation
// so a reader never mistakes a cut-off diagnostic for the whole story.
func (c *boundedCapture) excerpt() string {
	raw := c.buf.Bytes()
	truncated := c.overflowed()
	if truncated {
		raw = raw[:c.limit]
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ""
	}
	if truncated {
		text += " …(truncated)"
	}
	return text
}

// CallExec runs an exec provider with the FactQuery on stdin, a scrubbed
// environment/argv, and a verified digest pin. Refuses before spawn when the
// pin is missing or does not match the binary bytes (REQ-E5-S03-02).
//
// Three containment properties, all fail-closed (AUD2-S01):
//   - stdout is BOUNDED at MaxResponseBytes exactly as CallHTTP's body read is
//     (REL-01) — an over-limit provider yields an error and NO bytes, never a
//     truncated parse, and cannot grow the runner's heap without limit;
//   - cmd.WaitDelay is the operator's own timeout (REL-02), so a provider that
//     forks a background grandchild inheriting stdout cannot hold cmd.Run open
//     past ~2x the deadline; the resulting exec.ErrWaitDelay is an error like
//     any other and classifies as unavailable;
//   - stderr is captured into its OWN bounded buffer and folded into the
//     returned error (REL-07), so a failure explains itself. It is NEVER
//     concatenated into the returned bytes: ResolveFacts parses stdout as the
//     provider's answer, and mixing the streams would let a chatty provider
//     corrupt a decision input.
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
	stdout := newBoundedCapture(MaxResponseBytes)
	stderr := newBoundedCapture(maxStderrExcerptBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// The single operator-declared timeout is also the wait bound: killing the
	// child does not close a pipe its grandchildren still hold, so without this
	// Wait blocks indefinitely (REL-02).
	cmd.WaitDelay = opts.Timeout
	if err := cmd.Run(); err != nil {
		return nil, execFailure(err, stderr)
	}
	return stdout.bytesOrError()
}

// execFailure folds the provider's own stderr diagnostic into the run error,
// preserving the wrapped sentinel (exec.ErrWaitDelay, *exec.ExitError, …) so
// callers can still discriminate with errors.Is/As.
func execFailure(err error, stderr *boundedCapture) error {
	excerpt := stderr.excerpt()
	if excerpt == "" {
		return err
	}
	return fmt.Errorf("%w: provider stderr: %s", err, excerpt)
}

// FileDigestSHA256 returns the sha256:<hex> digest of the file at path.
func FileDigestSHA256(path string) (string, error) {
	// #nosec G304 -- path is an operator-declared digest-pinned exec binary from
	// host Config.Exec (D-065), not remote/attacker input; CallExec refuses on
	// digest mismatch before spawn.
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
