# P5-TCC — Toolchain consistency (one source of truth for tool versions)

**Epic ID / REQ prefix:** `TCC` / `REQ-TCC-Snn-nn`. Cross-cutting hygiene epic.

**Problem.** The operator wants the same tools, the same pins and the same entry-point names
across their repositories. Verified 2026-10-05 in this repo:

| Item | Current | Target |
| --- | --- | --- |
| Go in workflows | `go-version: stable` x **6** (`verify.yaml` x2, `codeql.yaml`, `schemas.yml`, `vulncheck.yaml`) vs `go-version-file: go.mod` in `release.yaml` x2 | `go-version-file: go.mod` everywhere |
| `go.mod` | `go 1.26.0`, `toolchain go1.26.6` | unchanged (the single Go source) |
| Task | `v3.52.0` (`verify.yaml` `env`) | same |
| golangci-lint | `v2.13.1` (`env`) | same |
| gitleaks | `v8.30.1` (`go run …@v8.30.1`) | same |
| git-cliff | `v2.13.1` (only an example in `hack/install-git-cliff.sh`; real pin in `release.yaml`/Taskfile — verify at implementation) | `v2.13.1` |
| govulncheck | **already `v1.6.0`** (`verify.yaml`, `vulncheck.yaml`) | latest released (>= v1.6.0), pin chosen at implementation |
| `mise.toml` | **absent from the tree** (referenced by `Taskfile.yml` comments as a local file) | committed, mirrors CI pins |
| Task entry points | `task check` exists; no `task verify` | `check` = full gate = CI; `verify` = open question |
| Dependency bot | Dependabot only; `hack/release/ci_audit_test.sh` asserts "no Renovate config" | OPEN QUESTION |

**Not in scope:** changing the audit/AUD gates' content; Python/uv pins for docs (already in
`docs/`); a Nix flake; Dependabot/Renovate switch decision's implementation until decided.

---

## TCC-S01 — `go-version-file: go.mod` in all workflows `[autonomous]`

**Depends on:** none. **Read D-158 first.**

**Counterpoint, honestly.** D-158 chose to KEEP `go-version: stable` because (a) golangci-lint
must be built with a Go >= the one whose stdlib it typechecks (a `stable` roll to 1.27
broke every PR), and (b) `govulncheck` reports stdlib vulnerabilities of the *running*
toolchain, so `stable` auto-picks up stdlib CVE fixes. `go-version-file: go.mod` **removes
(a) as a failure mode** (the toolchain only moves when someone edits `go.mod`, so linter
and Go cannot skew behind our back) and gives reproducible CI, but **transfers (b) to
us**: a stale `toolchain` line makes govulncheck red on a stdlib CVE until the line is
bumped. The trade is: surprise reds from upstream rolls -> deliberate reds that name the
exact fix (bump `toolchain`). Mitigation: the Dependabot `gomod` ecosystem proposes
`toolchain` bumps (verify at implementation; if not, TCC-S01-03 adds a drift note). D-158's
coupling comment is rewritten, not deleted: the linter must still be >= the toolchain's
minor, now checked when `toolchain` is bumped. Needs a new `D-nnn` row that supersedes the
"keep stable" half of D-158 (the golangci-lint bump half stands).

- Given any workflow using `actions/setup-go`, when parsed, then it uses
  `go-version-file: go.mod` and none uses `go-version: stable`/`latest`.
- Given `go.mod`'s `toolchain` older than the newest linter-supported Go, when `task check`
  runs, then the build is still green (no coupling surprise).
- **Adversarial** — given a new workflow adding `go-version: stable`, when the pin test
  runs, then red.

Requirements:

- **REQ-TCC-S01-01** — zero `go-version: stable` in `.github/workflows/*`. Test:
  `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-TCC-S01-02** *(adversarial)* — failing-direction self-test for a reintroduced
  `stable`. Test: same; Verify: `… --self-test`; Level: L0
- **REQ-TCC-S01-03** — new `D-nnn` supersedes D-158's keep-`stable` clause and updates the
  `verify.yaml` coupling comment. Test: `docs/decisions/decisions.md`; Verify:
  `rg 'go-version-file' docs/decisions/decisions.md`; Level: doc

---

## TCC-S02 — Pin baseline reconciled and single-sourced `[autonomous]`

**Depends on:** none.

- Given the baseline table above, when the repo is read, then Task, golangci-lint, gitleaks
  already match; git-cliff is `v2.13.1` wherever pinned; govulncheck is bumped to the latest
  released release (pin recorded) in **both** `verify.yaml` and `vulncheck.yaml` at once.
- Given a version literal that exists in two files, when the pin test runs, then it reds
  unless both agree (existing pattern: `TASK_VERSION`, `GOLANGCI_LINT_VERSION`).

Requirements:

- **REQ-TCC-S02-01** — each tool has one literal pin (or all copies agree) at the baseline
  values. Test: `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-TCC-S02-02** *(adversarial)* — editing one of two govulncheck pins alone reds the test. Test:
  same; Verify: `… --self-test`; Level: L0

---

## TCC-S03 — Committed `mise.toml` + drift test `[autonomous]`

