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
- [x] P change created (proposal + spec delta + tasks) — this commit.
- [ ] R spec-set review — fanout, free legs only.
- [ ] L task loop — T001..T005.
- [ ] B branch review.
- [ ] Hand-off — push branch, open NON-DRAFT PR with gh-axi (delivery contract:
      direct-PR; the brief's Definition of done overrides spec-loop's draft default).

## Lessons

- (empty — first tasks pending)
