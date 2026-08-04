# P5-PCS — PolicyComparisonSuite full runner (D-057)

**Epic ID / REQ prefix:** `PCS` / `REQ-PCS-S0n-nn`.

**Problem**: E6-S09 seeded `assent compare` with the smallest honest promotion-comparison
slice (D-057): one immutable `ReplayBundle`, explicit baseline/candidate `MergePolicy`
activations, two of six delta classifiers (`newly-auto-mergeable`, `explanation-only`), one
of five promotion gates (`bounded-auto-merge-widening`), and fail-closed `ErrUnclassifiable`
for everything else. Adopters and the named-consumer reference use case still cannot run the
**full** ADR-0018 `PolicyComparisonSuite` — the versioned corpus of pinned ReplayBundles,
closed six-kind taxonomy, five machine-enforceable promotion gates, per-delta `acceptedDeltas`
allowlist, and schema-valid `ComparisonRecord` emission — before promoting a candidate
`PolicyProfile`. PCS **extends the seed**; it does not re-implement `assent test` or touch
forge/provider paths.

**Key ground truth (de-risks the epic):**
- **Seed exists and stays the regression anchor:** `internal/compare/compare.go` +
  `cmd/assent/compare.go` (E6-S09) already load `ReplayBundle`, call
  `aggregate.CoverWithProfile` (byte-unchanged `internal/core`), classify two kinds, apply one
  gate, and map verdict → exit code. PCS stories **grow** `internal/compare/**` and the CLI
  shell — they do not fork a second runner.
- **Schemas frozen:** `schemas/comparison/v1alpha1/{comparison-record,comparison-suite}.schema.json`
  + `schemas/decision/v1alpha1/replay-bundle.schema.json` are the authority. Epic DoD:
  **`git diff schemas/` == 0** — no schema edits; drift guards (`TestKindConstantsAreFrozenTaxonomy`,
  `TestGateIsFrozenSuiteConformant`) already prove seed constants ⊆ frozen enums.
- **Reuse boundary (D-057):** `CoverWithProfile` resolves **write authority** from the
  precedence table; it does **not** switch evaluated policy by profile. PCS-S01 wires
  `Profile.spec.packs` → combined `MergePolicy` (same `combinePolicies`/`loadCatalogueInput`
  pattern as `cmd/assent/test.go`) so the decision delta flows from the profile's pack
  activation, not hand-authored side-by-side YAML files.
- **Side-effect-free invariant (ADR-0018 §3):** comparison never calls `Reconcile` or any
  forge write; both profiles are evaluated as recorders for delta classification.
- **Exit-code contract (ADR-0018):** full suite mode maps 1:1 to gate IDs (0 all-pass,
  1–5 first failing gate). Seed single-dir mode (E6-S09) currently uses 0/1/2 — PCS-S07
  migrates to the ADR table and reserves a distinct code for fail-closed errors (judgment
  **D-112**).

**Scope**: (S01) profile→pack activation resolver; (S02) classifiers
`destructive-or-authorization-intervention-missed` + `stricter-intervention-added`; (S03)
classifiers `subject-or-obligation-uncovered` + `score-threshold-change`; (S04)
`ComparisonRecord` per-delta emission; (S05) five-gate evaluator +
`acceptedDeltas` per-delta-identity allowlist; (S06) `PolicyComparisonSuite` loader +
digest-verified multi-case runner; (S07) CLI suite mode + ADR-0018 exit codes + seed dir
compat; (S08) adversarial corpus + CI dogfood; (S09) exit gate.

**Non-goals** (fenced): **Schema changes** (`git diff schemas/` must stay 0); **GitHub /
forge / provider / render paths**; **E10/E11/E12/E13**; **replacing `assent test`**;
**live promotion writes** (compare stays recorder-only); **Rego backend**; **observe-phase
forge-state diffs** (compare classifies decision/findings deltas only, per ADR-0018);
**modifying `internal/core`** except through existing `CoverWithProfile` entry (same
boundary as E6-S09 seed).

