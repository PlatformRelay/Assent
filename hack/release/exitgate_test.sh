#!/usr/bin/env bash
# REQ-E9-S13-01..03: E9 autonomous exit gate — snapshot + verify + product docs + backlog.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

echo "== release-exitgate CI env (uv for docs-install) =="
wf="$ROOT/.github/workflows/verify.yaml"
grep -q 'release-exitgate:' "$wf" \
  || fail "verify.yaml missing release-exitgate job"
# Job must install uv before task docs-build (docs-install uses uv).
awk '
  /^  release-exitgate:/ {injob=1; next}
  injob && /^  [a-zA-Z0-9_-]+:/ {injob=0}
  injob {print}
' "$wf" | grep -E 'astral-sh/setup-uv@|astral\.sh/uv/install' \
  || fail "release-exitgate must install uv (astral-sh/setup-uv or astral.sh installer) before docs-build"

# AUD-S03: the release job's verify-green gate is itself a release gate — run its polarity
# table + step-order assertion here so REQ-AUD-S03-02 is enforced by CI (this script runs in
# verify.yaml's release-exitgate job) and not only when someone types the command. Fast and
# offline: it drives a stubbed `gh`, no network. Needs jq, which GitHub-hosted runners ship.
echo "== AUD-S03 release verify-tag gate (REQ-AUD-S03-01/02) =="
bash "$ROOT/hack/release/verify_tag_gate_test.sh"

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
  || fail "backlog must reference D-111 (REQ-E9-S13-03)"

grep -q 'p5-e9-distribution/spec.md' "$later" \
  || fail "later-phases must reference p5-e9-distribution/spec.md (REQ-E9-S13-03)"
grep -qE 'E9.*AUTONOMOUS COMPLETE|AUTONOMOUS COMPLETE.*E9' "$later" \
  || fail "later-phases must mark E9 AUTONOMOUS COMPLETE (REQ-E9-S13-03)"

grep -q 'D-111' "$decisions" \
  || fail "decisions.md must contain D-111 (REQ-E9-S13-03)"
# Closed form (post-v0.1.0) or legacy "autonomous half closed" while infra was still pending.
grep -qiE 'exit gate CLOSED|autonomous half.*closed|autonomous slice.*closed' "$decisions" \
  || fail "D-111 must record exit gate / autonomous half closed (REQ-E9-S13-03)"
# Infra: proven @ tag with optional Homebrew residual, or legacy pending phrasing.
grep -qiE 'optional residual|infra half proven|Homebrew formula push optional|infra.*pending|infra half.*pending|infra-gated half' "$decisions" \
  || fail "D-111 must record infra proven/optional residual (or legacy pending) (REQ-E9-S13-03)"

readme="$ROOT/hack/release/README.md"
grep -q exitgate_test.sh "$readme" \
  || fail "hack/release/README.md must document exitgate_test.sh"

echo "OK: E9 autonomous exit gate — D-099–D-110 cited; D-111 closed (optional Homebrew residual)"
