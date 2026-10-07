# loop.md — p5-docs-p2-structure (spec-loop state, adapted to OpenSpec)

- Base branch: `main` at `3503cc0` (branch `fm/assent-docs-p2-structure` created from it).
- Feature: `openspec/changes/p5-docs-p2-structure/` (proposal + spec delta + tasks).
- Origin: `data/assent-docs-p2-structure` brief → `data/assent-docs-audit/report.md` P2
  items 9–11, re-verified at `3503cc0`.
- Out of scope: GAP-1 adoption path (held for the captain); `assent run` behaviour.
- **Reviewers: fanout (free only) — no Claude leg, ever.** Implementer: GLM-5.3 (this
  session). Task review legs: `DeepSeek-V4.1-Flash:diff`, `Qwen3.8-Flash-Next:diff`.
  Strong leg at R and B: `Qwen3.8-2.4T-A95B-NVFP4:spec` (R), `:spec,:adversarial` (B).
  Budget: `--budget hours=8`, Claude legs = 0.

## Re-verification record (orient)

Measured at `3503cc0` (= `origin/main`), 2026-10-06:

- `docs/index.md` — 18 lines, the GAP-2 stub. Item 9 applies unchanged.
- `mkdocs.yml:83–86` — Usage = Install → CLI reference → Walkthrough. Item 10 applies.
- `docs/usage/cli.md` — 396 lines; the five essays at `:98–251` (audit said `:97–250`;
  E10's `-forge` row shifted the table). Item 11 applies.
- Pin sweep: the ONLY grep pin touching `cli.md` is XREV-S03-06 (`truthlag_pins_test.sh:500–508`),
  which pins the `--config` flag-table row — a row that stays. No pin greps any essay
  sentence; the item-11 "update the pins in the same commit" duty reduces to keeping the
  existing gates green + retargeting the two references into the essays
  (`cli.md` doctor section; `walkthrough.md:212`).
- Cross-page anchor sweep: exactly one external link into the essays
  (`walkthrough.md:212`). `validation.anchors` is `info` (DOCSNAV-R01) so broken anchors
  build green — anchors verified by hand against the built site instead.
- PR 190 (P0) is OPEN against `4bd2d7f`, not merged; its diff touches README (3 hunks)
  and adds +187 lines of pins; it does not touch cli.md/index.md/mkdocs.yml. Disjoint.
- **Baseline red (pre-existing):** `task check` fails at the coverage stage on
  `3503cc0`: 90.5% < 91% floor (D-010/D-128). `internal/forge/github` 84.1%,
  `conformance` 88.8%, `schemadrift` 84.8% are the low packages; E10's merge is the
  likely cause. Docs work cannot move this; tasks run the docs-relevant subset.
- **Baseline defect found (this change's S00):** E10's merge duplicated README
  quick-start lines; the duplicate carries `@v0.1.0`, which reddens PR 190's future
  DOC-13 pin when 190 merges.

## Fitness functions (inventory at orient — docs subset)

| Characteristic | Command | Type | Baseline |
| --- | --- | --- | --- |
| Nav completeness (DOCSNAV-S01) | `task docs-build` (mkdocs --strict; `omitted_files: warn`) | triggered | green |
| Docs truth-lag pins (DOC-05/06/08/09/10/11/02, XREV-S03-*, E10-S15-*) | `bash hack/docs/truthlag_pins_test.sh` | triggered | green |
| README quick start executes (DOC-07) | `bash hack/docs/readme_smoke_test.sh` | triggered | green |
| Example format inventory | `bash hack/docs/example_format_inventory_test.sh` | triggered | green |
| Retired-phrase corpus (front-of-house) | inside `hack/audit/exitgate_test.sh` | triggered | green (runs at branch tip) |
| Pre-release claims | `go test ./cmd/assent -run TestNoStaleProductClaims` | triggered | green |
| Sanitization (D-002) | `bash hack/check-sanitization.sh` | triggered | green |
| Go coverage floor (D-010) | `task coverage` | triggered | **red at baseline (90.5% < 91%)** — out of scope |

## Stage log

- [x] 0 orient — re-verification above; reviewers recorded.
- [x] P change created (proposal + spec delta + tasks) — commit `6d3046f`.
- [x] R spec-set review — round 1: 7 free legs (6 reported; DeepSeek-V4.1-Flash:
      adversarial exit=truncated, empty file, not counted), register at
      `reviews/spec/register.md`. **Verdict: 2 CRITICAL, both with mechanical fixes
      the spec already implies, verified in code, raised by ≥2 legs each → fixed in
      the spec set and re-reviewed once instead of stopping** (mechanical-fix rule;
      decision logged in the dispositions below).
- [x] R re-review (round 2, final) — register at `reviews/spec-round2/register.md`
      (2/4 legs; the DeepSeek-spec and Qwen-2.4T legs timed out). Verdict: BLOCK on
      one recurring finding + one notation WARNING. **Round-2 disposition (two-round
      cap reached — fixed, logged, no third round, disclosed at hand-off):**
      - CRITICAL (GLM-5.3 + Qwen3.8-Flash-Next): my round-1 C2 fix over-corrected —
        PR 190's README is based on the pre-E10 tree, so sourcing the quick start
        from it would drop the forge-selection block and the GitHub comment-only note
        S01-01 mandates (its GitHub row even says **Planned**). VERIFIED in code:
        pr-190's README lacks `--forge github`/OQ-33/OQ-34 and says Planned at :111.
        FIX: copy source is the branch README with PR 190's three P0 hunks applied
        (S01 Goal, REQ-DOC2-S01-04, T002 step 2 rewritten).
      - WARNING (both legs): S03-04's mutant arithmetic notation garbled
        ("10−4 of 11"); conclusion unaffected. FIX: reworded with the true counts
        (11 listed, 10 present — quickstart.md is a pre-existing dead entry; 10−4=6 < 8).
- [ ] L task loop — T001..T005.
- [ ] B branch review.
- [ ] Hand-off — push branch, open NON-DRAFT PR with gh-axi (delivery contract:
      direct-PR; the brief's Definition of done overrides spec-loop's draft default).

## R round-1 dispositions (register: reviews/spec/register.md)

| # | Finding (leg) | Disposition |
| --- | --- | --- |
| C1 | operating-safely.md leaves the exitgate retired-phrase corpus (GLM-adv CRITICAL; Qwen-FN-spec + Qwen-2.4T WARNING/NOTE) | **fix** — page joins PHRASE_CORPUS in the same commit (REQ-DOC2-S03-04, T004 step 5); verified in code: fixed list, MIN 8, shrunk mutant 10−4=6 < 8 still red |
| C2 | T002 "copy the README wording" contradicts REQ-DOC2-S01-04 (Qwen-FN-security CRITICAL; GLM-adv + Qwen-2.4T WARNING) | **fix** — copy source is `git show pr-190:README.md`, named in T002 + S01 Goal + REQ-DOC2-S01-04; PR 190's wording wins on conflict |
| W | `-pack` caveat links a bare in-page anchor that leaves cli.md (3 legs) | **fix** — target is `operating-safely.md#how-to-keep-assent-advisory` (T004 step 2, REQ-DOC2-S03-02) |
| W | reference inventory short by one: ADR-0009:55 prose pointer (3 legs) | **fix (spec)** — inventory is three; ADR-0009 left untouched (immutability), disposition + hand-off note |
| W | advisory essay's "the CI snippet above" deixis stranded (3 legs) | **fix** — named navigation-only transformation allowed in S03 Goal + T004 step 1 |
| W | doctor link's "above" false after retarget (2 legs) | **fix** — drop the word when retargeting (S03-03, T004 step 2) |
| W | merge-order premise false: walkthrough.md is a second shared file (2 legs) | **fix** — proposal merge-order note rewritten with the six 190 hunks vs my :212 line, disjoint |
| W | origin `data/assent-docs-audit/report.md` is a dead reference (GLM-adv) | **reject** — repo precedent: `p5-xrev-final-remediation` cites its origin in the supervisor's `data/` home the same way |
| W | item 10 delivered only in half / adjudicating source not in repo (Qwen-FN-spec) | **reject** — the "+ later Writing-rules" half is item 8's territory, held for the captain (proposal non-goals); origin-citation precedent as above |
| W | --checkout disclosures stripped from the wall an operator hits (Qwen-FN-security) | **reject** — the audit prescribes exactly this shape; the `-checkout` row keeps its own "The tree must contain no symlinks" caveat in place, the essay is one link away |
| W | nothing protects the new safety links from anchor rot (Qwen-FN-security) | **defer → DOCSNAV-R01** (existing residual; raising `validation.anchors` is fenced as a different defect class); REQ-DOC2-S03-03 verifies the anchors at build time by grep |
| W | DOC-13's initial-chapter list can't gain the new page (Qwen-2.4T NOTE) | **defer** — open PR 190 owns that list; the page carries no version pin by construction; residual recorded in REQ-DOC2-S03-04 |
| N | index.md intro's fate unrecorded (Qwen-2.4T) | **fix** — T002 step 1 states the intro is replaced |
| N | audit's "seven links" miscount echoed | **fix** — proposal reworded to the measured count |
| N | adr/** said to build via not_in_nav (2 legs) | **fix** — S01-02 corrected: ADRs are navigated |
| N | S00-01's Test cannot redden the duplication (Qwen-FN-security) | **fix** — Test line states that honestly; L0 closes on diff review |
| N | check-migration-invariants.sh missing from gate set (Qwen-FN-security) | **fix** — tasks use `task docs-gates` |
| N | sanitization mechanical half not run at T002 (Qwen-FN-security) | **fix** — added to T002 step 4 |
| N | REQ-DOC2-S01-01 omits the GitHub comment-only note (2 legs) | **fix** — added to the REQ's content list |
| N | S03-02 silent on replacing the rows' italic tails (Qwen-FN-security) | **fix** — stated in S03-02 + T004 step 2 |

Decision log (loop-owned): the two CRITICALs were fixed in the spec set rather than
stopping the loop, under the mechanical-fix rule — both verified against the tree
(corpus list + mutant arithmetic; README's stale `E2–E8`/`@v0.1.0` at `:22`/`:76` vs
pr-190's hunks), each raised by two or more legs, and the fixes are edits to this
change's own spec set, not to any published surface.

## Lessons

- (empty — first tasks pending)

## B round-1 dispositions (register: reviews/branch/register.md, 9/9 legs)

| # | Finding (legs) | Disposition |
| --- | --- | --- |
| C1 | README↔index quick-start divergence gate-invisible until PR 190 (6 legs; spec legs hold S01-04 spec-sanctioned) | **defer** — merge-order recommendation in the proposal is the closure; follow-up README↔index sync pin recorded in the hand-off |
| C2 | <redacted: operator home path> ~149× + temp paths committed in logs/evidence/prompts (2 legs) | **fix (orchestrator, commit `e9a1ca6` lineage)** — untracked logs/, task-prompt-*.md, t005-*.log; .gitignore patterns added; evidence replaced by a named matrix. Sanitizer hardening (home-path pattern, CI-bare-layer gap) = hand-off proposal |
| C3 | corpus permits silent coverage loss of the new page: MIN headroom 2, required-surface loop omits operating-safely.md (2 legs) | **fix** — required-surface list gains `docs/usage/operating-safely.md` (T006, `a74076e`); missing-required mutant extended to remove it too (pre-round-2 batch); MIN stays 8 (spec-settled) |
| C4 | <redacted: internal model registry id> committed in logs (1 leg CRITICAL, 95) | **fix** — same untrack as C2; the string now appears only where it IS the finding (register rows), the quoted-pattern precedent the exitgate documents |
| W5 | spec inventory short: fourth pointer in p4-e1-s11 record (4 legs) | **fix** — T006 extended it to five (adds D-134's row); proposal mirror reconciled in the pre-round-2 batch |
| W6 | .gitignore widens exclusion register (**/reviews/**/*.err); ignored files leave the sanitization scan surface (4 legs; all judged the trade sound) | **accept** — logged trade: tool transcripts are not the committed record; sanitization's scan surface is deliberately the committed tree |
| W7 | cross-page anchors have no fitness function (2 legs) | **defer → DOCSNAV-R01** (existing residual, fenced) |
| W8 | TestNoStaleProductClaims evidence vacuous (bare ok) (1 leg, 80) | **fix** — T006 replaced it with the named `-v` PASS record |
| NOTE | T006 round-2: T005 smoke-quote cell un-emittable | **fix** — corrected by T006 in-scope |

Loop-made pre-round-2 batch (mechanical, verified by run): proposal.md inventory mirror
(three → five); exitgate missing-required mutant extended to operating-safely.md
(exitgate --text-only rc=0, truthlag green, docs-build green).

T006 deferral accepted: no per-entry mutation control beyond the extended
missing-required mutant — one mutant proving the mechanism reddens on this entry is
enough; per-member controls for every list row is over-engineering (logged decision).
