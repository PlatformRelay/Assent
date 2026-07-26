# P4-E1 — Walking skeleton with trust boundaries from day one

**Problem**: Phase 3 froze the contracts (schemas + fixtures) but wrote no engine code
(D-016). P4-E1 is the **first** engine slice: the thinnest real end-to-end path — CLI in a
GitLab CI job → parse a one-field YAML change on a generated sample repo → evaluate one
`assert` rule proving one obligation → post one resolvable thread OR approve + SHA-pinned
merge → emit the `DecisionRecord` report. The purpose of doing it now, thin, is to prove
the **trust boundaries** in real code before the full engine (Phase 5, E1–E9) is built —
not to defer them. The four dangerous design gaps ADR-0015/0017 fixed (policy from the MR
branch, source-only SHA pin, self-vouching policy, over-privileged provider) must be
**exercised by the skeleton**, not left as Phase-5 promises.

**Scope**: the thinnest vertical slice through every layer — a CLI + CI-env adapter
(`cmd/assent`); a modify-only YAML differ (`internal/change`); minimal order-independent
obligations aggregation (`internal/core`); minimal Reconcile (one thread; approve + pinned
merge) against the ADR-0011 forge port; the `DecisionRecord` report artifact emitted against
the frozen `schemas/decision/v1alpha1/decision-record.schema.json`; `doctor` preconditions
that refuse to arm auto-merge without a protected pipeline; the three trust-boundary goldens
(`.assent/**` → `assent-policy` BLOCK, SHA-guard rejection on target/source movement,
provider-less run); reuse of the Spike-B e2e harness; the determinism double-run gate; the
rerun-idempotence gate against the frozen P3-E5 reconciliation fixtures; and the operator-run
L3 e2e + D-012 adoption gate.

**Non-goals** (fence each thin slice against its Phase-5 owner — thinness is the point):
- add/delete/rename diffs and the opt-in rename fold — **E1** (this epic: **modify-only** YAML).
- multi-obligation `require:` composition, points/threshold scoring, one-shot arming for the
  full fact catalogue — **E2** (this epic: **one** obligation proven by **one** `assert` rule).
- multi-format adapters (JSON/HCL-tfvars), classifier matcher-domain breadth, `EntryRef`
  derivation for map/list collections — **E1** (this epic: **one** YAML field, `document` mode).
- the full finding-lifecycle state machine, duplicate-repair at scale, `serve` keyed lock,
  multi-slot reconciliation — **E4/E12** (this epic: **one** slot; rerun-idempotence proven
  against the frozen P3-E5 fixtures with the in-memory fake).
- live-forge provider host, real fact resolution — **E7** (this epic: **provider-less** run —
  the one `assert` reads only `old`/`new`/`changes`, no fact leaves the process).
- authoring new schemas or ADRs — the frozen Phase-3 schemas and ADR-0015/0017/0019 are
  consumed as-is; this epic writes **engine code against them**, never the reverse.

ADRs: 0008 (`assent-policy` meta-class routing, reserved-class lint), 0011 (core ports:
differ, decision, Reconcile), 0013 (`assert`/CEL leaf, purity, double-run), 0015 (§1 policy
from target ref, §2 SHA-guard, §4 protected-pipeline `doctor` precondition, §7 provider runs
tokenless, §8 execution-authority matrix), 0016 (four-record split), 0017 (§1 merge-result
preconditions, §2 order-independent aggregation, §4 expiring-fact arming, §7 Reconcile port +
serialized-schema-as-API, §9 `doctor` typed report), 0019 (marker/reconciliation protocol);
D-007 (no database), D-012 (adoption gate — one real repo), D-016 (schemas frozen first),
D-017 (rerun idempotence). Consumes the frozen artifacts: `schemas/decision/v1alpha1/*`,
`docs/contracts/p3-e5-publication-protocol/fixtures/{rerun-idempotence,crash-then-rerun,duplicate-repair}.yaml`,
`examples/repos/`, `examples/contracts/d016-strict-fixture/`.

## Executability classification (autonomous vs infra-gated)

Each story is tagged for a coordinator deciding what an **autonomous coding session with no
live infra** can build and gate:

- **`[autonomous]`** — buildable and gate-able with unit/golden/property tests, a FAKE
  in-memory forge or recorded cassette, and the frozen schemas/fixtures. No live GitLab, no
  token, no real repo. All engine slices, the CLI/CI-env parsing, `doctor` precondition logic,
  the trust-boundary decision goldens, Reconcile against the in-memory fake, the determinism
  double-run gate, and rerun-idempotence against the frozen P3-E5 fixtures are autonomous.
- **`[infra-gated: needs live GitLab / real repo]`** — cannot be truthfully closed without a
  live (or containerized) GitLab, real tokens, or a real repository. **Only two stories are
  infra-gated**: S10 (L3 skeleton e2e green + replayable) and S11 (D-012 adoption gate). The
  coordinator should **park** these for the operator, not attempt them. S09 (e2e harness
  *authoring/wiring*) is split so the wiring is autonomous and only the green-run rolls into
  S10.

