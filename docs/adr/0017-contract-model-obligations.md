# ADR-0017: Contract model — governed subjects, required obligations, typed facts, preconditioned reconciliation

| | |
| --- | --- |
| **Status** | Accepted (P2-E5 / D-016; north-star <1h still PENDING operator timed run — OQ-24) |
| **Date** | 2026-07-21 |
| **Deciders** | Konrad Heimel |
| **Context links** | supersedes/reshapes parts of [0003](0003-canonical-change-model.md) · [0005](0005-forge-abstraction-gitlab-first.md) · [0007](0007-rule-effects-decision-aggregation.md) · [0009](0009-execution-modes.md) · [0010](0010-config-files-repo-layout.md) · [0011](0011-core-ports-and-contracts.md) · [0014](0014-adopter-test-format.md) · [0015](0015-trust-boundaries-merge-integrity.md) · full design roast 2026-07-21 (P1-1..P1-8, P2-1..P2-10) · D-016 |

## Context

A third adversarial review found eight P1 design gaps that would have been frozen into the
Phase-3 contracts. The three most dangerous: decisions pinned to the **source** SHA while the
**target** can move underneath (unevaluated merge results can merge); **one broad vouch**
satisfying every independent safety concern; and resolved `challenge` threads being treated
as **authorization** the forge never proves. The review also showed the interface sketches
conflating four APIs (authored policy, runtime input, replay evidence, presentation/
publication) and the provider protocol being untyped and over-disclosing. This ADR adopts
the review's recommended direction wholesale; details freeze via the contract fixture (§8).

## Decision (proposed)

Each numbered item below closes exactly one P1/P2 finding from the review; together they
replace vouch-only coverage and single-fact "authorization" with a model that states, and can
lint, which specific safety property a change is missing.

### 1. Merge-result validity (P1-1)

Decision preconditions pin **source SHA + target SHA + evaluated merge-result digest**.
Auto-merge requires either a forge merge-train/queue or re-evaluation after any target
movement; `merge?sha=` alone (source-only CAS) is insufficient. L3 conformance case:
advance the target after evaluation → prove rejection or re-evaluation.

### 2. Required obligations replace anonymous vouch coverage (P1-2)

A binding declares the **named proofs** required per governed subject
(`require: [ownership, non-destructive, allowed-fields, …]`). A rule **proves exactly the
obligation it names** (`prove: {obligation, when}`) and declares `onFailure:
{effect, code}`. Auto-merge requires **every** required obligation satisfied for every
subject; denies remain a union. Composition is AND-only in v1 — no obligation `anyOf`.
This replaces `vouch`/`coverage: exclusive` (ADR-0007 §3 and amendment 2): adding or
removing a pack can no longer silently widen safety, and lint can finally say *which*
safety property is missing.

### 3. Challenge is acknowledgement; authorization is `require-review` (P1-3)

