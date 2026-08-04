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
| **P4-CODEQL** | Enable CodeQL (Go + Actions) | **DONE** (2026-08-03, D-045) — `.github/workflows/codeql.yaml` | no | Pinned, workflow-based CodeQL with a `go`(manual build)+`actions`(build-mode none) matrix, consistent with sibling repos (MKurator/Kollect); chosen over the zero-config default setup for SHA-pinned reproducibility |
| **P4-SEC-OSSF** | OpenSSF/security hardening (SECURITY.md, CODEOWNERS, Scorecard, scheduled govulncheck) | **DONE** (2026-08-03, D-045) | no | `SECURITY.md` + `.github/CODEOWNERS` + `scorecard.yaml` + schedule-only `vulncheck.yaml`; modeled on MKurator/Kollect. Residual: turn on branch protection + required checks on `main` (operator) |

## Code-health / SonarCloud maintainability residuals

**Snapshot (SonarCloud `PlatformRelay_assent`, live query 2026-08-03):** Quality Gate = **OK
(green)** on new-code conditions — **0 bugs, 0 vulnerabilities, 0 unreviewed hotspots**. The
`new_security_rating`=A gate cleared after the 2026-08-03 S6505 fix + S4036 accept (INBOX). All
**115 open issues are `CODE_SMELL` / maintainability on old code** (outside the leak window →
**non-gating, nothing is red or blocked**). Remediation below is opportunistic hygiene, not a
release blocker; sequence it *behind* E2 engine work. Lanes are cut by rule-family + blast radius
so each is an independently reviewable `[autonomous]` slice. **Invariant for every lane: behaviour
must not change** — `task check` green and each touched CI script re-run to identical output.

| ID | Item | Rules (count) | Blast radius | Status | Needs operator |
| --- | --- | --- | --- | --- | --- |
| **SONAR-SHELL** | Shell-script hygiene in `hack/*.sh` (`validate-schemas-stock.sh` 19 · `check-sanitization.sh` 18 · `check-migration-invariants.sh` 13 · `spikes/e2e/smoke.sh` 4) | `shelldre:S7688` use `[[` not `[` (34) · `S7679` assign positional param to local (9) · `S7682` explicit `return` (5) · `S131` add default `*)` case (3) · `S1192` de-dup literal (2) · `S7684` lower_case var (1) = **54** | **these are CI-gate scripts** — mechanical but must stay byte-behaviour-identical; re-run each script as the gate | **OPEN** `[autonomous]` | no |
| **SONAR-GO-CX-PROD** | Cognitive-complexity refactor of **production** Go (`internal/change/{diff,diff_hcl,limits,entryref}.go` · `cmd/assent/run.go` · `internal/forge/gitlab/gitlab.go` · `internal/core/classify/*` · `internal/core/hash/hash.go` · `hack/spikes/cel/*`) | `go:S3776` cognitive complexity > 15 (~12) | engine/determinism paths — extract helpers, **no behaviour change**; `internal/core` purity guards stay green | **OPEN** `[autonomous]` | no |
| **SONAR-GO-CX-TEST** | ~34 `go:S3776` hits in `*_test.go` (table-driven suites legitimately long by line-count, low by risk) | `go:S3776` (~34) | tests only | **OPEN — 🔴 decision** — recommend *exempt `**/*_test.go` from S3776* over churning 34 test files (see config note) | **yes — pick fix vs. exempt** |
| **SONAR-GO-MISC** | Small Go smells: `godre:S8184` comment blank imports (4) · `go:S1186` comment empty funcs (4) · `godre:S8196` single-method interface naming (2) · `go:S1135` resolve TODOs (2) · `godre:S8205` extract nested anon struct (1) · `go:S107` 8-param func (1) · `go:S1192` const for `"sha256:"` ×4 (1) = **15** | trivial, isolated | **OPEN** `[autonomous]` | no |