**ADRs / decisions**: D-057 (seed + defer full runner), ADR-0018 §3 (taxonomy, suite, gates,
CLI contract), ADR-0014 amendment (explanation-only never trips gates),
`docs/planning/policy-lifecycle-promotion-gates.md`. **Reuse**: E6-S09 seed,
`aggregate.CoverWithProfile`, `policy.LoadProfile`/`LoadMergePolicy`/`LoadConfig`,
`cmd/assent/{test,catalogue}.go` (`loadCatalogueInput`, `combinePolicies`, `selectBinding`),
frozen comparison + replay-bundle schemas, `schemas.ComparisonRecordSchema` /
`ComparisonSuiteSchema`. **New**: `internal/compare/{classify,gates,record,suite}.go` (or
equivalent split), `examples/comparison/**` corpus, suite-mode CLI flags, gate-scoped exit
mapping.

**Executability**: **every story `[autonomous]`** — pure `internal/compare` over frozen
fixtures, no live forge/provider/token. TDD; determinism double-run on comparisons;
`TestCorePurity` untouched for `internal/core`. **Engine-grade / fail-safety review:** S02,
S03 (classifier correctness), S05 (gate + allowlist footguns), S07 (exit-code honesty), S09
(exit gate).

**Judgment calls (decide-and-log / operator):**
(a) **RECOMMENDED — profile→pack activation reuses the `assent test` pack combiner.** Resolve
each `Policy.spec.packs[]` entry against the repo's loaded pack docs via
`loadCatalogueInput` + `combinePolicies`; baseline/candidate profiles name pack sets, not raw
MergePolicy files. Single-dir seed layout (`baseline.yaml`/`candidate.yaml`) remains for
one-case dev fixtures only. Log at implementation as **D-112** (or next free D-nnn).
(b) **RECOMMENDED — corpus root `examples/comparison/<suite>/`.** Each suite directory holds
`suite.yaml` (PolicyComparisonSuite), `cases/<caseId>/bundle.json` (+ optional
`replayBundleRef` paths), and committed `records/` golden ComparisonRecords for regression.
Immutable corpus rule: fixed `caseId` → fixed `replayBundleDigest`; revise by minting new
`caseId`. Log **D-113**.
(c) **RECOMMENDED — replay bundle digest = canonical JSON SHA-256** over the bundle bytes
(same canonicalization discipline as ADR-0017 §9 hash vectors; reuse existing hash helper if
present, else minimal canonical marshal in `internal/compare`). Mismatch vs suite
`replayBundleDigest` fails closed before evaluation. Log **D-114**.
(d) **RECOMMENDED — full suite mode adopts ADR-0018 exit codes 0–5** (gate-id order in
`docs/adr/0018-policy-lifecycle-phase-profile-comparison.md`); **fail-closed classification,
schema validation, and digest mismatch exit 6** (distinct from every gate code). Migrate E6-S09
seed tests (widening fail 1→4, unclassifiable stays non-gate) in PCS-S07. Log **D-115**.
(e) **RECOMMENDED — gate evaluation order is fixed table order** (destructive → authorization →
obligation-removal → auto-merge-widening → explicitly-accepted); first failing gate sets
process exit; stdout/report lists **all** gate results. Log **D-116**.
(f) **RECOMMENDED — per-delta classification is mutually exclusive at the case level** but
a case may emit **multiple deltas** (different rule/subject identities). Each delta kind is
assigned by priority: missed intervention > uncovered > newly-auto-mergeable >
score-threshold > stricter-added > explanation-only (documented in classifier — no free-text
"other"). Unclassified real differences remain `ErrUnclassifiable` (fail-closed). Log **D-117**.

**Dependency order**: **S01 → {S02, S03} → S04 → S05 → S06 → S07 → S08 → S09**. S02/S03 may
parallelize after S01 but both must land before S04. **Closes D-057 deferred scope: S02–S08.
Do first: PCS-S01** — without profile→pack activation, later classifiers cannot prove
profile promotion semantics the ADR describes.

**Coordination note:** PCS touches `internal/compare/**` and `cmd/assent/compare.go` only —
file-disjoint from E11/E12 if those epics start in parallel. Do not modify E6-S09 seed
behaviour until PCS-S07 explicitly migrates exit codes.

---

## PCS-S01 — Profile→pack activation resolver [autonomous]

**As a** policy maintainer **I want** baseline/candidate profiles to activate their declared
pack sets **so that** comparison deltas reflect profile promotion, not hand-picked MergePolicy
files.

