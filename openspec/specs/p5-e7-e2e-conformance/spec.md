# P5-E7 — E2E & conformance infra

**Epic ID / REQ prefix:** `E7` / `REQ-E7-S0n-nn`.

**Problem**: E4 closed the **autonomous half** of the forge port — `internal/forge/conformance`
holds L2 hermetic goldens (SHA-guard, P3-E5 replay, spoof filter) and `cmd/assent` proves
assent-policy BLOCK + hermetic Snapshot→Resolve→Reconcile (`TestE4ExitGateHermeticForgePath`,
D-079). P4-E1 scaffolded L3 wiring (`test/e2e/skeleton_test.go`, `go vet -tags e2e ./...` in
verify) and Spike B chose **testcontainer as CI default, kind for local/demo** (D-026). What
remains for E7 is **productionization and CI wiring**, not re-proving E4: sample-repo seeding
for all three `examples/repos/` shapes, a **forge-neutral conformance catalog** (ADR-0005) that
indexes E4 cases plus the remaining ADR-0015/0017 adversarial cases exercisable hermetically,
an **explicit determinism gate** step (P4-E1-S12 intent — today buried in scattered `-count=2`
tests), **security jobs** (gitleaks already in verify; sanitization check still local-only),
and an **L3 live re-run harness** so every conformance case has a home when infra is present.

**Key ground truth (de-risks the epic):**
- **Reuse E4, don't re-implement:** `internal/forge/conformance/**`, E4-S07–S09 tests, P3-E5
  fixtures, `hack/spikes/e2e/{boot-testcontainer,boot-kind,smoke}.sh`, and P4 `test/e2e/` are
  the substrate — E7 **indexes, extends, and wires** them.
- **Autonomous exit is achievable without live GitLab:** same pattern as E4-S10 / E4-S11 — S01–S05
  + S08 close the epic; S06 (kind lab) and S07 (live L3) are **`[infra-gated]`** and optional for
  merge.
- **PolicyComparisonSuite full runner stays deferred (D-057):** E6-S09 seeded `assent compare`;
  the six-delta / five-gate suite is **not** E7 scope.
- **`internal/core` stays I/O-free** (`TestCorePurity`); new adversarial cases that touch arming/
  doctor/run wiring live at `cmd/assent/**` or `internal/forge/conformance/**`, catalogued together.

**Scope**: (S01) Spike-B profile task targets + operator docs; (S02) sample-repo generator for all
three shapes; (S03) conformance catalog manifest + remaining hermetic adversarial cases; (S04)
determinism gate CI step; (S05) sanitization check in verify (+ confirm gitleaks/e2e-vet); (S06)
kind local lab scaffold (D-038); (S07) L3 live forge conformance harness; (S08) autonomous exit
gate. **Infra-gated:** S06 (docker+kind), S07 (live GitLab / testcontainer + token).

**Non-goals** (fenced): **Re-implementing E4 Snapshot/Resolve/Reconcile** (done); **GitHub e2e**
(E10, D-012 locked — catalog stubs only); **renderer / PresentationModel** (E8); **release /
goreleaser / cosign** (E9); **full PolicyComparisonSuite runner** (D-057 deferred epic);
**replacing httptest cassettes with live-only proof**.

**ADRs**: 0005 (forge-neutral conformance suite = executable port definition), 0006 (L3 e2e
strategy), 0015 §1/§2/§4/§8 (trust boundaries — self-vouch, SHA-guard, protected pipeline,
execution-authority matrix), 0017 §1/§4/§115–116 (merge-result pins, arming preconditions,
e2e build-tag vet). **Reuse**: E4 conformance package, P4 `test/e2e/`, Spike-B scripts, P1-E1
`examples/repos/**`, `hack/check-sanitization.sh`. **New**: generator script, conformance
catalog, determinism CI step, verify sanitization job, kind lab task targets (optional), L3
harness.

**Executability**: S01–S05 + S08 **`[autonomous]`** — pure scripts, CI yaml, hermetic tests, no
live forge. S06 **`[infra-gated: docker+kind]`**. S07 **`[infra-gated: live GitLab / token]`**.
TDD; determinism double-run on new goldens; `TestCorePurity` untouched for `internal/core`.

