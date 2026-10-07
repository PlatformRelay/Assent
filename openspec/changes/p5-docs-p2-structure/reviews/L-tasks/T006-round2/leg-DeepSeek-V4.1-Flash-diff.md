## Verdict: CONCERNS

## Findings
- [WARNING] The change adds (d) and (e) to the S03 Goal's disposition set but leaves the normative requirement dispositioning only ADR-0009, so Goal and requirement now disagree on which pointers are left untouched — `openspec/changes/p5-docs-p2-structure/specs/docs-p2/spec.md:218` vs `spec.md:163-174`
  Failure: a verifier executing REQ-DOC2-S03-03 checks only `cli.md`/`walkthrough.md`/ADR-0009; the p4-e1-s11 record (d) and the D-134 row (e) are never confirmed unchanged, which is exactly the class of silent-inventory defect this task closes.
  Fix: extend REQ-DOC2-S03-03's last sentence to name the frozen p4-e1-s11 record and the D-134 row alongside ADR-0009.
  Confidence: 75
- [WARNING] Commit claims "correct the reference inventory", but the same change's `proposal.md` mirror still counts the inventory as "three" — `openspec/changes/p5-docs-p2-structure/proposal.md:52-54`
  Failure: a reader taking proposal.md as the change summary gets the count (three) the branch register rated false; the spec says five. Two tracked files of one change diverge.
  Fix: mirror the five-pointer sentence into proposal.md, or replace the restatement with a pointer to the spec.
  Confidence: 90 (out of named scope; T006.md records it as deferred to the orchestrator — disclosed, not actioned)
- [NOTE] `evidence/T005.md`'s header asserts every row was run at tip `85eb7c8` by the orchestrator, but the edited pre-release row is a T006-tree run — `openspec/changes/p5-docs-p2-structure/evidence/T005.md:4` vs `:14`
  Failure: the matrix now mixes two tree states under a header claiming one; a reader reproducing at `85eb7c8` gets a different result for that row.
  Fix: state the tree/sha in the row (partly done) or amend the header to say the pre-release row is a later re-run.
  Confidence: 85
- [NOTE] The newly added required-surface entry is itself unpinned: the `missing-required` control still removes only `walkthrough.md`, so deleting `docs/usage/operating-safely.md` from the list reddens nothing — `hack/audit/exitgate_test.sh:831`, `:1923`
  Failure: a future edit dropping the added line stays green; the tightening this commit lands is not protected against its own reversal. (True of the other four entries too — not a regression.)
  Fix: loop the `missing-required` mutant over each required entry, or add one mutant that drops `operating-safely.md`.
  Confidence: 60 (that this is worth fixing now; 90 that it is unpinned)
- [NOTE] Register CRITICAL-3's second half (corpus headroom: 11 entries, MIN=8) is left unaddressed and not listed among the "Findings closed" residuals — `hack/audit/exitgate_test.sh:275`
  Failure: deleting two non-required surfaces keeps the phrase sweep green. Task text explicitly forbade touching MIN, so this is a genuine residual rather than an error, but T006.md does not record it as deferred.
  Fix: add one line to T006.md's deferred list naming the MIN-headroom residual for the orchestrator.
  Confidence: 80

## Could not check
- Ran no gate: `task docs-build`, `task docs-gates`, `exitgate_test.sh --text-only`, `go test ... TestNoStaleProductClaims`, sanitization — accepted from T006/T005 evidence. Only `bash -n` (syntax) and file reads were executed.
- The committed `evidence/T006.md` claims a `-v` PASS at the T006 tree; not re-run.
- Rendered `site/` anchor slugs for `operating-safely.md` (REQ-DOC2-S03-03's Test).
- The other round-2 leg's report (read-only run; intentionally not read).
- Verified the inventory sweep independently by grep (titles + `cli.md#` anchors outside the change dir): found no sixth pointer, matching T006's claim.
