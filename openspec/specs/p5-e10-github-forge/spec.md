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
`internal/forge/gitlab`, and **all 1,155 lines of `internal/forge/conformance` are in
`_test.go` files Go cannot import** — so the suite that defines "behaves like a forge" cannot
be run by a second adapter. The 2026-08-09 audit recorded this as ARCH-18/ARCH-19:
`e10-forge-port-lift.md` under-scopes the epic by two design buckets (capability model;
transport/auth policy) plus the unimportable suite.

**An adversarial review of this spec's first draft (2026-08-10) found three further buckets,
two of them P0, and they are why S00 exists.** All are *addressing or representation*
failures — a class the P1-E3-S03 dossier structurally could not surface, because it studied
GitHub's **endpoints**, not how the port **names** things: fork-head addressing (the port
reads head content by branch name in one project, so every GitHub fork PR would mint a
fabricated whole-file DELETE); HTTP-status → sentinel collapse (GitHub 404s permission
denials, so *forbidden* would read as *absent*); and the record surface (`pins` is
`additionalProperties:false` with a **single-string** `capabilityGap` required *iff*
`mergeResultDigest` is null — an eleven-capability report has nowhere valid to live).
**ADR-0021 items 5–8 decide all three**; this epic executes them.

*Corrections carried from that review, kept rather than quietly edited away:* an earlier
draft claimed the extraction would unblock `catalog.yaml`'s `github-deferred` rows. **False** —
both rows are `level: L3, package: test/e2e`, gated on live GitHub infrastructure (S18), not
on importability. The extraction's real justification is the executable contract. A second
draft claimed `capabilityGap` "already models the absent-capability case". **Also false** — it
models exactly one capability, merge-result pinning, which is why it is singular and coupled
to that field.

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
- **Frozen schemas stay frozen — at a stated cost.** Epic DoD is **`git diff schemas/` == 0**,
  which is only achievable because ADR-0021 item 8 scopes the multi-capability report to
  `doctor` output and arming-refusal reasons, **never the DecisionRecord**. The cost is
  explicit: a capability gap that blocks a merge leaves no trace in the record beyond the
  existing single `capabilityGap` string. Widening `pins` is a `v1alpha2` conversation.
- **`internal/core` stays I/O-free** (`TestCorePurity`); everything here is
  `internal/forge/**` plus the `cmd/assent` edge.
- **The seam is `internal/`, not public API.** `forge.RunPort` carries no `apiVersion` and no
  compatibility window; getting it wrong costs a refactor inside this epic, never a break.

**Scope**: **Seam wave** — (S00) the GitHub addressing & representation model, written before
any code; (S01) conformance suite extracted to an importable package; (S02)
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

**Executability**: S00–S17 **`[autonomous]`** with httptest servers (REST **and** GraphQL) and
the in-memory fake. S00 is a **design** story (a document, no code) and S00/S02/S04 additionally
**`[maintainer LGTM]`** — ADR-0021 names the port and
capability model core-contract work per GOVERNANCE, and `/agent-loop-auto`'s stop conditions
already require surfacing public-API/core-contract changes rather than auto-merging them. S18
**`[infra-gated · operator]`** (a real GitHub repository, live PRs, a token).

**Dependency order**: **S00** → S01 → S02 → {S03, S04} → S05 → S06 → {S07, S08, S09} → S10 →
S11 → S12 → S13 → S14 → {S15, S16} → S17 → S18. **Do first: S00** — the adversarial review
found two P0 representation defects by reading the port against the code, and both would
otherwise surface at S07 with the port signature frozen and the golden corpus pinned. S00 is
four questions and roughly a page; it is the cheapest risk reduction in the epic. **S01 is the
first code story** — until the suite is importable, every adapter story is written against no
executable contract.

## Judgment calls (decide-and-log / operator)

