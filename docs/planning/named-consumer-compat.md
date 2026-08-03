# Named-consumer compatibility review (D-017)

A behavioral-compatibility assessment was run between assent's committed design and a
private, production Kafka self-service merge gate that serves as the project's **reference
use case**. Assent is not affiliated with that platform or its operator organization; it is
an independent open-source project that generalizes the same class of use cases — the
reference system was purpose-built for one very complex self-service repository, assent is
the generic answer to that problem shape. The question assessed: can assent's architecture
reproduce the reference system's complete observable safety behavior?
The answer: **yes eventually; no under the committed v1 scope** — the gap is operational
contracts and policy lifecycle, not rule expressiveness. This document records the critical
reflection on the proposed changes and where each one lands in the backlog.

The reference use case is hereby treated as **the first named consumer** in the sense of D-012.
That is exactly what D-012 was designed for: deferred seams unlock when a real consumer
commits, and this consumer is real, demanding, and operated by the project owner.

Compatibility means equivalent *safety behavior* (same governed change → equivalent
decision, safeguards not weakenable, same required human evidence, equivalent freshness/
race/rate controls, auditable outcomes) — **not** identical commands, interfaces, report
wording, or policy source. Domain-specific (Kafka, org-directory) logic stays outside the
generic core as packs/providers.

## Disposition of the proposed changes

| # | Proposal | Disposition | Where | When |
| --- | --- | --- | --- | --- |
| B1 | Complex pure rule backend (multi-pass / cross-manifest / graph checks) | **Accept contract unlock**; implementation stays post-skeleton | E11 flips Locked → Unlocked (D-017); Phase-3 schemas stay backend-neutral | contract: Phase 3 · impl: named-consumer expansion |
| B2 | First-class rollout phase `off`/`observe`/`enforce` | **Accept** — reverses the OQ-21 lean | new [P3-E4](https://github.com/PlatformRelay/assent/blob/main/openspec/specs/later-phases.md) · [ADR-0018](../adr/0018-policy-lifecycle-phase-profile-comparison.md) (Accepted, D-030) | Phase 3 (schema) |
| B3 | Policy profiles + counterfactual comparison | **Accept records/schema**; `compare` CLI later | P3-E4 · [ADR-0018](../adr/0018-policy-lifecycle-phase-profile-comparison.md) (Accepted, D-030) | Phase 3 (schema) · impl Phase 5+ |
| B4 | Corpus snapshots + machine-enforceable promotion gates | **Accept** as versioned PolicyComparisonSuite | P3-E4 (format) + E6 (runner) · [ADR-0018](../adr/0018-policy-lifecycle-phase-profile-comparison.md) (Accepted, D-030) | Phase 3 (format) · Phase 5 (runner) |
| B5 | Typed ApprovalEvidence contract | **Accept** — resolves OQ-23 | P3-E1 schema slice (dossier P1-E3-S02 feeds it) | Phase 3 |
| B6 | Database-free marker/reconciliation protocol + per-MR serialization | **Accept** — preserves D-007 | new [P3-E5](https://github.com/PlatformRelay/assent/blob/main/openspec/specs/later-phases.md) (ADR-0019); P4-E1 exit gate extended | Phase 3 (freeze) · Phase 4 (minimal impl) |
| B7 | Merge caps / rate budgets | **Seam now, implement later**; guarantees scoped honestly (see below) | E12 (rescoped service tier) | post-Phase-4 |
| B8 | Post-merge audit + remediation (OQ-19) | **Seam now, implement later** — correlation pins already exist in the record model | E12 | post-Phase-4 |
| B9 | Batch/sweep orchestration (OQ-20) | **Seam now, implement later**; no bulk bypass of per-MR preconditions, ever | E12 | post-Phase-4 |
| B10 | Generated machine-readable rule catalogue | **Accept** — cheap, additive-tolerant report; not a safety schema | E3 seed (generation) + E9 (docs) | Phase 5 |
| B11 | Kubernetes CRD/CR validation adapter | **Spike first, then decide** — the report itself concedes the dependency risk | new [P2-E6 Spike D](https://github.com/PlatformRelay/assent/blob/main/openspec/specs/p2-e6-spike-crd/spec.md) → ADR-0020 → E14 (Planned, gated) | spike: Phase-3 window (D-018 — not first-wave) · adapter: post-skeleton |

## Where the report over-reaches (declined or softened)

1. **"Do not freeze Phase 3 contracts until the compatibility fixtures pass."** Softened.
   A full fixture corpus can only *pass* once an engine exists; making that a freeze
   precondition deadlocks contracts-before-engine. What the freeze gate actually gains:
   **one** sanitized named-consumer compatibility fixture that must *validate against the
   schemas* and be representable without inferring anything from messages/labels/names
   (phase, profile, comparison delta, approval evidence, marker fields, budget reservation
   as structured fields). The full passing corpus is the *compatibility claim* gate, after
   implementation.
2. **Forge artifacts as an atomic cross-run budget ledger.** Not assumed. Atomicity of
   forge-artifact CAS is unproven; until proven, the supported guarantee is per-run caps +
   one serialized sweep. Anything stronger is stated unsupported and `doctor` reports which
   guarantee the deployment actually provides. (The report allows this scoping; we make it
   the default rather than the fallback.)
3. **Multi-replica HA duplicate prevention.** Explicitly unsupported. Single publisher per
   MR (CI `resource_group` / keyed lock) is the supported topology; multi-replica setups
   only converge duplicates on the next reconciliation.
4. **CRD support as a committed first-class adapter.** Downgraded to spike-gated.
   Kubernetes structural-schema + `x-kubernetes-validations` semantics are either a large
   dependency bet (k8s libs vs small static binary) or a high-risk reimplementation. Spike D
   measures both before any contract commitment; the Phase-3 CRD fixture is added only if
   the spike verdict supports it.
5. **Ratified declines from the report**: no loading of the reference system's Go rules, no in-process
   plugin API, no command/flag/wording parity, no org- or Kafka-specific knowledge in core,
   no private policy content in this repository (D-002 sanitization applies to every
   fixture derived from the reference system).

## What stays locked

E10 (GitHub adapter) and E13 (remote packs) remain **Locked (D-012)** — the named consumer
runs GitLab and local packs; nothing here names a consumer for those seams.

## Sequencing (unchanged spine, extended gates)

Phases 1–2 proceed as planned (plus the parallel Spike D, which does not block the Phase-2
ADR-acceptance gate). Phase 3 grows two epics (P3-E4, P3-E5) and the extended P3-E1 gate.
The walking skeleton stays thin — one MR, one rule, one provider, one reconciled result —
but now proves rerun-idempotent publication. Only after the D-012 adoption gate (one real
repo on live MRs) does the named-consumer expansion start: port a representative rule set +
graph provider, observe/enforce comparison over an immutable corpus, approval evidence,
then service-tier lifecycle (E12), then the remaining rules and companion tooling.

New ADRs are authored inside their owning epics (ADR-0018 lifecycle, ADR-0019 publication
protocol, ADR-0020 Kubernetes adapter) and are accepted at the Phase-3 freeze review — the
P2-E5 acceptance round still covers only ADR-0002–0017.
