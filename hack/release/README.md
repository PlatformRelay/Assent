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
2. **Build:** `goreleaser release --clean` (`.goreleaser.yaml` keeps
   `release.disable: true` so goreleaser does not create the GitHub Release; Homebrew tap
   push still runs when `HOMEBREW_TAP_GITHUB_TOKEN` is set — do **not** pass `--skip=publish`,
   which skips the brew publisher). Softprops uploads archives below.
3. Run **`orhun/git-cliff-action`** (SHA-pinned) with `config: cliff.toml` and
   `args: --latest --strip header` so the action emits the latest tagged section body.
4. **`softprops/action-gh-release`** uploads `dist/` archives + `checksums.txt` with cliff body.

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
4. **`softprops/action-gh-release`** uploads archives, checksums, SBOMs, sigstore bundles, and
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
| `docs/usage/install.md` | Adopter docs (`go install`, curl script, Homebrew tap pending) |

Cosign skip-when-absent matches D-110 (autonomous/snapshot path). Use
`--require-signature` for post-S06 signed releases.

## Homebrew tap (E9-S07b, D-107)

| Path / task | Purpose |
| --- | --- |
| `.goreleaser.yaml` `brews` | Push `Formula/assent.rb` to `PlatformRelay/homebrew-tap` on tagged release |
| `hack/release/homebrew/assent.rb.template` | In-repo Formula review copy (placeholder checksums) |
| `hack/release/brew_test.sh` | REQ-E9-S07b-01 autonomous gate |
| `task release-brew-test` | Same check via Taskfile |
| `docs/usage/install.md` | Honest Homebrew status — Formula not yet published |

**Current state (2026-08-05):** Tap repo
[`PlatformRelay/homebrew-tap`](https://github.com/PlatformRelay/homebrew-tap) exists
(README only). Tag **`v0.1.0`** released without Formula push because
`HOMEBREW_TAP_GITHUB_TOKEN` is unset (`skip_upload`). Curl / `go install` work.

### Operator runbook — publish Formula

1. **Confirm tap** — `https://github.com/PlatformRelay/homebrew-tap` (public, `main`).
2. **Create a fine-grained PAT** (GitHub → Settings → Developer settings → Fine-grained
   tokens): resource owner **PlatformRelay**; repository access **Only** `homebrew-tap`;
   permission **Contents: Read and write**. (Classic `repo` PAT works; prefer fine-grained.)
3. **Add repo secret** on `PlatformRelay/assent` named exactly `HOMEBREW_TAP_GITHUB_TOKEN`:
   ```bash
   gh secret set HOMEBREW_TAP_GITHUB_TOKEN -R PlatformRelay/assent
   gh secret list -R PlatformRelay/assent   # name only; never log the value
   ```
4. **Publish Formula** (pick one):
   - **A (recommended):** rebuild existing tag — no new semver:
     ```bash
     gh workflow run release.yaml -R PlatformRelay/assent -f tag=v0.1.0
     ```
   - **B:** cut a patch tag (`v0.1.1`) on green `main` if you want a distinct “first brew”
     release line (`git tag` + `git push origin <tag>` — never force-push tags).
5. **Verify:**
   ```bash
   gh api repos/PlatformRelay/homebrew-tap/contents/Formula/assent.rb --jq .path
   brew tap PlatformRelay/tap
   brew install assent
   assent version
   ```
6. **Docs follow-up:** flip `docs/usage/install.md` from “Formula not yet published” to live
   `brew` instructions after step 5 succeeds.

**Autonomous path:** `task release-snapshot` and PR CI pass `--skip=homebrew`; goreleaser also sets
`skip_upload` when `HOMEBREW_TAP_GITHUB_TOKEN` is absent. Adopters use `go install` or
`hack/install.sh` until `brew tap PlatformRelay/tap && brew install assent` works.

**REQ-E9-S07b-02** (live `brew install` proof) remains infra-gated — verify manually after the
runbook above.


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

## Exit gate (E9-S13 autonomous half)

| Script / task | Purpose |
| --- | --- |
| `hack/release/exitgate_test.sh` | REQ-E9-S13-01/03: snapshot + verify + strict docs-build + backlog/decisions pins |
| `task release-exitgate-test` | Same check via Taskfile |

Autonomous close path: no publish credentials required. **D-111 infra half** (tagged signed
release, live install proof) remains operator-gated — see `docs/decisions/decisions.md`.
