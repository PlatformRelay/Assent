# P5-E4 — GitLab forge adapter: Snapshot / Resolve / Reconcile

**Epic ID / REQ prefix:** `E4` / `REQ-E4-S0n-nn`.

**Problem**: P4-E1 shipped a **walking-skeleton** forge slice — `internal/forge.Reconcile` against an
in-memory fake, a partial `internal/forge/gitlab.Client` (Forge writes + `GetMR`/`FileAtRef`), marker
render/parse, and `assent run` orchestration that always evaluates with **nil forge
`ApprovalEvidence`** and gates arming on the **D-034 INSECURE-PLACEHOLDER** env reader. ADR-0017 §7
names the full port as **`Snapshot → Resolve → Reconcile(DesiredReviewState, Preconditions) →
PublicationReceipt`**, but only the write half exists as product code; read-side Snapshot (MR state,
changed files, tier capabilities) and Resolve (`require-review` eligibility → typed
`ApprovalEvidence`) are unspecified stubs. P3-E5 (ADR-0019) freezes nine reconciliation steps; P4
implemented idempotent thread upsert, duplicate repair, and SHA-pinned merge on the fake — but **not**
occurrence supersession, resolve no-longer-desired findings, post-publication rescan, or the summary
slot. The exit gate in `later-phases.md` demands L2 cassette contract tests plus conformance cases
(target advanced → rejected, `.assent/**` self-vouch → BLOCK, P3-E5 reconciliation replay).

**Key ground truth (de-risks the epic):**
- **Reuse, don't re-invent:** `internal/forge/forge.go` (Reconcile, markers, fail-closed errors),
  `internal/forge/fake`, `internal/forge/gitlab` (httptest L2 tests), P3-E5 fixtures under
  `docs/contracts/p3-e5-publication-protocol/fixtures/`, and `docs/planning/forge-dossier-gitlab.md`
  (capability matrix + approval-evidence chain) are the starting point — E4 **extends** them to the
  ADR-0017 port, it does not replace P4.
- **Frozen schemas stay frozen:** `ApprovalEvidence` validates against
  `schemas/decision/v1alpha1/approval-evidence.schema.json`; epic DoD prefers **`git diff schemas/` ==
  0**. Serialization of resolved evidence into the decision path uses existing
  `aggregate.ApprovalEvidence` — no EvaluationInput widen.
- **E6 fence holds:** `assent test` continues to stub `approval.yaml`; live forge Resolve must not
  leak into the adopter harness (ADR-0014).
- **`internal/core` stays I/O-free** (`TestCorePurity`); Snapshot/Resolve live at
  `internal/forge/**` and the `cmd/assent` edge only.
- **Synthetic merge-result digest** (P4 `gitlab.SyntheticDigest`) remains the belt-and-suspenders CAS
  pin for plain-merge tiers; honest `capabilityGap` on the DecisionRecord is separate (already wired in
  `run.go`).

**Scope**: (S01) formal Snapshot + Resolve port types + hermetic fakes; (S02) GitLab Snapshot L2
cassettes (MR metadata, changed-file enumeration, merge-setting capabilities); (S03) GitLab Resolve
ApprovalEvidence L2 cassettes (P1-E3-S02 eligibility chain, tier gaps fail-closed); (S04) Reconcile
P3-E5 gaps on the shared engine (occurrence supersession, resolve no-longer-desired, post-publication
rescan); (S05) `assent doctor` forge-probed capability/precondition report (closes D-034 for the
forge-backed arming path); (S06) wire Snapshot/Resolve into `assent run` (forge evidence + changed-file
set for classifier); (S07–S09) conformance goldens (SHA-guard, assent-policy BLOCK, P3-E5 replay); (S10)
autonomous exit gate. **Infra-gated:** S11 live GitLab L3 re-run (operator/CI with token).

**Non-goals** (fenced): **GitHub adapter** (E10, D-012 locked); **`serve` / keyed per-MR lock**
(E12); **summary-comment slot** (reconciliation step 3 — deferred judgment call (a), presentation seam
→ E8); **merge trains / merged-results enforcement** beyond honest capability reporting (Premium/Ultimate
features documented, not implemented in v1 autonomous slice); **renderer / PresentationModel** changes
(E8); **widening frozen contracts**; **replacing the in-memory fake** (it remains the Reconcile unit-test
substrate).

