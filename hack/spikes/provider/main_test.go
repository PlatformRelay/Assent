package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Paths to the exec provider binaries, built once in TestMain.
var (
	toyExecBin       string
	maliciousExecBin string
)

// execTestTimeout is the transport deadline every spike test uses when it talks
// to a real child process or a local httptest server.
//
// It is deliberately generous, and it must stay that way. TestContract asserts
// HTTP/exec response equivalence and TestIsolation asserts env scrubbing —
// neither asserts timeout behaviour, so the deadline is incidental plumbing.
// TestStates keeps its own short, deliberately-blocking deadline because timeout
// classification is exactly what it asserts.
//
// The old value was 5s. Measured under `go test -race ./...` with 36 competing
// CPU burners, one spawn of a freshly-built child binary here took 1.2s-5.3s
// wall clock (~200ms unloaded) — the first exec of a just-written file pays
// page-in plus macOS code-signature validation while sibling package binaries
// compile and run. 5s therefore fired on a healthy child and surfaced as
// `signal: killed`. Do not "optimise" this back down: a short deadline here
// buys no coverage and only re-arms the flake.
const execTestTimeout = 60 * time.Second

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "provider-spike")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	build := func(out, pkg string) string {
		bin := filepath.Join(tmp, out)
		cmd := exec.Command("go", "build", "-o", bin, pkg) // #nosec G204 -- test fixture build with fixed args
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "building %s: %v\n", pkg, err)
			os.Exit(1)
		}
		return bin
	}
	toyExecBin = build("toyexec", "./toyexec")
	maliciousExecBin = build("maliciousexec", "./maliciousexec")

	os.Exit(m.Run())
}