**Judgment calls (decide-and-log / operator):**
(a) **DECIDED — E7 extends E4 conformance; it does not fork a second suite.** The catalog
manifest lists E4 REQ-IDs and adds E7-only cases; implementation stays in
`internal/forge/conformance` + thin `cmd/assent` run/doctor tests. Logged **D-080**.
(b) **DECIDED — Autonomous exit (S08) does not require live L3.** S07 mirrors E4-S11: skip
without `ASSENT_E2E_GITLAB` / token; operator-attested green run is evidence, not a merge
blocker. Logged **D-081**.
(c) **DECIDED — Sample-repo generator reads only committed `examples/repos/` trees.** No network
clone, no private-shape fetch; the generator copies/archives local content into a GitLab project
via API when S07/S06 infra is present. Logged **D-082**.
(d) **DECIDED — kind lab (S06) is optional; testcontainer remains CI default.** Spike B / D-026
unchanged; S06 promotes D-038 scaffold when an operator lane wants durable local demo — not on
the autonomous critical path. Logged **D-083**.
(e) **DECIDED — Conformance catalog is forge-neutral; GitHub adapter cases are stubbed.** Each
case records `gitlab | both | github-deferred`; live GitHub proof waits for E10 unlock. Logged
**D-084**.
(f) **DECIDED — Catalog lives at `internal/forge/conformance/catalog.yaml`.** Co-located with the
executable suite (ADR-0005); not under `docs/contracts/` (that tree is frozen fixture prose).
Logged **D-085**.
(g) **DECIDED — `.github/workflows/verify.yaml` is the authoritative CI superset** (determinism,
sanitization, e2e-vet, gitleaks, race tests, coverage). `task check` stays the local pre-commit
unit gate (fmt/vet/lint/test/coverage/build) and is **not** extended to duplicate verify-only
steps — PR merge requires verify green. Logged **D-086**.

**Dependency order**: **S01 → S02 → S03 → {S04 ∥ S05} → S08**; **S06** after S01 when operator
claims kind lab; **S07** after S02 + S03 when infra available (may reuse E4-S11 evidence).
**Closes P4-E1-S12 CI wiring gap: S04. Closes P1-E1-S01-02 sanitization-in-CI gap: S05.**

**Coordination note:** S03 adds catalog rows for tests outside `internal/forge/conformance`
(e.g. `cmd/assent/doctor_forge_test.go`, `run_self_vouch_test.go`) — **catalog-index only**,
no duplicate §4 doctor test. New run-path cases (§8, §4 max_age) follow E4-S08 patterns in
`cmd/assent/run_*`.

**Engine-grade / fail-safety review:** S03 (adversarial catalog cases), S07 (live SHA-guard +
self-vouch replay), S08 (exit gate).

---

## E7-S01 — Spike-B e2e profile: task targets + operator docs [autonomous]

**As a** maintainer **I want** first-class `task` entry points and docs for the Spike-B profiles
**so that** e2e wiring cannot rot and operators know which profile to boot.

**Goal**: Add `task e2e-vet` (alias: compile+vet all `e2e`-tagged packages — mirrors verify's
`go vet -tags e2e ./...`), document `ASSENT_E2E_GITLAB` / `ASSENT_SPIKE_PROFILE` in
`test/e2e/README.md`, and cross-link Spike-B scripts from `hack/spikes/e2e/README.md` (new,
short). Keep testcontainer as CI default; kind as local/demo per D-026/D-083.

**Dependencies**: P4-E1-S09 (skeleton), P2-E2 (Spike B decision).

**Definition of done**: `task e2e-vet` exits 0 on main; README lists both profiles with boot
commands; no live infra required.

Requirements:
- **REQ-E7-S01-01** — `task e2e-vet` runs `go vet -tags e2e ./...` and fails on compile drift.
  Test: `Taskfile.yml`; Verify: `task e2e-vet`; Level: L0
- **REQ-E7-S01-02** — `test/e2e/README.md` documents testcontainer (CI default) and kind
  (local/demo), env vars, and skip behaviour when unset. Test: same; Verify:
  `grep -q ASSENT_E2E_GITLAB test/e2e/README.md`; Level: doc
- **REQ-E7-S01-03** — Spike-B script paths are referenced from repo docs (no dead-end scaffold).
  Test: `hack/spikes/e2e/README.md`; Verify:
  `grep -q boot-testcontainer hack/spikes/e2e/README.md`; Level: doc

---

## E7-S02 — Sample-repo generator: seed all three `examples/repos/` shapes [autonomous]

**As a** maintainer **I want** one generator that materializes GitLab projects from the committed
sample corpus **so that** L3 runs and smoke scripts do not hand-copy trees.

