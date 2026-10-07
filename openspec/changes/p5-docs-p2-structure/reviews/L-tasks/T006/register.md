## Unified verdict: BLOCK  (legs ok: 2/2)

| # | Sev | Claim (one line) | Where | Models | Legs | Conf |
|---|---|---|---|---|---|---|
| 1 | CRITICAL | Review range `49d573f..HEAD` is empty (HEAD == 49d573f): T006 edits are uncommitted working-tree changes, so the evidence directs every review leg at zero lines | evidence/T006.md:14,44-45 | 2 | 2 | 100 |
| 2 | CRITICAL | Spec's exhaustive "four" pointer list omits the still-stale D-134 row ("decision matrix above is now published in `cli.md`" — now `docs/usage/operating-safely.md`), a fifth pointer recorded nowhere as a residual | specs/docs-p2/spec.md:158; docs/decisions/decisions.md:141 | 2 | 2 | 90 |
| 3 | WARNING | New `operating-safely.md` required-surface entry has no mutation control of its own — dropping it from the list reddens nothing (`missing-required` removes only walkthrough.md) | hack/audit/exitgate_test.sh:830-831,1923-1926 | 2 | 2 | 100 |
| 4 | WARNING | proposal.md mirror still counts the inventory as "three" while the spec now says "four" — same sentence, two tracked files, divergent | proposal.md:52-54 | 1 | 1 | 90 |
| 5 | WARNING | T005 row's Command cell lacks `-v` yet Named-evidence claims the `-v`-only `--- PASS:` line; reproducing the recorded command yields the bare-`ok` vacuity W8 was filed to remove | evidence/T005.md:14 | 1 | 1 | 88 |

## Disagreements
- D-134/"four" inventory: Qwen WARNING(75) vs DeepSeek NOTE(70) — same defect, one severity level apart; Qwen wants it added as (e), DeepSeek wants it explicitly classified as residual.
- T005 evidence staleness: Qwen flags the readme-smoke misquote (`:13`, NOTE 85), DeepSeek flags the `-v`/PASS mismatch (`:14`, WARNING 88) — adjacent cells, and neither leg names the other's.

## Nobody could check
- No leg executed any gate (exitgate `--text-only`, `task docs-gates`/`docs-build`, `task check`, `go test`); "green" claims accepted from logs/evidence only.
- Whether the full-exitgate baseline deviation (90.5% < 91%, E10) is still red at this tree.
- Rendered `site/` anchor slugs and mkdocs nav membership for `operating-safely.md`.
- `docs/decisions/**` exclusion from the retired-phrase corpus: asserted by comment (`exitgate_test.sh:257-261`), not verified in config.
