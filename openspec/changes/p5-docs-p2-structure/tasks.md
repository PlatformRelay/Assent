# P5-DOC2 — tasks

Docs-only vertical slices, dependency-ordered. One logical change per commit,
`:gitmoji: type(scope): summary`. No failing-test-first here: the gates are the tests, and
each task names the gate set it must keep green before its commit.

**Baseline honesty:** `task check` is red at `3503cc0` on the coverage stage (90.5% <
91% floor, D-010/D-128 — measured before this change's first commit; almost certainly the
E10 merge's new `internal/forge/github` at 84.1%). That stage is unreachable by docs work
and is NOT this change's to fix. Each task therefore runs the docs-relevant gate set below
instead of the full `task check`, and the branch's final report states the baseline red.

Per-task gate set (all must be green before each commit):

```
task docs-build
task docs-gates
go test ./cmd/assent -run TestNoStaleProductClaims
```

(`task docs-gates` runs `readme_smoke_test.sh`, `truthlag_pins_test.sh`,
`example_format_inventory_test.sh` and `check-migration-invariants.sh` — Taskfile.yml's
`docs-gates` block.)

## T001 — S00: de-duplicate the README quick start

- [x] — closed 2026-10-07, evidence: evidence/T001.md

1. Delete `README.md` lines 106–125 (the repeated go-install caveat, lint/test prose and
   block, "No repo of your own yet?" paragraph, second "Developers:" line), leaving the
   section to read install → caveat → lint/test → sample-repo → forge selection → GitHub
   comment-only → "Developers: gates live in the Taskfile" → task-check block. Touch
   nothing else in the file.
2. Verify: the per-task gate set; `git diff README.md` shows only deletions in that range.
3. Commit: `:bug: fix(readme): drop the quick-start block the E10 merge duplicated`

## T002 — S01: index.md front door

- [x] — closed 2026-10-07, evidence: evidence/T002.md

1. Rewrite `docs/index.md`: keep the hero block and H1; replace the existing two-sentence
   intro with the README's intro paragraph; add the status banner, Why, How-it-works
   (mermaid + statelessness paragraph) and Quick start (install, caveat, lint/test,
   sample-repo, forge selection, GitHub comment-only note). Keep "Start here" (vision,
   walkthrough, ADR index, C4, decision log) below the new content; move meta-plan +
   open-questions links into a "Contributing" tail.
2. **Copy source is this branch's README with PR 190's three P0 hunks applied** — both
   sources alone are wrong: the branch's README still says `E2–E8`/`@v0.1.0`, and
   PR 190's README is based on the pre-E10 tree (its GitHub-adapter row says **Planned**;
   it has no forge-selection block or GitHub comment-only note at all). Concretely: copy
   the branch's README wording for the banner, Why, How-it-works and quick-start text,
   then apply PR 190's three corrections — `:22` `E2–E8`→`E1–E9`, `:45–46` the `.tf`
   opaque clause ("`.tf` files are governed but opaque (whole-file REVIEW, never a
   partial parse)"), `:76` `@v0.1.0`→`@v0.4.0` (`git show pr-190:README.md` shows the
   corrected wordings; `git fetch origin pull/190/head:pr-190` if the ref is missing).
   Keep the branch's E10-era quick-start content (forge selection, GitHub comment-only
   note) as-is. Links are adjusted to docs-relative form (`adr/0015-...`,
   `architecture/c4-context.md`, `usage/install.md`, `planning/open-questions.md`,
   `api-stability.md`).
3. Verify: the per-task gate set; every link resolves (mkdocs strict); REQ-DOC2-S01-04's
   wording check against the branch README with the three hunks applied.
4. Run `bash hack/check-sanitization.sh` and record the result, with the REQ-DOCSNAV-S01-04
   sanitization read, in the story's evidence file.
5. Commit: `:memo: docs(index): carry the README's story on the site home`

## T003 — S02: Usage nav order

- [x] — closed 2026-10-07, evidence: evidence/T003.md

1. In `mkdocs.yml`, reorder Usage to Install, Walkthrough, CLI reference. Nothing else.
2. Verify: the per-task gate set.
3. Commit: `:memo: docs(nav): order Usage install → walkthrough → CLI reference`

## T004 — S03: operating-safely extraction

- [x] — closed 2026-10-07, evidence: evidence/T004.md

1. Create `docs/usage/operating-safely.md`: a short intro (what the page covers, links
   back to `cli.md` and `walkthrough.md`), then the five cli.md essays
   (`cli.md:98–251`) moved verbatim as `##` sections, subject only to the navigation-only
   transformations the spec's S03 Goal names: `#assent-doctor` anchors retargeted to
   `cli.md#assent-doctor`; the advisory essay's "reruns the CI snippet above" deixis
   retargeted to name and link the `assent run` invocation in the CLI reference; the
   `../adr/*` links kept (same-directory relative, still valid); the italics "see *X*
   below" cross-references kept.
2. In `cli.md`: delete the five sections; replace the `-arm` row's trailing "see *What
   gates approve and merge* below" and the `-checkout` row's two italic tails with links
   to `operating-safely.md#…` (keeping each row's own caveat text); give the `-pack` row
   a rollout-control caveat + link to `operating-safely.md#how-to-keep-assent-advisory`
   (NOT a bare in-page anchor — that heading leaves `cli.md` in this same change); leave
   the `--config` row (XREV-S03-06) untouched; add a pointer paragraph after the run exit
   codes linking to the new page; retarget the doctor section's "What gates approve and
   merge" link and drop its now-false "above".
3. In `docs/usage/walkthrough.md:212`: retarget
   `[How to keep assent advisory](cli.md#how-to-keep-assent-advisory)` to
   `operating-safely.md#how-to-keep-assent-advisory`.
4. In `mkdocs.yml`: add `Operating safely: usage/operating-safely.md` after CLI reference
   in Usage (REQ-DOCSNAV-S01-01 — same commit as the page, or `task docs-build` reddens).
5. In `hack/audit/exitgate_test.sh`: add `docs/usage/operating-safely.md` to
   PHRASE_CORPUS (after `docs/usage/install.md`), in the same commit as the page
   (REQ-DOC2-S03-04; the shrunk-corpus mutant stays red: 10−4 present of 11 < MIN 8).
6. Verify: the per-task gate set; `bash hack/audit/exitgate_test.sh`; grep the built
   `site/usage/operating-safely/index.html` for the five section anchors; confirm the two
   retargeted links hit existing anchors.
7. Record the REQ-DOCSNAV-S01-04 sanitization read + `bash hack/check-sanitization.sh`
   result in the story's evidence file.
8. Commit: `:memo: docs(usage): extract the cli.md trust-model essays into operating-safely`

## T005 — branch tip verification (no commit of its own unless a fix is needed)

1. Run the full docs-relevant gate set at the branch tip, plus
   `bash hack/audit/exitgate_test.sh` (the phrase corpus grades `docs/usage/cli.md` and
   `docs/index.md`) and `go test ./cmd/assent -run TestNoStaleProductClaims`.
2. If any gate reddens, classify (this change vs pre-existing baseline) before touching
   anything; the pre-existing coverage-below-floor failure is baseline, not this change's.
3. Report the gate matrix in the branch review evidence.

## T005 — branch tip verification

- [x] — closed 2026-10-07, evidence: evidence/T005.md (gate matrix all green at `85eb7c8`; no fix needed)
