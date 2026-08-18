package provider_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
)

// TestTransportTimeoutUnavailable — REQ-E5-S03-04: HTTP transport timeout
// classifies as unavailable, never resolved.
func TestTransportTimeoutUnavailable(t *testing.T) {
	q := isolationQuery()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer func() {
		close(release)
		srv.Close()
	}()

	call := func(ctx context.Context) ([]byte, error) {
		return provider.CallHTTP(ctx, srv.URL, q, 50*time.Millisecond)
	}
	got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
	f, ok := got.Facts["groups"]
	if !ok {
		t.Fatal("fact key silently absent — must never happen")
	}
	if f.State != provider.StateUnavailable {
		t.Fatalf("timeout state = %q, want unavailable (reason: %q)", f.State, f.Reason)
	}
	if f.State == provider.StateResolved || f.Value != nil {
		t.Fatal("transport timeout must never resolve or carry a value")
	}
	if got.AutoMergeEligible() {
		t.Fatal("timeout path must keep auto-merge disarmed")
	}
}

// TestBoundedReadOverLimitFailsClosed — AUD-S10 / REQ-AUD-S10-01 (findings
// REL-03, SEC-08). A provider response larger than provider.MaxResponseBytes is
// an ERROR, not a truncated parse: a hostile or broken provider must not be
// able to OOM the run, and the prefix it did send must never be mistaken for a
// complete FactSet. Driven through ResolveFacts (the production entry point) so
// an unwired limit fails the test.
func TestBoundedReadOverLimitFailsClosed(t *testing.T) {
	q := isolationQuery()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Syntactically OPEN JSON: a truncating read would still be handed to
		// the decoder, so only the byte bound can produce the limit error.
		_, _ = io.WriteString(w, `{"facts":{"groups":{"state":"resolved","value":"`)
		const chunk = 64 << 10
		filler := strings.Repeat("x", chunk)
		for n := provider.MaxResponseBytes + 1; n > 0; n -= chunk {
			if n < chunk {
				_, _ = io.WriteString(w, filler[:n])
				break
			}
			_, _ = io.WriteString(w, filler)
		}
		_, _ = io.WriteString(w, `"}}}`)
	}))
	defer srv.Close()

	raw, err := provider.CallHTTP(t.Context(), srv.URL, q, 30*time.Second)
	if err == nil {
		t.Fatal("over-limit provider body must fail closed, got nil error")
	}
	if raw != nil {
		t.Fatalf("over-limit read must return no bytes, got %d", len(raw))
	}
	if !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("error must name the byte limit, got %v", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprint(provider.MaxResponseBytes)) {
		t.Fatalf("error must state the limit %d, got %v", provider.MaxResponseBytes, err)
	}

	call := func(ctx context.Context) ([]byte, error) {
		return provider.CallHTTP(ctx, srv.URL, q, 30*time.Second)
	}
	got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
	f, ok := got.Facts["groups"]
	if !ok {
		t.Fatal("fact key silently absent — must never happen")
	}
	if f.State != provider.StateUnavailable {
		t.Fatalf("over-limit state = %q, want unavailable (reason: %q)", f.State, f.Reason)
	}
	if f.Value != nil {
		t.Fatal("an over-limit body must never carry a value")
	}
	if got.AutoMergeEligible() {
		t.Fatal("over-limit path must keep auto-merge disarmed")
	}
}

