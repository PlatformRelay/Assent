## Verdict: CLEAN

## Findings

- [NOTE] `docs/index.md` canonical-repo link uses lowercase `PlatformRelay/assent` — `docs/index.md:8`
  Failure: cosmetic only; the canonical casing is `PlatformRelay/Assent` (`mkdocs.yml:3-4`, gh remote). Copied verbatim from the branch README, so it is in-spec; no link breaks.
  Fix: none required (or uppercase to match `repo_name`).
  Confidence: 90.

- [NOTE] `.gitignore` gains a new exclusion register `**/reviews/**/*.err` — `.gitignore:19-21`
  Failure: an exclusion widened with no spec REQ naming it (scope addition beyond S00–S03). Verified harmless: no `.err` is tracked (`git ls-files '*.err'` empty) and the pattern is scoped to `reviews/` dirs, so no gate input is hidden.
  Fix: none; the comment justifies it as a spec-loop rule.
  Confidence: 85.

## Spec compliance (each REQ holds; deciding file:line)

- **S00-01** — README caveat/lint-test/sample-repo/forge/GitHub-note/task-check each appear exactly once, in order (`README.md:75-109`); the only diff hunks are deletions in the duplicated block. Holds.
- **S01-01/02/03** — index.md carries banner→Why(3 bullets)→How-it-works(mermaid+stateless)→Quick start in order (`docs/index.md:8-87`); every link target exists (`vision.md`, `usage/walkthrough.md`, `adr/README.md`, `architecture/c4-*.md`, `decisions/decisions.md`, `planning/*`, `api-stability.md`); Start-here vs Contributing split is correct (`:89-100`). Holds.
- **S01-04** — automated diff of index.md vs branch README (links normalised) shows only the three mandated P0 hunks: `E2–E8`→`E1–E9` (`index.md:9`), the `.tf` opaque clause (`:28-30`), `@v0.1.0`→`@v0.4.0` (`:60`); wording otherwise byte-identical. PR190's local `pr-190` ref carries exactly those wordings. Holds.
- **S02-01** — `mkdocs.yml:82-86` lists Install → Walkthrough → CLI reference. Holds.
- **S03-01** — `diff` of the five essays against `3503cc0:cli.md:98-251` shows only `###`→`##` heading levels, the doctor-link anchor retarget, and the CI-snippet deixis; no other word changed (`operating-safely.md:10-163`). Holds.
- **S03-02** — `-pack`/`-checkout`/`-arm` rows carry caveat+link, `-pack` targets `operating-safely.md#how-to-keep-assent-advisory`; `--config` row untouched (`cli.md:88-91`); pointer paragraph present. Holds.
- **S03-03** — doctor section retargeted without "above" (`cli.md:114`), walkthrough retargeted (`walkthrough.md:212`); both anchors match real headings; ADR-0009 untouched. Holds.
- **S03-04** — `operating-safely.md` added to `PHRASE_CORPUS` in the same commit as the page (8fffed0); shrunk mutant removes four *present* files leaving 6 < MIN 8, still red; canary token `assent` present; XREV-S03-06 still pins the retained `--config` row (`truthlag_pins_test.sh:504`). No hack/ or .github/ pin references any moved heading. Holds.

Stronger-than-spec / unmentioned: none found. The extraction is strictly a move; no gate was loosened (corpus widened, `not_in_nav` untouched).

## Could not check

- Ran no gate myself (trusted the stated branch-tip matrix); did not execute `mkdocs build`, `exitgate_test.sh`, `check-sanitization.sh` or `TestNoStaleProductClaims`.
- Did not inspect PR190 beyond its README hunks (via local `pr-190`); its DOC-13 chapter list is out of this base and unverified.
- Did not read `docs/decisions.md` / `decisions/evidence/` historical mentions of the moved headings (append-only, out of scope).
