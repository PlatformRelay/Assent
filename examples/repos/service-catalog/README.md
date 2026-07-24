# service-catalog — sample self-service repo (JSON, multi-entry files)

Fully generic, generated sample (D-002 — nothing here is copied from any private codebase).
It models the second common shape: a **service catalog** where one JSON file holds many
entries, so a single merge request routinely touches several entries at once.

## Governed workflow

1. A team edits its entries in `catalog/<env>/*.json` — registering a new service,
   changing its tier, or rotating the on-call rotation reference.
2. They open a merge request; CI runs the merge gate against the policy set.
3. Changes limited to the **allow-listed fields** (`oncall`, `endpoints`, `tags`) on
   entries the author's team owns prove their obligations and auto-merge. Tier changes,
   ownership transfers, and entry deletions always get a human.

Entry semantics: `owner` names the owning team; `tier` (1–3) drives alerting and is *not*
self-service; `oncall` must reference a rotation that exists (fact-provider lookup).

## Layout

```
catalog/
  prod/core-services.json   # customer-facing entries, tier changes blocked
  prod/data-services.json   # data-plane entries
  dev/services.json         # single flat file, looser rules
```

## Rule archetypes exercised (docs/vision.md)

- **Allow-listed fields** — only `oncall`, `endpoints`, `tags` may change for automerge.
- **Ownership** — `owner` field per entry; multi-entry diffs must be owned entirely.
- **Schema validity** — files must stay valid against the catalog schema.
- **No destruction** — removing an entry from a file requires human review.
- **Environment split** — `catalog/prod/**` vs `catalog/dev/**`.
- **Freshness/context facts** — `oncall` must resolve in the rotation system.

Starter pack (runnable `.assent/` policy + fixtures): [`examples/packs/service-catalog/`](../../packs/service-catalog/).
