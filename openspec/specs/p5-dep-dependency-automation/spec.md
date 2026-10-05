# P5-DEP — Dependency PR automation (bot PRs merge themselves, narrowly)

**Epic ID / REQ prefix:** `DEP` / `REQ-DEP-Snn-nn`. Cross-cutting hygiene epic.

**Problem.** Dependabot (`.github/dependabot.yml`: gomod, github-actions, pip, npm; weekly;
no `cooldown`) opens PRs that wait for a human. Auto-merge would clear the mechanical ones,
but the dependency PR is the supply-chain path an attacker wants, so scope is tight. Repo
facts that shape it:
1. Merges are **rebase-merge only** (workspace policy: linear history, never squash, never
   merge commit). attune auto-merges with **squash**; assent must not copy that.
2. A merge done with the default `GITHUB_TOKEN` does not trigger other workflows, so a
   bot-merged commit would get no push-to-main `verify` (needed by
   `hack/release/verify-tag-gate.sh`) — hence a GitHub App token.
3. D-177/D-178: the SonarCloud step is **skipped** on `dependabot[bot]` runs because
   Dependabot-triggered `pull_request` runs get **no repo Actions secrets**. The same
   mechanism governs this epic's secrets (S03).

**Coupling.** This epic keys on `github.actor == 'dependabot[bot]'` and
`dependabot/fetch-metadata`; it is **not** bot-agnostic. If TCC-S05 picks Renovate (A or B),
S02's classifier is reworked for Renovate's identity/labels in that change.

**Not in scope:** the choice of bot (TCC-S05); major bumps (always human); Go `go`/`toolchain`
directive bumps; third-party contributor PRs; branch-protection changes other than the
`[operator]` items below.

---

## DEP-S01 — `cooldown` in Dependabot config `[autonomous]`

**Depends on:** none.

- Given `.github/dependabot.yml`, when read, then each ecosystem has `cooldown:
  default-days: 7`. Dependabot holds a version-update PR until the release is >= 7 days old
  (security updates bypass cooldown by design). **This is the only age control** — S02 adds
  no PR-age check (it would double the delay to 14 days, and `fetch-metadata` exposes no
  release date).
- The `codeql-action` group is preserved.

Requirements:

- **REQ-DEP-S01-01** — every `updates:` entry has cooldown >= 7. Test:
  `hack/lint/dependabot_cfg_test.sh` (create); Verify:
  `bash hack/lint/dependabot_cfg_test.sh`; Level: L0
- **REQ-DEP-S01-02** *(adversarial)* — a new ecosystem without cooldown, or cooldown 0, is
  red. Test: same; Verify: `bash hack/lint/dependabot_cfg_test.sh --self-test`; Level: L0

---

## DEP-S02 — Auto-merge workflow `gh pr merge --auto --rebase`, scoped `[autonomous · needs S03]`

**Depends on:** DEP-S01, DEP-S03 (App token + secrets), DEP-S04 (required-check contract).

- Given a PR with `github.actor == 'dependabot[bot]'` **and** `fetch-metadata` reporting
  `version-update:semver-patch` or `semver-minor`, or an `github-actions` digest/patch/minor
  update, when evaluated, then the workflow runs `gh pr merge --auto --rebase <pr>` with the
  App token. GitHub merges only after the required checks pass.
- Given `semver-major`, a Go `go`/`toolchain` directive change, a group containing any
  major, or an unclassified update type, then auto-merge is **not** enabled and the PR is
  labelled `needs-human`.
- Given a merged bot PR, then a push-to-main `verify` run triggers (App token).
- **Adversarial** — a fork PR, or a human-authored PR on a `dependabot/…` branch name, gets no
  auto-merge (actor and metadata must both hold). The workflow, if on `pull_request_target`,
  checks out nothing from the PR head (zizmor-clean, CIH-S02).
- **Forbidden outcome 1** — auto-merge of a major or of any update not positively classified.
- **Forbidden outcome 2** — squash or merge-commit method anywhere in the workflow.
- **Forbidden outcome 3 (Sonar)** — see S04: green on a Dependabot PR excludes Sonar
  (D-178); Sonar for the merged commit comes from the push-to-main run.

Requirements:

- **REQ-DEP-S02-01** — classifier script table-tested with fixtures (patch, minor, major,
  digest, unknown, grouped-with-major, fork, renamed branch). Test:
  `hack/ci/dependabot_automerge_test.sh` (create); Verify:
  `bash hack/ci/dependabot_automerge_test.sh`; Level: L0
- **REQ-DEP-S02-02** *(adversarial)* — major, unknown, fork and renamed-branch fixtures are
  refused. Test: same; Verify: `bash hack/ci/dependabot_automerge_test.sh --self-test`; Level: L0
