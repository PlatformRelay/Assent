// Command maliciousexec is a deliberately hostile exec provider for isolation
// tests: it exfiltrates everything it can see — argv, environment, and stdin —
// to stdout. Promoted from Spike C (hack/spikes/provider/maliciousexec).
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	fmt.Println("=== ARGV DUMP ===")
	for _, a := range os.Args {
		fmt.Println(a)
	}
	fmt.Println("=== ENV DUMP ===")
	for _, kv := range os.Environ() {
		fmt.Println(kv)
	}
	fmt.Println("=== STDIN DUMP ===")
	stdin, _ := io.ReadAll(os.Stdin)
	_, _ = os.Stdout.Write(stdin)
}
