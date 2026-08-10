# P5-E11 — Complex-rule backend: Rego predicate tier

**Epic ID / REQ prefix:** `E11` / `REQ-E11-S0n-nn`.

**Unlock:** D-141 (2026-08-10). E11's *contract* was already unlocked by D-017; what was gated
was **implementation**, twice over: "after Phase 4" (satisfied — the Phase-4 adoption gate
closed with D-042) and, per D-017, **evidence-based per rule** — "each ported rule tries CEL
first, the backend is built when a concrete rule demonstrably exceeds the tier-1 ceiling."
D-141 records the operator lifting that per-rule evidence gate and what it does **not** waive.

**Problem**: ADR-0002 v2 promises "one Kyverno-style YAML envelope, **pluggable expression
backends**" — and the ADR index has carried the status line "**pluggable half unbuilt: Rego is
E11**" ever since. There is exactly one backend (CEL, ADR-0013), and the CEL leaf is
restricted to the frozen predicate-scope table: single-pass, single-subject, no set
operations across manifests, no graph relationships. The escape hatch was designed and
committed — `examples/policies/rego/bounded_change.rego` exists and is *quarantined* behind a
`# locked: D-012` marker with a CI guard (P3-E3-S03/S04) forbidding any declarative example
from referencing a `rego:` leaf. E11 is the epic that removes that quarantine and makes the
second tier real.

**Two hard constraints that shape every story:**