**Goal**: Add `hack/e2e/seed-sample-repo.sh` (+ `task e2e-seed`) accepting `--shape
{topic-registry,service-catalog,infra-vars}` and `--endpoint URL` (+ token env). The script
archives the matching `examples/repos/<shape>/` tree, creates/updates a GitLab project, pushes
initial commit, and prints project path + default branch for MR scripts. Hermetic unit: `--dry-run`
prints the file manifest without network. Judgment call (c): local trees only.

**Dependencies**: P1-E1-S01 (three shapes committed), E7-S01.

**Definition of done**: dry-run lists every governed file for each shape; smoke.sh can delegate
to the generator for topic-registry (refactor optional in S07, not required for S02 DoD).

Requirements:
- **REQ-E7-S02-01** — dry-run for `topic-registry` lists ≥1 YAML under `topics/` from the committed
  tree. Test: `hack/e2e/seed-sample-repo.sh`; Verify:
  `hack/e2e/seed-sample-repo.sh --dry-run --shape topic-registry | grep -q topics/`; Level: L0
- **REQ-E7-S02-02** — all three shapes are accepted (`topic-registry`, `service-catalog`,
  `infra-vars`). Test: same; Verify:
  `hack/e2e/seed-sample-repo.sh --dry-run --shape service-catalog && hack/e2e/seed-sample-repo.sh --dry-run --shape infra-vars`; Level: L0
- **REQ-E7-S02-03** — script refuses to run without `--endpoint` unless `--dry-run` (fail-closed).
  Test: same; Verify: `! hack/e2e/seed-sample-repo.sh --shape topic-registry 2>/dev/null`; Level: L0

---

## E7-S03 — Conformance catalog + remaining hermetic adversarial cases [autonomous · engine-grade]

**As a** maintainer **I want** a forge-neutral catalog of every ADR-0015/0017 adversarial case
with hermetic proof where possible **so that** ADR-0005 is the executable port definition and later
epics know where L3 cases live.

**Goal**: (1) Add `internal/forge/conformance/catalog.yaml` (judgment call (f), **D-085**)
listing each case: id, ADR clause, level (L1/L3), REQ id, test function, forge scope
(`gitlab|both|github-deferred`). Index all E4-S05–S09 + S08 cases, including E4-only tests
outside `internal/forge/conformance` (e.g. doctor, run self-vouch). (2) Implement **new hermetic**
cases not yet covered by any green test:
- **ADR-0015 §8** — fork/untrusted contributor context → run path performs **zero** forge writes
  (advisory-only).
- **ADR-0017 §4** — controlling fact past `facts.max_age` → arming blocked (injected clock;
  `cmd/assent` test, catalogued alongside forge cases).

**Catalog-index only (no duplicate test):** **ADR-0015 §4** unprotected pipeline → index existing
**REQ-E4-S05-05** / `TestDoctorForgeInsecureCITopology` in `cmd/assent/doctor_forge_test.go`.

Do **not** duplicate E4 SHA-guard / P3-E5 replay / self-vouch / §4 pipeline tests — reference them
in the catalog.

**Dependencies**: E4-S07–S10 (existing cases), E7-S01.

**Definition of done**: catalog lists ≥8 cases (E4 + E7 new); every L1 row has a green test;
`go test ./internal/forge/conformance/...` zero skipped autonomous cases.

Requirements:
- **REQ-E7-S03-01** — catalog indexes E4 SHA-guard cases with REQ-E4-S07-* ids and test names.
  Test: catalog file; Verify:
  `grep -q TestConformanceTargetAdvancedRejected internal/forge/conformance/catalog.yaml`; Level: doc
- **REQ-E7-S03-02** *(catalog-index · ADR-0015 §4)* — catalog row indexes **REQ-E4-S05-05** /
  `TestDoctorForgeInsecureCITopology` (author-editable-only CI → insecure topology /
  `ArmEligible=false`); **no new duplicate doctor test**. Test: `catalog.yaml`; Verify:
  `grep -q TestDoctorForgeInsecureCITopology internal/forge/conformance/catalog.yaml &&
  grep -q REQ-E4-S05-05 internal/forge/conformance/catalog.yaml`; Level: doc
- **REQ-E7-S03-03** *(fail-safe · ADR-0015 §8)* — fork/untrusted MR context → zero Approve/Merge/
  Reconcile writes. Test: `cmd/assent/run_authority_test.go`; Verify:
  `go test ./cmd/assent/... -run TestRunForkContextAdvisoryOnly`; Level: L1
- **REQ-E7-S03-04** *(fail-safe · ADR-0017 §4)* — expired controlling authorization fact blocks
  arming even when decision would otherwise APPROVE. Test: `cmd/assent/run_arming_test.go`; Verify:
  `go test ./cmd/assent/... -run TestRunExpiredFactBlocksArming`; Level: L1
