## Unified verdict: BLOCK  (legs ok: 2/2)

| # | Sev | Claim (one line) | Where | Models | Legs | Conf |
|---|---|---|---|---|---|---|
| 1 | CRITICAL | Proposal mirror still counts the reference inventory as "three" while the spec this commit edits says five — one-sided round-2 fix widened the divergence | `openspec/changes/p5-docs-p2-structure/proposal.md:52-54` | DeepSeek, Qwen | 2 | 100 |
| 2 | CRITICAL | Goal dispositions new residuals (d)/(e) but REQ-DOC2-S03-03 names only ADR-0009, so a requirement verifier never confirms the frozen p4-e1-s11 record or the D-134 row unchanged | `specs/docs-p2/spec.md` S03 Goal vs S03-03 | DeepSeek, Qwen | 2 | 95 |
| 3 | WARNING | Added required-surface entry is itself unpinned: `missing-required` mutant removes only `walkthrough.md`, so deleting `operating-safely.md` reddens nothing (disclosed/deferred) | `hack/audit/exitgate_test.sh:831` vs `:1923-1926` | DeepSeek, Qwen | 2 | 100 |
| 4 | WARNING | T005 docs-gates cell quotes `OK: 7 … green`, provably un-emittable (script always prints the "3 green, 4 skipped" form); T006.md verifies this then defers the one-cell fix while editing the rows below | `evidence/T005.md:13` vs `hack/docs/readme_smoke_test.sh:126` | Qwen | 1 | 85 |

Both CRITICALs verified verbatim by the unifier; the WARNING-4 mechanism verified (T006.md:82-83 confirms the mismatch itself). Dropped: 3 single-leg NOTEs (T005 header tree-state, MIN-headroom residual, T006.md:133 register path, commit-message scope). Both models independently confirmed the five inventory citations and found no sixth pointer.

## Disagreements
- Finding 1's scope: DeepSeek treats proposal.md as out-of-T006-scope/disclosed; Qwen argues the deferral made the state worse, not equal — register takes Qwen's framing.
- T005 evidence file: DeepSeek rates its defect NOTE (header tree-state), Qwen rates a cell in the same table WARNING (un-emittable quote) — divergent severity on one file.

## Nobody could check
- No gates executed by either leg — `task docs-build`/`docs-gates`, `exitgate --text-only`, `go test TestNoStaleProductClaims -v`, sanitization all accepted from reading/evidence.
- Committed `evidence/T006.md` `-v` PASS at the T006 tree not re-run.
- Built `site/` anchor presence for `operating-safely.md` (S03-03's Test).
- `hack/audit/README.md:302`'s 38/32 control counts vs HEAD (file outside diff).
- Full exitgate / `task check` (declared baseline-coverage deviation not re-checked).