// TestBoundedReadAtLimitStillSucceeds is the BOUNDARY positive control (review
// finding F2). An under-limit control alone does not pin WHERE the boundary
// sits: flipping `len(raw) > limit` to `>=` shifts it by one byte and an
// under-limit test never notices. A body of EXACTLY MaxResponseBytes is
// legitimate traffic and must parse; only this case kills that mutant.
func TestBoundedReadAtLimitStillSucceeds(t *testing.T) {
	q := isolationQuery()
	// A REAL, schema-valid FactResponse padded to exactly the limit with
	// INSIGNIFICANT TRAILING WHITESPACE — not a giant value. That keeps the
	// document decodable at the boundary, so the test can assert the at-limit
	// body both READS and RESOLVES.
	doc := resolvedResponseBytes(t, q, fixedAsOf.Add(time.Hour))
	pad := provider.MaxResponseBytes - len(doc)
	if pad <= 0 {
		t.Fatalf("MaxResponseBytes %d is too small for this fixture", provider.MaxResponseBytes)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(doc)
		const chunk = 64 << 10
		filler := strings.Repeat(" ", chunk)
		for n := pad; n > 0; n -= chunk {
			if n < chunk {
				_, _ = io.WriteString(w, filler[:n])
				break
			}
			_, _ = io.WriteString(w, filler)
		}
	}))
	defer srv.Close()

	raw, err := provider.CallHTTP(t.Context(), srv.URL, q, 30*time.Second)
	if err != nil {
		t.Fatalf("a body of exactly the limit is legitimate traffic: %v", err)
	}
	if len(raw) != provider.MaxResponseBytes {
		t.Fatalf("read %d bytes, want exactly the limit %d", len(raw), provider.MaxResponseBytes)
	}

	// Drive the production entry point too: the at-limit body must RESOLVE, not
	// merely be read — a bound that errors here would disarm auto-merge.
	call := func(ctx context.Context) ([]byte, error) {
		return provider.CallHTTP(ctx, srv.URL, q, 30*time.Second)
	}
	got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
	f, ok := got.Facts["groups"]
	if !ok {
		t.Fatal("fact key silently absent — must never happen")
	}
	if f.State != provider.StateResolved {
		t.Fatalf("at-limit state = %q, want resolved (reason: %q)", f.State, f.Reason)
	}
}

