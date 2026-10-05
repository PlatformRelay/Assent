# loop.md — p5-e10-github-forge-execution (spec-loop state, adapted to OpenSpec)

- Base branch: `main` (branch `fm/assent-github-support` created from 4bd2d7f).
- Feature: `openspec/specs/p5-e10-github-forge/spec.md` (E10, D-140 unlock, ADR-0021).
- Change dir: `openspec/changes/p5-e10-github-forge-execution/` (proposal + tasks + delta).
- Stories already landed upstream: S00 (a072933 + d385f32), S01 (99de1d7).
- Execution scope: S02–S17. Out: S18 (operator infra); OQ-33/OQ-34 stay open (fail-closed).
- Model routing: orchestrator + implementation here (GLM-5.3-Flash); reviews = three fresh
  reviewers on distinct free models (GLM-5.3-Flash / DeepSeek-V4.1-Flash / Qwen3.8-Flash-Next)
  via headless `opencode run`; `claude` available as paid fallback if a gate is BLOCK.

## Fitness functions (inventory at orient)

| Characteristic | Command | Type | Baseline |
| --- | --- | --- | --- |
| Coverage on internal/ (D-010 ratchet) | `task coverage` (floor `COVERAGE_MIN` in Taskfile.yml, currently 91%) | triggered | 91% |
| golangci-lint incl. depguard (ARCH-02 adapter leak) | `task lint`; `hack/lint/depguard_test.sh` | triggered | cmd/assent must not import adapters (3-symbol allowlist today; S02 empties it) |
| Determinism golden double-run | golden tests in internal/core, cmd goldens | triggered | goldens byte-identical |
| Schema freeze | `git diff schemas/` | triggered | == 0 |
| Sanitization | `bash hack/check-sanitization.sh` | triggered | no employer/internal names (D-002) |
| Core purity | TestCorePurity | triggered | internal/core I/O-free |
| Conformance suite | `go test ./internal/forge/...` | triggered | catalog-gated case set |
| Mutation (nightly) | TDS nightly | holistic | not run in-loop |

## Stage log

- [x] 0 orient — S00/S01 landed upstream; resume at S02. Reviewers: opencode free models
      available; claude available.
- [x] P change created (proposal + tasks + delta spec).
- [ ] R spec-set review (3 legs) → triage.
- [ ] L task loop (T1..T16).
- [ ] B branch review (3 legs) → fix → re-run (≤2 rounds).
- [ ] Hand-off: push branch, open PR (ready, not draft), no merge.

## Review log

- (pending)
