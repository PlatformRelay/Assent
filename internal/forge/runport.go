package forge

import "errors"

// RunPort is the single named, forge-neutral composite interface `cmd/assent`
// depends on (E10-S02 / ADR-0021 §1), replacing the anonymous interface literal
// at the `assent run` call site and the hand-rolled `refFilePort` in the
// provider host. Every method is forge-neutral — no adapter type appears in any
// signature — so a second adapter satisfies the port without a line changing in
// `cmd/assent`.
//
// The composition is deliberate, not a convenience union:
//
//	forge.Forge        — the ADR-0017 §7 write surface (Reconcile's substrate)
//	forge.Snapshotter  — E4-S06 forge Snapshot (heads, changed files, capabilities)
//	forge.Resolver     — E4-S06 require-review evidence resolution
//	GetMR              — MR metadata read (ADR-0021 item 1's "Describe"; the tree's
//	                     method is GetMR, which S00's drift table records)
//	FileAtRef          — ref-addressed reads, retained ONLY for the ref-addressed
//	                     decision inputs ADR-0015 §1 mandates (policy, binding,
//	                     config, pack, provider declarations, resource-owner
//	                     registry — all from the target ref, never an MR branch)
//	FileAtBase/FileAtHead — the governed subject's ONLY legal accessors
//	                     (ADR-0021 item 5): MR-relative, so an adapter owns how it
//	                     reaches a fork's head. FileAtBase/FileAtHead take the
//	                     composite (project, mr) handle — S00 §Forward obligations
//	                     2b delegated the binding choice to S02, and the composite
//	                     handle keeps the port stateless and symmetric with
//	                     GetMR/Forge/ResolveRequest, which all address an MR as
//	                     (project, mr).
//	Identity           — ADR-0021 item 7: the authenticated identity is a port
//	                     concept, because "which artifacts are ours?" is the basis
//	                     of marker filtering and spoof resistance under two auth
//	                     shapes (a PAT's identity is a User, an App's is a bot).
//
// The two content accessors are NOT interchangeable and neither replaces the
// other: FileAtRef survives for ref-addressed POLICY loads from the target ref
// (a fork's head must never reach them — ADR-0015 §1's trust boundary), while
// FileAtBase/FileAtHead are the governed subject's only legal accessors. An
// adapter that implements FileAtBase/Head by delegating to
// FileAtRef(project, path, sourceBranch) reintroduces the fabricated-DELETE
// defect item 5 exists to kill.
type RunPort interface {
	Forge
	Snapshotter
	Resolver

	// GetMR reads the merge request's metadata (heads, branches, fork flag,
	// labels) at the pinned read time.
	GetMR(project, mr string) (MRInfo, error)

	// FileAtRef reads a file at an explicit ref of an explicit project. Its ONLY
	// remaining legal callers are the ref-addressed decision-input loads: the
	// MergePolicy, RulesetBinding, Config and pack (run.go, all at the pinned
	// target SHA), the provider-host declaration and the resource-owner registry
	// (provider_host.go, both at the target ref — the latter decides WHO MAY
	// APPROVE, and D-130 records why it must never move onto an MR-relative
	// accessor).
	FileAtRef(project, path, ref string) ([]byte, error)

	// FileAtBase reads the governed subject's content on the BASE side of the
	// merge request. MR-relative: the adapter owns how it reaches the base
	// content, and the base side is the target project's target commit by
	// definition (the base of an MR is never forked).
	FileAtBase(project, mr, path string) ([]byte, error)

	// FileAtHead reads the governed subject's content on the HEAD side of the
	// merge request. On GitHub a fork PR's head lives in the fork — the adapter
	// resolves it via the PR head (refs/pull/N/head or the head SHA), never by a
	// branch name inside the base project, whose 404 would mint a fabricated
	// whole-file DELETE (S00 Q1).
	FileAtHead(project, mr, path string) ([]byte, error)

	// Identity returns the authenticated identity of the caller's credentials.
	// A PAT authenticates as a User; a GitHub App installation authenticates as
	// a bot. Marker filtering MUST match this identity — not "any bot" — so an
	// "exclude any bot" filter (which would blind assent to its own PAT-authored
	// comments) is structurally impossible to satisfy the port.
	Identity() (Identity, error)
}

// IdentityKind distinguishes the two auth shapes the port must not preclude
// (ADR-0021 item 7): a PAT's identity is a User; an App installation's is a bot.
type IdentityKind string

const (
	// IdentityUser is a human-or-PAT user identity (GitLab PAT, GitHub PAT).
	IdentityUser IdentityKind = "user"
	// IdentityApp is an application/bot identity (GitHub App installation).
	IdentityApp IdentityKind = "app"
)

// Identity is the authenticated caller identity the port exposes.
type Identity struct {
	Kind  IdentityKind
	Login string // the forge login/username the artifacts are authored as.
	ID    string // forge-assigned identity id ("" when the forge exposes none).
}

// ErrUnauthorized is the forge-neutral sentinel for a permission failure
// (401/403 — and, on GitHub, a 404 that provably hides a permission failure per
// the S00 Q4 mapping). It is a PRESENCE-DISCRIMINATION signal, never "absent":
// `ErrNotFound` means the resource does not exist at the addressed location;
// `ErrUnauthorized` means the caller cannot know. An adapter that cannot
// distinguish the two for an endpoint returns an error, not absence
// (ADR-0021 item 6).
//
// Adapters wrap it (the prefix pattern of ErrNotFound applies: the sentinel
// carries no forge prefix; the wrapping adapter names itself).
var ErrUnauthorized = errors.New("unauthorized (401/403)")
