# P5-DEP — Dependency PR automation (bot PRs merge themselves, narrowly)

**Epic ID / REQ prefix:** `DEP` / `REQ-DEP-Snn-nn`. Cross-cutting hygiene epic. Gate wiring: see
the [gate-wiring rule](../README.md) (D1).

**Problem.** Dependabot (`.github/dependabot.yml`: gomod, github-actions, pip, npm; weekly; no
`cooldown`; auto-merge scope excludes `github-actions`, see DEP-S02) opens PRs that wait for a human. Auto-merge would clear the mechanical ones, but the
dependency PR is the supply-chain path an attacker wants. Repo facts:
1. Merges are **rebase-merge only** (linear history, never squash/merge commit). attune
   auto-merges with **squash**; assent must not copy that.
2. A merge by the default `GITHUB_TOKEN` does not trigger other workflows; the merged commit
   needs a push `verify` (`hack/release/verify-tag-gate.sh`), so merges use a GitHub App token.
3. **Required checks do not cover everything.** `docs.yaml`, `schemas.yml`, `actionlint.yaml`
   and the `release.yaml` snapshot are path-filtered and `release-exitgate` is push-only
   (`verify.yaml:213-214`). A pip minor that breaks `mkdocs --strict` would pass `verify`,
   auto-merge, redden main and block releases through `verify-tag-gate.sh`. Auto-merge is
   therefore only as safe as what is *checked*, not what is *required*.
4. D-177/D-178: Sonar is skipped on `dependabot[bot]` runs (no repo secrets), so green on a
   Dependabot PR excludes Sonar; Sonar for the merged commit comes from the push run.

**Coupling.** Keys on Dependabot identity; if TCC-S05 picks Renovate (A/B) the classifier is
reworked in that change. Not bot-agnostic.

**Not in scope:** the bot choice (TCC-S05); major bumps; Go `go`-directive bumps (TCC-S06);
third-party PRs; any `pull_request_target` variant (none, at all).

**Self-dogfooding note (honest).** assent is itself a merge gate, so why not use its policy for
its own bot PRs? Not yet: (a) its change adapters cover JSON/YAML/HCL, and a `go.mod`/`go.sum`/
workflow diff is **opaque -> REVIEW** by design (ADR-0003); (b) assent loading policy from the
ref it protects and deciding its own regression is a bootstrap-trust loop that ADR-0015's
target-ref rule is meant to avoid. Candidate follow-up: a lockfile adapter plus a
"bot patch bump with green facts" policy, run by a *released* assent, not the one under change.

---

## DEP-S01 — `cooldown` in Dependabot config `[autonomous]`

**Depends on:** none.

- Each `updates:` entry has `cooldown: default-days: 7`. Dependabot holds a version-update PR
  until the release is >= 7 days old (security updates bypass it by design). This is the **only**
  age control: no PR-age check elsewhere (it would double the delay, and `fetch-metadata`
  exposes no release date).
- The `codeql-action` group is preserved. `github-actions` still gets cooldown but is **not** in
  the auto-merge scope (human-merged, DEP-S02).

Requirements:

- **REQ-DEP-S01-01** — every entry has cooldown >= 7. Test: `hack/lint/dependabot_cfg_test.sh`
  (create); Verify: `bash hack/lint/dependabot_cfg_test.sh`; Level: L0
- **REQ-DEP-S01-02** *(adversarial)* — a new ecosystem without cooldown, or 0, is red. Test:
  same; Verify: `bash hack/lint/dependabot_cfg_test.sh --self-test`; Level: L0

---

## DEP-S02 — Scheduled merger, rebase only, all-checks-green `[blocked-by DEP-S03 + CIH-S03 + CIH-S00]`

**Design.** A **scheduled** (+ `workflow_dispatch`) workflow `dependabot-merge.yaml` — never a
`pull_request`/`pull_request_target`/`workflow_run` trigger, so it runs no PR code and handles no
PR-controlled context. It lists open Dependabot PRs via `gh api` and, per PR, merges only when
**all** hold (D13):
1. **Identity:** `pull_request.user.login == dependabot[bot]` **and** head repo == this repo
   (**and** the triggering actor is the scheduled run itself) — never `github.actor` alone.
2. **Classification:** `fetch-metadata`-style update type is patch/minor in the `gomod`, `pip`
   or `npm` ecosystems (subject to item 3); **never** the **`github-actions` ecosystem** (it edits
   `.github/workflows/*`, which needs App `Workflows: write`; those PRs are human-merged),
   **never** major, `go`-directive, grouped-with-major,
   unknown, or digest/pinDigest updates with no release timestamp (cannot be cooldown-aged —
   excluded, not exempted).
3. **Scope exclusions:** nothing that executes inside a job holding a privileged/bypass token or
   in a release/publish workflow: a PR touching `release.yaml`, the merger workflow, or any
   workflow with `id-token: write`/`contents: write`/`pages: write`, and any ecosystem whose
   packages run in such a job, is `needs-human`. The implementer audits per ecosystem
   (gomod/pip/npm/actions) and records the result in the DEP-S04 doc.
4. **Checks (the BLOCKER fix):** **every check run on the head SHA has finished and is green**
   (including path-filtered ones that ran: docs, schemas, actionlint; nothing pending, nothing
   missing for a path the PR touched), **and the latest push-triggered `verify` on `main` is
   green** (so a PR cannot stack on a red main).
5. Method: `gh pr merge --rebase --match-head-commit <sha-checked-in-item-4>` (the head SHA
   read in item 4, so a Dependabot rebase between check and merge makes the merge fail closed);
   **never** `--auto` (it merges whatever head exists once *required* checks pass, reopening the
   hole of finding 3) and **never** `--squash`/`--merge`. Uses the App token so a push `verify`
   follows.
