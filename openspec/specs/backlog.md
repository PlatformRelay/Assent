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
| P2-E6 | Spike D — Kubernetes CRD/CR validation feasibility (D-017 B11) | [spec](p2-e6-spike-crd/spec.md) | Deferred to the Phase-3 window (D-018 — not first-wave); does **not** gate P2-E5 | no | feeds a dedicated ADR (number assigned when Spike D closes) + the E14 go/no-go |

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
| **P4-SEC-OSSF** | OpenSSF/security hardening (SECURITY.md, CODEOWNERS, Scorecard, scheduled govulncheck) | **DONE** (2026-08-03, D-045) | no | In-tree done. `main` now has required `verify`+CodeQL, `enforce_admins`, no force-push/delete. Remaining Scorecard points (mandatory approvers / required CODEOWNERS review) are **accepted solo-maintainer risk** (code-scanning #1/#4 dismissed 2026-08-13). Open Scorecard work is [P5-SEC-SC](#phase-5--sec-sc-openssf-scorecard-residuals) (fuzzing + Best Practices badge), not more branch-protection settings |
| **AUD-OPS** | Operator-only audit residuals: SEC-05 rotate `HOMEBREW_TAP_GITHUB_TOKEN` to fine-grained PAT · SEC-06 tag ruleset · RELSE-07 `enforce_admins` on main | **OPEN — operator** | **yes** | Fenced out of P5-AUD (live GitHub settings/secrets); see audit 2026-08-06 |
| **AUD-RELSE-08** | RELSE-08: make `release-exitgate` a required PR check on `main` (today it can be skipped/pending at merge) | **OPEN — operator** | **yes** | Dispatched from AUD-S03 not-in-scope (live GitHub branch-protection/required-checks settings); see audit 2026-08-06 |

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
| **REF-GAP-1** | Referenced-resource authorization fact source (a list value / ACL names *another* team's resource → who owns it?) | **CLOSED (E5-S08)** | `builtin/resource-owner` shipped; hermetic L0 + run-path wiring in E5-S10. Demonstrator fixture = C7, landed **P5-EX EX-S07** (`.assent/tests/topics/resource-ownership/`, D-143 — was deferred D-071) |
| **REF-GAP-2** | In-repo-state-as-a-fact (quota/placement/limits registries + in-repo reviewers files that today no provider reads) | **CLOSED (E5-S07)** | `builtin/repo-file` most-specific-first shipped; hermetic run path in E5-S10 (`TestE5ExitGateResolvedFacts`). C5/C6 fixtures landed **P5-EX EX-S07** (`.assent/tests/topics/quota-ceiling/`, `.assent/tests/vars/placement/`, D-143; was deferred D-071) |
| **REF-GAP-3** | Cross-class / companion-file correlation ("two-step delete": remove from file A *and* append to manifest B) | **OPEN — likely out of v1** | `changes` is class-slice-scoped by contract (ADR-0017 §5); C8 known-limitation fixture landed **P5-EX EX-S07** (`infra-vars` `vars/companion-delete`, measured REVIEW); the underlying correlation engine feature itself remains open, scope decided via OQ |
| **REF-GAP-4** | Plan-level blast radius (weighting the expanded IaC plan, not the request diff) | **OUT of model** | assent gates the request diff; `points`/`threshold` bulk-guard on the diff is the in-scope approximation |

**Generalized example/test candidates (a later sanitized authoring lane — passes `check-sanitization.sh`):**

| ID | Item | Status | Closest existing archetype |
| --- | --- | --- | --- |
| **REF-EX** | Author 8 domain-neutral archetype fixtures C1–C8 (list-no-shrink, privilege-tier allow-list, wildcard-grant block, soft-delete-as-field-add, quota-ceiling-from-fact, placement allow-list, referenced-resource-ownership [gap demo], companion-file delete [known-limitation]) | **CLOSED (P5-EX EX-S06/EX-S07)** — C1–C4 EX-S06, C5–C8 EX-S07, exit-gated by EX-S10 (`hack/examples/ex_exitgate_test.sh`: C1–C8 presence + C8 measured REVIEW, mutation-tested; **manual invocation only** — unlike `hack/audit/exitgate_test.sh` / `hack/release/exitgate_test.sh` (excluded from `task check` but still run directly in `verify.yaml`), this gate is invoked nowhere automated: no `task` entry, no CI step) | [p5-ex-complex-examples](p5-ex-complex-examples/spec.md). Extends no-destruction (C1/C4/C8), allowed-fields+ownership (C2/C3/C6/C7), bounded-change (C5, reused). Engine+E5 facts already shipped; D-071 deferral closed by this epic. **Not** P5-DEM (D-143) |

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
| E4-S05 | ⚠️ Doctor forge-probed capability report (closes D-034 forge path) | **[autonomous · engine-grade]** | E4-S02 | ADR-0017 §9 arming honesty; C17 probe |
| E4-S06 | ⚠️ Wire Snapshot/Resolve + forge-probed arming into `assent run` | **[autonomous · engine-grade]** | E4-S02, S03, S05; **E5-S05 (`run.go`)** | live forge path; **closes D-034 run path**; checkout precedence (D-077) |
| E4-S07 | Conformance: target/source advanced → SHA-guard rejection | **[autonomous]** | E4-S04, S06 | ADR-0015 §2 executable |
| E4-S08 | Conformance: `.assent/**` MR → assent-policy BLOCK | **[autonomous · engine-grade]** | E4-S06 | **closes D-042 F1** |
| E4-S09 | Conformance: P3-E5 replay + spoofed marker ignored | **[autonomous]** | E4-S04 | ADR-0019 + ADR-0005 suite |
| E4-S10 | Exit gate: L2 CI + hermetic forge path green | **[autonomous · engine-grade]** | E4-S01..S09 | **the E4 autonomous exit gate** |
| E4-S11 | Live GitLab L3 conformance re-run | **[infra-gated: live GitLab / token]** | E4-S10 | L3 adapter proof (optional) |

**Dependency order**: S01 → S02 → S03 → S04 → S05 → S06 → {S07 ∥ S08 ∥ S09} → S10; S11 after S10 when
infra available. **Closes D-034 (forge-backed): S05+S06 (run-path arming = S06). Closes D-042 F1: S08.**

> **E4 status: DONE (autonomous slice)** — S01–S10 closed; `task check` green (D-010 ≥90%); S11 live L3 optional (infra-gated).

## Phase 5 — E7 E2E & conformance infra stories

Full INVEST stories in [p5-e7-e2e-conformance/spec.md](p5-e7-e2e-conformance/spec.md). E7
**extends** E4's L2 conformance (`internal/forge/conformance`, D-079) — it does not re-prove
Snapshot/Resolve/Reconcile. Scope: Spike-B profile task wiring, sample-repo generator for all
three `examples/repos/` shapes, forge-neutral conformance catalog + remaining hermetic
ADR-0015/0017 adversarial cases, explicit determinism CI gate (P4-E1-S12 intent), sanitization
in verify, optional kind lab (D-038), L3 live harness (absorbs E4-S11). REQ IDs
`REQ-E7-S0n-nn`. **Autonomous stories S01–S05 + S08** close the epic without live GitLab.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E7-S01 | Spike-B e2e profile: `task e2e-vet` + operator docs | **[autonomous]** | P4-E1-S09, P2-E2 | e2e wiring cannot rot; **do first** |
| E7-S02 | Sample-repo generator: seed all three `examples/repos/` shapes | **[autonomous]** | E7-S01, P1-E1 | L3 seeding without hand-copy |
| E7-S03 | ⚠️ Conformance catalog + new hermetic adversarial cases (§4 pipeline **catalog-index E4-S05-05**, §8 fork advisory, §4 max_age arming) | **[autonomous · engine-grade]** | E4-S05..S10, E7-S01 | ADR-0005 executable catalog |
| E7-S04 | Determinism gate: explicit CI `-count=2` step | **[autonomous]** | E4 conformance, E2-S10 | closes P4-E1-S12 CI gap |
| E7-S05 | Security jobs: sanitization in verify + confirm gitleaks/e2e-vet | **[autonomous]** | P1-E1-S01-02, E7-S01 | D-002 hygiene in CI |
| E7-S06 | kind local lab scaffold (`task kind-up/down`) | **[infra-gated: docker+kind]** | E7-S01, S02 | D-038 local demo (optional) |
| E7-S07 | L3 live forge conformance harness (catalog replay) | **[infra-gated: live GitLab / token]** | E7-S02, S03, E4-S10 | L3 proof; absorbs E4-S11 |
| E7-S08 | Exit gate: autonomous infra wired + catalog green | **[autonomous · engine-grade]** | E7-S01..S05, S03 | **the E7 autonomous exit gate** |

**Dependency order**: S01 → S02 → S03 → {S04 ∥ S05} → S08; S06 after S01 when operator claims
kind lab; S07 after S02+S03 when infra available. **Judgment calls D-080–D086** in
`docs/decisions/decisions.md`. **Do first: S01** (smallest wiring slice, unblocks generator +
docs).

> **E7 status: AUTONOMOUS COMPLETE (S01–S05 + S08)** — S06 (kind lab) and S07 (live L3) remain
> infra-gated optional per D-081/D-083; E8/E9 may proceed on L3 catalog homes without live proof.

## Phase 5 — E8 Renderer & presentation stories

Full INVEST stories in [p5-e8-renderer/spec.md](p5-e8-renderer/spec.md). E8 delivers ADR-0016
tier 0 only (D-012/D-015): renderer-owned envelope, presentation config knobs, CEL messages,
`assent render`, default-theme goldens, presentation lint, `en` locale catalog, and summary-comment
slot (closes D-073). Consumes frozen `PresentationModel` — no parallel render contract. **Judgment
calls D-088–D-097** in `docs/decisions/decisions.md`. REQ prefix `REQ-E8-S0n-nn`. **All stories
autonomous** (fixture goldens + fake summary upsert; no live GitLab required).

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E8-S01 | Renderer scaffold + PresentationModel fixture loader | **[autonomous]** | P3 PresentationModel, E2 record | typed consumer seam; **do first** |
| E8-S02 | Presentation config knobs (`config.yaml` + schema D-088) | **[autonomous]** | E8-S01 | ADR-0016 tier-0 knobs |
| E8-S03 | `en` locale catalog (chrome strings) | **[autonomous]** | E8-S02 | ADR-0016 §5 |
| E8-S04 | ⚠️ Renderer-owned envelope + marker region | **[autonomous · engine-grade]** | E8-S01, forge Marker | ADR-0016 §1 / ADR-0019 |
| E8-S05 | ⚠️ Markdown/HTML escaping + length clamping | **[autonomous · engine-grade]** | E8-S04 | ADR-0012 injection-safe |
| E8-S06 | ⚠️ Sensitive fact redaction (D-068 handoff) | **[autonomous · engine-grade]** | E8-S05, E5-S04 | presentation redaction |
| E8-S07 | ⚠️ CEL message rendering (`{{ }}` scope) | **[autonomous · engine-grade]** | E8-S05, aggregate CEL | ADR-0016 §2 |
| E8-S08 | Default theme finding-thread + `buildDesired` wiring | **[autonomous]** | E8-S03..S07 | replaces run stub body |
| E8-S09 | Default-theme golden markdown snapshots | **[autonomous]** | E8-S08 | ADR-0016 §4 goldens |
| E8-S10 | `assent render --finding` CLI | **[autonomous]** | E8-S09 | preview without live MR (D-097) |
| E8-S11 | ⚠️ Presentation lint extends E3-S04 + tier-1 rejection | **[autonomous · engine-grade]** | E8-S07, E3-S04 | docs.summary/debug lint |
| E8-S12 | ⚠️ Forge port: UpsertComment + Summary field + Reconcile refactor | **[autonomous · engine-grade]** | E4 Reconcile | bot-note listing + step-3 hook |
| E8-S13 | Rendered summary body + P3-E5 step 3 (closes D-073) | **[autonomous · engine-grade]** | E8-S08, S12 | lane-e8-s13 |
| E8-S14 | Exit gate: render goldens + safety split proven | **[autonomous · engine-grade]** | E8-S01..S13 | **the E8 autonomous exit gate** |

**Dependency order**: S01 → S02 → S03 → S04 → S05 → S06 → S07 → S08 → S09 → S10 → S11 → S12 → S13 →
S14 (serialized). **Closes D-073: S12+S13. Closes D-068 presentation handoff: S06. Schema touch: S02 only (D-088).**

> **E8 status: AUTONOMOUS COMPLETE (S01–S14)** — renderer tier 0 shipped; render goldens +
> summary slot hermetic; safety split proven (D-098). S06/S07 remain infra-gated optional per
> D-081/D-083; E9 release may proceed.

## Phase 5 — E9 Distribution & release stories

Full INVEST stories in [p5-e9-distribution/spec.md](p5-e9-distribution/spec.md). E9 executes the
oss-playbook release surface: goreleaser binaries (brew, curl+checksum, `go install`), cosign
keyless + SLSA + SBOM, git-cliff notes without SHAs, CI hardening **audit** (extend D-045 jobs —
no duplicate CodeQL/Scorecard), MkDocs product-only nav, README maturity table, VHS demo tapes,
OQ-2 GitLab mirror disposition. **Judgment calls D-099–D-110** in `docs/decisions/decisions.md`.
REQ prefix `REQ-E9-S0n-nn`. **Autonomous close:** S01–S04 + S07a + S08→S09 + S11 + S12 (no publish
credentials). **Infra-gated:** S05, S06, S07b, S13 publish half.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E9-S01 | Semver ldflags + `assent version` contract | **[autonomous]** | none | stamped binaries; **do first** |
| E9-S02 | Goreleaser config + local snapshot dry-run | **[autonomous]** | S01 | cross-platform archives + checksums |
| E9-S03 | git-cliff changelog without SHAs + CHANGELOG.md sync | **[autonomous]** | S02 | oss-playbook #4 release notes |
| E9-S04 | CI hardening audit + residual gaps (extend, don't duplicate) | **[autonomous]** | S01 ∥ | actionlint + audit doc; D-102 |
| E9-S05 | Tag-triggered release workflow + goreleaser publish | **[infra-gated: GH release write]** | S02, S03 | GitHub Release assets |
| E9-S06 | ⚠️ cosign keyless + SLSA + SBOM on release artifacts | **[infra-gated · engine-grade]** | S05 | supply-chain verify surface |
| E9-S07a | curl+checksum install script + install docs | **[autonomous]** | S02 | checksum install path; D-110 skip-when-no-sig |
| E9-S07b | Homebrew tap wiring + goreleaser brews publish | **[infra-gated: tap push]** | S05, S06 | third install channel |
| E9-S08 | MkDocs product-only nav + install page | **[autonomous]** | S07a | D-103 docs fence |
| E9-S09 | README maturity formula + honest alpha status | **[autonomous]** | S08 | oss-playbook #3 |
| E9-S10 | VHS demo tape sources (GIF optional) | **[autonomous tapes · infra-gated GIF]** | none ∥ | oss-playbook #10 |
| E9-S11 | OQ-2 GitLab dogfood mirror decide-and-log (D-105 defer) | **[autonomous · decide-and-log]** | none ∥ | hosting disposition |
| E9-S12 | ⚠️ Release artifact verify harness (S02 snapshot) | **[autonomous · engine-grade]** | S02 | checksum verify; cosign optional |
| E9-S13 | Exit gate: tagged release + channels + docs live | **[autonomous half closed · infra-gated publish]** | S01..S12 | **the E9 exit gate** |

**Dependency order**: S01 → {S02, S04 ∥}; S02 → S03; S02 → S07a → S08 → S09; S02 → S12; S02 → S05
→ S06 → S07b; S10 ∥; S11 ∥; S13 last. **Closes oss-playbook #4–#5 nav/install: S02–S07a/b, S08–S09.
Next after E9: PolicyComparisonSuite runner (D-057) — separate epic, not absorbed.**

## Phase 5 — PCS PolicyComparisonSuite full runner stories

Full INVEST stories in [p5-pcs-policy-comparison/spec.md](p5-pcs-policy-comparison/spec.md).
PCS completes the D-057 deferred scope: six delta classifiers, five promotion gates,
`acceptedDeltas` per-delta allowlist, `ComparisonRecord` emission, multi-case
`PolicyComparisonSuite` corpus, and profile→pack activation — **extending** the E6-S09 seed
(`internal/compare`, `cmd/assent/compare.go`) without schema changes (`git diff schemas/`==0).
REQ prefix `REQ-PCS-S0n-nn`. **Every story `[autonomous]`.**

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| PCS-S01 | Catalogue extraction + pure profile→pack activation | **[autonomous]** | E6-S09 seed | `internal/catalogue` + CLI wiring; **do first** |
| PCS-S02 | Classifiers: missed intervention + stricter intervention | **[autonomous · engine-grade]** | S01 | zero-missed gate taxonomy |
| PCS-S03 | Classifiers: obligation uncovered + score threshold | **[autonomous · engine-grade]** | S01 (∥ S02) | obligation + explicitly-accepted gates |
| PCS-S04 | ComparisonRecord emission | **[autonomous]** | S02, S03 | auditable per-case deltas |
| PCS-S05 | Five-gate evaluator + acceptedDeltas allowlist | **[autonomous · engine-grade]** | S04 | promotion gate table |
| PCS-S06 | PolicyComparisonSuite loader + multi-case runner | **[autonomous]** | S01, S05 | immutable corpus replay |
| PCS-S07 | CLI suite mode + ADR-0018 exit codes | **[autonomous · engine-grade]** | S06 | `assent compare --suite` |
| PCS-S08 | Adversarial corpus + CI dogfood | **[autonomous]** | S07 | regression corpus |
| PCS-S09 | Exit gate: full runner + D-057 closed | **[autonomous · engine-grade]** | S01..S08 | **the PCS exit gate** |

**Dependency order**: S01 → {S02 ∥ S03} → S04 → S05 → S06 → S07 → S08 → S09. **Closes D-057:
S01–S09. Do first: S01.**

> **PCS status: AUTONOMOUS COMPLETE (S01–S09)** — `hack/compare/exitgate_test.sh` runs full
> suite corpus + E6 single-dir seed + `git diff schemas/` guard; **D-057 deferred scope closed;
> D-112–D-118 cited (D-118 exit gate).** Residual: human review of `acceptedDeltas` rationale
> text is not automated.

> **E9 status: AUTONOMOUS COMPLETE (S01–S04 + S07a + S08→S09 + S10 + S11 + S12 + S13)** —
> `task release-snapshot && task release-verify && task docs-build` green via
> `hack/release/exitgate_test.sh`; product docs nav fenced (D-103); snapshot checksum verify (D-110).
> **Judgment calls D-099–D-110 cited.** **D-111 CLOSED** @ tag `v0.1.0` (signed assets + live
> install proof). **Homebrew Formula published** @ `v0.1.0`. **Optional residual:** rotate `HOMEBREW_TAP_GITHUB_TOKEN` to a fine-grained PAT (Contents: write on `homebrew-tap` only).

## Phase 5 — AUD audit-remediation (2026-08-06) stories

Full INVEST stories in [p5-aud-audit-remediation/spec.md](p5-aud-audit-remediation/spec.md). AUD
remediates `agent-context/PROJECT-AUDIT-2026-08-06.md` (verdict READY WITH CONDITIONS at `e668d0e`).
**Next-tag release conditions: S01 (REL-07 P1 fail-closed enumeration), S02 (RELSE-01 changelog +
gate), S03 (RELSE-05 verify-gated release)** — everything else is engineering health. S01/S04/S16
bind to the architect decisions landed with the spec: **ADR-0020** (forge snapshot changed-file
completeness) + **D-119..D-123**. Lanes own disjoint paths (A forge · B CI/release · C run-path ·
D docs/CLI · E guardrails/tests). **Operator-only, fenced out:** SEC-05 PAT rotation, SEC-06 tag
ruleset, RELSE-07 enforce_admins (see the AUD-OPS residual row). **Every story `[autonomous]`.**

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| AUD-S01 | ⚠️ REL-07 P1: fail-closed truncated/404 changed-file enumeration (mechanism per ADR-0020/D-119) | **[autonomous · engine-grade]** | none | **release condition**; D-042 guard integrity; **do first** |
| AUD-S02 | RELSE-01: regenerate CHANGELOG + wire `changelog-verify` into `task check` + verify CI | **[autonomous]** | none | **release condition**; drift gate can't regress |
| AUD-S03 | RELSE-05: release job asserts verify-green on the tag SHA before build | **[autonomous]** | none | **release condition**; no red-CI tags ship |
| AUD-S04 | ARCH-03: `toolDigest` from Go build info + description-only schema edit (D-120) | **[autonomous]** | D-120 (landed) | record-pins contract honesty (semver-visible) |
| AUD-S05 | DOC-08: real CLI help + `docs/usage/cli.md` reference | **[autonomous]** | none | front-door truth (CLI-surface visible) |
| AUD-S06 | Docs truth-lag sweep: DOC-05/06/07/09/10/11 + stale comments | **[autonomous]** | S05 | quick-start runs green; published surfaces truthful |
| AUD-S07 | ARCH-01: depguard boundary rules + purity-walk extension (D-123; ADR-0011 Amendment 3 same-change) | **[autonomous]** | none | violating import fails CI (3rd-audit closure) |
| AUD-S08 | ⚠️ REL-08: emit DecisionRecord before reconcile, atomic write-then-rename (D-122) | **[autonomous · engine-grade]** | S04 (lane) | audit-trail integrity; no record ⇒ no action |
| AUD-S09 | SEC-04: pin Task version in verify.yaml | **[autonomous]** | S02 (lane) | no mutable gate toolchain |
| AUD-S10 | REL-03/SEC-08: bounded response reads + pagination caps (fail-closed) | **[autonomous]** | S01 (lane) | transport availability hardening |
| AUD-S11 | REL-04: idempotent-GET retry/backoff + context deadlines (writes never retried) | **[autonomous]** | S10 | transient-failure availability |
| AUD-S12 | ⚠️ REL-06: malformed bot-marker skip-with-warning (spoof surface unchanged) | **[autonomous · engine-grade]** | S10, S11 | reconcile un-brickable |
| AUD-S13 | TEST-02/05/06 depth bundle + aggregate internal/ coverage ≥ 91.0% | **[autonomous]** | S10–S12 (lane) | D-010 headroom bought with behavior tests |
| AUD-S14 | SEC-01 ajv lockfile (`npm ci`) + SEC-03 `persist-credentials: false` everywhere | **[autonomous]** | S09 (lane) | supply-chain pin closure |
| AUD-S15 | ⚠️ ARCH-02: lift `MRInfo`/`ErrNotFound` into the forge port (pre-E10) | **[autonomous · engine-grade]** | Lanes A+C drained | GitHub-adapter readiness; byte-identical refactor |
| AUD-S16 | ARCH-04: `ReplayBundleDigest` → `assent-jcs-v1` domain digest; byte artifacts stay raw (D-121) | **[autonomous]** | D-121 (landed); S08 (lane) | one documented hash truth (semver-visible) |
| AUD-S17 | ARCH-05: C4 diagrams synced (planned vs shipped legend) | **[autonomous]** | none | contributor-facing architecture truth |
| AUD-S18 | Exit gate: conditions closed + pins + ≥91% + disposition table complete | **[autonomous]** | S01..S17 | **the AUD exit gate** |

**Dependency order**: {S01 ∥ S02 ∥ S03} (release conditions, three lanes) → {S04–S09 ∥ across
lanes} → S10 → S11 → S12 → {S13, S14, S16, S17} → S15 (last code story) → S18. **Do first: S01**
(the P1; the next tag is conditioned on it). **Release conditions: S01+S02+S03. Operator residuals
(NOT stories): SEC-05, SEC-06, RELSE-07.**

**AUD status: AUTONOMOUS COMPLETE** — S01–S18 landed; the three next-tag release conditions
(S01 REL-07, S02 RELSE-01, S03 RELSE-05) are CLOSED; **D-119–D-125, D-128 and D-132 cited**.
The exit gate is `hack/audit/exitgate_test.sh` (AUD-S18, wired into the `release-exitgate` job of
`.github/workflows/verify.yaml`): one invocation over the S01 cassettes, `task check` across all 14
stages incl. `changelog-verify`, the RELSE-05 release gate, the DOC truth pins, the determinism
double-run, a **ref-relative** frozen-schema diff against the `v0.1.0` tag (D-132 — the old
working-tree `git diff schemas/` was blind to committed changes), and the finding→disposition
table. Every one of the **37** 2026-08-06 finding IDs is dispositioned in
[Appendix B of the AUD spec](p5-aud-audit-remediation/spec.md): **27 Done**, **4 Operator**,
**6 Accepted (D-132)**.

> **⚠️ AUD AUTONOMOUS COMPLETE IS NOT RELEASE CLEARANCE.** Two decision-path fail-opens found
> DURING this epic — **OQ-27** (a relational CEL leaf over string-bound operands returns a
> silently wrong boolean: a verified BLOCK→APPROVE flip; **closed** by D-131) and **OQ-28**
> (`builtin/repo-file` enforces path but not filesystem containment: a symlink yields facts from
> outside the declared roots; **closed** by D-133 on `main` plus D-129's provider-layer guard in
> the dedicated provider lane) — are **not** 2026-08-06 audit findings and **blocked the next tag
> independently**. See
> [open-questions.md](../../docs/planning/open-questions.md) and the AUD spec's
> *Post-audit release blockers* section, which the exit gate pins.
>
> **Operator residuals handed over, explicitly, not dropped** (rows `AUD-OPS` and `AUD-RELSE-08`
> above; all four are live GitHub settings/secrets no in-tree gate can reach):
> **SEC-05** rotate `HOMEBREW_TAP_GITHUB_TOKEN` from the classic PAT to a fine-grained one
> (Contents: write on `homebrew-tap` only) · **SEC-06** add a tag ruleset on `v*.*.*` ·
> **RELSE-07** set branch protection `enforce_admins` on `main` (today `non_admins`, so the
> admin/agent-loop push path is unbound) · **RELSE-08** make `release-exitgate` a required PR
> check (it is skipped on PRs today, so a regression is detected only post-merge — which is
> exactly what happened at `49ba1ad`).

## P5-E10 — GitHub forge adapter + Actions entrypoint (**UNLOCKED D-140**)

Spec: [p5-e10-github-forge/spec.md](p5-e10-github-forge/spec.md) · ADR: **0021** (the seam) ·
Dossier: [forge-dossier-github.md](../../docs/planning/forge-dossier-github.md).
**Ordering is normative — S00 before any code, and the seam (S01–S05) before the first GitHub
API call.** An adversarial review of the first draft (2026-08-10) found **two P0 representation
defects** by reading the port against the code: the port addresses head content by branch name
in one project, so **every GitHub fork PR would mint a fabricated whole-file DELETE**
(`run.go:274` → `fileAtRefOrAbsent` → `OneSidedLifecycle`); and `$defs.pins` is
`additionalProperties:false` with a **single-string** `capabilityGap` required iff
`mergeResultDigest` is null, so an eleven-capability report **has nowhere valid to be
recorded**. Both are decided in ADR-0021 (items 5–8) and gated by S00. S00/S02/S04 are
core-contract and require **maintainer LGTM** (GOVERNANCE); `/agent-loop-auto` must surface
them rather than auto-merge.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E10-S00 | ⚠️ GitHub addressing & representation model (4 questions, ~1 page) | **[autonomous · design · LGTM]** | none | **do first** — kills both P0s before the port freezes |
| E10-S01 | Extract the conformance suite into an importable package + observation surface | **[autonomous]** | S00 | first **code** story; no assertion may be weakened |
| E10-S02 | ⚠️ `forge.RunPort` + **neutral factory** + MR-relative addressing + identity | **[autonomous · engine-grade · LGTM]** | S00, S01 | one neutral seam; ARCH-02 cannot recur |
| E10-S03 | ⚠️ Collapse `SyntheticDigest` onto `Snapshot.Heads.MergeResultDigest` | **[autonomous · engine-grade]** | S02 | digest scheme adapter-owned; allowlist emptied |
| E10-S04 | ⚠️ Neutral capability model — `unknown` never arms | **[autonomous · engine-grade · LGTM]** | S02 | one fail-closed guarantee, not two |
| E10-S05 | Port-level transport requirements (bounded reads, caps, GET-only retry, deadlines) | **[autonomous]** | S01, S04 | availability behaviour can't diverge per adapter |
| E10-S06 | GitHub client: REST + GraphQL, PAT + App installation auth | **[autonomous]** | S05 | adapter foundation; no secret in any fixture |
| E10-S07 | GitHub Snapshot (MRInfo, ADR-0020 changed-file completeness, merge-result pin) | **[autonomous]** | S06 | absent-means-trusted closed on fork detection |
| E10-S08 | ⚠️ GitHub Resolve → `ApprovalEvidence` (author/bot excluded, dismissal-aware) | **[autonomous · engine-grade]** | S06 | unprovable eligibility ⇒ unsatisfiable |
| E10-S09 | ⚠️ GitHub capability report (11 flags; unverified ⇒ `unknown`) | **[autonomous · engine-grade]** | S04, S06 | honest gaps; exhaustiveness enforced |
| E10-S10 | ⚠️ GitHub Reconcile writes (ADR-0019 parity, GraphQL thread resolution) | **[autonomous · engine-grade]** | S07–S09 | same engine, second adapter |
| E10-S11 | ⚠️ SHA-guarded merge + deferred arming + revoke-on-push | **[autonomous · engine-grade]** | S10 | ADR-0015 §2 on GitHub |
| E10-S12 | ⚠️ Capability gaps fail closed — `merges == 0` **and paired `merges == 1`** | **[autonomous · engine-grade]** | S11 | positive control mandatory; else vacuous |
| E10-S13 | Forge selection in `run`/`doctor`; ambiguity fails closed | **[autonomous]** | S12 | no default-to-GitLab |
| E10-S14 | Conformance parity — **every** row needs an adapter disposition, not just deferrals | **[autonomous]** | S13 | else GitHub ships with 0 trust-boundary cases proven |
| E10-S15 | Docs & maturity truth (README tier, C4, `--forge`, dossier items) | **[autonomous]** | S14 | no doc claims an `unknown` capability |
| E10-S16 | Actions entrypoint (`action.yml`, pinned binary, base-ref trust) | **[autonomous]** | S15 | ✅ **operator-answered 2026-08-10: stays in E10**, last + independently droppable |
| E10-S17 | Exit gate | **[autonomous]** | S01–S16 | **the E10 exit gate** |
| E10-S18 | Live GitHub adoption proof on a real repo (mirrors D-042) | **[infra-gated · operator]** | S17 + infra | D-012-grade evidence; not an autonomous blocker |

## P5-E11 — Complex-rule backend: Rego predicate tier (**IMPLEMENTATION UNLOCKED D-141**)

Spec: [p5-e11-rego-backend/spec.md](p5-e11-rego-backend/spec.md) · ADR: **0002 v2** (governing).
**Two traps recorded in D-141**: E11 is the **first epic whose DoD is `git diff schemas/` != 0**
(announced additive `rego:` leaf; `schemas/decision/**` still frozen), and a **wall-clock
evaluation timeout would itself violate rule 7** — the budget must be machine-independent and
exceeding it is a process error, never a policy outcome. S02/S04/S06/S07 require **maintainer
LGTM** (published contract + the decision path itself). Independent of E10; may run in parallel.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| E11-S00 | ⚠️ **SPIKE, do first**: does OPA expose a deterministic (non-wall-clock) eval budget? Nested throwaway module — root `go.mod` unchanged | **[autonomous · spike]** | none | if not, S06 stalls the epic *after* S02+S03 commit |
| E11-S01 | ✅ **DONE (D-156)** — tier-1 (CEL) ceiling recorded: `docs/planning/rego-tier-ceiling.md` | **[autonomous]** | none | **E11 NARROWED**: cross-manifest + set-difference **struck**; only fold/aggregate + in-input graph survive. Cross-manifest is an *input* limit S05's identical `EvaluationInput` does not lift. Residuals OQ-35/OQ-36 |
| E11-S02 | ⚠️ Additive `rego:` leaf in the policy schema (announced, no `apiVersion` bump) | **[autonomous · engine-grade · LGTM]** | **S00**, S01 | drift guard scoped; both polarities tested |
| E11-S03 | 🔴 Module loading from the **target ref**; compile failure is a lint hard error — **blocked on the operator's rule-7 answer (d1/d2)**: this story adds OPA to `go.mod` inside the guarded tree | **[autonomous · engine-grade · LGTM]** | S02 + operator | no second, laxer load path; transitive purity guard under (d1) |
| E11-S04 | 🔴 OPA capability sandbox — **blocked on the operator's rule-7 *mechanism* answer (d1 vs d2)**; "accept and pin" settled only the supply-chain half | **[autonomous · engine-grade · LGTM]** | S03 + operator | both purity gates are non-transitive; see D-141 |
| E11-S05 | ⚠️ Input binding to the identical `EvaluationInput` | **[autonomous · engine-grade]** | S04 | proves P3-E1-S02 neutrality empirically |
| E11-S06 | ⚠️ Deterministic evaluation budget (never wall-clock) | **[autonomous · engine-grade · LGTM]** | S05 | N≥100 identical runs; budget ≠ decision |
| E11-S07 | ⚠️ Violations → findings; **zero violations never proves an obligation** | **[autonomous · engine-grade · LGTM]** | S06 | the failing polarity is tested |
| E11-S08 | ⚠️ Aggregation boundary — module cannot set effect/points/phase | **[autonomous · engine-grade]** | S07 | ADR-0002 v2 / ADR-0007 held structurally |
| E11-S09 | `assent lint` hard errors + faithful catalogue entries | **[autonomous]** | S08 | E3 parity for the second backend |
| E11-S10 | `assent test` support + both-polarity coverage | **[autonomous]** | S08 | ADR-0014 unchanged |
| E11-S11 | Remove the `# locked: D-012` quarantine; **update** the P3-E3-S04 guard | **[autonomous]** | S10 | only E11's lane may do this |
| E11-S12 | Docs & maturity truth; retire ADR-0002's "pluggable half unbuilt" line | **[autonomous]** | S11 | nothing still calls Rego locked |
| E11-S13 | Exit gate | **[autonomous]** | S00–S12 | **the E11 exit gate** |

## Phase 5 — EX complex in-tree examples / adopter tests / docs truth

Full INVEST stories in [p5-ex-complex-examples/spec.md](p5-ex-complex-examples/spec.md).
Operator ask (2026-08-15, **D-143**): thicker multi-field nested examples, tests, and docs
across **YAML, JSON, HCL, and tfvars** — not a one-field YAML happy path, and **not**
implementing P5-DEM. DEM remains the public-org demo + DEM-S00 routing + provider-broker
epic (0 `Test:`/`Verify:`/`Level:` annotations — that is why EX exists). EX extends
`examples/packs/`, `examples/archetypes/`, `assent test` fixtures, and `docs/`; closes
REF-EX C1–C8 in-tree. **No new HCL parser, no DEM-S00, no E10/E11/SEC-SC/AUD2, no schema
change, no `internal/core` edits.** `assent test` stays facts.yaml-stubbed. **Every story
`[autonomous]`.** REQ IDs `REQ-EX-S0n-nn`.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| EX-S01 | Docs + format-coverage inventory (paper-gate, both polarities) | **[autonomous]** | none | **do first** — claims cannot outrun dogfood |
| EX-S02 | Thicken topic-registry (YAML nested multi-field + nested-pointer rules) | **[autonomous]** | none | YAML complexity |
| EX-S03 | Thicken service-catalog (JSON nested objects/maps; D-061-safe) | **[autonomous]** | none | JSON complexity |
| EX-S04 | Thicken infra-vars (tfvars deeper nested maps) | **[autonomous]** | none | tfvars complexity |
| EX-S05 | HCL honesty: structured tfvars + `.tf` block → opaque → REVIEW | **[autonomous]** | S04 | fourth format; known-limitation fixture |
| EX-S06 | REF-EX C1–C4 in-tree fixtures | **[autonomous]** | S02, S03 | list-no-shrink, privilege-tier, wildcard-grant, soft-delete |
| EX-S07 | REF-EX C5–C8 (facts stubs + C8 known-limitation REVIEW) | **[autonomous]** | S04, S06 | closes D-071 demonstrators; C8 documented REVIEW |
| EX-S08 | Discover packs; wire `dogfood-examples` into `task check` + verify.yaml | **[autonomous]** | S01 | DEM-S12 wiring without demo-repo scope |
| EX-S09 | Product docs walkthrough byte-pinned to real complex-case CLI output | **[autonomous]** | S02–S07 | AUD-S06-style truth |
| EX-S10 | Exit gate: four formats, C1–C8, docs gates, schema freeze vs `v0.1.0` (D-132) | **[autonomous]** | S01–S09 | **the EX exit gate** |

**Dependency order:** S01 → S08 ∥ {S02, S03, S04} → S05 (after S04) → S06 (after S02/S03) →
S07 → S09 → S10. **Do first: S01.**

### P5-DEM — Public demo repositories + provider extensibility proof — spec: [p5-dem-demo-repos](p5-dem-demo-repos/spec.md)

Designed spec-first by **D-142**; the provider-credential gap found while designing is **OQ-32**.
Two public demo repos on a **two-tier contract**: tier 1 (`git clone && assent test .`) is
forge-independent and deliverable now on both; tier 2 (live MR/PR) is GitLab now, GitHub blocked
on E10-S18. Split by **governance shape, not by tool** — repo 1 is *referential* (an ACL names a
topic and a principal), repo 2 is *blast radius* + the opaque-change fallback. **15 stories,
S00–S14.** 🔴 **S00 is not optional and not cosmetic**: `(class, environment)` binding routing is
**not wired** (`run.go:493` fails closed on >1 binding; `test.go:366` collapses to strictest but
fails closed when bindings differ in `class`/`packs`/`require[]`), so both demo repos fail closed
in **both** tiers on **both** forges as designed. S13 is **operator-gated** — creating public
repos under the `PlatformRelay` org is outward-facing and **not** covered by AGENTS.md rule 2's
push grant to `PlatformRelay/assent` (D-142 judgment call (d)).

🔴 **No DEM story is implementable until its REQs carry `Test:`/`Verify:`/`Level:` annotations** —
the epic has **0** today against E10's 53, E11's 36 and E6's 42, so "green" is undefined for all
15 stories. Deferred to its own change, not waived.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| DEM-S00 | 🔴 Wire `(class, environment)` binding routing at the `cmd/assent` seam; **delete** `selectBindingForTest`'s strictest-collapse | **[autonomous · engine-grade · LGTM]** | none | **do first** — without it both repos fail closed in both tiers; supersedes D-060 |
| DEM-S01 | Provider host declarations: worked examples, docs, and a both-polarity gate | **[autonomous]** | S00 | closes G1 — today **every** shipped example's provider resolves to nothing |
| DEM-S02 | Provider-author guide | **[autonomous — but see OQ-32]** | S01 | 🔴 the guide **cannot publish** until OQ-32 is ruled; S02 may write everything else and must cross-link the OQ, but the `docs/vision.md:67` + ADR-0004 §1 amendment is operator-gated |
| DEM-S03 | Reference IdP-groups broker provider (`contrib/`, Entra + Keycloak adapters) | **[autonomous]** | S02 | the L2→L3 step is two files, zero lines of assent rebuilt |
| DEM-S04 | New sample shapes: Kafka ACL + ArgoCD Application | **[autonomous]** | S03 | referential shapes no shipped example exercises |
| DEM-S05 | `kafka-acl` class + cross-manifest reference rules | **[autonomous]** | S04 | doubles as the **E11-S01 tier-1 ceiling probe** (D-141) |
| DEM-S06 | `argocd-application` class + rules | **[autonomous]** | S05 | `builtin/resource-owner` (REF-GAP-1) end to end |
| DEM-S07 | Repo 1 assembly (`assent-demo-platform`, GitLab) | **[autonomous]** | S06 | tier 1 green on repo 1 |
| DEM-S08 | Raw `.tf` shape + HCL structuring truth | **[autonomous]** | S00 | states what assent can and cannot see in HCL |
| DEM-S09 | `tf-module-instance` class + rules | **[autonomous]** | S08 | magnitude / blast-radius archetype |
| DEM-S10 | `tf-backend` **ungoverned → REVIEW** (D-063) | **[autonomous]** | S09 | must **determine** unmatched-edit behaviour by running it, not assert it |
| DEM-S11 | Repo 2 assembly (`assent-demo-terraform`, GitHub) | **[autonomous]** | S10 | also **E10-S18's live adoption target** |
| DEM-S12 | Demo CI + mirror-drift gate | **[autonomous]** | S07, S11 | in-tree under `examples/demo/**` **so that `task check` will cover it — it does NOT today**: `task check` runs no example dogfood, `task dogfood-examples` is called by nothing, and `verify.yaml` duplicates it as a hardcoded pack loop. S12 must edit **both** |
| DEM-S13 | 🔴 Publish the repositories under the `PlatformRelay` org | **[operator]** | S12 | **operator-gated** — outward-facing, beyond rule 2's grant |
| DEM-S14 | Live tier-2 proof on GitLab | **[infra-gated · operator]** | S13 | **the DEM exit gate** |

## Phase 5 — SEC-SC OpenSSF Scorecard residuals

Full INVEST stories in [p5-sec-scorecard-residuals/spec.md](p5-sec-scorecard-residuals/spec.md).
Closes the two Scorecard code-scanning alerts that are real work after the 2026-08-13 security
sweep (Dependabot/CodeQL/secret-scanning/Sonar security all clean). **S01 is the slice to start.**
S02 cannot start until the operator creates the bestpractices.dev project (INBOX).

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| SEC-SC-S01 | Native Go fuzz targets on YAML/JSON/HCL differ (+ CI smoke) | **[autonomous]** | none | **do first** — Scorecard Fuzzing (#3); untrusted-byte crash/fail-open fence |
| SEC-SC-S02 | OpenSSF Best Practices passing badge + honest evidence page | **[operator-gated]** | operator creates the bestpractices.dev project | Scorecard CII-Best-Practices (#6); no fake README badge |

## Phase 5 — WG `writes: false` runtime gate (D-145)

No spec yet — decompose spec-first (`openspec/` change proposal) before implementation, per
AGENTS.md rule 4. D-145 resolved OQ-29 as option (a): `PolicyProfile.spec.writes: false` becomes
runtime-enforced on `assent run`, as a third zero-write arm of the existing write switch
(`cmd/assent/run.go`), refusing `forge.Reconcile` when `aggregate.CoverWithProfile` yields
`Result.WriteAllowed == false`. The published stopgap annotation on
`docs/architecture/policy-profiles.md` is removed by this lane and no earlier. Tracked here per
PROJECT-AUDIT-2026-08-18 ARCH-01/DOC-01: the D-145 commitment previously existed in no backlog
row. **Escalates back to P1 if unlanded at the next tag after v0.2.0's successor.**

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| WG-S01 | ⚠️ D-145: load covering profile on the run path; refuse `forge.Reconcile` when `WriteAllowed=false`; remove the docs stopgap annotation | **[autonomous · engine-grade · LGTM]** | spec-first proposal | published safety guarantee becomes true; verification target = zero forge writes against the fake under a `writes: false` profile (shape: `run_self_vouch_test.go`) |

## Phase 5 — AUD2 audit remediation (2026-08-18) — the risk-reduction wave

Full INVEST stories in [p5-aud2-audit-remediation/spec.md](p5-aud2-audit-remediation/spec.md).
AUD2 is the **"Next (risk reduction)" wave** named by
`agent-context/PROJECT-AUDIT-2026-08-18.md`. That audit's two P1 conditions (RELSE-01 changelog
regen, SEC-01 `toolchain go1.26.6`) were closed the same day and **v0.3.0 shipped** — so AUD2
carries **no release-condition story**, and that is a statement about the audit, not an omission.

Every story closes a finding the audit stated **together with its own verification recipe**;
those recipes are the `Verify:` lines in the spec, not invented ones. Three of the four findings
exist because a passing suite did not notice them (REL-01 is byte-identical across three
audits; TEST-02 is a mutation the auditor **demonstrated survives** every wired gate), so each
story's DoD names the mutation that must redden.

**Not claimed here:** the Later (hygiene) wave — TEST-01/03/04/05/06, SEC-04/05/06/07,
RELSE-03/04, ARCH-02/03/05, REL-04/05/06, DOC-02..06 — and **WG-S01**, which carries the
**LGTM** governance marker and is surfaced to the maintainer rather than auto-merged.

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| AUD2-S01 | REL-01/02/07: exec transport trio — bound stdout at `MaxResponseBytes`, `WaitDelay`, capture stderr | **[autonomous]** | none | closes the exec/HTTP containment asymmetry; a wedged provider cannot outlive its deadline |
| AUD2-S02 | ⚠️ REL-03: `errors.Is(err, forge.ErrNotFound)` discrimination on the provider-declaration fetch (D-130's sibling call site) | **[autonomous · engine-grade]** | none | a forge blip or token-scope misconfig can no longer masquerade as an absent declaration |
| AUD2-S03 | SEC-03: pin cosign signer identity + OIDC issuer in `hack/install.sh`, with a `SECURITY.md` drift gate | **[autonomous]** | none | `--require-signature` becomes a real guarantee, not a passing no-op |
| AUD2-S04 | TEST-02: kill the demonstrated `EffectChallenge` mutant (unit case + comparison-corpus entry) | **[autonomous]** | none | a wired gate reddens on the mutation the auditor proved survives |
| AUD2-S05 | Exit gate: S01–S04 dispositioned, wired **PR-visibly** into `task check` + `CHECK_STAGES` | **[autonomous]** | S01–S04 | **the AUD2 exit gate**; the RELSE-08 blind spot is not reproduced |

**Dependency order**: {S01 ∥ S02 ∥ S03 ∥ S04} — fully parallel, file-disjoint — → **S05**.
Path ownership is tabled in the spec. **`CHANGELOG.md` and this file's AUD2 status column are
Integrator-owned, not implementer-owned**: the changelog is regenerated *after* the final rebase
(rebasing rewrites the SHAs `task changelog-write` reads), and per-lane edits to it reddened
`main` twice in three days.

> ⚠️ **S05 must not be wired the way AUD-S18 was.** The `release-exitgate` job carries
> `if: github.event_name != 'pull_request'` (RELSE-08), so a gate wired only there is invisible
> to every PR — which is exactly how AUD-S18's own stale `CHECK_STAGES` pin survived four merges
> (INBOX 2026-08-16). AUD2's gate is a `task check` stage, and the **same commit** adds it to
> `CHECK_STAGES`.

**Found in flight, NOT an AUD2 story** (kept out of the table above on purpose: D-152 records
AUD2 as five stories and AUD2-S05's exit gate dispositions S01–S04, so a sixth row would move
that claim).

| ID | Follow-up | Execution | Depends on | Why it is separate |
| --- | --- | --- | --- | --- |
| AUD2-F01 | **DONE** — SEC-03's twin: `hack/release/verify-artifacts.sh:124` ran the same **unpinned** `cosign verify-blob --bundle` on the maintainer/CI path; `verify_cosign()` now pins the D-153 issuer/identity pair and `hack/release/install_cosign_pin_test.sh` grades it as the **third** file of the one drift gate | **[autonomous]** | AUD2-S03 (D-153's value) | found *by* S03, deliberately left alone there — not an owned path, and out of that story's stated scope |
| COUNT1-F01 | Two `go test` cache blind spots left inside `task test` / `task coverage` (`./...` and `./internal/...`): `internal/schemadrift` compares local `schemas/` against `git show origin/main:…` in a **subprocess**, and `internal/provider/isolation_test.go:29` `go build`s `./testdata/maliciousexec` at runtime | **[autonomous]** | — | found *by* the `-count=1` lane, deliberately left alone there: fixing them means carving packages out of `test`/`coverage` and losing an otherwise-honest whole-tree cache |
| CI-TOOLCHAIN | **DONE (D-158)** — `verify` was red on **every** PR, including a six-file Markdown diff with zero Go bytes, because `go-version: stable` rolled Go 1.26 → 1.27.0 while `GOLANGCI_LINT_VERSION` sat at `v2.12.2` (built with go1.26.x, so its `go/types` cannot read the 1.27 stdlib); pin bumped to `v2.13.1`, whose "go1.27 support" commit and `go 1.26.0` go.mod directive make it the first release that can | **[autonomous]** | — | not owned by any in-flight lane — the pin is a CI-only workflow env var, and the break blocked all of them |
| CI-TOOLCHAIN-F01 | The linter/toolchain coupling has **no early detector**: dependabot's `github-actions` ecosystem updates `uses:` refs, not `env:` literals, so `GOLANGCI_LINT_VERSION` rots silently until the next Go minor reds every PR at once (~6-monthly). Candidate: a text gate comparing the pinned linter release's go.mod `go 1.N` directive against the toolchain CI resolves, or an updater that watches the literal | **[autonomous]** | D-158 | deliberately out of scope of the fix lane: three lanes were parked behind it, and a new gate belongs with `hack/lint/**`, which was owned by a concurrent lane |
| BASH32-F01 | **DONE** — three `hack/**` gate scripts used bash 4+ features with no version guard; under stock macOS `/bin/bash` 3.2 `hack/docs/truthlag_pins_test.sh` died at its `declare -A`, skipped its final `OK:` banner and **exited 0**, so `task docs-gates` (and `task check`) read a gate that certified almost nothing as green. Fixed by a shared, per-script-parameterised `hack/lib/require-bash.sh` floor (4.0 / 4.0 / 4.4), enforced by a new `hack/lint/bash_version_guard_test.sh` wired as `task check` stage 20 (D-154) | **[autonomous]** | — | found *by* the AUD2 wave's macOS runs, not by any audit: CI is ubuntu/bash 5, so the hole is LOCAL-only and no CI lane could have surfaced it |

Sizing, from the S03 implementer's reading of the call site: `verify_cosign()`
(`hack/release/verify-artifacts.sh:118–125`) has the byte-identical unpinned
`cosign verify-blob --bundle "$bundle" "$archive"` shape, so the **same two flags with the same
D-153 value** close it — one call site, no bundle-discovery change (`find_sigstore_bundle`
already mirrors install.sh's candidate list). Three traps: **(1)** it runs via `task
release-verify` inside `hack/release/exitgate_test.sh`, i.e. on the **push-only**
`release-exitgate` job (RELSE-08), so the test needs the same offline **stubbed-cosign**
treatment rather than a real cosign; **(2)** the snapshot path ships no bundles, so the cosign
branch is skipped there and a naive test would be vacuous; **(3)** it must extend
`hack/release/install_cosign_pin_test.sh`'s drift comparison as a **third file** (~10 lines)
rather than start a second published truth (D-128).

Sizing, from the `-count=1` lane's measurements. **schemadrift is the real one**: the local
`schemas/` files are read in-process, so testlog records them and a local edit *does* invalidate
— but the baseline half is `git show origin/main:schemas/…` (`internal/schemadrift/drift_test.go:16,30`,
`d120_test.go:27`) run as a subprocess, which testlog cannot see. A cached PASS therefore goes
stale the moment `origin/main` moves with **no local schema edit**, which is exactly when a drift
gate is supposed to speak. **provider is narrower**: the production subject (`internal/provider`)
*is* linked into the test binary, so production changes invalidate normally; only edits to the
`testdata/maliciousexec` **fixture** are invisible. Two traps for whoever takes it: **(1)** the
obvious fix — `-count=1` on `test`/`coverage` — is the WRONG one, because it discards an honest
whole-tree cache to close two packages; scope it to the affected packages or make the subprocess
inputs visible to testlog instead; **(2)** `task coverage` is affected by inheritance
(`./internal/...` contains both packages), but its *gated number* stays honest either way — a
cached `-coverprofile` run regenerates the profile faithfully — so do not justify a change there
with the coverage percentage.

**AUD2 status: AUTONOMOUS COMPLETE** — S01–S05 landed. The four 2026-08-18 findings
(REL-01/02/07, REL-03, SEC-03, TEST-02) are closed, and `audit-aud2-exitgate-test` pins them
as the 19th `task check` stage — wired **PR-visibly** in the `verify` job, not only in the
push-only `release-exitgate` job, so the RELSE-08 blind spot is not reproduced. **AUD2-F01 is
now CLOSED too** — still a follow-up rather than an AUD2 story (D-152), so the five-story claim
stands. It added no stage: `CHECK_STAGES` is unchanged at 19, because the fix extends the
existing `release-install-cosign-pin-test` gate instead of starting a second published truth
(D-128). All three traps above were handled: the new sections are offline against the same
stubbed `cosign`, and the vacuity trap is closed by making the discriminator the **stub's argv
log** rather than the exit code — §5d requires that log to be non-empty and to carry both
pinned values, and §5e is the paired control proving a bundle-less (snapshot-shaped) `dist/`
leaves it empty. Mutation-proved red: each flag deleted from `verify-artifacts.sh`, the pre-fix
both-flags-missing shape, either field drifted from the other two files, the file disagreeing
with itself, and — the one that matters — both flags left textually in place while the cosign
branch is never entered. **COUNT1-F01 is now the one OPEN follow-up** — likewise not an AUD2
story (D-152), so the five-story claim is untouched by it either. It is also stage-neutral:
the `-count=1` lane that logged it added a *step* to the `verify` job and rewrote `Taskfile.yml`
recipes, not a `task check` stage, so `CHECK_STAGES` stays 19 there too.

## Phases 3–5

Epic paragraphs (goal, ADR constraints, exit gate, story seeds) in
[later-phases.md](later-phases.md). Summary:

| Phase | Epics | Gate |
| --- | --- | --- |
| 3 — Contracts first | P3-E1 schemas + contract fixture (incl. ApprovalEvidence + named-consumer fixture) · P3-E2 versioning/compat spec · P3-E3 example migration · P3-E4 lifecycle: phase/profiles/comparison (ADR-0018) · P3-E5 publication reconciliation protocol (ADR-0019) | strict end-to-end contract fixture validates (ADR-0017 §8, D-016); new ADRs 0018/0019 accepted at the freeze review |
| 4 — Walking skeleton | P4-E1 (+ rerun-idempotence gate, D-017) · **P2-E4-NS (OQ-24 timed run)** · holdout adjudication (OQ-25) | L3 skeleton green + **one real repo on live MRs** (D-012); north-star wording only after timed run |
| 5 — Implementation | E1–E7 **DONE**; **E7 AUTONOMOUS COMPLETE** (S01–S05+S08, D-087); **E8 AUTONOMOUS COMPLETE** ([p5-e8-renderer/spec.md](p5-e8-renderer/spec.md), S01–S14, D-098); **E9 AUTONOMOUS COMPLETE** ([p5-e9-distribution/spec.md](p5-e9-distribution/spec.md), S01–S13, D-099–D-111 CLOSED; Homebrew Formula live; PAT rotate optional); **PCS AUTONOMOUS COMPLETE** ([p5-pcs-policy-comparison/spec.md](p5-pcs-policy-comparison/spec.md), S01–S09, **D-057 closed**, D-118); **E10 UNLOCKED + DECOMPOSED** (D-140, [p5-e10-github-forge/spec.md](p5-e10-github-forge/spec.md), 19 stories, ADR-0021); **E11 IMPLEMENTATION UNLOCKED + DECOMPOSED** (D-141, [p5-e11-rego-backend/spec.md](p5-e11-rego-backend/spec.md), 14 stories); E12 **contract-unlocked** (D-017), not decomposed; E14 gated on Spike D; **E13 still locked** (D-012); **SEC-SC SPECIFIED, NOT STARTED** ([p5-sec-scorecard-residuals/spec.md](p5-sec-scorecard-residuals/spec.md), 2 stories — S01 autonomous fuzzing, S02 operator-gated Best Practices badge); **P5-EX AUTONOMOUS COMPLETE** (D-143, [p5-ex-complex-examples/spec.md](p5-ex-complex-examples/spec.md), S01–S10, exit gate `hack/examples/ex_exitgate_test.sh` — manual invocation only, not wired into `task check`/CI; **not** P5-DEM); **P5-DEM DESIGNED, NOT THIS ASK** (D-142, annotations still 0) | per-epic; E9 exit = tagged signed release + docs live + brew Formula (D-111); PAT rotate optional |

Named-consumer disposition (what unlocked, what stayed locked, and why):
[docs/planning/named-consumer-compat.md](../../docs/planning/named-consumer-compat.md).

## Reading order

1. [docs/vision.md](../../docs/vision.md) → [meta-plan](../../docs/planning/meta-plan.md)
2. ADR-0017 (contract model — newest, reshapes 0003/0005/0007/0009/0010/0011/0014/0015),
   then ADR-0013, 0014, 0015, 0016
3. [open-questions.md](../../docs/planning/open-questions.md) +
   [decisions.md](../../docs/decisions/decisions.md) (D-010, D-012, D-016)
4. This index → Phase-1 epic specs → Phase-2 epic specs → [later-phases.md](later-phases.md)