## Judgment calls fixed by this spec (logged to the operator INBOX as 🟡 DECIDED)

- **E2E CI profile**: the L3 skeleton reuses the Spike-B–chosen **testcontainer** profile as
  the CI default (backlog: "P2-E2 Done — CI default = testcontainer"; kind stays for
  local/demo). A testcontainer GitLab is still **live infra an autonomous session lacks**, so
  S10's green run is infra-gated even though it is not a hosted GitLab.
- **New engine package paths** (proposed, mirroring how P3-E1 proposed schema paths before
  they existed): `internal/change/` (differ), `internal/core/decision/` +
  `internal/core/aggregate/` (obligation aggregation → `DecisionRecord`),
  `internal/core/classify/` (minimal class routing incl. the `assent-policy` meta-class),
  `internal/forge/` and `internal/forge/fake/` (Reconcile port + in-memory fake, **outside**
  `internal/core`), `cmd/assent/` (CLI + CI-env adapter, **the only place env/CI vars are
  read**). Arch-lint keeps `internal/core`/`internal/change` pure and forge-free.
- **Trust-boundary goldens as one story with three adversarial REQs** (S07), rather than three
  stories — they share one golden harness and one decision-path assertion shape.
- **Determinism obligation**: every engine story states in its AC that `internal/core` /
  `internal/change` take **no** clock/random/env/network; freshness/`maxAge` arming uses an
  **injected clock** (ADR-0017 §4); aggregation is **order-independent** (ADR-0017 §2/§9);
  each golden **double-runs and diffs** (ADR-0013, GUIDELINES §5).

## Dependency order

```
S01 CLI + CI-env adapter ─────────────┐  (early/parallel; feeds EvaluationInput assembly)
S02 modify-only YAML differ ──────────┤
                                       ├─► S03 obligations aggregation ─► S04 DecisionRecord report
                                       │                                          │
S05 doctor preconditions ─────────────┘                                          │
                                                                                  ▼
                          S06 Reconcile: thread  ◄──────────────────────────────┤
                          S07 trust-boundary goldens ◄───────────────────────────┤
                          S08 Reconcile: approve + SHA-pinned merge ◄─────────────┘
                                          │
                                          ▼
                   S09 e2e harness wiring (autonomous) ─► S10 L3 e2e green [infra] ─► S11 D-012 [infra]
                   S12 determinism double-run + rerun-idempotence gate (autonomous, gates S03/S04/S06/S08)
```

**First slice: S02 (modify-only YAML differ).** It is the foundation both aggregation (S03)
and the report (S04) consume, it is pure (`internal/change`, easiest to gate at L0), and it
is the smallest thing that produces a real `ChangeSet` the rest of the skeleton is written
against — matching the epic's own story-seed order.

---

## P4-E1-S01 — CLI + CI-env adapter (EvaluationInput assembly) `[autonomous]`

**As a** platform operator running assent in a GitLab CI job **I want** the CLI to read the
CI environment and assemble a pinned `EvaluationInput` **so that** the engine receives
trusted, pinned inputs and never reads the environment itself.

- **Goal**: `cmd/assent` parses the GitLab CI environment (project, MR IID, source SHA,
  target SHA/ref) and CLI flags into a schema-valid `EvaluationInput`, loading `.assent/**`
  policy **from the target ref only** (ADR-0015 §1). All env/CI-var reads live in
  `cmd/assent`; `internal/core` and `internal/change` receive only pinned values (an injected
  clock, the pinned SHAs, the loaded policy), never the environment.
- **Operator input**: no.
- **Dependencies**: none (foundation adapter; consumes the frozen `evaluation-input.schema.json`).
- **Definition of done**: `cmd/assent` builds an `EvaluationInput` from a fixture CI
  environment that validates against `schemas/decision/v1alpha1/evaluation-input.schema.json`;
  policy is loaded from the target ref; an arch-lint/test asserts no `os.Getenv`/`os.Environ`
  in `internal/core` or `internal/change`.

Requirements:

- **REQ-P4-E1-S01-01** — Given a GitLab CI environment (`CI_PROJECT_ID`,
  `CI_MERGE_REQUEST_IID`, `CI_MERGE_REQUEST_SOURCE_BRANCH_SHA`,
  `CI_MERGE_REQUEST_TARGET_BRANCH_SHA`/ref), when `assent` runs, then `cmd/assent` assembles an
  `EvaluationInput` carrying those values as `mr` metadata and pinned SHAs, and it validates
  against `schemas/decision/v1alpha1/evaluation-input.schema.json`. The clock used for any
  freshness field is **injected** (a `--now`/interface seam), never `time.Now()` inside core.
  - Test: `cmd/assent/eval_input_test.go`
  - Verify: `go test ./cmd/assent/... -run TestAssembleEvaluationInput`
  - Level: L1
