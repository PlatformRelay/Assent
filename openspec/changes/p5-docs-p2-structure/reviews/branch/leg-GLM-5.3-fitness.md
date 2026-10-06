Spec read. Now the substantive diff hunks (not the logs/evidence yet):
Substantive hunks read. Now the fitness functions the branch touches — the gate scripts, their corpora, and the pin tests:
Corpus entry landed in the same commit as the page (8fffed0) — S03-04's same-commit requirement holds. `TestNoStaleProductClaims` walks `../../docs` as a directory, so the new page is automatically in scope. Now a mechanical verbatim check of the S03-01 essay move, plus anchor/existence checks:
Verbatim move verified clean (only the three spec-named transformations). Now the README quick-start order, the ADR-0009 residual record, the corpus-shrink mutant, and the recorded gate runs:
README order matches S00-01; ADR-0009 residual recorded. Now the corpus-shrink mutant the spec's math relies on, and the recorded gate runs at tip:
## Verdict: CONCERNS

## Findings

- [WARNING] The README→index.md quick-start copy has no equality ratchet in this base — `docs/index.md:69-103` vs `README.md:66-103`
  Failure: any future edit to the README quick-start (flag change, E11 wording) leaves `docs/index.md` silently stale — the exact GAP-2 class this audit family exists to close; the sanctioned closer (DOC-13, PR 190's index.md scan) is not in this base and PR 190 is still open, so until it lands the duplication window is unguarded. The spec records the residual (REQ-DOC2-S01-04: "after both PRs land, DOC-13's scan…") but nothing here enforces even the three sanctioned hunks.
  Fix: extend `hack/docs/truthlag_pins_test.sh` with a two-file pin asserting the four shared quick-start sentences (`.assent/.assent` caveat, `examples/packs/service-catalog`, E10-S13 forge line, GitHub comment-only line) exist in both README.md and docs/index.md — greps today, survives DOC-13 later.
  Confidence: 70 (gap is real; the spec sanctions it only as temporary).
- [NOTE] Cross-page doc anchors have no fitness function — `docs/usage/cli.md:61,103`, `docs/usage/operating-safely.md:10,152`, `docs/usage/walkthrough.md:212`
  Failure: `mkdocs build --strict` validates file existence, not anchors; a heading rename on cli.md (`## assent run`, `## assent doctor`) or operating-safely.md (`## What gates approve and merge`, `## How to keep assent advisory`) silently breaks 6 links (2 flag rows + 3 retargets + deixis). S03-03's anchor check was a hand grep recorded once in T004.md, not a gate.
  Fix: heading-presence pins (the four slugs above) in `hack/docs/truthlag_pins_test.sh` — slug stability without a site build.
  Confidence: 85.
- [NOTE] New corpus member is floor-invisible to the corpus gate — `hack/audit/exitgate_test.sh:269,275,823-835`
  Failure: presence floor is count-based (11 listed, MIN 8): deleting `operating-safely.md` alone leaves 9 ≥ 8, so the retired-phrase scan would stop covering the new page without red; the required-surface loop at `:833` pins only README/API_STABILITY/walkthrough/main.go. (Page deletion is caught elsewhere — mkdocs nav + docs-build — so severity is low, and the spec's own 4-removal mutant math at `:1927` holds: 6 < 8 red.)
  Fix: add `docs/usage/operating-safely.md` to the required-surface loop.
  Confidence: 80.
- [NOTE] Exclusion register widened — `.gitignore:19-20`
  Failure: `**/reviews/**/*.err` now excludes all review-leg stderr streams from version control; a substantive finding existing only in a `.err` would be silently untracked. Mitigated: register.md and leg-*.md (the deliberate record) remain tracked, and full transcripts T001–T004.log are committed.
  Fix: none needed; keep the pattern scoped to `reviews/` as it is.
  Confidence: 70.

## Fitness accounting (what moved, and what would have caught it)

- Characteristics moved: docs layering improved (cli.md is now a pure per-command/per-flag lookup, −148 lines; trust model one click away — spec's STYLE-1 aim); nav order teaching-before-reference (`mkdocs.yml:84-88`); sensor corpus +1 (`exitgate_test.sh:270`, added in the same commit as the page, 8fffed0 — S03-04's same-commit rule holds); deliberate two-file copy coupling introduced (S01); one ignore-register widened.
- Functions that would have failed on error: `docs-build --strict` (bad file links, nav entries), `check-sanitization.sh` and `TestNoStaleProductClaims` (walks `../../docs`, so the new page is automatically in scope), `truthlag_pins_test.sh` XREV-S03-06 (cli.md `--config` row untouched — verified), corpus floor + shrunk-corpus mutant. A mechanical diff I ran of the five moved essays against `3503cc0:cli.md` shows verbatim modulo exactly the three spec-named transformations — the S03-01 verbatim fence holds, and it was enforced only by review, as the spec itself states.
- Worth adding as a ratchet (smallest first): the README↔index shared-sentence pin (WARNING's fix), then the four anchor pins. Nothing else — the branch otherwise leaves the gate matrix intact and does not lower any baseline (PHRASE_CORPUS_MIN stays 8; COVERAGE_MIN untouched).

## Could not check

- I did not re-run any gate; I read the recorded t005-*.log runs (docs-gates, exitgate --text-only, nostale, sanitization — all green) instead. Note T005.md records the matrix at 85eb7c8 while branch tip is b7cf7cb; the delta is openspec/-only evidence files, so I judged it immaterial but did not verify a run at b7cf7cb itself.
- The built `site/` HTML (S03-03's anchor presence in built pages) — not built in this tree; I verified anchors exist as source headings and trusted T004's recorded grep.
- PR 190's actual contents (out of repo); its three P0 hunks were checked only as described in the spec.
