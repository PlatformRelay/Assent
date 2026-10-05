# P5-CIH — CI hardening (runner egress, workflow static analysis, PR dependency review, aggregate gate)

**Epic ID / REQ prefix:** `CIH` / `REQ-CIH-Snn-nn`. Cross-cutting hygiene epic (same class as
`AUD` / `SEC-SC`). Does **not** consume E12–E14.

**Problem.** Verified 2026-10-05 at `1c4e71e`: `.github/workflows/verify.yaml` has two jobs
(`verify` L38, `release-exitgate` L213) and **no `concurrency:`** (`scorecard`, `docs`,
`actionlint`, `vulncheck`, `codeql`, `release` all have one); there is no runner egress
control, no workflow security linter beyond `actionlint` and the pin tests in
`hack/lint/workflow_pins_test.sh`, and no `dependency-review-action` on PRs. External prior
art: attune (`github.com/attune-io/attune`) `.github/workflows/ci.yaml`, including its
`ci-gate` job.

**Self-guarding convention, stated precisely.** `workflow_pins_test.sh` has **no argument
handling**: its mutation controls (D-177 counts 15) run inline on **every plain
invocation**, so the way to extend it is to add controls to that file and verify with
`bash hack/lint/workflow_pins_test.sh` (which must run all controls and exit 0). A
`--self-test` flag exists **only** on scripts this epic creates, and each such script's
REQs spell out the full command.

**Counterpoint (recorded, decided here).** attune drops CI on push to `main` because it
squash-merges. assent **keeps** push-to-main `verify`: `hack/release/verify-tag-gate.sh`
(REQ-AUD-S03-01) requires a green `verify.yaml` run on the **tag SHA**, and merges are
rebase-merge (workspace policy), so the tagged commit is a push-to-main commit. Dropping the
push trigger would make every release fail its own gate: forbidden outcome (REQ-CIH-S05-03).
Concurrency must never cancel `main`/tag runs.

**Not in scope:** SHA-pin policy (exists); CodeQL/Scorecard changes; `harden-runner` in
`block` mode; changing which checks are required in repo settings except as an `[operator]`
step in S04.

---

## CIH-S01 — `concurrency:` cancel-in-progress for PR refs on `verify` `[autonomous]`

**Depends on:** none. Do first.

- Given a PR with a newer push, when `verify` triggers again, then the in-flight run for the
  same PR ref is cancelled.
- Given a push to `main`, a tag or a scheduled run, when runs overlap, then none is
  cancelled (`cancel-in-progress` only for `pull_request`).
- **Adversarial** — given an edit making `cancel-in-progress` unconditional, when the pin
  test runs, then it is red (a cancelled `main` run leaves the tag SHA without a green
  `verify`).

Requirements:

- **REQ-CIH-S01-01** — `verify.yaml` declares top-level `concurrency` keyed on
  workflow + `github.head_ref || github.run_id`, `cancel-in-progress: ${{ github.event_name == 'pull_request' }}`.
  Test: `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S01-02** *(adversarial)* — a new inline control in that file (unconditional
  cancel / missing block) must be red on the mutated copy and the plain run stays green.
  Test: `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`
  (output must list the new control; control count rises from 15); Level: L0

**Not in scope:** `release.yaml` concurrency (exists).

---

## CIH-S02 — `zizmor --offline` gate with a justified suppression file `[autonomous]`

**Depends on:** none.

- Given the workflows, when `zizmor --offline .github/workflows` runs in CI, then findings
  fail the job. zizmor is a Python/Rust tool, **not a Go module**: install via
  `uvx zizmor==X.Y.Z` (or `pipx` with `--require-hashes`); the version is single-sourced in
  the workflow `env:` block like `TASK_VERSION`.
- Given an accepted finding, when suppressed, then it lives only in committed
  `.github/zizmor.yml`, each entry carrying a `# why:` line (gate-weakening rule: loosening a
  check needs its own justification).
- **Adversarial** — a suppression with no `# why:`, or a blanket rule/workflow ignore, is red.
- **Adversarial** — the zizmor step removed, `continue-on-error`, or given an `if:` is red.
- **Adversarial fixture** — the seeded bad workflow under `hack/lint/testdata/` pins a
  **third-party** action by mutable ref (`uses: some-org/some-action@v1`) — zizmor's default
  unpinned-uses policy allows ref pins for `actions/*`, so an `actions/*` fixture would not
  fire.

Requirements:

- **REQ-CIH-S02-01** — zizmor offline runs in `verify.yaml`, version pinned, no
  `continue-on-error`/`if:`. Test: `hack/lint/zizmor_gate_test.sh` (create); Verify:
  `bash hack/lint/zizmor_gate_test.sh`; Level: L0
- **REQ-CIH-S02-02** — every `.github/zizmor.yml` ignore has an adjacent `# why:`; zero
  blanket ignores. Test: `hack/lint/zizmor_gate_test.sh`; Verify:
  `bash hack/lint/zizmor_gate_test.sh --self-test`; Level: L0
- **REQ-CIH-S02-03** *(adversarial)* — the third-party mutable-ref fixture is flagged. Test:
  `hack/lint/zizmor_gate_test.sh`; Verify: `bash hack/lint/zizmor_gate_test.sh --self-test`; Level: L0

**Counterpoint.** `actionlint` + pin tests overlap on pinning; zizmor adds template
injection, excessive permissions and `pull_request_target`/`workflow_run` audits. Accept the
overlap.

**Not in scope:** online audits; fixing findings beyond what turns the gate green (one
commit each).

---

## CIH-S03 — `step-security/harden-runner` in egress audit mode `[autonomous]`

