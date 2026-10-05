# P5-E10 — GitHub forge adapter execution: spec delta

**REQ prefix:** `REQ-E10X-01`.
**Vehicle:** this change executes stories S02–S17 of the authoritative epic spec
(`openspec/specs/p5-e10-github-forge/spec.md`), whose REQs remain the acceptance bar for
every task in [tasks.md](../tasks.md). This delta adds only what the epic spec deliberately
left open or that execution must state: the S00 addressing-model obligation 2b (the content
accessor's MR handle), the neutral factory requirement implied but not typed by the epic
spec, and the LGTM governance flag. Nothing here amends ADR-0021 items 1–8.

---

## E10X-S01 — The content accessors' MR handle is composite `[autonomous]`

**Goal:** resolve S00 §Forward obligations 2b before the port signature freezes:
`FileAtBase`/`FileAtHead` carry the target project explicitly rather than binding it inside
adapter state, so no adapter holds hidden per-construction state and every MR-addressing
method on the port shares one shape.

**Counterpoints:** a client bound to one project at construction makes `FileAtBase(mr, path)`
the shorter signature but hides a stateful binding in an otherwise stateless port, changes
every factory signature, and diverges from `GetMR`, `Forge` writes and `ResolveRequest`,
which all address an MR as `(project, mr)`. The composite handle has none of those costs.

- **REQ-E10X-01-01** — Given S00 obligation 2b, when `forge.RunPort` is declared, then
  `FileAtBase(project, mr, path string) ([]byte, error)` and
  `FileAtHead(project, mr, path string) ([]byte, error)` take the target project and MR
  explicitly, and no `RunPort` implementation binds a project in constructor state — proven
  by a compile-time assertion that the adapter's constructor takes no project argument and
  by the conformance factory constructing one backend usable for several `Config.Project`
  values in one test.
  - Test: `internal/forge/port_test.go`
  - Verify: `go test ./internal/forge/...`
  - Level: L1
- **REQ-E10X-01-02** — Given the governed-subject call sites migrate in T1, when
  `cmd/assent/run.go` reads the governed base/head, then it calls
  `FileAtBase(project, mr, path)` / `FileAtHead(project, mr, path)` with the MR's own
  identity — never a ref inside one project — so a fork PR's head content is addressed
  inside the source repository, and the fabricated whole-file DELETE is impossible by
  construction (S00 Q1; ADR-0021 item 5).
  - Test: `cmd/assent/run_test.go`, `internal/forge/conformance/`
  - Verify: `go test ./internal/forge/conformance/ -run 'TestConformanceFork'`
  - Level: L1

## E10X-S02 — The neutral adapter factory is a named deliverable `[autonomous]`

**Goal:** `cmd/assent` satisfies depguard's adapter-import denial while still constructing
real adapters, by routing construction through one neutral package that both adapters may be
imported by, and only by that package.

- **REQ-E10X-02-01** — Given depguard must deny `cmd/assent` importing
  `internal/forge/gitlab` or `internal/forge/github`, when the adapters are constructed,
  then the constructor lives in a dedicated package (e.g. `internal/forge/factory`) whose
  entire job is `Spec → forge.RunPort`; `cmd/assent` imports that package and no adapter;
  the factory is the only importer of both adapters.
  - Test: `hack/lint/depguard_test.sh`, `internal/forge/factory/`
  - Verify: `task lint && go build ./...`
  - Level: L1
- **REQ-E10X-02-02** — Given S02/S04 are `[maintainer LGTM]` core-contract work (epic spec
  Executability), when the change ships, then the PR description carries the LGTM flag on
  those stories and the PR is NOT merged by this change's worker — the merge decision is
  the operator's, which is exactly the surface the LGTM gate needs.
  - Test: PR description
  - Verify: manual review
  - Level: L0

## E10X-S03 — v1 GitHub ships comment-only, honestly `[autonomous]`

**Goal:** the open questions OQ-33/OQ-34 leave GitHub's `protected-pipeline-source` and
`eligible-approval-evidence` at `unknown` for v1 ⇒ non-arming. The change ships that
limitation visibly rather than papering over either question.

- **REQ-E10X-03-01** — Given ADR-0021's `unknown == absent` arming rule and both open
  questions, when the GitHub adapter reports its capabilities, then the two `unknown`
  capabilities refuse arming, the refusal reason is contributor-legible, and the
  fail-closed table test (E10-S12's REQ-02) proves `merges == 0` for all three simulated
  deltas plus the all-present positive control (`merges == 1`).
  - Test: `internal/forge/github/failclosed_test.go`
  - Verify: `go test ./internal/forge/github/ -run TestDeltasFailClosed`
  - Level: L1