- **REQ-DEP-S02-03** *(forbidden outcome)* — workflow contains `--rebase` and neither
  `--squash` nor `--merge`, and no PR-head checkout under `pull_request_target`. Test:
  `hack/lint/workflow_pins_test.sh` (new inline controls); Verify:
  `bash hack/lint/workflow_pins_test.sh`; Level: L0

**Counterpoint.** Auto-merging patch/minor is a trust decision about every upstream
maintainer; cooldown, required checks, dependency-review (CIH-S05) and major-exclusion bound
it. A malicious patch that passes tests still merges: residual risk stated. Squash would
tidy bot PRs, but assent's rebase-merge policy wins.

**Not in scope:** auto-approving; auto-merging release PRs.

---

## DEP-S03 — GitHub App token and secrets `[operator]`

**Depends on:** none (blocks S02).

- The operator creates a GitHub App (Contents: write, Pull requests: write, Metadata: read;
  installed on this repo only) and stores `APP_ID` and the private key.
- **Secret scope (review finding 4):** a workflow triggered by `dependabot[bot]` sees only
  **Dependabot secrets**, not repo Actions secrets (same mechanism as D-178). So either
  (a) store them as **Dependabot secrets** and run on `pull_request`, or (b) run the merge
  step from a `pull_request_target`/`workflow_run` workflow that checks out nothing from the
  PR head and uses Actions secrets. Decision recorded in the `D-nnn` row; default (a).
- **Silent-failure fence:** if the secret is empty on a run whose actor is
  `dependabot[bot]`, the workflow emits a visible `::warning::`/job summary
  ("auto-merge disabled: App secret missing") — it must **not** silently pass, because that
  would hide a feature that never runs. On a non-Dependabot actor an empty secret is a
  quiet no-op.
- Repo settings `[operator]`: **Allow auto-merge** on; **Allow rebase merging** the only merge
  method.
- **Adversarial** — the key is referenced only by the token step and never by a step that
  can run PR-head code.

Requirements:

- **REQ-DEP-S03-01** — `D-nnn` row records App ID (not key), secret scope choice, settings.
  Test: `docs/decisions/decisions.md`; Verify: `rg 'DEP-S03' docs/decisions/decisions.md`; Level: doc
- **REQ-DEP-S03-02** *(adversarial)* — empty-secret + Dependabot actor produces the warning
  and no merge call; key referenced in the token step only. Test:
  `hack/ci/dependabot_automerge_test.sh`; Verify:
  `bash hack/ci/dependabot_automerge_test.sh --self-test`; Level: L0

---

## DEP-S04 — Required-check contract and the Sonar coupling `[operator precondition]`

**Depends on:** CIH-S04 (only if a `ci-gate` check is added).

The set of **required status checks is live branch-protection configuration, unreachable from
in-tree files** (D-177; backlog item **AUD-SONAR-REQUIRED**). So this story makes no offline
guard claim.

- Precondition `[operator]`, linked to AUD-SONAR-REQUIRED: before S02 is enabled, the operator
  confirms the required checks include `verify` (and CodeQL). **If Sonar is ever made
  required** (AUD-SONAR-REQUIRED), bot auto-merge must be disabled until a token-bearing
  analysis exists for bot PRs, because a skipped Sonar step on a Dependabot PR would otherwise
  be read as green (D-178).
- Detection: a **scheduled** job (e.g. in `nightly.yaml`) probes the branch-protection API
  (`gh api repos/{repo}/branches/main/protection`, needs an admin-scope token → `[operator]`
  to provision) and reds if a Sonar check name is required while the auto-merge workflow
  exists. Until provisioned, the coupling is a documented operator precondition only.
- A contributor doc states the contract and the D-178 caveat.

Requirements:

- **REQ-DEP-S04-01** — doc states the contract and D-178 caveat. Test:
  `docs/contributing/dependency-automation.md` (create); Verify:
  `rg 'D-178' docs/contributing/dependency-automation.md`; Level: doc
- **REQ-DEP-S04-02** *(forbidden outcome · live probe)* — the probe script reds on a
  fixture protection payload that requires Sonar while auto-merge is enabled. Test:
  `hack/ci/protection_probe_test.sh` (create; fixture-driven, offline); Verify:
  `bash hack/ci/protection_probe_test.sh`; Level: L0 (the live API call itself is
  `[operator]`-provisioned and not claimed as an offline guard)

---

## Exit

One Dependabot patch PR auto-merged by rebase after green checks with a push `verify` on the
merged commit; one major PR left untouched with `needs-human`; fixtures prove both.
New scripts hook into an existing `check:` stage (no `CHECK_STAGES` change) unless one
becomes a new stage.
