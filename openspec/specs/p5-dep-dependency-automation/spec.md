# P5-DEP — Dependency PR automation (bot PRs merge themselves, narrowly)

**Epic ID / REQ prefix:** `DEP` / `REQ-DEP-Snn-nn`. Cross-cutting hygiene epic.

**Problem.** Dependabot (`.github/dependabot.yml`: gomod, github-actions, pip, npm; weekly;
no `cooldown`) opens PRs that wait for a human; the 2026-08/09 sweeps show the queue is
mostly mechanical. Auto-merge would clear it, but only safely under tight scope, because
the dependency PR is exactly the supply-chain path an attacker wants. Two repo facts
shape it: (1) merges are **rebase-merge only** (workspace policy: linear history, never
squash, never merge commit) — prior art attune auto-merges with **squash**; assent must
not copy that; (2) a merge performed with the default `GITHUB_TOKEN` does **not** trigger
other workflows, so a bot-merged commit would get no push-to-main `verify` (needed by
`hack/release/verify-tag-gate.sh`) — hence a GitHub App token. (3) D-177/D-178: the
SonarCloud step is **skipped** on `dependabot[bot]` runs (no secrets), so a job-level
"green" on a Dependabot PR does **not** include Sonar.

**Not in scope:** the choice of dependency bot itself (`P5-TCC` TCC-S05, open question —
this epic is written to work with Dependabot and survives a switch to Renovate); major
bumps (always human); Go toolchain/`go` directive bumps; third-party contributor PRs;
changing branch protection beyond what is listed `[operator]`.

---

## DEP-S01 — `cooldown` and update grouping in Dependabot config `[autonomous]`

**Depends on:** none.

- Given `.github/dependabot.yml`, when read, then each ecosystem carries
  `cooldown: default-days: 7` (security updates bypass cooldown by design, state this).
- Given the existing `codeql-action` group, then it is preserved.

Requirements:

- **REQ-DEP-S01-01** — every `updates:` entry has cooldown >= 7 days. Test:
  `hack/release/ci_audit_test.sh` (extend) or `hack/lint/dependabot_cfg_test.sh` (create);
  Verify: `bash hack/lint/dependabot_cfg_test.sh`; Level: L0
- **REQ-DEP-S01-02** *(adversarial)* — a new ecosystem without cooldown, or cooldown 0,
  reds (failing-direction self-test). Test: same; Verify: `… --self-test`; Level: L0

---

## DEP-S02 — Auto-merge workflow: `gh pr merge --auto --rebase`, scoped `[autonomous · operator for S03]`

**Depends on:** DEP-S01, DEP-S03 (App token), CIH-S04 (`ci-gate`) recommended.

- Given a PR whose author is `dependabot[bot]` (verified via `github.actor` **and**
  `dependabot/fetch-metadata`), when metadata says `update-type` is `version-update:semver-patch`
  or `semver-minor`, or the ecosystem is `github-actions` with a digest/patch/minor update,
  then the workflow enables `gh pr merge --auto --rebase <pr>` (never `--squash`,
  never `--merge`) using the App token. GitHub then merges only after required checks pass.
- Given a `semver-major` update, a Go `toolchain`/`go` directive change, a grouped PR
  containing any major, or an unknown update type, when evaluated, then auto-merge is
  **not** enabled and the PR gets a `needs-human` label.
