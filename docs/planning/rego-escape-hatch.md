# Rego escape-hatch quarantine (`# locked: D-012`)

`examples/policies/rego/**` is the contract-side escape hatch for a Rego predicate
backend inside the YAML envelope (ADR-0002 v2): the envelope owns match / effect /
points; a Rego module only computes violations over `PolicyInput`. The example is
**committed so the contract shape is reviewable**, but it is **not a v1-runnable
predicate backend**.

## Marker meaning

Every file under `examples/policies/rego/**` must carry this token on its first line
(or on or before its first non-comment line):

```text
# locked: D-012 — …
```

The literal `# locked: D-012` token is kept for grep consistency with the epic brief
(P3-E3-S03). It does **not** mean the Rego *contract* is still D-012-locked. E11's
actual status is **Unlocked (D-017), implementation after Phase 4** — a milder gate
than E10/E13's real `Locked (D-012)`. Accompanying prose on the marker must state
both facts (contract unlocked under D-017/E11; implementation gated until the
Phase-4 adoption gate) so a reader cannot misread the token as "the contract itself
is still locked."

Until the marker is removed:

- Do not wire `examples/policies/rego/**` into `assent test`, the schema-validation
  CI job, or any starter pack / archetype golden corpus.
- Declarative examples, archetype fixtures, and starter packs must not reference a
  `rego:` predicate leaf (structurally excluded from P3-E3-S01 / S02).

## Who may remove it

Only **E11's own implementation lane**, after the Phase-4 adoption gate, may remove
the marker and wire Rego into the engine / CI / golden corpus. No other lane
(including P3-E3 migration work) may treat removal as in-scope.

## Structural enforcement (owned by P3-E3-S04)

REQ-P3-E3-S03-02's CI guard — assert every `examples/policies/rego/**` file carries
the marker, and that no file under `examples/policies/declarative/**`,
`examples/archetypes/**`, or `examples/packs/**` references a `rego:` leaf — is
**owned by P3-E3-S04** (`hack/check-migration-invariants.sh` / the migration-
invariant CI step). This document records the convention; S04 implements the guard.
Do not add that script from this story.