**Depends on:** TCC-S01, TCC-S02.

- Given a fresh checkout, when `mise install` runs, then it provides Task, golangci-lint,
  gitleaks, git-cliff, govulncheck at the CI pins; Go is read from `go.mod` via mise's
  idiomatic version-file setting (`go = "go.mod"` / `idiomatic_version_file`), **not
  restated**; `uv`/Python as `Taskfile.yml` already requires.
- Given `mise.toml` disagreeing with any workflow pin, when the drift test runs in CI,
  then red naming the tool and both values.
- **Adversarial** — given `mise.toml` restating a Go version literal, then red.
- Sanitization (D-002): no local paths, hostnames or private registries in `mise.toml`.

Requirements:

- **REQ-TCC-S03-01** — `mise.toml` tracked, pins match workflows, no Go literal. Test:
  `hack/lint/mise_drift_test.sh` (create); Verify: `bash hack/lint/mise_drift_test.sh`; Level: L0
- **REQ-TCC-S03-02** *(adversarial)* — bump a pin on one side only: red (failing-direction
  self-test). Test: same; Verify: `… --self-test`; Level: L0
- **REQ-TCC-S03-03** — wired into `task check` and `hack/audit/exitgate_test.sh`
  `CHECK_STAGES` deliberately (the gate pins the stage count). Test:
  `hack/audit/exitgate_test.sh`; Verify: `bash hack/audit/exitgate_test.sh`; Level: L0

**Counterpoint.** A fourth place that names versions is a new drift surface — acceptable
only because the drift test makes it mechanical. If the dependency-bot decision (S05)
picks Renovate regex managers, they keep it moving too.

---

## TCC-S04 — Standard task entry points `[autonomous · open question]`

**Depends on:** none.

- Given every repo, when a contributor runs `task check`, then it is the full gate that CI runs
  (assent already satisfies this).
- **Open question Q1:** add `task verify` as an alias of the *fast subset* (lint + unit tests,
  no audit/exit gates, no Python docs)? Recommended **yes**, named in `Taskfile.yml` desc
  as "fast pre-push subset; `check` is the full gate". Risk: `verify` is also the CI
  workflow/job name and `verify-tag-gate.sh` keys on `verify.yaml` — name overlap is
  confusing, not breaking. Alternative: `task fast`. Operator decides; do not implement
  before an answer.

Requirements (conditional on Q1=yes):

- **REQ-TCC-S04-01** — `task verify` exists and its stages are a strict subset of `check`'s.
  Test: `hack/lint/task_entrypoints_test.sh` (create); Verify:
  `bash hack/lint/task_entrypoints_test.sh`; Level: L0
- **REQ-TCC-S04-02** *(adversarial)* — a stage in `verify` that is absent from `check` reds. Test: same;
  Verify: `… --self-test`; Level: L0

---

## TCC-S05 — Dependency bot choice `[OPEN QUESTION · operator]`

**Depends on:** none. **Not decided here.**

**Context.** Dependabot (current) moves `gomod`, `github-actions`, `pip`, `npm`
(`.github/dependabot.yml`) but cannot move tool pins in `Taskfile.yml`, `env:` blocks,
`go run …@vX` strings or `mise.toml`; those rot unless a bot with regex managers keeps them.
`Taskfile.yml` target `ci-audit-test` (via `hack/release/ci_audit_test.sh`) currently asserts
"Dependabot-only; no Renovate config" (operator 2026-08-13) — **that guard changes under
B or A**.

- **A. Renovate (GitHub App token), Dependabot off.** One bot, regex managers move
  Taskfile/workflow/mise pins together, grouped. Cost: App + secrets (operator), guard
  rewritten, loses Dependabot security-update integration unless kept enabled.
- **B. Dependabot + Renovate for regex managers only (attune's approach).** Dependabot keeps
  the ecosystems it is good at and GitHub security updates; Renovate only owns custom pins.
  Cost: two bots, two PR streams, App needed; guard changes to "Renovate config may exist
  only with `enabledManagers: [custom.regex]`".
- **C. Dependabot only (current).** No change; tool pins stay manual, protected only by the
  S02/S03 drift tests, which turn a stale pin into a red CI rather than a silent one.

**Recommendation: B**, because it keeps the security-update path the project already trusts
and fixes exactly the gap Dependabot cannot; fall back to **C** if the operator will not
create the App. Counterpoint to A: single bot is simpler but replaces a working updater.

Requirements (any option; verify via the chosen guard):

- **REQ-TCC-S05-01** — an operator decision row `D-nnn` records A/B/C. Test:
  `docs/decisions/decisions.md`; Verify: `rg 'TCC-S05' docs/decisions/decisions.md`; Level: doc
- **REQ-TCC-S05-02** — `ci_audit_test.sh` is updated in the same change to assert whatever the
  decision is (never left asserting the old state). Test: `hack/release/ci_audit_test.sh`;
  Verify: `bash hack/release/ci_audit_test.sh`; Level: L0

---

## Exit

S01–S03 landed; S04/S05 either implemented per operator answer or explicitly parked in the
INBOX. `task check` and `release-exitgate` green with `go-version-file`.
