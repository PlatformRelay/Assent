# ADR-0018: Policy lifecycle — phase, profiles, comparison

| | |
| --- | --- |
| **Status** | Accepted (D-030 — Phase-3 freeze review) |
| **Date** | 2026-07-24 |
| **Deciders** | Konrad Heimel |
| **Context links** | [ADR-0007](0007-rule-effects-decision-aggregation.md) (aggregation) · [ADR-0008](0008-change-classification-routing-scope.md) (routing) · [ADR-0010](0010-config-files-repo-layout.md) (pack / config layout) · [ADR-0014](0014-adopter-test-format.md) (presentation-split amendment) · [ADR-0017](0017-contract-model-obligations.md) §2–4, §7, §9 · D-017 (B2–B4) · OQ-21 (reversed) · [named-consumer-compat.md](../planning/named-consumer-compat.md) B2–B4 · schemas under `schemas/policy/v1alpha1/` + `schemas/comparison/v1alpha1/` · planning docs `docs/planning/policy-lifecycle-*.md` |

## Context

D-017 (B2–B4) reverses OQ-21's lean: rollout is an explicit `off`/`observe`/`enforce`
phase on rules and packs (not effect-editing), named **policy profiles** distinguish a
single writing profile from recorder-only counterfactuals, and a closed semantic
**comparison** taxonomy plus a versioned, promotion-gated `PolicyComparisonSuite` judge
whether a pack/profile change is safe *before* it reaches `enforce`. P3-E4-S01..S03 froze
the schemas and planning contracts; without a named ADR, Phase-3 freeze review has no
decision record to accept, and the named-consumer B2–B4 disposition rows have nothing
precise to link.

This ADR records those three contracts as independently supersedable decisions and freezes
the doc-level `assent compare` CLI contract (implementation is Phase 5+ / E6). It does not
implement the runner or weaken ADR-0017's enforcing aggregation / Reconcile obligations.

## Options

| Option | Pros | Cons |
| --- | --- | --- |
| A. Keep contracts only in schemas/planning — no ADR until engine impl | Thin Phase-3 surface | Freeze review has no ADR to accept; B2–B4 stay "new P3-E4"; supersession path unclear |
| B. Single ADR with three numbered decisions (this ADR) | Matches D-017 (B2–B4); each concern supersedable; adopters find one place | Slight restatement of schemas + planning docs |
| C. Three separate ADRs (phase / profiles / comparison) | Maximal independence | Index churn; cross-links heavier than the coupling warrants |

## Decision

### 1. Rollout phase field + DecisionRecord observed/enforcing split

Every MergePolicy rule and every Pack manifest requires an explicit `phase` field with
enum `off | observe | enforce` — no default (ADR-0017 §9 strict-decode). Pack `phase` is a
**ceiling**, never additive: pack `off` evaluates no rules; pack `observe` caps contained
rules at `observe`; only pack `enforce` lets each rule's own phase stand.

| Phase | Evaluate | Finding array | Feeds aggregation (`decision` / `blocks` / `requiredReviews` / `score`) |
| --- | --- | --- | --- |
| `off` | no | none | no |
| `observe` | yes | `findings.observed` | **no** (structurally excluded) |
| `enforce` | yes | `findings.enforcing` | yes |

Lint hard-error `no-implicit-enforce-phase` rejects missing `phase`. Effect-editing to
simulate rollout is an unsupported modeling choice; phase transitions must be visible in a
generic structural diff of two loaded-policy snapshots (and mirrored as
`findings.observed` ↔ `findings.enforcing` path moves on DecisionRecord) with no
phase-aware special case. Normative schemas/docs:
`schemas/policy/v1alpha1/merge-policy.schema.json`,
`schemas/policy/v1alpha1/pack.schema.json`,
`schemas/decision/v1alpha1/decision-record.schema.json`,
`docs/planning/policy-lifecycle-phase.md`,
`docs/planning/lint-hard-errors.md`.

### 2. Named policy profiles + single-writer + precedence

A `PolicyProfile` names a coherent activation of packs for `(environment, class)` bindings.
Exactly one covering profile may resolve `writes: true` for any given binding; every other
covering profile is **recorder-only** (`writes: false`) — evaluated for comparison, never
authorized to call `Reconcile` (ADR-0017 §7) or any forge write. Lint hard-error
`single-writer-profile` rejects zero or more-than-one writers for the same binding (never
last-one-wins).

Precedence is one schema-level artifact on `Config.profiles` (ordered `{name}` refs) —
**not** a second `match`/routing block. Resolution: coverage → specificity (narrower wins)
→ Config order tie-break; single-writer is checked across **all** covering profiles.
Normative schemas/docs: `schemas/policy/v1alpha1/profile.schema.json`,
`schemas/policy/v1alpha1/config.schema.json`,
`docs/planning/policy-lifecycle-profiles.md`,
`docs/architecture/policy-profiles.md`.

### 3. Comparison delta taxonomy + PolicyComparisonSuite + promotion gates + `assent compare`

Every baseline↔candidate decision difference over a suite case classifies into exactly one
closed taxonomy member — no `"other"` / free-text kind; unclassified differences are a hard
classification error (fail-closed):

- `stricter-intervention-added`
- `destructive-or-authorization-intervention-missed`
- `subject-or-obligation-uncovered`
- `newly-auto-mergeable`
- `score-threshold-change`
- `explanation-only` (includes wording-only `message` template changes; never trips a gate)

