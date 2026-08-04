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
- **CI (E9-S05):** the release workflow and/or `verify.yaml` changelog job will run
  `task changelog-verify` on PRs and main — not bundled into the standard `task check` path.

### Release workflow hook (E9-S05)

The tag-triggered `.github/workflows/release.yaml` lane (E9-S05) will consume git-cliff output
for GitHub Release notes — same pattern as mkurator:

1. Checkout with `fetch-depth: 0` (full history + tags).
2. Run **`orhun/git-cliff-action`** (SHA-pinned) with `config: cliff.toml` and
   `args: --latest --strip header` so the action emits the latest tagged section body.
3. Pass `${{ steps.cliff.outputs.content }}` into the release-notes assembly step (install footer,
   artifact links) before `softprops/action-gh-release`.

Until S05 lands, maintainers regenerate `CHANGELOG.md` on main via `task changelog-write` after
merging user-facing commits; `verify-changelog.sh` keeps the committed file in sync.

## Snapshot verify (E9-S02)

| Script | Purpose |
| --- | --- |
| `task release-snapshot` | Local goreleaser snapshot under `dist/` |
| `hack/release/snapshot_test.sh` | REQ-E9-S02 gate: archives, checksums, stamped version |
| `hack/release/ci_audit_test.sh` | REQ-E9-S04 gate: no duplicate CodeQL workflow |