**Goal**: Add `compare.ResolveActivation(repoDir, profileName, bind, precedence, profiles)`
(or equivalent) that loads each named pack via the existing catalogue path, combines multi-doc
pack policies with `combinePolicies`, applies profile phase ceiling, and returns a
`compare.Profile` ready for `CoverWithProfile`. Fail closed on unknown pack names, dangling
precedence refs, or single-writer violations (reuse `policy.CoveringProfiles` /
`aggregate.ResolveProfile` errors). Unit-test with a minimal two-pack fixture mirroring
`examples/packs/**` layout.

**Dependencies**: E6-S09 seed (existing `Profile` struct + `Compare` entry).

**Definition of done**: resolver tests pass; seed `Compare` path unchanged; no `internal/core`
diff.

Requirements:
- **REQ-PCS-S01-01** — Given a Profile document whose `spec.packs` lists two existing example
  packs, when `ResolveActivation` runs against that repo, then the returned `Profile.Policy`
  unions rules from both packs (same semantics as `combinePolicies`). Test:
  `internal/compare/activation_test.go`; Verify:
  `go test ./internal/compare/... -run TestResolveActivationCombinesPacks`; Level: L0
- **REQ-PCS-S01-02** — Given a Profile referencing a non-existent pack name, when resolved,
  then error is fail-closed (no partial policy). Test: same; Verify:
  `go test ./internal/compare/... -run TestResolveActivationUnknownPack`; Level: L0
- **REQ-PCS-S01-03** — Given the same ReplayBundle and two profiles differing only in pack
  set, when `Compare` runs with resolved activations, then baseline/candidate decisions differ
  observably (proves activation drives delta). Test: same; Verify:
  `go test ./internal/compare/... -run TestCompareUsesResolvedProfilePacks`; Level: L0

---

## PCS-S02 — Classifiers: missed intervention + stricter intervention [autonomous · engine-grade]

**As a** promotion reviewer **I want** missed destructive/authorization interventions and
newly stricter interventions classified **so that** the zero-missed gates can fire on real
deltas.

**Goal**: Extend `classify` (or dedicated helpers) to emit
`destructive-or-authorization-intervention-missed` when baseline had a BLOCK/require-review
intervention on an identity the candidate lacks or weakens without reaching APPROVE, and
`stricter-intervention-added` when candidate adds BLOCK/require-review/challenge where
baseline had none (same decision or lenient). Preserve E6-S09 behaviour for
`newly-auto-mergeable` and `explanation-only`. Golden tests per kind; unclassified edge
(BLOCK→REVIEW without APPROVE) stays `ErrUnclassifiable` until explicitly covered.

**Dependencies**: PCS-S01.

**Definition of done**: both kinds classify in unit tests; seed tests still green; taxonomy
constants still validate against frozen schema.

Requirements:
- **REQ-PCS-S02-01** — Given baseline BLOCK on a finding identity and candidate with no
  finding on that identity (decision APPROVE or lenient REVIEW), when classified, then kind
  is `destructive-or-authorization-intervention-missed`. Test:
  `internal/compare/classify_test.go`; Verify:
  `go test ./internal/compare/... -run TestClassifyMissedIntervention`; Level: L0
- **REQ-PCS-S02-02** — Given baseline APPROVE with no intervention on an identity and
  candidate BLOCK/require-review on that identity, when classified, then kind is
  `stricter-intervention-added`. Test: same; Verify:
  `go test ./internal/compare/... -run TestClassifyStricterInterventionAdded`; Level: L0
- **REQ-PCS-S02-03** — E6-S09 seed tests (`TestCompareClassifiesOneDelta`,
  `TestCompareExplanationOnlyNeverTripsGate`, `TestCompareUnclassifiableDeltaFailsClosed`) remain
  green without weakening fail-closed semantics. Test: `internal/compare/compare_test.go`;
  Verify: `go test ./internal/compare/... -run 'TestCompare(C|Unclassifiable|E)'`; Level: L0

---

## PCS-S03 — Classifiers: obligation uncovered + score threshold [autonomous · engine-grade]

**As a** promotion reviewer **I want** obligation coverage regressions and score/threshold
outcome changes classified **so that** the obligation-removal and explicitly-accepted gates
apply.