- **REQ-P4-E1-S01-02** — Given an MR whose branch edits a policy file under `.assent/`, when
  the CLI loads policy, then it reads `.assent/**` from the **target ref**, not the source
  branch (ADR-0015 §1) — a golden asserts the loaded policy bytes equal the target-ref
  version even when the source branch's version differs.
  - Test: `cmd/assent/policy_load_test.go`
  - Verify: `go test ./cmd/assent/... -run TestPolicyLoadsFromTargetRef`
  - Level: L1
- **REQ-P4-E1-S01-03** — Given the determinism hard rule (GUIDELINES §5), when the packages
  are built, then an arch/purity test asserts `internal/core/**` and `internal/change/**`
  reference no `os.Getenv`, `os.Environ`, `time.Now`, `rand`, or network package — env and
  clock enter only through `cmd/assent`. Adversarial case: a test that adds an `os.Getenv`
  call inside `internal/core` fails the purity check.
  - Test: `internal/core/purity_test.go`
  - Verify: `go test ./internal/... -run TestCorePurity`
  - Level: L0

## P4-E1-S02 — Modify-only YAML differ `[autonomous]`

**As a** rule author **I want** a one-field YAML modification to produce a canonical
`ChangeSet` **so that** an `assert` rule can see the old and new value of the changed field.

- **Goal**: a **pure** differ in `internal/change` that, for a single YAML document changed
  between base and head, emits a `ChangeSet` with the changed pointer, its old value, and its
  new value — **modify-only** (add/delete/rename fold is E1, explicitly out of scope). Opaque
  or unparseable input fails **closed** to REVIEW (GUIDELINES §2, ADR-0015 §9).
- **Operator input**: no.
- **Dependencies**: none — this is the foundation slice S03/S04 consume.
- **Definition of done**: `internal/change` differ produces a golden `ChangeSet` for a
  modify-only YAML fixture drawn from `examples/repos/`; the differ is a pure function (no
  clock/env/network); an unparseable/opaque fixture yields an opaque/REVIEW marker, never a
  silent empty diff; the golden double-runs and diffs identically.
- **Not in scope**: add/delete entries, rename fold, JSON/HCL adapters, `branch`-scope tree
  parsing — all **E1**.

Requirements:

- **REQ-P4-E1-S02-01** — Given a YAML document with exactly one scalar field changed between
  base and head (e.g. a `replicas: 3 → 5` edit from an `examples/repos/` sample), when the
  differ runs, then it emits a `ChangeSet` with one entry whose `path`/pointer targets the
  changed field, `old` = the base value, `new` = the head value, and no other entry — proven
  by a byte-stable golden.
  - Test: `internal/change/diff_test.go`
  - Verify: `go test ./internal/change/... -run TestModifyOnlyYAMLDiff`
  - Level: L0
- **REQ-P4-E1-S02-02** — Given the differ is in `internal/change`, when a golden runs, then it
  is executed **twice** and the two `ChangeSet` outputs are byte-identical (double-run gate,
  ADR-0013/GUIDELINES §5), and the function references no clock/random/env/network.
  - Test: `internal/change/diff_test.go`
  - Verify: `go test ./internal/change/... -run TestDiffDoubleRunStable`
  - Level: L0
- **REQ-P4-E1-S02-03** — Given an unparseable or opaque YAML head (billion-laughs alias
  expansion, or a byte sequence that is not valid YAML), when the differ runs, then it returns
  an **opaque** result (or error) that the caller maps to REVIEW — never a silent empty
  `ChangeSet` that could read as "nothing changed → safe" (fail-safe direction, GUIDELINES §2).
  - Test: `internal/change/diff_test.go`
  - Verify: `go test ./internal/change/... -run TestOpaqueInputFailsSafe`
  - Level: L0

## P4-E1-S03 — Minimal obligations aggregation `[autonomous]`

**As a** policy operator **I want** one `assert` rule proving one named obligation to
aggregate into an APPROVE/REVIEW/BLOCK decision **so that** an unsatisfied obligation never
auto-merges.

- **Goal**: a **pure**, order-independent aggregator in `internal/core` that evaluates one
  `assert`/CEL rule (`prove: {obligation, when}` with `onFailure: {effect, code}`, ADR-0017
  §2) over the S02 `ChangeSet` and reduces it to a decision. A binding's single required
  obligation, satisfied → APPROVE; unsatisfied → the rule's `onFailure` effect; a predicate
  **error** fails safe to REVIEW (tri-state, ADR-0017 §6). Provider-less: the `assert` reads
  only `old`/`new`/`changes`, no fact.
- **Operator input**: no.
- **Dependencies**: S02 (consumes the `ChangeSet`).
- **Definition of done**: given a one-obligation binding and the S02 `ChangeSet`, the
  aggregator returns APPROVE when the obligation is proven and the `onFailure` effect
  otherwise; aggregation is **order-independent** (shuffling rule/finding order yields an
  identical decision, ADR-0017 §9); a predicate error → REVIEW; the golden double-runs.
