# Release tooling (`hack/release/`)

Local release helpers for **assent** (P5-E9). Snapshot builds (E9-S02) and changelog sync
(E9-S03) run here without publish credentials.

## Changelog (`git-cliff`)

Release notes and root `CHANGELOG.md` are generated from [git-cliff](https://git-cliff.org/)
using `cliff.toml` at the repo root. Commit subjects use gitmoji-conventional format; the body
template emits **categorized subject lines only — no commit SHAs** (D-101, oss-playbook #4).

| Task | Purpose |
| --- | --- |
| `task changelog` | Preview the **Unreleased** section (stdout) |
| `task changelog-write` | Regenerate `CHANGELOG.md` from tags + unreleased commits |
| `task changelog-verify` | Fail closed if `CHANGELOG.md` drifts from `cliff.toml` output (release gate) |
| `bash hack/release/verify-changelog.sh` | Same check as `task changelog-verify` (script entry point) |

**Not in `task check`:** regenerating `CHANGELOG.md` is a separate commit (`task changelog-write`).
Running verify inside every local `task check` would chicken-egg — any commit after
`changelog-write` fails until the next regeneration. Instead:

- **Local / PR:** run `task changelog-verify` (or `bash hack/release/changelog_test.sh`) when
  touching release docs or before opening a release-prep PR.
- **CI (E9-S05):** PRs that touch release paths run the `snapshot` job in
  `.github/workflows/release.yaml` (goreleaser `--snapshot --skip=publish`). Changelog drift is
  still a separate gate (`task changelog-verify`) — not bundled into `task check`.

### Release workflow (E9-S05)

The tag-triggered `.github/workflows/release.yaml` consumes git-cliff output for GitHub Release
notes — same pattern as mkurator:

1. Checkout with `fetch-depth: 0` (full history + tags).
2. **Verify-green gate (AUD-S03):** `hack/release/verify-tag-gate.sh` — see below.
3. **Build:** `goreleaser release --clean` (`.goreleaser.yaml` keeps
   `release.disable: true` so goreleaser does not create the GitHub Release; Homebrew tap
   push still runs when `HOMEBREW_TAP_GITHUB_TOKEN` is set — do **not** pass `--skip=publish`,
   which skips the brew publisher). Softprops uploads archives below.
4. Run **`orhun/git-cliff-action`** (SHA-pinned) with `config: cliff.toml` and
   `args: --latest --strip header` so the action emits the latest tagged section body.
5. **`softprops/action-gh-release`** uploads `dist/` archives + `checksums.txt` with cliff body.

**Triggers:** `push.tags: v*.*.*`, `workflow_dispatch` (rebuild an existing tag), and PR
`snapshot` dry-run on release-related paths.

**REQ-E9-S05-03 autonomous path:** PR `snapshot` job **or** local `task release-snapshot` /
`hack/release/snapshot_test.sh` — both run goreleaser without publish credentials (sign/sbom
skipped — no fake signatures locally).

### Supply chain on tagged release (E9-S06, D-109)

The publish job in `.github/workflows/release.yaml` (tag push / `workflow_dispatch` only):

1. **`sigstore/cosign-installer`** (SHA-pinned) + **`anchore/sbom-action/download-syft`** for
   goreleaser `signs` / `sboms` in `.goreleaser.yaml`.
2. **`goreleaser release --clean`** — builds archives, SPDX SBOMs (`*.spdx.json`),
   cosign keyless `.sigstore.json` bundles (archives + `checksums.txt`), OIDC via
   `id-token: write`. Also pushes Homebrew Formula when `HOMEBREW_TAP_GITHUB_TOKEN` is set
   (`release.disable: true` still blocks goreleaser’s own GitHub Release).
3. **`actions/attest`** with `subject-checksums: dist/checksums.txt` — SLSA provenance; exported
   as `dist/release-provenance.intoto.jsonl` (mkurator pattern).
5. **`softprops/action-gh-release`** uploads archives, checksums, SBOMs, sigstore bundles, and
   provenance bundle.

**Autonomous gates:** `bash hack/release/supply_chain_test.sh` (config + SECURITY.md wiring).
**Live cosign/SBOM/SLSA proof:** infra-gated — first tagged release after merge (E9-S13).

**Local snapshot skip path:** `task release-snapshot` and PR CI use
`--skip=publish,sign,sbom,homebrew` — checksum verify still works; cosign branch skips when bundles absent
(D-110); Homebrew tap push skips when `HOMEBREW_TAP_GITHUB_TOKEN` is unset (E9-S07b). Do not invent
fake signatures outside GitHub Actions OIDC.

Verification commands: [`SECURITY.md`](../SECURITY.md) (REQ-E9-S06-03).

Maintainers regenerate `CHANGELOG.md` on main via `task changelog-write` after merging user-facing
commits; `verify-changelog.sh` keeps the committed file in sync.

## Verify-green tag gate (AUD-S03, REQ-AUD-S03-01/02)

`v0.1.0` published signed, attested artifacts from a commit whose `release-exitgate` was red.
The `release` job's first step after checkout now refuses to build unless the `verify` workflow
is green on the exact commit the tag points at.

| Script / task | Purpose |
| --- | --- |
| `hack/release/verify-tag-gate.sh` | The gate itself — run by `.github/workflows/release.yaml` before any build/sign/publish step |
| `hack/release/verify_tag_gate_test.sh` | REQ-AUD-S03 gate: polarity table via a stubbed `gh` + release.yaml step-order assertion |
| `task release-verify-tag-gate-test` | Same check via Taskfile (also runs inside `hack/release/exitgate_test.sh`, so CI enforces it) |

**Rule.** The tag is resolved to its commit SHA via `gh api repos/{repo}/commits/{tag}` on both
the `push` and `workflow_dispatch` paths (the rebuild path is not a bypass), then **every**
`verify.yaml` run on that SHA must be `completed` + `success`, **and at least one green run must
not be a `pull_request` run**. Both halves matter: a PR run skips `release-exitgate`
(`if: github.event_name != 'pull_request'`), so its success says nothing about the release exit
gate — whether it stands alongside a red push run or stands alone. GitHub only creates a push
run for the **tip** of a push, so intermediate commits of a multi-commit ff-merged PR carry a
lone green PR run; tag the push tip.

No wait-loop by design: `queued`/`in_progress` fails immediately, because re-dispatching a
release is cheap and waiting hides red.

**When the gate blocks you.** It reports each offending run's URL. A red *or cancelled* run
stays on the SHA until it is re-run — use **Re-run all jobs** on that run in the Actions tab
(re-running updates the existing run's conclusion), then re-dispatch the release. There is no
override env var: the pinned workflow name and the polarity rules are not tunable, so a
misconfiguration fails the release closed rather than open.

The job needs `actions: read` in its `permissions:` block for the workflow-runs API — an
explicit `permissions:` block sets every unlisted scope to `none`, so this is not covered by
`contents: write`.

## Snapshot verify (E9-S02)

| Script | Purpose |
| --- | --- |
| `task release-snapshot` | Local goreleaser snapshot under `dist/` |
| `hack/release/snapshot_test.sh` | REQ-E9-S02 gate: archives, checksums, stamped version |
| `hack/release/ci_audit_test.sh` | REQ-E9-S04 gate: no duplicate CodeQL workflow |

## Install script (E9-S07a)

| Script | Purpose |
| --- | --- |
| `hack/install.sh` | SHA256-verify archive, optional cosign, install binary |
| `hack/release/install_test.sh` | REQ-E9-S07a gate: mismatch reject + snapshot-no-sig + docs |
| `docs/usage/install.md` | Adopter docs (`go install`, curl script, Homebrew) |

Cosign skip-when-absent matches D-110 (autonomous/snapshot path). Use
`--require-signature` for post-S06 signed releases.

## Homebrew tap (E9-S07b, D-107)

| Path / task | Purpose |
| --- | --- |
| `.goreleaser.yaml` `brews` | Push `Formula/assent.rb` to `PlatformRelay/homebrew-tap` on tagged release |
| `hack/release/homebrew/assent.rb.template` | In-repo Formula review copy (placeholder checksums) |
| `hack/release/brew_test.sh` | REQ-E9-S07b-01 autonomous gate |
| `task release-brew-test` | Same check via Taskfile |
| `docs/usage/install.md` | Live Homebrew install (`brew tap` / `brew trust` / `brew install`) |

**Current state (2026-08-05):** Formula **`Formula/assent.rb`** is on
[`PlatformRelay/homebrew-tap`](https://github.com/PlatformRelay/homebrew-tap) for tag
**`v0.1.0`** (release rebuild after removing tagged `--skip=publish`). Adopter path:
`brew tap PlatformRelay/tap && brew trust PlatformRelay/tap && brew install assent`.

Prefer a **fine-grained PAT** for `HOMEBREW_TAP_GITHUB_TOKEN` (Contents: write on
`homebrew-tap` only); classic `repo` PAT works but is broader.

### Operator runbook — (re)publish Formula

1. **Confirm tap** — `https://github.com/PlatformRelay/homebrew-tap` (public, `main`).
2. **Token:** fine-grained PAT (owner **PlatformRelay**, only `homebrew-tap`, **Contents:
   Read and write**) as repo secret `HOMEBREW_TAP_GITHUB_TOKEN` on `PlatformRelay/assent`:
   ```bash
   gh secret set HOMEBREW_TAP_GITHUB_TOKEN -R PlatformRelay/assent
   ```
3. **Publish** on a tagged release (tagged job must **not** pass `--skip=publish` — that
   skips the brew publisher; `release.disable: true` already blocks goreleaser’s GH Release):
   ```bash
   gh workflow run release.yaml -R PlatformRelay/assent -f tag=v0.1.0   # rebuild
   # or: git tag vX.Y.Z && git push origin vX.Y.Z
   ```
4. **Verify:**
   ```bash
   gh api repos/PlatformRelay/homebrew-tap/contents/Formula/assent.rb --jq .path
   brew tap PlatformRelay/tap
   brew trust PlatformRelay/tap
   brew install assent
   assent version
   ```

**Autonomous path:** `task release-snapshot` and PR CI pass `--skip=homebrew`; goreleaser also sets
`skip_upload` when `HOMEBREW_TAP_GITHUB_TOKEN` is absent.

**REQ-E9-S07b-02** live `brew install` proof: done @ `v0.1.0` (manual).


## Artifact verify (E9-S12)

| Script / task | Purpose |
| --- | --- |
| `hack/release/verify-artifacts.sh` | Verify `dist/` checksums, stamped `assent version`, optional cosign |
| `task release-verify` | Same check against default `dist/` (run after `task release-snapshot`) |
| `hack/release/verify_test.sh` | REQ-E9-S12 gate: snapshot pass, tamper reject, cosign skip-when-absent |

Given a goreleaser `dist/` directory, `verify-artifacts.sh`:

1. **Checksums (required)** — every line in `checksums.txt` must match its archive; every
   archive must be listed (fail-closed on tamper).
2. **Cosign (optional, D-110)** — when a sibling `.sigstore.json` bundle exists beside an
   archive, `cosign verify-blob` runs; when absent (snapshot/autonomous path), verification
   **skips cosign** and succeeds. Use `--require-signature` to fail closed if bundles are
   missing (post-S06 signed releases).
3. **Stamped version** — each archive's `assent version` must match the expected semver
   (from `metadata.json`, archive names, or `--expected-version`).

Local gate: `task release-snapshot && task release-verify`. CI (E9-S05/S13) runs
`hack/release/verify_test.sh` after snapshot builds.

## Exit gate (E9-S13)

| Script / task | Purpose |
| --- | --- |
| `hack/release/exitgate_test.sh` | REQ-E9-S13-01/03: snapshot + verify + strict docs-build + backlog/decisions pins |
| `task release-exitgate-test` | Same check via Taskfile |

**D-111 CLOSED** @ tag `v0.1.0` (signed assets + live install + Homebrew Formula). Autonomous
exitgate still runs without publish credentials (snapshot path). See
`docs/decisions/decisions.md`.
