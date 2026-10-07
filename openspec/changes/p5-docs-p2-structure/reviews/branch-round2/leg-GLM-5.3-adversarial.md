Now I have the shape of the change. Let me verify the round-1 fixes mechanically: verbatim-move diff, README dedup vs index copy, exitgate corpus math, transcript-leak hygiene.
Verbatim move is clean — only the four allowed transformations. Now the README→index copy and exitgate math:
Corpus math checks out (11 entries / 10 present / MIN 8 → shrunk mutant 6<8 red). Now the mechanical copy diff, README vs index.md:
Copy diff matches spec exactly (link targets + the three P0 hunks, nothing else). Now the round-1 disposition records and leak-hygiene sweep:
Round-1 register read. Now verify the round-1 fixes landed: untracking, named nostale record, C1 deferral record, and a leak sweep over tracked files:
The nostale fix is real (named `-v` PASS records in T005/T006). But my leak sweep matched 7 *tracked* files — let me see exactly what they carry:
The string residuals are visible in tracked review records. Let me check whether the T006-round2 register litigated this, read loop.md for the C1 record, and sweep the whole tree:
Two last verifications — T005's corrected cell and tasks.md state:
## Verdict: BLOCK

One CRITICAL: the C2/C4 fix closed the bulk leak but re-committed the offender strings in the review records that fix itself landed. Everything else in round 1 (C1 deferral, C3, W5/W6/W7/W8, T006-round2's two CRITICALs + smoke-quote cell) I verified as closed or recorded. Spec REQ-DOC2-S00..S03: all four hold; verified mechanically, not from the evidence's claims alone.

## Findings

- [CRITICAL] Round-1 C2/C4 residual: the offending strings survive in 4 *tracked* review-record files — `<redacted: operator home path>` ×6 and `<redacted: internal model registry id>`/`<redacted>` ×6 — committed by the fix commits themselves — `openspec/changes/p5-docs-p2-structure/reviews/branch/register.md:4,27`, `loop.md:123`, `reviews/branch/leg-GLM-5.3-security.md:9,17,18`, `reviews/branch/leg-Qwen3.8-Flash-Next-security.md:6,11,21`
  Failure: operator authorizes push per AGENTS.md rule 2 → employer-internal provider name and operator home path enter public GitHub history in curated records; nothing reddens — the sanitizer provably can't catch either (CI layer runs bare, `host_pat` misses hyphenated `internal-` prefixes; round-1 leg proved the gate passed with the violation in-tree).
  Fix: scrub the 4 files (`<redacted: operator home path>`→`~`, provider path→"the internal model-provider path"); finding text stays actionable. Update loop.md:123's disposition — its "quoted-pattern precedent" doesn't hold: exitgate quotes the repo's *own* retired product phrases, not employer-internal identifiers. Then add the home-path pattern to `check-sanitization.sh`'s CI layer (the round-1 hand-off proposal, still undone).
  Confidence: 70 (loop.md:123 records a deliberate disposition, but it is unratified by an operator decision and conflicts with AGENTS.md hard rule 1's binary wording; round-1 rated the identical string CRITICAL in logs)

- [WARNING] Site home inherits a false deixis from the sanctioned verbatim copy — `docs/index.md:82`
  Failure: index.md says the quick-start sample-repo block is "the fixture `hack/docs/readme_smoke_test.sh` executes this block against", but the smoke test executes only the README's block, never index.md's — a small published untruth on the site home. Spec S01-04 mandates as-is carriage, yet S03 explicitly lists *its* two allowed deixis fixes while S01 lists none — the spec set missed its own third deixis case.
  Fix: one-word fix ("the README's equivalent block") plus a one-line S01 allowed-transformation note; or fold into PR 190's DOC-13 closer already recorded as the divergence follow-up.
  Confidence: 75

- [NOTE] `T006` is invisible from `tasks.md` — the round-1 fix task (gate re-runs, named nostale record, mutant pin, MIN-headroom deferral) exists only in loop.md/evidence/registers; tasks.md has five `[x]` rows and no pointer to loop.md — `openspec/changes/p5-docs-p2-structure/tasks.md:120`
  Failure: a fresh session reads tasks.md as the burn-down, sees 5/5 closed, misses the T006 chain and its recorded deferrals.
  Fix: append a closed T006 row pointing at `evidence/T006.md` + the round-2 register.
  Confidence: 60

- [NOTE] Corpus floor still permits single-file silent coverage loss: MIN 8 with 10/11 present and the required-surface loop covering only 5 members — deleting exactly one non-required member (e.g. `cli.md`) leaves 9 present ≥ 8, required loop green, that page's phrase coverage gone — `hack/audit/exitgate_test.sh:275,829`
  Failure: a page rename/delete reddens nothing until a second member also vanishes.
  Fix: none now — spec-settled MIN and the T006 "per-member controls are over-engineering" decision are recorded; carrying it so the adjudication is findable.
  Confidence: 90 that the gap is real, 0 that it is unadjudicated

- [NOTE] C1 (README↔index duplication) remains deferred by design — merge-order recommendation in proposal, follow-up pin in loop.md C1 row; index.md now re-carries the quick-start that S00 de-duplicated from README. Not re-litigated; the deferral is the round-1 adjudication.
  Confidence: 100 (verified recorded, `loop.md:106-108`)

Round-1 closure evidence I verified directly: verbatim-move diff of the five essays = only the four Goal-named transforms (S03-01 holds); index↔README copy diff = link targets + exactly the three P0 hunks (E1–E9 confirmed at index.md:9 vs README.md:22; `@v0.4.0`; `.tf` clause) (S01-04 holds); README S00 block = pure 20-line deletion, order per S00-01; mkdocs nav order per S02-01; three dispositioned pointer files byte-unchanged; required-surface mutant removes both walkthrough+operating-safely while 9 present ≥ 8, so it genuinely tests the loop, not the floor (C3 closed); named `--- PASS: TestNoStaleProductClaims` at T005.md:14/T006.md:75 (W8 closed); proposal inventory = five (T006-round2 CRITICAL-1 closed); S03-03 now names all three unretargeted pointers (T006-round2 CRITICAL-2 closed); smoke-quote cell corrected with inline rationale at T005.md:13. No tracked-but-ignored files exist. Whole-tree scan: no other file carries the leak strings.

## Could not check

- No gate executed by me (read-only review): docs-build/docs-gates/exitgate `--text-only`/TestNoStaleProductClaims `-v`/sanitization accepted from the prompt's branch-tip claim, loop.md's pre-round-2 batch record, and T005/T006 evidence — not re-run at HEAD `d003e96`.
- Built `site/` anchor slugs (S03-03's Test): derived from headings + mkdocs naming, never grepped from rendered HTML.
- PR 190's actual diff (open PR, outside this repo): the three P0 hunks verified only as the spec describes them; DOC-13 green-on-land unevaluable.
- Whether the operator's local `ASSENT_SANITIZE_DENYLIST` contains "<redacted>" — deliberately uncommitted.
- Full exitgate non-text layers (cassettes, full `task check`) — baseline-coverage red at 90.5%<91% accepted as pre-existing E10 state, out of scope.