- **Not in scope**: multi-obligation AND composition, points/threshold scoring, `require-review`
  authorization, one-shot arming — all **E2**.

Requirements:

- **REQ-P4-E1-S03-01** — Given a binding with one `require: [<obligation>]` and one rule that
  `prove`s exactly that obligation with `when: <assert>`, when the assert is **true** over the
  `ChangeSet`, then the decision is `APPROVE`; when it is **false**, the decision reflects the
  rule's `onFailure.effect` (`comment`/`challenge`/`block`/`require-review`) and is **never**
  `APPROVE` (an unproven obligation cannot auto-merge, ADR-0017 §2).
  - Test: `internal/core/aggregate/aggregate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestOneObligationDecision`
  - Level: L0
- **REQ-P4-E1-S03-02** — Given a run where the `assert`/CEL leaf **errors** (e.g. references a
  field absent from the change), when aggregation runs, then the finding fails **safe** to
  REVIEW/BLOCK, never APPROVE (tri-state fail-safe, ADR-0017 §6, GUIDELINES §2).
  - Test: `internal/core/aggregate/aggregate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestPredicateErrorFailsSafe`
  - Level: L0
- **REQ-P4-E1-S03-03** — Given findings produced in arbitrary order, when the aggregator
  reduces them, then the resulting decision and `findings[]` set are **order-independent** —
  a shuffled input yields a byte-identical `DecisionRecord.findings` (after canonical sort),
  and the golden double-runs to the same bytes (ADR-0017 §9, ADR-0013).
  - Test: `internal/core/aggregate/aggregate_test.go`
  - Verify: `go test ./internal/core/aggregate/... -run TestAggregationOrderIndependent`
  - Level: L0

## P4-E1-S04 — DecisionRecord report artifact `[autonomous]`

**As an** auditor **I want** every run to emit a schema-valid `DecisionRecord` report with
its pins **so that** the decision is replayable and pinned to the exact commits evaluated.

- **Goal**: serialize the S03 decision into a `DecisionRecord` (+ redacted
  `PresentationModel`) that validates against the frozen
  `schemas/decision/v1alpha1/decision-record.schema.json`, with `pins` populated
  (`toolVersion`, `toolDigest`, `policySha`, `sourceSha`, `targetSha`, `mergeResultDigest`)
  and no raw sensitive fact value present (redacted by construction, ADR-0016 §3).
- **Operator input**: no.
- **Dependencies**: S03 (the decision), S01 (the pinned SHAs).
- **Definition of done**: a full skeleton run emits a `DecisionRecord` JSON that validates in
  the schemas CI; `pins` carries source + target + merge-result digests; the companion
  `PresentationModel` carries no raw fact value; a `decision: APPROVE` with a null
  `mergeResultDigest` and no `capabilityGap` marker is rejected (silent widening not
  representable, ADR-0017 §1).
- **Not in scope**: markdown rendering (`assent render`, ADR-0016 §4), `ReplayBundle`
  replay-execution (schema exists; hermetic *re-run* is E-later).

Requirements:

- **REQ-P4-E1-S04-01** — Given the S03 decision and the S01 pins, when the report is emitted,
  then the `DecisionRecord` validates against
  `schemas/decision/v1alpha1/decision-record.schema.json`, `decision ∈ {APPROVE, REVIEW,
  BLOCK}`, and `pins` requires `toolVersion`, `toolDigest`, `policySha`, `sourceSha`,
  `targetSha`, and `mergeResultDigest`.
  - Test: `internal/core/decision/record_test.go`
  - Verify: `go test ./internal/core/decision/... -run TestDecisionRecordValidates`
  - Level: L0
- **REQ-P4-E1-S04-02** — Given the redaction rule (ADR-0016 §3), when the companion
  `PresentationModel` is emitted, then it carries no raw fact `value` and no rendered markdown,
  and validates against `presentation-model.schema.json`. Adversarial case: a `DecisionRecord`
  claiming `APPROVE` with `mergeResultDigest: null` and no `capabilityGap` marker fails
  validation.
  - Test: `internal/core/decision/record_test.go`
  - Verify: `go test ./internal/core/decision/... -run TestApproveRequiresMergeResultPin`
  - Level: L0

## P4-E1-S05 — doctor preconditions (protected pipeline arming) `[autonomous]`

**As a** platform operator **I want** `assent doctor` to refuse to arm auto-merge unless the
pipeline is protected **so that** an author-editable CI job can never grant itself write
authority.

- **Goal**: implement the `doctor` **precondition logic** (ADR-0015 §4/§8, ADR-0017 §9): emit
  a typed capability/precondition report and, when the assent job cannot be verified to come
  from a **protected source**, refuse to arm auto-merge (advisory-only). The logic is testable
  against a **fake/injected environment description**, not a live pipeline.
