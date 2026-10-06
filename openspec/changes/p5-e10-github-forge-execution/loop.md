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

- [x] 0 orient — S00/S01 landed upstream; resume at S02.
- [x] P change created (proposal + tasks + delta spec) — 1a07099.
- [x] R spec-set review — 3 legs (GLM-5.3-Flash / DeepSeek-V4.1-Flash /
      Qwen3.8-Flash-Next), all FIX-FIRST, one consensus CRITICAL (the twelve
      S00 case IDs unowned) → reconciled 291ef56; register at
      reviews/register.md.
- [x] L task loop — T0..T16 executed: S02/S03 seam (c5b1d62), S04 capability
      model + D-186 (8733c07), S05 transport cases (48d1005), S06-S08 adapter
      (0c62cf3), S10-S12 factory+deltas (84db812), S13 selection (3b92c8c),
      S15/S16/S17 docs+action+gate (b03be84).
- [x] B branch review — round 1: 4 legs (3 free + Opus), all
      REQUEST_CHANGES, 1 CRITICAL (read chain) + endpoint mis-route + record
      gap → fixed 75089a2/2b5f934/d919778; round 2 (≤2 rounds rule): Opus +
      free legs REQUEST_CHANGES on fix-quality → fixed f91f8fa. Remaining
      open findings go to the PR description.
- [ ] Hand-off: push branch, open PR.

## Review log (round summaries)

- Round 1 register: reviews/branch-leg-*.md (+ claude-opus). Key fixes: one
  read chain (GetMR fills the pin), forge-consistent endpoints,
  TestForgeSelection, capability-gated merge-result pin, honest grading,
  catalog integrity.
- Round 2 register: reviews/branch2-leg-*.md (+ claude-opus). Key fixes:
  Snapshot read-chain check in orchestrate, first-write-wins pin, atomic
  harness counters (-race), live-shaped CreateThread, changed_files
  bidirectional cross-check, D-188 (deltas consulted only by the deferred-
  arming verb).
- Deliberate deferrals carried to the PR: S18 live adoption proof (operator);
  OQ-33/OQ-34 arming/require-review promotion; identity cross-check
  (GET /user / GET /app) for the marker filter is configured-login-based in
  v1 (REQ-E10-S10-02's mechanism noted as the promotion requirement); the
  "both auth shapes" App conformance leg.
