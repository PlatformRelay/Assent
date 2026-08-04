package main

import (
	"fmt"
	"io"

	"github.com/PlatformRelay/assent/internal/catalogue"
)

// catalogue.go is the thin filesystem+command shell for `assent catalogue <dir>`
// (E3-S07). It discovers the repo's `.assent/**` tree via catalogue.LoadFromDir,
// and hands the loaded Input to the pure internal/catalogue generator, which emits
// the deterministic, additive-tolerant rule catalogue (D-017 B10) as JSON on stdout
// for the docs pipeline.
//
// Surface (D-048): a DISTINCT subcommand, not `assent lint --catalogue` —
// catalogue generation is a docs-pipeline artifact, not a pass/fail gate, so it
// carries no error-diagnostic exit semantics. A clean generation over loadable
// packs exits 0; a malformed pack the strict loader rejects, or an undiscoverable
// tree, is a hard error (non-zero) — the catalogue is generated over conformant
// packs (lint owns tolerant diagnosis of malformed ones).
//
// The directory walk + loader calls are the ONLY I/O; the generator is pure.

// runCatalogue is the testable entry point for `assent catalogue`. args[0] is the
// repo directory (the tree containing `.assent/**`). It returns a process exit
// code: 0 on a clean generation, 2 on a usage/discovery/load error.
func runCatalogue(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] == "" {
		_, _ = fmt.Fprintln(stderr, "assent catalogue: a directory argument is required (usage: assent catalogue <dir>)")
		return 2
	}
	dir := args[0]

	in, err := catalogue.LoadFromDir(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent catalogue:", err)
		return 2
	}

	out, err := catalogue.Build(in).Marshal()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent catalogue:", err)
		return 2
	}
	_, _ = stdout.Write(out)
	return 0
}
