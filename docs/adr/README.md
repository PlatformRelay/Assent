# Architecture Decision Records

Format: see [`template.md`](template.md). Numbered, immutable once **Accepted** (supersede,
don't edit). Lifecycle: draft → **Accepted** → (`Superseded by ADR-nnnn`).

Design-phase records are firmed up via trade-off matrices and spikes in the planning
workflow (see [`../planning/meta-plan.md`](../planning/meta-plan.md)).
**Phase-2 gate (P2-E5, 2026-07-21):** ADR-0002–0017 Accepted — evidence in
[`../planning/adr-acceptance-review.md`](../planning/adr-acceptance-review.md); partial
supersessions by ADR-0016/0017 are noted on each ADR's status line (not full
`Superseded by` replacements).

| ADR | Title | Status |
| --- | --- | --- |
| [0001](0001-language-go-single-binary.md) | Implementation language: Go, single static binary | Accepted |
| [0002](0002-policy-frontends-rego-declarative.md) | Policy surface: one Kyverno-style YAML envelope, pluggable expression backends | Accepted (v2) — **pluggable half unbuilt: Rego is E11** |
| [0003](0003-canonical-change-model.md) | Canonical change model for JSON / YAML / HCL-tfvars (incl. deletions & renames) | Accepted (partial: ADR-0017 §5) |
| [0004](0004-plugin-architecture.md) | Plugin architecture for permission & fact providers | Accepted |
| [0005](0005-forge-abstraction-gitlab-first.md) | Forge abstraction: GitLab first, GitHub second | Accepted (partial: ADR-0017 §1/§7) |
| [0006](0006-testing-strategy.md) | Testing strategy: spec-driven pyramid with real-forge e2e | Accepted — **no CI e2e job; L3 is operator-armed** |
| [0007](0007-rule-effects-decision-aggregation.md) | Rule effects and decision aggregation (incl. risk points) | Accepted (partial: ADR-0017 §2/§3) |
| [0008](0008-change-classification-routing-scope.md) | Change classification, ruleset routing, and rule scope | Accepted |
| [0009](0009-execution-modes.md) | Execution modes: CI, local/dry-run, explain, webhook, scan/stats | Accepted (partial: ADR-0017 §4) — **substantially unimplemented: only `run` exists** (D-135) |
| [0010](0010-config-files-repo-layout.md) | Configuration files and governed-repo layout | Accepted (partial: ADR-0017 §2/§5) |
| [0011](0011-core-ports-and-contracts.md) | Core Go ports and public contracts (draft shapes) | Accepted (partial: ADR-0017 §1/§7) |
| [0012](0012-presentation-templates-debug.md) | Presentation: comment rendering, expandable details, docs links, rule debug | Accepted (override → ADR-0016) |
| [0013](0013-assert-syntax-and-backend.md) | `assert` syntax and backend: CEL-leaf condition trees on cel-go ([gallery](0013-appendix-syntax-gallery.md)) | Accepted |
| [0014](0014-adopter-test-format.md) | Adopter test format — policy tests as a public contract | Accepted (partial: ADR-0017) |
| [0015](0015-trust-boundaries-merge-integrity.md) | Trust boundaries and merge-time integrity (from the 2026-07-21 adversarial review) | Accepted (partial: ADR-0017 §1/§4/§6) |
| [0016](0016-presentation-theming.md) | Presentation theming: config knobs, slots, CEL messages, render contract | Accepted |
| [0017](0017-contract-model-obligations.md) | Contract model: governed subjects, required obligations, typed facts, preconditioned reconciliation | Accepted |
| [0018](0018-policy-lifecycle-phase-profile-comparison.md) | Policy lifecycle — phase, profiles, comparison | Accepted (D-030) |
| [0019](0019-publication-marker-reconciliation-protocol.md) | Publication marker + reconciliation protocol (database-free) | Accepted (D-030) — **one MUST unmet: doctor omits `duplicate_prevention:`** |
| [0020](0020-forge-snapshot-changed-file-completeness.md) | Forge snapshot changed-file completeness contract | Accepted (D-119) |