(a) **✅ OPERATOR-ANSWERED (2026-08-10): option (a) — Actions entrypoint (S16) is IN scope but
LAST and independently droppable.** `later-phases.md` titles the epic "GitHub adapter + Actions
entrypoint", so dropping it silently would contradict the plan; but a composite action is a
*distribution* concern (E9's domain) sitting on top of an adapter, and it is the one story whose
absence leaves everything else useful. The operator confirmed the recommended default: **keep
S16 as the final pre-gate story**; should it later be cut to a follow-on, the exit gate (S17)
drops its row without any other change. This sub-question of D-140 is **closed** — S16 is not
blocked and needs no further operator input.

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

### E10-S00 — GitHub addressing & representation model `[autonomous · design · maintainer LGTM]`

- **Goal**: answer, on paper and before any code, the four questions whose wrong answers the
  adversarial review demonstrated are P0 — each of which would otherwise be discovered after
  the port signature is frozen.
- **Dependencies**: none. **This is the first story in the epic.**
- **Definition of done**: `docs/planning/github-addressing-model.md` answers all four with a
  decision, and each answer names the conformance case that will prove it. Answers that
  contradict ADR-0021 items 5–8 amend the ADR in the same change.

- **REQ-E10-S00-01** — **How does the port name the head content of a fork PR?** Given
  `run.go:270,274` reads base/head by branch name in one project and `MRInfo` carries no
  source-repository identifier, when the model is written, then it fixes the addressing shape
  (ADR-0021 item 5 proposes `FileAtBase(mr, path)` / `FileAtHead(mr, path)`), and states the
  concrete failure it prevents: a fork PR 404s → `fileAtRefOrAbsent` → `nil` →
  `OneSidedLifecycle` → **fabricated whole-file DELETE**.
  - Test: `docs/planning/github-addressing-model.md`
  - Verify: manual review; the named conformance case appears in S01's catalog
  - Level: L0
- **REQ-E10-S00-02** — **What operationally decidable predicate makes each of the eleven
  capability flags `supported`, per forge?** Given a tri-state with no membership test is a
  vocabulary rather than a model, when the model is written, then every flag has a concrete
  probeable condition per adapter — **explicitly including `protected-pipeline-source`**
  (ADR-0015 §4's arming prerequisite, which has no single readable GitHub analogue) and
  **`eligible-approval-evidence`** (dossier §2: no API returns the computed per-PR eligible
  code owners). Where no predicate exists, the model says so and the consequence — the gate
  cannot be armed on that forge — is stated as a product limitation, never papered over with a
  heuristic (the SEC-04 failure mode).
  - Test: `docs/planning/github-addressing-model.md`
  - Verify: manual review; S04's enum cannot be frozen until every flag has an entry
  - Level: L0
- **REQ-E10-S00-03** — **Where does an eleven-capability report live in the record?** Given
  `$defs.pins` is `additionalProperties:false` with a single-string `capabilityGap` required
  *iff* `mergeResultDigest` is null, when the model is written, then it confirms or overturns
  ADR-0021 item 8's choice (report scoped to `doctor`/refusal reasons, not the DecisionRecord)
  and states the audit-trail cost plainly. Recording it in the record's open top-level object
  is rejected — an unvalidated safety-bearing field is a fail-closed guarantee in name only.
  - Test: `docs/planning/github-addressing-model.md`
  - Verify: manual review
  - Level: L0
- **REQ-E10-S00-04** — **What is each adapter's HTTP-status → port-sentinel mapping?** Given
  `forge.ErrNotFound` is a semantic *presence* signal and GitHub 404s permission denials, when
  the model is written, then it fixes the mapping per adapter and names the conformance case
  proving *forbidden* never renders as *absent*.
  - Test: `docs/planning/github-addressing-model.md`
  - Verify: manual review; the named case appears in S01's catalog
  - Level: L0

### E10-S01 — Extract the conformance suite into an importable package `[autonomous]`

- **Goal**: a second adapter can execute the *existing* forge conformance cases without
  copying them.
- **Why now**: `internal/forge/conformance` is 1,155 lines across four `_test.go` files (the
  package totals 1,166 including the 11-line non-test `doc.go`). Go
  cannot import `_test.go`, so today the only way to conformance-test a new adapter is
  duplication — which guarantees drift and makes D-084's `github-deferred` rows unflippable.
- **Dependencies**: S00.
- **Definition of done**: case bodies live in importable Go; `go test ./internal/forge/...`
  passes with **no case deleted, renamed, or weakened**; the GitLab entry point is a thin
  `_test.go` calling the shared runner.
- **The tension to resolve deliberately, not cheaply**: the existing cases assert on
  `*fake.Forge` internals — `sha_guard_test.go:49` takes `*fake.Forge`, and
  `reconciliation_test.go:220` type-asserts to it — reading recorded writes (`Merges`,
  `Approvals`). A `Factory` returning a `forge.RunPort` is therefore only half a contract. The
  cheap resolution is to weaken assertions to what both backends can observe, which is exactly
  how a suite silently stops proving the SHA-guard. REQ-E10-S01-04 forbids that resolution.

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
- **REQ-E10-S01-04** — Given the cases assert on `*fake.Forge` internals, when the suite is
  extracted, then the package defines an explicit **port-level observation surface** (the
  recorded-writes view a case may assert on: merges, approvals, threads, comments) that both
  adapters implement, and **no assertion is downgraded to accommodate a backend**. Proven by a
  test asserting the extracted SHA-guard case still observes a merge *attempt count*, not
  merely a returned error — the specific weakening this REQ exists to prevent.
  - Test: `internal/forge/conformance/observe.go`, `suite_test.go`
  - Verify: `go test ./internal/forge/conformance/ -run TestSHAGuardObservesMergeAttempts`
  - Level: L1

### E10-S02 — `forge.RunPort` composite port + depguard `[autonomous · engine-grade · maintainer LGTM]`

- **Goal**: `cmd/assent` depends on one named, forge-neutral interface and on no concrete
  adapter package.
- **Dependencies**: S00, S01 (so the port change is proven by an executable suite).
- **Definition of done**: `forge.RunPort` declared in `internal/forge`; **both** of
  `cmd/assent`'s port declarations retired — `run.go:64 forgePort` (the anonymous literal at
  the call site) **and** `provider_host.go:246 refFilePort`, a second, hand-rolled
  `FileAtRef`-only interface. Naming only the first is how this story closes while
  `cmd/assent` still depends on a private port: replacing `forgePort` alone leaves
  `refFilePort` standing, the DoD reads satisfied, and `go build` + `task lint` stay green.
  (`refFilePort` is a *named* interface, not an anonymous literal — the DoD's original wording
  did not describe it and so did not cover it.) Plus: **the neutral adapter factory lands in
  this story**; depguard denies **both** concrete adapters from `cmd/assent`; zero behaviour
  change (goldens and conformance byte-identical). **The two names above are evidence, not the
  contract** — an allowlist of the ports that exist today goes stale the moment a third is
  added, and this enumeration has now been wrong three times. REQ-E10-S02-07 states the
  invariant and enforces it mechanically.
- **Corrected after adversarial review — the first draft of this story could not close.** It
  required depguard to deny both adapters "with no symbol allowlist", but `cmd/assent/main.go:72,83`
  calls `gitlab.New(endpoint, token, botAuthor)` and no story supplied a neutral factory until
  **S13**, ten stories later. Worse, `hack/lint/depguard_test.sh:356-363` is an anti-vacuity
  guard that **fails the build** unless it sees ≥4 `gitlab.<Exported>` references in
  `cmd/assent` *including* `New` and `SyntheticDigest` — and `task lint` is this story's own
  Verify command. The factory therefore moves from S13 into S02, and REQ-E10-S02-04 replaces
  the scanner's positive control rather than deleting it.

- **REQ-E10-S02-01** — Given ADR-0021 §1, when `forge.RunPort` is declared, then it composes
  `forge.Forge`, `forge.Snapshotter`, `forge.Resolver`, `Describe(project, mr string)
  (forge.MRInfo, error)`, `FileAtRef(project, path, ref string) ([]byte, error)` **and**
  `FileAtBase(mr, path string) ([]byte, error)` / `FileAtHead(mr, path string) ([]byte, error)`,
  and `cmd/assent` references that named type only. **Both accessors are required and they are
  not interchangeable** — REQ-E10-S02-05 binds which is legal where. `FileAtRef` is retained
  **only** for the ref-addressed *policy* loads ADR-0015 §1 mandates (`cmd/assent/run.go:203`,
  `:211`, `:230`, `:249` — MergePolicy, RulesetBinding, **Config** and pack — **plus
  `cmd/assent/provider_host.go:82` (provider host declaration) and `:275` (resource-owner
  registry)**, all from the target ref by name. That is **six** call sites, not four: the
  `run.go` list alone is not exhaustive for `cmd/assent`. `provider_host.go:275` is the single
  most dangerous one to migrate — the registry decides **who may approve**, and preferring the
  checkout there was the D-130 vouching escalation);
  implementing this REQ by freezing `FileAtRef` as the *sole* content accessor satisfies the
  signature while preserving the fabricated-DELETE defect, and is a failure of this story.
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
- **REQ-E10-S02-04** — Given `hack/lint/depguard_test.sh:356-363` hard-fails unless it sees
  `gitlab.New` and `gitlab.SyntheticDigest` in `cmd/assent`, when those call-sites are removed
  by the factory, then the scanner's positive control is **replaced, never deleted**: it
  asserts against a violating *copy* of `cmd/assent` (the polarity-A tree the script already
  builds in `$WORK`), so the scanner is still proven to see real code while the real tree
  legitimately contains zero adapter symbols.
  - Test: `hack/lint/depguard_test.sh`
  - Verify: `task lint`
  - Level: L1
- **REQ-E10-S02-05** — Given ADR-0021 item 5, when `RunPort` is declared, then **the governed
  subject** is addressed **relative to the merge request** (`FileAtBase(mr, path)` /
  `FileAtHead(mr, path)`), not by `(project, branch-name)`, so an adapter owns how it reaches a
  fork's head. A conformance case proves a **fork MR with an unchanged governed file yields NO
  lifecycle event** on every adapter — the fabricated-DELETE defect. Smuggling
  `refs/pull/N/head` into `MRInfo.SourceBranch` is rejected: it corrupts a documented field and
  leaks into rendering.
  **The boundary is enforced in both directions, and both are asserted:**
  (i) the governed-subject reads (`cmd/assent/run.go:270`, `:274`, via `fileAtRefOrAbsent`)
  call `FileAtBase`/`FileAtHead` and **no** `FileAtRef` call remains on the governed-subject
  path — asserted by a source-level guard, because a green `TestForkMRNoFabricatedDelete`
  against a fake that happens to serve the right bytes does not prove the call was rewritten.
  **Read this as scoped to the forge-sourced path, not as "all governed-subject sourcing is now
  MR-relative":** under `--checkout`, `run.go:283` overrides base/head from the local tree via
  `dirCheckout.FileContents(governed)`, which carries no `FileAtRef` call and is therefore
  invisible to the source-level guard. That is intended existing behaviour (EFE-S03 /
  ADR-0008 §4 — the local head tree is the presence authority), and S02 does not change it;
  (ii) the **policy and decision-input** loads (`run.go:203`, `:211`, `:230`, `:249` and
  `provider_host.go:82`, `:275` — all six) still use
  `FileAtRef(project, path, targetRef)` and are **not** migrated — a test asserts policy is
  read from the target ref of the target project even for a fork MR, so a well-meaning
  "consistency" refactor onto an MR-relative accessor (which would let a fork's head reach the
  policy load) fails the suite rather than silently crossing ADR-0015 §1's trust boundary.
  - Test: `internal/forge/port.go`, `internal/forge/conformance/`, `cmd/assent/run.go`
  - Verify: `go test ./... -run 'TestForkMRNoFabricatedDelete|TestPolicyLoadsFromTargetRefOnForkMR'`
  - Level: L1
- **REQ-E10-S02-07** — Given an allowlist of today's ports cannot survive a port added
  tomorrow, when `hack/lint/depguard_test.sh` runs, then it enforces the **invariant** rather
  than the list: **no interface declared in `cmd/assent` may carry a forge read or write method
  except `forge.RunPort` itself**. The scanner already walks `cmd/assent` source and already
  carries the mutation-control pattern at `:356-363` that REQ-E10-S02-04 rebuilds, so this is
  an added assertion, not new machinery. A mutation control proves it goes **red** on a
  hand-rolled `interface{ FileAtRef(...) }` reintroduced anywhere in `cmd/assent`.
  `checkout.go:27 localCheckout` must stay **green** — it is a local-tree seam carrying no
  forge method, and a guard that cannot tell those apart would either block legitimate seams or
  be switched off. Without this REQ, "both port declarations retired" is provable only by a
  human re-reading the tree at S02 time — which is exactly what failed on this enumeration
  three times over.
  - Test: `hack/lint/depguard_test.sh`, `cmd/assent/run.go`, `cmd/assent/provider_host.go`
  - Verify: `task lint && task lint-depguard-test`
  - Level: L1
- **REQ-E10-S02-06** — Given ADR-0021 item 7, when the port is declared, then it exposes the
  **authenticated identity**, and a case proves markers are recognised as our own under
  **both** auth shapes — a PAT identity is a `User`, so an "exclude any bot" filter would make
  assent blind to its own comments and duplicate threads forever.
  - Test: `internal/forge/port.go`, `internal/forge/conformance/`
  - Verify: `go test ./internal/forge/... -run TestOwnMarkersRecognisedBothAuthShapes`
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
  and a distinguishable *reason* string. **Paired positive control (required, not optional):**
  the same table asserts that all-capabilities-`supported` **does** arm. Without it the REQ is
  satisfied by an adapter that never arms under any conditions — the documented
  tests-that-cannot-fail defect class, whose in-repo antidote is the mutation-control pattern
  at `hack/lint/depguard_test.sh:356-363`.
  - Test: `internal/forge/capability_test.go`
  - Verify: `go test ./internal/forge/ -run 'TestUnknownDoesNotArm|TestAllSupportedDoesArm'`
  - Level: L1
- **REQ-E10-S04-05** — Given a transport failure during probing is not the same fact as an
  absent capability, when a probe request fails (5xx, timeout), then it is a **hard process
  error**, never a silent downgrade to `unknown`. Otherwise a 502 on one endpoint flips
  APPROVE→REVIEW on an otherwise successful exit-0 run, making decisions network-dependent and
  breaking ReplayBundle reproducibility — the same separation E11's evaluation budget gets right.
  - Test: `internal/forge/capability.go`, `capability_test.go`
  - Verify: `go test ./internal/forge/ -run TestProbeFailureIsNotUnknown`
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
  - Verify: `bash hack/check-sanitization.sh && task check`
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
  listed, then the filter matches **the token identity's own user/app id** (`GET /user` /
  `GET /app`) — the GitLab precedent is `first.Author.Username != c.botAuthor`, our *specific*
  identity, and dossier C9 says "identical to GitLab: match `user.id` of the token identity".
  An "exclude any bot" filter is explicitly wrong: this repo runs Renovate, so any second app
  causes marker collision, and anyone who can install an app gets marker spoofing. A malformed
  marker is **skipped with a warning** rather than bricking reconciliation (RELI-06 precedent).
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
- **REQ-E10-S11-03** — Given ADR-0017 §1 requires `pins.mergeResultDigest` to be the
  **evaluated** merge-result digest, and given a merge queue builds
  `gh-readonly-queue/{base}/…` containing the target **plus other queued PRs** (dossier C14),
  when the queue is in use, then the adapter proves at the **hermetic** tier that the recorded
  pin corresponds to the commit the queue actually merges — or reports
  `merge-result-pinning` as a capability gap and does not arm. The only catalog row that would
  otherwise catch this (`github-merge-queue-sha-guard`) is `level: L3, package: test/e2e`, so
  nothing in the autonomous slice can detect the divergence.
  - Test: `internal/forge/github/merge_test.go`, `internal/forge/conformance/`
  - Verify: `go test ./internal/forge/github/ -run TestMergeQueuePinMatchesMergedCommit`
  - Level: L2
