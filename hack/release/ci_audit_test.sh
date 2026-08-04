#!/usr/bin/env bash
# REQ-E9-S04-03 — exactly one CodeQL workflow (D-045 shipped; E9 must not duplicate).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

codeql_count="$(grep -l 'name: CodeQL' .github/workflows/*.yaml 2>/dev/null | wc -l | tr -d ' ')"
if [[ "$codeql_count" -ne 1 ]]; then
  echo "FAIL: expected exactly 1 CodeQL workflow, found $codeql_count" >&2
  exit 1
fi

if ! grep -q codeql.yaml docs/planning/ci-hardening-status.md; then
  echo "FAIL: ci-hardening-status.md must reference codeql.yaml (REQ-E9-S04-01)" >&2
  exit 1
fi

echo "OK: CI audit — single CodeQL workflow; hardening inventory present"
