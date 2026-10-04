package spikes

import (
	"go/build/constraint"
	"os"
	"strings"
	"testing"
)

// TestMaliciousExecIsBuildConstrained pins D17 (XREV-S04-02): the hostile exec
// provider spike must carry a `//go:build ignore` constraint so it is excluded
// from the public module's buildable package set and cannot be fetched with
// `go install github.com/PlatformRelay/assent/hack/spikes/provider/maliciousexec`.
// Without this test the one-line constraint could be deleted and the
// exfiltrator would silently become installable again.
func TestMaliciousExecIsBuildConstrained(t *testing.T) {
	const path = "provider/maliciousexec/main.go"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var expr constraint.Expr
	for _, line := range strings.Split(string(raw), "\n") {
		if !constraint.IsGoBuild(line) {
			continue
		}
		expr, err = constraint.Parse(line)
		if err != nil {
			t.Fatalf("parse build constraint %q: %v", line, err)
		}
	}
	if expr == nil {
		t.Fatalf("%s carries no //go:build constraint; it is installable under the public module path (D17)", path)
	}
	// `ignore` is a tag that is never set, so the file is always excluded from a
	// package build. Assert the expression evaluates false with no tags set.
	if expr.Eval(func(string) bool { return false }) {
		t.Fatalf("%s's build constraint %q is satisfied with no tags set — the file is still buildable (D17)", path, expr.String())
	}
}
