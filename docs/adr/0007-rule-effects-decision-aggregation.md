# ADR-0007: Rule effects and decision aggregation (incl. risk points)

| | |
| --- | --- |
| **Status** | Accepted (partial: vouch/coverage superseded by ADR-0017 §2; challenge authz by §3; P2-E5) |
| **Date** | 2026-07-21 |
| **Deciders** | Konrad Heimel |
| **Context links** | [ADR-0002 policy surface](0002-policy-frontends-rego-declarative.md) · [ADR-0008 routing](0008-change-classification-routing-scope.md) · [ADR-0005 forge](0005-forge-abstraction-gitlab-first.md) |

## Context

"A rule fired" is not one thing. In practice a rule's outcome is one of: an informational
comment; a resolvable "are you sure?" thread that must be acknowledged before merge; a hard
block (with optional explanation); or a positive signal that a change is safe to auto-approve.
Severity levels don't capture this — these are **effects** with different forge actions and
different aggregation semantics. Additionally, a scalar **risk score** is wanted so that many
small oddities can add up to "human, please look" even when no single rule blocks.

A predicate is not strictly binary either — a missing fact, a type mismatch, or a cost-limit
hit must still resolve to *something*, never a silent pass. And a bulk change authored as many
small edits must not slip under a per-rule threshold just because each edit fires its own rule
once. The effect table, aggregation order, and scoring rule below hold both properties
together.

## Decision (proposed)

### Effects (per rule, declared in the envelope)

| Effect | Meaning | Forge action | Blocks merge? |
| --- | --- | --- | --- |
| `comment` | informational finding | plain MR/PR comment (batched) | no |
| `challenge` | "are you sure?" | **resolvable thread**; merge waits for resolution | until resolved |
| `block` | hard deny | thread/comment (optional custom message) + deny | yes |
| `vouch` | this change is understood and safe | none (recorded in report) | enables automerge |
| `score` | contribute risk points (`points: N`) | none (recorded) | via threshold |

A rule = match + predicate + **one effect** (plus optional `points`, allowed alongside any
effect, accruing **per firing** rather than per rule — see amendment 2 for the bulk-change
arithmetic this implies) and an optional `onFail:` block (`effect`, `message`, `points`)
applied when that same predicate returns **false**. One predicate serving both outcomes avoids
hand-negated twin rules that drift apart; see amendment 2 for the worked example. Findings
carry rule id, effect, paths, message, points.

### Aggregation (deterministic, order-independent)

1. Any `block` finding → **BLOCK**.
2. Else any unresolved `challenge` → **REVIEW** (threads posted; on the forge the MR merges
   only after all threads are resolved *and* re-evaluation passes).
3. Else **coverage check**: every entry in the ChangeSet must be matched by ≥1 `vouch` rule
   from the routed pack set — packs may combine (union of denies, single-vouch trust; see
   amendment 1 for the multi-pack semantics and the `coverage: exclusive` escape). Unvouched
   changes → **REVIEW** with an explicit "uncovered change" finding. Fail-safe by construction:
   an empty or non-matching policy set never automerges anything.
4. Else **risk check**: `sum(points)` ≤ threshold for the active (environment, change class)
   binding (ADR-0008) → **APPROVE** (+ merge); over threshold → **REVIEW**.

`comment` findings are emitted in every outcome. The JSON report always contains the full
finding list, per-rule traces, score arithmetic, and the aggregation path taken.

## Consequences

- Auto-approve is not an effect a single rule grants unilaterally; it is the *aggregate* of
  full vouch coverage + no blocks/challenges + score under threshold. This keeps the
  default-deny posture while letting packs be composed.
- The forge port (ADR-0005) must support: batched comments, resolvable threads, deny, approve
  + merge — already in scope; `challenge` resolution semantics on GitHub is OQ-7.
- Open sub-questions: score scale conventions and whether thresholds may also *downgrade*
  (e.g. force `challenge` → `block` in prod) — OQ-13.

## Counterpoints considered

- *"Severity levels are simpler."* — They conflate presentation with control flow; mapping
  severities to forge actions per repo would end up re-inventing exactly this effect table,
  but implicitly.
- *"Vouch-coverage is annoying; default-allow with deny rules is less work."* — Default-allow
  automerge on config repos is how outages happen; annoyance is the feature.

## Amendment 1 (2026-07-21, adversarial review F6/F7/F10)

Pinning down the tri-state, multi-pack, and cross-MR properties anticipated above:

**Tri-state predicates (F6).** A predicate evaluates to true / false / **error** (missing
fact, type mismatch, cost-limit hit, undefined). Error is fail-safe by effect: on a `vouch`
rule it means **no vouch**; on a `block`/`challenge`/`comment` rule the effect **fires**,
with the error surfaced in the finding details. An error can never silently make a rule
"not fire" in the permissive direction.

**Cross-pack coverage (F10).** When multiple packs are routed (union of findings, strictest
threshold), coverage is: an entry is covered iff **at least one routed pack vouches it**, and
**any** `block`/`challenge` from **any** routed pack stands — union of denies, single-vouch
trust. A strict pack that must not be undercut by a broad base-pack vouch declares
`coverage: exclusive` in its `pack.yaml`, requiring the vouch to come from itself for the
classes it owns. The multi-pack worked example belongs in the Phase-3 spec.

**Score is intra-MR only (F7).** Risk points do not accumulate across MRs (stateless,
D-007); salami-slicing a large change into many small MRs defeats the threshold by design.
Operators must not read the score histogram as a rate limiter; cross-MR accumulation would
require serve-mode state and is explicitly out of scope for v1.

## Amendment 2 (2026-07-21, second review P1-6/P1-7)

Arithmetic and a worked example for the `points` and `onFail` primitives declared above:

**Points multiplicity.** The predicate runs once per matched change (ADR-0011 amendment);
`points` accrue **per firing**, not per rule. `vouch` + `points` is therefore the built-in
bulk-change guard: ten vouched partition bumps at `points: 1` against a prod threshold of 4
→ REVIEW, by design ("individually safe, collectively worth a look"). Starter packs must
use points sparingly and the docs must state this multiplication explicitly.

**Rego vouch polarity.** Rego modules are violations-shaped; a violation feeds
`comment`/`challenge`/`block` effects. A rego-backed `vouch` rule must return the explicit
set of vouched change paths (`vouch contains path if { … }`); anything not in the set stays
uncovered. No implicit "no violation = vouch".

**`onFail` in practice.** The bounded-change worked example shows the shape `onFail` is for:
`vouch` on `new >= old && new <= quota` alone leaves the quota-exceeded case silently
uncovered with no message; with `onFail: {effect: challenge, message: "…exceeds quota…"}` the
contributor gets told. Predicate **error** remains its own case (tri-state, amendment 1):
errors never take the `onFail` branch — they fail safe by effect.
