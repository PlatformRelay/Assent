# P2-E6 — Spike D: Kubernetes CRD/CR validation feasibility

**Problem**: the named-consumer review (D-017) proposes an optional Kubernetes adapter —
validate `CustomResourceDefinition` documents themselves *and* Custom Resource instances
against the matching CRD version's structural OpenAPI v3 schema, including
`x-kubernetes-validations` CEL rules. Reimplementing Kubernetes structural-schema semantics
is high-risk; importing Kubernetes libraries may conflict with the small-static-binary goal.
Decide feasibility and dependency strategy **before** any Phase-3 CRD contract fixture or
E14 commitment.
**Scope**: dependency measurement, semantic conformance on real CRDs, trust-rule
demonstration. **Non-goals**: adapter implementation (E14); admission webhooks, mutating
admission, live cluster state; Helm/Kustomize rendering (rendered manifests only).
ADRs: feeds a dedicated ADR (new, authored post-spike; number assigned when Spike D closes); 0003 (fail-closed limits), 0015 §1
(target-ref trust); D-017 B11. Does **not** gate P2-E5 (which covers ADR-0002–0017 only).

## P2-E6-S01 — Dependency and semantics spike on real CRDs

- **Goal**: validate a small corpus of real public CRDs and conforming/violating CR
  instances using the Kubernetes apiextensions libraries (structural schema validation +
  `x-kubernetes-validations` CEL, including `oldSelf` transition rules when a prior
  resource is supplied); measure the dependency and binary cost of that import; record a
  feasibility verdict: adopt-libs / reimplement-bounded-subset / defer.
- **Operator input**: no.
- **Dependencies**: none (runs parallel to P2-E1..E5).
- **Definition of done**: `docs/planning/spikes/spike-crd.md` has sections
  `## Conformance results`, `## Dependency cost` (module count + binary-size delta vs a
  baseline build), `## Feasibility verdict`; adversarial inputs (structurally invalid CRD,
  unknown served version, unsupported conversion/defaulting) all land on fail-closed
  REVIEW-shaped outcomes, never silent pass.

Requirements:

- **REQ-P2-E6-S01-01** — Given a real CRD and a pair of conforming/violating CR instances,
  when validated through the spike harness, then the accept/reject outcomes match the
  expected fixture verdicts for every case in the corpus, with structured findings carrying
  GVK, schema path, and instance path.
  - Test: `hack/spikes/crd/conformance_test.go`
  - Verify: `go test ./hack/spikes/crd/ -run TestConformance`
  - Level: L0
- **REQ-P2-E6-S01-02** — Given a CRD carrying an `x-kubernetes-validations` rule with
  `oldSelf`, when the prior resource is supplied the transition rule evaluates correctly,
  and when it is absent the result is fail-closed (REVIEW-shaped), never assumed-valid.
  - Test: `hack/spikes/crd/celrules_test.go`
  - Verify: `go test ./hack/spikes/crd/ -run TestCELRules`
  - Level: L0
- **REQ-P2-E6-S01-03** — Given the spike module built with and without the Kubernetes
  validation imports, when sizes and module graphs are compared, then the report records
  the binary-size delta and direct/transitive module counts, and the
  `## Feasibility verdict` states one of adopt-libs / reimplement-bounded-subset / defer
  with the decisive reason.
  - Test: `docs/planning/spikes/spike-crd.md`
  - Verify: `grep -q "Feasibility verdict" docs/planning/spikes/spike-crd.md && grep -qi "binary" docs/planning/spikes/spike-crd.md`
  - Level: doc

## P2-E6-S02 — Trust rule: branch-modified schemas cannot self-authorize

- **Goal**: demonstrate the non-negotiable trust behavior on the spike harness: when a
  change modifies a CRD **and** matching CR instances together, the gating validation uses
  the target-ref schema; the proposed schema may be compiled for observe-style comparison
  only; a brand-new CRD arriving with its first instances (no trusted schema) yields
  human-review, not auto-merge.
- **Operator input**: no.
- **Dependencies**: P2-E6-S01.
- **Definition of done**: both scenarios encoded as fixtures with expected outcomes; report
  section `## Trust rule` states the rule and links the fixtures; the outcome feeds the
  Kubernetes-adapter ADR draft (number assigned when Spike D closes) and the decision whether the Phase-3 fixture set gains a CRD fixture.

Requirements:

- **REQ-P2-E6-S02-01** — Given an MR that relaxes a CRD schema and adds a CR valid only
  under the relaxed schema, when gating validation runs, then the CR is judged against the
  target-ref schema (violation reported); the relaxed-schema result appears only as a
  separate observe-labelled record.
  - Test: `hack/spikes/crd/trust_test.go`
  - Verify: `go test ./hack/spikes/crd/ -run TestTrustTargetRef`
  - Level: L0
- **REQ-P2-E6-S02-02** — Given a new CRD introduced together with its first CR instances,
  when gating validation runs with no trusted schema available at target ref, then the
  outcome is REVIEW-shaped (human required), never auto-approve.
  - Test: `hack/spikes/crd/trust_test.go`
  - Verify: `go test ./hack/spikes/crd/ -run TestTrustNewCRD`
  - Level: L0
