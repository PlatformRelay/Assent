# Rule-archetype inventory (P1-E2-S01)

Phase-1 gate inventory of every rule class the vision archetypes, sample-repo shapes, and
corpus require. Each archetype proves a **named obligation** (ADR-0017 §2), matches on an
explicit **domain** (ADR-0017 §5), and names a governed **subject** (`EntryRef`). Examples
live under [`examples/archetypes/`](https://github.com/PlatformRelay/assent/tree/main/examples/archetypes).

**Effect vocabulary** (ADR-0007 as reshaped by ADR-0017 §3):

| Outcome | Meaning |
| --- | --- |
| `require-review` | Authorization — satisfied only by forge-proven eligible approval (P1-E3-S02 / OQ-23 ApprovalEvidence). **Not** an author-resolvable thread. |
| `challenge` | Acknowledgement only — wording must never claim an identity requirement. |
| `block` | Hard deny; no auto-merge path. |
| `vouch` / `prove` | Satisfies the named obligation when the predicate holds. |

Authorization archetypes below cite **forge evidence needed** (link target: P1-E3-S02 dossier)
and document the adversarial case that the author self-resolving a discussion thread must
**not** satisfy the obligation.

---

## ownership

Author may only modify entries whose `owner` (group/team) they belong to. Membership comes
from a permission provider (Keycloak, LDAP, forge groups, ownership file) — the rule does
not care which.

| Field | Value |
| --- | --- |
| **Inputs** | Change paths under the governed class; `entry.owner`; `facts.author.groups` (or equivalent membership fact); MR author identity. |
| **Subject (EntryRef)** | Map/list entry identity, e.g. `topic-registry:orders.events.v1`, `catalog-service:orders-api`. |
| **Matcher domain** | `files` (document-per-resource) or `valueChanges` within a multi-entry document whose identity pointer is declared. |
| **Obligation proved** | `ownership` — author is a member of `entry.owner`. |
| **onFailure effect+code** | `require-review` / `ownership.unauthorized` — **not** `challenge`. |
| **Failure mode (tri-state)** | Predicate **false** → `require-review`. Fact **error**/unavailable → fail-safe `require-review` (never silent pass; ADR-0007 tri-state). Predicate **true** → obligation satisfied. |
| **Forge outcome** | REVIEW until ApprovalEvidence shows an eligible principal (CODEOWNERS / approval-rule member for the owning group — not merely the MR author) approved. Auto-merge never armed on self-resolution of a bot thread. |

**Forge evidence needed (P1-E3-S02):** typed `ApprovalEvidence` — principal, source/rule,
eligibility proof, pins, expiry; forge capability to verify eligible approval. If the forge
cannot prove it → stay REVIEW / no auto-merge (never degrade to `challenge`).

**Adversarial case:** author opens MR touching another team's entry, bot posts a thread, author
self-resolves / self-approves. That must **not** satisfy `ownership`. Only forge-proven
eligible-identity approval does.

**Sample shape:** `examples/repos/topic-registry/` (owner field), corpus `kubernetes-org`
membership edits.

---

## bounded-change

Numeric (or similarly ordered) fields may change only within a declared band — e.g.
`partitions` may increase up to a quota, never decrease.

| Field | Value |
| --- | --- |
| **Inputs** | Matched value change (`old`/`new`); band/quota facts (e.g. `facts.quota.max_partitions`); optional env from classifier. |
| **Subject (EntryRef)** | Containing entry, e.g. `topic-registry:orders.events.v1`. |
| **Matcher domain** | `values.pointers` (e.g. `/partitions`) or `valueChanges` filtered by pointer. |
| **Obligation proved** | `bounded-change` (alias: within-band). |
| **onFailure effect+code** | `challenge` / `bounded-change.out-of-band` (acknowledgement that the bump is intentional) **or** `block` / `bounded-change.decrease-forbidden` when the predicate encodes a hard physical constraint (Kafka partition decrease). Packs choose; default starter: `challenge` for over-quota, `block` for decrease. |
| **Failure mode (tri-state)** | **false** → `onFailure`. **error** (missing quota fact) → effect fires fail-safe (challenge/block), never vouch. **true** → prove. |
| **Forge outcome** | On challenge: REVIEW until thread resolved + re-eval. On block: BLOCK. On prove + other obligations: path to APPROVE. |

**Sample shape:** topic-registry `partitions` / `retention_hours`; infra-vars `max_replicas` /
`memory_mb`; JulieOps `num_partitions`.

---

## allow-listed-fields

Only a named set of fields may change for automerge; anything else requires human attention.

| Field | Value |
| --- | --- |
| **Inputs** | Per-change paths in the ChangeSet; allow-list declared on the binding/class. |
| **Subject (EntryRef)** | Containing entry (each changed pointer attributed to its entry). |
| **Matcher domain** | `values.pointers` (per-pointer allow check) and/or `valueChanges`. |
| **Obligation proved** | `allowed-fields`. |
| **onFailure effect+code** | `challenge` / `allowed-fields.unlisted` for unexpected but non-destructive fields; packs may escalate sensitive pointers (e.g. `/owner`) to `require-review` / `allowed-fields.sensitive`. |
| **Failure mode (tri-state)** | **false** → onFailure. Path parse **error**/opaque → fail-safe REVIEW (no prove). **true** for every change under the subject → prove. |
| **Forge outcome** | Unlisted field → REVIEW (challenge resolved ≠ authorization for sensitive fields). Full allow-list coverage → obligation satisfied. |

**Sample shape:** topic-registry allow `/partitions`, `/retention_hours`, `/description`;
catalog ownership/`tier` edits out of allow-list → review.

---

## no-destruction

Deletions of whole entries/files — and renames of the same class — always require human
authorization. Per ADR-0010 amendment / ADR-0003 amendment: **rename is never treated laxer
than delete** of the same class. Fold-to-rename is opt-in (`classes[].renames: detect|raw`,
default `raw`).

| Field | Value |
| --- | --- |
| **Inputs** | File/entry delete and rename events; optional similarity metadata when fold is enabled; class delete/rename effect binding. |
| **Subject (EntryRef)** | Deleted or renamed entry/file, e.g. `topic-registry:payments.transactions.v1`. |
| **Matcher domain** | `fileEvents` (`deleted` \| `renamed`) and entry-level delete/rename in `valueChanges` / structural events. |
| **Obligation proved** | `non-destructive` — subject is not being destroyed or identity-moved without authorization. |
| **onFailure effect+code** | `require-review` / `destruction.forbidden` (or `block` / `destruction.blocked` for packs that never auto-merge deletes). **Not** `challenge`. |
| **Failure mode (tri-state)** | Any matched delete/rename → obligation **fails** into require-review/block. Ambiguous near-threshold fold → treat as delete+add (stricter). Engine must apply **max(strictness(delete), strictness(rename))**. |
| **Forge outcome** | REVIEW/BLOCK until eligible approval (or permanent block). Auto-merge never armed from author thread resolution alone. |

**Forge evidence needed (P1-E3-S02):** same ApprovalEvidence contract as ownership — eligible
reviewer/approver per forge rules for destructive classes (often platform CODEOWNERS).

**Adversarial case:** (1) author deletes a topic, resolves the bot's "are you sure?" thread
themselves — must **not** satisfy `non-destructive`. (2) Near-similarity delete+add crafted
to sit just above a fold threshold to downgrade `block` delete into soft rename —
engine must not treat rename laxer than delete; golden examples under
`examples/archetypes/no-destruction/`.

**Cases required:** delete, rename, adversarial near-similarity delete+add — all expected
REVIEW or BLOCK (never APPROVE).

**Sample shape:** topic-registry file delete; catalog entry removal; octoDNS record deletion.

---

## environment-split

Changes touching production paths need stricter bands/thresholds/packs than non-prod
(ADR-0008 bindings). Prefer routing via bindings; in-rule env conditionals are secondary.

| Field | Value |
| --- | --- |
| **Inputs** | Classifier `env` (from path/`bindings.yaml`); env-specific thresholds, packs, and quotas. |
| **Subject (EntryRef)** | Same entry identity; binding selects which obligation thresholds apply, e.g. prod vs dev `topic-registry:orders.events.v1`. |
| **Matcher domain** | `files` (path glob `prod/**` vs `dev/**`) plus whatever domain the routed pack uses (`values.pointers`, etc.). |
| **Obligation proved** | Environment-qualified forms of other obligations (e.g. `bounded-change` under prod threshold). The split itself is **routing**, not a separate proof — inventory tracks it because packs/bindings must declare distinct require-lists / scores per env. |
| **onFailure effect+code** | Inherited from the routed pack (typically stricter: lower quota → `challenge`/`block`; higher risk threshold → REVIEW). Code prefix `env.<name>.…`. |
| **Failure mode (tri-state)** | Missing env classification → fail-safe REVIEW (unrouted / uncovered). Wrong-pack match must not vouch under a laxer env. |
| **Forge outcome** | Prod over-band → REVIEW/BLOCK even when the identical numeric delta would APPROVE in dev. |

**Sample shape:** `topics/prod/` vs `topics/dev/`; `catalog/prod/` vs `catalog/dev/`;
`envs/prod/` vs `envs/dev/` tfvars.

---

## schema-validity

The changed file/entry must still validate against the repo's declared schema (JSON Schema,
OpenAPI fragment, or format-specific validator).

