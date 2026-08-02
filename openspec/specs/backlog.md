# assent backlog — phase → epic index

Spec-driven backlog per [meta-plan](../../docs/planning/meta-plan.md) and the rules in
[openspec/config.yaml](../config.yaml). Phases 1–2 carry full stories/REQs (one directory per
epic); Phases 3–5 are epic paragraphs in [later-phases.md](later-phases.md) and get full
stories during Phase 3 ("contracts first").

Design-phase epic IDs: `P1-En` / `P2-En`. Implementation epics (Phase 5): `E1..E13`.
REQ ID format: `REQ-<epic>-S<story>-<nn>` (e.g. `REQ-P1-E2-S01-01`).

## Phase 1 — Requirements harvest

| ID | Epic | Spec | Status | Needs operator | Gate contribution |
| --- | --- | --- | --- | --- | --- |
| P1-E1 | Sample corpus: generalized repos + OSS corpus (OQ-16, D-008) | [spec](p1-e1-sample-corpus/spec.md) | **Done** (3 samples + OSS corpus). Extra private shapes **deferred, kept in corpus plan** (D-029) | optional later (shapes 4–5) | fixtures for the archetype gate |
| P1-E2 | Rule-archetype inventory + success metric (OQ-25) | [spec](p1-e2-archetype-inventory/spec.md) | **Done** — **Phase-1 gate CLOSED** | yes (holdout adjudication still operator) | **the Phase-1 gate**: every archetype has example change + expected decision |
| P1-E3 | Forge behaviour dossier: GitLab + GitHub (OQ-7/18/23) | [spec](p1-e3-forge-dossier/spec.md) | **Done** | no | feeds ADR-0005/0017 acceptance |
| P1-E4 | Prior-art review | [spec](p1-e4-prior-art/spec.md) | **Done** | no | feeds ADR acceptance round |

**Phase-1 gate**: CLOSED — archetype inventory + per-archetype DRAFT fixtures on main.

## Phase 2 — Spikes & ADR firming

| ID | Epic | Spec | Status | Needs operator | Gate contribution |
| --- | --- | --- | --- | --- | --- |
| P2-E1 | Spike A — CEL: coercion, error UX, cost/purity, trace, activation model | [spec](p2-e1-spike-cel/spec.md) | **Done** | no | ADR-0013/0016 acceptance evidence |
| P2-E2 | Spike B — GitLab-in-kind vs testcontainer (OQ-6) | [spec](p2-e2-spike-e2e/spec.md) | **Done** (CI default = testcontainer) | no | ADR-0006 acceptance evidence |
| P2-E3 | Spike C — typed HTTP/exec provider contract + token isolation | [spec](p2-e3-spike-provider/spec.md) | **Done** | no | ADR-0004/0017 §6 acceptance evidence |
| P2-E4 | Secure-setup adoption spike (OQ-24) | [spec](p2-e4-spike-secure-setup/spec.md) | **Done** (topology only) | — | topology evidence for ADR-0017 §9 |
| **P2-E4-NS** | **OQ-24 north-star timed clean-room run** | [spike-secure-setup.md](../../docs/planning/spikes/spike-secure-setup.md) § North-star | **OPEN — operator** (stopwatch walkthrough; &lt;1h unconfirmed) | **yes** | north-star wording HOLDS/AMEND; do not claim &lt;1h until done |
| P2-E5 | ADR acceptance round (0002–0017 → Accepted/Superseded) | [spec](p2-e5-adr-acceptance/spec.md) | **Done** — **Phase-2 gate CLOSED** (D-020) | yes (ratify 🔴 DECIDED INBOX) | **the Phase-2 gate** |
| P2-E6 | Spike D — Kubernetes CRD/CR validation feasibility (D-017 B11) | [spec](p2-e6-spike-crd/spec.md) | Deferred to the Phase-3 window (D-018 — not first-wave); does **not** gate P2-E5 | no | feeds ADR-0020 + the E14 go/no-go |

**Phase-2 gate**: CLOSED — ADR-0002..0017 Accepted (D-020); evidence in
[adr-acceptance-review.md](../../docs/planning/adr-acceptance-review.md).

## Phase-3 freeze residuals (open)

| ID | Item | Status | Needs operator | Notes |
| --- | --- | --- | --- | --- |
| **P3-P1-3** | Stock Draft 2020-12 validator in schemas CI (roast P1-3) | **OPEN** (D-027 → Option A) | no (agent lane) | Prove structure/`$ref` outside Go compiler; `x-uniqueKeys` uniqueness follow-up separate if still invisible to stock tools |
| **P3-OQ1** | Replace `assent.dev` apiVersion/`$id` group | **OPEN** (D-028/D-031 — path A: own a domain; **exact domain TBD**) | **yes — name the domain** | Then rename lane across consts/`$id`s/docs/fixtures |
| P3-ADR-freeze | Accept ADR-0018 + ADR-0019 (Proposed → Accepted) | **DONE** (D-030) | — | Phase-3 freeze ADRs Accepted |

