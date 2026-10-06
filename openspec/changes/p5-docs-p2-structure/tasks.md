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
bash hack/docs/truthlag_pins_test.sh
bash hack/docs/readme_smoke_test.sh
bash hack/docs/example_format_inventory_test.sh
go test ./cmd/assent -run TestNoStaleProductClaims
```

## T001 — S00: de-duplicate the README quick start

1. Delete `README.md` lines 106–125 (the repeated go-install caveat, lint/test block,
   "No repo of your own yet?" paragraph, second "Developers:" line), leaving the section
   to read install → caveat → lint/test → sample-repo → forge selection → GitHub
   comment-only → "Developers: gates live in the Taskfile" → task-check block. Touch
   nothing else in the file.
2. Verify: the per-task gate set; `git diff README.md` shows only deletions in that range.
3. Commit: `:bug: fix(readme): drop the quick-start block the E10 merge duplicated`

## T002 — S01: index.md front door

1. Rewrite `docs/index.md`: keep the hero block and H1; add the README's status banner
   (E1–E9, `.tf` opaque clause per PR 190), Why, How-it-works (mermaid + statelessness
   paragraph), Quick start (install, caveat, lint/test, sample-repo, forge selection,
   GitHub comment-only note) — every link docs-relative (`adr/0015-...`,
   `architecture/c4-context.md`, `usage/install.md`, `planning/open-questions.md`,
   `api-stability.md`); keep "Start here" (vision, walkthrough, ADR index, C4, decision
   log) below the new content; move meta-plan + open-questions links into a
   "Contributing" tail. Copy the README wording, do not reword it.
2. Verify: the per-task gate set; every link resolves (mkdocs strict); REQ-DOC2-S01-04's
   wording check against PR 190's README hunks.
3. Record the REQ-DOCSNAV-S01-04 sanitization read in the story's evidence file.
4. Commit: `:memo: docs(index): carry the README's story on the site home`

## T003 — S02: Usage nav order

1. In `mkdocs.yml`, reorder Usage to Install, Walkthrough, CLI reference. Nothing else.
2. Verify: the per-task gate set.
3. Commit: `:memo: docs(nav): order Usage install → walkthrough → CLI reference`

## T004 — S03: operating-safely extraction

1. Create `docs/usage/operating-safely.md`: one-line intro (what the page covers, links
   back to `cli.md` and `walkthrough.md`), then the five cli.md essays
   (`cli.md:98–251`) moved verbatim as `##` sections; retarget `#assent-doctor`
   in-text anchors to `cli.md#assent-doctor`; keep the `../adr/*` links (same-directory
   relative, still valid); keep the italics "see *X* below" cross-references.
2. In `cli.md`: delete the five sections; give the `-arm` row its caveat + link to
   `operating-safely.md#what-gates-approve-and-merge`, the `-checkout` row links to the
   symlink and checkout-less sections, the `-pack` row its rollout-control caveat + link
   to `#how-to-keep-assent-advisory`; leave the `--config` row (XREV-S03-06) untouched;
   add a pointer paragraph after the run exit codes linking to the new page; retarget the
   doctor section's "What gates approve and merge" link.
3. In `docs/usage/walkthrough.md:212`: retarget
   `[How to keep assent advisory](cli.md#how-to-keep-assent-advisory)` to
   `operating-safely.md#how-to-keep-assent-advisory`.
4. In `mkdocs.yml`: add `Operating safely: usage/operating-safely.md` after CLI reference
   in Usage (REQ-DOCSNAV-S01-01 — same commit as the page, or `task docs-build` reddens).
5. Verify: the per-task gate set; grep the built `site/usage/operating-safely/index.html`
   for the five section anchors; confirm the two retargeted links hit existing anchors.
6. Record the REQ-DOCSNAV-S01-04 sanitization read + `bash hack/check-sanitization.sh`
   result in the story's evidence file.
7. Commit: `:memo: docs(usage): extract the cli.md trust-model essays into operating-safely`

## T005 — branch tip verification (no commit of its own unless a fix is needed)

1. Run the full docs-relevant gate set at the branch tip, plus
   `bash hack/audit/exitgate_test.sh` (the phrase corpus grades `docs/usage/cli.md` and
   `docs/index.md`) and `go test ./cmd/assent -run TestNoStaleProductClaims`.
2. If any gate reddens, classify (this change vs pre-existing baseline) before touching
   anything; the pre-existing coverage-below-floor failure is baseline, not this change's.
3. Report the gate matrix in the branch review evidence.
