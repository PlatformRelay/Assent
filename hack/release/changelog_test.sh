#!/usr/bin/env bash
# REQ-E9-S03-01..03: git-cliff output has no SHAs; CHANGELOG seeded; verify fails on drift.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

echo "== REQ-E9-S03-01: unreleased changelog contains no commit SHAs =="
if task changelog | grep -qE '[0-9a-f]{7,40}'; then
  echo "FAIL: task changelog output contains SHA-like tokens (REQ-E9-S03-01)" >&2
  exit 1
fi
echo "OK: no SHA tokens in unreleased preview"

echo "== REQ-E9-S03-02: CHANGELOG.md has released sections + historical stub, and NO Unreleased section (D-180) =="
if [[ ! -f CHANGELOG.md ]]; then
  echo "FAIL: CHANGELOG.md missing (REQ-E9-S03-02)" >&2
  exit 1
fi
# D-180: the committed file holds released versions only; `task changelog` is
# where the unreleased entries are shown.
if grep -qE '^## Unreleased$' CHANGELOG.md; then
  echo "FAIL: CHANGELOG.md carries an Unreleased section — it holds released versions only since D-180 (REQ-E9-S03-02)" >&2
  exit 1
fi
if ! grep -qE '^## \[[0-9]+\.[0-9]+\.[0-9]+\] - ' CHANGELOG.md; then
  echo "FAIL: CHANGELOG.md has no released section (REQ-E9-S03-02)" >&2
  exit 1
fi
if ! grep -q 'Pre-release development' CHANGELOG.md; then
  echo "FAIL: CHANGELOG.md missing historical stub (REQ-E9-S03-02)" >&2
  exit 1
fi
echo "OK: CHANGELOG.md structure"

echo "== REQ-E9-S03-03: verify-changelog passes when in sync =="
bash hack/release/verify-changelog.sh

echo "== REQ-E9-S03-03: verify-changelog fails closed on stale CHANGELOG =="
backup="$(mktemp)"
cp CHANGELOG.md "${backup}"
trap 'mv -f "${backup}" CHANGELOG.md' EXIT
printf '\n<!-- stale drift probe -->\n' >>CHANGELOG.md
if bash hack/release/verify-changelog.sh 2>/dev/null; then
  echo "FAIL: verify-changelog should exit non-zero on drift (REQ-E9-S03-03)" >&2
  exit 1
fi
mv -f "${backup}" CHANGELOG.md
trap - EXIT
echo "OK: verify-changelog fail-closed on drift"

echo "OK: changelog_test.sh"