| Field | Value |
| --- | --- |
| **Inputs** | Head (and optionally base) document bytes; schema fact or bundled schema ref; parser diagnostics. |
| **Subject (EntryRef)** | File or entry under validation, e.g. `catalog-service:orders-api` or file subject `file:catalog/prod/core-services.json`. |
| **Matcher domain** | `files` (re-validate whole document on any touch) or `valueChanges` when partial validation is defined. |
| **Obligation proved** | `schema-valid`. |
| **onFailure effect+code** | `block` / `schema.invalid` — invalid config must not merge. Opaque/unparseable → same fail-safe block or REVIEW per pack (default **block** for governed formats). |
| **Failure mode (tri-state)** | Validation **false** → block. Schema fact **unavailable** → fail-safe block/REVIEW (never prove). **true** → prove. |
| **Forge outcome** | BLOCK (or REVIEW if pack chooses soft mode); never APPROVE on invalid head. |

**Sample shape:** service-catalog JSON; topic-registry YAML required fields; Prow-validated
kubernetes/org config in corpus.

---

## freshness / context-fact

Referenced external context (cost center, on-call rotation, quota object, usage signal) must
exist and be fresh — resolved by a fact-provider plugin (ADR-0004 / ADR-0017 §6).

| Field | Value |
| --- | --- |
| **Inputs** | Typed fact query projections; `resolved \| unavailable \| invalid \| expired` with `observedAt`/`expiresAt`; `facts.maxAge` / per-fact `maxAge` as **arming** precondition (ADR-0017 §4). |
| **Subject (EntryRef)** | Entry that references the external id, e.g. `catalog-service:orders-api` → `oncall: orders-rotation`. |
| **Matcher domain** | `valueChanges` / `values.pointers` on the referencing field; fact subject may be a separate typed subject. |
| **Obligation proved** | `context-fresh` (alias: `freshness`). |
| **onFailure effect+code** | `challenge` / `context.missing` when the referent is absent but human may confirm; `block` / `context.expired` when controlling authz facts expired and one-shot cannot revoke (ADR-0017 §4) → do not arm auto-merge. |
| **Failure mode (tri-state)** | `unavailable`/`invalid`/`expired` → **not** prove; fail closed per effect. Never treat absent map key as empty-success. |
| **Forge outcome** | REVIEW/BLOCK; deferred auto-merge only when fact validity deadline still holds at reconcile time. |

