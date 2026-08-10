# Design note: forge port lift (pre-GitHub-adapter) — seeds E10

Status: **SUPERSEDED as the design authority by ADR-0021** (2026-08-10), which took the
decision this note deferred. E10 opened with D-140; the epic is
`openspec/specs/p5-e10-github-forge/spec.md`. This note is retained as the record of the
pre-AUD-S15 problem and of steps 1–2, which shipped.
Trigger: ARCH-02, PROJECT-AUDIT-2026-08-06.

> **This note is INCOMPLETE as an epic scope — that was a finding, not an omission you should
> work around.** The 2026-08-09 audit recorded ARCH-18/ARCH-19: this note "under-scopes the
> epic by two design buckets, and the conformance suite cannot be run by a second adapter
> because all ~1,155 lines live in `_test.go` files Go cannot import."
>
> The original ARCH-18/ARCH-19 finding text is **not in the repo** — only the one-line summary
> at `agent-context/PROJECT-AUDIT-2026-08-09.md:412` survives. The two buckets were therefore
> **re-derived** during the 2026-08-10 design session, and are recorded as a re-derivation,
> not as a citation:
>
> - **Bucket A — capability model.** `docs/planning/forge-dossier-github.md` §4 enumerates
>   eleven capability flags the port needs; `probeCapabilities` reads three project fields,
>   and `capabilityGap` is computed in GitLab terms. Arming (ADR-0015 §4) hangs off that
>   vocabulary, so a second adapter would restate it or silently arm under a different meaning
>   of "capable". ADR-0021 §3 resolves this: a port-owned capability enum, `supported |
>   absent | unknown`, gap computed at the port, and **`unknown` never arms**.
> - **Bucket B — transport and auth policy.** Bounded reads and pagination caps (AUD-S10),
>   idempotent-GET retry/backoff and deadlines (AUD-S11) were built into
>   `internal/forge/gitlab`. GitHub additionally needs a **GraphQL** client (thread resolution
>   is GraphQL-only, dossier §4) and **two auth shapes** (PAT, App installation token). Left
>   at the adapter, the forges' availability and fail-closed behaviour diverge undetected.
>   ADR-0021 §4 makes these port requirements with conformance cases, while leaving protocol
>   and auth as adapter-internal freedom.
>
> The unimportable conformance suite is tracked separately as **E10-S01, story zero**
> (ADR-0021 §2) — until it is fixed, no adapter can be developed against an executable
> contract and D-084's `github-deferred` catalog rows are unflippable by construction.

Progress: **steps 1 and 2 below shipped in AUD-S15** (`internal/forge/port.go`,
`internal/forge/port_test.go`, the ARCH-02 section of `hack/lint/depguard_test.sh`). Steps
3–5 remain open for E10. The "Problem" paragraph therefore describes the PRE-AUD-S15 state
and is kept verbatim as the record of what was fixed; what is still true of `cmd/assent`
today is only the `gitlab.SyntheticDigest` call (step 4), now grep-pinned to an allowlist
of exactly `New`, `WithSleeper`, `SyntheticDigest`.

Problem (pre-AUD-S15): `cmd/assent`'s orchestration read port leaks GitLab-named types —
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
