# ADR-0002: Policy surface: one Kyverno-style YAML envelope, pluggable expression backends

| | |
| --- | --- |
| **Status** | Accepted (v2 — supersedes the "two parallel frontends" draft of this ADR; P2-E5). **The pluggable-backend half is UNBUILT as of 2026-08-09**: the YAML envelope and the CEL/`assert` backend are Core, but the **Rego backend is deferred to E11** and nothing selects a backend today — no `opa`/`rego` module dependency, no backend field in any frozen v1alpha1 policy schema. `README.md`'s maturity table (*Rego backend — Locked, E11*) and [`docs/architecture/c4-container.md`](../architecture/c4-container.md) (*PLANNED — E11*) are the accurate surfaces; this ADR is the design, not a statement of what ships. Do **not** cite Rego as an available escape hatch — see [ADR-0013](0013-assert-syntax-and-backend.md) Amendment 1 and D-012. |
| **Date** | 2026-07-21 (revised) |
| **Deciders** | Konrad Heimel |
| **Context links** | [ADR-0003 change model](0003-canonical-change-model.md) · [ADR-0007 effects](0007-rule-effects-decision-aggregation.md) · [ADR-0008 routing](0008-change-classification-routing-scope.md) · D-006 |

## Context

The first draft proposed Rego and declarative YAML as two *parallel, equivalent* frontends.
Review verdict: **too much** — two documentation surfaces, an equivalence test matrix, and
permanent feature drift between them. At the same time, YAML-only hits an expressiveness
ceiling and Rego-only scares off the primary audience (operator preference is Kyverno-style —
D-006).

The unlock: routing, matching, effects, risk points, and scope (ADR-0007/0008) are
**structural** concerns that belong in a declarative envelope no matter what — Rego should
never own orchestration. Only the *predicate inside a rule* needs an expression language. That
part can be pluggable without creating a second frontend.

## Options

| Option | Pros | Cons |
| --- | --- | --- |
| Two parallel frontends (v1 draft) | each audience fully served | double docs/tests, drift; rejected |
| Rego only | max power, OPA tooling | wall of Rego; loses the preferred UX |
| YAML only (assertion trees) | lowest barrier | ceiling: cross-entry logic, branch-state conventions get ugly |
| **One YAML envelope; rule bodies choose a backend: `assert` (assertion tree / CEL) or `rego` (module escape hatch)** | one document model, one doc set; 80% never see Rego; Rego available where it earns its keep; backends are tiers, not equivalents — no equivalence testing | envelope schema must be designed carefully; two expression languages to document (but scoped to rule bodies) |
| Call Kyverno proper as engine | reuse mature engine | Kyverno's engine is K8s-native (GVK match, admission semantics, CRD lifecycle) — we'd fake AdmissionReviews and lose old/new diff semantics; wrong fit |

## Decision (proposed)

**One policy document format** — Kyverno-inspired YAML (`MergePolicy` + `RulesetBinding`
kinds). The envelope owns: match/classification hooks, environment routing, rule scope,
effects, risk points, messages. Each rule's predicate is one of:

1. **`assert`** — declarative assertion tree / CEL expression. Default tier; covers the
   archetypes. Implementation candidates (Spike A decides, OQ-11):
   **[kyverno-json](https://kyverno.github.io/kyverno-json/latest/go-library/)** embedded as a
   Go library (`pkg/jsonengine`) — genuine Kyverno assertion-tree semantics and syntax
   familiarity for free — vs. a native **CEL** (`cel-go`) evaluator (Kyverno itself moved to
   CEL for its new ValidatingPolicy types, so CEL *is* Kyverno-style now). Either way the
   engine is wrapped behind our own interface so the choice is reversible.
2. **`rego`** — inline or file-referenced Rego module (embedded OPA), receiving the same
   PolicyInput scope and returning findings data only. Escape hatch for cross-entry checks,
   complex derivations, whole-branch conventions.

Rego **never** controls routing, effects, or aggregation — it computes; the envelope decides.
Downstream (engine, findings, harness, docs) a rule is a rule regardless of backend.

## Consequences

- The "isn't this too much?" problem dissolves: there is exactly one frontend; backends are
  tiers of one surface, documented as "start with `assert`, graduate to `rego`".
- Syntax familiarity is deliberately *stolen* from Kyverno (match/exclude, validate, message
  templating, `apiVersion`/`kind` envelope); semantics for git-diff payloads are ours.
- Chainsaw is the wrong layer for the engine (it's a K8s e2e test orchestrator), but its
  declarative assert-file UX is the model for our **policy test harness** fixture format.
- Whether `assert` is implemented on kyverno-json or cel-go is an implementation detail
  hidden behind the wrapper — but the *authored syntax* it implies is not; Spike A must fix
  the syntax before Phase 3 freezes contracts.

## Counterpoints considered

- *"Just use conftest/Rego, it exists."* — conftest proves Rego-over-config-files works, but
  offers no envelope: no effects, routing, risk, or resolvable-thread semantics. We'd rebuild
  the envelope anyway — the actual product — and inherit the steep default UX.
- *"kyverno-json is pre-1.0 with a small maintainer pool."* — True; that's why it sits behind
  our wrapper interface with cel-go as the recorded fallback (OQ-11).
