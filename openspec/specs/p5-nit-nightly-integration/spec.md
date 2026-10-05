# P5-NIT — Nightly integration truth (CI-failure reporter + scheduled e2e)

**Epic ID / REQ prefix:** `NIT` / `REQ-NIT-Snn-nn`. Cross-cutting hygiene epic. Gate wiring: see
the [gate-wiring rule](../README.md) (D1).

**Problem.** Verified 2026-10-05: `test/e2e` (build tag `e2e`) is only compiled by
`go vet -tags e2e ./...`; `TestSkeletonE2E` skips unless `ASSENT_E2E_GITLAB` is set; `task e2e`
(`Taskfile.yml:269-280`) is not part of `task check`, runs without `-v`, and nothing schedules
it. Failures of scheduled/push-only jobs go unseen: **main is red at `1c4e71e`** in the
`release-exitgate` job (restoration is CIH-S00, not here). Prior art: attune
`scripts/nightly-failure-issue-body.sh`.

**What this epic does not specify.** The live e2e *scenarios*: **E7-S07**
(`p5-e7-e2e-conformance/spec.md`, L3 live conformance: stale-SHA guard, self-vouch BLOCK, rerun
idempotence; absorbs E4-S11) and **P4-E1-S10** (L3 skeleton green + replayable). Boot path:
`hack/spikes/e2e/boot-testcontainer.sh`.

**Not in scope:** the scenarios themselves; fixing the red main (CIH-S00); GitHub-forge e2e
(E10); making e2e a PR gate; chat/email notification.

**Landing order inside the epic:** **S02 first** (reporter, immediately useful, independent),
then S01 (blocked on E7-S07/P4-E1-S10 and operator infra).

---

## NIT-S02 — CI-failure reporter (deduplicated issue lifecycle) `[autonomous]`

**Depends on:** CIH-S02 (zizmor gate, because this adds a `workflow_run` workflow subject to
`dangerous-triggers`).

A `job` cannot `needs:` a job in another workflow, so a separate
`.github/workflows/ci-failure-issue.yaml` is triggered by `workflow_run` (`types: [completed]`)
for `verify`, `vulncheck`, and the scheduled workflows (nightly e2e; plus `codeql`/`release` push
failures if cheap). Rules (D6):

- **Guards:** default branch only; `workflow_run.head_repository.full_name == github.repository`;
  `event` in (`schedule`, `push`); `workflow_dispatch` runs excluded; no PR/fork event ever writes.
- **No checkout** of the triggering run; reads conclusion/jobs via `gh api`; permissions
  `issues: write`, `actions: read` only. Body builder **never evals log text**.
- `concurrency: {group: ci-failure-issue-<reported workflow>, cancel-in-progress: false}`.
- **Find the open issue by label AND bot author** (`ci-failure` + `ci-failure/<workflow>`,
  author `github-actions[bot]`) — never by title alone (a user can spoof a title).
- **Order events by `(run_number, run_attempt)`**; the issue body records the pair it reflects.
  A successful re-run (same `run_number`, higher `run_attempt`) closes the issue; a stale event
  makes no write.
- Outcomes: failure + no open issue -> open **one** issue; failure + open -> comment;
  success + open -> comment "recovered" and close; success + none -> no write call.
  `cancelled` whose jobs never started = no-op; other `cancelled`/`timed_out` = failure.
- One issue per reported workflow. Noisy signals (mutation, benchmarks) live in their own
  workflows so they cannot mask regressions.
- **Untrusted strings** (job/step names, branch, commit subject) neutralised: `@mentions`,
  backticks, markdown links.

Requirements:

- **REQ-NIT-S02-01** — body builder yields the documented sections for a fixture. Test:
  `hack/ci/failure_issue_body_test.sh` (create); Verify:
  `bash hack/ci/failure_issue_body_test.sh`; Level: L0
- **REQ-NIT-S02-02** — transition table incl. re-run recovery (same run_number, higher attempt),
  stale event, cancelled-never-started, dispatch exclusion, and a **race** fixture (two
  simultaneous failures -> one issue). Test: same; Verify:
  `bash hack/ci/failure_issue_body_test.sh --transitions`; Level: L0
