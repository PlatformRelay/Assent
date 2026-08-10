# P5-E10 — GitHub forge adapter + Actions entrypoint

**Epic ID / REQ prefix:** `E10` / `REQ-E10-S0n-nn`.

**Unlock:** D-140 (2026-08-10). E10 was **Locked (D-012)** — "unlocks with a named consumer",
reaffirmed locked by D-017 and D-019. The operator unlocked it directly; D-140 records the
authority, the v1 scope, and what the unlock does **not** authorize. No spec text in this file
predates that row.

**Problem**: assent has exactly one forge adapter, and the seam a second one plugs into is
half-built. AUD-S15 lifted `MRInfo`/`ErrNotFound` into `internal/forge/port.go`, but
`cmd/assent`'s `forgePort` is still an anonymous interface literal at the call site, `run.go`
still calls `gitlab.SyntheticDigest`, capability vocabulary is GitLab-private, transport
policy (bounded reads, pagination caps, retry, deadlines) lives inside
`internal/forge/gitlab`, and **all ~1,166 lines of `internal/forge/conformance` are in
`_test.go` files Go cannot import** — so the suite that defines "behaves like a forge" cannot
be run by a second adapter. `catalog.yaml`'s `github-deferred` rows (D-084) are therefore
unflippable by construction, not merely unimplemented. The 2026-08-09 audit recorded this as
ARCH-18/ARCH-19: `e10-forge-port-lift.md` under-scopes the epic by two design buckets
(capability model; transport/auth policy) plus the unimportable suite. ADR-0021 decides all
three; this epic executes it.

**Key ground truth (de-risks the epic):**
- **The dossier is the spec input, not guesswork.** `docs/planning/forge-dossier-github.md`
  already answers OQ-7 and OQ-18 (parity for the gate, not the device;
  required-conversation-resolution carries acknowledgement; `REQUEST_CHANGES` reserved for
  block) and §4 records the port-design consequences: eleven capability flags, two
  GitHub-only behaviours the port must not preclude (review lifecycle submit→dismiss→
  re-request; **GraphQL-only thread resolution**), and the finding that nothing in the GitLab
  precondition table is GitLab-only.
- **Reuse, don't re-invent:** `internal/forge/forge.go` (Reconcile, markers, fail-closed
  errors), `internal/forge/fake`, `internal/forge/conformance` (cases exist; only their
  *packaging* is wrong), P3-E5 fixtures under
  `docs/contracts/p3-e5-publication-protocol/fixtures/`, and the E4 GitLab adapter as the
  reference implementation of every port method.
- **Frozen schemas stay frozen:** epic DoD is **`git diff schemas/` == 0**. GitHub introduces
  no new decision-contract field; `capabilityGap` and `pins.mergeResultDigest` already model
  the absent-capability case (ADR-0017 §1, nullable only when the capability is absent).
- **`internal/core` stays I/O-free** (`TestCorePurity`); everything here is
  `internal/forge/**` plus the `cmd/assent` edge.
- **The seam is `internal/`, not public API.** `forge.RunPort` carries no `apiVersion` and no
  compatibility window; getting it wrong costs a refactor inside this epic, never a break.

**Scope**: **Seam wave** — (S01) conformance suite extracted to an importable package; (S02)
`forge.RunPort` named composite port + depguard; (S03) `SyntheticDigest` collapse; (S04)
neutral capability model; (S05) port-level transport requirements. **Adapter wave** — (S06)
GitHub client (REST + GraphQL, PAT + App auth); (S07) Snapshot; (S08) Resolve →
`ApprovalEvidence`; (S09) GitHub capability report; (S10) Reconcile writes; (S11) SHA-guarded
merge + deferred arming; (S12) capability gaps fail closed. **Integration wave** — (S13) forge
selection in `run`/`doctor`; (S14) conformance parity + catalog flip; (S15) docs/maturity
truth; (S16) Actions entrypoint; (S17) exit gate. **Infra-gated:** (S18) live GitHub adoption
proof on a real repository.

