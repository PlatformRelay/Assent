I'll start with the repository state and the files the change touches, then the gates and the audit.
Working tree is at the spec commit (`6d3046f`) on top of `3503cc0` — change not yet executed, so line numbers in the tasks are testable against this tree. Now the CLI reference (the extraction source), the walkthrough link, and repo-wide links into the essay anchors.
The `reviews/` directory inside the change folder contains earlier review legs of this same spec set — out of bounds per my brief; I'll ignore those files entirely and base findings only on repo sources. Legit link hits: only `cli.md:264` (doctor) and `walkthrough.md:212` point into essay anchors — the "two references" claim holds. Now the gates that the spec claims won't break.
Pin claim verified for `truthlag_pins_test.sh`: the only `cli.md` grep is XREV-S03-06 on the `--config` row (stays). Now the phrase corpora in `TestNoStaleProductClaims`, the exitgate, and the other two docs gates.
The exitgate corpus lists `docs/index.md`, `docs/usage/cli.md`, `README.md` explicitly — I need the banned-phrase list and the remaining gates, plus the audit's P2 items and the docs-nav requirements.
`check-sanitization.sh` is a repo-wide hygiene scan (no content pins) — fine. Taskfile confirms floor 91 and the docs targets. But the audit report cited as the change's authoritative origin appears missing — checking that, the docs-nav requirements, and the remaining gates.
The origin audit report cited by the proposal and spec (`data/assent-docs-audit/report.md`) does not exist anywhere — not in the tree, not tracked, not in history. I verified the underlying findings directly instead (index stub, nav order, essay range, README duplication — all true at this tree). Now PR 190's actual hunks (the spec copies its text) and the remaining gate internals.
PR 190's README hunks match the proposal exactly (`:22` E1–E9, `:45–46` `.tf` clause, `:76` `@v0.4.0`). It also touches two more docs files — checking its full file list, and whether its DOC-13 pin scans `docs/index.md` (REQ-DOC2-S01-04's verify clause claims it does).
PR 190's full diff: DOC-13's pin scope includes `docs/index.md` (REQ-DOC2-S01-04's verify clause is real, not vacuous) — but 190 also touches `docs/usage/walkthrough.md`, which T004 edits, contradicting the proposal's "only shared file" claim. One last check: how the exitgate ban consumes its file list.
## Verdict: BLOCK

Tasks as written **would deliver audit P2 items 9–11** — I verified every load-bearing premise against the tree (index stub at 18 lines ✓, Usage nav Install→CLI→Walkthrough at `mkdocs.yml:84-86` ✓, essays exactly at `cli.md:98-251`, five `###` sections ✓, README duplicate at `:107-125` ✓, PR 190's hunks match the proposal's claims ✓, only two links point into essay anchors repo-wide ✓). BLOCK is for a gate-register defect, not scope.

## Findings

- [CRITICAL] S03 moves the trust-model essays onto a published page the exitgate retired-phrase ban never scans — `hack/audit/exitgate_test.sh:272-282`, `spec.md:160-171`
  Failure: `PHRASE_CORPUS` lists files explicitly; `docs/usage/cli.md` (the essays' home today) is listed, the new `docs/usage/operating-safely.md` is not. DOC-08's own header says the corpus exists so "a moved or renamed file would otherwise silently shrink the search" — yet the canary and min-count still pass with the list unchanged, so any future reintroduction of a banned phrase ("not yet implemented", "design fiction", …) on the repo's most claim-laden front-of-house page is invisible to every gate. Verified: `check_retired_phrases` reads only the listed files; nothing else scans the new page.
  Fix: one line — add `docs/usage/operating-safely.md` to `PHRASE_CORPUS` in T004. This is not a truthlag pin, so the "no new gate pins" non-goal (`proposal.md:62-66`) does not apply to it.
  Confidence: 85
- [WARNING] The change's authoritative origin is a dead reference — `proposal.md:4`, `spec.md:8`
  Failure: `data/assent-docs-audit/report.md` is not in the tree, not tracked, never committed, not gitignored. A committed openspec change telling readers the origin report is "the authoritative evidence for every finding" ships unresolvable provenance to a public repo; items 9–11 cannot be re-verified from it by anyone but the author.
  Fix: state the report is workspace-local (as the sanitization denylist does) and inline the three item texts, or commit a sanitized extract.
  Confidence: 95 (absence verified; whether the omission is deliberate is unrecorded)
- [WARNING] The merge-order note's premise is false — `proposal.md:82-84`
  Failure: "the two PRs touch disjoint regions of the only shared file (`README.md`)" — PR 190's diff also hunks `docs/usage/walkthrough.md` (≈6 hunks, incl. a 36-line inserted rule block), `docs/usage/install.md`, `docs/vision.md`, `docs/architecture/c4-container.md`, and T004 edits `walkthrough.md:212`. The hunks are content-disjoint so "either order merges mechanically clean" still holds, but the operator's decision note misstates the shared surface. Related staleness risk: S01's copied text is pinned to an open PR's current hunks; if 190 is amended after T002, the only reconciliation gate (DOC-13 over `docs/index.md`) runs after both land, on main.
  Fix: correct the note to "two shared files (README.md, walkthrough.md); hunks disjoint", and re-diff T002's copy against 190's hunks immediately before merge.
  Confidence: 90
- [WARNING] The extraction strands a prose antecedent the spec forbids fixing — `docs/usage/cli.md:168-171`, `spec.md:160-164`
  Failure: "an operator who edits it and reruns **the CI snippet above** — which passes no `--pack`" refers to the `assent run` usage block at `cli.md:69-71`, which stays in cli.md while the sentence moves to operating-safely.md. REQ-DOC2-S03-01 demands each sentence unchanged and enumerates retargets for `#assent-doctor` links and italics refs only, so the implementer is contractually prevented from repairing the dangling "above".
  Fix: add one exception to REQ-DOC2-S03-01 retargeting this reference to a `cli.md` link, as done for the doctor references.
  Confidence: 85
- [NOTE] REQ-DOC2-S01-01 omits the GitHub comment-only note that T002 copies — `spec.md:78-80` vs `tasks.md:37`
  Failure: a diff review against the requirement would pass with the note dropped from index.md.
  Fix: add it to the REQ's list. Confidence: 80
- [NOTE] Retargeting the doctor link leaves a false "above" — `docs/usage/cli.md:264`
  Failure: "See [What gates approve and merge](#…) above" points to another page after S03 while T004 step 2 changes only the link target.
  Fix: reword to "in *operating safely*". Confidence: 70

Verification underpinning REQ-DOC2-S03-04's measured claim: it is true — truthlag's only `cli.md` grep is XREV-S03-06 (`truthlag_pins_test.sh:500-507`, targets the `--config` row which stays); TestNoStaleProductClaims walks `../../docs` and keeps scanning the moved text — but "reduces to keeping these green" conceals the CRITICAL's silent coverage loss.

## Could not check

- `data/assent-docs-audit/report.md` — absent from repo, so the audit's exact P2 wording (sub-parts beyond the proposal's restatement, e.g. the full text of item 10's deferred half) is unread; findings were verified against the tree instead.
- `task check` / the 90.5%-coverage baseline claim — Taskfile floor 91 verified (`Taskfile.yml:44`); the 90.5% number itself not measured, and the exitgate/tests were read, not run (`readme_smoke_test.sh`, `truthlag_pins_test.sh`, `check-sanitization.sh` verified by reading).
- `task docs-build` not executed (uv toolchain untouched); anchor-fragment behaviour inferred from `mkdocs.yml:115-123` (no anchors key) — compensated by T004's built-HTML anchor grep; the pinned mkdocs version could not be determined.
- `openspec/changes/p5-docs-p2-structure/reviews/` — earlier review legs of this same spec set; out of bounds per brief, not read.
- PR 190 beyond its diff — review state and amendment likelihood; its text is treated as frozen per REQ-DOC2-S01-04's assumption, unverifiable from here.
