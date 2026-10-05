# P5-CIH — CI hardening (green main, concurrency, workflow static analysis, dependency review)

**Epic ID / REQ prefix:** `CIH` / `REQ-CIH-Snn-nn`. Cross-cutting hygiene epic (same class as
`AUD` / `SEC-SC`). Does **not** consume E12–E14. Gate wiring: see the
[gate-wiring rule](../README.md) (D1) — every new guard and mode runs in the required `verify`
job and in `task check`.

**Problem.** Verified 2026-10-05: `verify.yaml` has two jobs (`verify` L38, `release-exitgate`
L213, push/schedule only) and **no `concurrency:`**; there is no workflow security linter beyond
`actionlint` and the pin tests in `hack/lint/workflow_pins_test.sh`; no `dependency-review-action`
on PRs; and **main is red at `1c4e71e`**: step "AUD audit exit gate (AUD-S18)" reports
`TestDeterminismDoubleRun passed 0 time(s) under -count=2` — a guard only push builds run, so no
PR could have caught it. Prior art: attune (`github.com/attune-io/attune`) `ci.yaml`.

**Pin-test convention.** `workflow_pins_test.sh` has **no argument handling**; its mutation
controls run inline on every plain invocation (already dozens — re-count at implementation and
assert the count only grows, never a literal). Extend it by adding controls; verify with
`bash hack/lint/workflow_pins_test.sh`. `--self-test` exists only on scripts an epic creates.

**Counterpoint (decided).** attune drops CI on push to `main` (squash-merge). assent **keeps**
push-to-main `verify`: `hack/release/verify-tag-gate.sh` (REQ-AUD-S03-01) needs a green
`verify.yaml` run on the tag SHA and merges are rebase-merge. Forbidden outcome (REQ-CIH-S03-03).

**Not in scope:** SHA-pin policy (exists); CodeQL/Scorecard changes; branch-protection edits other
than the `[operator]` steps named below.

**Deferred (not built here).** `step-security/harden-runner`: audit mode gates nothing, adds a
privileged third-party agent to token-holding jobs, and nobody reads the telemetry. Revive only
with a written **block-mode egress allowlist plan**. All harden-runner REQs were removed.

---

## CIH-S00 — Restore green main: AUD-S18 determinism guard reports 0 runs `[autonomous]`

**Depends on:** none. **First: every later epic's exit needs a green push `verify` on main.**

- Given main at `1c4e71e`, when the `release-exitgate` job runs the AUD audit exit gate, then
  `TestDeterminismDoubleRun` executes under `-count=2` and the gate is green. Root cause is
  **diagnosed first** (why 0 runs: filter/package/build-tag/`-run` regex drift) and recorded in
  the commit body; do not weaken the gate (loosening needs its own justification line).
- Given a PR, when `verify` runs, then the determinism double-run evidence check also runs on the
  PR path (argument-pinned step), so the class "guard only push builds run" cannot recur.
- **Adversarial** — a change that makes `TestDeterminismDoubleRun` match zero tests is red on the PR.

Requirements:

- **REQ-CIH-S00-01** — the double-run test executes twice. Test:
  `hack/audit/exitgate_test.sh`; Verify:
  `go test -count=2 -run '^TestDeterminismDoubleRun$' -v ./... | rg -c '^--- PASS: TestDeterminismDoubleRun'`
  prints `2`; Level: L0
- **REQ-CIH-S00-02** *(adversarial)* — a PR-path step fails on "passed 0 time(s)". Test:
  `hack/audit/exitgate_test.sh` + `hack/lint/workflow_pins_test.sh`; Verify:
  `bash hack/audit/exitgate_test.sh && bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S00-03** — push `verify` on main is green on the fix commit. Test: GitHub run;
  Verify: `gh run list -R PlatformRelay/assent -w verify -b main -L1 --json conclusion`; Level: L3 (**post-merge**, D14)

---

## CIH-S01 — `concurrency:` on `verify` `[autonomous]`

**Depends on:** none.

- Given a PR with a newer push, then the in-flight run for the same PR is cancelled; given a
  push to `main`, a tag, a schedule or `workflow_dispatch`, then nothing is cancelled.
- Key (D5): `group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.sha }}`
  (not `head_ref`: fork branches named alike would collide); `cancel-in-progress:
  ${{ github.event_name == 'pull_request' }}`.
- **Adversarial** — unconditional `cancel-in-progress: true` or a `head_ref` key is red (a
  cancelled `main` run leaves the tag SHA without green `verify`).

Requirements:

- **REQ-CIH-S01-01** — block present with the D5 key and condition. Test:
  `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S01-02** *(adversarial)* — new inline controls for unconditional cancel and
  `head_ref` key are red on the mutated copy; plain run stays green and lists them. Test: same;
  Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0

---

