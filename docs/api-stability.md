# API stability

Public compatibility promises for assent's frozen contracts (oss-playbook #8).
Schemas under `schemas/` are the public API (ADR-0017 §7) — there is no v1 SDK.
This document states what each contract guarantees at the current version — policy schema,
decision contract, and adopter test format — when a bump may break adopters, and which
generalizations are permanently out of scope.

Cross-links (planning / design authority, not duplicate catalogs):

- Compatibility rules & scope guards: [`docs/adr/0017-contract-model-obligations.md`](adr/0017-contract-model-obligations.md) §9
- Broader `assent lint` hard-error list: [`docs/planning/lint-hard-errors.md`](planning/lint-hard-errors.md)
- Executable fixtures for §9 rules: `schemas/testdata/compat/` (P3-E2-S01..S05)

## Contract guarantees

| Public contract | Current version | Compatibility guarantee | Graduation criteria (leaving `v1alpha1`) |
| --- | --- | --- | --- |
| **Policy schema** (Config, RulesetBinding, MergePolicy under `schemas/policy/`) | `assent.dev/v1alpha1` | **Strict-decode, no unannounced breaking change.** Unknown fields, unknown enums, and duplicate named-collection IDs reject at decode/lint. Additive field additions within a major require an openspec change + version bump before they become required. No silent coercion. | Phase-3 freeze review ratifies the shapes; ≥1 named consumer validates against the published schemas; breaking changes ship only as a new `apiVersion` with a documented migration window. |
| **Decision contract** (EvaluationInput, DecisionRecord, ReplayBundle, PresentationModel, PublicationReceipt under `schemas/decision/`) | `assent.dev/v1alpha1` | **Reports are additive-tolerant within a major** (unknown *top-level* fields must not hard-fail older consumers); nested safety-bearing shapes stay closed. Provider protocol majors negotiate exactly (`accept` iff majors match; otherwise capability gap → facts unavailable, never auto-merged). Hashes use canonical JSON with schema-version domain separation. | Same Phase-3 freeze gate; additive report fields may appear without a major bump only when older consumers remain correct by ignoring unknowns; removing or reinterpreting a required nested field requires a new `apiVersion`. |
| **Adopter test format** (TestExpectation / `expect.yaml` + `cases.yaml` under `schemas/testfixture/`) | `v1alpha1` (ADR-0014) | **Must-contain findings by default** (`exact: true` is opt-in closed-list). Decision enum and finding shape are closed within the version; new assertion fields are additive-only after an openspec change. Fixture directory layout (`.assent/tests/…`) is part of the contract. | Adopter harness (`assent test`) and shipped example packs validate against the same schema; leaving `v1alpha1` requires a migration note for any default-semantics change (especially `exact` / findings matching). |

## Graduation criteria (summary)

Leaving `v1alpha1` for any row above requires all of:

1. **Freeze review** — Phase-3 contract freeze ratified (meta-plan gate / ADR-0017 §8 fixture).
2. **Named-consumer proof** — at least one real consumer validates production-shaped documents against the published schemas (D-012 / D-017).
3. **Versioned break** — any incompatible change introduces a new `apiVersion` string; within a major, changes are additive-only for reports and announced-only for authored policy.
4. **No silent re-baseline** — hash vectors, strict-decode, and do-not-generalize fixture suites stay green; expected digests and adversarial fixtures are never quietly rewritten to hide a regression.

## Do-not-generalize (lint errors)

ADR-0017 §9 freezes a do-not-generalize list for v1 (extends D-012). Each item below is a
**named, stable `assent lint` hard-error code**. Proposing any of these on the accepted
config/policy surface must fail lint (and already fails schema validation via the fixtures
under `schemas/testdata/compat/do-not-generalize/`). These codes complement — they do not
replace — the broader hard-error table in [`docs/planning/lint-hard-errors.md`](planning/lint-hard-errors.md).

| Stable error code | Forbidden surface | Rationale (ADR-0017 §9) |
| --- | --- | --- |
| `no-user-defined-effects` | Effects outside the frozen enum (`comment` / `challenge` / `block` / `require-review` on failure) | ADR-0017 §9: no user-defined effects — `score` is not an effect; points stay rule-outcome contributions. |
| `no-custom-aggregators` | Author-supplied aggregation functions / plugins on bindings or packs | ADR-0017 §9: no custom aggregators — decision aggregation stays the closed engine path from ADR-0007 as reshaped by §2. |
| `no-obligation-anyof` | Obligation composition via `anyOf` / OR over `require:` | ADR-0017 §9 / §2: obligation composition is AND-only in v1 — no obligation `anyOf`. |
| `no-entry-selector-query` | Generic query language for entry/match selection beyond the four match domains | ADR-0017 §9 / §5: no generic entry-selector query language — match is exactly one of `files` / `values` / `fileEvents` / `valueChanges`. |
| `no-lcd-forge-api` | Lowest-common-denominator forge capability surface in authored config | ADR-0017 §9 (extends ADR-0005): no LCD forge API — adapters expose forge-native behaviour or declare a capability gap; config must not invent a flattened forge API. |

Executable guards: `go test ./schemas/... -run TestDoNotGeneralize`. Removing a schema guard
so one of these fixtures validates is a failing regression, not a silent policy expansion.

> **Match-domain implementation status (E1-S06).** The four match-domain *primitives* shipped in
> `internal/core/classify/matcher.go` are `files` / `values.pointers` / `valueChanges` /
> **`entryEvents`**. `entryEvents` matches collection-*entry* identity churn (a keyed map/list
> entry added/removed/renamed within one file, via the E1-S05 `EntryRef`); it is deliberately a
> distinct domain from ADR-0017 §5's **`fileEvents`**, which denotes ADR-0003's whole-file
> git-detected add/delete/rename. Whole-file `fileEvents` is **not yet implemented** (deferred to a
> fast-follow after E1-S08, which first enumerates the MR's full changed-file set). The frozen §5
> vocabulary above is unchanged; this note records that the shipped primitive set substitutes
> `entryEvents` for `fileEvents` until the latter lands.

## Portability notes (validators)

- **`x-uniqueKeys`**: named-collection uniqueness is enforced by assent's schema compiler
  (`schemas/uniquekeys.go`), not by stock Draft 2020-12 validators. External consumers that
  validate with a stock tool must additionally enforce the unique-ID rules documented on each
  collection, or use assent's compiler.
- **Cross-file `$ref`**: decision schemas share `$def` shapes across files; consumers must
  load them as a bundle (as `schemas` does) rather than resolving `https://assent.dev/…`
  `$id`s from the network today.