**ADRs**: 0005 (forge dossier, conformance suite), 0011 (forge ports), 0012 (finding lifecycle),
0015 §2/§4/§8 (SHA-guard, protected pipeline, execution-authority matrix), 0017 §1/§3/§7/§9
(merge-result pins, require-review evidence, Snapshot→Resolve→Reconcile, doctor typed report), 0019
(marker/reconciliation protocol). **Reuse**: P4 forge/gitlab/fake, P3-E5 fixtures, P1-E3 GitLab dossier,
E2 `aggregate.ApprovalEvidence` / `CoverWithApproval`, E5 provider host (facts path stays separate).
**New**: Snapshot/Resolve ports, full reconciliation engine behaviours, doctor forge probe, run-path
wiring, L2 cassette conformance suite.

**Executability**: S01–S10 **`[autonomous]`** with httptest servers + in-memory fakes/fixtures. S11
**`[infra-gated]`** (live gitlab.com / testcontainer). TDD; determinism double-run on forge outputs
where applicable; `TestCorePurity` untouched for `internal/core`.

**Judgment calls (decide-and-log / operator):**
(a) **🟡 DECIDED (default) — defer `summary-comment` slot (P3-E5 step 3) to E8.** P4's one-finding
thread slice never posted a summary artifact; E4 closes steps 6/7/9 and the marker-keyed finding thread
path first. E8 owns renderer envelope + summary upsert when PresentationModel publication lands.
Revisit only if a named consumer needs summary-before-E8.
(b) **DECIDED — D-034 replacement is forge-probe-primary when a forge token is present.** When
`assent run`/`doctor` holds a GitLab PAT, capability/precondition signals come from Snapshot (project
merge settings, tier-detected approval-rule APIs) — not the spoofable env vars. The env reader remains
only for **provider-less / forge-less `assent doctor`** diagnostics and must never alone arm a real
merge when forge probe data is available.
(c) **DECIDED — Free-tier GitLab: `capabilityGap` blocks auto-merge and `require-review` evidence is
unsatisfiable.** Per forge dossier §1 C6/C7: without Premium approval rules the adapter cannot prove
eligible approval → never APPROVE via missing evidence; doctor reports the gap explicitly.
(d) **🟡 decide-and-log in-lane — changed-file enumeration source.** Prefer GitLab MR diffs API
(`GET .../merge_requests/:iid/changes` → `changes[].new_path`/`old_path`) for Snapshot; fall back to
compare API only if diffs shape is insufficient. Implementers log the choice in `decisions.md` if the
API choice affects self-vouch detection.

**Dependency order**: **S01 → S02 → S03 → S04 → S05 → S06 → {S07 ∥ S08 ∥ S09} → S10**; **S11**
after S10 when infra available. **Closes D-034 (forge-backed path): S05+S06. Closes D-042 live self-vouch
gap: S08.**

**Engine-grade / fail-safety review:** S03 (approval evidence), S04 (reconciliation), S05 (arming),
S06 (run wiring), S07 (SHA-guard), S08 (assent-policy BLOCK), S10 (exit gate).

---

## E4-S01 — Snapshot + Resolve port types + hermetic fakes [autonomous]

**As a** forge implementer **I want** typed Snapshot and Resolve ports beside the existing Reconcile
port **so that** read-side forge state and approval evidence have one testable seam before GitLab HTTP
lands.

**Goal**: Add `Snapshot` (read MR heads, changed paths, bot threads, tier capability flags) and
`Resolve` (map a `require-review` subject + pinned SHAs → `aggregate.ApprovalEvidence` or explicit gap)
interfaces under `internal/forge/`, with hermetic fake implementations that mirror the GitLab shapes.
Keep `Forge` as the write port; document the ADR-0017 §7 three-phase flow in package docs. No HTTP yet.

**Dependencies**: P4 forge types, frozen `ApprovalEvidence` schema, E2 `aggregate.ApprovalEvidence`.

**Definition of done**: fake Snapshot returns deterministic MR/changed-file/capability fixtures; fake
Resolve returns schema-valid evidence or a typed `CapabilityGap`; ports compile; `git diff schemas/` == 0.

Requirements:
- **REQ-E4-S01-01** — `Snapshot` exposes MR heads, changed-file paths, and capability flags as typed
  structs (no `map[string]any` at the port boundary). Test: `internal/forge/snapshot_fake_test.go`;
  Verify: `go test ./internal/forge/... -run TestSnapshotFake`; Level: L0
- **REQ-E4-S01-02** — `Resolve` returns `aggregate.ApprovalEvidence` with explicit eligibility fields
  OR a typed gap (never silent APPROVE on missing evidence). Test: `internal/forge/resolve_fake_test.go`;
  Verify: `go test ./internal/forge/... -run TestResolveFake`; Level: L0
