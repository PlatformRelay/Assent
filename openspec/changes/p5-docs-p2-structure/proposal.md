# P5-DOC2 — docs audit P2: structure and storytelling

**Change ID:** `p5-docs-p2-structure`
**Origin:** `data/assent-docs-audit/report.md` "Proposed changes" P2, items 9–11, re-verified
at `3503cc0` on `main` (2026-10-06). The P0 factual corrections (items 1–7) are open as PR
190 and are NOT in this branch's base; where this change copies README wording it sources
PR 190's corrected text so the two PRs agree once both land.
**Vehicle:** docs-only structural change. No factual claim in any moved or copied sentence is
edited; every sentence that moves is already published and gate-clean. The one scope
addition found at re-verification (S00) is itself structural, not factual.

---

## Problem

The audit's P2 findings, still true at `3503cc0`:

- **Item 9 (GAP-2).** `docs/index.md`, the published site's front door, is 18 lines of bare
  links: one sentence, seven links, no Why, no quick start, no status warning. The README
  carries the whole narrative and the site home carries none of it — while the README calls
  the site "the map". Its "Start here" links point at `planning/meta-plan.md` and
  `planning/open-questions.md`, working documents the nav deliberately excludes.
- **Item 10 (GAP-3, half).** The Usage nav order is Install → CLI reference → Walkthrough:
  the 395-line lookup table sits between the install page and the teaching page.
- **Item 11 (STYLE-1).** `docs/usage/cli.md` is a reference manual wearing a safety manual:
  roughly 155 of its 396 lines (`:98–251`) are five trust-model essays. Every claim in them
  was re-verified true by the audit; the defect is container, not content. Someone looking up
  `-checkout` wades through two pages of trust boundary, and the same material exists in
  ADR-0008 §Amendment 2 / ADR-0015 / ADR-0017 / ADR-0020.

**Found at re-verification (S00, not in the audit).** The E10 merge (#191) duplicated the
README quick-start block: `README.md:105–129` repeats the go-install caveat, the lint/test
block and the "No repo of your own yet?" paragraph that already sit at `:75–91`, leaving a
stranded "Developers: gates live in the Taskfile:" line in between. The duplicate still
carries `@v0.1.0`. Once PR 190 merges, its new DOC-13 pin — every copy-pasteable `@vX.Y.Z`
pin in the initial chapters equals the latest tag — will scan `README.md` and go red on that
second copy. De-duplicating is a precondition for both this change's item 9 (it copies the
quick start) and PR 190's clean landing.

## Scope

- **S00** — drop the README quick-start duplication (structural; no sentence it leaves
  behind is edited).
- **S01 (item 9)** — give `docs/index.md` a real first screen: the README's status banner,
  Why, How-it-works and Quick start, links adjusted to docs-relative form; keep "Start
  here" below the new content; move the planning links into a "Contributing" tail.
- **S02 (item 10)** — Usage nav order: Install → Walkthrough → CLI reference.
- **S03 (item 11)** — extract the five cli.md trust-model essays verbatim into
  `docs/usage/operating-safely.md` (Usage nav, per REQ-DOCSNAV-S01-01), leaving the flag
  table plus a one-line caveat + link per safety-relevant flag and a pointer paragraph;
  retarget the two cross-page/in-page references that point at the essays.

## Non-goals (deliberate)

- **GAP-1 / P1 item 8** — the adoption-path rework (walkthrough Steps 1+5, run-shape
  example files, the pack→run OQ) is held for the captain; this change does not touch
  `assent run` behaviour or any shipped default path.
- **No factual claims change.** Copied sentences carry PR 190's corrected wording
  (E1–E9; the `.tf` opaque clause; `@v0.4.0`) but nothing else is reworded. Moved essays
  are byte-identical modulo heading level and internal-link targets.
- **No new gate pins.** The repo's corrections-and-pins culture pairs each correction with
  a pin, but PR 190 (open) adds pins DOC-12..DOC-15 to `hack/docs/truthlag_pins_test.sh`;
  a pin added here would race its numbering and could not be co-edited into an open PR.
  The README↔index.md duplication this change creates is recorded in the hand-off as the
  follow-up pin candidate (a DOC-16 or numbered-after-190 pin comparing the copied block
  against the README it came from).
- **No "Writing rules" page** (the audit's item 10 "+ later" half) — it belongs with item 8.
- **No walkthrough banner, console-block or step edit** — the walkthrough is P0/P1
  territory; only its one link into a moved essay is retargeted (S03).

## ADRs / decisions

- REQ-DOCSNAV-S01-01 governs the new page's nav row; REQ-DOCSNAV-S01-04 requires the
  sanitization read recorded for it. No new D-nnn decision: this change implements audit
  recommendations, it does not decide architecture.

---

## Merge-order note (for the operator)

PR 190 is open against `4bd2d7f` and this branch is based on `3503cc0`. The two PRs touch
disjoint regions of the only shared file (`README.md`: 190 edits `:22`, `:45–46`, `:76`;
this change deletes `:105–129`) and this change does not touch
`hack/docs/truthlag_pins_test.sh`, which 190 extends by 187 lines. Either order merges
mechanically clean. Landing **this branch first** keeps `main`'s gates green throughout:
if 190 lands first into today's tree, its DOC-13 pin goes red on the duplicated `@v0.1.0`
until this branch removes it. The merge authority decides; the recommendation is here
because the branch with the pin can only be sequenced by the operator.
