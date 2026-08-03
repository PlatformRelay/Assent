# Marker grammar — four concepts, non-goals, spoofing ignore

**Status**: frozen (P3-E5-S01). **Scope**: this document plus
[`marker-grammar.schema.json`](marker-grammar.schema.json) is the exact, versioned shape of the
hidden-HTML marker payload the Reconcile port (ADR-0017 §7) and the finding-lifecycle state
machine (ADR-0011 amendment 2 `UpsertComment`/`SyncThreads`, ADR-0012 amendment 2, ADR-0016 §1
renderer-owned marker region) read and write on every bot-authored forge comment/thread. It
exists so D-007 ("no database — the report artifact + the forge itself are the storage format")
and D-017 (B6, database-free marker/reconciliation protocol) have something precise to
implement against. Non-goals, authoring ADR-0019, and the numbered reconciliation contract are
out of scope here — see the epic header in
[`openspec/specs/p3-e5-publication-protocol/spec.md`](https://github.com/PlatformRelay/assent/blob/main/openspec/specs/p3-e5-publication-protocol/spec.md).

A marker is an HTML comment (`<!-- assent:marker ... -->`) embedded by the renderer in the
artifact body, outside any user-customizable slot (ADR-0016 §1's envelope invariant). It is
**never parsed back as presentation** — only the Reconcile port reads it, and only to answer
"which slot/occurrence/decision produced this artifact".

## The four concepts

Every marker payload has exactly four top-level properties — `slot`, `occurrence`, `decision`,
`artifact` — enforced by `marker-grammar.schema.json`'s `required` list (no fifth concept, no
optional fifth concept: an unrecognized top-level key is a schema violation, not a forward-compat
escape hatch).

### `slot` — stable identity

`slot` is the tuple that decides whether two runs are talking about **the same** correlation
target. It generalizes ADR-0012 amendment 2's finding-key (`rule id, file, path, value-hash`) to
governed subjects (ADR-0017 §5) and to the newer obligation/anchor vocabulary:

| Field | Source | Required |
| --- | --- | --- |
| `project` | forge-stable project identity | yes |
| `mr` | forge-stable MR/PR IID | yes |
| `rule` | rule id (ADR-0007) | yes |
| `obligation` | named obligation the rule proves (ADR-0017 §2 `prove: {obligation, when}`) | only for obligation-proving rules |
| `entryRef` | governed-subject identity (ADR-0017 §5 `EntryRef`) | only when the finding has one governed subject |
| `effect` | rule effect: `comment` \| `challenge` \| `block` \| `require-review` (ADR-0007) | yes |
| `anchor` | file+line/column span id (ADR-0011 amendment 2 Positions) | only for inline-anchored findings |

Two markers name the same slot iff every field **present** on both is equal. A slot never
inherits identity from a subset match — omission of `obligation`/`entryRef`/`anchor` on both
sides is required, not merely permitted, for that comparison to count.

### `occurrence` — hash of the judged content

`occurrence` is a `sha256:<hex>` digest of the safety-relevant content that was actually judged
for this slot on this run (e.g. the old/new value pair a `challenge` rule evaluated, or the
governed entry's content digest). It exists precisely so that **changed content cannot inherit a
prior resolution** — see the worked example below. The occurrence hash is recomputed from
scratch every run; it is never carried forward or merged with a prior occurrence.

### `decision` — the requesting DecisionRecord

`decision` is the `sha256:<hex>` hash of the DecisionRecord (ADR-0016 §3) whose evaluation
requested this marker's state. It is what the summary comment embeds today (ADR-0012
amendment 2: "the summary comment embeds the decision hash and report-artifact link") and what
lets a human or `assent explain` trace an artifact back to the exact run and report that
produced it. Like `occurrence`, it is correlation metadata: reconciliation reads it to answer
"was this artifact written by the run I'm looking at", never to answer "is this finding safe".

### `artifact` — kind + schema version of the marker itself

| Field | Meaning |
| --- | --- |
| `kind` | `finding-thread` (one resolvable thread per slot) \| `summary-comment` (the one per-MR summary, edited in place, never re-posted — ADR-0012 amendment 2) |
| `schemaVersion` | version of *this marker grammar* (`v1alpha1`), independent of the DecisionRecord/PresentationModel/PublicationReceipt schema versions those artifacts may separately carry |

`artifact` tells the reconciler how to treat the artifact structurally (upsert-one-per-slot vs.
edit-the-single-summary-in-place) without inspecting rendered Markdown, and lets a future
grammar change (`v1beta1`) run side by side with `v1alpha1` markers already posted on open MRs
instead of silently reinterpreting them.

### Worked example marker payload

A `challenge` finding on rule `topic-safety/retention-shrink-challenge`, governed subject
`topic-registry:orders.events.v1`, requested by decision `sha256:926d...` (DecisionRecord),
whose judged content hashes to `sha256:c695...` (occurrence):

```json
{
  "slot": {
    "project": "platform/orders-service",
    "mr": "482",
    "rule": "topic-safety/retention-shrink-challenge",
    "entryRef": "topic-registry:orders.events.v1",
    "effect": "challenge"
  },
  "occurrence": "sha256:c6957a516c95532386bed08f56441dfbb8d18efda24f5abdab1e48437aa3357d",
  "decision": "sha256:926da50737a3908c8a0edfdc1df59a814b192b77e3cee3fd33996d115ce5fc74",
  "artifact": {
    "kind": "finding-thread",
    "schemaVersion": "v1alpha1"
  }
}
```

## Occurrence supersession — changed content cannot inherit a prior resolution

**Adversarial example.** Run 1: an obligation's target value is `retentionMs: 604800000`; the
rule fires `challenge`, occurrence hashes to `sha256:aaa1...`; a reviewer resolves the thread
("yes, shrink to 7 days is fine"). Run 2 (later MR update): the same slot's target value is
edited to `retentionMs: 86400000` (1 day) — the occurrence hash is recomputed over the new
value and comes out as `sha256:bbb2...`, **different** from `sha256:aaa1...`.

Because `occurrence` changed, the resolved state attached to `sha256:aaa1...` **cannot inherit**
to `sha256:bbb2...`: the prior resolution stays scoped to the superseded occurrence only, and the
reconciler must post a **fresh** challenge thread for the new occurrence rather than treating the
slot as already resolved. A resolved "shrink to 7 days — sure?" thread never authorizes a later,
unreviewed shrink to 1 day (this is the same failure ADR-0012 amendment 2 names: "a resolved
[thread] never authorizes a later shrink to Y"). The old thread is left resolved-but-stale, not
deleted — it remains an honest record of what was actually reviewed.

## Non-goals: correlation metadata only

Markers are **correlation metadata only** — they match a forge artifact to the
slot/occurrence/decision that produced it. They are **never decision input or authorization
evidence**. Concretely:

- A rule predicate must never read a marker value as if it were a fact.
- `require-review` eligibility is satisfied only by forge-proven `ApprovalEvidence` (ADR-0017
  §3); a marker's `effect: require-review` correlates an artifact to that requirement, it never
  substitutes for the evidence.
- An obligation proof must never treat a marker's presence, absence, or field value as proof
  input; the marker records that a decision *was requested*, not what the decision *should be*.
- Reconciliation state (resolved/unresolved) is read from the forge's own thread-resolution API,
  never re-derived by parsing marker content (ADR-0012 amendment: "state is never parsed back
  from comment text").

Any code path that branches evaluation, obligation-proof, or `require-review` eligibility on a
parsed marker field is a defect against this contract, full stop — never decision input.

## Spoofing ignore: only bot-authored comments are parsed

**Adversarial fixture.** A contributor (non-bot identity) posts a comment on MR 482 containing a
well-formed marker payload for the existing `topic-safety/retention-shrink-challenge` slot,
claiming (via a hand-crafted marker) that the BLOCK-backing challenge is already resolved:

```json
{
  "slot": {
    "project": "platform/orders-service",
    "mr": "482",
    "rule": "topic-safety/retention-shrink-challenge",
    "entryRef": "topic-registry:orders.events.v1",
    "effect": "challenge"
  },
  "occurrence": "sha256:0453a59198fc6a565d36e39045f3da4f015d554f3756edc361e2e9c11cc84cab",
  "decision": "sha256:323cc513883f071fc65029cf930ec17a267dbc6c44fdf9a0e3a5ec511152baa3",
  "artifact": { "kind": "finding-thread", "schemaVersion": "v1alpha1" }
}
```

**Expected outcome**: this comment is excluded by construction when reconciliation lists
bot-authored artifacts — the exclusion is an **author-identity filter** (comment author id must
equal the configured bot/service-account identity), not a check on marker content or
well-formedness. A syntactically perfect, schema-valid marker from a contributor is treated
identically to no marker at all: it is invisible to the listing step and has **zero effect** on
the computed reconciliation state. Spoofing a marker is exactly as ineffective as spoofing a
`decision: APPROVE` string in a plain-text comment — the reconciler never looks at contributor
comments for markers in the first place, so there is nothing for a spoofed value to override.
This is the explicit expected outcome of the adversarial fixture, not an assumed side effect of
"the bot happens to post first".

## Prohibited marker content

Comment bodies are visible to any project member and subject to CI artifact retention (they are
not a private channel). A marker payload must **never** contain:

- **Secrets** — credentials, tokens, or any provider-sourced sensitive value (ADR-0012 amendment
  2 secret redaction applies to markers too).
- **Fact values** — the actual resolved value of a provider fact (only the rule/effect/subject
  identity that used it belongs in a marker; the value lives in the report artifact only).
- **User-controlled Markdown** — raw diff content, commit messages, or any attacker-influenced
  free text.
- **Raw policy expression** text — CEL/Rego source, `assert` bodies, or any authored predicate
  text.

`marker-grammar.schema.json` makes this structural, not conventional: every property is a
bounded enum (`effect`, `artifact.kind`, `artifact.schemaVersion`), a fixed-shape hash
(`occurrence`, `decision`: `sha256:<64 hex>`), or a length-capped, safe-charset ID string
(`slot.*`: `^[A-Za-z0-9][A-Za-z0-9._:/-]{0,254}$`, `additionalProperties: false` throughout).
There is no field wide enough, or with a permissive enough character set, to carry a secret, a
raw policy expression, or a paragraph of user-controlled Markdown — an attempt to stuff any of
the above into a marker field fails schema validation before it ever reaches a comment body.
