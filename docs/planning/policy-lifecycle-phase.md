# Policy lifecycle — rollout phase transitions (D-017 B2 / P3-E4-S01)

Rollout is an explicit `phase` field on every MergePolicy rule and every Pack
manifest (`off` | `observe` | `enforce`). There is no default: omission is a
hard lint/schema error (`no-implicit-enforce-phase`). Phase is **not** simulated
by editing `effect`/`onFailure` — that anti-pattern loses policy identity and
breaks before/after comparison.

## Semantics

| Phase | Parse / lint | Evaluate | Record finding | Feeds aggregation (`decision` / `blocks` / `requiredReviews` / `score`) |
| --- | --- | --- | --- | --- |
| `off` | yes | no | no | no |
| `observe` | yes | yes | `findings.observed` | **no** (structurally excluded) |
| `enforce` | yes | yes | `findings.enforcing` | yes |

Pack `spec.phase` is a **ceiling**, never additive:

- pack `off` → no contained rule is evaluated, regardless of the rule's own phase;
- pack `observe` → every contained rule is capped at `observe` even if it declares `enforce`;
- pack `enforce` → each rule's own declared phase stands.

## Phase transitions are visible via structural diff

Two loaded-policy snapshots that differ only in one rule's `phase` value produce
exactly that field change under a generic structural differ — no phase-aware
special case is required. That is the D-017 (B2) "phase transitions are visible
in policy diffs" guarantee: promotion from `observe` → `enforce` (or parking a
rule at `off`) is a plain key/value delta on the authored document.

Worked sketch (generic structural diff; only the changed field is shown):

```diff
 rules[name=shadow-block]:
-  phase: observe
+  phase: enforce
```

The same transition is mirrored on the DecisionRecord side without bespoke
diff logic: a finding that lived under `findings.observed` moves to
`findings.enforcing` (and only then becomes eligible as an aggregation input).
A plain structural diff of the two DecisionRecords surfaces that path change.

## Contracts

- Rules: `schemas/policy/v1alpha1/merge-policy.schema.json` (`rule.phase` required)
- Pack ceiling: `schemas/policy/v1alpha1/pack.schema.json` (`spec.phase` required)
- Findings split: `schemas/decision/v1alpha1/decision-record.schema.json`
  (`findings.observed` / `findings.enforcing`)
- Lint cross-reference: `docs/planning/lint-hard-errors.md` → `no-implicit-enforce-phase`