1. **The authoring schema names the backend; the decision schema does not.** P3-E1-S02's
   backend-neutrality guarantee (REQ-P3-E1-S02-01: "no field naming a predicate backend …
   anywhere in the schema") applies to `EvaluationInput` — and it holds, so **no decision
   contract changes**. But `schemas/policy/v1alpha1/merge-policy.schema.json` defines the
   predicate leaf as `{"additionalProperties": false, "required": ["cel"], "properties":
   {"cel": …, "message": …}}`. A `rego:` leaf therefore **requires a policy-schema change**.
   `API_STABILITY.md:19` permits exactly this within `v1alpha1`: "Additive field additions
   within a major require an openspec change + version bump before they become required" and
   "within a major, changes are additive-only for reports and **announced-only for authored
   policy**." This spec *is* that openspec change. **E11 is therefore the first epic whose DoD
   is `git diff schemas/` != 0** — every prior epic required it to be zero, and a reviewer
   applying the old habit will flag the correct change as a violation.

2. **Rule 7 (determinism) is the sharpest constraint, and the obvious implementation breaks
   it.** AGENTS.md rule 7 forbids anything probabilistic or wall-clock-dependent in the
   decision path. Rego's standard builtins include `time.now_ns()`, `rand.intn()`, and
   `http.send()` — each of which would make the same policy over the same ChangeSet produce
   different decisions on different runs. Worse, the *reflexive* safety measure — "bound Rego
   evaluation with a timeout" — is itself a rule-7 violation: a wall-clock deadline makes the
   decision depend on machine speed and load, so a policy that passes on a fast runner blocks
   on a slow one. E11 must bound evaluation **deterministically** (S06) and deny the
   non-deterministic builtins **structurally** (S04), not by convention.

**Key ground truth (de-risks the epic):**
- **The contract shape is already decided and reviewable.** ADR-0002 v2: the envelope owns
  `match` / `effect` / `points`; a Rego module **only computes violations** over the policy
  input. `examples/policies/rego/bounded_change.rego` is the committed, reviewed illustration
  of that shape. E11 implements the decided contract; it does not redesign it.
- **`later-phases.md` fixes the boundaries**: violations-shaped modules, **explicit
  obligation-proof polarity (no implicit "no violation = proof")**, OPA capability sandbox
  (D-013), the same typed `EvaluationInput`, structured proof/finding output, declared data,
  **no I/O**, and **no control over aggregation**. Hard boundary: no domain-aware joins, no Go
  rule plugins in `internal/core`.
- **`docs/planning/rego-escape-hatch.md` names E11 as the only lane permitted to remove the
  quarantine marker** — and the guard that enforces it lives in P3-E3-S04
  (`hack/check-migration-invariants.sh`). Unquarantining is a story here (S11), not a
  side-effect of any other story.
- **`internal/core` stays I/O-free** (`TestCorePurity`). Module *loading* is I/O and lives at
  the loader tier alongside CEL compilation; module *evaluation* is pure computation and may
  live in core only once S04's sandbox makes that true by construction.

**Scope**: (S01) the tier-1 ceiling, recorded with concrete exceeding rules; (S02) additive
`rego:` leaf in the policy schema; (S03) module loading + compile-time errors; (S04) OPA
capability sandbox (D-013); (S05) input binding to the identical `EvaluationInput`; (S06)
deterministic evaluation budget; (S07) violations → findings with explicit obligation-proof
polarity; (S08) aggregation boundary — the module never controls effect/points; (S09)
`assent lint` hard errors for Rego rules; (S10) `assent test` + goldens; (S11) remove the
quarantine + update the P3-E3-S04 guard; (S12) docs/maturity truth; (S13) exit gate.

**Non-goals** (fenced): **GitHub adapter** (E10 — separate spec, unlocked by D-140);
**`serve`** (E12); **remote packs** (E13, still Locked per D-012); **domain-aware joins** and
**in-process Go rule plugins** (D-017 declined both, permanently — not deferred);
**giving Rego control over aggregation, effects, or points** (ADR-0002 v2 boundary);
**WASM or gRPC predicate backends** (still Locked per D-012 — D-141 unlocks Rego only);
**widening the frozen predicate-scope table for CEL**; **any `EvaluationInput` change**
(the whole point of P3-E1-S02's neutrality is that none is needed — if a story finds one
necessary, that is a design failure to surface, not a schema bump to make).

**ADRs**: 0002 v2 (**the governing ADR** — pluggable backends; its "pluggable half unbuilt"
status line is retired by S12), 0007 (effects and aggregation — the boundary Rego must not
cross), 0013 (CEL as tier 1; the ceiling E11 sits above), 0017 §2/§3/§9 (obligations, proof
polarity, do-not-generalize list), 0018 (phase/profile — a Rego rule is phase-gated like any
other). **Related decisions**: D-012 (escape-hatch quarantine), D-013 (OPA capability
sandbox), D-017 (contract unlock + declined items), D-141 (implementation unlock).
**Reuse**: the E2 decision engine's leaf-evaluation seam, `internal/core/policy`'s strict
loader, E3's lint hard-error framework, E6's `assent test` harness, the committed example.
**New**: `rego:` leaf, OPA integration + capability file, deterministic budget, violation
mapping.

**Executability**: S01–S13 all **`[autonomous]`** — hermetic, no infrastructure. S02, S04,
S06, S07 are **engine-grade** (frozen-schema change, sandbox, determinism, decision polarity)
and additionally **`[maintainer LGTM]`**: S02 changes a published contract and S04/S06/S07 are
the decision path itself.

**Dependency order**: S01 → S02 → S03 → S04 → S05 → S06 → S07 → S08 → {S09, S10} → S11 → S12
→ S13. **Do first: S01** — the ceiling document determines whether the backend's shape is
right; building it without one reproduces the speculative-generality risk D-012 existed to
prevent.

## Judgment calls (decide-and-log / operator)

(a) **DECIDED — the `rego:` leaf is an additive `oneOf` alternative, not a replacement.**
Every policy valid before E11 stays valid; a `rego:` leaf is rejected by an older assent
binary through strict-decode, which is the **correct** direction (an old binary must not
silently ignore a rule it cannot evaluate). This is backward-compatible and deliberately
forward-**in**compatible, and S02 records it in `API_STABILITY.md` as an announced additive
change — no `apiVersion` bump.

(b) **DECIDED — evaluation is bounded by a deterministic budget, never a wall-clock timeout.**
Per rule 7: the bound is an OPA evaluation-step/instruction budget that yields the identical
outcome on any machine. A wall-clock deadline is permitted **only** as an outer backstop that
can never change a decision — i.e. exceeding it is a hard process error, never a policy
outcome (not a BLOCK, not an APPROVE, not a skipped rule). If that separation cannot be
implemented cleanly, the story fails and the operator is asked; it is not resolved by
"timeout → BLOCK", which would make the decision machine-dependent while *looking* fail-closed.

(c) **DECIDED — zero violations is NOT proof of an obligation.** `later-phases.md` names this
explicitly and it is the single easiest thing to get wrong: a module that errors, is
misconfigured, matches nothing, or returns an empty set would otherwise silently *satisfy* a
required obligation. A Rego-backed obligation is proven only by an explicit, structured proof
value; absence of violations satisfies **non-obligation** rules only. S07 owns both polarities
and must test the failing one.

(d) **🟡 OPERATOR — the OPA dependency is a supply-chain decision, not just an import.**
`github.com/open-policy-agent/opa` is a large dependency with a large transitive tree, on a
project whose release story includes SLSA-grade provenance, cosign signing, `govulncheck`, and
Scorecard. Adding it materially changes binary size, vulnerability surface, and the
`renovate`/`govulncheck` maintenance load. Recommended default: **accept**, since a
hand-rolled Rego evaluator would be far worse, and pin + vendor-audit it in S03. Flagged for
an explicit operator ack because it is the kind of change D-012's philosophy ("no speculative
frozen contracts for tiers without users") exists to make deliberate. **Recorded as D-141's
open sub-question.**

(e) **DECIDED — Rego modules are policy, and load from the target ref like all policy.**
ADR-0010/ADR-0015's trust rules apply unchanged: a module is loaded from the target ref,
never from the PR head, so a contributor cannot ship a rule change and have it govern their
own PR. S03 must not introduce a second, laxer load path.

(f) **DECIDED — E11 and E10 are independent and may run in either order or in parallel.**
They share no files: E10 is `internal/forge/**` + `cmd/assent` edge; E11 is
`internal/core/**` + `schemas/policy/**` + `examples/policies/rego/**`. The only coupling is
review bandwidth. Recommended sequencing is **E10 first** — it has a live-adoption story
(S18) whose infrastructure the operator must arrange, so starting it early parallelizes the
human dependency.

---

### E11-S01 — Record the tier-1 ceiling with concrete exceeding rules `[autonomous]`

- **Goal**: a written, reviewable statement of what CEL *cannot* express, grounded in real
  rules — the artifact D-017's evidence gate was protecting.
- **Why first**: D-141 lifts the per-rule evidence *gate*, not the design need. The four
  shapes `later-phases.md` names (multi-pass, cross-manifest, set-difference,
  graph-relationship) determine what the input binding (S05) and the violation shape (S07)
  must support. Building those without the ceiling document is guesswork.
- **Dependencies**: none.
- **Definition of done**: `docs/planning/rego-tier-ceiling.md` exists with ≥1 concrete,
  sanitized rule per named shape, each showing the CEL attempt and *why* it fails.

- **REQ-E11-S01-01** — Given the four shapes, when the ceiling document is authored, then each
  carries a concrete generic rule, the attempted CEL leaf, and the specific reason it cannot
  be expressed within the frozen predicate-scope table (`docs/planning/predicate-scope.md`) —
  and no employer or internal system name appears (D-002).
  - Test: `docs/planning/rego-tier-ceiling.md`
  - Verify: `task scrub && task check`
  - Level: L0
- **REQ-E11-S01-02** — Given a shape might in fact be CEL-expressible, when the document is
  reviewed, then any shape found expressible in CEL is **struck from E11's scope** and
  recorded — the epic narrows rather than building an unjustified tier.
  - Test: `docs/planning/rego-tier-ceiling.md`, `openspec/specs/p5-e11-rego-backend/spec.md`
  - Verify: manual review
  - Level: L0

### E11-S02 — Additive `rego:` leaf in the policy schema `[autonomous · engine-grade · maintainer LGTM]`

- **Dependencies**: S01.
- **Definition of done**: `merge-policy.schema.json`'s `leaf` becomes a `oneOf` over the
  existing `cel` shape and a new `rego` shape; strict-decode still rejects unknown fields and
  a leaf carrying **both** backends; `API_STABILITY.md` records the announced additive change;
  every pre-E11 policy fixture still validates unchanged.

- **REQ-E11-S02-01** — Given the leaf is `additionalProperties:false, required:["cel"]`, when
  the schema is extended, then a `{"rego": {...}}` leaf validates, a `{"cel": ..., "rego": ...}`
  leaf is **rejected** (exactly one backend per leaf), and an unknown key is still rejected.
  - Test: `schemas/policy/v1alpha1/merge-policy.schema.json`, `schemas/schema_test.go`
  - Verify: `go test ./schemas/... -run TestMergePolicySchema`
  - Level: L0
- **REQ-E11-S02-02** — Given backward compatibility, when the full pre-E11 example and
  fixture corpus is validated against the new schema, then **every document still validates**
  and no golden changes.
  - Test: `examples/**`, `test/**`
  - Verify: `task test && git diff --exit-code -- examples/ test/`
  - Level: L1
- **REQ-E11-S02-03** — Given `API_STABILITY.md:19`'s "announced-only for authored policy",
  when the schema changes, then `API_STABILITY.md` and the changelog record it as an announced
  additive change within `v1alpha1` with **no `apiVersion` bump**, and state explicitly that an
  older binary rejects a `rego:` leaf by design.
  - Test: `API_STABILITY.md`, `CHANGELOG.md`
  - Verify: `task check`
  - Level: L0
- **REQ-E11-S02-04** — Given every prior epic's DoD was `git diff schemas/` == 0, when E11's
  gates run, then the schema-drift guard is **scoped**, not deleted: drift is permitted only
  in `merge-policy.schema.json` and only for this change; drift in any `schemas/decision/**`
  file still fails.
  - Test: `hack/` schema-drift guard
  - Verify: `task check`
  - Level: L1

### E11-S03 — Module loading and compile-time errors `[autonomous]`

- **Dependencies**: S02.
- **Definition of done**: modules resolve from the pack directory **on the target ref**
  (judgment call (e)); a module that fails to compile is a **load-time hard error** (E3 lint
  parity), never a runtime surprise; the OPA dependency is pinned.

- **REQ-E11-S03-01** — Given ADR-0010/0015 trust rules, when a Rego module is loaded, then it
  resolves through the **same target-ref policy load path** as YAML policy, and no second
  loader can read a module from the PR head — asserted by a test that places a hostile module
  on the head ref and proves it is not evaluated.
  - Test: `internal/core/policy/rego_load.go`, `rego_load_test.go`
  - Verify: `go test ./internal/core/... -run TestRegoLoadsFromTargetRef`
  - Level: L1
- **REQ-E11-S03-02** — Given E3's hard-error framework, when a module fails to compile or
  references an undefined rule, then `assent lint` reports it as a **hard error** with the
  file and position, and `assent run` refuses to evaluate — fail closed, never skip the rule.
  - Test: `internal/core/policy/rego_load_test.go`
  - Verify: `go test ./internal/core/... -run TestRegoCompileErrorIsHardError`
  - Level: L1
- **REQ-E11-S03-03** — Given judgment call (d), when OPA is added to `go.mod`, then the
  version is pinned, `govulncheck` and `renovate` cover it, and the binary-size delta is
  recorded in the story's notes.
  - Test: `go.mod`, `go.sum`
  - Verify: `task check && govulncheck ./...`
  - Level: L0

### E11-S04 — OPA capability sandbox `[autonomous · engine-grade · maintainer LGTM]`

- **Dependencies**: S03.
- **Definition of done**: D-013's sandbox is real — a capability set that **denies by
  default** and allows an explicit, enumerated builtin list; `http.send`, `net.*`,
  `opa.runtime`, `time.*`, `rand.*`, and any I/O builtin are unavailable; a module using one
  fails to **compile**, not at runtime.

- **REQ-E11-S04-01** — Given rule 7 and D-013, when a module calls `http.send`, `net.lookup_ip_addr`,
  `time.now_ns`, `rand.intn`, or `opa.runtime`, then compilation **fails** with a message
  naming the denied builtin — one test case per denied builtin, each asserting failure.
  - Test: `internal/core/policy/rego_capabilities.go`, `rego_capabilities_test.go`
  - Verify: `go test ./internal/core/... -run TestDeniedBuiltins`
  - Level: L1
- **REQ-E11-S04-02** — Given allowlists rot silently, when the allowed builtin set changes,
  then a test comparing the effective set against a **committed golden list** fails — so
  adding a builtin is a deliberate, reviewed act, and an OPA upgrade that introduces new
  builtins cannot widen the sandbox unnoticed.
  - Test: `internal/core/policy/testdata/allowed-builtins.golden`
  - Verify: `go test ./internal/core/... -run TestAllowedBuiltinsGolden`
  - Level: L1
- **REQ-E11-S04-03** — Given `TestCorePurity`, when Rego evaluation lives in `internal/core`,
  then the purity test still passes and the sandbox is what makes that true by construction —
  if evaluation cannot be made pure, it moves out of core rather than weakening the test.
  - Test: `internal/core/` purity test
  - Verify: `go test ./internal/core/... -run TestCorePurity`
  - Level: L1

### E11-S05 — Input binding: the identical `EvaluationInput` `[autonomous · engine-grade]`

- **Dependencies**: S04.
- **Definition of done**: a Rego module sees the same typed input a CEL leaf sees, proving
  P3-E1-S02's neutrality claim empirically; **no `EvaluationInput` schema change**.

- **REQ-E11-S05-01** — Given REQ-P3-E1-S02-01, when a rule is evaluated by either backend,
  then both receive input derived from the **same** `EvaluationInput` instance — asserted by a
  test that evaluates an equivalent rule under both backends and compares the bound input.
  - Test: `internal/core/policy/rego_input_test.go`
  - Verify: `go test ./internal/core/... -run TestBackendsShareInput`
  - Level: L1
- **REQ-E11-S05-02** — Given typed facts (Spike C / OQ-17), when a fact is `unavailable`,
  `invalid`, or `expired`, then the module observes that **typed state** and cannot mistake it
  for a value — an unavailable fact must not read as absent-and-therefore-fine (the
  "absent-means-trusted" pattern the 2026-08-09 audit found three times).
  - Test: `internal/core/policy/rego_input_test.go`
  - Verify: `go test ./internal/core/... -run TestUnavailableFactVisibleToModule`
  - Level: L1
- **REQ-E11-S05-03** — Given `git diff schemas/decision/` must stay empty, when S05 lands,
  then no decision-contract file has changed.
  - Test: `schemas/decision/**`
  - Verify: `git diff --exit-code -- schemas/decision/`
  - Level: L0

### E11-S06 — Deterministic evaluation budget `[autonomous · engine-grade · maintainer LGTM]`

- **Dependencies**: S05.
- **Definition of done**: evaluation is bounded by a machine-independent budget; exceeding it
  can **never** produce a policy outcome (judgment call (b)); rule-7 determinism is proven by
  repeated evaluation.

- **REQ-E11-S06-01** — Given rule 7, when the same module evaluates the same input N times
  (N ≥ 100), then the output — violations, their order, and their messages — is **byte-identical
  every time**, including on a machine under load.
  - Test: `internal/core/policy/rego_determinism_test.go`
  - Verify: `go test ./internal/core/... -run TestRegoDeterminism -count=1`
  - Level: L1
- **REQ-E11-S06-02** — Given judgment call (b), when the evaluation budget is exceeded, then
  the run fails with a **process error**, and a test asserts the outcome is neither APPROVE nor
  BLOCK nor a silently-skipped rule — the failure mode is "assent could not decide", not a
  decision.
  - Test: `internal/core/policy/rego_budget_test.go`
  - Verify: `go test ./internal/core/... -run TestBudgetExceededIsNotADecision`
  - Level: L1
- **REQ-E11-S06-03** — Given Go map iteration order is random, when violations are emitted,
  then they are canonically sorted before entering the decision path — the same defect class
  the audit found in `internal/change/diff_hcl.go` and `entries.go`.
  - Test: `internal/core/policy/rego_eval.go`
  - Verify: `task determinism`
  - Level: L1

### E11-S07 — Violations → findings, with explicit obligation-proof polarity `[autonomous · engine-grade · maintainer LGTM]`

- **Dependencies**: S06.
- **Definition of done**: a module's violation set maps to findings with `EntryRef` subjects;
  **zero violations never proves an obligation** (judgment call (c)); both polarities tested.

- **REQ-E11-S07-01** — Given ADR-0002 v2's shape, when a module returns violations, then each
  maps to a finding carrying the rule, subject (`EntryRef`), and message, validating against
  the frozen `DecisionRecord` schema with no schema change.
  - Test: `internal/core/policy/rego_findings.go`, `rego_findings_test.go`
  - Verify: `go test ./internal/core/... -run TestRegoViolationsToFindings`
  - Level: L1
- **REQ-E11-S07-02** — Given `later-phases.md`'s explicit polarity rule, when a Rego-backed
  **required obligation** yields zero violations **without** an explicit structured proof,
  then the obligation is **NOT satisfied** and the decision is not APPROVE — asserted by a
  test whose module returns an empty violation set and whose expected outcome is
  *unsatisfied*.
  - Test: `internal/core/policy/rego_polarity_test.go`
  - Verify: `go test ./internal/core/... -run TestEmptyViolationsDoNotProveObligation`
  - Level: L1
- **REQ-E11-S07-03** — Given a module can be broken in ways that resemble success, when a
  module is undefined, returns a non-set value, or produces a malformed violation, then each
  case fails **closed** — three distinct test cases, each asserting `approve == false`.
  - Test: `internal/core/policy/rego_polarity_test.go`
  - Verify: `go test ./internal/core/... -run TestMalformedModuleFailsClosed`
  - Level: L1

### E11-S08 — Aggregation boundary `[autonomous · engine-grade]`

- **Dependencies**: S07.
- **Definition of done**: the envelope owns `match`, `effect`, and `points`; a module cannot
  influence any of them, and the boundary is enforced structurally rather than by convention.

- **REQ-E11-S08-01** — Given ADR-0002 v2 and ADR-0007, when a module emits a value that
  *looks* like an effect or a points value (e.g. `{"effect": "block", "points": 99}`), then it
  is **ignored**: the finding's effect and points come from the envelope, proven by a test
  whose module tries to escalate and whose expected outcome is the envelope's.
  - Test: `internal/core/policy/rego_boundary_test.go`
  - Verify: `go test ./internal/core/... -run TestModuleCannotSetEffectOrPoints`
  - Level: L1
- **REQ-E11-S08-02** — Given ADR-0018, when a Rego-backed rule carries a `phase`, then the
  same never-additive phase ceiling applies as for CEL rules (`CoverWithPhaseCeiling`) — a
  Rego rule is not a phase bypass.
  - Test: `internal/core/policy/rego_boundary_test.go`
  - Verify: `go test ./internal/core/... -run TestRegoRuleRespectsPhaseCeiling`
  - Level: L1

### E11-S09 — `assent lint` hard errors for Rego rules `[autonomous]`

- **Dependencies**: S08.
- **Definition of done**: E3's six hard-error checks have Rego equivalents where meaningful,
  plus Rego-specific ones (missing module, undefined entry rule, denied builtin, both-backends
  leaf); the rule catalogue (`assent catalogue`) reports Rego-backed rules faithfully.

- **REQ-E11-S09-01** — Given E3's framework, when a pack contains a broken Rego rule, then
  `assent lint` exits non-zero with a positioned, contributor-legible message per failure
  class — one test per class.
  - Test: `internal/core/lint/rego_test.go`
  - Verify: `go test ./internal/core/lint/`
  - Level: L1
- **REQ-E11-S09-02** — Given D-048's catalogue rules, when a Rego-backed rule is catalogued,
  then its entry is faithful (authored `phase`, `effectivePhase`, generated `docs.url`) and
  fabricates no lifecycle metadata.
  - Test: `internal/core/catalogue/`
  - Verify: `go test ./internal/core/catalogue/`
  - Level: L1

### E11-S10 — `assent test` support and goldens `[autonomous]`

- **Dependencies**: S08.
- **Definition of done**: an adopter can test a Rego-backed rule with the same
  `expect.yaml` / `cases.yaml` contract (ADR-0014, no schema change); `--coverage` counts Rego
  rules in **both** polarities.

- **REQ-E11-S10-01** — Given ADR-0014, when a Rego-backed rule is exercised by `assent test`,
  then the expectation format is unchanged and both a violating and a non-violating case are
  covered.
  - Test: `examples/` test fixtures
  - Verify: `assent test ./examples/... && task check`
  - Level: L1
- **REQ-E11-S10-02** — Given E6's both-polarity coverage rule, when `--coverage` runs, then a
  Rego rule counts as covered only when **both** polarities are exercised.
  - Test: `internal/core/testharness/`
  - Verify: `go test ./internal/core/...`
  - Level: L1

### E11-S11 — Remove the quarantine `[autonomous]`

- **Dependencies**: S10.
- **Definition of done**: per `docs/planning/rego-escape-hatch.md` ("Only **E11's own
  implementation lane** … may remove the marker"), the `# locked: D-012` marker is removed
  from `examples/policies/rego/**`, the P3-E3-S04 guard is **updated rather than deleted**,
  and the examples enter the schema-validation CI job and the golden corpus.

- **REQ-E11-S11-01** — Given the guard asserts the marker's presence, when the marker is
  removed, then `hack/check-migration-invariants.sh` is updated so it still forbids what
  remains forbidden (no `rego:` leaf in an archetype/starter pack that has not been migrated)
  and no longer asserts a marker that must not exist — the guard is never simply deleted.
  - Test: `hack/check-migration-invariants.sh`
  - Verify: `task check`
  - Level: L1
- **REQ-E11-S11-02** — Given the example was excluded from CI, when the quarantine lifts, then
  `examples/policies/rego/bounded_change.rego` validates, compiles under S04's capabilities,
  and is exercised by `assent test`.
  - Test: `examples/policies/rego/bounded_change.rego`
  - Verify: `task check`
  - Level: L1

### E11-S12 — Docs and maturity truth `[autonomous]`

- **Dependencies**: S11.
- **Definition of done**: README feature-maturity moves Rego from **Locked** to its earned
  tier; **ADR-0002's index status line "pluggable half unbuilt: Rego is E11" is retired**;
  `docs/planning/rego-escape-hatch.md` records that the quarantine was lifted by E11-S11 and
  by what authority; predicate-scope docs state which backend each restriction applies to.

- **REQ-E11-S12-01** — Given the audit's docs-truth family, when E11 ships, then no document
  describes Rego as locked, quarantined, or unbuilt, and none claims a capability S01 struck
  from scope.
  - Test: `README.md`, `docs/adr/README.md`, `docs/planning/rego-escape-hatch.md`
  - Verify: `task check`
  - Level: L1

### E11-S13 — Exit gate `[autonomous]`

- **Dependencies**: S01–S12.
- **Definition of done**: `hack/policy/e11_exitgate_test.sh` proves in one invocation: denied
  builtins fail compilation (all cases); determinism over N ≥ 100 runs; the empty-violations
  polarity test present and failing-closed; the effect/points boundary held; the scoped schema
  drift confined to `merge-policy.schema.json`; `schemas/decision/**` unchanged; the example
  unquarantined and green; `task check` green.

- **REQ-E11-S13-01** — Given every prior story, when the exit gate runs, then it fails on any
  regression above and cites D-141 plus ADR-0002 v2.
  - Test: `hack/policy/e11_exitgate_test.sh`
  - Verify: `bash hack/policy/e11_exitgate_test.sh && task check`
  - Level: L1
