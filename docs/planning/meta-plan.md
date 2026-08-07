# Meta-plan: how we get to a precise plan

This is deliberately a **plan for making the plan**. The scaffold (this repo) fixes the frame;
the phases below turn the frame into specs precise enough to implement test-first. Each phase
has an exit gate — we do not start implementation epics until Phase 4's gate passes.

## Phase 0 — Frame (done with this scaffold)

- Vision written ([vision.md](../vision.md)), seed ADRs drafted (0001 accepted, 0002–0006
  proposed), C4 sketches, decision log started.
- **Gate**: operator has read vision + ADRs and vetoed nothing fundamental.

## Phase 1 — Requirements harvest

Collect the raw material the specs will be distilled from:

1. **Sample repos** — operator provides 2–3 further representative self-service repo shapes
   (topic-style YAML, catalog JSON, tfvars); we *re-create generic equivalents* under
   `examples/repos/` — never verbatim copies of any private content (D-002/D-008). In
   parallel, curate **open-source corpora** (kubernetes/org, JulieOps topologies, octoDNS
   zones, Backstage catalogs — candidates in `examples/repos/README.md`, OQ-16) as public
   test targets and demos.
2. **Rule archetype inventory** — enumerate every rule class the archetypes in vision.md must
   generalize; for each: inputs needed (change paths, facts, permissions), decision semantics,
   failure mode. This becomes the acceptance bar for the policy frontends.
3. **Forge behaviour dossier** — for GitLab *and* GitHub: exact API mechanics for resolvable
   threads, approvals, self-approval limits, bot identity, merge preconditions. Feeds ADR-0005.
4. **Prior-art review** — OPA/conftest, Kyverno (syntax inspiration), Mergify, Bors/merge
  queues, Renovate's automerge, Prow/Tide, danger.js: what each got right/wrong for *this*
  use case; recorded per tool in `docs/planning/prior-art.md`.

- **Gate**: archetype inventory reviewed; every archetype has at least one concrete example
  change + expected decision written down.

## Phase 2 — Decide the proposed ADRs

Run each Proposed ADR (0002–0006) through a weighted trade-off matrix + targeted spikes:

- **Spike A (policy):** implement the *bounded-change* and *ownership* archetypes in raw Rego
  against a hand-written PolicyInput; then sketch the YAML equivalent; decide "YAML lowers to
  Rego" vs. "two evaluators" (OQ-3).
- **Spike B (e2e):** boot GitLab CE as a testcontainer and in kind; measure boot time, RAM,
  flakiness; decide the CI default (OQ-6).
- **Spike C (plugins):** wire a toy Keycloak-group provider twice — as exec provider and as
  go-plugin — and decide whether tier 3 ships in v1 (OQ-4).

- **Gate**: ADR 0002–0006 moved to Accepted (or superseded), each with matrix + spike evidence.

## Phase 3 — Contracts first

Freeze the five public contracts as versioned schemas **with fixtures before any engine code**:
PolicyInput, Decision/Findings, Provider request/response, Forge-port conformance suite
(as executable spec skeletons), and the **adopter test fixture format** (ADR-0014, D-010).

- **Gate**: contracts reviewed; golden fixtures exist; `openspec/specs/` epics written with
  REQ IDs, each REQ carrying `Test:` and `Verify:` per [openspec/config.yaml](https://github.com/PlatformRelay/assent/blob/main/openspec/config.yaml).
- **Gate artifact (ADR-0017 §8, D-016)**: the **strict end-to-end contract fixture** —
  pinned target/merge result, renamed entry with stable identity, two required obligations,
  an expired typed fact, a missing required approval, expected DecisionRecord +
  PresentationModel + publication preconditions — validated against the same JSON Schemas
  as every example. No engine code before this fixture exists.

## Phase 4 — Walking skeleton (first implementation slice)

Thinnest end-to-end slice, TDD throughout: CLI runs in a GitLab CI job on a kind-hosted GitLab
sample repo → parses a one-field YAML change → evaluates one declarative rule → posts one
resolvable thread or approves + merges → emits the JSON report. Everything real, everything
minimal.

- **Gate**: the L3 e2e for the skeleton is green and replayable; determinism gate active.
- **Adoption gate (D-012)**: Phase 4 is not "done" until **one real repository** (a personal/
  demo self-service repo counts, a synthetic fixture does not) has run assent on live MRs.
  Deferred tiers (rego, gRPC, WASM, GitHub, serve, remote packs) unlock only with a named
  consumer — seams stay designed, contracts stay unfrozen until then.

## Phase 5 — Epic execution

Spec-first, vertical slices per epic. The E-numbering below is the one that actually
executed — each row names its spec under `openspec/specs/` — and it matches the README
feature-maturity table. (The Phase-2 proposed cut had E2 as a Rego frontend and E8 as the
GitHub adapter; both moved to the deferred tier, so the numbering shifted.)

| Epic | Slice | Spec | Status |
| --- | --- | --- | --- |
| E1 | Canonical change model: JSON + YAML (+ HCL/tfvars) | `p5-e1-canonical-change-model` | shipped |
| E2 | Decision engine + CEL predicate backend | `p5-e2-decision-engine` | shipped |
| E3 | Policy surface: `assent lint` hard errors + rule catalogue | `p5-e3-policy-surface` | shipped |
| E4 | GitLab forge adapter: Snapshot / Resolve / Reconcile | `p5-e4-gitlab-forge` | shipped |
| E5 | Provider host + builtins (HTTP/exec, gitlab-groups, ownership) | `p5-e5-provider-host` | shipped |
| E6 | Adopter test harness (`assent test`) + `assent compare` seed | `p5-e6-adopter-test` | shipped |
| E7 | E2E & conformance infra | `p5-e7-e2e-conformance` | shipped |
| E8 | Renderer & presentation (ADR-0016 tier 0) | `p5-e8-renderer` | shipped |
| E9 | Distribution & release (oss-playbook) | `p5-e9-distribution` | shipped (v0.1.0) |

Follow-on epics cut during Phase 5, outside the E1–E9 sequence: **EFE**
(`p5-e-fileevents`, whole-file `match.fileEvents`), **PCS**
(`p5-pcs-policy-comparison`, full comparison-suite runner), **AUD**
(`p5-aud-audit-remediation`, post-release audit remediation).

Deferred tiers keep their own numbers and unlock only with a named consumer (D-012):
**E10** GitHub adapter, **E11** Rego backend, **E12** `serve` (HTTP API), **E13** remote
packs — see the feature-maturity table in `README.md`.

Ordering constraint: E7 starts early (alongside E1) because every later epic's exit gate
depends on it.

## Standing rules

- TDD mandatory; one logical change per commit; gitmoji-conventional commits.
- No employer/internal references in any artifact (D-002). Sanitization check in CI later.
- Open questions live in [open-questions.md](open-questions.md) with OQ IDs; an OQ blocks the
  phase gate it is tagged with.