**Depends on:** CIH-S02.

- Given each job in `verify.yaml` (both jobs) and `vulncheck.yaml`, when it starts, then
  `harden-runner` is the first step with `egress-policy: audit`, SHA-pinned.
- **Forbidden outcome** — `egress-policy: block` without an allowlist derived from >= one
  week of audit data.

Requirements:

- **REQ-CIH-S03-01** — every job in the named workflows starts with pinned harden-runner at
  `audit`. Test: `hack/lint/workflow_pins_test.sh`; Verify:
  `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S03-02** *(adversarial)* — inline controls: step removed from a job, or `block`
  without an allowlist file, are red on the mutated copy. Test: same file; Verify:
  `bash hack/lint/workflow_pins_test.sh`; Level: L0

**Counterpoint.** Audit only logs; the audit-to-block follow-up is an INBOX item, not a promise.

---

## CIH-S04 — `ci-gate` aggregate job `[deferred · autonomous when triggered]`

**Reality check (review finding 7).** The earlier premise — "a path-filtered job cannot be
required" — is **not grounded today**: `verify.yaml` has two jobs, no path filters, and the
always-run `verify` job already works as a required check. Required checks on `main` also
include CodeQL (a separate workflow); **`ci-gate` cannot aggregate other workflows and does
not replace CodeQL, which stays required.** So S04 is scoped narrowly:

**Trigger (do S04 only when true):** a new PR-only or path-filtered job is added (the first
candidate is S05's dependency-review job), so a required check could be absent/skipped.
Until then S04 stays deferred; do not add a job whose only purpose is to exist.

**Depends on:** CIH-S05 (or any new filtered job). Settings step is `[operator]`.

- Given `verify.yaml` with a `ci-gate` job `needs:` all other jobs and `if: always()`, when
  it evaluates, then it is **red iff** `contains(needs.*.result, 'failure') ||
  contains(needs.*.result, 'cancelled')`; `skipped` and `success` are green.
- **`if: always()` pitfall** — a job with `if: always()` runs even when the workflow is
  cancelled, and `failure()` is false for a cancelled run; grading must use the
  `contains(needs.*.result, …)` form above, never `failure()`.
- **Adversarial** — a job added without being listed in `ci-gate.needs` is red.
- **Adversarial** — `failure`/`cancelled` is never masked by skipped-as-pass.
- `[operator]` — add `ci-gate` as a required check **beside** `verify` and CodeQL; do not
  remove either.

Requirements:

- **REQ-CIH-S04-01** — `ci-gate` has `if: always()` and `needs` covering all other
  `verify.yaml` jobs. Test: `hack/lint/ci_gate_test.sh` (create); Verify:
  `bash hack/lint/ci_gate_test.sh`; Level: L0
- **REQ-CIH-S04-02** *(adversarial)* — the grading expression/script is table-tested incl.
  `cancelled`, and the file contains no `failure()` grading. Test:
  `hack/lint/ci_gate_test.sh`; Verify: `bash hack/lint/ci_gate_test.sh --self-test`; Level: L0
- **REQ-CIH-S04-03** — operator step recorded in a `D-nnn` row. Test:
  `docs/decisions/decisions.md`; Verify: `rg 'ci-gate' docs/decisions/decisions.md`; Level: doc

**Counterpoint.** Skipped-as-pass at the job level does not re-prove step-level skips
(Sonar on Dependabot PRs, D-177/D-178); those stay governed by their own pins.

---

## CIH-S05 — `dependency-review-action` on PRs; push-to-main fence `[autonomous]`

**Depends on:** none.

- Given a PR that adds/bumps a dependency, when the `pull_request`-only job runs, then
  `actions/dependency-review-action` (SHA-pinned) fails on new vulnerabilities at or above
  the configured severity. The action's **default `fail-on-severity` is `low`**; choosing
  `high` is itself a **loosening that needs its own `# why:` line** (operator's
  gate-weakening rule). Default decision: leave the default (`low`) unless the first
  trial run shows noise, then loosen with the justification.
- Licence handling: Go modules with an unknown/unrecognised licence would false-red an
  allow-list; use a **deny-list** (copyleft incompatible with Apache-2.0) and
  `allow-dependencies-licenses` entries per module, each with a `# why:`; unknown licence
  is reported, not failed.
- Given a Dependabot PR, then the job works without secrets.
- **Forbidden outcome** — `verify.yaml` losing `push.branches: [main]` or the weekly schedule.

Requirements:

- **REQ-CIH-S05-01** — job exists on `pull_request` only, pinned, no `continue-on-error`.
  Test: `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S05-02** *(adversarial)* — a `fail-on-severity` other than the default without an
  adjacent `# why:`, or the step dropped, is red (inline control). Test: same file; Verify:
  `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S05-03** *(forbidden outcome)* — `on:` keeps `push.branches: [main]` and
  `schedule`, with a comment citing `verify-tag-gate.sh`. Test: same file; Verify:
  `bash hack/lint/workflow_pins_test.sh`; Level: L0

**Not in scope:** SBOM; Dependabot config (see DEP).

---

## Exit

S01–S03 and S05 green. New `hack/lint/*` scripts (`zizmor_gate_test.sh`, later
`ci_gate_test.sh`) hook into an **existing** `task check` stage (the one running
`workflow_pins_test.sh`), so `CHECK_STAGES` in `hack/audit/exitgate_test.sh` is unchanged;
only a script that becomes a **new `check:` stage** needs a deliberate pin
(`exitgate_test.sh:539-554`). `release-exitgate` is a **job** in `verify.yaml`, not a
workflow.
