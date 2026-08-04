#!/usr/bin/env bash
# REQ-E9-S13-01..03: E9 autonomous exit gate — snapshot + verify + product docs + backlog.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

echo "== E9-S13 autonomous exit gate (REQ-E9-S13-01) =="
task release-snapshot
task release-verify
task docs-build
echo "OK: release-snapshot + release-verify + docs-build"

echo "== backlog / later-phases authoritative (REQ-E9-S13-03) =="
backlog="$ROOT/openspec/specs/backlog.md"
later="$ROOT/openspec/specs/later-phases.md"
decisions="$ROOT/docs/decisions/decisions.md"

grep -q 'p5-e9-distribution/spec.md' "$backlog" \
  || fail "backlog must reference p5-e9-distribution/spec.md (REQ-E9-S13-03)"
grep -qE 'E9 (status: )?\*\*AUTONOMOUS COMPLETE\*\*|\*\*E9 AUTONOMOUS COMPLETE\*\*' "$backlog" \
  || fail "backlog must mark E9 AUTONOMOUS COMPLETE (REQ-E9-S13-03)"
grep -q 'D-099' "$backlog" && grep -q 'D-110' "$backlog" \
  || fail "backlog must cite D-099–D-110 (REQ-E9-S13-03)"
grep -q 'D-111' "$backlog" \
  || fail "backlog must reference D-111 infra pending (REQ-E9-S13-03)"

grep -q 'p5-e9-distribution/spec.md' "$later" \
  || fail "later-phases must reference p5-e9-distribution/spec.md (REQ-E9-S13-03)"
grep -qE 'E9.*AUTONOMOUS COMPLETE|AUTONOMOUS COMPLETE.*E9' "$later" \
  || fail "later-phases must mark E9 AUTONOMOUS COMPLETE (REQ-E9-S13-03)"

grep -q 'D-111' "$decisions" \
  || fail "decisions.md must contain D-111 (REQ-E9-S13-03)"
grep -qi 'autonomous half.*closed\|autonomous slice.*closed' "$decisions" \
  || fail "D-111 must record autonomous half closed (REQ-E9-S13-03)"
grep -qi 'infra.*pending\|infra half.*pending\|infra-gated half' "$decisions" \
  || fail "D-111 must leave infra half pending (REQ-E9-S13-03)"

readme="$ROOT/hack/release/README.md"
grep -q exitgate_test.sh "$readme" \
  || fail "hack/release/README.md must document exitgate_test.sh"

echo "OK: E9 autonomous exit gate — D-099–D-110 cited; D-111 infra half pending"
