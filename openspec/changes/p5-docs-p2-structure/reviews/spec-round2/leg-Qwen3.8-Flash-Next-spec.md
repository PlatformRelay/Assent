## Verdict: CONCERNS

Round-1 both CRITICALs are mechanically closed (corpus mutant verified red stays red; copy-source named), but the C2 fix baked in a new false premise about the PR-190/branch diff, which breaks the S01 verification and risks dropping E10 content from the new front door. Executing T001/T003/T004 as written delivers audit items 9–11; T002 as written can silently under-deliver item 9.

Per-REQ status (target = the spec set; code verified, not prose):
- REQ-DOC2-S00-01: holds — `README.md:106–125` is exactly the E10 duplicate (verified `git diff 4bd2d7f 3503cc0 -- README.md`); smoke skips `go install`/`task` (`hack/docs/readme_smoke_test.sh:78-99`), duplication-blindness honestly stated; DOC-11 caveat pin is presence-based (`hack/docs/truthlag_pins_test.sh:170-177`) so the deletion is safe.
- REQ-DOC2-S01-01: partial — see W1.
- REQ-DOC2-S01-02: holds — `planning/**` in `not_in_nav` (`mkdocs.yml:106`), ADRs navigated (`mkdocs.yml:60-78`), `omitted_files/unrecognized_links: warn` + strict (`mkdocs.yml:121-124`); mermaid fence configured (`mkdocs.yml:44-46`) so the copied flowchart builds.
- REQ-DOC2-S01-03: holds — planning links currently in Start here (`docs/index.md:17`), move-out is the task.
- REQ-DOC2-S01-04: contradicted — see W1.
- REQ-DOC2-S02-01: holds — current order `mkdocs.yml:83-86`.
- REQ-DOC2-S03-01: holds — essays at `cli.md:98–251`; both pages same dir so `../adr/*` links survive; deixis `cli.md:170` and anchor `cli.md:116` exactly as spec names.
- REQ-DOC2-S03-02: partial — see N1; `--config` pin safe (`truthlag_pins_test.sh:500-508`).
- REQ-DOC2-S03-03: holds — `walkthrough.md:212` and `docs/adr/0009-execution-modes.md:55` confirmed; inventory really three.
- REQ-DOC2-S03-04: holds — corpus list + MIN read at `hack/audit/exitgate_test.sh:262-274`; mutant removes 4 named files (`:1926-1927`); present=10 after the add, 10−4=6<8 → mutant still red. Verified by reading, not execution.

## Findings
- [WARNING] REQ-DOC2-S01-04's Test premise is false: `git show pr-190:README.md` differs from the branch README in six hunks, not three — E10 also added the forge-selection + GitHub comment-only quick-start blocks (`README.md:92-103`) and rewrote the GitHub-adapter maturity row — and pr-190's copy-source does not contain the forge-selection/comment-only content REQ-DOC2-S01-01 mandates — `openspec/changes/p5-docs-p2-structure/specs/docs-p2/spec.md:84-85,107-109`, `tasks.md:43-51`
  Failure: implementer runs T002 step 3's diff check, sees six hunks where "three are the only differences" was promised, and either "resolves" the mismatch by copying only what pr-190 has (dropping required quick-start content from item 9) or records a check that cannot pass. The S01 "Dependencies: S00 (the quick start it copies must not carry the duplication)" rationale is also stale under C2's fix — pr-190's quick start has no duplication to begin with.
  Fix: one clause in T002/REQ-DOC2-S01-04: the three hunks are the only differences in banner/Why/How/quick-start-common text; the E10 forge-selection and comment-only blocks exist only in the branch README and are copied from it; record the fetched pr-190 SHA in evidence.
  Confidence: 90
- [NOTE] REQ-DOC2-S03-02 says the `-arm`/`-checkout`/`-pack` rows' "existing italic 'see X below' tails are replaced" — the `-pack` row has no such tail (`docs/usage/cli.md:88`); T004 handles it correctly ("give … a caveat + link"), but the REQ misdescribes a row it names, and the added caveat is new prose in the reference table, in mild tension with "no sentence is edited" — `spec.md:191-197`
  Fix: in S03-02, say `-pack` gains (not replaces) its caveat, wording drawn from the moved advisory essay.
  Confidence: 85
- [NOTE] REQ-DOC2-S03-04's arithmetic sentence "10−4 of 11" is wrong-but-harmless: the real present-count after the add is 10 of 11 listed (`docs/usage/quickstart.md` is in the corpus but absent on disk — pre-existing dead entry, `exitgate_test.sh:270`); 10−4=6<8 keeps the mutant red. The loop.md C1 disposition ("10−4=6") has it right, so the spec line is the sloppy one.
  Fix: reword to "10 present − 4 removed = 6 < MIN 8". Don't fix the quickstart.md entry here — corpus registers are for their owner.
  Confidence: 85
- [NOTE] S01 couples the site home to an open, unmerged PR head: if PR 190's wording changes before merge, README and index.md silently diverge — DOC-13 pins only version pins (`gh pr diff 190` truthlag hunk), and the README↔index wording pin is explicitly deferred to hand-off (`proposal.md:70-74`). Accepted residual; recording the fetched SHA (per W1's fix) is what makes later drift datable.
  Confidence: 70

Nothing the target does is unrequested except S00 itself, which the spec labels as scope-found-at-re-verification. Merge-order disjointness claim verified hunk-by-hunk against `gh pr diff 190` (walkthrough old-side hunks 1–7, 67–74, 193–199, 223–229, 232–238, 252–258 all miss :212; README hunks miss 106–125).

## Could not check
- `task check` coverage-red baseline (90.5%<91%) — not executed; taken from loop.md's record.
- Actual gate runs (`task docs-build`/`docs-gates`, `exitgate_test.sh`, `TestNoStaleProductClaims`) — plan-mode read-only; claims checked by reading the scripts, including the walk in `cmd/assent/main_help_test.go:106-140` which covers the new page identically to `cli.md`.
- Audit report items 9–11 verbatim — `data/assent-docs-audit/report.md` is not in the repo (`data/` absent); Round-1 rejected this as a citation defect by precedent; not relitigated.
- PR 190 head content after today; round-1 review register and `reviews/spec-round2/` (other reviewers — out of bounds).
