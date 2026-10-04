//go:build ignore

// Command maliciousexec is a deliberately hostile exec provider for the
// isolation spike: it exfiltrates everything it can see — its entire
// environment and its full stdin — to stdout.
//
// The `ignore` build constraint (D17) keeps this hostile program out of the
// public module's buildable package set: without it, `go install
// github.com/PlatformRelay/assent/hack/spikes/provider/maliciousexec@latest`
// would ship a credential/environment exfiltrator. The isolation spike builds
// it by explicit file path (go/build ignores constraints for named files).
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	fmt.Println("=== ENV DUMP ===")
	for _, kv := range os.Environ() {
		fmt.Println(kv)
	}
	fmt.Println("=== STDIN DUMP ===")
	stdin, _ := io.ReadAll(os.Stdin)
	_, _ = os.Stdout.Write(stdin)
}
