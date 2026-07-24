# Policy profiles — recorder-only inertness (D-017 B3 / P3-E4-S02)

A `PolicyProfile` names a coherent activation of packs for a set of
`(environment, class)` bindings. Exactly one active profile may hold **write
authority** (`spec.writes: true`) for any given binding; every other covering
profile is **recorder-only** (`spec.writes: false`).

## Write vs recorder-only

| `spec.writes` | Role | DecisionRecord | Forge path |
| --- | --- | --- | --- |
| `true` | Writing profile | Carries `profile` identity; findings may feed aggregation / DesiredReviewState | May reach `Reconcile` (ADR-0017 §7) |
| `false` | Recorder-only (counterfactual) | Carries `profile` identity; outcome recorded for comparison only | **Never** calls `Reconcile` — no approve, merge, block, thread sync, or other forge write |

Recorder-only evaluation is an **architectural invariant**, not a runtime best-effort
check: no code path reachable from a `writes: false` profile's evaluation may invoke
the forge `Reconcile` port (or any write adapter method that `Reconcile` would call).
Side-effect-free comparison (`assent compare`, Phase 5+ / E6) evaluates recorder
profiles over the same ChangeSet solely to produce DecisionRecords for delta
classification.

## Contracts

- Profile schema: `schemas/policy/v1alpha1/profile.schema.json` (`spec.writes` required)
- Precedence table on Config: `schemas/policy/v1alpha1/config.schema.json` (`profiles[]`)
- Resolution + worked examples: `docs/planning/policy-lifecycle-profiles.md`
- Lint: `docs/planning/lint-hard-errors.md` → `single-writer-profile`
