#!/usr/bin/env bash
# REQ-E9-S10-03 — README must not embed demo GIFs until committed assets exist.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

if grep -qE 'demos/.*\.gif' README.md; then
  if ! compgen -G 'docs/assets/demos/*.gif' >/dev/null; then
    echo "FAIL: README references demo GIFs but none committed (REQ-E9-S10-03)" >&2
    exit 1
  fi
fi

echo "OK: demo GIF guard — README promises match committed assets"