**Goal**: Classify `subject-or-obligation-uncovered` when baseline covered a required
obligation/subject pair the candidate no longer covers (decision worsens or obligation proof
absent). Classify `score-threshold-change` when decision changes solely due to points/threshold
arithmetic with identical intervention identities (or documented score-only delta). Tests use
multi-rule packs with explicit `points`/`risk.threshold`.

**Dependencies**: PCS-S01, PCS-S02 (shared classifier module).

**Definition of done**: both kinds covered by goldens; no schema changes.

Requirements:
- **REQ-PCS-S03-01** — Given baseline that proves obligation O on subject S and candidate
  that omits O from coverage while decision degrades, when classified, then kind is
  `subject-or-obligation-uncovered`. Test: `internal/compare/classify_test.go`; Verify:
  `go test ./internal/compare/... -run TestClassifyObligationUncovered`; Level: L0
- **REQ-PCS-S03-02** — Given baseline/candidate policies identical except threshold/points
  causing REVIEW→APPROVE flip with no new/removed finding identities, when classified, then
  kind is `score-threshold-change`. Test: same; Verify:
  `go test ./internal/compare/... -run TestClassifyScoreThresholdChange`; Level: L0
- **REQ-PCS-S03-03** — Classifier returns `ErrUnclassifiable` for decision changes that match
  none of the six kinds (fail-closed). Test: same; Verify:
  `go test ./internal/compare/... -run TestClassifyFailClosed`; Level: L0

---

## PCS-S04 — ComparisonRecord emission [autonomous]

**As a** CI operator **I want** schema-valid ComparisonRecords per case **so that** promotion
reviews have an auditable delta list.

**Goal**: Build `compare.ComparisonRecord` (Go struct or map) with `baselineProfile`,
`candidateProfile`, `caseId`, and `deltas[]` entries carrying kind + rule + subject +
optional obligation + baseline/candidate outcome sides (`present`, `decision`, `effect`,
`points`). Validate emitted JSON against `schemas.ComparisonRecordSchema`. Empty deltas when
fully agree.

**Dependencies**: PCS-S02, PCS-S03.

**Definition of done**: golden ComparisonRecord validates; explanation-only deltas appear but
do not imply gate failure in the record structure.

Requirements:
- **REQ-PCS-S04-01** — Given a classified widening case, when record is built, then JSON
  validates against `ComparisonRecordSchema` and includes per-delta identity fields. Test:
  `internal/compare/record_test.go`; Verify:
  `go test ./internal/compare/... -run TestComparisonRecordValidates`; Level: L0
- **REQ-PCS-S04-02** — Given an explanation-only delta, when record is built, then delta kind
  is `explanation-only` and baseline/candidate outcome sides share decision/effect identity.
  Test: same; Verify:
  `go test ./internal/compare/... -run TestComparisonRecordExplanationOnly`; Level: L0
- **REQ-PCS-S04-03** — `x-uniqueKeys` on deltas (kind,rule,subject) is respected — duplicate
  identities fail closed at build time. Test: same; Verify:
  `go test ./internal/compare/... -run TestComparisonRecordDuplicateDeltaRejected`; Level: L0

---

## PCS-S05 — Five-gate evaluator + acceptedDeltas allowlist [autonomous · engine-grade]

**As a** release manager **I want** all promotion gates evaluated against suite data **so that**
only explicitly accepted deltas can pass a failing gate.

**Goal**: Implement gate table evaluation from a loaded `PolicyComparisonSuite` spec:
for each gate, check case deltas whose kinds ∈ `failOnKinds`; honor `acceptance:
per-delta-identity` by consulting `acceptedDeltas` keyed by `caseId`+kind+rule+subject
(+obligation when present). `explanation-only` never fails any gate. Return per-gate PASS/FAIL
+ aggregate first-failure. Unit-test allowlist footgun (kind-only accept must NOT work).

**Dependencies**: PCS-S04.

**Definition of done**: all five gate IDs exercised; allowlist positive/negative tests; seed gate
row still schema-conformant.

Requirements:
- **REQ-PCS-S05-01** — Given a `newly-auto-mergeable` delta not in `acceptedDeltas`, when
  gates run, then `bounded-auto-merge-widening` is FAIL and others per their kinds. Test:
  `internal/compare/gates_test.go`; Verify:
  `go test ./internal/compare/... -run TestGateBoundedAutoMergeWidening`; Level: L0
