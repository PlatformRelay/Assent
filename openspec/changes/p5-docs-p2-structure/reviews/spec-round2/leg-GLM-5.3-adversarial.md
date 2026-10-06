Now reading the surrounding context the spec set depends on — the actual files the tasks would touch.
Spec-set claims confirmed so far (index.md stub, nav order, README duplication at exactly the claimed lines). Now the extraction target and the gates it must keep green.
cli.md matches the spec's line inventory exactly. Now the remaining touched files and the gate scripts.
The shrunk-corpus mutant (exitgate_test.sh:1926–1929) deletes 4 members; with MIN=8 the arithmetic holds after the addition (11−4=7<8). Now the pins, Taskfile, and PR 190's actual diff.
The local `pr-190` ref shows **more than three hunks** against this branch's README — including the maturity-table row and the dedup itself. Let me establish PR 190's true diff base.
PR 190's own diff is indeed three README hunks — but the diff against **this branch's** README is four regions, and `pr-190`'s README **lacks the E10 forge-selection and comment-only blocks entirely** (pre-E10 base). That collides with S01-01's content list. Let me pin the remaining merge-order claims.
Merge-order note verified for walkthrough (base tree identical to branch's, hunks disjoint around :212). Now the remaining sensor checks: what the smoke test executes, what `TestNoStaleProductClaims` scans, and whether any reference into the essays was missed.
The Go pin scans all `docs/**` markdown for stale pre-release claims — new pages are clean. Last context checks: the audit origin, tags for the `@v0.4.0` claim, and PR 190's DOC-13 pin scope.
## Verdict: BLOCK

Round-1 C1 (corpus hole) is genuinely closed — verified against the real mutant. C2 (copy-source contradiction) is narrowed but **not closed**: the declared copy source does not contain content the requirement mandates. Everything else in the target verified against the tree: README duplication exactly at `:75–91`/`:106–125` with both `Developers:` lines at `:105`/`:125`; cli.md essays at `:98–251`; doctor pointer `:264`; walkthrough link `:212`; ADR-0009 prose pointer `:55`; nav order `mkdocs.yml:83–86`; only cli.md pin is XREV-S03-06 (`:504`, targets the `--config` row that stays); merge-order disjointness true for both shared files (PR 190's walkthrough diff is the six claimed hunks; base walkthrough identical to branch's).

## Findings

- [CRITICAL] The copy-source rule and REQ-DOC2-S01-01 contradict each other for two mandated quick-start blocks — `specs/docs-p2/spec.md:80-85` vs `tasks.md:44-51`
  Failure: `git show pr-190:README.md` (verified, ref = live PR head via ls-remote) has **no forge-selection block and no GitHub comment-only note** — PR 190's base `4bd2d7f` predates the E10 merge that added them. S01-01 requires "forge-selection and GitHub comment-only-note content" in index.md, but S01 Goal/T002 say the copy source is PR 190's README and "where the two READMEs differ, PR 190's wording wins" (unscoped) — so the only permitted source for those two blocks is the branch's README, which the rule forbids. Executing T002 as written yields an index.md violating S01-01, or an implementer deviating with no spec sanction — the round-1 C2 class, recurring narrower.
  Fix: scope the rule: "PR 190's wording wins for the three corrected facts (banner, Why `.tf` clause, `@v0.4.0`); the E10 forge-selection and comment-only blocks have no PR-190 counterpart and are sourced verbatim from this branch's README."
  Confidence: 90
- [WARNING] "Its three hunks … are the only differences from the branch's README" is false against this branch — `tasks.md:45-47`, `specs/docs-p2/spec.md:107-109`
  Failure: `git diff 8efd685 pr-190 -- README.md` = four regions: `:22`, `:45–46`, the `:73–108` region (190's one-line `@v0.4.0` change conflated with the E10 blocks its base lacks), and `:139–142` where pr-190's README says GitHub adapter **Planned** vs the branch's **Core (comment-only in v1)**. The three-hunk claim holds only against 190's base `4bd2d7f`. The S01-04 diff-verification as written will surface differences the spec asserts cannot exist.
  Fix: reword to "PR 190's diff against its own base is three hunks; against this branch the ref additionally lacks E10's quick-start additions" (folded into F1's fix).
  Confidence: 92
- [NOTE] Garbled arithmetic in the mutant rationale: "10−4 of 11 is below the MIN of 8" — `specs/docs-p2/spec.md:214-215`
  Failure: none — the real mutant (`hack/audit/exitgate_test.sh:1926-1929`) deletes 4 members, so post-addition it leaves 7 of 11 present < MIN 8 and stays red; conclusion correct, notation wrong, and T004 step 5 runs the mutant live anyway.
  Fix: write "11−4=7 of 11 < 8".
  Confidence: 90
- [NOTE] S03's ADR-0009 justification slightly overstates: the pointer (`docs/adr/0009-execution-modes.md:55`) names `docs/usage/cli.md` §*How to keep assent advisory* — the section survives on the new page, but the pointer's file reference becomes false after the move. Already recorded as a known limitation; tighten the wording to say exactly that. — `specs/docs-p2/spec.md:160-162`
  Confidence: 85

## Could not check

- Did not execute `task docs-build` / `task docs-gates` / the exit gate (read-only review); all gate claims verified by reading the scripts, their pinned rules and mutation controls instead.
- `reviews/spec/register.md` (round-1 register) — outside this repo; not read.
- `data/assent-docs-audit/report.md` — absent from the repo (round-1 dispositioned as repo precedent; honored, not re-litigated).
- The 90.5% baseline coverage figure in tasks.md — not measured (needs a full test run).
- post-merge behavior of both PRs verified by diff/merge-base analysis only, not a trial merge.