## CIH-S02 — `zizmor --offline` gate; existing findings fixed `[autonomous]`

**Depends on:** none.

- Given the workflows, when `zizmor --offline --min-severity <pinned> .github/workflows` runs
  (one exact pinned version >= 1.30.1, single-sourced in the workflow `env:` block; installed
  via `uvx zizmor==X` or pipx `--require-hashes`, **not** `go install`), then findings fail.
- Today offline zizmor reports **2 high `cache-poisoning`** in `release.yaml` (setup-go cache in
  the tag-triggered publish path) and 1 informational `superfluous-actions`. These are
  **fixed**, not suppressed (`cache: false` on those `setup-go` steps; drop the superfluous
  action). Any suppression in `.github/zizmor.yml` carries a `# why:` line; no blanket ignores.
- **No skip switch** (`ZIZMOR_SKIP*`/equivalent) may appear in any workflow (guarded).
- Later changes that add workflows (e.g. NIT-S02's `workflow_run` reporter -> `dangerous-triggers`)
  must pass this gate in their own tasks.
- Fixture seeds a **third-party** mutable-ref action (`actions/*` ref pins are allowed by default).
- Local: `task check` requires `uvx` like it already requires `uv` + Python 3.12 (`Taskfile.yml`
  comment ~L156); no silent skip.

Requirements:

- **REQ-CIH-S02-01** — gate step in `verify.yaml`, pinned version >= floor, `--min-severity` set,
  no `if:`/`continue-on-error`. Test: `hack/lint/zizmor_gate_test.sh` (create); Verify:
  `bash hack/lint/zizmor_gate_test.sh`; Level: L0
- **REQ-CIH-S02-02** — `release.yaml` cache-poisoning findings fixed with zero suppressions for
  them. Test: same; Verify: `bash hack/lint/zizmor_gate_test.sh --self-test`; Level: L0
- **REQ-CIH-S02-03** *(adversarial)* — third-party mutable-ref fixture flagged; suppression
  without `# why:` red; any skip env in a workflow red. Test: same; Verify:
  `bash hack/lint/zizmor_gate_test.sh --self-test`; Level: L0

**Counterpoint.** Overlaps `actionlint` + pin tests on pinning; zizmor adds template injection,
permissions and `pull_request_target`/`workflow_run` audits.

---

## CIH-S03 — `dependency-review-action` on PRs; push-trigger fence `[autonomous · operator for required check]`

**Depends on:** none.

- Given a PR that adds/bumps a dependency, when the `pull_request`-only job runs, then
  `actions/dependency-review-action` (SHA-pinned) fails at the action's **default
  `fail-on-severity` (low)**; loosening is allowed only after a measured noisy trial and with its
  own `# why:` line.
- Licences: `allow-licenses` (Apache-2.0-compatible set) plus per-module
  `allow-dependencies-licenses` each with a reason — **not** the deprecated `deny-licenses`
  (action issue #938). Unknown licences are reported, not failed.
- Works on Dependabot PRs without secrets. `[operator]`: make it a **required** check.
- **Forbidden outcome** — `verify.yaml` losing `push.branches: [main]` or the weekly schedule.

Requirements:

- **REQ-CIH-S03-01** — PR-only job, pinned, no `continue-on-error`, `allow-licenses` present,
  no `deny-licenses`. Test: `hack/lint/workflow_pins_test.sh`; Verify:
  `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S03-02** *(adversarial)* — a severity override without `# why:` or the step dropped
  is red (inline control). Test: same; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S03-03** *(forbidden outcome)* — `on:` keeps `push.branches: [main]` and `schedule`
  with a comment citing `verify-tag-gate.sh`. Test: same; Verify:
  `bash hack/lint/workflow_pins_test.sh`; Level: L0

---

## CIH-S04 — `ci-gate` aggregate job `[deferred]`

Path-filtered workflows already exist (`docs.yaml`, `schemas.yml`, `actionlint.yaml`,
`release.yaml` use `paths:`), but `ci-gate` **cannot aggregate other workflows**, and
CodeQL stays a separate required check. Bot auto-merge safety is handled directly in DEP-S02
(all check runs on the head SHA + green main), so this story stays deferred. **Trigger:** a new
PR-only job inside `verify.yaml` that needs aggregating. If built: `if: always()`, graded on
`contains(needs.*.result, 'failure') || contains(needs.*.result, 'cancelled')` (never
`failure()`), every sibling job in `needs`, `skipped` = pass, required beside `verify` and CodeQL.
Requirements are written then.

---

## Exit

S00–S03 green on a push `verify` at main; wiring per the [gate-wiring rule](../README.md): every
new script/mode is a pinned `verify` step and a `task check` stage with `STAGE_BODY_PINS`
entries. `release-exitgate` is a **job** in `verify.yaml`, not a workflow.
