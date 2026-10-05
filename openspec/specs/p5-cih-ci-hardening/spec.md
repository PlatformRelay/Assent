# P5-CIH — CI hardening (runner egress, workflow static analysis, PR dependency review, aggregate gate)

**Epic ID / REQ prefix:** `CIH` / `REQ-CIH-Snn-nn`. Cross-cutting hygiene epic (same class as
`AUD` / `SEC-SC`). Does **not** consume E12–E14.

**Problem.** Verified 2026-10-05 at `1c4e71e`: `.github/workflows/verify.yaml` has no
`concurrency:` (superseded PR pushes burn runners; `scorecard`, `docs`, `actionlint`, `vulncheck`,
`codeql`, `release` all have one); there is no runner egress control, no workflow security
linter beyond `actionlint` and the pin tests in `hack/lint/workflow_pins_test.sh`, no
`dependency-review-action` on PRs, and no single always-run job that branch protection can
require — `verify` has path-sensitive and event-sensitive steps, so a required check cannot
be added for a path-filtered job without risking a permanent "pending". External prior art:
attune (`github.com/attune-io/attune`) `.github/workflows/ci.yaml` job `ci-gate`.

**Why now:** assent is the gate other repos trust; its own CI is its supply chain (D-045,
AUD-S09 pins). Each control below is cheap and each is the kind that rots silently, so each
ships with a self-guarding test in the established `hack/lint/*_test.sh` style.

**Counterpoint (recorded, decided here).** attune drops CI on push to `main` because it
squash-merges. assent **keeps** push-to-main `verify`: `hack/release/verify-tag-gate.sh`
(REQ-AUD-S03-01) requires a green `verify.yaml` run on the **tag SHA**, and merges are
rebase-merge (workspace policy), so the tagged commit is a push-to-main commit, never a PR
head. Dropping the push trigger would make every release fail its own gate. That is a
forbidden outcome (REQ-CIH-S05-03). Concurrency must also never cancel `main`/tag runs.

**Not in scope:** SHA-pin policy (exists, `workflow_pins_test.sh`); CodeQL/Scorecard changes;
enforcing `harden-runner` in `block` egress mode (audit-first only; blocking is a follow-up
after a week of audit data); making `ci-gate` the repo-settings required check (that is an
`[operator]` step in S04).

**Lanes:** **A** `verify.yaml` + `.github/workflows/*` · **B** `hack/lint/*` self-guards.

---

## CIH-S01 — `concurrency:` cancel-in-progress for PR refs on `verify` `[autonomous]`

**Depends on:** none. **Do first** (smallest, unblocks cheaper CI for the rest).

- Given a PR with a newer push, when `verify` is triggered again, then the in-flight run for
  the same PR ref is cancelled.
- Given a push to `main`, a tag, or a scheduled run, when runs overlap, then none is
  cancelled (`cancel-in-progress` is true only for `pull_request`).
- **Adversarial** — given a workflow edit that makes `cancel-in-progress` unconditional,
  when the pin test runs, then it is red (a cancelled `main` run would leave the tag SHA
  without a green `verify`, breaking `verify-tag-gate.sh`).

Requirements:

- **REQ-CIH-S01-01** — `verify.yaml` declares top-level `concurrency` keyed on workflow +
  `github.head_ref || github.run_id`, with `cancel-in-progress: ${{ github.event_name == 'pull_request' }}`.
  Test: `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S01-02** *(adversarial)* — the pin test has a failing-direction self-test: an
  unconditional `cancel-in-progress: true` or a missing block reds it. Test:
  `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh --self-test`; Level: L0

**Not in scope:** concurrency on `release.yaml` (already has one; do not touch).

---

## CIH-S02 — `zizmor --offline` gate with a justified suppression file `[autonomous]`

**Depends on:** none.

- Given the workflows, when `zizmor --offline .github/workflows` runs in CI (version pinned
  single-source in the `env:` block like `TASK_VERSION`), then findings fail the job.
- Given a finding that is accepted, when it is suppressed, then it lives only in a committed
  `.github/zizmor.yml`, each entry carrying a `# why:` justification line (gate-weakening
  rule: loosening a check needs its own justification).
- **Adversarial** — given a suppression with no `# why:` comment, or a blanket `ignore`
  of a whole rule/workflow, when the guard test runs, then it is red.
- **Adversarial** — given the zizmor step is removed, made `continue-on-error`, or given an
  `if:`, when the guard runs, then it is red.

Requirements:

- **REQ-CIH-S02-01** — zizmor runs offline in `verify.yaml`, pinned (version + digest or
  `go install`/`uvx` with exact version), no `continue-on-error`/`if:`. Test:
  `hack/lint/zizmor_gate_test.sh` (create); Verify: `bash hack/lint/zizmor_gate_test.sh`; Level: L0
- **REQ-CIH-S02-02** — every `.github/zizmor.yml` ignore entry has an adjacent `# why:`; zero
  rule-wide or workflow-wide ignores. Test: same script; Verify:
  `bash hack/lint/zizmor_gate_test.sh --self-test`; Level: L0
- **REQ-CIH-S02-03** *(adversarial)* — a seeded unpinned-`uses:`/template-injection fixture
  under `hack/lint/testdata/` is flagged (proves the gate can fail). Test: same script;
  Verify: `bash hack/lint/zizmor_gate_test.sh --self-test`; Level: L0