- **REQ-E4-S01-03** — fake implementations are deterministic (fixed clock, stable ordering). Test: same;
  Verify: `go test ./internal/forge/... -run 'TestSnapshotFake|TestResolveFake' -count=2`; Level: L0

---

## E4-S02 — GitLab Snapshot: MR metadata, changed files, capability flags [autonomous · L2 cassette]

**As a** policy runtime **I want** the GitLab adapter to snapshot MR state and merge capabilities via
recorded HTTP cassettes **so that** evaluation pins honest SHAs and tier gaps without live infra in CI.

**Goal**: Extend `internal/forge/gitlab` with Snapshot reads: `GetMR` (existing), changed-file list
(judgment call (d)), project merge settings needed for doctor (`only_allow_merge_if_all_discussions_are_resolved`,
approval-rules presence / tier detection). httptest cassettes cover pagination and 404 fail-safe paths.
PAT redaction contract from P4 preserved.

**Dependencies**: E4-S01.

**Definition of done**: L2 tests green with httptest; changed files returned for a fixture MR; Free vs
Premium capability flags distinguishable from cassette responses.

Requirements:
- **REQ-E4-S02-01** — Snapshot returns source/target SHAs and branch names matching `GetMR` semantics
  (target = branch tip, not merge-base). Test: `internal/forge/gitlab/snapshot_test.go`; Verify:
  `go test ./internal/forge/gitlab/... -run TestSnapshotMRHeads`; Level: L1
- **REQ-E4-S02-02** — changed-file enumeration returns every path touched by the MR diff fixture
  (add/modify/delete/rename as paths). Test: same; Verify:
  `go test ./internal/forge/gitlab/... -run TestSnapshotChangedFiles`; Level: L1
- **REQ-E4-S02-03** — capability flags expose tier gaps (e.g. no approval rules on Free) without
  inventing Premium features. Test: same; Verify:
  `go test ./internal/forge/gitlab/... -run TestSnapshotCapabilityFlags`; Level: L1
- **REQ-E4-S02-04** *(SECURITY)* — PAT never appears in URLs, bodies, or test failure messages. Test:
  existing `gitlab_test.go` redaction tests + snapshot tests; Verify:
  `go test ./internal/forge/gitlab/... -run TestSnapshot`; Level: L1

---

## E4-S03 — ⚠️ GitLab Resolve: ApprovalEvidence eligibility chain [autonomous · L2 cassette · engine-grade]

> **⚠️ Authorization surface (ADR-0017 §3).** Wrong eligibility → APPROVE without proof. Reviewer pointed
> at author/bot exclusion, sha pins, and tier-gap fail-closed behaviour.

**As a** policy author **I want** `require-review` satisfied only by forge-proven eligible approval
**so that** failed authorization never degrades into an author-resolvable thread.

**Goal**: Implement `Resolve` on `gitlab.Client` per P1-E3-S02 / forge dossier §4: fetch approval
rules/state, map eligible approvers, collect actual approvals with identities, exclude MR author and bot
client-side, pin `sourceSha`/`targetSha`/`mergeResultDigest` and observation time on the evidence.
Premium cassette proves satisfied path; Free/missing-rules cassette → `CapabilityGap` (judgment call (c)).
Emit evidence validating against frozen schema.

**Dependencies**: E4-S01, E4-S02.

**Definition of done**: httptest cassettes for satisfied + gap paths; self-approval excluded; sha mismatch
→ evidence rejected; `git diff schemas/` == 0.

Requirements:
- **REQ-E4-S03-01** *(ENGINE · fail-safe)* — eligible approval from cassette → schema-valid
  `ApprovalEvidence` with matching sha pins. Test: `internal/forge/gitlab/resolve_test.go`; Verify:
  `go test ./internal/forge/gitlab/... -run TestResolveEligibleApproval`; Level: L1
- **REQ-E4-S03-02** *(fail-safe)* — MR-author approval excluded even if forge accepted it. Test: same;
  Verify: `go test ./internal/forge/gitlab/... -run TestResolveExcludesAuthorApproval`; Level: L1
- **REQ-E4-S03-03** *(fail-safe)* — Free tier / missing approval-rules API → explicit capability gap
  (never fabricated evidence). Test: same; Verify:
  `go test ./internal/forge/gitlab/... -run TestResolveTierGapFailClosed`; Level: L1
