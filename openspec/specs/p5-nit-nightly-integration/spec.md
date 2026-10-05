# P5-NIT — Nightly integration truth (scheduled e2e + failure issue lifecycle)

**Epic ID / REQ prefix:** `NIT` / `REQ-NIT-Snn-nn`. Cross-cutting hygiene epic.

**Problem.** Verified 2026-10-05: `test/e2e` (build tag `e2e`) is only compiled by
`go vet -tags e2e ./...` in `verify.yaml`, and `TestSkeletonE2E` skips unless
`ASSENT_E2E_GITLAB` is set. `task e2e` (`Taskfile.yml:269-279`) is **never part of
`task check`** and nothing runs it on a schedule, so the L3 claim is never executed
automatically. Separately, scheduled/main-only jobs fail unseen: **main is red at `1c4e71e`**
(2026-10-04) in step "AUD audit exit gate (AUD-S18)" of the `release-exitgate` job. This
epic does **not** fix that red; it is the motivation for failure notification. Prior art:
attune `scripts/nightly-failure-issue-body.sh`.

**What this epic does NOT specify (review finding 2).** The live e2e *scenarios* are already
specified: **E7-S07** (`p5-e7-e2e-conformance/spec.md`, L3 live forge conformance harness:
SHA-guard stale SHA, self-vouch BLOCK, rerun idempotence; absorbs E4-S11) and **P4-E1-S10**
(`p4-e1-walking-skeleton`, L3 skeleton green + replayable, the exit gate). This epic must
not duplicate or redefine them; it only **schedules** them and guards against silent skips.
The boot path is `hack/spikes/e2e/boot-testcontainer.sh` (not `hack/e2e/`).

**Dependency:** NIT-S01 is meaningful only after E7-S07 / P4-E1-S10 give the suite real
assertions (both infra-gated, operator-parked). Until then the scheduled job would run a
skeleton; S01's no-silent-skip guard therefore **fails** a run with zero executed e2e tests,
which keeps the dependency visible instead of green-washing it.

**Not in scope:** the e2e scenarios themselves (E7-S07, P4-E1-S10); fixing the AUD-S18 red;
GitHub-forge e2e (E10); making e2e a PR gate; chat/email notification.

---

## NIT-S01 — Scheduled e2e job with a no-silent-skip guard `[autonomous · infra-gated]`

**Depends on:** E7-S07 and/or P4-E1-S10 landed; a runner that can boot the GitLab CE
testcontainer (operator if a larger runner is needed).

- Given `.github/workflows/nightly.yaml` (cron daily + `workflow_dispatch`), when it runs,
  then it boots `hack/spikes/e2e/boot-testcontainer.sh`, runs `task e2e` with
  `ASSENT_E2E_GITLAB` set, and uploads logs on failure.
- Given the run's `go test -v` output, when the guard script parses it, then it requires
  `=== RUN` lines for the E7-S07/P4-E1-S10 named tests, >= 1 `--- PASS`, and **zero
  `--- SKIP`**; otherwise red.
- Given `ASSENT_E2E_GITLAB` unset on a developer machine, then the suite still skips there
  (the local/PR gate is unaffected — `task check` never runs e2e).
- **Adversarial** — `continue-on-error`, `|| true`, or the e2e step removed is red;
  permissions are `contents: read` (the S02 report job adds `issues: write` separately).

Requirements:

- **REQ-NIT-S01-01** — workflow scheduled, runs `task e2e` with the env var set, pinned
  actions, `timeout-minutes`. Test: `hack/lint/nightly_wiring_test.sh` (create); Verify:
  `bash hack/lint/nightly_wiring_test.sh`; Level: L0
- **REQ-NIT-S01-02** *(adversarial)* — the guard rejects fixture outputs containing a
  `--- SKIP` or zero `=== RUN`, accepts a clean one. Test: `hack/ci/e2e_ran_test.sh`
  (create); Verify: `bash hack/ci/e2e_ran_test.sh --self-test`; Level: L0
- **REQ-NIT-S01-03** *(adversarial)* — weakening (`continue-on-error`, `|| true`, step
  removed) reds the wiring guard. Test: `hack/lint/nightly_wiring_test.sh`; Verify:
  `bash hack/lint/nightly_wiring_test.sh --self-test`; Level: L0

**Counterpoint.** GitLab CE boot is slow and flaky: nightly, not PR. S02's dedup bounds noise.

**Not in scope:** self-hosted runners; a GitLab-version matrix.

---

## NIT-S02 — Deduplicated failure issue with recovery transitions `[autonomous]`

**Depends on:** NIT-S01 (for the nightly) — but it also covers the `release-exitgate` job.

**Trigger mechanism (review finding 3).** `release-exitgate` lives in `verify.yaml`, and a
job cannot `needs:` a job in another workflow. So the reporter is a separate
`.github/workflows/ci-failure-issue.yaml` triggered by **`workflow_run`** for the `verify`
and `nightly` workflows (`types: [completed]`). `workflow_run` runs with a privileged
context, so:
- it runs only for `github.event.workflow_run.head_branch == default branch` and
  `event` in (`schedule`, `workflow_dispatch`, `push` on main), never for PR/fork events;
- it checks out **nothing** from the triggering run and reads only the run's
  conclusion/jobs via `gh api` with `issues: write`, `actions: read` only;
- it is zizmor-reviewed (CIH-S02) with no suppression.

State machine over the previous run's outcome for the same workflow (last completed run on
the default branch before this one):
- success -> failure: open **one** issue (label `ci-nightly-failure`, title keyed on workflow
  name) with run URL, failed job/step, SHA.
- failure -> failure: **comment** on the existing open issue; no second issue.
- failure -> success: comment "recovered" and close.
- success -> success: **no write call** (reads are allowed).
- The body builder is a script under test (prior art above); no logic inline in YAML.
- **Adversarial** — untrusted strings (step names, branch names, commit subjects) are
  escaped: `@mentions`, backticks, and markdown links neutralised.
- **Adversarial** — a PR/fork `workflow_run` event produces no write call.

Requirements:

- **REQ-NIT-S02-01** — body builder yields the documented sections for a fixture. Test:
  `hack/ci/failure_issue_body_test.sh` (create); Verify:
  `bash hack/ci/failure_issue_body_test.sh`; Level: L0
- **REQ-NIT-S02-02** — transition table (the four above) with a stubbed `gh`, asserting
  which calls occur. Test: same; Verify:
  `bash hack/ci/failure_issue_body_test.sh --transitions`; Level: L0
- **REQ-NIT-S02-03** *(adversarial)* — injection fixtures neutralised. Test: same; Verify:
  `bash hack/ci/failure_issue_body_test.sh --self-test`; Level: L0
- **REQ-NIT-S02-04** *(adversarial)* — the workflow is `workflow_run`-gated to the default
  branch and non-PR events, checks out nothing, has minimal permissions. Test:
  `hack/lint/nightly_wiring_test.sh`; Verify:
  `bash hack/lint/nightly_wiring_test.sh --self-test`; Level: L0

**Counterpoint.** Auto-issues can pile up; dedup + auto-close bounds it. `workflow_run` is a
known privilege-escalation surface, hence the no-checkout rule and the explicit guard.

---

## Exit

Nightly (after its dependencies) runs the real suite with zero skips; a deliberately broken
scheduled `release-exitgate` or nightly run opens exactly one issue and a fix closes it. The
new `hack/lint`/`hack/ci` scripts hook into an existing `check:` stage (no `CHECK_STAGES`
change) unless one becomes a new stage, which then needs a deliberate pin.