- **REQ-E7-S03-05** — catalog marks GitHub-only rows `github-deferred` (E10). Test: catalog;
  Verify: `grep -q github-deferred internal/forge/conformance/catalog.yaml`; Level: doc

---

## E7-S04 — Determinism gate: explicit CI double-run step [autonomous]

**As a** maintainer **I want** a named CI step that double-runs engine + conformance goldens
**so that** nondeterminism fails the PR gate visibly (P4-E1-S12, GUIDELINES §5).

**Goal**: Add a verify workflow step (and `task determinism` local mirror) running `-count=2` over
named packages and gate tests:
- `./internal/core/aggregate/...` — `TestDeterminismDoubleRun`, `TestD016ReproductionDoubleRunStable`
- `./internal/core/decision/...` — `TestReportDoubleRunStable`
- `./internal/forge/conformance/...` — `TestConformanceTargetAdvancedRejected`,
  `TestConformanceRerunIdempotence`, `TestConformanceDuplicateRepair`
- `./internal/forge/...` — `TestMergeFailsClosedDoubleRun` (SHA-guard receipt)

Step name must be greppable (`determinism gate`). Judgment call (g): verify.yaml is authoritative;
`task determinism` mirrors the step for local pre-push, not `task check`.

**Dependencies**: E4 conformance, E2-S10, P4-E1-S12 intent.

**Definition of done**: CI step fails if any listed test diverges on second run; `task determinism`
matches CI locally.

Requirements:
- **REQ-E7-S04-01** — verify workflow contains an explicit determinism step. Test:
  `.github/workflows/verify.yaml`; Verify:
  `grep -qi determinism .github/workflows/verify.yaml`; Level: L0
- **REQ-E7-S04-02** — step runs the named packages with `-count=2` (see Goal list). Test: same;
  Verify: `task determinism`; Level: L0
- **REQ-E7-S04-03** — injecting map-order nondeterminism would fail the gate (regression guard
  documented in test comment — existing `TestDeterminismDoubleRun`). Test:
  `internal/core/aggregate/aggregate_test.go`; Verify:
  `go test ./internal/core/aggregate/... -run TestDeterminismDoubleRun -count=2`; Level: L0

---

## E7-S05 — Security jobs: sanitization in verify + e2e-vet confirmation [autonomous]

**As a** maintainer **I want** the P1 sanitization gate and e2e compile vet enforced on every PR
**so that** D-002 hygiene and ADR-0017 §115–116 cannot regress silently.

**Goal**: Wire `hack/check-sanitization.sh` into `.github/workflows/verify.yaml`; confirm gitleaks
step remains (already present). Document that **verify is the CI superset** (judgment call (g),
**D-086**); `task check` is not extended with verify-only steps. No new secret scanners — E9 owns
further hardening.

**Dependencies**: P1-E1-S01-02, E7-S01.

**Definition of done**: verify runs sanitization; gitleaks still runs; `task e2e-vet` documented
as part of PR checklist in spec/backlog.

Requirements:
- **REQ-E7-S05-01** — verify runs `hack/check-sanitization.sh` and fails on denylist hits. Test:
  `.github/workflows/verify.yaml`; Verify: `grep -q check-sanitization .github/workflows/verify.yaml`; Level: L0
- **REQ-E7-S05-02** — gitleaks step still present on PR/push to main. Test: same; Verify:
  `grep -q gitleaks .github/workflows/verify.yaml`; Level: L0
- **REQ-E7-S05-03** — `go vet -tags e2e ./...` remains in verify (P4-E1-S09). Test: same; Verify:
  `grep -q 'tags e2e' .github/workflows/verify.yaml`; Level: L0

---

## E7-S06 — kind local lab scaffold [infra-gated: docker + kind]

**As a** operator **I want** durable `task kind-up` / `kind-down` targets **so that** local demo
does not re-run cold-boot Spike measurements every time (D-038).

**Goal**: Promote `hack/kind/` from README-only to idempotent `setup.sh` / `status.sh` /
`teardown.sh` + Task targets wrapping Spike B's `boot-kind.sh` CE-in-pod pattern. Optional seed
hook calling E7-S02 generator. **Not** on autonomous critical path.

**Dependencies**: E7-S01, E7-S02; operator docker+kind.

**Definition of done**: `task kind-up` brings GitLab to readiness or prints actionable error;
`task kind-down` tears down; documented in `hack/kind/README.md`.

