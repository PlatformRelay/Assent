Now the surrounding context: README quick-start, current index.md, cli.md essays, mkdocs nav, and the gates the tasks claim.
The cited origin report is missing from the working tree. Checking git history and the referenced gates:
The origin report is not in the repo at all — noting that. Now verifying the gates the tasks claim, and the DOCSNAV requirements they lean on:
Now the remaining gate details: exitgate phrase corpus over cli.md/index.md, TestNoStaleProductClaims, the smoke test's block parsing, and the DOCSNAV spec requirements:
macOS sed syntax issue — using the read tool instead:
The stale-claims test walks `docs/` but explicitly not repo-root markdown — that matters for T002's copy. Checking the claim list, the exitgate phrase list, the smoke test, and the DOCSNAV spec:
Two things left to pin down: the ADR-0009 cross-reference the spec's retarget list may have missed, and what `task docs-gates` actually runs:
One last check — whether any truthlag pin reads `docs/index.md` or counts README caveat occurrences (T001/T002 surface):
Final link-target existence check for T002's named targets:
## Verdict: CONCERNS

Items 9–11 as restated are delivered by S01/S02/S03, and S00's precondition is sound: I verified the essays at `cli.md:98–251`, the XREV-S03-06 pin targets only the `--config` row that stays (`hack/docs/truthlag_pins_test.sh:500–507`), the exitgate phrase corpus greps both `docs/index.md` and `docs/usage/cli.md` (`hack/audit/exitgate_test.sh:262–273`) but no essay sentence matches any retired phrase, `readme_smoke_test.sh` extracts bash blocks by heading+fence (content-driven, so the S00 deletion keeps it green and the dup currently executes twice), the nav gate makes page+nav same-commit correct (T004 step 4), and all T002 link targets exist on disk. The trust-model essays — this change's security payload — move verbatim with caveat+link rows at the flag table, so the operator-facing safety posture is preserved. Two warnings remain.

## Findings

- [WARNING] The spec's "the two cross-page/in-page references" is false: a third published nav page points at a moved essay by page+section — `docs/adr/0009-execution-modes.md:55` — openspec/changes/p5-docs-p2-structure/specs/docs-p2/spec.md:145
  Failure: after S03, "docs/usage/cli.md §*How to keep assent advisory* states this" is stale (the section lives in operating-safely.md); it is prose, not a link, so `mkdocs build --strict`, the phrase corpus and every pin stay green while a published ADR carries a wrong locator — the exact truth-lag species D-134 exists to catch.
  Fix: add ADR-0009:55 to S03's retarget list and T004 step 3 (or record in the spec why an ADR's live pointer is deliberately left).
  Confidence: 82
- [WARNING] S01's copied wording is sourced from PR 190's hunks, which are neither in this branch's base nor pinned verbatim in the spec — spec.md:59–60, tasks.md:44
  Failure: PR 190 is open and mutable; if its E1–E9/`.tf`/`@v0.4.0` text is amended while this branch is open, T002's commit bakes in a stale copy and the two surfaces diverge, reddening DOC-13's post-merge scan of `docs/index.md` (spec.md:101–102) with no task owning the reconciliation.
  Fix: at T002, snapshot PR 190's exact hunks into the story's evidence file at a pinned head SHA and re-diff against the live PR before merge.
  Confidence: 70
- [NOTE] REQ-DOCSNAV-S01-04's mechanical half never runs at T002 — tasks.md:13–21
  Failure: `bash hack/check-sanitization.sh` is absent from the per-task gate block; T002's copied text is committed, and the tree-wide scan only runs at T004 step 6 (plus CI `verify.yaml`), so a hit can sit in two commits before anything fires. Impact is low — the copied text is already published — but the gate is the process claim.
  Fix: add the one-line script to the per-task gate set; it is tree-wide and cheap.
  Confidence: 75
- [NOTE] S03-02 requires caveat+link on the retained flag rows but never says the rows' existing "see *X* below" italics are replaced; S03-01's "kept verbatim" applies to essays only — spec.md:168–172, docs/usage/cli.md:89,91
  Failure: an implementer keeping the row text intact leaves "see *Symlinks in the checkout tree* below" pointing at a section that is now on another page, with a link appended — a doubled, half-stale caveat.
  Fix: one sentence in S03-02: the rows' "below" wording becomes the link text.
  Confidence: 60
- [NOTE] T005's "full docs-relevant gate set" is the tasks' 5-command block, which omits `check-migration-invariants.sh` that `task docs-gates` runs (Taskfile.yml:236–242) — tasks.md:13–21,78–84
  Failure: T005 under-runs the spec's own REQ-DOC2-S03-04 verify ("`task docs-gates`"), so the branch-tip evidence is weaker than the requirement it cites.
  Fix: T005 step 1 runs `task docs-gates` instead of the ad-hoc five commands.
  Confidence: 70

## Could not check

- `data/assent-docs-audit/report.md` — absent from the working tree and from git history; items 9–11 are taken from the proposal's restatement, and the "re-verified at 3503cc0" claims are unverifiable here.
- PR 190's actual hunks and DOC-13's scan surface (the PR is not in this branch's base; only its existence was confirmed).
- Any gate execution (`task docs-build`, the four gate scripts, `go test ./cmd/assent`, the baseline coverage-red claim at `3503cc0`) — this session is read-only; every "green" statement above is read off the gate source, not a run.
