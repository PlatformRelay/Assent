## Verdict: CONCERNS

## Findings

- [WARNING] The stated target range `49d573f..HEAD` is empty — the three edits exist only as unstaged working-tree changes, so the diff-lens claim ("the change does what its commit message claims") has no commit to check, and every review leg dispatched on that range was fed nothing.
  Failure: `git log 49d573f..HEAD` → no commits; `git status` shows `hack/audit/exitgate_test.sh`, `evidence/T005.md`, `specs/docs-p2/spec.md` modified and `evidence/T006.md` + `reviews/L-tasks/T006/` untracked. `evidence/T006.md:44-45` instructs reviewers to read `git diff 49d573f..HEAD`, which prints nothing.
  Fix: commit the three edits (+ T006 evidence/register), then re-run the legs on the real range.
  Confidence: 95
- [WARNING] The fix moved the inventory claim to four in the spec but left its mirror at three — the same sentence, now divergent between two tracked files of the same change.
  Failure: `openspec/changes/p5-docs-p2-structure/proposal.md:52-54` still reads "is three, not two: cli.md's doctor section, walkthrough.md:212, and a prose pointer inside accepted ADR-0009"; a reader taking the proposal as the change summary gets the count the register (row 5, 4 legs, conf 100) called false.
  Fix: mirror the (d) clause into `proposal.md:52-54`.
  Confidence: 90
- [WARNING] The amended sentence asserts an exhaustive "four" while the register row it closes names a fifth prose pointer in the decision log that is still unlisted and unrecorded as a residual.
  Failure: `docs/decisions/decisions.md:141` (D-134) says "The *What gates approve and merge* section this row added…" and "The decision matrix above is now published in `cli.md`" — that matrix is now `docs/usage/operating-safely.md:10`, so the page attribution is false post-move (verified at `3503cc0` too). `evidence/T004.md:258-261` lists residuals and omits it. Consequence: a doc-truth sweep treats D-134 as clean while (d) is held to the opposite standard.
  Fix: add "(e) D-134 (`docs/decisions/decisions.md:141`)" with the same frozen-record disposition and append it to the residual list; or scope the sentence to "the four pointers that name a moved section by title".
  Confidence: 75 (T006 declares this half out of scope, but the sentence was strengthened anyway)
- [NOTE] Register row 3's other half — corpus headroom — is not closed: deletion of any two non-required surfaces still leaves the phrase sweep green.
  Failure: present=10 (dead `docs/usage/quickstart.md` entry, `loop.md:75`), `PHRASE_CORPUS_MIN=8` (`hack/audit/exitgate_test.sh:275`); delete `docs/index.md` + `examples/README.md` → 8 ≥ 8, green, and a retired phrase confined to either is unswept.
  Fix: drop the dead quickstart entry and set `PHRASE_CORPUS_MIN=9`.
  Confidence: 80
- [NOTE] The new required-surface entry has no mutation control of its own, so the added line itself is unpinned.
  Failure: `missing-required` removes only `docs/usage/walkthrough.md` (`hack/audit/exitgate_test.sh:1923-1926`); deleting `docs/usage/operating-safely.md` from the list at `:830-831` keeps every control green (file present in the live root).
  Fix: loop the mutant over each required entry, or add one mutant that drops `operating-safely.md`.
  Confidence: 85
- [NOTE] This diff edits `evidence/T005.md` yet leaves a known-false quote one row below the edit.
  Failure: `evidence/T005.md:13` still says readme smoke = `OK: 7 README quick-start command(s) green`; `evidence/T006.md:70-73` verifies the script prints `3 green, 4 skipped with a stated reason`. Disclosed, not corrected, so the tracked gate matrix keeps a missummarised sensor line.
  Fix: correct that cell while T005 is still open.
  Confidence: 85

## Could not check
- Ran no gate (read-only run): `--text-only` exitgate, `task docs-gates`, `task docs-build`, the `-v` PASS line and sanitization accepted from `logs/T006.log` / `evidence/*.log`, not re-executed.
- Full (non-`--text-only`) exitgate and `task check` — not run per the declared baseline-coverage deviation (90.5% < 91%, E10); unverified whether that red still stands at this tree.
- Built `site/` anchor slugs for the new page; `not_in_nav`/mkdocs nav membership of `operating-safely.md` taken from evidence, not rendered output.
- PR 190's DOC-13 closer (open PR) — merge-order effect on the corpus/README pins.
- Anything outside this repo (sibling checkouts, `_workbench` records) — not attempted.
