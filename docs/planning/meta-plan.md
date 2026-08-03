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

Spec-first, vertical slices per epic (proposed cut, refined in Phase 3):

| Epic | Slice |
| --- | --- |
| E1 | Canonical change model: JSON + YAML (+ HCL/tfvars) |
| E2 | Decision engine + Rego frontend |
| E3 | Declarative YAML frontend |
| E4 | Forge: GitLab adapter (threads, approve, merge) |
| E5 | Provider host: built-ins + HTTP/exec |
| E6 | Adopter test harness (`assent test`) + examples |
| E7 | E2E infra: kind GitLab, sample-repo generator, conformance suite |
| E8 | Forge: GitHub adapter + Actions entrypoint |
| E9 | Distribution: releases, container, CI templates, docs site |

Ordering constraint: E7 starts early (alongside E1) because every later epic's exit gate
depends on it.

## Standing rules

- TDD mandatory; one logical change per commit; gitmoji-conventional commits.
- No employer/internal references in any artifact (D-002). Sanitization check in CI later.
- Open questions live in [open-questions.md](open-questions.md) with OQ IDs; an OQ blocks the
  phase gate it is tagged with.
