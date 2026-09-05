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
`.github/workflows/docs.yaml`.

**Non-goals** (fenced): removing any page from the site (`exclude_docs` is rejected — see
REQ-DOCSNAV-S01-02); editing page bodies; `Taskfile.yml` wiring (`docs-build` is not a `check:`
stage today and this epic does not change that); `hack/**`; the theme; raising
`validation.anchors` (a different defect class — recorded as a residual below).

**ADRs / decisions**: D-170 (this epic). Prior art: D-103 / E9-S08 (product-only nav trim),
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

## Residual (not this epic)

| ID | Item | Status | Notes |
| --- | --- | --- | --- |
| DOCSNAV-R01 | `validation.anchors` defaults to `info`, the same invisibility class this epic closes for `omitted_files`. One real broken anchor exists today (`planning/spikes/spike-secure-setup.md` → `#setup-walkthrough-draft--clean-room-runner`) and builds green under `--strict` | **OPEN** | Raising it to `warn` requires fixing that anchor first; a separate lane, because it gates a different property (intra-page targets) than site reachability |
