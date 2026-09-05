# P5-DOCSNAV — Published-site reachability of `docs/**`

**Epic ID / REQ prefix:** `DOCSNAV` / `REQ-DOCSNAV-S0n-nn`.

**Problem**: a page under `docs/` that is in no `nav:` entry builds green and ships. `mkdocs.yml`
set `validation.omitted_files: info`, and `--strict` (`.github/workflows/docs.yaml:48`, the docs
gate) escalates WARNING and above only — so the omitted-files check was structurally invisible.
Measured on `main` at `20c80cb`: **56 of 66 `docs/**.md` files were absent from the nav** and
`mkdocs build --strict` was green over all 56. Two different things were hiding behind that one
number, and nothing in the repo could tell them apart:

- **25 product pages that simply fell out** — all 22 ADRs, `architecture/policy-profiles.md`, and
  both frozen `contracts/p3-e5-publication-protocol/` documents. E9-S08 (D-103) trimmed the nav to
  product pages and, in trimming `planning/`, never re-added these; the ADR index page
  (`adr/README.md`) is in the nav and links every ADR, so they were link-reachable but had no
  place in the site's navigation at all.
- **31 pages that are deliberately not product docs** — `planning/**` (27, kept out by
  GUIDELINES.md "Repository discipline" and E9-S08/D-103), the two `assets/**` maintainer
  runbooks, `decisions/evidence/**`, and the ADR authoring template.

The defect is therefore not "56 pages are missing from the nav". It is that **deliberate exclusion
and accidental omission were indistinguishable, to a reader and to CI alike**, and that the check
which could have told them apart was pinned below the level `--strict` acts on. Two spec rows
already record the limitation in prose without closing it (REQ-DEM-S01-03, REQ-DEM-S02-01: "nav
linkage is therefore **manual review** unless a later change bumps `omitted_files` to `warn`").

**Invariant**: a page under `docs/` that is not reachable from the published site cannot sit there
unnoticed. Either it is in `nav:`, or it is named by an explicit, documented exclusion whose
justification is in the decision log — and anything that is neither fails the docs gate.

**Scope**: (S01) partition every `docs/**.md` into nav pages and a graded exclusion list, arm
`validation.omitted_files: warn` so `--strict` catches the residue, and guard the gate itself in
`.github/workflows/docs.yaml`; (S02) give that gate local pre-merge evidence by making the strict
build a `task check` stage.

**Non-goals** (fenced): removing any page from the site (`exclude_docs` is rejected — see
REQ-DOCSNAV-S01-02); editing page bodies; `hack/**`; the theme; raising `validation.anchors` (a
different defect class — recorded as a residual below).

> **Amended 2026-09-05 (D-174).** This list also fenced off "`Taskfile.yml` wiring (`docs-build` is
> not a `check:` stage today and this epic does not change that)", quoted here verbatim because
> **DOCSNAV-S02 below reverses it**. Leaving the wiring out is what left the S01 invariant with
> **zero local pre-merge evidence**: `task check` was green over a `docs/**.md` page in neither
> list, and `main` reddened on push, taking the Pages deploy with it. The fence was written when
> S01's gate did not exist yet; once it did, "CI-only" was the defect, not the scope.

**ADRs / decisions**: D-170 (S01), D-174 (S02). Prior art: D-103 / E9-S08 (product-only nav trim),
D-044 (MkDocs pipeline), GUIDELINES.md "Repository discipline".

---

## DOCSNAV-S01 — Nav completeness is gated, exclusions are explicit `[autonomous]`

**As a** reader of the published docs **I want** every product page to appear in the site
navigation **so that** a page is not effectively unpublished by being unreachable, and **as a**
maintainer **I want** the build to red when a new page is neither navigated nor deliberately
excluded, **so that** the next such page cannot go unnoticed for another 56.

**REQ-DOCSNAV-S01-01** — Every `docs/**.md` file is either an entry in `mkdocs.yml`'s `nav:` or
matched by a pattern in `mkdocs.yml`'s `not_in_nav:`; the two sets are disjoint from each other's
intent and jointly cover the tree.

- Given the repository as committed, when `mkdocs build --strict` runs, then it exits 0 and emits
  no `pages exist in the docs directory, but are not included in the "nav" configuration` warning.
- Given a new page created under `docs/` outside every `not_in_nav:` pattern and absent from
  `nav:`, when `mkdocs build --strict` runs, then it aborts in strict mode naming that page.
- Test: `mkdocs.yml`
- Verify: `task docs-build` (`mkdocs build --strict`); the adversarial half is the workflow guard
  pinned by REQ-DOCSNAV-S01-03, which performs exactly that creation on every docs CI run