## Phase 4 — P4-E1 walking-skeleton stories

Full INVEST stories in [p4-e1-walking-skeleton/spec.md](p4-e1-walking-skeleton/spec.md).
The first engine code (D-016 lifts): thinnest real slice, trust boundaries exercised, not
deferred. REQ IDs `REQ-P4-E1-S0n-nn`. **Execution** tags what an autonomous coding session
(no live infra) can build+gate versus what needs a live GitLab / real repo.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| P4-E1-S01 | CLI + CI-env adapter (EvaluationInput assembly; env read only in `cmd/assent`) | **[autonomous]** | none | feeds every slice; target-ref policy load |
| P4-E1-S02 | Modify-only YAML differ (pure `internal/change`; opaque → REVIEW) | **[autonomous]** | none | foundation ChangeSet; **do first** |
| P4-E1-S03 | Minimal obligations aggregation (one obligation, order-independent, fail-safe) | **[autonomous]** | S02 | APPROVE/REVIEW/BLOCK core |
| P4-E1-S04 | DecisionRecord report artifact (schema-valid, pinned, redacted) | **[autonomous]** | S03, S01 | the report the exit gate emits |
| P4-E1-S05 | doctor preconditions (protected-pipeline arming refusal) | **[autonomous]** | S01 | ADR-0015 §4/§8 arming guard |
| P4-E1-S06 | Minimal Reconcile: post one resolvable thread (fake forge, idempotent) | **[autonomous]** | S04 | REVIEW publication path |
| P4-E1-S07 | Trust-boundary goldens (`.assent/**` BLOCK · SHA-guard · provider-less) | **[autonomous]** | S03/S04/S06/S08 | the three boundaries in real code |
| P4-E1-S08 | Minimal Reconcile: approve + SHA-pinned merge (fake forge) | **[autonomous]** | S04/S05 | APPROVE publication path |
| P4-E1-S09 | E2E harness wiring (Spike-B reuse; compiles+vets, no green-run) | **[autonomous]** | S06/S08 | makes S10 one operator command away |
| P4-E1-S10 | L3 skeleton e2e green + replayable (+ live crash-then-rerun) | **[infra-gated: needs live GitLab / real repo]** | S09, S01–S08 | **exit gate**: L3 green + replayable |
| P4-E1-S11 | D-012 adoption gate: one real repo on live MRs | **[infra-gated: needs live GitLab / real repo]** | S10 | **exit gate**: D-012 (synthetic doesn't count) |
| P4-E1-S12 | Determinism double-run + rerun-idempotence CI gate (fakes/frozen fixtures) | **[autonomous]** | S03/S04/S06/S08 | **exit gate**: determinism + rerun idempotence (autonomous half) |

**Dependency order** (autonomous): S02 differ → S03 aggregation → S04 report → {S05 doctor,
S06 thread, S07 goldens, S08 approve+merge} → S12 determinism/rerun gate; S01 CLI+CI adapter
runs early/parallel. **Infra-gated (park for operator)**: S10 (L3 e2e green) then S11 (D-012).
**Do first: S02** — the pure differ both aggregation and the report consume.



## Phase-4 operator/infra residuals

| ID | Item | Status | Needs operator | Notes |
| --- | --- | --- | --- | --- |
| **P4-KIND-LAB** | Durable local kind GitLab lab (`task kind-up`, etc.) | **OPEN — authorized, deferred** (D-038) | no (agent lane when claimed) | Promote Spike-B `boot-kind.sh`; CI stays testcontainer |
| **P4-CODEQL** | Enable CodeQL default setup (Go + Actions) | **OPEN** — found via cross-repo CodeQL/SonarQube sweep (2026-07-29) | no (Settings → Code security → Default, or `gh api --method PUT repos/PlatformRelay/assent/code-scanning/default-setup`) | `state: not-configured` today despite `schemas.yml`/`verify.yaml` CI; zero-cost enablement, no SonarQube config exists either (not proposing one) |

## Phase 5 — E1 canonical change model stories

Full INVEST stories in [p5-e1-canonical-change-model/spec.md](p5-e1-canonical-change-model/spec.md).
E1 extends the P4-E1-shipped modify-only YAML differ (`internal/change/diff.go`) and minimal
`assent-policy` classifier (`internal/core/classify/classify.go`) to the full canonical change
model config.yaml describes: add/delete/rename diffs, JSON + HCL/tfvars adapters over one
canonical value tree, `EntryRef` derivation for map/list collections, three of the four
ADR-0017 §5 matcher domains plus a new `entryEvents` domain (the real whole-file `fileEvents` is
explicitly deferred — see the spec's Non-goals), input resource limits, and closing the D-042
"S10 review F1" live-adapter gap
(`assent run` enumerating the MR's full changed-file set instead of one hardcoded governed
subject). REQ IDs `REQ-E1-S0n-nn`. **Every story is `[autonomous]`** — pure `internal/change`
(+ `internal/core/classify`) engine code plus the `cmd/assent` adapter, gated against fixtures
already in `examples/repos/` and injected/fake changed-file lists, no live infra (contrast with
P4-E1's S10/S11).

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E1-S01 | Add/delete diffs + source positions on every Change (extends the shipped YAML differ) | **[autonomous]** | none | foundation `Kind`s + positions; **do first** |
| E1-S02 | Opt-in rename fold (delete+add → rename, default raw, never laxer than delete) | **[autonomous]** | S01 | ADR-0003 amendment rename semantics |
| E1-S03 | Canonical value tree + JSON format adapter | **[autonomous]** | S01 | format-agnostic differ; second-format proof |
| E1-S04 | HCL/tfvars format adapter (literal-only) | **[autonomous]** | S03 | third-format proof over the shared tree |
| E1-S05 | `EntryRef` derivation for map/list collections (identity-keyed, unkeyed lists rejected) | **[autonomous]** | S01, S03 | stable per-entry subjects (ADR-0017 §5) |
| E1-S06 | Classifier matcher-domain breadth (`files`/`values.pointers`/`entryEvents`/`valueChanges`) | **[autonomous]** | S01, S02, S05 | ADR-0017 §5 matcher vocabulary (`entryEvents` in place of true `fileEvents`, deferred); preserves `assent-policy` dominance |
| E1-S07 | Input resource limits (size/depth/entry-count/alias-expansion ceilings, fail-closed) | **[autonomous]** | S03, S04 | ADR-0003 Amendment 2 limits, generalized across formats |
| E1-S08 | `cmd/assent`: enumerate the MR's full changed-file set (closes D-042 "S10 review F1") | **[autonomous]** | none | live-adapter self-vouch (`.assent/**` BLOCK) proof, complementing the P4-E1-S07 engine golden |

**Dependency order**: S01 (add/delete + positions) → {S02 rename fold, S03 value tree + JSON
adapter → S04 HCL adapter, S05 EntryRef (also needs S03), S07 limits (also needs S04)}; S06
matcher domains needs S01+S02+S05. **S08 is independent and startable from day one** — it closes an
already-logged live security gap (D-042) and does not depend on any other E1 story landing
first. **Do first: S01** — the smallest pure extension of already-shipped code, and the
dependency root for rename fold, the value tree, EntryRef derivation, and matcher domains.


## Phase 5 — E2 decision engine + CEL predicate backend stories

Full INVEST stories in [p5-e2-decision-engine/spec.md](p5-e2-decision-engine/spec.md).
E2 grows the P4-E1 walking-skeleton evaluator (`internal/core/aggregate/aggregate.go` — one CEL
string `when`, `old`/`new`/`changes`-only env, toy `cmd/assent/policy.go` YAML, hardcoded
`Points: 0`, no threshold, no `require-review` path, no phase) into the full decision engine of
ADR-0007 (effects + aggregation + risk points), ADR-0013 (CEL `all`/`any`/`not` backend),
ADR-0017 (obligations, `require-review`, arming, tri-state fail-safe), and ADR-0018 (phase/profile
lifecycle), **re-seated on the frozen P3 schemas**, so it *reproduces* the strict D-016 §8
exit-gate DecisionRecord (not just validates its shape). REQ IDs `REQ-E2-S0n-nn`. **Every story is
`[autonomous]`** — pure `internal/core` engine (+ the `cmd/assent` loader/run re-seat), gated
against `schemas/` + `examples/contracts/` fixtures and in-memory injected inputs
(`EvaluationInput`, `ApprovalEvidence`), no live forge/provider/token (contrast with P4-E1's
S10/S11). E2 is sequenced **before E7** (decide-and-log): its exit gate is pure-engine and needs
no live infra, and realized value of E1's primitives waits on a working engine, not on infra —
E7 remains the next epic to claim after E2.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E2-F | Fixture-fix (two corrections) to d016 `partitions-must-not-shrink`: (1) `when` `input.new>=input.old` → `new>=old` (out-of-scope `input` vs ADR-0013/predicate-scope); (2) add missing `points: 10` (golden shows 10 but rule authors none; S06 has no engine default) | **[autonomous]** `🔴 DECIDED` (edits P3-frozen fixture) | none | unblocks a clean S10 reproduction; **land early (before/with S02)** |
| E2-S01 | Frozen-contract policy loader (`MergePolicy`/`RulesetBinding`/`Config`/`Pack`, strict decode via reused frozen schemas; assertTree decoded structurally, not CEL-compiled; self-contained, no `aggregate` import) | **[autonomous]** | none | retires engine↔contract drift; **do first** |
| E2-S02 | Evaluator re-seated on `EvaluationInput` + full frozen predicate scope (single-leaf `when`); **retires toy `cmd/assent/policy.go` + re-seats run.go** (moved from S01 — needs `EvaluationInput`'s per-change subjects) | **[autonomous]** | S01 (+F) | real activation model; closes numeric-coercion risk |
| E2-S03 | `all`/`any`/`not` combinator walker + per-leaf message | **[autonomous]** | S02 | ADR-0013 tree backend (**off S10 critical path**) |
| E2-S04 | Multi-obligation AND coverage across subjects (no `anyOf`) | **[autonomous]** | S02 | ADR-0017 §2 obligation coverage |
| E2-S05 | Fact tri-state fail-safe (`unavailable`/`invalid`/`expired` never APPROVE) | **[autonomous]** | S02 | ADR-0007 F6 / §4 arming precondition (decision-side) |
| E2-S06 | Points per firing + per-binding risk threshold (author-declared `rule.points`, no engine default) | **[autonomous]** | S04 | ADR-0007 aggregation tail; retires `Points: 0` |
| E2-S07 | `require-review` via injected `ApprovalEvidence` (sha-matched, eligible, capability-gated) | **[autonomous]** | S04 | ADR-0017 §3; closes staleness fail-open |
| E2-S08 | Phase `off`/`observe`/`enforce` + pack ceiling (threads `record.go:209`) | **[autonomous]** | S04 | ADR-0018 §1 lifecycle |
| E2-S09 | Profile resolution + single-writer authority | **[autonomous]** | S08 | ADR-0018 §2 (**off S10 critical path**) |
| E2-S10 | D-016 strict-fixture end-to-end DecisionRecord reproduction | **[autonomous]** | S01,S02,S04,S05,S06,S07,S08 (+F) | closes the contracts↔engine loop (§8 exit gate) |

**Dependency order**: S01 (loader) → S02 (evaluator re-seat) → {S03 tree walker, S04 obligation
coverage, S05 fact tri-state}; S04 → {S06 points/threshold, S07 require-review, S08 phase}; S08 →
S09 profiles. **S10 reproduces the frozen D-016 DecisionRecord** and depends on the
fixture-exercised stories (S01,S02,S04,S05,S06,S07,S08 + the E2-F fixture-fix) — **not** S03
(fixture `when`s are single-leaf) or S09 (no profile declared), which are validated by their own
goldens. **Do first: S01** (smallest independently-valuable slice, retires the toy loader every
later story consumes) alongside **E2-F** (a one-line fixture correction, startable day one).


## Phases 3–5

Epic paragraphs (goal, ADR constraints, exit gate, story seeds) in
[later-phases.md](later-phases.md). Summary:

| Phase | Epics | Gate |
| --- | --- | --- |
| 3 — Contracts first | P3-E1 schemas + contract fixture (incl. ApprovalEvidence + named-consumer fixture) · P3-E2 versioning/compat spec · P3-E3 example migration · P3-E4 lifecycle: phase/profiles/comparison (ADR-0018) · P3-E5 publication reconciliation protocol (ADR-0019) | strict end-to-end contract fixture validates (ADR-0017 §8, D-016); new ADRs 0018/0019 accepted at the freeze review |
| 4 — Walking skeleton | P4-E1 (+ rerun-idempotence gate, D-017) · **P2-E4-NS (OQ-24 timed run)** · holdout adjudication (OQ-25) | L3 skeleton green + **one real repo on live MRs** (D-012); north-star wording only after timed run |
| 5 — Implementation | E1–E9 active — **E1 has full INVEST stories**: [p5-e1-canonical-change-model/spec.md](p5-e1-canonical-change-model/spec.md); E11/E12 **unlocked** (D-017, post-Phase-4); E14 gated on Spike D; E10/E13 **locked** (D-012) | per-epic; E7 starts alongside E1 |

Named-consumer disposition (what unlocked, what stayed locked, and why):
[docs/planning/named-consumer-compat.md](../../docs/planning/named-consumer-compat.md).

## Reading order

1. [docs/vision.md](../../docs/vision.md) → [meta-plan](../../docs/planning/meta-plan.md)
2. ADR-0017 (contract model — newest, reshapes 0003/0005/0007/0009/0010/0011/0014/0015),
   then ADR-0013, 0014, 0015, 0016
3. [open-questions.md](../../docs/planning/open-questions.md) +
   [decisions.md](../../docs/decisions/decisions.md) (D-010, D-012, D-016)
4. This index → Phase-1 epic specs → Phase-2 epic specs → [later-phases.md](later-phases.md)
