## Verdict: CONCERNS

## Findings
- [WARNING] Item 10 is delivered only in half, and the adjudicating source is not in the repo — `openspec/changes/p5-docs-p2-structure/proposal.md:4,66`, `spec.md:9`
  Failure: the change header claims it "executes audit items 9–11", but the item-10 "+ later Writing rules" half is an explicit non-goal; the origin `data/assent-docs-audit/report.md` exists in no commit of this repo (`git log --all -- data/assent-docs-audit` empty), so "items 9–11 done" cannot be checked in-tree at all — the spec calls it "the authoritative evidence" for every finding.
  Fix: vendor the audit report (or its P2 section) into the repo at the cited path, and either carry the Writing-rules page or re-title the change "items 9, 10-half, 11".
  Confidence: 85 (local refs only; the report may live on an unfetched branch or the workbench)
- [WARNING] The mandated verbatim move leaves a dangling deictic reference in the new page — `docs/usage/cli.md:168–171` vs `spec.md:160–164`
  Failure: "reruns the CI snippet above" refers to the run-invocation example at `cli.md:70/73`, which stays in `cli.md`; in `operating-safely.md` nothing above the sentence is a snippet. `mkdocs --strict` (link-only) and the anchor grep (REQ-DOC2-S03-03) cannot catch prose, and REQ-DOC2-S03-01's verbatim clause forbids the smallest fix.
  Fix: add the run-usage snippet to the new page's intro, or carve this one sentence out of the verbatim requirement to link to `cli.md#assent-run`.
  Confidence: 80
- [WARNING] The safety copy moves out of reach of the front-of-house sensors, and no requirement restores coverage — `hack/audit/exitgate_test.sh:262–273`
  Failure: `PHRASE_CORPUS` grades `docs/usage/cli.md`/`docs/index.md` only; after S03, the retired-phrase absence checks no longer see any of the 154 lines of trust-model copy (a future "not yet implemented"-class phrase inserted there is invisible). PR 190's DOC-13 likewise scans a fixed file list that excludes `operating-safely.md`. REQ-DOC2-S03-04 correctly reports the gates stay green — that is precisely the coverage-shrink that no gate can fail on.
  Fix: one commit adding `docs/usage/operating-safely.md` to `PHRASE_CORPUS` (corpus canary `assent` is trivially present), inside T004.
  Confidence: 90
- [NOTE] REQ-DOC2-S01-01's top-to-bottom chain omits the GitHub comment-only note that T002 copies — `spec.md:79–80` vs `tasks.md:37`
  Failure: a diff review graded on the requirement rejects the block, or an executor drops it, silently diverging index.md from README — the one property the whole story is about.
  Fix: add "GitHub comment-only note" to the requirement's chain.
  Confidence: 85
- [NOTE] Retargeting `cli.md`'s doctor link keeps the word "above" pointing at another page — `docs/usage/cli.md:264`, `tasks.md:66`
  Failure: post-T004 the sentence reads "See [link → operating-safely] above" with nothing above. Cosmetic, ungated.
  Fix: drop "above" in the same edit.
  Confidence: 85
- [NOTE] S01 copies wording from an open PR with no ratchet between landings — `spec.md:97–103`, `proposal.md:61–66`
  Failure: if PR 190's wording changes during review (it touches `README.md:22,45–46,76`, all verified against its diff), index.md silently mismatches; between this branch landing and 190 landing, README says `@v0.4.0`-era facts while… README still carries `@v0.1.0` (`README.md:76`), and the DOC-16 pin is deferred. Acknowledged in the spec; residual risk noted.
  Fix: none required; land this branch first, as the merge-order note recommends (verified: DOC-13 would go red on the `README.md:108` duplicate, and the latest tag is `v0.4.0`).
  Confidence: 75

Checked and holding: README duplication at 105–129 and the 106–125 deletion yielding exactly the required order; `readme_smoke_test.sh` extracts/executes quick-start bash blocks; essays at `cli.md:98–251` of 396 lines, `###`-level so all `##` command sections survive for `TestCLIDocCoversSubcommands`/`main_clidoc_test.go:11`; exactly two references into the essays (`cli.md:264`, `walkthrough.md:212`) — the spec's "two" claim holds; XREV-S03-06 (`truthlag_pins_test.sh:500–508`) pins only the staying `--config` row; `TestNoStaleProductClaims` walks all of `docs/`, so the verbatim page stays green; canary token is `assent` (`exitgate_test.sh:841`), so the index.md rewrite is safe; `not_in_nav` keeps planning links buildable (`mkdocs.yml:98–121`); REQ-DOCSNAV-S01-01/04 exist as cited (`openspec/specs/p5-docsnav-site-reachability/spec.md:59,99`).

## Could not check
- The audit report itself (absent from every local ref) — item 9–11 wording is taken from the proposal's paraphrase only.
- Did not run `task check` (baseline 90.5% < 91% coverage-red claim from `loop.md:31` unverified), nor `docs-build`/`truthlag`/`readme_smoke`/`exitgate`/`check-sanitization` — file contents read, commands not executed.
- PR 190's full 187-line pin diff spot-checked around DOC-13/DOC-12 only; whether its README hunks are exactly `:22, :45–46, :76` at its head SHA.
- `example_format_inventory_test.sh` and `hack/check-sanitization.sh` internals (assumed unaffected by docs-only moves).