**Config note (blocks the exempt option in SONAR-GO-CX-TEST + any per-rule tuning):** the project
runs SonarCloud **Automatic Analysis** (GitHub App, no config file). Path/rule exclusions such as
`sonar.issue.ignore.multicriteria` (to exempt `**/*_test.go` from `S3776`) require a checked-in
`sonar-project.properties` **and** switching to CI-based analysis (Automatic Analysis and a config
file are mutually exclusive) — a real tradeoff (extra CI step + `SONAR_TOKEN` in Actions vs. the
current zero-config setup). Cutting the exclusion file is therefore itself an operator decision,
not an autonomous lane. If the exempt path is rejected, SONAR-GO-CX-TEST folds into
SONAR-GO-CX-PROD as subtest-extraction work.

**Suggested order:** SONAR-SHELL (biggest count, lowest risk, fully mechanical) → SONAR-GO-MISC
(trivial) → SONAR-GO-CX-PROD (real refactor value) → SONAR-GO-CX-TEST (only after the config
decision). None gate a release; all are startable once the operator wants to spend cycles on
hygiene rather than E2 feature work.

## Reference-derived coverage findings & example candidates

A read-only analysis (2026-08-03) of four real self-service repo shapes (provided as gitignored
`references/`, third-party IP — never committed; only generalized equivalents enter the tree, D-002)
assessed whether assent can gate their MRs and mined generic patterns. **Verdict: assent already
gates ~70–85% of those MRs at the REVIEW/BLOCK tier today** (same "needs a human" outcome the teams
make by hand, but deterministic + explained); the engine's shape fits — **the missing pieces are fact
providers, not core-model redesign**. All rows below are generalized (invented names, synthetic data).

**Fact-provider / model gaps (ranked by MR-weight):**

