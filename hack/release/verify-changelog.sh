#!/usr/bin/env bash
# REQ-E9-S03-03: fail closed when CHANGELOG.md drifts from cliff.toml output.
#
# D-180: compares against the COMMITTED form rendered by render-changelog.sh —
# released versions only, no `## Unreleased` section. An ordinary commit
# therefore never makes this red. What does:
#   * a hand edit of CHANGELOG.md;
#   * a cliff.toml change that re-renders released history (header, grouping,
#     template);
#   * a pushed release tag whose section has not been stamped yet — the
#     post-tag `:memo: chore(release): stamp the vX.Y.Z section after tagging`
#     commit (`task changelog-write`) is what adds it.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

if [[ ! -f CHANGELOG.md ]]; then
  echo "verify-changelog: CHANGELOG.md missing — run 'task changelog-write' and commit" >&2
  exit 1
fi

scratch="$(mktemp)"
trap 'rm -f "${scratch}"' EXIT

bash hack/release/render-changelog.sh "${scratch}"

if ! diff -u CHANGELOG.md "${scratch}"; then
  echo "verify-changelog: CHANGELOG.md drift — it holds released versions only (D-180), so this means a hand edit, a cliff.toml change that re-renders released history, or a new tag not yet stamped: run 'task changelog-write' and commit it as ':memo: chore(release): …'" >&2
  exit 1
fi

echo "verify-changelog: ok"