**Non-goals** (fenced): **Rego backend** (E11 — separate spec, unlocked by D-141);
**`serve` / webhooks / keyed per-MR lock** (E12); **remote policy packs** (E13, still Locked
per D-012 — D-140 unlocks E10 only); **CRD adapter** (E14, gated on Spike D); **a third forge
or a plugin/gRPC forge protocol** (ADR-0021 Option D, rejected — no named consumer);
**widening any frozen schema**; **fixing the audit's pre-existing GitLab findings**
(SEC-01/SEC-04/SEC-05, RELI-01/02/03 — deferred by D-138/D-139 and tracked there, not here);
**replacing the in-memory fake** (it stays the Reconcile unit-test substrate and becomes the
first `forge.RunPort` implementation).

**ADRs**: **0021** (this epic's governing ADR — `RunPort`, capability model, transport
policy), 0005 (forge abstraction, conformance suite), 0011 (core ports), 0012 (finding
lifecycle), 0015 §2/§4/§8 (SHA-guard, protected pipeline, execution-authority matrix), 0017
§1/§3/§7/§9 (merge-result pins, require-review evidence, Snapshot→Resolve→Reconcile, doctor
typed report), 0019 (marker/reconciliation protocol), 0020 (changed-file completeness).
**Reuse**: E4 GitLab adapter, `internal/forge/fake`, existing conformance cases, P3-E5
fixtures, P1-E3-S03 GitHub dossier, AUD-S10/S11 transport hardening, AUD-S15 port lift.
**New**: importable conformance runner, `forge.RunPort`, `forge.Capability`/`CapabilityReport`,
`internal/forge/github`, forge selection, `action.yml`.

**Executability**: S01–S17 **`[autonomous]`** with httptest servers (REST **and** GraphQL) and
the in-memory fake. S02/S04 additionally **`[maintainer LGTM]`** — ADR-0021 names the port and
capability model core-contract work per GOVERNANCE, and `/agent-loop-auto`'s stop conditions
already require surfacing public-API/core-contract changes rather than auto-merging them. S18
**`[infra-gated · operator]`** (a real GitHub repository, live PRs, a token).

**Dependency order**: S01 → S02 → {S03, S04} → S05 → S06 → {S07, S08, S09} → S10 → S11 → S12
→ S13 → S14 → {S15, S16} → S17 → S18. **Do first: S01** — until the suite is importable, every
adapter story is written against no executable contract.

## Judgment calls (decide-and-log / operator)