**Sample shape:** catalog `oncall` rotation existence; topic schema subject registration;
infra cost-center fact.

---

## assent-policy (meta-class)

Any MR that touches `.assent/**` (config, bindings, packs, Rego, templates, test expectations)
routes to the built-in meta-class **`assent-policy`** (ADR-0015 §1). Policy is loaded from the
**target/base ref**, never the MR branch. Policy changes are `block`-by-default and can never
be vouched by the policies they modify. Repos may relax only to `challenge`, never to `vouch`.

| Field | Value |
| --- | --- |
| **Inputs** | File events under `.assent/**`; target-ref policy pin; classifier meta-class. |
| **Subject (EntryRef)** | Policy artifact paths as file subjects, e.g. `file:.assent/config.yaml`, `file:.assent/packs/ownership.yaml`. |
| **Matcher domain** | `files` / `fileEvents` on `.assent/**`. |
| **Obligation proved** | None may be auto-proved by branch policy — human review is mandatory. Implicit obligation: `policy-integrity` (human-gated). |
| **onFailure effect+code** | Default `block` / `assent-policy.changed`. Optional repo relax: `challenge` / `assent-policy.ack` only — **never** `vouch`, **never** `require-review` satisfied by the policy author alone without forge eligible approval if the repo also adds CODEOWNERS (recommended hardening ADR-0015 §5). |
| **Failure mode (tri-state)** | Any `.assent/**` touch → meta-class fires. Error loading target policy → fail closed (no auto-merge). |
| **Forge outcome** | BLOCK (default) or REVIEW if relaxed to challenge. Golden e2e: *"MR edits its own policy → BLOCK."* |

**Adversarial case:** MR weakens ownership pack on the source branch and expects the weakened
pack to vouch the same MR — impossible because gating loads target-ref policy; meta-class
still blocks/challenges the `.assent/**` touch.

**Sample shape:** any self-service repo adopting assent; mandatory case in examples.

---

## Coverage matrix (vision → inventory)

| Vision archetype | Inventory section | Primary sample |
| --- | --- | --- |
| Ownership | ownership | topic-registry, kubernetes-org |
| Bounded change | bounded-change | topic-registry, infra-vars, julieops |
| Allow-listed fields | allow-listed-fields | topic-registry, service-catalog |
| No destruction | no-destruction | topic-registry, octodns |
| Environment split | environment-split | prod/ vs dev/ trees |
| Schema validity | schema-validity | service-catalog JSON |
| Freshness/context facts | freshness | catalog oncall fact |
| (trust) Policy self-edit | assent-policy | `.assent/**` |
