# P5-TCC — Toolchain consistency (one source of truth for tool versions)

**Epic ID / REQ prefix:** `TCC` / `REQ-TCC-Snn-nn`. Cross-cutting hygiene epic. Gate wiring: see
the [gate-wiring rule](../README.md) (D1).

**Problem.** The operator wants the same tools, pins and entry-point names across repositories.
Verified 2026-10-05:

| Item | Current | Target |
| --- | --- | --- |
| Go in workflows | `go-version: stable` x **5** (`verify.yaml:72,234`, `codeql.yaml:45`, `schemas.yml:57`, `vulncheck.yaml:30`); `go-version-file: go.mod` in `release.yaml` x2 | `go-version-file: go.mod` everywhere |
| `go.mod` | `go 1.26.0` + `toolchain go1.26.6` | one `go` line equal to the shipped toolchain; **no separate `toolchain` line** |
| Task / golangci-lint / gitleaks | `v3.52.0` / `v2.13.1` / `v8.30.1` | same; drift test asserts agree AND >= floor |
| git-cliff | `v2.13.1` in two literals: `Taskfile.yml:5` `GIT_CLIFF_VERSION`, `release.yaml:138` | both tied by the test |
| govulncheck | `v1.6.0` literals in `verify.yaml` and `vulncheck.yaml` | one Taskfile var `GOVULNCHECK_VERSION` (>= v1.6.0, chosen at implementation) |
| `mise.toml` | absent from tree (local only) | committed, mirrors CI pins |
| Entry points | `task check` exists | `check` = full local gate (D12) |
| Dependency bot | Dependabot only | OPEN QUESTION |

**Rule, not number (D9/D10).** Durable text states "go.mod's Go equals the shipped toolchain";
the concrete patch lives only in tasks. Drift tests assert "all sites agree AND >= floor", never
exact values.

**Not in scope:** audit/AUD gate content; Python/uv pins for docs; a Nix flake.

---

## TCC-S01 — go.mod bump + `go-version-file` everywhere + tool-compatibility check `[autonomous]`

**One PR, one story; go.mod first.** Order is forced: `setup-go` prefers the `toolchain` line and
exports `GOTOOLCHAIN=local`, so switching to `go-version-file` **before** bumping `go.mod` would
downgrade CI from `stable` (1.27.x) to 1.26.6 and turn govulncheck red. Therefore the first commit
bumps `go.mod` to the latest patch of the current minor (`go` line only, `toolchain` line
removed); then the switch; then the checks below.

