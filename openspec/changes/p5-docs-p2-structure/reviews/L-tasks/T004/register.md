## Unified verdict: CLEAN  (legs ok: 2/2)

| # | Sev | Claim (one line) | Where | Models | Legs | Conf |
|---|---|---|---|---|---|---|
| 1 | LOW | Stale prose pointers into the moved essays survive outside task scope (p4-e1-s11 README:27; ADR-0009:55) — spec dispositions both; DeepSeek flags the README one as a marginal frozen-record reader trap, Qwen verifies the named title still resolves on the new page | docs/decisions/evidence/p4-e1-s11-adoption/README.md:27; docs/adr/0009-execution-modes.md:55 | DeepSeek+Qwen | 2/2 | 100 |

## Disagreements
- p4-e1-s11 README:27: DeepSeek frames the surviving stale pointer as a defect worth flagging (reader follows frozen record into cli.md); Qwen frames the same pointer as correctly dispositioned by spec, no action.

## Nobody could check
- Full-mode `bash hack/audit/exitgate_test.sh`: recorded red (baseline coverage 90.5% < 91%) never reproduced; DeepSeek ran `--text-only` only (green, mutant arithmetic hand-verified), Qwen ran no gate at all.
- `task docs-build` / fresh site build: neither leg built; both grepped the author's uncommitted, possibly-stale `site/` (anchors and retargeted hrefs verified present).
- `docs/planning/loop.md` and T001–T003 evidence only spot-checked via quotes in T004.md (Qwen).

## Dropped by rule (NOTE × one leg)
- Qwen only: two Go-test comments still point readers at cli.md for promises that moved (`cmd/assent/run_checkout_containment_test.go:215`, `cmd/assent/run_arming_test.go:116`); evidence `T004.md:160` miscounts the new page (158 vs actual 162, essays 154); exitgate `PHRASE_CORPUS_MIN=8` has slack of 2 after this commit (optional ratchet).
- DeepSeek only: new-page intro overstates fidelity ("unchanged apart from links retargeted" — heading level `###`→`##` and one sentence also changed, both Goal-authorised).
