# Examples

Living documentation. Rules are written in the **YAML envelope** (ADR-0002 v2/ADR-0013):
`assert` predicates (CEL leaves, string shorthand) for the archetypes, `rego` modules where
logic outgrows tier 1 — the rego example here sketches that escape hatch. Examples that
don't run are lies, so the runnable ones are executed by the gates, not just read.

- [`packs/`](packs/) — complete adopter policy trees (`topic-registry`, `service-catalog`,
  `infra-vars`). Each is a repo root: `assent lint <pack>` is clean and
  `assent test <pack>` passes, both under `task check`. Start here.
  Input formats: yaml, json, tfvars, tf.
- [`policies/declarative/`](policies/declarative/) — standalone envelope rules with
  `assert` predicates
- [`policies/rego/`](policies/rego/) — the tier-2 escape hatch for the same archetype.
  **Illustrative only**: the Rego backend is a deferred tier (E11); the CEL/assert path is
  what ships.
- [`archetypes/`](archetypes/) — one directory per rule archetype from `docs/vision.md`
- [`lint-fixtures/`](lint-fixtures/) — `good`/`bad` pairs pinning each `assent lint`
  hard error in both polarities
- [`repos/`](repos/) — generic sample self-service repo layouts (generated; e2e seeds) and
  the open-source corpus snapshots
- [`comparison/`](comparison/) — `assent compare` suites and promotion-gate fixtures
- [`render/`](render/) — committed finding fixtures for `assent render`
- [`contracts/`](contracts/) — frozen contract fixtures (D-016 strict, named-consumer compat)

`.tf` is governed (never silently un-reviewed change) but does not yet structurally
diff at all — measured, not assumed: the differ only routes the `.tfvars` extension
to the HCL parser, so a `.tf` file's content, blocks or bare literals alike, is
opaque and falls back to REVIEW, never a partial parse (ADR-0003) — see the
`infra-vars` pack's `tf-opaque` case. Only `.tfvars` gets structured diffing today.

The authored surfaces here are the **frozen** `assent.dev/v1alpha1` schemas under
`schemas/`, not drafts; the compatibility promises attached to them are in
[`API_STABILITY.md`](../API_STABILITY.md).