(a) **🟡 OPERATOR — Actions entrypoint (S16) is IN scope but LAST and independently
droppable.** `later-phases.md` titles the epic "GitHub adapter + Actions entrypoint", so
dropping it silently would contradict the plan; but a composite action is a *distribution*
concern (E9's domain) sitting on top of an adapter, and it is the one story whose absence
leaves everything else useful. Recommended default: keep S16 as the final pre-gate story;
if the operator prefers, cut it to a follow-on and the exit gate (S17) drops its row without
any other change. **Recorded as D-140's open sub-question.**

(b) **DECIDED — v1 GitHub target is behavioural parity for the *gate*, with capability gaps
failing closed.** Per dossier §3 and OQ-7/OQ-18: required-conversation-resolution carries
acknowledgement, `REQUEST_CHANGES` is reserved for block, and the three known deltas
(review dismissal, auto-merge revoke, merge queue) are modelled as capabilities, not as
special cases. Where GitHub cannot prove what the gate needs, the adapter reports the gap and
**never arms** — the same shape as GitLab free tier (E4 judgment call (c), D-012 C6/C7).

(c) **DECIDED — GraphQL is adapter-internal freedom.** Thread resolution is GraphQL-only on
GitHub (dossier §4). The port never names a transport, so the adapter owns both clients; the
conformance suite asserts *behaviour* (thread resolved / unresolved / superseded), never the
protocol used to achieve it.

(d) **DECIDED — `unknown` capabilities are treated as `absent` for arming.** ADR-0021 §3. The
audit found three live cases where an unprobed forge setting was cited as a safety argument
(SEC-01 `auto_cancel_redundant_pipelines`, SEC-04 C17, RELI-03 `reset_approvals_on_push`).
Making "unprobed" a distinct, non-arming state converts that class of defect from a
per-adapter bug into a port-level impossibility.

(e) **DECIDED — GitLab arming behaviour changes get their own decision row.** S04 forces every
capability the GitLab adapter *implicitly* assumed to be stated explicitly, which may turn an
arming path that passes today into an honest capability gap. That is the intended outcome, but
it is a user-visible behaviour change: it must be recorded as a `D-nnn` row of its own and
called out in the changelog, never absorbed silently into "E10 refactor".

(f) **DECIDED — the live adoption proof (S18) mirrors D-042, and is not an exit-gate
blocker for the autonomous slice.** D-042 closed the D-012 GitLab adoption gate with real
MRs on a real project. GitHub deserves the same evidence, but it needs operator-provided
infrastructure; S17 gates the autonomous work, S18 records the live proof when the operator
runs it.

---

## Seam wave

### E10-S01 — Extract the conformance suite into an importable package `[autonomous]`

- **Goal**: a second adapter can execute the *existing* forge conformance cases without
  copying them.
- **Why now**: `internal/forge/conformance` is 1,166 lines across four `_test.go` files. Go
  cannot import `_test.go`, so today the only way to conformance-test a new adapter is
  duplication — which guarantees drift and makes D-084's `github-deferred` rows unflippable.
- **Dependencies**: none. **This is story zero.**
- **Definition of done**: case bodies live in importable Go; `go test ./internal/forge/...`
  passes with **no case deleted, renamed, or weakened**; the GitLab entry point is a thin
  `_test.go` calling the shared runner.

- **REQ-E10-S01-01** — Given the conformance cases currently in `_test.go`, when the package
  is restructured, then it exports `conformance.RunSuite(t *testing.T, f Factory)` where
  `Factory` constructs a `forge.RunPort` plus its fixture state, and every existing case runs
  through it.
  - Test: `internal/forge/conformance/suite.go`, `internal/forge/conformance/suite_test.go`
  - Verify: `go test ./internal/forge/conformance/...`
  - Level: L1
- **REQ-E10-S01-02** — Given extraction is a refactor, when the suite runs against the GitLab
  factory, then the set of executed case IDs is **identical** to the pre-extraction set — proven
  by a test that reads `catalog.yaml` and asserts every non-`github-deferred` row's `test`
  field is executed, failing on any missing or extra case.
  - Test: `internal/forge/conformance/catalog_test.go`
  - Verify: `go test ./internal/forge/conformance/ -run TestCatalogMatchesExecutedCases`
  - Level: L1
- **REQ-E10-S01-03** — Given `catalog.yaml` indexes cases by `forge:`, when the schema of that
  file is extended, then each row gains an explicit adapter list and the loader **rejects an
  unknown adapter name** (strict-decode, P3-E2) rather than silently ignoring it.
  - Test: `internal/forge/conformance/catalog.yaml`, `catalog_test.go`
  - Verify: `go test ./internal/forge/conformance/ -run TestCatalogStrictDecode`
  - Level: L1

### E10-S02 — `forge.RunPort` composite port + depguard `[autonomous · engine-grade · maintainer LGTM]`

- **Goal**: `cmd/assent` depends on one named, forge-neutral interface and on no concrete
  adapter package.
- **Dependencies**: S01 (so the port change is proven by an executable suite).
- **Definition of done**: `forge.RunPort` declared in `internal/forge`; `cmd/assent`'s
  anonymous port literal deleted; depguard denies **both** concrete adapters from `cmd/assent`;
  zero behaviour change (goldens and conformance byte-identical).

- **REQ-E10-S02-01** — Given ADR-0021 §1, when `forge.RunPort` is declared, then it composes
  `forge.Forge`, `forge.Snapshotter`, `forge.Resolver`, `Describe(project, mr string)
  (forge.MRInfo, error)` and `FileAtRef(project, path, ref string) ([]byte, error)`, and
  `cmd/assent` references that named type only.
  - Test: `internal/forge/port.go`, `cmd/assent/run.go`
  - Verify: `go build ./... && go test ./cmd/... ./internal/forge/...`
  - Level: L1
- **REQ-E10-S02-02** — Given ARCH-02's leak must not recur with a second adapter, when
  `hack/lint/depguard_test.sh` runs, then it denies `cmd/assent` importing
  `internal/forge/gitlab` **or** `internal/forge/github` with no symbol allowlist — replacing
  the current `New`/`WithSleeper`/`SyntheticDigest` allowlist, which S03 empties.
  - Test: `hack/lint/depguard_test.sh`, `.golangci.yml`
  - Verify: `task lint`
  - Level: L1
- **REQ-E10-S02-03** — Given the fake is the Reconcile substrate, when `internal/forge/fake`
  is updated, then it implements `forge.RunPort` **directly** (design-note step 5), making the
  port — not the GitLab client — the conformance-tested thing.
  - Test: `internal/forge/fake/fake.go`, `internal/forge/conformance/suite_test.go`
  - Verify: `go test ./internal/forge/...`
  - Level: L1

### E10-S03 — Collapse `SyntheticDigest` onto `Snapshot.Heads.MergeResultDigest` `[autonomous · engine-grade]`

- **Goal**: the merge-result digest *scheme* is adapter-owned; `cmd/assent` computes no
  forge-specific hash.
- **Dependencies**: S02.
- **Definition of done**: `run.go` reads the digest from the snapshot the adapter already
  produced; the depguard symbol allowlist from S02 is now **empty**; DecisionRecord
  `pins.mergeResultDigest` is byte-identical for GitLab on every existing golden.

- **REQ-E10-S03-01** — Given design-note step 4, when `run.go` needs a merge-result pin, then
  it uses `snapshot.Heads.MergeResultDigest` and calls no adapter digest function; the pin
  stays nullable exactly when the capability is absent (ADR-0017 §1).
  - Test: `cmd/assent/run.go`, `cmd/assent/run_test.go`
  - Verify: `go test ./cmd/... && task check`
  - Level: L1
- **REQ-E10-S03-02** — Given this is a byte-identical refactor, when the E4/E7 goldens are
  regenerated, then `git diff` over the golden corpus is **empty**.
  - Test: existing golden corpus
  - Verify: `task test && git diff --exit-code -- test/ examples/`
  - Level: L1

### E10-S04 — Neutral capability model `[autonomous · engine-grade · maintainer LGTM]`

- **Goal**: `capabilityGap` means the same thing on every forge, and unprobed never arms.
- **Dependencies**: S02.
- **Definition of done**: `forge.Capability` closed enum seeded from dossier §4's eleven
  flags; `forge.CapabilityReport` returns `supported | absent | unknown` + reason per
  capability; the gap is computed **at the port**; `unknown` blocks arming identically to
  `absent`; `assent doctor` prints the typed report (ADR-0017 §9, ADR-0019's
  `duplicate_prevention:` MUST).

- **REQ-E10-S04-01** — Given dossier §4, when `forge.Capability` is declared, then it contains
  exactly the eleven named flags (`resolvable-threads`, `threads-block-merge`,
  `blocking-review`, `review-dismissal-restrictions`, `sha-guarded-merge`,
  `deferred-merge-arming`, `arming-revoked-on-push`, `merge-result-pinning`,
  `eligible-approval-evidence`, `approval-reset-on-push`, `protected-pipeline-source`), and
  decoding an unknown capability name is an error, not a skip.
  - Test: `internal/forge/capability.go`, `internal/forge/capability_test.go`
  - Verify: `go test ./internal/forge/ -run TestCapabilityEnumClosed`
  - Level: L0
- **REQ-E10-S04-02** — Given ADR-0021 §3 and judgment call (d), when a capability is reported
  `unknown`, then every arming decision treats it as `absent` — proven by a table test over
  all three states asserting `unknown` and `absent` produce the identical non-arming outcome
  and a distinguishable *reason* string.
  - Test: `internal/forge/capability_test.go`
  - Verify: `go test ./internal/forge/ -run TestUnknownDoesNotArm`
  - Level: L1
- **REQ-E10-S04-03** — Given `capabilityGap` is port-computed, when the GitLab adapter is
  migrated to return a `CapabilityReport`, then no adapter computes a gap itself, and any
  capability the adapter does not actually probe is reported `unknown` — **not** `supported`.
  - Test: `internal/forge/gitlab/capability.go`, `internal/forge/gitlab/capability_test.go`
  - Verify: `go test ./internal/forge/gitlab/`
  - Level: L1
- **REQ-E10-S04-04** — Given judgment call (e), when S04 changes any GitLab arming outcome
  that passed before, then a `D-nnn` row records it and the changelog carries a user-facing
  entry; when no outcome changes, the story records that explicitly.
  - Test: `docs/decisions/decisions.md`
  - Verify: manual — story cannot close without one of the two statements present
  - Level: L0

### E10-S05 — Port-level transport requirements `[autonomous]`

- **Goal**: availability and fail-closed behaviour are properties of the port, not of one
  HTTP client.
- **Dependencies**: S01, S04.
- **Definition of done**: bounded response reads, pagination caps, idempotent-GET-only retry
  with backoff, and context deadlines are stated as port requirements with conformance cases;
  the GitLab adapter (AUD-S10/S11) satisfies them unchanged; **writes are never retried**.

- **REQ-E10-S05-01** — Given AUD-S10, when any adapter reads a forge response, then the read
  is byte-bounded and paginated collection reads are capped, with exhaustion failing **closed**
  (never a silent truncation) — asserted by conformance cases that serve oversized and
  over-paginated responses.
  - Test: `internal/forge/conformance/transport.go`, `catalog.yaml`
  - Verify: `go test ./internal/forge/conformance/ -run TestConformanceBoundedReads`
  - Level: L1
- **REQ-E10-S05-02** — Given AUD-S11, when a request fails transiently, then only idempotent
  GETs retry (bounded, backed off, deadline-bounded) and **no write is ever retried** —
  asserted by a conformance case counting write attempts across an injected 5xx.
  - Test: `internal/forge/conformance/transport.go`
  - Verify: `go test ./internal/forge/conformance/ -run TestConformanceWritesNeverRetried`
  - Level: L1

## Adapter wave

### E10-S06 — GitHub client: REST + GraphQL transports, PAT and App auth `[autonomous]`

- **Dependencies**: S05.
- **Definition of done**: `internal/forge/github` with both transports behind adapter-internal
  interfaces, httptest cassettes for both, and two auth shapes; **no token value ever logged
  or embedded in an error**; the package compiles with zero imports from `cmd/assent`.

- **REQ-E10-S06-01** — Given dossier §4, when the adapter needs thread resolution, then it
  holds a GraphQL client alongside the REST client, both exercised by httptest cassettes, and
  the port surface names neither.
  - Test: `internal/forge/github/client.go`, `internal/forge/github/client_test.go`
  - Verify: `go test ./internal/forge/github/`
  - Level: L2
- **REQ-E10-S06-02** — Given GitHub supports PAT and GitHub App installation tokens, when
  credentials are supplied, then both shapes authenticate, installation-token refresh is
  handled, and a missing/expired credential fails **closed** with an error naming no secret
  material.
  - Test: `internal/forge/github/auth.go`, `internal/forge/github/auth_test.go`
  - Verify: `go test ./internal/forge/github/ -run TestAuth`
  - Level: L2
- **REQ-E10-S06-03** — Given `gitleaks` and D-002, when the adapter and its cassettes are
  committed, then no real token, org name, or private repository name appears in any fixture.
  - Test: `internal/forge/github/testdata/**`
  - Verify: `task scrub && task check`
  - Level: L0

### E10-S07 — GitHub Snapshot `[autonomous]`

- **Dependencies**: S06.
- **Definition of done**: PR metadata → `forge.MRInfo` (head/base SHAs, fork detection),
  changed-file enumeration satisfying **ADR-0020 completeness** (truncation is an opaque
  enumeration failure, never a short list), and merge-result pinning via `refs/pull/N/merge`.

- **REQ-E10-S07-01** — Given `forge.MRInfo`'s contract, when a PR is described, then
  `SourceSHA` is the PR head, `TargetSHA` is the **base branch tip** (not the merge base),
  and `ForkMR` is true iff the head repository differs from the base repository — with the
  **absent-means-trusted** trap closed: an absent/`null` head-repo field yields an error, not
  `ForkMR=false` (audit SEC-05's GitLab analogue).
  - Test: `internal/forge/github/snapshot.go`, `snapshot_test.go`
  - Verify: `go test ./internal/forge/github/ -run TestSnapshotMRInfo`
  - Level: L2
- **REQ-E10-S07-02** — Given ADR-0020, when the changed-file listing is truncated or paginated
  past the cap, then Snapshot reports an **opaque enumeration failure** and the run fails
  closed; a complete listing reports completeness explicitly.
  - Test: `internal/forge/github/snapshot.go`
  - Verify: `go test ./internal/forge/github/ -run TestChangedFilesCompleteness`
  - Level: L2
- **REQ-E10-S07-03** — Given dossier C16, when a merge-result pin is available, then
  `Heads.MergeResultDigest` is derived from the merge ref, and when merge-queue or merge-ref
  semantics make it unavailable, the digest is **nil with a capability gap** — never a
  fabricated value.
  - Test: `internal/forge/github/snapshot.go`
  - Verify: `go test ./internal/forge/github/ -run TestMergeResultPin`
  - Level: L2

### E10-S08 — GitHub Resolve → `ApprovalEvidence` `[autonomous · engine-grade]`

- **Dependencies**: S06.
- **Definition of done**: typed `ApprovalEvidence` per dossier §2, validating against the
  frozen `schemas/decision/v1alpha1/approval-evidence.schema.json` with `git diff schemas/`
  == 0; PR author and bot identities excluded; dismissed reviews never count.

- **REQ-E10-S08-01** — Given dossier §2, when reviews are resolved, then evidence is built
  from the review chain (latest non-dismissed review per eligible reviewer), the **PR author
  is excluded**, bot identities are excluded, and a `REQUEST_CHANGES` review is carried as
  block signal — never as approval.
  - Test: `internal/forge/github/resolve.go`, `resolve_test.go`
  - Verify: `go test ./internal/forge/github/ -run TestResolveApprovalEvidence`
  - Level: L2
- **REQ-E10-S08-02** — Given the review lifecycle (submit → dismiss → re-request), when a
  previously-approving review is dismissed or the reviewer is re-requested, then the evidence
  no longer counts that approval — asserted on a cassette replaying all three transitions.
  - Test: `internal/forge/github/resolve_test.go`
  - Verify: `go test ./internal/forge/github/ -run TestDismissedApprovalNotCounted`
  - Level: L2
- **REQ-E10-S08-03** — Given eligibility cannot always be proven (dossier §2 plan gating),
  when the eligible-approver set is unavailable, then Resolve reports a capability gap and
  `require-review` is **unsatisfiable** — never satisfied by an unproven approval.
  - Test: `internal/forge/github/resolve.go`
  - Verify: `go test ./internal/forge/github/ -run TestUnprovableEligibilityFailsClosed`
  - Level: L2

### E10-S09 — GitHub capability report `[autonomous · engine-grade]`

- **Dependencies**: S04, S06.
- **Definition of done**: the adapter returns a `CapabilityReport` covering all eleven flags;
  every capability it does not actually probe is `unknown`; plan/visibility gating is
  reflected honestly.

- **REQ-E10-S09-01** — Given S04's enum, when the GitHub adapter reports capabilities, then
  every one of the eleven flags carries a state and a reason, and a compile-time-exhaustive
  test fails if a new capability is added without a GitHub answer.
  - Test: `internal/forge/github/capability.go`, `capability_test.go`
  - Verify: `go test ./internal/forge/github/ -run TestCapabilityExhaustive`
  - Level: L1
- **REQ-E10-S09-02** — Given dossier "open verification items", when a capability's real
  behaviour is unverified against a live API (merge-queue plan gating, `enablePullRequestAutoMerge`
  preconditions), then it is reported `unknown` with the open item cited — not optimistically
  `supported`.
  - Test: `internal/forge/github/capability.go`
  - Verify: `go test ./internal/forge/github/ -run TestUnverifiedReportedUnknown`
  - Level: L1

### E10-S10 — GitHub Reconcile writes `[autonomous · engine-grade]`

- **Dependencies**: S07, S08, S09.
- **Definition of done**: ADR-0019 marker protocol parity — idempotent finding threads,
  duplicate repair, occurrence supersession, resolve-no-longer-desired, post-publication
  rescan, summary slot — all through the shared `internal/forge` engine, with GraphQL used
  for thread resolution.

- **REQ-E10-S10-01** — Given ADR-0019, when Reconcile runs on GitHub, then the **same**
  `internal/forge` engine drives it (the adapter supplies primitives only) and the S01
  conformance replay cases pass against the GitHub factory.
  - Test: `internal/forge/github/reconcile_test.go`, `internal/forge/conformance/`
  - Verify: `go test ./internal/forge/... -run TestConformance`
  - Level: L1
- **REQ-E10-S10-02** — Given contributor marker spoofing (E4-S09 precedent), when threads are
  listed, then the author-identity filter excludes non-bot authors, and a malformed marker is
  **skipped with a warning** rather than bricking reconciliation (RELI-06 precedent).
  - Test: `internal/forge/github/reconcile_test.go`
  - Verify: `go test ./internal/forge/github/ -run TestMarkerSpoofAndMalformed`
  - Level: L2

### E10-S11 — SHA-guarded merge + deferred arming `[autonomous · engine-grade]`

- **Dependencies**: S10.
- **Definition of done**: merge is SHA-pinned (ADR-0015 §2); deferred arming uses
  `enablePullRequestAutoMerge`; merge queue is treated as the merge-result pin;
  arming-revoked-on-push is honoured.

- **REQ-E10-S11-01** — Given ADR-0015 §2, when head or base has moved since evaluation, then
  the merge fails closed with the shared `ErrSHAMoved` — asserted by the S01 SHA-guard
  conformance cases running against the GitHub factory.
  - Test: `internal/forge/conformance/` (GitHub factory)
  - Verify: `go test ./internal/forge/conformance/ -run TestConformanceSHAGuard`
  - Level: L1
- **REQ-E10-S11-02** — Given dossier C8′/C11/C14, when arming is requested, then
  `enablePullRequestAutoMerge` is used, a subsequent push revokes the arming, and if the
  revoke-on-push capability is `unknown` or `absent`, **arming is refused**.
  - Test: `internal/forge/github/merge.go`, `merge_test.go`
  - Verify: `go test ./internal/forge/github/ -run TestArmingRevokeOnPush`
  - Level: L2

### E10-S12 — Capability gaps fail closed on GitHub `[autonomous · engine-grade]`

- **Dependencies**: S11.
- **Definition of done**: each of the three known GitHub deltas (dismissal restrictions,
  auto-merge revoke, merge queue) has an explicit fail-closed test proving no auto-merge when
  the capability is `absent` or `unknown`.

- **REQ-E10-S12-01** — Given judgment call (b), when any capability required by the armed path
  is `absent` or `unknown`, then the DecisionRecord carries an honest `capabilityGap`, the run
  does not merge, and the reason is contributor-legible in the posted comment.
  - Test: `internal/forge/github/`, `cmd/assent/run_test.go`
  - Verify: `go test ./... -run TestCapabilityGapBlocksMerge`
  - Level: L1
- **REQ-E10-S12-02** — Given fail-closed claims are worthless untested, when each of the three
  deltas is simulated absent, then a table test asserts `merges == 0` for all three — the
  polarity E6/AUD reviews repeatedly found untested.
  - Test: `internal/forge/github/failclosed_test.go`
  - Verify: `go test ./internal/forge/github/ -run TestDeltasFailClosed`
  - Level: L1

## Integration wave

### E10-S13 — Forge selection in `run` / `doctor` `[autonomous]`

- **Dependencies**: S12.
- **Definition of done**: explicit `--forge {gitlab|github}` plus remote-host autodetect;
  **ambiguity or an unrecognised host fails closed** (no default-to-GitLab); `cmd/assent`
  still imports no concrete adapter (S02's depguard holds).

- **REQ-E10-S13-01** — Given two adapters exist, when the forge is not explicitly selected and
  cannot be unambiguously detected, then the run **errors** rather than defaulting — asserted
  including the unknown-host and conflicting-signal cases.
  - Test: `cmd/assent/forge_select.go`, `forge_select_test.go`
  - Verify: `go test ./cmd/... -run TestForgeSelection`
  - Level: L1
- **REQ-E10-S13-02** — Given S02, when forge selection is wired, then construction happens
  behind a factory returning `forge.RunPort`, and `task lint`'s depguard still denies
  `cmd/assent` importing either adapter package.
  - Test: `hack/lint/depguard_test.sh`
  - Verify: `task lint`
  - Level: L1

### E10-S14 — Conformance parity + catalog flip `[autonomous]`

- **Dependencies**: S13.
- **Definition of done**: the S01 suite runs against the GitHub factory in CI; every
  `github-deferred` row in `catalog.yaml` is either flipped to `both` or **retains the
  deferral with a named, cited reason**; D-084 is dispositioned.

- **REQ-E10-S14-01** — Given D-084, when the catalog is updated, then no row remains
  `github-deferred` without a reason field naming the blocking open-verification item, and a
  test fails on any bare deferral.
  - Test: `internal/forge/conformance/catalog.yaml`, `catalog_test.go`
  - Verify: `go test ./internal/forge/conformance/ -run TestNoBareDeferrals`
  - Level: L1
- **REQ-E10-S14-02** — Given both adapters implement `forge.RunPort`, when CI runs, then
  `RunSuite` executes against **both** factories in the same job, and a new case added for one
  forge fails the build until the other declares support or a cited deferral.
  - Test: `internal/forge/conformance/suite_test.go`, `.github/workflows/verify.yaml`
  - Verify: `task check`
  - Level: L1

### E10-S15 — Docs and maturity truth `[autonomous]`

- **Dependencies**: S14.
- **Definition of done**: README feature-maturity table moves GitHub from **Planned** to its
  earned tier; C4 diagrams updated (ARCH-05 precedent — planned vs shipped legend); `cli.md`
  documents `--forge`; dossier "open verification items" each dispositioned
  (verified / still open / reported `unknown`).

- **REQ-E10-S15-01** — Given the audit's docs-truth family, when GitHub ships, then no
  document claims a GitHub capability the capability report marks `unknown` — asserted by a
  docs-truth test comparing the README maturity row against the adapter's reported states.
  - Test: `README.md`, `hack/docs/maturity_test.sh`
  - Verify: `task check`
  - Level: L1
- **REQ-E10-S15-02** — Given `later-phases.md` and `meta-plan.md` carry E10's status, when the
  epic closes, then both are updated in the same change as the exit gate, and the E10 row
  cites D-140.
  - Test: `openspec/specs/later-phases.md`, `docs/planning/meta-plan.md`
  - Verify: manual + `task check`
  - Level: L0

### E10-S16 — Actions entrypoint `[autonomous — scope-flagged, judgment call (a)]`

- **Dependencies**: S15.
- **Definition of done**: a composite `action.yml` invoking the released binary at a pinned
  version, an example workflow, and documentation; **no new adapter behaviour** — the action
  is packaging only.

- **REQ-E10-S16-01** — Given E9's distribution model, when the action runs, then it consumes a
  **pinned, checksum-verified** released binary (never `go install` at HEAD), and the pin is
  asserted by a test reading `action.yml`.
  - Test: `action.yml`, `hack/release/action_pin_test.sh`
  - Verify: `task check`
  - Level: L1
- **REQ-E10-S16-02** — Given ADR-0015's trust boundaries, when the action is documented, then
  the workflow example uses the **base-ref workflow trust** model from the dossier (policy
  loaded from the target ref, never the PR head) and states the required token scopes.
  - Test: `docs/`, `action.yml`
  - Verify: manual review + `task check`
  - Level: L0

### E10-S17 — Exit gate `[autonomous]`

- **Dependencies**: S01–S16.
- **Definition of done**: `hack/forge/e10_exitgate_test.sh` proves, in one invocation:
  `RunSuite` green against **both** factories; zero bare `github-deferred` rows; `task check`
  green; `git diff schemas/` == 0; depguard denies both adapters from `cmd/assent`; the
  capability enum is exhaustively answered by both adapters; every fail-closed polarity test
  present. If judgment call (a) drops S16, the gate drops that row and nothing else.

- **REQ-E10-S17-01** — Given every prior story, when the exit gate runs, then it fails if any
  of the above conditions regresses, and it cites D-140 plus ADR-0021.
  - Test: `hack/forge/e10_exitgate_test.sh`
  - Verify: `bash hack/forge/e10_exitgate_test.sh && task check`
  - Level: L1

### E10-S18 — Live GitHub adoption proof `[infra-gated · operator]`

- **Dependencies**: S17 + operator-provided infrastructure.
- **Definition of done**: mirroring D-042 — assent runs on **live PRs** in a real GitHub
  repository, producing at least one REVIEW (with a resolvable thread) and one APPROVE with a
  real SHA-pinned merge; DecisionRecords retained under
  `docs/decisions/evidence/p5-e10-s18-adoption/`; a `D-nnn` row records the proof.

- **REQ-E10-S18-01** — Given D-012's "synthetic does not count" standard, when the proof is
  recorded, then the evidence names a real repository and real PR URLs, and the open
  verification items resolved by the live run are moved out of `unknown` in S09's report.
  - Test: `docs/decisions/evidence/p5-e10-s18-adoption/`
  - Verify: operator-run; evidence committed
  - Level: L3
