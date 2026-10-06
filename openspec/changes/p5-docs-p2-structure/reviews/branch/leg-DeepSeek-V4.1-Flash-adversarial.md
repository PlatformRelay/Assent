## Verdict: CONCERNS

## Findings

- [WARNING] Corpus anti-vacuity floor was not ratcheted after adding a member — `hack/audit/exitgate_test.sh:275`
  Failure: this change adds `docs/usage/operating-safely.md` to `PHRASE_CORPUS` (now 11 entries, 10 present) but leaves `PHRASE_CORPUS_MIN=8`. Headroom to silent shrinkage rises from one file to two: delete `docs/index.md` + `docs/usage/cli.md` in a later reorg and `check_retired_phrases` sees 8 ≥ 8 and goes green over a shrunken set, dropping their retired-phrase coverage. AGENTS.md treats such registers as shrink-only.
  Fix: set `PHRASE_CORPUS_MIN=10` in the same commit (normal run 10 ≥ 10 green; the shrunk mutant still 6 < 10 red; the `missing-required` control is unaffected).
  Confidence: 85

- [NOTE] The spec's "inventory is three" claim is false — `spec.md:158-164` vs `docs/decisions/evidence/p4-e1-s11-adoption/README.md:27`
  Failure: that page names "the CLI reference's *What gates approve and merge*", a section that no longer exists in `cli.md`. The change's own evidence calls it a "spec-unlisted fourth prose pointer" (`evidence/T004.md:259`), yet the spec text still asserts the inventory is complete at three. Low impact (frozen record, `not_in_nav`), but the spec sentence should say four.
  Confidence: 95 (real); severity low

- [NOTE] One stale pointer is absent from the hand-off residual list — `docs/decisions/decisions.md:141`
  Failure: D-134 states the decision matrix "is now published in `cli.md`"; after the move it is in `operating-safely.md`. `evidence/T004.md:258-262` records ADR-0009, the evidence README and the two Go-test comments but not this one. Historical record (supersede-don't-edit), so the fix is to add it to the residuals list.
  Confidence: 90

- [NOTE] New page's fidelity self-description is still inexact — `docs/usage/operating-safely.md:4-5`
  Failure: "word-for-word, with heading levels and the retargeted links as the only adjustments" omits the advisory essay's deixis rewrite ("the CI snippet above" → the linked `assent run` invocation), which the spec itself lists as a separate allowed transformation (`spec.md:166-171`). The evidence notes this line was revised once for this class and still isn't exact.
  Confidence: 60

## Could not check
- I ran no gate. The branch-tip green matrix is taken from `evidence/T005.md`; I did not execute `mkdocs build --strict`, `exitgate_test.sh`, `docs-gates` or the Go tests.
- I read `exitgate_test.sh` to line 1517 plus the phrase-mutation block; the schema-freeze, CI-wiring and disposition-table checks were skimmed, not fully verified.
- Built-site anchors were verified by hand (heading→slug), not by building; `validation.anchors` is off by default, so a wrong slug would not be caught by `docs-build`.
- Moved-text fidelity was machine-diffed (only the two authorised edits survive) — but only after normalising `###`→`##`; I did not diff the new page's intro against any prior draft.
- Baseline coverage red (90.5% < 91%) and the openspec review registers under `reviews/` were not re-measured or read.
