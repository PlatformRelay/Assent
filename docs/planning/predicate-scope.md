# Predicate scope — the frozen `assert`/`cel` field table (ADR-0013 consequences)

This table is the **closed set** of top-level fields a CEL leaf inside `assert`/`when`
(ADR-0013) may reference. It exists so `MergePolicy`'s `assert`/`cel` leaves have one
authoritative field list, referenced by name from
[`schemas/policy/v1alpha1/merge-policy.schema.json`](https://github.com/PlatformRelay/assent/blob/main/schemas/policy/v1alpha1/merge-policy.schema.json),
that a future `assent lint` implementation checks unknown-field references against
(ADR-0016 §2: unknown fields are load-time errors, never `<no value>`). Adding a field here is
a schema-fixture-gated change, not a drive-by edit — REQ-P3-E1-S06-01 lists "undeclared
predicate-scope fields" as a lint hard error precisely to keep this table authoritative.

| Field | Meaning | Source type | Scope |
| --- | --- | --- | --- |
| `old` | Pre-change value at the matched pointer | value (same shape as the pointer's target) | change |
| `new` | Post-change value at the matched pointer | value (same shape as the pointer's target) | change |
| `path` | The matched pointer/path itself | string | change |
| `kind` | The matched change's lifecycle kind | `add \| modify \| delete \| rename` | change |
| `file` | Repo-relative path of the file the matched change lives in | string | change |
| `entry` | Head-state value tree of the containing `EntryRef` | value tree (shape of the governed entry) | change |
| `oldEntry` | Base-state value tree of the containing `EntryRef` (absent for `add`) | value tree, nullable | change |
| `changes` | All changes in this class slice (the whole matched set, not just the current one) | array of `Change` | change |
| `facts` | Provider results, keyed by provider name then output name (ADR-0004) | map of typed fact values | change + mr |
| `mr` | MR/PR metadata: author, branches, labels | object | change + mr |
| `env` | Environment label from the classifier (ADR-0008) | string | change + mr |

Naming precedent: Kubernetes `ValidatingAdmissionPolicy`'s `object`/`oldObject` (`entry`/
`oldEntry` here play the same role, scoped to one governed `EntryRef` rather than the whole
admission object).

## Rules

- This is the **entire** set. No other top-level identifier may appear in a CEL leaf; a
  reference to anything else is an `assent lint` hard error (docs/planning/lint-hard-errors.md),
  never a silent `<no value>` or a runtime CEL "no such attribute" surprise.
- `facts.<provider>.<output>` and `mr.<field>` are the only two fields with a further-nested,
  provider/forge-defined shape; every other field's shape is fixed by the schemas in this epic
  (`EntryRef`, `Change`, the four matcher-domain shapes).
- The ordering operators `<`, `<=`, `>`, `>=` compare **numbers**, not text. If an operand
  actually evaluates to a string — a YAML `!!str` such as `partitions: "12"` stays a string by
  design — the leaf is an evaluation error (→ `predicate.error` → REVIEW), never a lexical
  answer (ADR-0013 Amendment 1, D-129). Order quoted numerics with `int(new) >= int(old)` or
  `double(...)`, and dates with `timestamp(a) < timestamp(b)`. Equality, `in`, and the string
  functions are unaffected.
- Adding a field to this table requires a schema-fixture change (a new positive fixture that
  exercises it) — this table and `merge-policy.schema.json`'s `assert`/`cel` `description` stay
  in lockstep by construction, not by convention.

## Source

Carried over verbatim in substance from
[ADR-0013 appendix, "Predicate scope"](../adr/0013-appendix-syntax-gallery.md#predicate-scope-variables-visible-to-assert),
frozen here as the companion contract ADR-0013's consequences promised ("Predicate scope becomes
contract... Freezes with fixtures in Phase 3").
