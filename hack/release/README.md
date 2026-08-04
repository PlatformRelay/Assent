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
2. **Build:** `goreleaser release --clean --skip=publish` (`.goreleaser.yaml` keeps
   `release.disable: true`; publish is via `softprops/action-gh-release`).
3. Run **`orhun/git-cliff-action`** (SHA-pinned) with `config: cliff.toml` and
   `args: --latest --strip header` so the action emits the latest tagged section body.
4. **`softprops/action-gh-release`** uploads `dist/` archives + `checksums.txt` with cliff body.

**Triggers:** `push.tags: v*.*.*`, `workflow_dispatch` (rebuild an existing tag), and PR
`snapshot` dry-run on release-related paths.

**REQ-E9-S05-03 autonomous path:** PR `snapshot` job **or** local `task release-snapshot` /
`hack/release/snapshot_test.sh` — both run goreleaser without publish credentials.

**E9-S06 hooks (next lane):** comments in `release.yaml` mark where cosign, SBOM, and
`actions/attest` SLSA attach (D-109).

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
| `docs/usage/install.md` | Adopter docs (`go install`, curl script, Homebrew coming soon) |

Cosign skip-when-absent matches D-110 (autonomous/snapshot path). Use
`--require-signature` for post-S06 signed releases.

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