- Level: L1

**REQ-DOCSNAV-S01-02** — The exclusion list is `not_in_nav:` (pages stay built and link-reachable),
never `exclude_docs:` (pages removed from the site). Rationale, recorded because it is the
non-obvious half of the decision: 7 published pages carry 19 links into `docs/planning/**`, 10 of
them from `docs/decisions/decisions.md`. Dropping those pages out of the build would turn every one
of those links into a `--strict` failure and force edits to the decision log's existing rows.

- Given `mkdocs.yml` as committed, when it is read, then `exclude_docs` is absent and every
  excluded area is still present under `site/` after a build.
- Test: `mkdocs.yml`
- Verify: `! grep -q '^exclude_docs' mkdocs.yml && task docs-build && test -d site/planning`
- Level: L0

**REQ-DOCSNAV-S01-03** — The nav gate has its own guard. `validation.omitted_files` cannot be set
above `warn` (mkdocs rejects `error` for that key), so the gate bites only under `--strict`;
`.github/workflows/docs.yaml` therefore carries a step that (a) pins the `Build site` step to
`mkdocs build --strict` and (b) creates a page in neither `nav:` nor `not_in_nav:` and asserts the
strict build fails *naming that page*.

- Given the workflow as committed, when the guard step runs, then it reports the gate armed and
  leaves no probe file behind.
- **Adversarial**: given `omitted_files` lowered to `info`, or `not_in_nav` widened to `**`, or
  `--strict` removed from the `Build site` step, when the guard step runs, then it fails with a
  message naming which disarmament it detected. All three were verified RED before this row landed.
- Test: `.github/workflows/docs.yaml`
- Verify: the `Prove the nav-completeness gate can fail (D-170)` step on any `docs`-workflow run
- Level: L1

**REQ-DOCSNAV-S01-04** — Every page newly added to `nav:` was read in full against AGENTS.md hard
rule 1 (no employer names, internal system names, internal policy content, or verbatim private
material) before being exposed in the site navigation, and the repository's own sanitization gate
passes over the tree.

- Given the 25 newly navigated pages, when reviewed, then none carries employer or internal system
  names; the examples are generic (`platform/orders-service`, `topic-registry:orders.events.v1`)
  and the only third-party names are public products (GitLab, GitHub, Kyverno, OPA, cel-go,
  Keycloak, Entra ID).
- Test: `docs/adr/*.md`, `docs/architecture/policy-profiles.md`,
  `docs/contracts/p3-e5-publication-protocol/*.md`
- Verify: `bash hack/check-sanitization.sh` (mechanical half) + the manual read recorded in D-170
  (the half no predicate decides)
- Level: L0

**Definition of done**: strict build green with 25 more pages in the nav; no broken internal link
in `site/` (measured: 3 816 internal links across 66 pages, 0 broken); no duplicate nav label or
page `<title>`; the guard step red in all three disarmament states.

---

## DOCSNAV-S02 — The nav gate has local pre-merge evidence `[autonomous]`

**As a** maintainer **I want** the nav-completeness gate to run in `task check` **so that** an
unlisted page reds on my machine, before the commit, rather than on `main` after the push.

S01 armed the gate in `.github/workflows/docs.yaml` only. `task docs-build` is not a `check:`
stage, so the S01 invariant had **no local pre-merge evidence at all**: `task check` — the
per-commit precondition this repository grades everything else against — was green over a page in
neither `nav:` nor `not_in_nav:`, and the first signal was a red `docs` workflow on `main`, which
also fails the Pages deploy. This is the D-124 species ("a gate invoked by nothing is not a gate")
in its weaker form: a gate invoked only where it is too late to act on cheaply.