| ID | Gap | Status | Notes |
| --- | --- | --- | --- |
| **REF-GAP-1** | Referenced-resource authorization fact source (a list value / ACL names *another* team's resource → who owns it?) | **CLOSED (E5-S08)** | `builtin/resource-owner` shipped; hermetic L0 + run-path wiring in E5-S10. Demonstrator fixture = C7 (deferred — D-071) |
| **REF-GAP-2** | In-repo-state-as-a-fact (quota/placement/limits registries + in-repo reviewers files that today no provider reads) | **CLOSED (E5-S07)** | `builtin/repo-file` most-specific-first shipped; hermetic run path in E5-S10 (`TestE5ExitGateResolvedFacts`). C5/C6 fixtures deferred — D-071 |
| **REF-GAP-3** | Cross-class / companion-file correlation ("two-step delete": remove from file A *and* append to manifest B) | **OPEN — likely out of v1** | `changes` is class-slice-scoped by contract (ADR-0017 §5); ship C8 as a known-limitation fixture (expected REVIEW), decide scope via OQ |
| **REF-GAP-4** | Plan-level blast radius (weighting the expanded IaC plan, not the request diff) | **OUT of model** | assent gates the request diff; `points`/`threshold` bulk-guard on the diff is the in-scope approximation |

**Generalized example/test candidates (a later sanitized authoring lane — passes `check-sanitization.sh`):**

| ID | Item | Status | Closest existing archetype |
| --- | --- | --- | --- |
| **REF-EX** | Author 8 domain-neutral archetype fixtures C1–C8 (list-no-shrink, privilege-tier allow-list, wildcard-grant block, soft-delete-as-field-add, quota-ceiling-from-fact, placement allow-list, referenced-resource-ownership [gap demo], companion-file delete [known-limitation]) | **OPEN** (agent lane; do AFTER the E2 engine + E5 facts for C5/C6/C7) | extends no-destruction (C1/C4/C8), allowed-fields+ownership (C2/C3/C6/C7), bounded-change (C5); none duplicates an existing fixture |

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


## Phase 5 — E3 policy surface (`assent lint` + rule catalogue) stories

Full INVEST stories in [p5-e3-policy-surface/spec.md](p5-e3-policy-surface/spec.md). E3's loader / CEL
backend / tree walker / interpolation seeds are **already delivered by E2** — E3 is the human-facing
**`assent lint`** pass (8 hard errors over the `.assent/**` authoring surface, caught statically
BEFORE any MR) + the **generated rule catalogue** (D-017 B10). Pure Go/CLI, no live infra. Closes the
items E2 lanes deferred "to E3 lint" (S04 → S03 message-scope; S03 → S05 non-dot-facts + the
fact-model `.value` decision; S05 → widen the fail-open scan). Reuses `policy.ValidateProviderPosture`,
`aggregate.ResolveProfile`, the E2 CEL compile path; adds `cmd/assent/{lint,catalogue}.go`,
`internal/lint/**`, `internal/catalogue/**`. **Every story `[autonomous]`.**

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E3-S01 | `assent lint` scaffold + tolerant fail-many ingestion + obligation-coverage hard error (anchor) | **[autonomous]** `🟡 decide: subcommand + tolerant ingestion` | E2-S01, E2-S04 | the lint diagnostic accumulator every check plugs into; **do first** |
| E3-S02 | Structural hard errors: reserved-class, no-implicit-enforce-phase, unkeyed-lists | **[autonomous]** | E3-S01 | ADR-0015 §1 / 0018 §1 / 0017 §5 static catches |
| E3-S03 | Fact-model `.value` DECISION + non-dot facts-reference lint (closes S05 non-dot) | **[autonomous]** `🔴 DECIDED (fact-model D-nnn)` | E3-S01 | makes the S05 controlling-fact scan sound; precedes S04 |
| E3-S04 | Predicate-scope + `{{ }}` message-template lint (closes S03 message-scope) | **[autonomous]** | E3-S01, E3-S03 | ADR-0016 §2 undeclared-identifier catch; new exported compile helper in `aggregate` |
| E3-S05 | Config-posture: fail-open (WIDENED to 3 controlling archetypes) + single-writer-profile | **[autonomous]** | E3-S01, E3-S03 | ADR-0017 §6 / 0018 §2; reuses ValidateProviderPosture + ResolveProfile |
| E3-S06 | Tests-per-rule hard error (static presence, NOT the `assent test` runner) | **[autonomous]** | E3-S01 | ADR-0010/0014 |
| E3-S07 | Generated rule catalogue (D-017 B10) — additive-tolerant, single source for docs+lint | **[autonomous]** `🟡 decide: catalogue surface` | E2-S01 | parallelizable; `assent catalogue` |
| E3-C | Pack-conformance lane: add required `phase` + apply S03 fact-model decision to `examples/packs/**` (all 11 rule files omit `phase` today) | **[autonomous]** | E3-S03 | E3's analog to E2 lane F; land before S08 |
| E3-S08 | Exit gate: hard-error fixture corpus + archetype packs load+evaluate (internal `Cover`) + catalogue generates | **[autonomous]** | E3-S01..S07 + C | **the E3 exit gate** |

**Dependency order**: S01 → {S02, S05, S06}; S03 → S04; S07 ∥ (loader-only); C after S03, before S08; S08 last. **Do first: S01.**

> **E2 + E3 status: DONE** (main tip `5893df1`, CI green) — the two core Phase-5 epics (decision engine + policy-authoring lint) are complete.

## Phase 5 — E6 adopter test harness (`assent test` + `assent compare` seed) stories

Full INVEST stories in [p5-e6-adopter-test/spec.md](p5-e6-adopter-test/spec.md). E6 turns the FROZEN
ADR-0014 fixture format (schema-frozen at `schemas/testfixture/v1alpha1/test-expectation.schema.json` —
**reused as the strict-decode authority, no new schema**) into a runnable, dogfooded harness: `assent
test` builds an `EvaluationInput` from a case's `base/`↔`head/`+`facts.yaml` and evaluates the pack via the
E2 engine, so the example packs gate **themselves** in CI. **Closes the E3-S08 full-replay deferral** (S01
= the facts→resolved-envelope half; S02 = the entry-binding half). Reuses `change.Diff`/`DiffEntries`,
`aggregate.Cover*`, `buildEvaluationInput`, `loadCatalogueInput`; adds `cmd/assent/{test,compare}.go`,
`internal/adoptertest/**`, `internal/compare/**`. **Every story `[autonomous]`.**

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E6-S01 | `assent test` scaffold + directory-case loader + facts→resolved-envelope + single-rule `Cover` decision (anchor, INPUT-SIDE ONLY, no `internal/core` touch) | **[autonomous]** | E1, E2 | the `assent test` pipeline every later story plugs into; **do first** |
| E6-S02 | ⚠️ **DECISION-PATH lane** (D-053): `bindLeafActivation` per-EntryRef entry-object binding (Part A engine, fail-safe fallback) + entry-tree/mr/approval assembler (Part B harness) — **closes the E3-S08 full-replay deferral** | **[autonomous · engine-grade review]** `🔴 DECIDED D-053` | E6-S01 | Part A reviewed as an ENGINE change pointed at fail-safety, AHEAD of Part B |
| E6-S03 | Expectation matcher: findings must-contain / `exact` / `absent` / `score` / `message~`, fail-closed (`path`=error-as-unsupported, D-054) | **[autonomous]** | E6-S01, E6-S02 | which-rule-fired-with-which-effect assertions |
| E6-S04 | Failure UX: expected/actual diff + ready-to-copy actual block (ADR-0012) | **[autonomous]** | E6-S03 | the review-by-diff surface |
| E6-S05 | `--update` golden-refresh with comment-preserving write + overwrite safety | **[autonomous]** | E6-S03 | cheap golden maintenance |
| E6-S06 | Inline `cases.yaml` shorthand (alternate front-end onto the S01/S02 pipeline) | **[autonomous]** | E6-S01 | compact one-field cases |
| E6-S07 | `--coverage` per-rule both-polarity (every-rule supersedes retired-vouch, D-054) | **[autonomous]** `🟡 DECIDED D-054` | E6-S02, E6-S03 | run-time counterpart to E3-S06 static presence |
| E6-S08 | Exit gate: every `examples/packs/**` green under `assent test` + broken-pack diff + dogfood CI | **[autonomous]** | E6-S01..S07 | **the E6 exit gate** |
| E6-S09 | `assent compare` seed: one ReplayBundle, baseline↔candidate, one delta classified, one gate (full suite → own epic, D-054) | **[autonomous]** | E2 (`CoverWithProfile`) | de-risks the promotion-gate reuse; ∥ the `assent test` chain |

**Dependency order**: S01 → S02 (Part A engine → Part B harness) → S03 → {S04, S05, S06}; S07 after S02+S03; S08 last; S09 ∥ (independent of S01–S08). **Do first: S01.**

> **E6 status: DONE** (main tip `ec91226`, CI green) — all 9 stories S01–S09. Full `assent test` harness + `assent compare` seed.

## Phase 5 — E-FILEEVENTS whole-file add/delete match domain (`match.fileEvents`) stories

> **EFE status: DONE** — all 5 stories S01–S05. `match.fileEvents` domain live; D-052/D-061 closed
> (D-066/D-067); exit gate pins CREATE+DELETE fixtures, corpus `--coverage`, determinism + `schemas/`==0.
> 🔴 open operator question **D-063** remains (unmatched-whole-file-delete default = REVIEW, pending
> confirm-or-relax-to-APPROVE) — shipped default, not a blocker for epic close.

Full INVEST stories in [p5-e-fileevents/spec.md](p5-e-fileevents/spec.md). Implements the FROZEN-but-
unimplemented `match.fileEvents` domain (`policy.FileEventsMatch`, the `fileEventsMatch` schema `$def`),
removing the three hard-rejects (`internal/core/policy/loader.go:32`, `internal/core/aggregate/coverage.go:451`,
`internal/adoptertest/coverage.go:301`). The frozen `evaluation-input.schema.json` ALREADY models a whole-file
event (`path:""`, `kind:delete`, `entryRef file:<path>`) — **no schema change** (`git diff schemas/`==0). Closes
**D-052** (topic-registry unpin) + **D-061** (service-catalog file-delete→BLOCK reconcile). **Every story
`[autonomous]`.**

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| EFE-S01 | ⚠️ **DECISION-PATH**: loader accept (kinds ⊆ {add,delete}, reject modify/rename fail-closed) + engine `fileEvents` matcher + `path==""` disjointness + mirror + repoint known-blockers pin | **[autonomous · engine-grade review]** `🔴 DECIDED D-062 (b)` | E1, E2, E3, E6 | loader+matcher core; **do first** |
| EFE-S02 | ⚠️ **fail-safety**: `change.FileEvent` constructor + harness minting from one-sided presence (ambiguity → opaque→REVIEW) + **unmatched-delete REVIEW-default** | **[autonomous · engine-grade review]** `🔴 OPERATOR D-063` | EFE-S01 | completes base/head→decision; resolves (a) |
| EFE-S03 | `cmd/assent` live-checkout file-lifecycle population (presence signal, not empty bytes) | **[autonomous]** | EFE-S01, EFE-S02 | live-adapter completeness (closes neither gap) |
| EFE-S04 | Unpin topic-registry (**closes D-052**) + service-catalog file-delete→BLOCK + reconcile divergence (**closes D-061**) | **[autonomous]** | EFE-S01, EFE-S02 | the two tracked-gap closures |
| EFE-S05 | Exit gate: fileEvents create/delete fixtures + topic-registry green under `assent test --coverage` + determinism | **[autonomous]** | EFE-S01..S04 | **the E-FILEEVENTS exit gate** |

**Dependency order**: S01 → S02 → {S03 ∥, S04} → S05. **Do first: S01** (engine lane, hand-built inputs, no
semantic change — ahead of the S02 minting lane, D-053 precedent). **Closes D-052 + D-061: S04.**

## Phase 5 — E5 provider host + builtins stories

Full INVEST stories in [p5-e5-provider-host/spec.md](p5-e5-provider-host/spec.md). Promotes Spike C into
product (`ResolveFacts`, projection minimization, HTTP/exec + isolation, sensitive tier), wires facts into
`assent run`, and closes **REF-GAP-1** (`resource→owner`) + **REF-GAP-2** (`builtin/repo-file`). E6 stays
`facts.yaml`-stubbed (ADR-0014). **Judgment call (a):** digest-pin/declarations via internal host config first —
no silent `config.schema.json` widen. **Engine-grade:** S01–S05, S07, S08, S10.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E5-S01 | ⚠️ Promote Spike `ResolveFacts` + negotiation + state goldens | **[autonomous · engine-grade]** | P3 schemas, Spike C | host core; **do first** |
| E5-S02 | Projection-minimized query + maxAge defaults + fullContent gate | **[autonomous · engine-grade]** | E5-S01 | minimization + freshness |
| E5-S03 | HTTP/exec transports + ScrubEnv + isolation CI + digest-pin | **[autonomous · engine-grade]** | E5-S01, S02 | ADR-0015 §7 |
| E5-S04 | ⚠️ Sensitive tier (15m, propagate `Fact.Sensitive`) — INBOX F1/F2 | **[autonomous · engine-grade]** | E5-S02 | sensitive must-hold |
| E5-S05 | Wire host into `assent run` (Facts + `factsResolvedAt`); E6 fence | **[autonomous · engine-grade]** | E5-S01..S04 | live facts path |
| E5-S06 | Builtin forge-groups | **[autonomous]** hermetic; live **[infra-gated]** | E5-S05 | ownership packs live |
| E5-S07 | Builtin `repo-file` most-specific-first | **[autonomous · engine-grade]** | E5-S05 | **closes REF-GAP-2**; unlocks C5/C6 |
| E5-S08 | Referenced-resource ownership (`resource→owner`) | **[autonomous · engine-grade]** | E5-S05/S07 | **closes REF-GAP-1**; unlocks C7 |
| E5-S09 | Optional ownership-file / HTTP polish (defer OIDC/LDAP) | **Deferred** (D-070) | E5-S05 | ADR-0004 polish |
| E5-S10 | Exit gate: isolation CI + goldens + hermetic resolved facts + seed C5–C7 | **[autonomous · engine-grade]** | E5-S01..S08 | **the E5 exit gate** |

**Dependency order**: S01 → S02 → S03 → S04 → S05 → {S06 ∥ S07} → S08 → S09? → S10.
**Closes REF-GAP-2: S07. Closes REF-GAP-1: S08. Sensitive tier: S04.**

> **E5 status: DONE** (main tip `3e4fb5b`, CI green) — provider host + builtins complete.

## Phase 5 — E4 GitLab forge adapter stories

Full INVEST stories in [p5-e4-gitlab-forge/spec.md](p5-e4-gitlab-forge/spec.md). E4 extends the
P4 walking-skeleton forge (`internal/forge`, `internal/forge/gitlab`, `internal/forge/fake`) to the
full ADR-0017 §7 **`Snapshot → Resolve → Reconcile`** port: L2 httptest cassettes for read-side MR
state + ApprovalEvidence, P3-E5 reconciliation gaps (supersession, resolve stale, rescan), forge-probed
`assent doctor` (closes D-034 forge-backed path), run-path wiring (changed files + live approval evidence;
closes D-042 F1), and an autonomous conformance suite. **Judgment call (a):** summary-comment slot
deferred to E8. **Engine-grade:** S03, S04, S05, S06, S08, S10.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E4-S01 | Snapshot + Resolve port types + hermetic fakes | **[autonomous]** | P4 forge | read-side port seam; **do first** |
| E4-S02 | GitLab Snapshot L2 cassettes (MR, changed files, capabilities) | **[autonomous]** | E4-S01 | honest SHAs + tier flags |
| E4-S03 | ⚠️ GitLab Resolve ApprovalEvidence eligibility chain | **[autonomous · engine-grade]** | E4-S01, S02 | ADR-0017 §3 require-review proof |
| E4-S04 | ⚠️ Reconcile P3-E5 gaps (supersession, resolve stale, rescan) | **[autonomous · engine-grade]** | E4-S01 | ADR-0019 steps 6/7/9 |
| E4-S05 | ⚠️ Doctor forge-probed capability report (closes D-034 forge path) | **[autonomous · engine-grade]** | E4-S02 | ADR-0017 §9 arming honesty |
| E4-S06 | ⚠️ Wire Snapshot/Resolve into `assent run`; E6 fence | **[autonomous · engine-grade]** | E4-S02, S03, S05 | live forge evaluation path |
| E4-S07 | Conformance: target/source advanced → SHA-guard rejection | **[autonomous]** | E4-S04, S06 | ADR-0015 §2 executable |
| E4-S08 | Conformance: `.assent/**` MR → assent-policy BLOCK | **[autonomous · engine-grade]** | E4-S06 | **closes D-042 F1** |
| E4-S09 | Conformance: P3-E5 replay + spoofed marker ignored | **[autonomous]** | E4-S04 | ADR-0019 + ADR-0005 suite |
| E4-S10 | Exit gate: L2 CI + hermetic forge path green | **[autonomous · engine-grade]** | E4-S01..S09 | **the E4 autonomous exit gate** |
| E4-S11 | Live GitLab L3 conformance re-run | **[infra-gated: live GitLab / token]** | E4-S10 | L3 adapter proof (optional) |

**Dependency order**: S01 → S02 → S03 → S04 → S05 → S06 → {S07 ∥ S08 ∥ S09} → S10; S11 after S10 when
infra available. **Closes D-034 (forge-backed): S05+S06. Closes D-042 F1: S08.**

> **E4 status: SPEC READY** — autonomous implementers may claim after spec lands on `main`.

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