**D-158 interaction.** D-158 kept `stable` because golangci-lint cannot typecheck a stdlib newer
than the Go it was built with. v2.13.1's own `go.mod` supports <= 1.27 (D-158's heuristic) and
D-158 saw 0 issues on 1.27.0; the implementer **re-verifies on the chosen patch and bumps the
linter in the same PR if not**. Pinning removes the surprise `stable` roll but makes the coupling a
deliberate edit. **Hazards:** (a) tools run via `go run`/`go install` (`verify.yaml:100,104,131,249`)
cannot auto-download a newer Go under `GOTOOLCHAIN=local`; (b) a stale `go` line makes govulncheck
red on stdlib CVEs (sensor: TCC-S06; Dependabot's go-directive handling is unreliable —
dependabot-core #7895, and a `go_module_path_mismatch` with cel-go seen in this repo).

- Given any `actions/setup-go` step, when parsed, then it uses `go-version-file: go.mod`; none
  uses `stable`/a literal.
- Given each `go run …@vX`/`go install …@vX` pin, when the check runs under `GOTOOLCHAIN=local`,
  then its module `go` directive (read via `go mod download -json`, cheaper than building) is
  <= go.mod's Go, else red naming the tool.
- **Local breakage.** A `go install`ed golangci-lint built with an older Go panics on a newer
  stdlib (OPERATOR-BOARD item): `task lint` fails fast with a clear message when the binary's
  built-with Go is older than go.mod's Go (reinstall hint), and TCC-S03's mise ships the official
  binary.
- **Adversarial** — a new workflow with `go-version: stable`, or a tool needing a newer Go, is red.

Requirements:

- **REQ-TCC-S01-01** — `go.mod` has no `toolchain` line and no workflow has a `go-version:`
  literal (`release-exitgate` included): every CI path resolves to go.mod's Go. Test:
  `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-TCC-S01-02** *(adversarial)* — inline controls: reintroduced `stable`, reintroduced
  `toolchain` line. Test: same; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-TCC-S01-03** — tool-compatibility script (network; pinned `verify` step, and a
  `task check` stage that skips only when offline **with a loud message and red in CI**). Test:
  `hack/lint/pinned_tools_toolchain_test.sh` (create); Verify:
  `bash hack/lint/pinned_tools_toolchain_test.sh`; Level: L0
- **REQ-TCC-S01-04** *(adversarial)* — fixture module declaring a newer `go` is red. Test: same;
  Verify: `bash hack/lint/pinned_tools_toolchain_test.sh --self-test`; Level: L0
- **REQ-TCC-S01-05** — new `D-nnn` supersedes D-158's keep-`stable` clause and rewrites the
  `verify.yaml` coupling comment. Test: `docs/decisions/decisions.md`; Verify:
  `rg 'go-version-file' docs/decisions/decisions.md`; Level: doc
- **REQ-TCC-S01-06** — `task lint` fails fast on a golangci-lint built with an older Go. Test:
  `hack/lint/lint_binary_go_test.sh` (create); Verify:
  `bash hack/lint/lint_binary_go_test.sh --self-test`; Level: L0

---

## TCC-S02 — Pin baseline with floor semantics `[autonomous]`

**Depends on:** TCC-S01.

- Given the baseline, when read, then Task, golangci-lint, gitleaks match; git-cliff agrees in
  `Taskfile.yml:5` and `release.yaml:138`; govulncheck comes from one Taskfile var
  `GOVULNCHECK_VERSION` used by both workflows (>= v1.6.0; exact pin at implementation).
- Drift tests assert agree AND >= floor; an exact-value assertion is itself red (D10).

Requirements:

- **REQ-TCC-S02-01** — one literal per tool or all copies agree and >= floor. Test:
  `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-TCC-S02-02** *(adversarial)* — editing one of two copies reds (inline control). Test:
  same; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0

---

## TCC-S03 — Committed `mise.toml` + drift test `[autonomous]`

**Depends on:** TCC-S01, TCC-S02.

- `mise install` provides Task, golangci-lint (the **official release binary**, not a
  `go install` build), gitleaks, git-cliff, govulncheck at the CI pins, plus `uv`/Python as
  `Taskfile.yml` requires.
- **Go is not listed under `[tools]`** (D11): `[settings] idiomatic_version_file_enable_tools =
  ["go"]` makes mise read `go.mod`. A restated Go literal is red.
- Drift test (D10 semantics) fails naming tool and both values. D-002: no local paths/hosts.

Requirements:

- **REQ-TCC-S03-01** — `mise.toml` tracked, pins agree/>= floor, no Go under `[tools]`, setting
  present. Test: `hack/lint/mise_drift_test.sh` (create); Verify:
  `bash hack/lint/mise_drift_test.sh`; Level: L0
- **REQ-TCC-S03-02** *(adversarial)* — one-sided bump and a restated Go literal are red. Test:
  same; Verify: `bash hack/lint/mise_drift_test.sh --self-test`; Level: L0

---

## TCC-S04 — Task entry points `[autonomous]`

- `task check` is the **full local gate**: everything CI requires that can run locally; the
  exclusions (network-only/infra-gated like e2e, push-only evidence) are named in its `desc`.
- **No `task verify` as a fast subset** (D12). `verify` is reserved for "generated-artifact
  drift" if such a task exists; do not introduce it otherwise.

Requirements:

- **REQ-TCC-S04-01** — `task check` `desc` names its exclusions and no `verify` fast-subset
  alias exists. Test: `hack/lint/task_entrypoints_test.sh` (create); Verify:
  `bash hack/lint/task_entrypoints_test.sh`; Level: L0
- **REQ-TCC-S04-02** *(adversarial)* — a `verify:` task whose body is a subset of `check` is red.
  Test: same; Verify: `bash hack/lint/task_entrypoints_test.sh --self-test`; Level: L0

---

## TCC-S05 — Dependency bot choice `[OPEN QUESTION · operator]`

**Context.** Dependabot (`.github/dependabot.yml`: gomod, github-actions, pip, npm) cannot move
tool pins in `Taskfile.yml`, `env:` blocks, `go run …@vX` strings or `mise.toml`. `ci-audit-test`
(`hack/release/ci_audit_test.sh`) currently asserts "Dependabot-only; no Renovate config"
(operator 2026-08-13) — that guard changes under A or B.

- **A. Renovate (hosted app), Dependabot off.** One bot; regex managers keep tool pins and mise
  together. Cost: installing the hosted app (operator; no secrets); guard rewritten; loses
  GitHub security-update integration unless kept enabled.
- **B. Dependabot + Renovate regex managers only.** Keeps Dependabot's ecosystems and security
  updates; Renovate owns custom pins. Cost: two bots; guard allows Renovate only with
  `enabledManagers: [custom.regex]`.
- **C. Dependabot only (current).** No change; tool pins stay manual, protected only by the
  S02/S03 drift tests (stale pin = red CI, not silent).

**Recommendation: B**, fall back to **C** if the operator will not install the app.

Requirements:

- **REQ-TCC-S05-01** — a `D-nnn` row records A/B/C. Test: `docs/decisions/decisions.md`; Verify:
  `rg 'TCC-S05' docs/decisions/decisions.md`; Level: doc
- **REQ-TCC-S05-02** — `ci_audit_test.sh` is updated in the same change to assert the decision.
  Test: `hack/release/ci_audit_test.sh`; Verify: `bash hack/release/ci_audit_test.sh`; Level: L0

---

## TCC-S06 — Go patch-lag sensor `[blocked-by NIT-S02]`

Because Dependabot's go-directive handling is unreliable (see S01), a **scheduled** workflow
`go-patch-lag.yaml` (own workflow, own issue) compares go.mod's Go with the latest patch of its
minor (from the official release feed) and goes red when behind; NIT-S02's reporter opens the
issue. Keeping `go` bumps coherent after that is the bot decision (S05): under A/B a Renovate
`gomod` manager; under C, a manual bump driven by the sensor.

Requirements:

- **REQ-TCC-S06-01** — comparison logic table-tested (behind, equal, ahead, minor mismatch,
  feed failure = red). Test: `hack/ci/go_patch_lag_test.sh` (create); Verify:
  `bash hack/ci/go_patch_lag_test.sh`; Level: L0
- **REQ-TCC-S06-02** *(adversarial)* — a feed failure is red, never silently green. Test: same;
  Verify: `bash hack/ci/go_patch_lag_test.sh --self-test`; Level: L0

---

## Exit

S01–S04 and S06 landed; S05 answered or parked in the INBOX. Wiring per the
[gate-wiring rule](../README.md): every script/mode is a pinned `verify` step and a `task check`
stage with `STAGE_BODY_PINS` entries.