- **REQ-E4-S03-04** — stale sha pins → evidence rejected (re-evaluate, not APPROVE). Test: same; Verify:
  `go test ./internal/forge/gitlab/... -run TestResolveShaMismatch`; Level: L1

---

## E4-S04 — ⚠️ Reconcile P3-E5 gaps: supersession, resolve stale, rescan [autonomous · engine-grade]

> **⚠️ Reconciliation completeness.** Partial protocol → duplicate/stale threads or false success.
> Reviewer pointed at steps 6, 7, 9 from ADR-0019.

**As a** maintainer **I want** the shared Reconcile engine to implement the remaining P3-E5 protocol
steps **so that** reruns converge on the forge surface without a database.

**Goal**: On the shared `internal/forge.Reconcile` path (fake + gitlab write port): (6) supersede stale
occurrences with a fresh challenge thread; (7) resolve no-longer-desired finding threads; (9) rescan
bot threads after writes before reporting success. Reuse existing idempotence + duplicate repair (P4-S12).
Author-identity filter unchanged. Summary slot explicitly out of scope (judgment call (a)).

**Dependencies**: E4-S01; P4 Reconcile baseline.

**Definition of done**: new fixtures or extensions to P3-E5 YAML replay green on fake; zero false-success
when rescan would fail; determinism double-run on receipts.

Requirements:
- **REQ-E4-S04-01** — stale occurrence → new thread posted; old occurrence left resolved-but-stale (not
  reused). Test: `internal/forge/reconcile_supersede_test.go`; Verify:
  `go test ./internal/forge/... -run TestReconcileSupersedesStaleOccurrence`; Level: L1
- **REQ-E4-S04-02** — slot absent from desired state → existing bot thread resolved (no orphan open
  findings). Test: `internal/forge/reconcile_resolve_stale_test.go`; Verify:
  `go test ./internal/forge/... -run TestReconcileResolvesNoLongerDesired`; Level: L1
- **REQ-E4-S04-03** — post-write rescan confirms forge state before success; mismatch → error, not a
  receipt claiming success. Test: `internal/forge/reconcile_rescan_test.go`; Verify:
  `go test ./internal/forge/... -run TestReconcileRescanBeforeSuccess`; Level: L1
- **REQ-E4-S04-04** — P3-E5 fixtures (`rerun-idempotence`, `duplicate-repair`, `crash-then-rerun`) still
  green after engine extension. Test: existing; Verify:
  `go test ./internal/forge/... -run TestReconcileReplaysP3E5Fixtures`; Level: L1

---

## E4-S05 — ⚠️ Doctor: forge-probed capability + precondition report [autonomous · engine-grade]

> **⚠️ Closes D-034 for forge-backed runs.** Arming must not trust spoofable env when forge probe exists.

**As an** operator **I want** `assent doctor` to report forge-derived capabilities and refuse arming when
the GitLab project lacks required merge gates **so that** author-editable CI cannot self-arm.

**Goal**: When a forge Snapshot is available, `Doctor` consumes probe results (protected-pipeline signal
from project CI config path where detectable, discussions-resolved gate, approval-rules tier, duplicate
prevention guarantee = serialized topology per P3-E5) instead of env self-assertion (judgment call (b)).
Provider-less `assent doctor` keeps today's env diagnostic path but prints an explicit INSECURE banner.
Typed `PreconditionReport` gains additive capability fields (`AutoMergeEligible`, `DuplicatePrevention`).

**Dependencies**: E4-S02.

**Definition of done**: httptest doctor probe green; unprotected/Free-gap fixtures → `ArmEligible=false`
with typed reasons; env-only doctor still works without token.

Requirements:
- **REQ-E4-S05-01** *(fail-safe)* — forge probe reports missing discussions-resolved gate →
  `ArmEligible=false`. Test: `cmd/assent/doctor_forge_test.go`; Verify:
  `go test ./cmd/assent/... -run TestDoctorForgeMissingDiscussionGate`; Level: L1
- **REQ-E4-S05-02** — forge probe detects tier gap → typed capability gap in report (never arms). Test:
  same; Verify: `go test ./cmd/assent/... -run TestDoctorForgeTierGap`; Level: L1
- **REQ-E4-S05-03** — env-only doctor without forge prints INSECURE banner and does not claim verified
  protected-source. Test: same; Verify:
  `go test ./cmd/assent/... -run TestDoctorEnvOnlyInsecureBanner`; Level: L1