**REQ-DOCSNAV-S02-01** — `docs-build` (`mkdocs build --strict`) is a stage of `Taskfile.yml`'s
`check:` list, and is pinned in `CHECK_STAGES` in `hack/audit/exitgate_test.sh` **in the same
commit**. The gate's implementation is not duplicated locally: the local stage and the CI `Build
site` step run the same `mkdocs build --strict` against the same `mkdocs.yml`, so there is one
implementation of nav completeness and it cannot skew.

- Given a `docs/**.md` page in neither `nav:` nor `not_in_nav:`, when `task check` runs, then it
  exits non-zero at the `docs-build` stage with a message naming that page.
- Given a page under a `not_in_nav:` pattern (for example `docs/planning/**`), when `task check`
  runs, then the `docs-build` stage is green — the stage grades *unlisted*, not *new*.
- Given `- task: docs-build` deleted from `check:` **or** `docs-build` deleted from
  `CHECK_STAGES`, when `hack/audit/exitgate_test.sh` runs, then it fails: the two lists must be
  *equal*, so neither half can move alone.
- Test: `Taskfile.yml`, `hack/audit/exitgate_test.sh`
- Verify: `mise exec -- task check` (the `docs-build` stage). Deliberately no stage count or
  ordinal: `CHECK_STAGES` is the single graded source, and every prose restatement of it in this
  repository has gone stale (D-174).
- Level: L1

**Cost, measured rather than assumed** (2026-09-05, this worktree): the stage itself is **~1.7 s**
after `docs-install`, whose `uv venv` + `uv pip install --require-hashes` is cached by go-task's
`sources:`/`generates:` and re-runs only when `docs/requirements-docs.txt` changes. It is the one
`check:` stage that is **not** offline on a cold machine — the first run downloads the pinned,
hash-checked docs toolchain. That was accepted deliberately: the alternative, a bash
re-implementation of mkdocs' `nav:`/`not_in_nav:` matching, would be a second implementation of the
property and free to drift from the one CI grades, which is the defect this epic exists to remove.
`.github/workflows/verify.yaml`'s `release-exitgate` job already installs Python and `uv` for
exactly this task, so the `task check` it runs there needs no new setup.

**Two costs this story imposes, stated rather than discovered.** (1) **`task check` now requires
`uv` and Python 3.12 on `PATH`**, through `docs-install`. That is a new hard tool prerequisite for
the per-commit gate, and it matters more than it looks because `mise.toml` is untracked: a
contributor whose environment supplies Go and `golangci-lint` but not `uv` now fails `check:` at
`docs-build` on a tree with nothing wrong in it. (2) **The stage grades the WORKING TREE, not the
commit.** `mkdocs build --strict` reads `docs/` as it is on disk, so an *untracked, in-progress*
`docs/**.md` page in neither list reds `task check` and therefore blocks every unrelated commit
until it is listed — the reviewer's probe demonstrated exactly that, and the shared checkout's
in-progress `docs/adr/0022-container-image-distribution.md` is in neither list today (the nav
carries ADR-0001..0021), so its author's next `task check` goes red the moment this lands. That is
the gate working — it is the same red they would otherwise have taken on `main`, moved earlier and
made cheaper — but the unblocking move must be published, not guessed: add the page's `nav:` row if
it is product documentation, or a justified `not_in_nav:` glob if it is deliberately not, which is
the publication decision REQ-DOCSNAV-S01-02 exists to force; for a page that is genuinely scratch,
keep it outside `docs/` until it is ready. No third option is offered on purpose: silencing the
page by widening `not_in_nav:` to swallow the tree is the disarmament REQ-DOCSNAV-S01-03's guard
already reds on.

**Known residual, named rather than implied**: the only thing that reds when `- task: docs-build`
is deleted from `check:` is `hack/audit/exitgate_test.sh`'s `CHECK_STAGES` equality assertion,
which reaches CI **only** through the `release-exitgate` job — and that job carries
`if: github.event_name != 'pull_request'`. So deleting the stage is invisible on a pull request:
the RELSE-08 visibility class, unchanged by this story and not newly introduced by it (every other
`check:` stage sits behind the same pin). Tracked as `DOCSNAV-R02` below.

---

## Residual (not this epic)

| ID | Item | Status | Notes |
| --- | --- | --- | --- |
| DOCSNAV-R01 | `validation.anchors` defaults to `info`, the same invisibility class this epic closes for `omitted_files`. One real broken anchor exists today (`planning/spikes/spike-secure-setup.md` → `#setup-walkthrough-draft--clean-room-runner`) and builds green under `--strict` | **OPEN** | Raising it to `warn` requires fixing that anchor first; a separate lane, because it gates a different property (intra-page targets) than site reachability |
| DOCSNAV-R02 | Deleting a `check:` stage is graded **only** by `CHECK_STAGES` in `hack/audit/exitgate_test.sh`, whose sole CI caller is the `release-exitgate` job, guarded `if: github.event_name != 'pull_request'` — so a PR that drops `- task: docs-build` (or any other stage) merges green and reds `main` afterwards | **OPEN** | Pre-existing and repo-wide, not specific to `docs-build`; it is the RELSE-08 class the AUD2-S05 and GATES3 lanes closed for individual gates by making each gate assert its own stage wiring. The general fix is a PR-visible assertion that the `check:` list equals `CHECK_STAGES`; D-174 declined to bolt that onto `hack/audit/aud2_exitgate_test.sh`, whose REQs are the 2026-08-18 remediations, rather than invent a scope for it |