- Given the PR is younger than the cooldown (7 days since release), when evaluated, then
  not enabled (defence in depth beside S01's `cooldown`).
- Given a merged bot PR, then a push-to-main `verify` run is triggered (App token) and the
  commit is a rebased original commit (linear).
- **Adversarial** — given a PR from a fork, or authored by someone else who merely
  renamed the branch `dependabot/…`, when evaluated, then no auto-merge (actor check +
  metadata check); the workflow uses `pull_request_target` only if it checks out nothing
  from the PR head (zizmor-clean; see CIH-S02).
- **Forbidden outcome 1** — auto-merge of a major bump or of any PR whose update type is
  not positively classified patch/minor/digest.
- **Forbidden outcome 2** — Sonar skipped treated as green: auto-merge is enabled **only if**
  the required checks (S04) include a check that does not depend on Sonar having run, and
  the spec explicitly states that a Dependabot PR's green verify excludes Sonar
  (D-178); Sonar coverage for the merged commit comes from the push-to-main run. If Sonar
  ever becomes a **required** check, auto-merge must be disabled for bot PRs until a
  token-bearing analysis exists — the guard test asserts this coupling.
- **Forbidden outcome 3** — squash or merge-commit method anywhere in the workflow.

Requirements:

- **REQ-DEP-S02-01** — workflow exists; classification table-tested with fixtures
  (patch, minor, major, digest, unknown, grouped-with-major, fork, renamed branch). Test:
  `hack/ci/dependabot_automerge_test.sh` (create; classifier is a script, not inline YAML);
  Verify: `bash hack/ci/dependabot_automerge_test.sh`; Level: L0
- **REQ-DEP-S02-02** *(adversarial)* — major, unknown and fork/renamed-branch fixtures are
  refused. Test: same; Verify: `… --self-test`; Level: L0
- **REQ-DEP-S02-03** *(forbidden outcome)* — the workflow contains `--rebase` and neither
  `--squash` nor `--merge`; no `pull_request_target` head checkout. Test:
  `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh --self-test`; Level: L0
- **REQ-DEP-S02-04** *(forbidden outcome · Sonar)* — guard reds if Sonar becomes a required
  check name while the bot auto-merge workflow is enabled. Test:
  `hack/lint/dependabot_sonar_coupling_test.sh` (create); Verify:
  `bash hack/lint/dependabot_sonar_coupling_test.sh --self-test`; Level: L0

**Counterpoint.** Auto-merging patch/minor is a trust decision about every upstream
maintainer; the 7-day cooldown, required checks, `dependency-review` (CIH-S05), and
major-excluded scope bound it. A malicious patch that passes tests + review action still
merges: the residual risk is stated, not hidden. Squash would hide a bot's multi-commit
PR noise, but assent's rebase-merge policy and per-commit conventions win (attune differs
because it is squash-only).

**Not in scope:** auto-merging release PRs; auto-approving (merge is via required checks
only; no bot approval — solo-maintainer review already dismissed in SEC-SC).

---

## DEP-S03 — GitHub App token for merges `[operator]`

**Depends on:** none (blocks S02).

- Given the operator creates a GitHub App (Contents: write, Pull requests: write, Metadata:
  read; installed on this repo only) and stores `APP_ID` + private key as repo/Actions
  secrets, when S02's workflow runs, then it mints a short-lived token with a pinned
  `actions/create-github-app-token` and uses it for `gh pr merge`.
- Given the repo settings, when the operator enables them, then **Allow auto-merge** is on
  and **Allow rebase merging** is the only enabled merge method (workspace policy).
- Given the secrets are absent, when the workflow runs, then it exits green without
  merging and without failing the PR (feature-off is safe, not red).
- **Adversarial** — the App private key is never echoed or passed to a PR-head-controlled
  step; the token step is the only consumer; token scope is least-privilege.
- Record the App ID (not the key) and the settings choices in a `D-nnn` row.

Requirements:

- **REQ-DEP-S03-01** — `D-nnn` row records the App and settings. Test:
  `docs/decisions/decisions.md`; Verify: `rg 'DEP-S03' docs/decisions/decisions.md`; Level: doc
- **REQ-DEP-S03-02** *(adversarial)* — the workflow references the key only in the token
  step and no-ops when the secret is empty. Test: `hack/ci/dependabot_automerge_test.sh`;
  Verify: `… --self-test`; Level: L0

---

## DEP-S04 — Required-check contract for bot merges `[autonomous · operator]`

**Depends on:** CIH-S04.

- Given `ci-gate` is a required check (operator setting, CIH-S04), when a bot PR has it
  green, then GitHub's `--auto` merges it; if any required check is pending or red, it waits.
- Given a skipped Sonar step on a bot PR, then docs state it is covered post-merge on `main`
  (D-177/D-178) and the S02 guard applies.

Requirements:

- **REQ-DEP-S04-01** — `docs/` contributor page states the contract and the D-178 caveat.
  Test: `docs/contributing/dependency-automation.md` (create); Verify:
  `rg 'D-178' docs/contributing/dependency-automation.md`; Level: doc

---

## Exit

One Dependabot patch PR auto-merged via rebase after green checks with a triggered push
`verify`; one major PR left untouched with `needs-human`; fixtures prove both.