- **REQ-E10-S11-04** — Given "Require merge queue" may reject `PUT /pulls/{n}/merge` outright,
  when the queue is enabled, then S00's predicate table records whether SHA-pinned direct merge
  and queue-based merge are **mutually exclusive paths** rather than one, and the adapter
  implements whichever the forge permits — failing closed if neither can be SHA-pinned.
  - Test: `docs/planning/github-addressing-model.md`, `internal/forge/github/merge.go`
  - Verify: `go test ./internal/forge/github/ -run TestQueueAndDirectMergeExclusivity`
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
  polarity E6/AUD reviews repeatedly found untested — **and, in the same table, that the
  all-capabilities-present case yields `merges == 1`.** The positive control is mandatory:
  without it every row passes vacuously on an adapter that never arms at all, and S17's exit
  gate goes green on an adapter that does nothing.
  - Test: `internal/forge/github/failclosed_test.go`
  - Verify: `go test ./internal/forge/github/ -run TestDeltasFailClosed`
  - Level: L1
- **REQ-E10-S12-03** — Given the adversarial review established that a GitHub adapter which
  **never arms** is a plausible shipped outcome (`protected-pipeline-source` and
  `eligible-approval-evidence` both plausibly `unknown` forever), when this story closes, then
  the epic records which capabilities are actually `supported` against a real repository, and
  **if no arming path is reachable, that is surfaced to the operator as a product limitation
  before S15 writes the maturity table** — never discovered at S18 when the live proof turns
  out to be unreachable.
  - Test: `docs/planning/github-addressing-model.md` (S00's predicate table, filled in)
  - Verify: manual — S15 is blocked until the table has real values
  - Level: L0

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
- **REQ-E10-S13-02** — Given the factory now lands in **S02** (not here — see that story's
  correction note), when forge selection is wired, then it selects *through* the existing
  factory and `task lint`'s depguard still denies `cmd/assent` importing either adapter package.
  - Test: `hack/lint/depguard_test.sh`
  - Verify: `task lint`
  - Level: L1

### E10-S14 — Conformance parity + catalog flip `[autonomous]`

- **Dependencies**: S13.
- **Definition of done**: the S01 suite runs against the GitHub factory in CI; every
  `github-deferred` row in `catalog.yaml` is either flipped to `both` or **retains the
  deferral with a named, cited reason**; D-084 is dispositioned.

- **REQ-E10-S14-01** — Given **every one of the 14 non-deferred catalog rows is
  `forge: gitlab`**, when the catalog is updated, then **every row** — not only
  `github-deferred` ones — carries an explicit per-adapter disposition (`both`, or a single
  forge **plus a cited reason**), and a test fails on any bare `forge: gitlab` row.
  *Corrected after adversarial review*: the first draft required reasons only for
  `github-deferred` rows, which would have shipped the GitHub adapter with **zero** of the
  trust-boundary cases proven — `TestRunForkContextAdvisoryOnly`,
  `TestRunPolicyFromTargetRefOnly`, `TestDoctorForgeInsecureCITopology`,
  `TestConformanceSpoofedMarkerIgnored`, `TestRunEnumerationIncompleteNeverApproves`
  (`exitgate_test.go:27-40`) — while S17 went green. Those are precisely the cases most likely
  to differ between forges (`pull_request_target`, `GITHUB_TOKEN` scope, `refs/pull/N/*`).
  - Test: `internal/forge/conformance/catalog.yaml`, `catalog_test.go`
  - Verify: `go test ./internal/forge/conformance/ -run TestEveryRowHasAdapterDisposition`
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