Requirements:
- **REQ-E7-S06-01** — `task kind-up` exits 0 with GitLab readiness on a machine with docker+kind.
  Test: `Taskfile.yml`; Verify: `task kind-up` (skip in CI without kind); Level: L3
- **REQ-E7-S06-02** — `task kind-down` tears down the `assent` cluster without orphan containers.
  Test: same; Verify: `task kind-down`; Level: L3

---

## E7-S07 — L3 live forge conformance harness [infra-gated: live GitLab / token]

**As a** maintainer **I want** the conformance catalog replayed against a live GitLab project
**so that** adapter semantics are proven beyond httptest (ADR-0005 L3, ADR-0017 §115–116).

**Goal**: Implement `test/e2e/forge_conformance_test.go` (`//go:build e2e`) that, when
`ASSENT_E2E_GITLAB` + token are set: seeds via E7-S02, opens an MR, runs `bin/assent run`, and
asserts live outcomes for catalog cases marked L3 (minimum: SHA-guard rejection on stale SHA,
self-vouch BLOCK, rerun idempotence — mirroring E4-S11 scope). Skip cleanly when env unset.
Absorbs E4-S11 as the canonical live re-run (backlog cross-ref).

**Dependencies**: E7-S02, E7-S03, E4-S10; operator token (D-037 lab or testcontainer).

**Definition of done**: one operator-attested green run recorded in `docs/decisions/evidence/` **or**
skip documented when infra absent (does not block S08).

Requirements:
- **REQ-E7-S07-01** — live SHA-guard: push after evaluation → merge rejected observable. Test:
  `test/e2e/forge_conformance_test.go`; Verify:
  `go test -tags e2e ./test/e2e/... -run TestLiveSHAGuard` (skip without env); Level: L3
- **REQ-E7-S07-02** — live self-vouch: MR touching `.assent/**` → BLOCK, zero bot threads. Test:
  same; Verify: `go test -tags e2e ./test/e2e/... -run TestLiveAssentPolicyBlock`; Level: L3
- **REQ-E7-S07-03** — second assent run creates zero new threads (rerun idempotence). Test: same;
  Verify: operator evidence pointer in `decisions.md`; Level: doc (infra)

---

## E7-S08 — Exit gate: autonomous infra wired + catalog green [autonomous · engine-grade]

**As a** maintainer **I want** the autonomous half of E7 proven in CI **so that** E8/E9 can
claim L3 homes without re-litigating infra.

**Goal**: (1) **verify.yaml** (authoritative CI superset, D-086) enforces S04 determinism +
S05 sanitization + e2e-vet; (2) conformance catalog complete and every L1 case green (indexed or
new); (3) `task e2e-vet` + generator dry-runs green locally; (4) `task check` green (local unit
gate — unchanged scope); (5) backlog marks E7 autonomous slice closed; (6) `git diff schemas/` == 0.

**Dependencies**: E7-S01..S05, S03.

**Definition of done**: exit-gate checklist green; D-080–D086 recorded; S06/S07 optional notes
in backlog.

Requirements:
- **REQ-E7-S08-01** — verify enforces determinism + sanitization + e2e-vet (S04+S05); merge
  blocked on verify failure (D-086). Test: `.github/workflows/verify.yaml`; Verify:
  `grep -qi determinism .github/workflows/verify.yaml &&
  grep -q check-sanitization .github/workflows/verify.yaml`; Level: L1
- **REQ-E7-S08-02** — conformance catalog lists all E4 + E7 L1 cases with green tests (indexed or
  new). Test: `internal/forge/conformance/catalog.yaml`; Verify:
  `go test ./internal/forge/conformance/... ./cmd/assent/... -run 'Conformance|ForgeInsecure|ForkContext|ExpiredFact|AssentPolicySelfModification'`; Level: L1
- **REQ-E7-S08-03** — backlog + later-phases mark E7 spec authoritative and autonomous slice
  closable without live L3. Test: `openspec/specs/backlog.md`; Verify:
  `grep -q p5-e7-e2e-conformance/spec.md openspec/specs/backlog.md`; Level: doc

---

## Epic definition of done

| Gate | Criterion |
| --- | --- |
| **Autonomous (S08)** | S01–S05 + S08 green in CI; catalog complete; generator dry-runs pass; determinism + sanitization + e2e-vet wired |
| **Infra optional (S06, S07)** | kind lab + live L3 harness skip cleanly; operator evidence when run |
| **Schemas frozen** | `git diff schemas/` == 0 |
| **Next epic** | E8 renderer may proceed; E9 release inherits security/determinism wiring |

**Story count:** 8 — **6 autonomous**, **2 infra-gated** (S06, S07).
