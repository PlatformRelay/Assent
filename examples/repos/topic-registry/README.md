# topic-registry — sample self-service repo (YAML, one file per topic)

Fully generic, generated sample (D-002 — nothing here is copied from any private codebase).
It models the most common self-service shape: a **message-topic registry** where each topic
is one YAML file and teams request changes via merge request.

## Governed workflow

1. A team edits (or adds) *their* topic file under `topics/<env>/<topic-name>.yaml`.
2. They open a merge request; CI runs the merge gate against the policy set.
3. Routine changes — a partition bump within quota on a topic the author's team owns —
   prove their obligations and auto-merge. Everything else gets an explained review thread.

Entry semantics: each file is **map-at-root** — the topic-name key is the entry identity
(matching `examples/packs/topic-registry` `mode: map`, `root: ""`). Nested fields carry
`owner` (owning team, resolved against a permission provider), `partitions` (may only grow
within quota), and `retention_hours` (bounded per environment). Deleting a topic file is
always a human-review event.

## Layout

```
topics/
  prod/   # stricter thresholds, no destructive automerge
  dev/    # looser bounds for experimentation
```

Example file shape (map-at-root):

```yaml
orders.events.v1:
  owner: orders-team
  partitions: 12
  replication_factor: 3
  retention_hours: 168
  cleanup_policy: delete
  schema:
    format: avro
    subject: orders.events.v1-value
```

## Rule archetypes exercised (docs/vision.md)

- **Ownership** — `owner` field per entry; author must belong to the owning team.
- **Bounded change** — `partitions` may increase up to quota, never decrease;
  `retention_hours` within a per-env band.
- **No destruction** — removing a topic file requires human review (`match.fileEvents`).
- **Environment split** — `topics/prod/**` vs `topics/dev/**` paths.
- **Schema validity** — every file must keep the topic schema valid.

Starter pack (runnable `.assent/` policy + fixtures): [`examples/packs/topic-registry/`](../../packs/topic-registry/).
