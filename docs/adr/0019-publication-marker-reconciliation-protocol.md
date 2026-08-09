# ADR-0019: Publication marker + reconciliation protocol (database-free)

| | |
| --- | --- |
| **Status** | Accepted (D-030 — Phase-3 freeze review). **One MUST is unmet:** `assent doctor` never emits `duplicate_prevention:` — see the *Implementation status* note below and [D-138](../decisions/decisions.md). |
| **Date** | 2026-07-24 |
| **Deciders** | Konrad Heimel |
| **Context links** | [ADR-0011](0011-core-ports-and-contracts.md) (`UpsertComment`/`SyncThreads`) · [ADR-0012](0012-presentation-templates-debug.md) (finding-key / marker comments) · [ADR-0015](0015-trust-boundaries-merge-integrity.md) §6 (serve dedup) · [ADR-0016](0016-presentation-theming.md) §1 (renderer-owned marker region) · [ADR-0017](0017-contract-model-obligations.md) §7 (`Reconcile`) · D-007 · D-017 (B6) · frozen contract [marker grammar](../contracts/p3-e5-publication-protocol/marker-grammar.md) |

## Context

D-017 (B6) commits to freezing the marker + reconciliation protocol that the Reconcile port
(ADR-0017 §7) and the finding-lifecycle state machine (ADR-0011, ADR-0012) already assume,
while preserving D-007 (no database — the forge itself, via hidden-HTML markers on
bot-authored comments, is the durable reconciliation surface). Without a named ADR stating
the marker grammar, the numbered reconciliation contract, and the one-publisher-per-MR
topology as independently supersedable decisions, "rerun idempotence" (P4-E1) has nothing
precise to implement against, and duplicate-comment incidents have no deterministic repair
rule.

The content of this ADR is lifted from the Phase-3 frozen contracts under
`docs/contracts/p3-e5-publication-protocol/` (P3-E5-S01..S03). It does not invent new
protocol content; it records the authorship criteria for Phase-3 freeze review acceptance.

## Options

| Option | Pros | Cons |
| --- | --- | --- |
| A. Keep protocol only in contracts/ — no ADR until engine impl | Thin Phase-3 surface | Freeze review has no ADR to accept; walkthrough/doctor drift; supersession path unclear |
| B. Single ADR with three numbered decisions (this ADR) | Matches D-017 (B6); each concern supersedable; adopters find one place | Slight restatement of contracts/ |
| C. Three separate ADRs (marker / reconcile / topology) | Maximal independence | Index churn; cross-links heavier than the coupling warrants |

## Decision

### 1. Four-concept marker split

Every bot-authored forge comment/thread carries a hidden-HTML marker whose payload has
exactly four top-level concepts — `slot`, `occurrence`, `decision`, `artifact` — as frozen
in [`marker-grammar.md`](../contracts/p3-e5-publication-protocol/marker-grammar.md) and
[`marker-grammar.schema.json`](../contracts/p3-e5-publication-protocol/marker-grammar.schema.json):

- **`slot`**: stable identity from project/MR, rule ID, obligation, EntryRef, effect, anchor.
- **`occurrence`**: hash of the safety-relevant judged content (changed content cannot inherit
  a prior resolution).
- **`decision`**: DecisionRecord hash that requested this state.
- **`artifact`**: kind (`finding-thread` \| `summary-comment`) + marker schema version.

Markers are **correlation metadata only** — never decision input or authorization evidence.
Only bot-authored comments are parsed (contributor spoofing is ignored by author-identity
filter). Markers carry no secrets, fact values, user-controlled Markdown, or raw policy
expressions. This decision preserves D-007: the forge comment list + markers are the sole
durable reconciliation surface.

### 2. Nine-step reconciliation contract

Every reconciliation run — fresh, plain rerun, or crash-then-rerun — executes the nine
numbered steps frozen in
[`reconciliation-state-table.md`](../contracts/p3-e5-publication-protocol/reconciliation-state-table.md),
in order:

1. Recompute `DesiredReviewState` from trusted inputs (target-ref trust).
2. List paginated bot-authored artifacts (no-database/D-007).
3. Update the one summary slot in place (determinism).
4. Leave the same unresolved occurrence untouched (determinism).
5. Preserve resolution of the same occurrence across reruns (no-database/D-007).
6. Supersede stale occurrences with a fresh challenge (determinism).
7. Resolve no-longer-desired findings (target-ref trust).
8. Deterministically repair pre-existing duplicates — lowest forge ID canonical; repairs
   recorded in `PublicationReceipt.repairs` (determinism).
9. Rescan after publication before reporting success (no-database/D-007).

The (existing-artifact-state × desired-state) → action table and the fixtures
(`rerun-idempotence`, `crash-then-rerun`, `duplicate-repair`) are normative for P4-E1's
rerun-idempotence exit gate. No row triggers more than one action.

### 3. One-publisher-per-MR topology