- **Operator input**: no.
- **Dependencies**: S01 (shares the CI-env adapter that supplies the pipeline-source signal).
- **Definition of done**: given a fixture "protected-pipeline = true" env, `doctor` reports
  arm-eligible; given "protected-pipeline = unverifiable/false", `doctor` reports advisory-only
  and the run emits **no** approve/merge write; a typed precondition report is produced.
- **Not in scope**: the full doctor checklist, `duplicate_prevention` serialization reporting
  (P3-E5-S03 owns the field spec; wired in E4/E12), forge-capability probing of a live GitLab.

Requirements:

- **REQ-P4-E1-S05-01** — Given a fixture environment declaring the assent job comes from a
  protected/included config outside the MR branch's control, when `doctor` runs, then it
  reports the arming precondition **met** and the run is permitted to arm auto-merge
  (ADR-0015 §8 matrix: CI-from-protected-config → may auto-merge).
  - Test: `cmd/assent/doctor_test.go`
  - Verify: `go test ./cmd/assent/... -run TestDoctorProtectedPipelineArms`
  - Level: L1
- **REQ-P4-E1-S05-02** — Given a fixture environment where the protected-config precondition
  **cannot be verified** (or is author-editable), when `doctor` runs, then it reports
  advisory-only and the run performs **no** approve/merge write — `doctor` refuses to arm
  (ADR-0015 §4/§8). Adversarial case: an author-editable `.gitlab-ci.yml` topology with a
  privileged token is reported unsupported/insecure and never arms.
  - Test: `cmd/assent/doctor_test.go`
  - Verify: `go test ./cmd/assent/... -run TestDoctorRefusesArmWhenUnprotected`
  - Level: L1

## P4-E1-S06 — Minimal Reconcile: post one resolvable thread `[autonomous]`

**As a** reviewer **I want** a REVIEW decision to post one resolvable thread on the MR
**so that** the finding is visible and can be resolved — with no duplicate on rerun.

- **Goal**: implement the Reconcile port (ADR-0017 §7:
  `Reconcile(DesiredReviewState, Preconditions) -> PublicationReceipt`) for the **thread**
  case against an **in-memory fake forge** (`internal/forge/fake`), emitting a
  `PublicationReceipt`. Idempotent by the P3-E5 marker: a rerun posts **zero** new threads.
- **Operator input**: no.
- **Dependencies**: S04 (the DecisionRecord → DesiredReviewState).
- **Definition of done**: a REVIEW decision posts exactly one thread with the P3-E5 marker on
  the fake forge; a `PublicationReceipt` records it; a **rerun** against the same fake state
  creates zero new threads (matches `docs/contracts/p3-e5-publication-protocol/fixtures/rerun-idempotence.yaml`).
- **Not in scope**: real GitLab thread API (L3, S10), multi-slot reconciliation,
  duplicate-repair at scale, `serve` keyed lock — E4/E12.

Requirements:

- **REQ-P4-E1-S06-01** — Given a REVIEW `DecisionRecord`, when Reconcile runs against the fake
  forge, then exactly one resolvable thread is created carrying the P3-E5 `slot`/`occurrence`
  marker, and the returned `PublicationReceipt` records a `kind: thread` operation and
  validates against `schemas/decision/v1alpha1/publication-receipt.schema.json`.
  - Test: `internal/forge/reconcile_thread_test.go`
  - Verify: `go test ./internal/forge/... -run TestReconcilePostsOneThread`
  - Level: L1
- **REQ-P4-E1-S06-02** — Given the fake forge already holds the thread from a prior run (same
  slot/occurrence), when Reconcile runs again, then it creates **zero** new threads and leaves
  the existing one untouched — matching `fixtures/rerun-idempotence.yaml`. Adversarial case: a
  contributor (non-bot) comment carrying a well-formed marker is **excluded** by author-identity
  filter and has zero effect on the reconciliation (ADR-0019 / P3-E5-S01-04).
  - Test: `internal/forge/reconcile_thread_test.go`
  - Verify: `go test ./internal/forge/... -run TestReconcileThreadIdempotent`
  - Level: L1

## P4-E1-S07 — Trust-boundary decision goldens `[autonomous]`

**As a** security reviewer **I want** the three trust boundaries proven by golden decision
tests **so that** a regression that weakens them fails CI before it ships.

- **Goal**: three adversarial goldens exercising the trust boundaries in the skeleton's real
  decision path: (1) `.assent/**` MR → `assent-policy` meta-class **BLOCK** (ADR-0015 §1,
  ADR-0008); (2) SHA-guard **rejection** when the target or source SHA has moved since
  evaluation (merge-result precondition, ADR-0015 §2 / ADR-0017 §1); (3) **provider-less** run
  — the approve/merge token is never present in the evaluation path (ADR-0015 §7). Gated
  against the fake forge and in-memory pins — no live infra.
