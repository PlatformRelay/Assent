package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// REQ-E9-S01-01: link-time -X main.version injection is reflected on `assent version`.
func TestVersionLdflags(t *testing.T) {
	bin := buildAssent(t, "-X main.version=1.2.3-test")
	out := runAssentVersion(t, bin)
	if got, want := strings.TrimSpace(out), "assent 1.2.3-test"; got != want {
		t.Fatalf("assent version = %q, want %q", got, want)
	}
}

// REQ-E9-S01-02: default dev build (no injection) keeps the source default semver.
func TestVersionDevDefault(t *testing.T) {
	bin := buildAssent(t, "")
	out := runAssentVersion(t, bin)
	if got, want := strings.TrimSpace(out), "assent 0.0.0-dev"; got != want {
		t.Fatalf("assent version = %q, want %q", got, want)
	}
}

func buildAssent(t *testing.T, ldflags string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "assent")
	args := []string{"build", "-o", bin}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, ".")
	cmd := exec.Command("go", args...) // #nosec G204 -- test-controlled build of this package
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func runAssentVersion(t *testing.T, bin string) string {
	t.Helper()
	cmd := exec.Command(bin, "version") // #nosec G204 -- test-built binary path
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s version: %v", bin, err)
	}
	return buf.String()
}
