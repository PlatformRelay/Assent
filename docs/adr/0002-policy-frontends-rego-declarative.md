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
| YAML only (assertion trees) | lowest barrier | ceiling: cross-entry logic, branch-state conventions get ugly — **superseded 2026-09-03, see Amendment 1** |
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
   PolicyInput scope and returning findings data only. Its reach over tier 1 was **measured**
   against the surface this repo actually binds (E11-S01 / D-156,
   [`docs/planning/rego-tier-ceiling.md`](../planning/rego-tier-ceiling.md)) and is **two
   shapes, both unconditional on today's shipped input contract**: **folds and aggregates** over
   an in-input collection — tier 1 has `size()` and no `sum`, `reduce`, `math.*` or `lists.*`, so
   *counting* is expressible and *summing* is not — and **unbounded graph reachability** over an
   adjacency **deliverable** as a `{type: string, cardinality: set}` fact, which tier 1 cannot
   spell at any depth because the number of nested iteration *levels* in a CEL expression is
   *syntactic*. (Deliverable, not shipped: no provider in the corpus returns an encoded adjacency
   today — the ceiling record's §5 caveat.) **Not** cross-entry,
   cross-manifest or whole-branch checks: those fail on **input availability**, and the `rego`
   tier is pinned to the identical `EvaluationInput` (REQ-E11-S05-01), so it fails them
   identically. See **Amendment 1** for the claim this replaces.

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
- The wrapper (`PredicateBackend`, ADR-0011) is what keeps adding the `rego` tier a placement
  decision rather than a redesign: where the OPA evaluator lives and how its runtime
  capabilities (`http.send`, wall-clock, randomness) stay contained inside the guarded core
  tree is settled in ADR-0011's amendments (D-141/D-144). Rego still never touches routing,
  effects, or aggregation — only what it computes as a predicate.

## Counterpoints considered

- *"Just use conftest/Rego, it exists."* — conftest proves Rego-over-config-files works, but
  offers no envelope: no effects, routing, risk, or resolvable-thread semantics. We'd rebuild
  the envelope anyway — the actual product — and inherit the steep default UX.
- *"kyverno-json is pre-1.0 with a small maintainer pool."* — True; that's why it sits behind
  our wrapper interface with cel-go as the recorded fallback (OQ-11).

## Amendment 1 (2026-09-03, D-166 — the `rego` tier's justification is folds and unbounded graph reachability, **not** cross-entry checks)

**Withdrawn.** Until this amendment the `rego` bullet above read, verbatim:

> Escape hatch for cross-entry checks, complex derivations, whole-branch conventions.

**Why it was wrong, not merely imprecise.** E11-S01 measured the tier-1 CEL ceiling against the
surface this repo actually binds — `newEvalEnv`'s eleven frozen predicate-scope variables, **zero
extension libraries** — and D-156 (2026-08-23) struck three of the four shapes that sentence
gestures at:

- **Cross-entry / cross-manifest** checks are expressible at tier 1 where the data is present
  (`x in facts.<p>.<n>.value` for membership; a purpose-built provider or
  `facts.<p>.<n>.value[key]` for keyed lookup — both shipped and lint-clean), and where the data
  is *absent* the blocker is **input availability, not expressiveness**. The evaluation unit is
  one file, and REQ-E11-S05-01 pins the Rego module to the *identical* `EvaluationInput` with the
  tier fenced to "declared data, no I/O" — **so Rego fails those rules identically**. A second
  backend does not fix an input-contract limitation.
- **"Complex derivations"** — the named-intermediate shape — is struck: `cel.bind` is absent, but
  re-deriving the sub-expression inside each leaf is semantically identical, and the surface's
  value binder `[expr].all(v, …)` computes it once. The residual is legibility, which is not a
  licence to add a backend.
- **"Whole-branch conventions"** is the same input-availability limit under another name.

**What survives, and it is narrower.** Two shapes, both **unconditional** on today's shipped
input contract: folds/aggregates over an in-input collection, and unbounded graph reachability
over an in-input adjacency. The `rego` bullet above now states those and nothing else.

**Blast radius of the wrong sentence.** It was published on an Accepted ADR and reachable from
the ADR index; the correction was tracked as backlog residual **E11-R01** precisely because
REQ-E11-S12-01's `Test:` list **omitted** `docs/adr/0002-*`, so **no gate would ever have caught
it** — it was invisible to CI by construction, and E11-S12 is story 12 of 14. **That omission is
now closed:** this file **is** named in REQ-E11-S12-01's `Test:` list, so it sits inside S12's
sweep rather than outside it. That buys a **review** pin and not a gate — nothing
machine-consumes a REQ's `Test:` list, and the only gate that reads ADRs at all compares Status
rows against `docs/adr/README.md`, never body content — so a regression re-inserting the withdrawn
sentence into this file would still fail no check today.

**Left standing deliberately.** The **Options** table's *"YAML only (assertion trees) — ceiling:
cross-entry logic, branch-state conventions get ugly"* cell is a record of what was believed
during the 2026-07-21 deliberation, not a claim this ADR makes today. It is superseded by this
amendment and is not restated as current.