- **REQ-E4-S05-04** — `duplicate_prevention` guarantee field populated per P3-E5 (serialized / explicit
  unsupported). Test: same; Verify:
  `go test ./cmd/assent/... -run TestDoctorDuplicatePreventionReport`; Level: L1

---

## E4-S06 — ⚠️ Wire Snapshot/Resolve into `assent run` [autonomous · engine-grade]

**As a** repo operator **I want** live MR evaluation to use forge Snapshot data and Resolve approval
evidence **so that** require-review and assent-policy routing reflect real forge state.

**Goal**: At the `cmd/assent` edge: Snapshot → enumerate changed files for classifier (closes D-042 F1
live path when combined with S08 golden); Resolve → populate `ApprovalContext` before `CoverWithApproval`;
honest `capabilityGap` on DecisionRecord when tier lacks merge-result digest or approval rules; preserve
provider-less / fake-forge test paths. E6 / `assent test` untouched.

**Dependencies**: E4-S02, E4-S03, E4-S05; E5-S05 (facts path may run in parallel when file-disjoint).

**Definition of done**: httptest run path evaluates with forge-resolved evidence affecting decision;
changed-file list drives classifier; `assent test` fence test still green.

Requirements:
- **REQ-E4-S06-01** — forge Resolve supplies `ApprovalEvidence` that can satisfy a require-review fixture
  decision when eligible. Test: `cmd/assent/run_forge_test.go`; Verify:
  `go test ./cmd/assent/... -run TestRunResolveApprovalEvidence`; Level: L1
- **REQ-E4-S06-02** — Snapshot changed files feed classifier (not single hardcoded subject only). Test:
  same; Verify: `go test ./cmd/assent/... -run TestRunSnapshotChangedFiles`; Level: L1
- **REQ-E4-S06-03** *(compat · E6 fence)* — `assent test` never calls live forge Resolve. Test:
  `cmd/assent/test_forge_fence_test.go`; Verify:
  `go test ./cmd/assent/... -run TestAssentTestNeverCallsForgeResolve`; Level: L1
- **REQ-E4-S06-04** — tier gap → DecisionRecord carries honest `capabilityGap`; never APPROVE on missing
  approval evidence. Test: `cmd/assent/run_forge_test.go`; Verify:
  `go test ./cmd/assent/... -run TestRunTierGapNeverApproves`; Level: L1

---

## E4-S07 — Conformance: target advanced after evaluation → SHA-guard rejection [autonomous]

**As a** maintainer **I want** a conformance golden proving target/source movement after evaluation fails
closed **so that** the ADR-0015 §2 SHA-guard is executable contract, not prose.

**Goal**: L2 cassette (or fake with drift hook) drives full orchestrate → Reconcile path; moved target or
source after Snapshot pins → `ErrSHAMoved`, zero merge, typed summary. Extends P4-S07-02 / gitlab
`TestMergeCASTargetMovedNoMerge` through the product path.

**Dependencies**: E4-S04, E4-S06.

**Definition of done**: conformance test green; double-run deterministic.

Requirements:
- **REQ-E4-S07-01** — target tip moved after evaluation → Reconcile/merge refused with `ErrSHAMoved`. Test:
  `internal/forge/conformance/sha_guard_test.go`; Verify:
  `go test ./internal/forge/conformance/... -run TestConformanceTargetAdvancedRejected`; Level: L1
- **REQ-E4-S07-02** — source head moved → merge CAS refused (`409`/`406` mapping preserved). Test: same;
  Verify: `go test ./internal/forge/conformance/... -run TestConformanceSourceMovedRejected`; Level: L1

---

## E4-S08 — Conformance: `.assent/**` MR → assent-policy BLOCK [autonomous · engine-grade]

**As a** maintainer **I want** the live run path to BLOCK when the MR modifies `.assent/**` **so that**
the trust boundary proven in P4-S07 engine goldens holds on the adapter path (D-042 follow-up).

**Goal**: Snapshot changed-files includes `.assent/policy.yaml` (or pack path) → classifier routes
`assent-policy` meta-class → BLOCK before any forge write; conformance golden via httptest run path.

**Dependencies**: E4-S06.

**Definition of done**: self-modifying policy MR → BLOCK decision, zero Reconcile writes.

Requirements:
- **REQ-E4-S08-01** *(fail-safe · closes D-042 F1)* — changed `.assent/**` on MR → BLOCK, no thread/merge
  writes. Test: `cmd/assent/run_self_vouch_test.go`; Verify:
  `go test ./cmd/assent/... -run TestRunAssentPolicySelfModificationBlocks`; Level: L1