- **REQ-PCS-S05-02** — Given a matching `acceptedDeltas` entry for the exact caseId+identity+kind,
  when gates run, then that delta does not fail its gate. Test: same; Verify:
  `go test ./internal/compare/... -run TestAcceptedDeltaAllowsSpecificIdentity`; Level: L0
- **REQ-PCS-S05-03** — Given a delta kind-only allowlist entry without rule/subject (invalid
  suite doc), when suite loads, then schema validation rejects it (frozen suite schema). Test:
  `schemas/comparison_test.go` or gates test; Verify:
  `go test ./schemas/... -run TestComparisonSuiteAcceptedDeltaRequiresIdentity`; Level: L0
- **REQ-PCS-S05-04** — `explanation-only` deltas never flip any gate to FAIL. Test:
  `internal/compare/gates_test.go`; Verify:
  `go test ./internal/compare/... -run TestExplanationOnlyNeverFailsGate`; Level: L0

---

## PCS-S06 — PolicyComparisonSuite loader + multi-case runner [autonomous]

**As a** maintainer **I want** to run an entire suite corpus in one invocation **so that**
promotion is gated by all pinned cases.

**Goal**: Load strict-decoded `PolicyComparisonSuite` YAML/JSON; for each case resolve
`replayBundleRef` (or embedded path), verify `replayBundleDigest`, load bundle, resolve
baseline/candidate profiles from suite defaults + CLI overrides (PCS-S01), run `Compare` per
case, collect ComparisonRecords + gate results. Fail closed on digest mismatch, unknown caseId
in allowlist, or missing bundle file.

**Dependencies**: PCS-S01, PCS-S05.

**Definition of done**: multi-case run returns aggregate report; determinism double-run stable.

Requirements:
- **REQ-PCS-S06-01** — Given a suite with ≥2 cases, when `RunSuite` executes, then each case
  produces one ComparisonRecord and gate results. Test: `internal/compare/suite_test.go`;
  Verify: `go test ./internal/compare/... -run TestRunSuiteMultiCase`; Level: L0
- **REQ-PCS-S06-02** — Given a bundle whose digest does not match the case's
  `replayBundleDigest`, when run, then error is fail-closed before evaluation. Test: same;
  Verify: `go test ./internal/compare/... -run TestRunSuiteDigestMismatch`; Level: L0
- **REQ-PCS-S06-03** — Double-run of `RunSuite` is byte-identical (determinism). Test: same;
  Verify: `go test ./internal/compare/... -run TestRunSuiteDeterministic`; Level: L0

---

## PCS-S07 — CLI suite mode + ADR-0018 exit codes [autonomous · engine-grade]

**As a** CI adopter **I want** `assent compare --suite …` with gate-scoped exit codes **so that**
pipelines can fail on the specific promotion gate violated.

**Goal**: Extend `cmd/assent/compare.go`: `--suite`, `--baseline-profile`, `--candidate-profile`,
optional `--record` (write ComparisonRecord JSON per case). Map process exit per D-115 (0 pass,
1–5 gate order, 6 fail-closed/load/digest). Preserve positional single-dir seed layout for
one-case fixtures (updated exit codes). Document usage in `docs/planning/policy-lifecycle-promotion-gates.md`
or compare section of usage docs.

**Dependencies**: PCS-S06.

**Definition of done**: CLI integration tests cover suite mode + exit codes; E6-S09 tests updated
for ADR exit mapping.

Requirements:
- **REQ-PCS-S07-01** — `assent compare --suite examples/comparison/...` exits 0 when all gates
  pass. Test: `cmd/assent/compare_test.go`; Verify:
  `go test ./cmd/assent/... -run TestCompareSuiteAllPass`; Level: L1
- **REQ-PCS-S07-02** — When `bounded-auto-merge-widening` fails, exit code is **4** (ADR-0018).
  Test: same; Verify: `go test ./cmd/assent/... -run TestCompareSuiteExitBoundedWidening`; Level: L1
- **REQ-PCS-S07-03** — Fail-closed classification exits **6**, not 0 or a gate code. Test: same;
  Verify: `go test ./cmd/assent/... -run TestCompareSuiteExitUnclassified`; Level: L1