- Otherwise the PR gets `needs-human` and no merge call.

- **Forbidden outcomes:** auto-merging any `github-actions` ecosystem PR or any PR touching
  `.github/workflows/*` (and granting the App `Workflows: write`); merging a major or
  unclassified update; `--auto`; merging without `--match-head-commit`; any squash/merge-commit method;
  merging with a pending/failed/absent check run on the head SHA; merging while main's latest
  push `verify` is red; treating a Sonar skip as green when Sonar is required (DEP-S04); any
  privileged-trigger variant.

Requirements:

- **REQ-DEP-S02-01** — classifier + gate script table-tested with fixtures: patch, minor, major,
  digest-without-timestamp, unknown, grouped-with-major, `github-actions` ecosystem, workflow-touching, head-SHA-changes-mid-merge, fork, human PR on a `dependabot/…`
  branch, touches release.yaml, check run pending, check run red, main red. Test:
  `hack/ci/dependabot_merge_test.sh` (create); Verify:
  `bash hack/ci/dependabot_merge_test.sh`; Level: L0
- **REQ-DEP-S02-02** *(adversarial)* — every refusal fixture above yields no merge call. Test:
  same; Verify: `bash hack/ci/dependabot_merge_test.sh --self-test`; Level: L0
- **REQ-DEP-S02-05** *(adversarial · race)* — a fixture where the head SHA changes between
  check and merge yields a failed (not performed) merge; the script always passes
  `--match-head-commit`, and a `github-actions` fixture is refused. Test: same script; Verify:
  `bash hack/ci/dependabot_merge_test.sh --self-test`; Level: L0
- **REQ-DEP-S02-03** *(forbidden outcome)* — the workflow has only `schedule`/`workflow_dispatch`
  triggers, uses `--rebase --match-head-commit`, contains no `--auto`/`--squash`/`--merge`/`pull_request_target`/`workflow_run`,
  and checks out nothing. Test: `hack/lint/workflow_pins_test.sh`; Verify:
  `bash hack/lint/workflow_pins_test.sh`; Level: L0
- **REQ-DEP-S02-04** — first live run merges one patch bump by rebase with a push `verify` and
  leaves one major untouched. Test: live run; Verify:
  `gh pr list -R PlatformRelay/assent --label needs-human`; Level: L3 (**post-merge**, D14; named
  follow-up evidence PR)

**Counterpoint.** Auto-merging patch/minor is a trust decision about every upstream maintainer;
cooldown, all-checks-green, dependency-review, and the major/privileged exclusions bound it; a
malicious patch that passes tests still merges — residual risk stated. Polling (hourly) is slower
than event-driven but needs no privileged trigger.

---

## DEP-S03 — GitHub App token, secrets, settings `[operator]`

**Depends on:** none (blocks S02). Operator prerequisites, explicit and in this order:
1. **Required checks first (D1):** every new guard from the other epics is a required check or in
   a required aggregator's `needs`; CIH-S03 (dependency-review) is required.
2. Create a GitHub App (Contents: write, Pull requests: write, Metadata: read; this repo only;
   **no `Workflows: write`** — `github-actions` PRs are human-merged, see DEP-S02);
   store `APP_ID` and the private key as **Actions secrets** (the merger is a scheduled workflow,
   so repo Actions secrets are visible; Dependabot secrets are not needed and there is **no**
   `pull_request_target` option).
3. Settings: **Allow auto-merge** on; **Allow rebase merging** the only merge method.
4. Record the App ID (not the key), the secret scope and settings in a `D-nnn` row.

- Empty secret on a scheduled run: the (non-required) merger job **fails red** with
  "auto-merge disabled: App secret missing" — visible, blocks nothing, never a silent pass.
- **Adversarial** — the key is referenced only by the token step; no step runs PR-head code.

Requirements:

- **REQ-DEP-S03-01** — `D-nnn` row records the above. Test: `docs/decisions/decisions.md`;
  Verify: `rg 'DEP-S03' docs/decisions/decisions.md`; Level: doc
- **REQ-DEP-S03-02** *(adversarial)* — empty secret fails the job with no merge call; key used
  only in the token step. Test: `hack/ci/dependabot_merge_test.sh`; Verify:
  `bash hack/ci/dependabot_merge_test.sh --self-test`; Level: L0

---

## DEP-S04 — Contract doc and Sonar precondition `[autonomous · operator precondition]`

The set of required checks is live branch-protection config, **unreachable from in-tree files**
(D-177; backlog **AUD-SONAR-REQUIRED**), so no in-tree guard is claimed and no live probe is
built (cut).

- A contributor doc `docs/contributing/dependency-automation.md` states the merger contract,
  the per-ecosystem privilege audit result, and the D-178 caveat (green on a bot PR excludes
  Sonar).
- **Operator precondition**, linked to AUD-SONAR-REQUIRED: if Sonar is ever made a required
  check, disable the merger until a token-bearing analysis exists for bot PRs.

Requirements:

- **REQ-DEP-S04-01** — doc states the contract, audit and D-178 caveat. Test:
  `docs/contributing/dependency-automation.md` (create); Verify:
  `rg -c 'D-178|AUD-SONAR-REQUIRED' docs/contributing/dependency-automation.md`; Level: doc

---

## Exit

One Dependabot patch PR merged by rebase with a push `verify` on the merged commit; one major
untouched with `needs-human` (both **post-merge** rows, D14). Wiring per the
[gate-wiring rule](../README.md): every script/mode is a pinned `verify` step and a `task check`
stage with `STAGE_BODY_PINS` entries.
