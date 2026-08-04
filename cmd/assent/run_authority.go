package main

import (
	"github.com/PlatformRelay/assent/internal/forge"
)

// untrustedExecutionContext reports ADR-0015 §8 fork/untrusted contributor MR
// contexts where the run path must stay advisory-only (report, zero forge writes).
func untrustedExecutionContext(snapshot forge.Snapshot) bool {
	return snapshot.Heads.ForkMR
}