- **REQ-PCS-S07-04** — Single-dir seed layout still works (`assent compare <dir>`) for E6-S09
  fixtures after exit-code migration. Test: same; Verify:
  `go test ./cmd/assent/... -run TestCompareGateExitCodes`; Level: L1

---

## PCS-S08 — Adversarial corpus + CI dogfood [autonomous]

**As a** project maintainer **I want** a committed comparison corpus **so that** gate regressions
fail CI before adopters see them.

**Goal**: Add `examples/comparison/promotion-gates/` (or similar) with a schema-valid
`PolicyComparisonSuite` covering ≥1 case per gate kind (including explanation-only and an
accepted-delta pass). Wire `go test` or `task check` step to run the suite hermetically. Include
named-consumer-compat signals as structured cases where applicable (no prose inference).

**Dependencies**: PCS-S07.

**Definition of done**: corpus runs green on main; one deliberately failing fixture proves gate
failure path in test.

Requirements:
- **REQ-PCS-S08-01** — `examples/comparison/**/suite.yaml` validates against
  `ComparisonSuiteSchema`. Test: `examples/comparison/validate_test.go`; Verify:
  `go test ./examples/comparison/...`; Level: L0
- **REQ-PCS-S08-02** — Corpus contains at least five cases mapping to the five gate IDs (may
  share bundles but distinct caseIds). Test: same; Verify:
  `go test ./examples/comparison/... -run TestCorpusCoversAllGates`; Level: L0
- **REQ-PCS-S08-03** — CI (verify or schemas job) runs the suite compare command and fails when
  a broken gate fixture is present (negative test). Test: workflow or harness; Verify:
  `grep -q comparison .github/workflows/verify.yaml || grep -q examples/comparison Taskfile.yml`;
  Level: L1

---

## PCS-S09 — Exit gate: full runner + D-057 closed [autonomous · engine-grade]

**As a** maintainer **I want** the PCS epic proven closed **so that** D-057 deferred scope is
shipped and E11/E12 can proceed without compare debt.

**Goal**: Exit checklist: all PCS stories green under `task check`; E6-S09 seed tests + new corpus
green; `git diff schemas/` == 0; backlog/later-phases mark PCS **AUTONOMOUS COMPLETE**; log
**D-118** (PCS exit gate). Document residual: compare does not replace human review of
`acceptedDeltas` rationale text.

**Dependencies**: PCS-S01..S08.

**Definition of done**: `hack/compare/exitgate_test.sh` or equivalent passes; D-057 scope listed
as closed in decisions cross-reference.

Requirements:
- **REQ-PCS-S09-01** — Exit script runs full suite + seed dir compare + schema drift guard. Test:
  `hack/compare/exitgate_test.sh`; Verify: `task check`; Level: L1
- **REQ-PCS-S09-02** — `git diff schemas/` is empty at exit gate commit. Test: exit script;
  Verify: `test -z "$(git diff schemas/)"`; Level: L0
- **REQ-PCS-S09-03** — `openspec/specs/backlog.md` + `later-phases.md` mark PCS IMPLEMENTING →
  CLOSED. Test: backlog; Verify:
  `grep -q p5-pcs-policy-comparison/spec.md openspec/specs/backlog.md`; Level: doc

---

## Epic definition of done

| Gate | Criterion |
| --- | --- |
| **Taxonomy** | All six delta kinds classify; unclassified fail-closed |
| **Gates** | All five promotion gates + per-delta `acceptedDeltas` enforced |
| **Records** | Schema-valid `ComparisonRecord` per case |
| **Corpus** | Multi-case suite committed + CI dogfood |
| **CLI** | `assent compare --suite` side-effect-free; ADR-0018 exit codes |
| **Schema freeze** | `git diff schemas/` == 0 |
| **Seed** | E6-S09 regression preserved (behaviour extended, not deleted) |
| **Deferred elsewhere** | E10/E11/E12/E13 unchanged |

**Story count:** 9 — **9 autonomous** (no infra-gated stories).

**Do first:** **PCS-S01** — profile→pack activation is the prerequisite for meaningful
profile promotion comparison; classifiers and gates without it only replay the seed's
explicit MergePolicy files.
