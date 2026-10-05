# P5-NIT — Nightly integration truth (real e2e on a schedule + failure issue lifecycle)

**Epic ID / REQ prefix:** `NIT` / `REQ-NIT-Snn-nn`. Cross-cutting hygiene epic.

**Problem.** Verified 2026-10-05: `test/e2e` (build tag `e2e`) is only compiled by
`go vet -tags e2e ./...` in `verify.yaml`; `TestSkeletonE2E` (`test/e2e/skeleton_test.go`)
calls `t.Skipf` unless `ASSENT_E2E_GITLAB` is set and its body is "wired but not yet
asserted" (S10 sketch). So the project's strongest claim — L3, real GitLab CE
testcontainer — is never executed anywhere automatically. Separately, scheduled/main-only
jobs fail unseen: **main is red at `1c4e71e`** (2026-10-04) in step "AUD audit exit gate
(AUD-S18)" of `release-exitgate` (`if: github.event_name != 'pull_request'`), and nobody is
notified except by looking. This epic does **not** fix that red; it is the motivation for a
failure-notification mechanism. Prior art: attune `scripts/nightly-failure-issue-body.sh`
and its nightly workflow.

**Not in scope:** fixing the AUD-S18 red; the GitHub-forge e2e (E10); making nightly e2e a
PR gate; Slack/email notification; auto-fixing failures.

**Lanes:** **A** `test/e2e` scenarios + workflow · **B** `hack/ci/*` issue lifecycle.

---

## NIT-S01 — Minimal real e2e scenarios that mean something `[autonomous]`

**Depends on:** `hack/e2e/` testcontainer profile (Spike B; exists), `e2e-seed`. Infra-gated:
needs a runner able to boot GitLab CE (S02 decides the runner).

Minimum scenario set (each a real assertion against live GitLab, no skip when the endpoint
is set):

1. **APPROVE path** — seeded safe-change MR → assent run → approval recorded and merge
   performed SHA-pinned (ADR-0015 §2).
2. **REVIEW path** — a change outside any vouch → no approval, thread/comment present.
3. **BLOCK path** — a `.assent/**` MR → blocked, no write beyond the comment (meta-class).
4. **SHA drift** — push a new commit between decide and merge → merge refused (fail-closed).
5. **Determinism replay** — the emitted DecisionRecord replays identically.

- Given `ASSENT_E2E_GITLAB` set, when `go test -tags e2e ./test/e2e/...` runs, then all five
  execute and none `t.Skip`s.
- Given the env var **unset** locally, when `task check` runs, then the suite still skips
  (autonomous gate stays green) — but nightly (S02) sets it and **fails if zero tests ran**.
- **Adversarial** — given scenario 4 replaced by a no-op, when the mutation-control test
  runs, then it reds (a drift test that cannot fail is not evidence).

Requirements:

- **REQ-NIT-S01-01** — scenarios 1–3 execute against the testcontainer. Test:
  `test/e2e/scenarios_test.go` (create); Verify:
  `ASSENT_E2E_GITLAB=… go test -count=1 -tags e2e -run 'TestE2E' -v ./test/e2e/...` (output must show `=== RUN` for each); Level: L3
- **REQ-NIT-S01-02** *(adversarial)* — scenario 4: SHA drift refuses the merge. Test: same
  file; Verify: `… -run TestE2E_SHADrift -v`; Level: L3
- **REQ-NIT-S01-03** — scenario 5 replay. Test: same; Verify: `… -run TestE2E_Replay -v`; Level: L3
- **REQ-NIT-S01-04** *(no-silent-skip)* — a wrapper script asserts the nightly run executed
  ≥5 tests and 0 skipped. Test: `hack/ci/e2e_ran_test.sh` (create); Verify:
  `bash hack/ci/e2e_ran_test.sh --self-test`; Level: L0

**Counterpoint.** GitLab CE boot is slow (minutes) and flaky: hence nightly, not PR. A
nightly that is itself flaky erodes trust; S03 issue dedup keeps noise to one open issue.

---

## NIT-S02 — Scheduled e2e job `[autonomous · operator if a larger runner is needed]`

**Depends on:** NIT-S01.

- Given `.github/workflows/nightly.yaml` (cron daily, plus `workflow_dispatch`), when it
  runs, then it boots the testcontainer, runs `task e2e` and uploads logs on failure.
- Given the job, when linted, then it carries `timeout-minutes`, `permissions: contents: read`
  (+ `issues: write` only on the reporting job from S03), SHA-pinned actions, and passes
  the existing pin tests and (when CIH lands) zizmor.
- **Adversarial** — given `continue-on-error` or `|| true` on the e2e step, when the guard
  runs, then red.

Requirements:

- **REQ-NIT-S02-01** — workflow exists, scheduled, runs `task e2e` with the env var set.
  Test: `hack/lint/nightly_wiring_test.sh` (create); Verify:
  `bash hack/lint/nightly_wiring_test.sh`; Level: L0
- **REQ-NIT-S02-02** *(adversarial)* — weakening (`continue-on-error`, `|| true`, step
  removed) reds the guard. Test: same; Verify: `… --self-test`; Level: L0

**Not in scope:** self-hosted runners; matrix over GitLab versions (follow-up).

---

## NIT-S03 — Deduplicated failure issue with recovery transitions `[autonomous]`

**Depends on:** NIT-S02. Also covers scheduled `release-exitgate` (`verify.yaml` schedule)
failures — a `report` job `needs:` both and runs `if: always() && github.event_name == 'schedule'`.

- Given success → failure, when the report job runs, then it opens **one** issue (label
  `ci-nightly-failure`, title keyed on workflow name) with run URL, failed job/step, SHA.
- Given failure → failure, when it runs, then it **comments** on the same open issue (no
  second issue).
- Given failure → success, when it runs, then it comments "recovered" and closes the issue.
- Given success → success, when it runs, then no API call is made.
- The issue-body builder is a script under test (prior art above); no logic inline in YAML.
- **Adversarial** — given a fork or a PR event, when the report job evaluates, then it does
  not run (no `issues: write` from untrusted contexts); given untrusted strings (step
  names, branch names) in the body, then they are escaped (no markdown/mention injection:
  `@team` and backticks neutralised).

Requirements:

- **REQ-NIT-S03-01** — body builder produces the documented sections for a fixture of
  failed jobs. Test: `hack/ci/nightly_issue_body_test.sh` (create); Verify:
  `bash hack/ci/nightly_issue_body_test.sh`; Level: L0
- **REQ-NIT-S03-02** — transition table (none→open, open→comment, fail→recover→close,
  ok→ok noop) tested with a stubbed `gh`. Test: same; Verify: `… --transitions`; Level: L0
- **REQ-NIT-S03-03** *(adversarial)* — mention/markdown injection in job names neutralised.
  Test: same; Verify: `… --self-test`; Level: L0
- **REQ-NIT-S03-04** *(adversarial)* — reporting job gated to `schedule`/`workflow_dispatch`
  on the default branch only. Test: `hack/lint/nightly_wiring_test.sh`; Verify:
  `bash hack/lint/nightly_wiring_test.sh --self-test`; Level: L0

**Counterpoint.** Auto-issues can become a graveyard; dedup + auto-close on recovery bounds it.

---

## Exit

Nightly e2e green on a live GitLab at least once with ≥5 executed tests; a deliberately
broken scheduled run opens exactly one issue and a fix closes it; scripts wired into
`CHECK_STAGES` in `hack/audit/exitgate_test.sh` deliberately.
