# Specs

Spec-driven backlog. Start at **[backlog.md](backlog.md)** — the phase → epic index.

- Phases 1–2 (requirements harvest, spikes & ADR firming): full epics with INVEST stories
  and REQ IDs (`REQ-<epic>-S<story>-<nn>`, each with Given/When/Then, `Test:`, `Verify:`),
  one directory per epic.
- Phases 3–5: epic paragraphs in [later-phases.md](later-phases.md); their stories are
  authored during Phase 3 ("contracts first"), after the contract fixture (ADR-0017 §8)
  exists.

Authoring rules: [../config.yaml](../config.yaml). Levels: L0–L3 per
[ADR-0006](../../docs/adr/0006-testing-strategy.md); design artifacts use `Level: doc`.

## Gate-wiring rule for guards (applies to every hygiene epic: CIH, NIT, TDS, TCC, DEP, CMY)

A gate that cannot block a merge is not a gate. Every new guard script, **in every mode it has**
(`--self-test`, `--classify`, `--transitions`, ...), must (1) run on `pull_request` in the
required `verify` job (or in a job that is in the `needs` of the required aggregator), as its
**own argument-pinned step** in the style of `verify.yaml:63-69` (unargumented or fully
argumented, no `if:`, no `continue-on-error:`, asserted by `workflow_pins_test.sh`), and (2) run
locally via `task check`, with each mode pinned in `STAGE_BODY_PINS`
(`hack/audit/exitgate_test.sh:207`). Reaching CI only through `release-exitgate` (push-only,
`verify.yaml:213-214`) is not enough: an unwired mode is the D-159 orphan class. Each epic's
Exit section references this rule instead of restating it.
