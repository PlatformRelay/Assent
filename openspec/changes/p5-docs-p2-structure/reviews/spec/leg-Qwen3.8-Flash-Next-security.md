## Verdict: BLOCK

## Findings
- [CRITICAL] T002's closing instruction ("Copy the README wording, do not reword it") contradicts REQ-DOC2-S01-04, and the failure is deferred past every gate this branch runs.
  Failure: an implementer copies today's `README.md` verbatim into `docs/index.md`, carrying `@v0.1.0` (`README.md:76`) and `E2–E8` (`README.md:22`). All five per-task gates stay green because the DOC-13 version pin exists only in open PR 190; when 190 merges, its DOC-13 scan set is `README.md docs/index.md docs/vision.md …` (verified in 190's diff of `hack/docs/truthlag_pins_test.sh`), so `main` reddens on the *new* file. This is exactly the stale-pin defect S00 exists to remove, re-armed at a new path.
  Fix: make T002.1 say the copied block is PR 190's corrected text (`@v0.4.0`, E1–E9, `.tf` clause) and delete the "do not reword" sentence, which currently overrides the requirement.
  Confidence: 85
- [WARNING] The `--checkout` security disclosures are stripped from the surface where an operator hits the wall.
  Failure: `docs/usage/cli.md:89` keeps only a one-line caveat + link, so the operative obligations for the still-unfixed SEC-01 ("construct `head/` from the MR head commit", "cancel superseded pipelines", `cli.md:227-232`) and the any-symlink-anywhere scope (`cli.md:175-179`) leave the flag table. D-133 (`docs/decisions/decisions.md:140`) and D-139 (`:146`) record the opposite rule: a limitation is documented *only* where the person hits it. An operator reading the `-checkout` row can now satisfy the row without ever seeing that assent never verifies the judged tree is the merged commit.
  Fix: keep the two `--checkout` caveats' operative clauses in the `-checkout` row (link for the prose only); move the essays without their instructions.
  Confidence: 70
- [WARNING] The reference inventory is short by one, and the missed one is invisible to every gate.
  Failure: `docs/adr/0009-execution-modes.md:55` points readers to "`docs/usage/cli.md` §*How to keep assent advisory*" in prose. After T004 the section is gone; ADR-0009 (a nav page) then sends operators to a non-existent section, and `mkdocs build --strict` cannot see it because it is not a link. `tasks.md:67` and `specs/docs-p2/spec.md:145-147` count only two references (the cli.md doctor pointer and `walkthrough.md:212`).
  Fix: add ADR-0009:55 to T004's retarget list.
  Confidence: 90
- [WARNING] Nothing protects the new safety links from anchor rot.
  Failure: `mkdocs.yml:115-123` configures `omitted_files`/`unrecognized_links`/`absolute_links` but no `validation.links`, so mkdocs 1.6 (`docs/requirements-docs.txt:242`) leaves `anchors` at its `info` default — a mistyped `operating-safely.md#known-limitation-the-checkout-is-not-bound…` builds green under `--strict`. `site/` is gitignored (`.gitignore:15`), so REQ-DOC2-S03-03's "Test: the built site/index.html" is a one-off local grep; DOCSNAV-R01 (`openspec/specs/p5-docsnav-site-reachability/spec.md:192`) records this class as OPEN and defers raising it. Result: a safety caveat can link nowhere and CI is silent.
  Fix: add a new `hack/docs/anchor_pins_test.sh` (a new file, so no DOC-nnn numbering race with PR 190) asserting the five anchors exist in the built page and the two retargeted links resolve; wire it into `docs-gates`.
  Confidence: 80
- [WARNING] The proposal's merge-order premise is false: `README.md` is not the only shared file.
  Failure: `proposal.md:81-88` asserts disjoint edits to "the only shared file". PR 190 also edits `docs/usage/walkthrough.md` (`@@ -67,8 +67,44 @@`, +36 lines), which this change edits at `:212` (`tasks.md:67`). If 190 lands first, that hardcoded line is ~36 off and T004.3's line-addressed edit is aimed at the wrong text.
  Fix: reword the note to name both shared files and address the walkthrough edit by its link text, not a line number.
  Confidence: 85
- [WARNING] `proposal.md:83` names the wrong deletion range.
  Failure: "this change deletes `:105–129`" vs `tasks.md:25`/REQ-DOC2-S00-01's `106–125`. Deleting 105–129 removes the surviving "Developers:" line *and* the `task check` block, so the section ends with no task-check block — contradicting REQ-DOC2-S00-01, and `XREV-S03-01`'s `grep -q 'task check   # the full gate'` (`hack/docs/truthlag_pins_test.sh:398`) then reddens.
  Fix: correct the proposal to `106–125`.
  Confidence: 90
- [NOTE] REQ-DOC2-S00-01's named Test cannot fail for its stated condition.
  Failure: `readme_smoke_test.sh` executes whatever bash it finds and asserts only `ran > 0` (`hack/docs/readme_smoke_test.sh:88-118`); the duplicated and de-duplicated README both pass, as does `grep -q`-based DOC-11 (`truthlag_pins_test.sh:170-177`) — the same non-discrimination that let E10's duplication land. The proposal defers the ratchet (`proposal.md:61-66`), so the defect that will redden PR 190 once can recur invisibly.
  Fix: assert the block's occurrence count (e.g. `grep -c 'go install' README.md == 1`) in the same pin sweep that adds DOC-13.
  Confidence: 75

## Could not check
- `data/assent-docs-audit/report.md` is not in the tree, so "delivers the audit's P2 items 9–11" is judged against this change's own restatement of them, not the audit text.
- Did not run `task docs-build`, `task check`, mkdocs, or any gate (read-only run); mkdocs anchor behaviour inferred from 1.6.1 + `mkdocs.yml` config and DOCSNAV-R01, not observed.
- PR 190 read via `gh pr diff` only (hunk headers + pins); its walkthrough/vision hunks not fully read.
- Other reviewers' files under `openspec/changes/p5-docs-p2-structure/reviews/` not read.
- `hack/audit/exitgate_test.sh` retired-phrase values not enumerated; inferred no new phrase risk from README already being in `PHRASE_CORPUS` (`hack/audit/exitgate_test.sh:261-270`).
