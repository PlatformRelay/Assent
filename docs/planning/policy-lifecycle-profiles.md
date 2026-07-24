# Policy lifecycle — named profiles + precedence table (D-017 B3 / P3-E4-S02)

Named `PolicyProfile` resources activate coherent pack sets for comparison and
rollout. They compose with the existing ADR-0008 / ADR-0010 routing model
(`RulesetBinding` still binds `(class, environment) → packs`); profiles are
**not** a second `match`/routing block.

The schema-level precedence artifact lives on `Config.profiles` — an ordered list
of `{name}` refs to `PolicyProfile` documents. Write authority is the required
boolean `spec.writes` on each profile.

## Precedence table

For a binding `B = (environment, class)`:

| Step | Rule |
| --- | --- |
| 1. Active set | Only profiles named in `Config.profiles` participate. |
| 2. Coverage | A profile covers `B` when its `spec.environments` contains `environment` or `*`, **and** its `spec.classes` contains `class` or `*`. |
| 3. Specificity | Narrower wins: score = (1 if environments is not solely `*`) + (1 if classes is not solely `*`). Higher score wins. |
| 4. Tie-break | Equal specificity → earlier entry in `Config.profiles` wins. |
| 5. Single-writer | Among **all** covering profiles (not only the precedence winner), `count(writes: true)` must be exactly **1**. Zero or more than one → lint hard error `single-writer-profile` (never last-one-wins). |

The precedence winner is the profile whose pack activation is primary for `B`.
Recorder-only profiles that also cover `B` remain evaluable for counterfactual
comparison; they never hold write authority.

## Worked example — narrower wins

Profiles:

| Profile | `writes` | `environments` | `classes` |
| --- | --- | --- | --- |
| `default-writer` | `true` | `["*"]` | `["*"]` |
| `prod-strict` | `true` | `["prod"]` | `["kafka-topic"]` |
| `candidate-shadow` | `false` | `["*"]` | `["*"]` |

`Config.profiles` order: `prod-strict`, `default-writer`, `candidate-shadow`.

Binding `(prod, kafka-topic)`:

- All three cover the binding.
- Specificity: `prod-strict` = 2, others = 0 → **`prod-strict` wins** (narrower over broader).
- Single-writer check: **fails** — both `default-writer` and `prod-strict` have
  `writes: true` with overlapping scope. Lint hard-errors; never silently pick one.

Corrected authoring (single writer for the nested scope): keep `prod-strict`
`writes: true` and set `default-writer` to `writes: false` **or** narrow
`default-writer` so it no longer covers `prod`/`kafka-topic` (e.g. `environments: ["dev"]`).
With `default-writer` as recorder-only (or out of scope), `(prod, kafka-topic)` has
exactly one writer (`prod-strict`); `candidate-shadow` remains recorder-only.

## Worked example — disjoint scopes

| Profile | `writes` | `environments` | `classes` |
| --- | --- | --- | --- |
| `prod-writer` | `true` | `["prod"]` | `["*"]` |
| `dev-writer` | `true` | `["dev"]` | `["*"]` |
| `prod-candidate` | `false` | `["prod"]` | `["kafka-topic"]` |

Bindings:

- `(prod, kafka-topic)` — covered by `prod-writer` + `prod-candidate`; exactly one
  writer (`prod-writer`); `prod-candidate` is recorder-only. Both profiles active;
  narrower recorder does not steal writes.
- `(dev, infra-vars)` — covered only by `dev-writer`; exactly one writer.
- Scopes are disjoint for write authority: both writers may coexist because no
  single binding is covered by two `writes: true` profiles.

## Contracts

- `schemas/policy/v1alpha1/profile.schema.json` — required `spec.writes`
- `schemas/policy/v1alpha1/config.schema.json` — `profiles[]` precedence artifact
- Architecture invariant: `docs/architecture/policy-profiles.md` (recorder-only /
  never `Reconcile`)
- Lint: `docs/planning/lint-hard-errors.md` → `single-writer-profile`
