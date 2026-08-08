package forge

import "errors"

// port.go holds the FORGE-NEUTRAL half of the orchestration READ port — the
// types `cmd/assent` needs from whichever forge it is driving, with no adapter
// in their names (AUD-S15 / audit finding ARCH-02).
//
// Before this file, `cmd/assent`'s read port spoke `gitlab.MRInfo` and matched
// absent files against `gitlab.ErrNotFound`, so a second adapter could not
// satisfy the port without `cmd/assent` importing GitLab. These declarations
// move that vocabulary to the port; `internal/forge/gitlab` keeps transitional
// aliases so the adapter and its tests are untouched.
//
// Scope note (deliberate, not an omission): the composite `forge.RunPort`
// interface and the collapse of `gitlab.SyntheticDigest` onto
// `Snapshot.Heads.MergeResultDigest` are E10 work, per
// docs/planning/design-notes/e10-forge-port-lift.md. This file ships only the
// mechanical, behaviour-preserving lift.

// ErrNotFound is the forge-neutral sentinel for "the resource is absent at the
// requested ref". `FileAtRef` returns it (wrapped) for a 404, and callers match
// it with errors.Is — an absent governed file is a presence SIGNAL the caller
// interprets (EFE-S03), never a crash.
//
// Unlike every other sentinel in this package it carries NO `forge: ` prefix,
// on purpose: the others are returned directly by this package's own code and
// name themselves, whereas this one is only ever returned WRAPPED by an
// adapter, which supplies its own prefix. The GitLab adapter renders it as
//
//	gitlab: resource not found (404): file "x" at ref "y"
//
// A prefix here would double up ("gitlab: forge: resource not found").
var ErrNotFound = errors.New("resource not found (404)")

// MRInfo is the merge-request metadata `assent run` pins its evaluation to. All
// SHAs are the exact values the forge reports at read time; TargetSHA is the
// target BRANCH TIP, NOT the merge-base (GitLab's diff_refs.base_sha — a
// different commit).
type MRInfo struct {
	IID          string
	ProjectID    string
	SourceBranch string
	TargetBranch string
	SourceSHA    string // the MR's current source head.
	TargetSHA    string // the target branch tip.
	// ForkMR is true when the source project differs from the target project
	// (fork workflow).
	ForkMR bool
}
