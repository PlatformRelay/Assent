# infra-vars — sample self-service repo (tfvars, per-env variable files)

Fully generic, generated sample (D-002 — nothing here is copied from any private codebase).
It models the third common shape: **Terraform variable files per environment**, where teams
tune the sizing of their own workloads via merge request and a platform pipeline applies
the result.

## Governed workflow

1. A team edits the entry for its workload in `envs/<env>/*.tfvars` — e.g. raising
   `memory_mb` or `max_replicas` inside the approved band.
2. They open a merge request; CI runs the merge gate against the policy set.
3. Sizing changes within band on entries the author's team owns prove their obligations
   and auto-merge. New workloads, band-exceeding values, and removals get a human.

Entry semantics: each workload is a keyed object with an `owner` attribute; numeric
attributes carry per-environment bands (prod bands are tighter); anything the HCL parser
cannot classify falls back to opaque-change handling — which never auto-merges.

## Layout

```
envs/
  prod/compute.tfvars   # workload sizing, tight bands
  prod/network.tfvars   # allow-listed CIDR/port entries
  dev/compute.tfvars    # same shape, looser bands
  dev/network.tfvars
```

## Rule archetypes exercised (docs/vision.md)

- **Environment split** — `envs/prod/**` vs `envs/dev/**` paths.
- **Bounded change** — `memory_mb`, `min_replicas`/`max_replicas` within per-env bands.
- **Ownership** — `owner` attribute per workload entry.
- **Opaque-change fallback** — HCL constructs outside the modeled shape never automerge.
- **No destruction** — removing a workload entry requires human review.

Starter pack (runnable `.assent/` policy + fixtures): [`examples/packs/infra-vars/`](../../packs/infra-vars/).
