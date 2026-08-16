# Archetype golden corpus — `assent test` seed manifest

**Manifest version:** `2`  
**Owned by:** P3-E3-S04  
**Corpus root:** [`examples/archetypes/`](https://github.com/PlatformRelay/assent/tree/main/examples/archetypes)
**Inventory:** [`archetypes.md`](archetypes.md)

This is the **named, versioned seed corpus** the future Phase-5 `assent test` golden-run
contract must satisfy. Adding or renaming an archetype fixture is a reviewable diff against
this manifest — the runner iterates *this list*, not an invented directory glob.

Each row is a directory that carries a `base` / `head` / `facts.yaml` / `expected.yaml`
quadruple (or a `negative/` / case-subdir variant of that shape). `decision` is the
`DecisionRecord.decision` the golden must produce.

| Path | Expected `decision` | Notes |
| --- | --- | --- |
| `examples/archetypes/allow-listed-fields/` | `APPROVE` | Satisfied `allowed-fields` prove; empty findings. |
| `examples/archetypes/allow-listed-fields/negative/` | `REVIEW` | Sensitive field change → `require-review`. |
| `examples/archetypes/assent-policy/` | `BLOCK` | Source-branch policy edit → `policy-integrity` block. |
| `examples/archetypes/assent-policy/negative/` | `BLOCK` | Mixed routine + policy edit; meta-class wins. |
| `examples/archetypes/bounded-change/` | `APPROVE` | In-band partition bump; empty findings. |
| `examples/archetypes/bounded-change/negative/` | `REVIEW` | Over-quota → `challenge` / `bounded-change.out-of-band`. |
| `examples/archetypes/environment-split/` | `APPROVE` | Dev binding; in-band under loose quota. |
| `examples/archetypes/environment-split/negative/` | `REVIEW` | Prod binding; same bump exceeds prod quota. |
| `examples/archetypes/freshness/` | `APPROVE` | Context fact fresh within `maxAge`. |
| `examples/archetypes/freshness/negative/` | `REVIEW` | Expired controlling context fact. |
| `examples/archetypes/ownership/` | `APPROVE` | Author in owning group; empty findings. |
| `examples/archetypes/ownership/negative/` | `REVIEW` | Unauthorized author → `require-review` (not self-resolve). |
| `examples/archetypes/schema-validity/` | `APPROVE` | Document validates; empty findings. |
| `examples/archetypes/schema-validity/negative/` | `BLOCK` | Missing required property → `schema.invalid`. |
| `examples/archetypes/no-destruction/delete/` | `BLOCK` | File delete; never author-thread authorization. |
| `examples/archetypes/no-destruction/rename/` | `REVIEW` | Rename ≥ delete strictness; `require-review`. |
| `examples/archetypes/no-destruction/near-similarity/` | `BLOCK` | Near-similarity must not downgrade below delete. |
| `examples/archetypes/quota-ceiling/` | `APPROVE` | REF-EX C5 — in-band partition bump against a stubbed quota fact; empty findings. |
| `examples/archetypes/quota-ceiling/negative/` | `REVIEW` | Over-quota → `challenge` / `bounded-change.out-of-band` (same mechanism as `bounded-change/negative`, named for C5 discoverability). |
| `examples/archetypes/placement-allow-list/` | `APPROVE` | REF-EX C6 — `instance_set` change onto an allow-listed value (stubbed `facts.placement.allowed.value`); empty findings. |
| `examples/archetypes/placement-allow-list/negative/` | `REVIEW` | `instance_set` off the allow-list → `require-review` / `placement.disallowed-instance-set`. |
| `examples/archetypes/referenced-resource-ownership/` | `APPROVE` | REF-EX C7 (gap demo, judgment call (b)) — `acl.resource` names a resource the stubbed `facts.resource_owner.owner.value` confirms the author's group owns; empty findings. |
| `examples/archetypes/referenced-resource-ownership/negative/` | `REVIEW` | Resolved owner does not match the author's groups → `require-review` / `acl.resource-ownership-mismatch`. |

**Index (not a golden quadruple):** `examples/archetypes/no-destruction/expected.yaml`
lists the three case directories above and reaffirms `onFailure.effect: require-review`
(never challenge-as-authorization). It is not itself an `assent test` case.

**Excluded:** `examples/policies/rego/**` — quarantined (`# locked: D-012`); not part of this
corpus until E11's post-Phase-4 implementation lane removes the marker
([rego-escape-hatch.md](rego-escape-hatch.md)).

## Versioning

Bump **Manifest version** whenever a row is added, removed, or its expected `decision`
changes. Phase-5 `assent test` should pin against a named version of this file so corpus
drift is an explicit contract change.