- **Operator input**: no.
- **Dependencies**: S03/S04 (decision path), S06/S08 (write path for the SHA-guard case).
- **Definition of done**: the three goldens exist, each double-runs, and each fails-closed as
  specified; a reserved-class routing golden proves an `.assent/**` MR cannot vouch itself.
- **Not in scope**: real-forge conformance of these boundaries (L3, S10) — the goldens prove
  the **engine** behaviour; S10 proves the **adapter** behaviour.

Requirements:

- **REQ-P4-E1-S07-01** — Given an MR that touches `.assent/**` (edits its own policy), when
  the classifier routes it, then it lands in the built-in `assent-policy` meta-class and the
  decision is **BLOCK** (never APPROVE/vouch), independent of what any rule predicate evaluates
  to — the mandatory "MR edits its own policy → BLOCK" golden (ADR-0015 §1). Adversarial case:
  a pack rule attempting to route `assent-policy` to `vouch`/obligation-satisfying is rejected
  (reserved-class, ADR-0008 amendment).
  - Test: `internal/core/classify/assent_policy_golden_test.go`
  - Verify: `go test ./internal/core/... -run TestAssentPolicyBlockGolden`
  - Level: L0
- **REQ-P4-E1-S07-02** — Given a decision pinned to `sourceSha`/`targetSha`, when a write
  (approve/merge) is attempted after the **target** (or source) SHA has moved, then the write
  **fails closed** (SHA-guard rejection / re-evaluation required) and no merge occurs — a
  source-only pin is insufficient (ADR-0017 §1, ADR-0015 §2). Adversarial case: advancing the
  target after evaluation yields rejection, not a silent merge of an unevaluated result.
  - Test: `internal/forge/sha_guard_test.go`
  - Verify: `go test ./internal/forge/... -run TestMergeFailsClosedOnMovedSHA`
  - Level: L1
- **REQ-P4-E1-S07-03** — Given ADR-0015 §7 (providers run tokenless), when the evaluation path
  runs provider-less, then a test asserts the approve/merge credential is **not** reachable
  from the evaluation/decision code path (no forge write token in the decision function's
  reachable inputs) — the one `assert` reads only `old`/`new`/`changes`. Adversarial case: a
  test that threads the write token into the decision inputs fails the boundary assertion.
  - Test: `internal/core/decision/tokenless_test.go`
  - Verify: `go test ./internal/core/... -run TestEvaluationIsProviderless`
  - Level: L0

## P4-E1-S08 — Minimal Reconcile: approve + SHA-pinned merge `[autonomous]`

**As a** platform operator **I want** an APPROVE decision to approve and SHA-pinned-merge the
MR **so that** only the exact evaluated commit is merged, and never a moved one.

- **Goal**: implement the Reconcile **approve + merge** path against the in-memory fake forge,
  carrying the evaluated head SHA (`Decision.Pins`) into a compare-and-swap merge (GitLab
  `merge?sha=` semantics, ADR-0015 §2). APPROVE arms the write only when the S05 precondition
  is met; on SHA drift it fails closed (re-uses S07-02).
