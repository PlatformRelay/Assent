## Provisional register (legs only, not unified) — legs ok: 6/7

### leg-DeepSeek-V4.1-Flash-spec
## Verdict: CONCERNS
- [WARNING] T004.2 points the `-pack` caveat at the in-page anchor `#how-to-keep-assent-advisory`, but that heading is deleted from `cli.md` in the same task — `openspec/changes/p5-docs-p2-structure/tasks.md:64`
- [WARNING] S03 counts "two references" into the essays, but there are three — `docs/adr/0009-execution-modes.md:55` states in prose "`docs/usage/cli.md` §*How to keep assent advisory* states this" — `openspec/changes/p5-docs-p2-structure/specs/docs-p2/spec.md:146-147`
- [WARNING] S01 must copy PR 190's corrected wording (E1–E9, the `.tf` clause, `@v0.4.0`), but this branch's README says `(E2–E8 …)` and `@v0.1.0`; T002 orders both "copy the README wording, do not reword it" and "per PR 190" — `openspec/changes/p5-docs-p2-structure/tasks.md:36-42`, `README.md:22,76`
- [NOTE] `proposal.md:83` says the change deletes `README.md:105–129`, contradicting `spec.md:25` and `tasks.md:25` (`106–125`) — `openspec/changes/p5-docs-p2-structure/proposal.md:83`
- [NOTE] REQ-DOC2-S01-02 asserts `adr/**` pages build via `not_in_nav`, but they are listed in `nav:` — `mkdocs.yml:54-78`

### leg-GLM-5.3-adversarial
## Verdict: BLOCK
- [CRITICAL] S03 moves the trust-model essays onto a published page the exitgate retired-phrase ban never scans — `hack/audit/exitgate_test.sh:272-282`, `spec.md:160-171`
- [WARNING] The change's authoritative origin is a dead reference — `proposal.md:4`, `spec.md:8`
- [WARNING] The merge-order note's premise is false — `proposal.md:82-84`
- [WARNING] The extraction strands a prose antecedent the spec forbids fixing — `docs/usage/cli.md:168-171`, `spec.md:160-164`
- [NOTE] REQ-DOC2-S01-01 omits the GitHub comment-only note that T002 copies — `spec.md:78-80` vs `tasks.md:37`
- [NOTE] Retargeting the doctor link leaves a false "above" — `docs/usage/cli.md:264`

### leg-GLM-5.3-security
## Verdict: CONCERNS
- [WARNING] The spec's "the two cross-page/in-page references" is false: a third published nav page points at a moved essay by page+section — `docs/adr/0009-execution-modes.md:55` — openspec/changes/p5-docs-p2-structure/specs/docs-p2/spec.md:145
- [WARNING] S01's copied wording is sourced from PR 190's hunks, which are neither in this branch's base nor pinned verbatim in the spec — spec.md:59–60, tasks.md:44
- [NOTE] REQ-DOCSNAV-S01-04's mechanical half never runs at T002 — tasks.md:13–21
- [NOTE] S03-02 requires caveat+link on the retained flag rows but never says the rows' existing "see *X* below" italics are replaced; S03-01's "kept verbatim" applies to essays only — spec.md:168–172, docs/usage/cli.md:89,91
- [NOTE] T005's "full docs-relevant gate set" is the tasks' 5-command block, which omits `check-migration-invariants.sh` that `task docs-gates` runs (Taskfile.yml:236–242) — tasks.md:13–21,78–84

### leg-Qwen3.8-2.4T-A95B-NVFP4-spec
## Verdict: CONCERNS
- [WARNING] T004.2 contradicts REQ-DOC2-S03-02: `-pack` row gets a bare `#how-to-keep-assent-advisory` link, but that anchor leaves `cli.md` in the same task — `openspec/changes/p5-docs-p2-structure/tasks.md:63` vs spec.md:168–172
- [WARNING] S03 counts "the two references" into the essays; a third live one exists — `docs/adr/0009-execution-modes.md:55` says `` `docs/usage/cli.md` §*How to keep assent advisory* states this `` (spec.md:145–147; tasks.md:66–69)
- [WARNING] Proposal's merge-order note says this change deletes `README.md:105–129`; spec S00 and T001 correctly say 106–125 — `proposal.md:32,83` vs spec.md:25, tasks.md:25
- [NOTE] The new `docs/usage/operating-safely.md` silently leaves two grep registers: the exitgate PHRASE_CORPUS (`hack/audit/exitgate_test.sh:262–273`) and PR 190's DOC-13 initial-chapter list. Both are absence checks and the essays move verbatim, so nothing breaks today, but the change's "no new pins" non-goal (proposal.md:61–66) records only the README↔index.md duplication as the follow-up pin candidate, not this corpus hole — spec.md:182–188 claims the pin duty "reduces to keeping these green", which is true now and silently weaker later.
- [NOTE] S01 never states the fate of index.md's existing intro paragraph (`docs/index.md:8–9`, the "APPROVE, REVIEW, or BLOCK" sentence); REQ-DOC2-S01-01's top-to-bottom list starts with the status banner, implying deletion, but the "no surviving sentence edited" discipline of S00 has no counterpart here — executor gets unrecorded discretion over published text. Minor proposal-prose inaccuracies too: item 9 says "one sentence, seven links" where index.md actually has a two-sentence intro and 8 links.

### leg-Qwen3.8-Flash-Next-security
## Verdict: BLOCK
- [CRITICAL] T002's closing instruction ("Copy the README wording, do not reword it") contradicts REQ-DOC2-S01-04, and the failure is deferred past every gate this branch runs.
- [WARNING] The `--checkout` security disclosures are stripped from the surface where an operator hits the wall.
- [WARNING] The reference inventory is short by one, and the missed one is invisible to every gate.
- [WARNING] Nothing protects the new safety links from anchor rot.
- [WARNING] The proposal's merge-order premise is false: `README.md` is not the only shared file.
- [WARNING] `proposal.md:83` names the wrong deletion range.
- [NOTE] REQ-DOC2-S00-01's named Test cannot fail for its stated condition.

### leg-Qwen3.8-Flash-Next-spec
## Verdict: CONCERNS
- [WARNING] Item 10 is delivered only in half, and the adjudicating source is not in the repo — `openspec/changes/p5-docs-p2-structure/proposal.md:4,66`, `spec.md:9`
- [WARNING] The mandated verbatim move leaves a dangling deictic reference in the new page — `docs/usage/cli.md:168–171` vs `spec.md:160–164`
- [WARNING] The safety copy moves out of reach of the front-of-house sensors, and no requirement restores coverage — `hack/audit/exitgate_test.sh:262–273`
- [NOTE] REQ-DOC2-S01-01's top-to-bottom chain omits the GitHub comment-only note that T002 copies — `spec.md:79–80` vs `tasks.md:37`
- [NOTE] Retargeting `cli.md`'s doctor link keeps the word "above" pointing at another page — `docs/usage/cli.md:264`, `tasks.md:66`
- [NOTE] S01 copies wording from an open PR with no ratchet between landings — `spec.md:97–103`, `proposal.md:61–66`