// TestBoundedReadUnderLimitUnaffected is the POSITIVE CONTROL for the bound: a
// legitimate (small) provider payload still resolves. Without it, a limit of
// zero would pass TestBoundedReadOverLimitFailsClosed.
func TestBoundedReadUnderLimitUnaffected(t *testing.T) {
	q := isolationQuery()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"facts":{"groups":{"state":"resolved","value":["a"]}}}`)
	}))
	defer srv.Close()

	raw, err := provider.CallHTTP(t.Context(), srv.URL, q, 30*time.Second)
	if err != nil {
		t.Fatalf("a legitimate payload must be unaffected by the bound: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("legitimate payload read returned no bytes")
	}
	if provider.MaxResponseBytes < 1<<20 {
		t.Fatalf("MaxResponseBytes = %d — the bound must stay MB-order, generously above "+
			"legitimate provider payloads", provider.MaxResponseBytes)
	}
}

// --- AUD2-S01: exec transport trio (REL-01 bound stdout / REL-02 WaitDelay /
// --- REL-07 stderr capture). The three exec bound tests below deliberately
// --- mirror the three HTTP ones above: an exec provider and an HTTP provider
// --- answer the same FactQuery and feed the same resolver, so a containment
// --- asymmetry between them is the finding, not a design choice.

// execStub writes `script` as an executable stub provider under t.TempDir() and
// returns ExecOpts pinned to its real digest (CallExec refuses to spawn an
// unpinned binary, REQ-E5-S03-02). A shell script is the provider "binary":
// nothing in these tests needs Go, and building four child binaries would cost
// more than the behaviour under test.
func execStub(t *testing.T, script string, timeout time.Duration) provider.ExecOpts {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "stub-provider")
	if err := os.WriteFile(bin, []byte(script), 0o600); err != nil {
		t.Fatalf("write stub provider: %v", err)
	}
	// #nosec G302 -- a stub provider under t.TempDir() must be executable to be
	// spawned at all; it is created by this test, not by an untrusted party.
	if err := os.Chmod(bin, 0o700); err != nil {
		t.Fatalf("chmod stub provider: %v", err)
	}
	digest, err := provider.FileDigestSHA256(bin)
	if err != nil {
		t.Fatalf("digest stub provider: %v", err)
	}
	return provider.ExecOpts{Binary: bin, Digest: digest, Timeout: timeout}
}

// writeStubPayload writes `payload` next to the stub and returns a script that
// cats it verbatim — exact byte counts are produced in Go, where they can be
// computed, rather than in shell.
func writeStubPayload(t *testing.T, payload []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write stub payload: %v", err)
	}
	return "#!/bin/sh\nexec cat " + path + "\n"
}

// TestExecBoundedStdoutOverLimitFailsClosed — REQ-AUD2-S01-01 / -07 (finding
// REL-01, byte-identical across three audits). Child stdout larger than
// provider.MaxResponseBytes is an ERROR with NO bytes, exactly as for HTTP: a
// runaway exec provider must not OOM the runner, and the prefix it did write
// must never be mistaken for a complete FactSet. Driven through ResolveFacts
// too, so an unwired bound fails the test.
func TestExecBoundedStdoutOverLimitFailsClosed(t *testing.T) {
	q := isolationQuery()
	// Syntactically OPEN JSON: a truncating read would still be handed to the
	// decoder, so only the byte bound can produce the limit error.
	payload := []byte(`{"facts":{"groups":{"state":"resolved","value":"`)
	payload = append(payload, bytes.Repeat([]byte("x"), provider.MaxResponseBytes+1-len(payload))...)
	opts := execStub(t, writeStubPayload(t, payload), execTestTimeout)

	raw, err := provider.CallExec(t.Context(), opts, q)
	if err == nil {
		t.Fatal("over-limit exec provider stdout must fail closed, got nil error")
	}
	if raw != nil {
		t.Fatalf("over-limit exec read must return no bytes, got %d", len(raw))
	}
	if !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("error must name the byte limit, got %v", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprint(provider.MaxResponseBytes)) {
		t.Fatalf("error must state the limit %d, got %v", provider.MaxResponseBytes, err)
	}

	call := func(ctx context.Context) ([]byte, error) { return provider.CallExec(ctx, opts, q) }
	got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
	f, ok := got.Facts["groups"]
	if !ok {
		t.Fatal("fact key silently absent — must never happen")
	}
	if f.State != provider.StateUnavailable {
		t.Fatalf("over-limit exec state = %q, want unavailable (reason: %q)", f.State, f.Reason)
	}
	if f.Value != nil {
		t.Fatal("an over-limit exec response must never carry a value")
	}
	if got.AutoMergeEligible() {
		t.Fatal("over-limit exec path must keep auto-merge disarmed")
	}
}

// TestExecBoundedStdoutAtLimitStillSucceeds — REQ-AUD2-S01-02. The BOUNDARY
// positive control, mirroring the HTTP one: stdout of EXACTLY the limit is
// legitimate traffic and must both read and resolve. Only this case kills the
// `>` → `>=` mutant.
func TestExecBoundedStdoutAtLimitStillSucceeds(t *testing.T) {
	q := isolationQuery()
	doc := resolvedResponseBytes(t, q, fixedAsOf.Add(time.Hour))
	pad := provider.MaxResponseBytes - len(doc)
	if pad <= 0 {
		t.Fatalf("MaxResponseBytes %d is too small for this fixture", provider.MaxResponseBytes)
	}
	// Pad with INSIGNIFICANT TRAILING WHITESPACE so the document stays decodable
	// at the boundary and the at-limit response can be asserted to RESOLVE.
	payload := append(doc, bytes.Repeat([]byte(" "), pad)...)
	opts := execStub(t, writeStubPayload(t, payload), execTestTimeout)

	raw, err := provider.CallExec(t.Context(), opts, q)
	if err != nil {
		t.Fatalf("stdout of exactly the limit is legitimate traffic: %v", err)
	}
	if len(raw) != provider.MaxResponseBytes {
		t.Fatalf("read %d bytes, want exactly the limit %d", len(raw), provider.MaxResponseBytes)
	}

	call := func(ctx context.Context) ([]byte, error) { return provider.CallExec(ctx, opts, q) }
	got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
	f, ok := got.Facts["groups"]
	if !ok {
		t.Fatal("fact key silently absent — must never happen")
	}
	if f.State != provider.StateResolved {
		t.Fatalf("at-limit exec state = %q, want resolved (reason: %q)", f.State, f.Reason)
	}
}

// TestExecBoundedStdoutUnderLimitUnaffected is the POSITIVE CONTROL for the
// exec bound: without it, a limit of zero would pass the over-limit test.
func TestExecBoundedStdoutUnderLimitUnaffected(t *testing.T) {
	q := isolationQuery()
	opts := execStub(t, writeStubPayload(t, resolvedResponseBytes(t, q, fixedAsOf.Add(time.Hour))), execTestTimeout)

	raw, err := provider.CallExec(t.Context(), opts, q)
	if err != nil {
		t.Fatalf("a legitimate exec payload must be unaffected by the bound: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("legitimate exec payload read returned no bytes")
	}
	if provider.MaxResponseBytes < 1<<20 {
		t.Fatalf("MaxResponseBytes = %d — the bound must stay MB-order, generously above "+
			"legitimate provider payloads", provider.MaxResponseBytes)
	}
}

// TestExecWaitDelayBoundsOrphanedStdout — REQ-AUD2-S01-04 / -05 / -07 (finding
// REL-02). A provider that forks a background grandchild inheriting stdout and
// then exits leaves the stdout pipe open: without cmd.WaitDelay, cmd.Run blocks
// until every writer closes it — here 60 seconds, long past the operator's
// declared deadline, with no decision and no diagnostic.
//
// The assertion is wall-clock: the call must return in ~2x opts.Timeout, an
// order of magnitude below the grandchild's lifetime. Removing the WaitDelay
// assignment makes this test wait the full 60s and fail on the elapsed bound
// (it does not merely rely on `go test -timeout`).
func TestExecWaitDelayBoundsOrphanedStdout(t *testing.T) {
	const (
		timeout     = 2 * time.Second
		grandkid    = 60 * time.Second // how long the orphan holds the pipe
		mustBeUnder = 30 * time.Second
	)
	q := isolationQuery()
	opts := execStub(t, "#!/bin/sh\nsleep 60 &\nprintf 'partial'\nexit 0\n", timeout)

	start := time.Now()
	raw, err := provider.CallExec(t.Context(), opts, q)
	elapsed := time.Since(start)

	if elapsed >= mustBeUnder {
		t.Fatalf("CallExec took %s — a grandchild holding stdout for %s blocked the call past "+
			"its %s deadline (cmd.WaitDelay unset?)", elapsed, grandkid, timeout)
	}
	if err == nil {
		t.Fatalf("a provider killed by the deadline/wait-delay must fail closed, got nil error (raw %q)", raw)
	}
	if raw != nil {
		t.Fatalf("a wait-delay-killed provider must return no bytes, got %d", len(raw))
	}
	// The exact sentinel depends on which timer fired first (process exit vs the
	// context deadline) and a loaded machine can reorder them, so accept either
	// documented shape — but never a success.
	if !errors.Is(err, exec.ErrWaitDelay) && !errors.Is(err, context.DeadlineExceeded) &&
		!strings.Contains(err.Error(), "killed") {
		t.Fatalf("unexpected error shape for a wait-delay/deadline kill: %v", err)
	}

	call := func(ctx context.Context) ([]byte, error) { return provider.CallExec(ctx, opts, q) }
	got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
	f, ok := got.Facts["groups"]
	if !ok {
		t.Fatal("fact key silently absent — must never happen")
	}
	if f.State != provider.StateUnavailable {
		t.Fatalf("wait-delay state = %q, want unavailable (reason: %q)", f.State, f.Reason)
	}
	if got.AutoMergeEligible() {
		t.Fatal("wait-delay path must keep auto-merge disarmed")
	}
}

// TestExecStderrFoldedIntoError — REQ-AUD2-S01-06 (finding REL-07). A failing
// provider used to yield a bare "exit status 1": nothing for the operator
// debugging a fail-closed REVIEW to read. Its stderr now reaches the error —
// and, per judgment call (c), NEVER the fact bytes: the resolver parses stdout
// as the provider's answer, so merging the streams would let a chatty provider
// corrupt a decision input.
func TestExecStderrFoldedIntoError(t *testing.T) {
	const canary = "upstream-ldap-unreachable-canary"
	q := isolationQuery()

	t.Run("nonzero_exit_reports_stderr", func(t *testing.T) {
		opts := execStub(t, "#!/bin/sh\necho '"+canary+"' >&2\nexit 7\n", execTestTimeout)
		raw, err := provider.CallExec(t.Context(), opts, q)
		if err == nil {
			t.Fatal("a provider exiting non-zero must fail closed")
		}
		if !strings.Contains(err.Error(), canary) {
			t.Fatalf("error must carry the provider's stderr diagnostic, got %v", err)
		}
		if !strings.Contains(err.Error(), "exit status 7") {
			t.Fatalf("error must still name the exit status, got %v", err)
		}
		if raw != nil {
			t.Fatalf("a failed provider call must return no bytes, got %q", raw)
		}
	})

	t.Run("stderr_never_merged_into_facts", func(t *testing.T) {
		// Exit 0 with a VALID response on stdout and noise on stderr: the noise
		// must not appear in the returned bytes (which would break decoding) and
		// the fact must still resolve. Without this case the "streams are not
		// merged" half of the requirement is vacuous, because the failure path
		// returns nil bytes anyway.
		doc := resolvedResponseBytes(t, q, fixedAsOf.Add(time.Hour))
		payload := filepath.Join(t.TempDir(), "resp.json")
		if err := os.WriteFile(payload, doc, 0o600); err != nil {
			t.Fatalf("write payload: %v", err)
		}
		opts := execStub(t, "#!/bin/sh\necho '"+canary+"' >&2\nexec cat "+payload+"\n", execTestTimeout)

		raw, err := provider.CallExec(t.Context(), opts, q)
		if err != nil {
			t.Fatalf("a chatty but successful provider must succeed: %v", err)
		}
		if strings.Contains(string(raw), canary) {
			t.Fatalf("stderr leaked into the fact bytes: %q", raw)
		}
		call := func(ctx context.Context) ([]byte, error) { return provider.CallExec(ctx, opts, q) }
		got := provider.ResolveFacts(t.Context(), call, q, fixedAsOf)
		if f := got.Facts["groups"]; f.State != provider.StateResolved {
			t.Fatalf("chatty provider state = %q, want resolved (reason: %q)", f.State, f.Reason)
		}
	})

	t.Run("runaway_stderr_is_bounded_and_truncated", func(t *testing.T) {
		// REL-01 must not be reopened through the back door: the stderr buffer is
		// bounded too, so a provider spewing megabytes of diagnostics can neither
		// exhaust memory nor produce an unreadable error.
		noise := filepath.Join(t.TempDir(), "noise")
		if err := os.WriteFile(noise, bytes.Repeat([]byte("N"), 4<<20), 0o600); err != nil {
			t.Fatalf("write noise: %v", err)
		}
		opts := execStub(t, "#!/bin/sh\ncat "+noise+" >&2\nexit 3\n", execTestTimeout)

		_, err := provider.CallExec(t.Context(), opts, q)
		if err == nil {
			t.Fatal("a provider exiting non-zero must fail closed")
		}
		if len(err.Error()) > 64<<10 {
			t.Fatalf("stderr capture is unbounded: error message is %d bytes", len(err.Error()))
		}
		if !strings.Contains(err.Error(), "truncated") {
			t.Fatalf("a truncated stderr excerpt must say so, got %d bytes: %.200v", len(err.Error()), err)
		}
	})
}