Strict duplicate *prevention* (as opposed to step 8's post-hoc *repair*) requires exactly
**one publisher per MR** at a time:

| Mode | Serialization mechanism |
| --- | --- |
| One-shot CI (`assent run`) | GitLab CI `resource_group` keyed per MR IID (e.g. `assent-mr-$CI_MERGE_REQUEST_IID`) |
| Long-lived `serve` | Keyed per-MR lock held for the duration of one Reconcile call |

**Multi-replica HA is unsupported.** Concurrent unserialized publishers converge only on the
next reconciliation (step 8 repair), never immediately. `assent doctor` MUST emit
`duplicate_prevention: single-writer-serialized | unserialized-best-effort` and must **never**
claim `single-writer-serialized` when it cannot verify the serialization mechanism — the safe
default on ambiguity is `unserialized-best-effort`. The setup walkthrough's CI step and the
doctor checklist must state this requirement explicitly (P3-E5-S04).

> **Implementation status (2026-08-09, [D-138](../decisions/decisions.md)) — the
> `duplicate_prevention:` MUST above is UNMET.** The value is computed and typed all the way to
> the report — `internal/forge/precondition.go` defines both constants and defaults to
> `unserialized-best-effort`, and `cmd/assent/doctor_forge.go:19` copies it into
> `PreconditionReport.DuplicatePrevention` — and then `emitDoctorReport` (same file, `:85-99`)
> prints only the arm-eligible verdict and the refusal reasons. **No `assent doctor` output
> contains the string `duplicate_prevention`.** The *safe-default* half of the MUST does hold:
> nothing can claim `single-writer-serialized` without the serialization mechanism, because
> `PreconditionFromCapabilities` seeds the field to `unserialized-best-effort`. What is missing
> is the emission, so an operator cannot read the guarantee level off the tool.
>
> Recorded rather than fixed, deliberately: this is one instance of the broader
> **audit ARCH-11** — doctor computes a typed capability report and prints essentially none of
> it, against the MUSTs of two ADRs. Emitting this one field would half-close ARCH-11 and leave
> the report inconsistent with itself, and it is a user-visible CLI output change; both belong
> in the v0.2.1 ARCH-11 slice with its own tests, not in a docs-truth lane.

## Consequences

- Phase-3 freeze review **accepted** this ADR (D-030); Status is **Accepted** and matches the
  ADR index row.
- Engine impl (Reconcile port, `doctor`, `serve` keyed lock) implements against the frozen
  contracts + this ADR; superseding any one of the three numbered decisions does not require
  rewriting the other two.
- Adopters copying the walkthrough without reading this ADR still get `resource_group` (or
  serve keyed-lock) wiring; doctor warns rather than silently claiming serialization.
- GitHub marker parity remains Locked (D-012); this ADR is GitLab-first.

## Counterpoints considered

**"Contracts alone are enough — an ADR restates."** Rejected: D-017 (B6) and the named-consumer
compat note require New ADRs authored inside their owning epics and accepted at the Phase-3
freeze review. Without ADR-0019, freeze review has no decision record to accept, and the three
concerns cannot be superseded independently.

**"Fold marker + reconcile + topology into one unnumbered Decision paragraph."** Rejected: the
spec (REQ-P3-E5-S04-01) requires three separately numbered decisions so each can be superseded
independently later — e.g. a future multi-replica lock protocol must not force rewriting the
marker grammar.

## Amendment (2026-08-08, AUD-S12 / audit finding REL-06 — malformed-marker resilience)

Decision 2's step 2 ("list paginated bot-authored artifacts") did not say what happens when a
bot-authored artifact carries the marker sentinel but an **undecodable payload**. The
implementation returned a hard error, so a single corrupted marker made every subsequent
reconciliation on that MR fail until a human deleted the note: fail-closed, but it bricked the
MR, and nothing in this ADR justified that severity.

As of AUD-S12, step 2 **skips** such an artifact — it is treated as not-a-slot-note — records a
warning, and reconciliation proceeds. Three properties make the skip safe, and they are the
reason this is an amendment rather than a supersession:

- **Decision 1 is unchanged and load-bearing.** Markers remain correlation metadata only — never
  decision input, never authorization evidence — so an artifact whose marker could not be read
  can never approve anything. The worst case is that its slot looks unoccupied.
- **The author-identity filter is unchanged and still runs FIRST.** A contributor comment is
  excluded before its marker is examined, so it stays invisible whether its marker is well-formed
  or corrupt, and it never produces a warning. The spoofing surface is exactly as it was.
- **The worst case converges — by reuse, NOT by step-8 repair.** Because the skipped artifact is
  filtered out of the step-2 listing, it is invisible to every later step: it can never present
  as a *visible* duplicate, so **step 8 never fires for it** and `PublicationReceipt.repairs`
  stays empty. Convergence comes from the ordinary idempotent reuse path instead, identically for
  both artifact kinds: run 1 posts exactly one healthy artifact for the slot, and every later run
  finds and reuses that one — a re-posted thread is matched by (slot, occurrence) and left
  untouched (step 4), and a re-posted summary note is edited in place (step 3). Either way no
  second duplicate accumulates. The corrupted artifact itself is deliberately **not** auto-deleted
  — write minimisation — so it keeps warning until an operator removes it, which is why the
  warning must be visible.

`PublicationReceipt` therefore gains a top-level `warnings` array of operator-facing strings,
each naming the skipped artifact. It follows `repairs` exactly: additive, `omitempty`, and
carried by the receipt schema's top-level `additionalProperties: true`, so no schema change is
required and no prior receipt changes shape. Entries are deduplicated and sorted (step 9's rescan
sees the same artifact more than once), which keeps a double run byte-identical. Warnings are
attached on **every** Reconcile return, including the typed refusals
(`ErrArmingRefused`/`ErrIncompletePreconditions`/`ErrSHAMoved`) — an unarmed advisory run is the
default adopter posture, and dropping the warning there would restore the invisibility this
amendment exists to remove.
