# Design note: forge port lift (pre-GitHub-adapter) — seeds E10

Status: note only (no decision taken; decide via ADR when the GitHub adapter epic opens).
Trigger: ARCH-02, PROJECT-AUDIT-2026-08-06.

Problem: `cmd/assent`'s orchestration read port leaks GitLab-named types —
`forgePort` embeds `GetMR(...) (gitlab.MRInfo, error)` and `FileAtRef` whose absent-file
sentinel is `gitlab.ErrNotFound` (run.go:57-63, :420; plus `gitlab.MRInfo` threaded
through `decide`, `resolveRunApproval`, `mrFrom`, `buildDesired`, `run_render.go`), and
run.go calls `gitlab.SyntheticDigest` directly. A GitHub adapter cannot satisfy this port.

Target shape (all in `internal/forge`, zero behavior change):

1. `forge.MRInfo` — move the struct verbatim (IID, ProjectID, branches, SHAs, ForkMR).
   Transition: `type MRInfo = forge.MRInfo` alias in `internal/forge/gitlab` so the lift
   is a mechanical import swap; drop the alias when cmd/assent no longer imports gitlab.
2. `forge.ErrNotFound` — neutral sentinel owned by `internal/forge`; the gitlab adapter
   wraps its 404s to it (`errors.Is` compatible). `fileAtRefOrAbsent` matches the
   neutral sentinel only.
3. `forgePort` becomes fully neutral:
   `interface { forge.Forge; forge.Snapshotter; forge.Resolver;
   Describe(project, mr string) (forge.MRInfo, error);
   FileAtRef(project, path, ref string) ([]byte, error) }` — and moves into
   `internal/forge` as the named composite port (`forge.RunPort`), with a conformance
   contract: FileAtRef absent-file behavior, Describe error taxonomy, Snapshot
   completeness fields (ADR-0020).
4. `gitlab.SyntheticDigest` call-sites in run.go collapse onto
   `snapshot.Heads.MergeResultDigest` (already computed by Snapshot); the merge-digest
   *scheme* is adapter-owned, never computed in cmd.
5. The fake forge implements `forge.RunPort` directly, making the port (not the gitlab
   client) the thing conformance-tested — the actual seam a GitHub adapter plugs into.

Order of work when E10 opens: (1)+(2) mechanical, (3) port move + conformance cases,
(4) digest collapse, then the GitHub adapter against `forge.RunPort` only.
Flag: touching the port is core-contract work — maintainer LGTM required per GOVERNANCE.
