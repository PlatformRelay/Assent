## Verdict: CONCERNS

## Findings

- [WARNING] The diff edits `evidence/T005.md`'s table yet leaves an adjacent cell whose quote is provably un-emittable by the script it cites — `openspec/changes/p5-docs-p2-structure/evidence/T005.md:13`
  Failure: the readme-smoke script prints `OK: $ran ... green, $skipped skipped with a stated reason` always (`hack/docs/readme_smoke_test.sh:126`) and is unchanged since `3503cc0` (verified: empty `git log 3503cc0..HEAD --` on it); README is identical between T005's recorded tip `85eb7c8` (the dedup `247187a` is its ancestor — verified) and HEAD, so T005's recorded run could only have printed `3 green, 4 skipped`, never `OK: 7 ... green`. T006.md itself verifies this, then defers the one-cell fix while editing the two rows below it — the change closes W8 (vacuous evidence) while knowingly leaving a false sensor line in the same table.
  Fix: correct the `:13` cell to the literal line, same commit.
  Confidence: 85

- [WARNING] The commit introduces a Goal↔requirement contradiction inside the file it edits — `openspec/changes/p5-docs-p2-structure/specs/docs-p2/spec.md:167-174` vs `:213-218`
  Failure: the Goal now dispositions (c), (d) and (e) as hand-off residuals, but REQ-DOC2-S03-03's normative text still names only ADR-0009; a verifier checking the requirement never checks the p4-e1-s11 record or the D-134 row are untouched and recorded — exactly the check WARNING-5 was filed to force.
  Fix: extend S03-03's disposition clause to name the frozen record and the D-134 row.
  Confidence: 80

- [WARNING] The proposal mirror was left divergent and the divergence was widened — `openspec/changes/p5-docs-p2-structure/proposal.md:52-54`
  Failure: the proposal still says the inventory "is three"; after this commit the spec says five. Same claim, two tracked files of the same change, one false — round-1 flagged the class (three vs four), and the round-2 fix moved only one side, so the deferred state is worse than the one deferred.
  Fix: one clause in `proposal.md:52-54`, or replace the count with "see S03 Goal".
  Confidence: 90

- [NOTE] The added required-surface entry is itself unpinned — `hack/audit/exitgate_test.sh:830-831` vs the `missing-required` mutant at `:1923-1926` (removes only `walkthrough.md`)
  Failure: deleting `docs/usage/operating-safely.md` from the new list entry reddens nothing. Disclosed and deferred; the deferral's ground is real — `hack/audit/README.md:302` pins 38/32 controls per invocation — but the evidence phrase "the new entry's requiredness is graded by the same loop the `missing-required` control covers" grades the mechanism, not the entry.
  Fix: when the control floor next moves, loop the mutant over required entries.
  Confidence: 80

- [NOTE] The round-2 register path committed in the evidence is already wrong — `openspec/changes/p5-docs-p2-structure/evidence/T006.md:133` says `T006/register-round2.md`; the round-2 legs are landing in `reviews/L-tasks/T006-round2/`
  Failure: the "(Pending)" update must also fix the pointer or the register becomes unfindable; round 1's lesson was precisely a dangling review-object reference.
  Fix: point at `T006-round2/register.md` when the pending line is updated.
  Confidence: 70

- [NOTE] Commit message names two of the three changes (corpus requirement, inventory fix); the T005 `-v` record (WARNING-8) is unmentioned — `git log -1 a74076e`. Covered by the evidence file; one logical change otherwise intact.
  Confidence: 60

What I actively verified: all five inventory citations `(a)–(e)` hold verbatim at `3503cc0` (`walkthrough.md:212`, `adr/0009:55`, `p4-e1-s11/README.md:27`, `decisions.md:141`); an independent sweep of both essay titles and both hyphenated slugs over `3503cc0` outside `cli.md`/`operating-safely.md`/`openspec/` found no sixth pointer; the corpus arithmetic (11 listed, 10 present, `quickstart.md` dead, MIN 8; `shrunk` removes 4 → 6 < 8) matches `exitgate_test.sh:262-275,1928-1931`; `bash -n` passes; the addition is a pure tightening — no allow-list, exclusion or baseline widened (corpus membership arrived earlier in `8fffed0`); a misspelled new path cannot pass silently (the live-root green check would red).

## Could not check
- Ran no gate: `exitgate --text-only`, `task docs-build`/`docs-gates`, `go test -run TestNoStaleProductClaims -v` (the `(0.11s)` line), sanitization — green claims verified by reading, not execution (test exists: `cmd/assent/main_help_test.go:106`).
- Whether `hack/audit/README.md:302`'s 38/32 control counts still match HEAD (README outside diff).
- The untracked round-2 legs/register in `T006-round2/` — other reviewers' work, out of bounds.
- Built `site/` anchor presence (S03-03's Test); full exitgate / `task check` (declared baseline-coverage deviation not re-checked).