**Counterpoint.** `actionlint` + pin tests already overlap on pinning; zizmor adds
template-injection, excessive-permissions and `pull_request_target` audits. Accept the
overlap; drop only checks proven fully redundant, with a justification line.

**Not in scope:** online audits (`--offline` only, no token); fixing findings beyond what
the gate needs to go green (separate fixes, one commit each).

---

## CIH-S03 — `step-security/harden-runner` in egress audit mode `[autonomous]`

**Depends on:** CIH-S02 (so the new `uses:` is itself linted).

- Given each job in `verify.yaml`, `release-exitgate`, and `vulncheck`, when it starts, then
  `harden-runner` is its first step with `egress-policy: audit`, SHA-pinned.
- Given a job that is missing it, when the guard runs, then red.
- **Forbidden outcome** — `egress-policy: block` shipping without an allowlist derived from
  at least one week of audit data (an outage vector for a gate other repos depend on).

Requirements:

- **REQ-CIH-S03-01** — every job in the named workflows starts with a pinned harden-runner
  at `egress-policy: audit`. Test: `hack/lint/workflow_pins_test.sh`; Verify:
  `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S03-02** *(adversarial)* — removing the step from any job, or setting `block`
  without an allowlist file, reds the guard; failing-direction self-test. Test: same;
  Verify: `bash hack/lint/workflow_pins_test.sh --self-test`; Level: L0

**Counterpoint.** Audit mode only logs; value is realised only if someone reads it. S03
therefore records the audit-to-block follow-up as an INBOX item, not a promise.

**Not in scope:** block mode; third-party SaaS dashboard setup (`[operator]` if desired).

---

## CIH-S04 — Always-run aggregate job `ci-gate` `[autonomous · operator for settings]`

**Depends on:** none (autonomous part). The repo-settings switch is `[operator]`.

- Given `verify.yaml`, when it runs, then a final `ci-gate` job has `needs:` every other job,
  `if: always()`, and fails iff any needed job's result is `failure` or `cancelled`;
  `skipped` counts as pass (prior art: attune `ci.yaml` `ci-gate`).
- Given a path-filtered or event-filtered job skipped, when `ci-gate` runs, then it passes.
- **Adversarial** — given a new job added without being listed in `ci-gate.needs`, when the
  guard runs, then red (otherwise a new job is silently unrequired).
- **Adversarial** — given the job result is `failure`, when `ci-gate` evaluates, then it
  fails; a `skipped`-as-pass rule must never mask `failure`/`cancelled`.
- `[operator]` — switch required status checks from `verify` to `ci-gate` (repo settings;
  keep `verify` required until `ci-gate` has one green PR + one green push).

Requirements:

- **REQ-CIH-S04-01** — `ci-gate` exists with `if: always()` and `needs` ⊇ all other
  `verify.yaml` jobs. Test: `hack/lint/ci_gate_test.sh` (create); Verify:
  `bash hack/lint/ci_gate_test.sh`; Level: L0
- **REQ-CIH-S04-02** *(adversarial)* — the evaluator script treats `failure`/`cancelled` as
  red and `skipped`/`success` as green, table-tested including a mixed set. Test: same;
  Verify: `bash hack/lint/ci_gate_test.sh --self-test`; Level: L0
- **REQ-CIH-S04-03** — operator step recorded in a `D-nnn` row once done. Test:
  `docs/decisions/decisions.md`; Verify: `rg 'ci-gate' docs/decisions/decisions.md`; Level: doc

**Counterpoint.** Skipped-as-pass is the point (path filters) but it is also how a Sonar
step silently skipped on Dependabot PRs (D-178) reads green. `ci-gate` aggregates **jobs**,
not steps, so step-level skips stay governed by D-177/D-178 pins; stated so nobody assumes
`ci-gate` re-proves them.

**Not in scope:** moving existing steps into separate jobs for path filtering.

---

## CIH-S05 — `dependency-review-action` on PRs; push-to-main trigger fence `[autonomous]`

**Depends on:** none.

- Given a PR that adds/bumps a dependency, when `verify` (or a dedicated `pull_request`-only
  job) runs, then `actions/dependency-review-action` (SHA-pinned) fails on new
  high-severity vulnerabilities or disallowed licences (Apache-2.0-compatible allowlist).
- Given a Dependabot PR, when it runs, then it works without secrets.
- **Forbidden outcome** — `verify.yaml` losing `push: branches: [main]` or the weekly
  schedule (REQ-CIH-S05-03).

Requirements:

- **REQ-CIH-S05-01** — dependency-review job on `pull_request` only, pinned, no
  `continue-on-error`. Test: `hack/lint/workflow_pins_test.sh`; Verify:
  `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-CIH-S05-02** *(adversarial)* — the guard reds when `fail-on-severity` is lowered
  to `critical`-only without a justification line, or the step is dropped. Test: same;
  Verify: `bash hack/lint/workflow_pins_test.sh --self-test`; Level: L0
- **REQ-CIH-S05-03** *(forbidden outcome)* — `on:` of `verify.yaml` keeps `push.branches: [main]`
  and `schedule`, with a comment citing `verify-tag-gate.sh`. Test:
  `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh --self-test`; Level: L0

**Not in scope:** SBOM/licence policy beyond the allowlist; Dependabot config (see DEP).

---

## Exit

All S01–S05 REQs green in `task check` (the new `hack/lint/*_test.sh` scripts wired into the
`check` stage list **and** into `hack/audit/exitgate_test.sh` `CHECK_STAGES`, which pins the
stage count — add deliberately). S04's operator step is the only non-autonomous item.