- **REQ-E4-S08-02** — policy still loaded from target ref only (unchanged trust rule). Test: same; Verify:
  `go test ./cmd/assent/... -run TestRunPolicyFromTargetRefOnly`; Level: L1

---

## E4-S09 — Conformance: P3-E5 reconciliation + spoofed-marker replay [autonomous]

**As a** maintainer **I want** the executable conformance suite to replay every P3-E5 adversarial fixture
**so that** ADR-0019 is the forge port definition (ADR-0005).

**Goal**: Package `internal/forge/conformance` (or extend existing tests) replays: rerun idempotence,
crash-then-rerun, duplicate repair, contributor marker spoof ignored — against fake AND gitlab httptest
write/list path where applicable. Spoof case proves author-identity filter on ListBotThreads.

**Dependencies**: E4-S04.

**Definition of done**: all four fixture behaviours have named conformance tests; spoof case fails closed
if contributor marker would have been trusted.

Requirements:
- **REQ-E4-S09-01** — rerun + crash-then-rerun → zero new bot threads (replay P3-E5 YAML). Test:
  `internal/forge/conformance/reconciliation_test.go`; Verify:
  `go test ./internal/forge/conformance/... -run TestConformanceRerunIdempotence`; Level: L1
- **REQ-E4-S09-02** — duplicate repair → lowest forge id canonical + `PublicationReceipt.repairs`. Test:
  same; Verify: `go test ./internal/forge/conformance/... -run TestConformanceDuplicateRepair`; Level: L1
- **REQ-E4-S09-03** — contributor spoof marker excluded (zero reconciliation effect). Test: same; Verify:
  `go test ./internal/forge/conformance/... -run TestConformanceSpoofedMarkerIgnored`; Level: L1

---

## E4-S10 — Exit gate: L2 cassette CI + hermetic forge path green [autonomous · engine-grade]

**As a** maintainer **I want** the autonomous half of E4 proven in CI **so that** implementers can land
the epic without live GitLab.

**Goal**: (1) `task check` runs all E4 L0/L1 cassette + conformance tests; (2) hermetic `assent run`
httptest path exercises Snapshot→Resolve→Reconcile once with resolved approval on a require-review fixture;
(3) doctor forge probe path covered; (4) `git diff schemas/` == 0; (5) backlog marks E4 autonomous slice
closed (S11 infra optional).

**Dependencies**: E4-S01..S09.

**Definition of done**: exit-gate tests green; D-034 forge-backed closure recorded; D-042 F1 closed on
adapter path.

Requirements:
- **REQ-E4-S10-01** — all `internal/forge/**` + `cmd/assent/*forge*` tests enforced in verify. Test: CI;
  Verify: `task check`; Level: L1
- **REQ-E4-S10-02** — hermetic run path end-to-end with forge Resolve affecting decision. Test:
  `cmd/assent/run_forge_test.go`; Verify:
  `go test ./cmd/assent/... -run TestE4ExitGateHermeticForgePath`; Level: L1
- **REQ-E4-S10-03** — conformance package runs under `./...` with zero skipped autonomous cases. Test:
  `internal/forge/conformance/...`; Verify:
  `go test ./internal/forge/conformance/...`; Level: L1

---

## E4-S11 — Live GitLab L3 conformance re-run [infra-gated: needs live GitLab / token]

**As a** maintainer **I want** the conformance suite green against a live GitLab project **so that** L3
adapter semantics are proven beyond httptest cassettes.

**Goal**: Re-run S07–S09 cases (or a thin `test/e2e/forge_conformance_test.go` with `-tags e2e`) against
the D-037 lab project when `GITLAB_TOKEN` + endpoint are configured; skip otherwise. Mirrors P4-E1-S10
pattern.

**Dependencies**: E4-S10; operator token (D-037).

**Definition of done**: one operator-attested green run recorded in `docs/decisions/evidence/` OR skip
documented in board when infra absent (does not block autonomous exit).

Requirements:
- **REQ-E4-S11-01** — live run: SHA-guard rejection observable on real MR push (operator evidence). Test:
  `test/e2e/forge_conformance_test.go` (e2e tag); Verify: `go test -tags e2e ./test/e2e/... -run TestLiveSHAGuard`
  (skip without env); Level: L3
- **REQ-E4-S11-02** — live rerun idempotence: second assent run creates zero new threads. Test: same;
  Verify: operator evidence pointer in `decisions.md`; Level: doc (infra)