`challenge` = resolvable acknowledgement, nothing more; its wording must never claim an
identity requirement. Authorization needs the new **`require-review`** outcome: satisfied
only by forge-proven eligible approval (approval rules / CODEOWNERS evidence, typed
eligible principals). Failed authorization never degrades into an author-resolvable thread;
if the forge cannot prove eligible approval, auto-merge is not armed. (The ownership
example's "owner must resolve" message was the canonical bug.)

### 4. One-shot mode must not arm what it cannot revoke (P1-4)

A one-shot run may arm deferred auto-merge **only** when all controlling inputs stay valid
until a forge-enforced event. Decisions depending on **expiring authorization facts** either
merge immediately, stay REVIEW, or require a service/scheduled reconciler that can revoke.
`facts.max_age` (ADR-0015 §3) is thereby demoted from advisory comment to arming
precondition.

### 5. Governed subjects and matcher domains (P1-5)

Each class defines how content becomes **stable subjects**: `entries: {mode:
document|list|map, root, identity.pointer}` → runtime `EntryRef`
(`catalog-service:payments-api`). Matchers split into explicit domains — `files`,
`values.pointers`, `fileEvents`, `valueChanges` — ending the `path`-as-glob-and-pointer
overload. v1 supports document-per-resource, map-keyed, and list collections **with a
declared identity key**; unkeyed lists are rejected at lint, not guessed.

### 6. Typed, minimized provider protocol (P1-7, P2-9)

Providers declare typed outputs (`type`, `cardinality`, `subject`, `sensitive`, `maxAge`);
requests carry **only declared projections** (full old/new values need an explicit trusted
capability). Responses are versioned envelopes; every fact is
`resolved | unavailable | invalid | expired` with `observedAt`/`expiresAt` and subject
identity — distinct machine states, never a silently absent map key. Controlling facts
never fail open (reaffirms ADR-0004 amendment).

### 7. Declarative publication (P2-1) and contract ownership (P1-6)

The forge boundary becomes `Snapshot → Resolve → Reconcile(DesiredReviewState,
Preconditions) → PublicationReceipt`; preconditions carry source/target/merge digests,
decision hash, fact validity deadline, capabilities. The four-record split
(DecisionRecord / ReplayBundle / PresentationModel / PublicationReceipt, ADR-0016 §3) plus
`EvaluationInput` and authored resources each own exactly one concern; the **serialized
schemas are the public API — the Go interfaces stay internal, no v1 SDK** (P2-10). CEL
semantics are the commitment; only the Go implementation is swappable (tempers ADR-0013's
reversibility claim).

### 8. The contract fixture is the Phase-3 gate

Before any engine code: **one strict, versioned end-to-end fixture** covering a pinned
target/merge result; a renamed entry in a multi-entry document with stable identity; two
independently required obligations; an unavailable/expired typed fact; a missing required
approval; and the expected DecisionRecord, redacted PresentationModel, and publication
preconditions. All examples validate against the same JSON Schemas from then on.

### 9. Compatibility rules & scope guards

Safety-bearing authored resources reject unknown fields, duplicate keys/IDs, unknown enums;
reports are additive-tolerant; provider majors are negotiated. Named collections are lists
with mandatory unique IDs; source order has no meaning without explicit `priority`. Hashes
are over canonical normalized JSON with schema-version domain separation. Do-not-generalize
in v1 (extends D-012): no user-defined effects, custom aggregators, obligation `anyOf`,
generic entry-selector query language, or LCD forge API. `score` is not an effect — points
are rule-outcome contributions with per-binding scores retained (P2-4). Adoption reality
(P1-8): a timed clean-room **secure-setup spike** on one supported GitLab tier gates the
"under one hour" north star; `doctor` emits a typed capability/precondition report. The
success metric gains an independently defined denominator + adjudicated holdout + false-
auto-merge budget (P2-8).

## Consequences

- ADR-0007/0010/0011/0014/0015 are reshaped as stated; ADR-0014's `expect.yaml` moves to
  obligation/predicate/finding-code assertions with **exact** as the recommended opt-in mode for closed-list cases — not the default; an omitted `exact` is must-contain (P2-2,
  already partially amended). Examples migrate to `prove`/`onFailure` when the schemas land
  (P2-5) — until then they carry DRAFT markers.
- e2e build tags are compiled/vetted on every PR; real-forge conformance gates forge-adapter
  changes and releases (P2-6).
- The review formally **refuted** four risks as already-controlled (self-weakening policy,
  rename-fold, template injection, parser exhaustion) — those designs stand.

## Counterpoints considered

- *"Obligations are more ceremony than vouch."* — Yes: one `require:` list per binding. That
  ceremony is the safety composition — it's what makes pack changes local and lintable. The
  roast's failure scenario (broad ownership vouch auto-merges an unknown dangerous field) is
  otherwise unfixable by lint.
- *"Merge-result pinning may be impossible on some forge tiers."* — Then that tier doesn't
  get deferred auto-merge (capability gap, fail closed) — same principle as ADR-0015 §2.