- **REQ-NIT-S02-03** *(adversarial)* — injection fixtures neutralised; a title-spoofed issue by a
  non-bot author is ignored. Test: same; Verify:
  `bash hack/ci/failure_issue_body_test.sh --self-test`; Level: L0
- **REQ-NIT-S02-04** *(adversarial)* — workflow guards (default branch, same-repo head,
  event filter, concurrency group, no checkout, minimal permissions) asserted. Test:
  `hack/lint/nightly_wiring_test.sh` (create); Verify:
  `bash hack/lint/nightly_wiring_test.sh --self-test`; Level: L0
- **REQ-NIT-S02-05** — a deliberately failed scheduled run opens exactly one issue and the fix
  closes it. Test: live run; Verify: `gh issue list -R PlatformRelay/assent -l ci-failure`;
  Level: L3 (**post-merge**, D14; named follow-up evidence PR)

**Counterpoint.** `workflow_run` is a known privilege surface: hence guards, no-checkout, and the
zizmor gate. Auto-issues pile up: dedup + auto-close bound it.

---

## NIT-S01 — Scheduled real e2e with a no-silent-skip guard `[blocked · infra-gated]`

**Depends on:** E7-S07 and/or P4-E1-S10 landed; NIT-S02 (so a red nightly is reported); a runner
that can boot the GitLab CE container.

Explicit prerequisites (the earlier draft assumed them):
1. `task e2e` gains `-v` for the nightly (or the workflow calls `go test -v` directly) so
   `=== RUN`/`--- PASS`/`--- SKIP` are visible.
2. The workflow **builds `bin/assent`** (`task build`), since the e2e drives a prebuilt binary.
3. A **GitLab API token on a fresh CE container** (E7-S07 `E7:278-279` needs one): the
   implementer checks whether E7-S07 already bootstraps it; if not, this story adds
   `hack/ci/e2e-gitlab-token.sh` (root PAT via the container's console/API) and wires it.

- Given `.github/workflows/nightly-e2e.yaml` (cron daily + `workflow_dispatch`, own workflow so
  its failures get their own issue), when it runs, then it boots the container, builds the
  binary, provisions the token, runs the e2e verbosely and uploads logs on failure.
- Given the `-v` output, when the guard parses it, then it requires `=== RUN` for the E7-S07 /
  P4-E1-S10 named tests, >= 1 `--- PASS`, and **zero `--- SKIP`**; otherwise red.
- **Adversarial** — `continue-on-error`, `|| true` or the step removed is red; permissions
  `contents: read`.

Requirements:

- **REQ-NIT-S01-01** — workflow wiring (scheduled, build, token, verbose run, pinned actions,
  `timeout-minutes`). Test: `hack/lint/nightly_wiring_test.sh`; Verify:
  `bash hack/lint/nightly_wiring_test.sh`; Level: L0
- **REQ-NIT-S01-02** *(adversarial)* — the guard rejects outputs with a `--- SKIP` or zero
  `=== RUN`, accepts a clean one. Test: `hack/ci/e2e_ran_test.sh` (create); Verify:
  `bash hack/ci/e2e_ran_test.sh --self-test`; Level: L0
- **REQ-NIT-S01-03** *(adversarial)* — weakening the nightly reds the wiring guard. Test:
  `hack/lint/nightly_wiring_test.sh`; Verify:
  `bash hack/lint/nightly_wiring_test.sh --self-test`; Level: L0
- **REQ-NIT-S01-04** — one real nightly run executes the named tests with zero skips. Test: live
  run; Verify: `gh run view -R PlatformRelay/assent --log | rg -c '^--- PASS'`;
  Level: L3 (**post-merge**, D14)

**Not in scope:** self-hosted runners; a GitLab-version matrix.

---

## Exit

S02 live and S01 (when unblocked) per the [gate-wiring rule](../README.md): every new script and
mode (`--transitions`, `--self-test`) is a pinned `verify` step and a `task check` stage with
`STAGE_BODY_PINS` entries.
