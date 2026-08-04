#!/usr/bin/env bash
# REQ-E9-S10-01..03: VHS tape sources, demo README, README GIF guard.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

echo "== REQ-E9-S10-01: >=3 .tape files under docs/assets/demos/ =="
tape_count="$(find docs/assets/demos -maxdepth 1 -name '*.tape' 2>/dev/null | wc -l | tr -d ' ')"
if [[ "$tape_count" -lt 3 ]]; then
  echo "FAIL: expected >=3 .tape files, found ${tape_count} (REQ-E9-S10-01)" >&2
  exit 1
fi
echo "OK: ${tape_count} tape(s)"

echo "== REQ-E9-S10-02: demo README documents vhs pin + render command =="
if [[ ! -f docs/assets/demos/README.md ]]; then
  echo "FAIL: docs/assets/demos/README.md missing (REQ-E9-S10-02)" >&2
  exit 1
fi
if ! grep -q vhs docs/assets/demos/README.md; then
  echo "FAIL: demo README must document vhs (REQ-E9-S10-02)" >&2
  exit 1
fi
if ! grep -q 'v0.10.0' docs/assets/demos/README.md; then
  echo "FAIL: demo README must pin vhs version (REQ-E9-S10-02)" >&2
  exit 1
fi
echo "OK: demo README"

echo "== REQ-E9-S10-03: README demo GIF guard =="
bash hack/release/demo_guard_test.sh

if command -v vhs >/dev/null 2>&1; then
  echo "== optional: vhs validate =="
  vhs validate docs/assets/demos/*.tape
fi

echo "OK: demo_test.sh"