A versioned `PolicyComparisonSuite` pins an immutable corpus of `ReplayBundle`s by stable
`caseId` + `replayBundleDigest` (revise by minting a new `caseId`, never in-place edit) and
carries five machine-enforceable **promotion gates** plus an `acceptedDeltas` allowlist keyed
by `caseId` + delta identity (`rule` / `subject` [/ `obligation`]) + `kind` — never by kind
alone. Normative schemas/docs:
`schemas/comparison/v1alpha1/comparison-record.schema.json`,
`schemas/comparison/v1alpha1/comparison-suite.schema.json`,
`docs/planning/policy-lifecycle-promotion-gates.md`.

#### `assent compare` CLI contract (doc-level; impl Phase 5+ / E6)

| | |
| --- | --- |
| **Inputs** | Baseline `PolicyProfile` ref; candidate `PolicyProfile` ref; `PolicyComparisonSuite` ref (suite may supply default profile refs; CLI args override) |
| **Output** | Comparison report whose per-case delta list reuses the `ComparisonRecord` schema (closed taxonomy + per-delta identity + gate outcomes) |
| **Side effects** | **Side-effect-free** — must never call `Reconcile` (ADR-0017 §7) or any forge write path; both profiles are evaluated as recorders for delta classification even when one is the writing profile in live serve/CI |

Exit codes map 1:1 to promotion-gate outcomes (first failing gate wins if multiple fail;
reporting still lists all gate results in the report):

| Exit code | Meaning |
| --- | --- |
| `0` | All-pass — every promotion gate passed (or failed deltas were individually listed in `acceptedDeltas`) |
| `1` | Gate `zero-missed-destructive` failed |
| `2` | Gate `zero-missed-authorization-ownership` failed |
| `3` | Gate `no-unexpected-obligation-removal` failed |
| `4` | Gate `bounded-auto-merge-widening` failed |
| `5` | Gate `explicitly-accepted-deltas` failed |

## Consequences

- Phase-3 freeze review **accepted** this ADR (D-030); Status is **Accepted** and matches the
  ADR index row. Named-consumer-compat.md B2–B4 may link here as the accepted ADR.
- Engine impl (`assent compare`, profile evaluation, phase-aware aggregation) implements
  against the frozen schemas + this ADR; superseding any one of the three numbered decisions
  does not require rewriting the other two.
- Recorder-only / compare inertness remains an architectural invariant: comparison never
  widens forge write authority.
- Effect-editing rollout remains an unsupported anti-pattern; the sanctioned path is
  explicit `phase` + profile promotion via the suite gates.

## Counterpoints considered

**"Schemas and planning docs alone are enough — an ADR restates."** Rejected: D-017 (B2–B4)
and named-consumer-compat require New ADRs authored inside their owning epics and accepted
at the Phase-3 freeze review. Without ADR-0018, freeze review has no decision record to
accept, and B2–B4 cannot flip from "new P3-E4" to an accepted ADR link.

**"Goal/DoD prose says this story does not create `docs/adr/0018-*.md`."** Rejected for this
implementation lane: REQ-P3-E4-S04-01..03 Verify greps require the ADR file, README row, and
`assent compare` / side-effect-free contract to exist. Following Goal/Non-goals alone would
leave the story vacuous. Chose Verifies (same resolve as P3-E5-S04 / ADR-0019). Acceptance
remains a later operator story.

**"Fold phase + profiles + comparison into one unnumbered Decision paragraph."** Rejected:
the three concerns map to D-017 (B2–B4) and must remain independently supersedable — e.g. a
future multi-writer lock protocol must not force rewriting the phase ceiling semantics.

## Amendment (2026-08-08, D-121 — the corpus digest is algorithm-versioned, not eternal)

The Decision above states that a `PolicyComparisonSuite` pins its corpus by stable `caseId`
+ `replayBundleDigest`, "revise by minting a new `caseId`, never in-place edit". Read
literally, that made every published digest *value* permanent. It was written to fence one
specific abuse — silently re-pointing an existing `caseId` at different bundle bytes, which
would let a corpus entry drift out from under the gates — and that fence stands unchanged.

It did not anticipate a change to the digest *function*. **D-121** made one, once, before
v1: `compare.ReplayBundleDigest` moved from an undomained `sha256(json.Marshal(decoded))` to
the domain-separated `assent-jcs-v1` digest — canonical JSON hashed under the replay-bundle
schema `$id` (ADR-0017 §9) — rendered as bare lowercase hex with no `sha256:` tag, since the
value is no longer sha256 over bytes. Every pin in `examples/comparison/*/suite.yaml` was
regenerated in the same commit, with no `caseId` reused or retired and no bundle byte
changed.

The invariant is therefore restated as: **the corpus identity (`caseId` → bundle bytes) is
immutable; the digest value is immutable *under a fixed algorithm*.** An algorithm revision
requires its own logged decision, must regenerate the whole corpus atomically, and must not
be used as a route to re-point a `caseId`. Stale pins fail closed with a digest mismatch
before evaluation — the corpus cannot silently degrade.

This amendment corrects the ADR's reach, not its intent; D-114 (which claimed the canonical
digest the implementation never performed) is superseded by D-121 on that point.