- **Operator input**: no.
- **Dependencies**: S04 (APPROVE decision), S05 (arming precondition). (S07-02 is the golden
  proving this slice's SHA-guard rejection — S08 does not depend on the rest of S07.)
- **Definition of done**: an APPROVE decision on the fake forge produces one approval + one
  SHA-pinned merge, recorded in `PublicationReceipt`; a moved SHA fails the merge closed;
  APPROVE with the arming precondition unmet performs no write.
- **Not in scope**: real GitLab merge-train/queue integration (L3, S10), deferred/one-shot
  arming for expiring facts (E2, ADR-0017 §4).

Requirements:

- **REQ-P4-E1-S08-01** — Given an APPROVE `DecisionRecord` with the arming precondition met,
  when Reconcile runs, then the fake forge records one approval and one merge whose merge-result
  precondition honours **`pins.sourceSha` + `pins.targetSha` + `pins.mergeResultDigest`** (not
  source-only CAS — `merge?sha=` alone is insufficient, ADR-0017 §1), and the
  `PublicationReceipt` records `kind: approval` and `kind: merge` operations that validate
  against the schema. Target-movement rejection is proven by S07-02 (the SHA-guard golden).
  - Test: `internal/forge/reconcile_merge_test.go`
  - Verify: `go test ./internal/forge/... -run TestReconcileApprovesAndPinnedMerges`
  - Level: L1
- **REQ-P4-E1-S08-02** — Given an APPROVE decision but the S05 arming precondition **unmet**
  (unprotected pipeline), when Reconcile runs, then **no** approve/merge write occurs — the run
  degrades to advisory/report-only (ADR-0015 §8). Adversarial case: the fake forge records zero
  write operations when arming is refused.
  - Test: `internal/forge/reconcile_merge_test.go`
  - Verify: `go test ./internal/forge/... -run TestNoMergeWhenArmingRefused`
  - Level: L1

## P4-E1-S09 — E2E harness wiring (Spike-B reuse, no green-run) `[autonomous]`

**As a** maintainer **I want** the skeleton wired into the Spike-B e2e harness under the
`e2e` build tag **so that** the L3 run (S10) is one operator command away, and the wiring
itself compiles/vets on every PR.

- **Goal**: author the L3 skeleton e2e **test scaffold** under `test/e2e/` (build tag `e2e`)
  that reuses the Spike-B testcontainer boot (`hack/spikes/e2e/boot-testcontainer.sh`,
  `smoke.sh`) and seeds one `examples/repos/` sample — **without** requiring a live GitLab to
  compile/vet. The green run itself is S10.
- **Operator input**: no (wiring only; the green run needs infra).
- **Dependencies**: S06/S08 (the Reconcile flows the e2e exercises).
- **Definition of done**: `test/e2e/skeleton_test.go` (tag `e2e`) exists and **compiles + vets**
  under `go vet -tags e2e ./...` on every PR (GUIDELINES: CI scans all build tags); it
  references the testcontainer profile and an `examples/repos/` seed; it is **skipped** (not
  failed) when no GitLab endpoint is configured, so the autonomous PR gate stays green.
- **Not in scope**: actually booting GitLab and asserting a live thread/approval/merge — that
  is S10 (infra-gated).

Requirements:

- **REQ-P4-E1-S09-01** — Given the `e2e` build tag, when `go vet -tags e2e ./...` runs in CI,
  then `test/e2e/skeleton_test.go` compiles and vets clean, wiring the CLI → differ →
  aggregation → Reconcile flow against the Spike-B testcontainer profile and an
  `examples/repos/` seed.
  - Test: `test/e2e/skeleton_test.go`
  - Verify: `go vet -tags e2e ./test/e2e/...`
  - Level: L2
- **REQ-P4-E1-S09-02** — Given no GitLab endpoint is configured (autonomous session), when the
  e2e test runs without the tag or without infra, then it is **skipped** with a clear reason,
  never failed — the skeleton's unit/golden gate is independent of live infra. Adversarial
  case: running `task check` (no `e2e` tag) never invokes the containerized boot.
  - Test: `test/e2e/skeleton_test.go`
  - Verify: `go test ./test/e2e/... -run TestSkeletonE2E` (skips without `-tags e2e`)
  - Level: L2

## P4-E1-S10 — L3 skeleton e2e green + replayable `[infra-gated: needs live GitLab / real repo]`

**As a** maintainer **I want** the skeleton to run green against a live/containerized GitLab
and be replayable **so that** the adapter's real thread/approval/merge semantics are proven,
not mocked.

- **Goal**: run the S09 scaffold **green** on the Spike-B testcontainer GitLab — CLI in a real
  CI job parses the YAML change, evaluates the one rule, posts a resolvable thread OR
  approves + SHA-pinned merges, emits the `DecisionRecord`, and the run is **replayable** from
  the `ReplayBundle`. Includes the **live crash-then-rerun** producing zero duplicate
  comments/threads under the serialized (`resource_group`-per-MR) topology.
- **Operator input**: **yes** — needs a live/containerized GitLab, a project token, and the
  testcontainer runtime an autonomous session lacks. **Coordinator: park this for the operator.**
- **Dependencies**: S09 (wiring), and all autonomous engine stories S01–S08.
- **Definition of done**: `task e2e` (`go test -tags e2e ./test/e2e/...`) is green against the
  testcontainer profile; the run's `DecisionRecord`/`ReplayBundle` replay to the same decision;
  a live crash-then-rerun creates zero duplicate threads (matches the frozen
  `crash-then-rerun.yaml` behaviour on real threads).
- **Not in scope**: hosted-GitLab.com validation (D-012/S11 covers a real repo), GitHub parity
  (E10, Locked).

Requirements:

- **REQ-P4-E1-S10-01** — Given the testcontainer GitLab and a seeded `examples/repos/` MR with
  a one-field YAML change, when `task e2e` runs, then the skeleton posts one resolvable thread
  **or** approves + SHA-pinned merges, emits a schema-valid `DecisionRecord`, and the run is
  green under `//go:build e2e`.
  - Test: `test/e2e/skeleton_test.go`
  - Verify: `go test -tags e2e ./test/e2e/... -run TestSkeletonE2E` (requires live GitLab)
  - Level: L3
- **REQ-P4-E1-S10-02** — Given a run that crashes after posting a thread but before the
  post-publication rescan, when the pipeline re-runs under the `resource_group`-per-MR
  serialized topology, then the second run creates **zero** duplicate threads — the live analogue
  of `docs/contracts/p3-e5-publication-protocol/fixtures/crash-then-rerun.yaml`.
  - Test: `test/e2e/skeleton_test.go`
  - Verify: `go test -tags e2e ./test/e2e/... -run TestSkeletonCrashThenRerunNoDup`
  - Level: L3

## P4-E1-S11 — D-012 adoption gate: one real repo on live MRs `[infra-gated: needs live GitLab / real repo]`

**As the** project sponsor **I want** assent to have run on live MRs of **one real
repository** **so that** the walking-skeleton exit gate reflects reality, not a synthetic
fixture.

- **Goal**: satisfy the D-012 adoption gate — one **real** repository runs the skeleton on
  **live** MRs (a synthetic testcontainer fixture does **not** count), and the outcome is
  recorded in the decision log.
- **Operator input**: **yes** — a real repository, real MRs, and real credentials. **This is
  the human-in-the-loop exit gate; the coordinator must not attempt it.**
- **Dependencies**: S10 (the skeleton must be L3-green first).
- **Definition of done**: `docs/decisions/decisions.md` gains a dated entry recording that a
  named (or generically-attributed, per D-002 hygiene) real repository ran assent on at least
  one live MR, with the resulting `DecisionRecord`/`PublicationReceipt` retained as evidence;
  `openspec/specs/later-phases.md`'s P4-E1 paragraph gains an "adoption: satisfied" note.
- **Not in scope**: fleet rollout, the OQ-24 timed secure-setup run (P2-E4-NS, separate gate).

Requirements:

- **REQ-P4-E1-S11-01** — Given a real repository and a live MR, when assent runs on it, then a
  real thread/approval/merge action is performed on that MR and its `DecisionRecord` +
  `PublicationReceipt` are retained; `docs/decisions/decisions.md` records D-012 as satisfied
  with the date and evidence pointer. Adversarial case: a synthetic/testcontainer-only run is
  explicitly stated as **not** satisfying D-012.
  - Test: `docs/decisions/decisions.md`
  - Verify: `grep -qi "D-012" docs/decisions/decisions.md && grep -qi "adoption" docs/decisions/decisions.md`
  - Level: doc (operator-attested; the underlying run is L3/live)

## P4-E1-S12 — Determinism double-run + rerun-idempotence CI gate `[autonomous]`

**As a** maintainer **I want** a CI gate that double-runs the engine and proves rerun
idempotence against the frozen P3-E5 fixtures **so that** nondeterminism or duplicate
publication cannot silently merge.

- **Goal**: wire the **determinism double-run gate** (every engine golden runs twice and diffs,
  ADR-0006/0013) and the **rerun-idempotence gate** (Reconcile against the in-memory fake
  replays the frozen `rerun-idempotence.yaml`, `crash-then-rerun.yaml`, and `duplicate-repair.yaml`
  fixtures with zero duplicate creation and deterministic repair) into CI — **all autonomous**,
  against fakes/fixtures, no live infra.
- **Operator input**: no.
- **Dependencies**: S03/S04 (double-run subjects), S06/S08 (rerun subjects).
- **Definition of done**: a CI job double-runs the engine goldens and fails on any diff; the
  fake-forge Reconcile replays the three P3-E5 fixtures — a rerun and a crash-then-rerun create
  zero duplicates, and seeded pre-existing duplicates are repaired deterministically
  (lowest-forge-ID canonical, recorded in `PublicationReceipt.repairs`).
- **Not in scope**: the **live** crash-then-rerun (S10, infra-gated) — this story proves the
  same invariants against the fake, which is what an autonomous session can gate.

Requirements:

- **REQ-P4-E1-S12-01** — Given every engine golden (S02/S03/S04), when the determinism gate
  runs in CI, then each golden is executed **twice** and the outputs are byte-diffed; any
  difference fails the job (double-run gate, GUIDELINES §5). Adversarial case: injecting a
  map-iteration-order dependence into aggregation makes the double-run diff fail.
  - Test: `.github/workflows/*.yml` (determinism step) + `internal/core/aggregate/aggregate_test.go`
  - Verify: `go test ./internal/... -run TestDeterminismDoubleRun`
  - Level: L0
- **REQ-P4-E1-S12-02** — Given the frozen `rerun-idempotence.yaml` and `crash-then-rerun.yaml`
  fixtures, when the fake-forge Reconcile replays them, then a rerun and a crash-then-rerun each
  produce **zero** new comments/threads — the autonomous analogue of S10's live gate.
  - Test: `internal/forge/reconcile_idempotence_test.go`
  - Verify: `go test ./internal/forge/... -run TestReconcileReplaysP3E5Fixtures`
  - Level: L1
- **REQ-P4-E1-S12-03** — Given the frozen `duplicate-repair.yaml` fixture (two+ bot artifacts
  seeded on one slot), when Reconcile repairs them, then the **lowest-forge-ID** artifact is
  canonical, every other is resolved/removed, and `PublicationReceipt.repairs` records each
  repaired ID against the canonical — deterministic, not first-seen-wins by scan order
  (P3-E5-S03-01). Adversarial case: reversing the scan order yields the **same** canonical.
  - Test: `internal/forge/reconcile_idempotence_test.go`
  - Verify: `go test ./internal/forge/... -run TestDuplicateRepairDeterministic`
  - Level: L1
