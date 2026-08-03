# Success metric — "≥70% of routine MRs" (OQ-25 / P1-E2-S03)

North-star wording from [`docs/vision.md`](../vision.md): after adoption, **≥70% of
routine MRs merge without human attention**, with every decision explained and
replayable. This document defines the measurable contract (ADR-0017 §9 / roast P2-8):
an independently defined **denominator**, an **adjudicated holdout** protocol, a
**false-auto-merge budget**, and how `scan`/`stats` report against it
(ADR-0009 amendment 2).

Closes **OQ-25** with the leading answer below. Holdout **labels themselves** are an
operator task — this file designs the protocol; it does **not** invent adjudicated
labels as final truth.

---

## Denominator

**Routine MR** is defined **independently of assent's own classifier and decision**.

| Rule | Detail |
| --- | --- |
| Population | Merged (and optionally closed-unmerged) MRs/PRs against the governed default branch in a sampling window, drawn from the adopter repo and/or the pinned open-source corpus ([`examples/repos/corpus.md`](https://github.com/PlatformRelay/assent/blob/main/examples/repos/corpus.md)). |
| Inclusion (routine) | Human reviewers at the time treated the change as mechanical self-service: single-class config edit, no incident/revert follow-up within a soak window, no security/compliance exception label, and (when available) review comments that only acknowledge ownership/bounds rather than design debate. |
| Exclusion (non-routine) | Multi-pack / cross-env redesigns, deletions/renames of live resources, `.assent/**` policy edits, schema migrations, MRs that required ≥2 human review rounds for substance, or any MR later reverted for correctness/safety. |
| Independence | Labels come from forge history + human adjudication — **never** from "assent would have APPROVEd". Using assent's decision as the denominator would circularly inflate the north star. |
| Point estimate | `automerge_eligible_routine / |routine_holdout|` where the numerator is MRs in the routine set for which assent's decision is APPROVE **and** the historical human outcome was merge-without-substantive-change (see confusion matrix). Target: **≥ 0.70**. |

Corpus note: kubernetes/org membership PRs are the best public stand-in for "routine
self-service"; JulieOps/octoDNS excerpts supply format diversity. Private adopter
histories (when sanitized per D-002) dominate once available.

---

## Holdout protocol

1. **Sample** — Stratified draw from corpus + adopter history (by env path, change class,
   size). Suggested v1 size: ≥100 MRs or 90 days of history, whichever yields more after
   exclusions; document the draw seed and pin SHAs.
2. **Blind labels** — Two labelers mark each MR `routine` | `non-routine` | `exclude`
   using only the denominator rules above and the forge UI (diff, discussion, outcome).
   **Labeler ≠ policy author** for that repo's packs (adversarial independence).
3. **Adjudication** — Disagreements go to a third adjudicator (operator or designated
   platform lead). Majority (or adjudicator break) becomes the holdout label.
4. **Freeze** — Write labels to a holdout manifest (path TBD in Phase 3 fixtures; until
   then an operator-owned spreadsheet/artifact). Do not silently relabel after measuring.
5. **Re-measure** — On policy change, re-run `scan` against the **same** holdout; do not
   redraw unless the operator opens a new holdout generation.

**Operator task:** adjudicate (or appoint labelers for) the first holdout set. No invented
"gold" labels ship in-tree as final truth in Phase 1.

---

## False-auto-merge budget

A **false auto-merge** is: assent decision APPROVE on a holdout MR whose adjudicated
label is `non-routine`, **or** whose historical outcome was revert / emergency fix /
human rejection for safety — i.e. assent would have automerged something humans later
treated as needing judgment.

| Knob | v1 default |
| --- | --- |
| Budget | **≤ 1%** of routine-denominator size per measurement window, and **zero** on MRs labeled non-routine that touch destruction / `.assent/**` / authz obligations. |
| Response action | If budget exceeded: (1) disable auto-merge arming for the offending class/pack (`phase: observe` once ADR-0018 lands; until then remove vouch/prove for that obligation), (2) file a pack fix with fixture from the offending MR, (3) re-`scan` holdout before re-arming. |
| Asymmetry | False **REVIEW** (missed automerge) hurts the 70% rate but not safety — tune packs. False **APPROVE** burns trust — budget is hard-fail for release/enablement. |

---

## Measurement via scan/stats

Per ADR-0009 amendment 2, `scan` records each historical MR's **actual outcome**
(merged / closed / reverted) alongside the decision under the policy ref under test.
`stats` aggregates a **decision-vs-outcome confusion matrix**:

|  | Human merged unchanged | Human rejected / reverted / heavy-edit |
| --- | --- | --- |
| assent APPROVE | true automerge candidate (feeds ≥70% numerator when MR ∈ routine) | **false auto-merge** (counts against budget) |
| assent REVIEW/BLOCK | expected caution / false REVIEW | true positive caution |

Reporting requirements for the north star:

1. Restrict the matrix to the **adjudicated routine holdout** for the 70% rate.
2. Report false-auto-merge rate on the full scanned set **and** on non-routine labels
   separately (must stay within budget).
3. Never quote "would-have-automerged %" alone — that is self-consistency, not trust
   (ADR-0009 amendment 2).

Until `scan` exists, Phase 1 records the metric definition here; Phase 3+ fixtures bind
the manifest schema.

---

## Leading answer (OQ-25)

| Question | Leading answer |
| --- | --- |
| Who defines "routine"? | Denominator rules above — forge history + human labels, independent of assent. |
| Holdout? | Blind dual-label + operator adjudication; labeler ≠ policy author; freeze then re-scan. |
| False-auto-merge budget? | ≤1% of routine set; hard zero on destruction/policy/authz; disable arming + fix pack on breach. |
| Measurement? | `scan` outcomes × `stats` confusion matrix filtered to holdout. |
