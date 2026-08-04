#!/usr/bin/env bash
# REQ-PCS-S09-01..03: PCS autonomous exit gate — full suite + E6 seed dir + schema drift guard.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

echo "== PCS-S09 schema drift guard (REQ-PCS-S09-02) =="
if [[ -n "$(git diff schemas/)" ]]; then
  git diff schemas/ >&2 || true
  fail "git diff schemas/ must be empty at PCS exit gate"
fi
echo "OK: schemas/ frozen (git diff schemas/ empty)"

echo "== PCS-S09 build assent (REQ-PCS-S09-01) =="
task build
BIN="$ROOT/bin/assent"
[[ -x "$BIN" ]] || fail "missing $BIN after task build"

echo "== PCS-S09 full suite compare (REQ-PCS-S09-01) =="
for suite in promotion-gates wording-only; do
  dir="$ROOT/examples/comparison/$suite"
  [[ -d "$dir" ]] || fail "missing corpus suite $dir"
  echo "-- assent compare --suite $suite"
  out="$("$BIN" compare --suite "$dir" 2>&1)" || fail "assent compare --suite $suite failed"
  echo "$out" | grep -q 'bounded-auto-merge-widening=PASS' \
    || fail "suite $suite stdout missing bounded-auto-merge-widening=PASS"
done
echo "OK: promotion-gates + wording-only suites green"

echo "== PCS-S09 E6 seed dir compare (REQ-PCS-S09-01) =="
seed="$ROOT/examples/comparison/e6-seed"
[[ -d "$seed" ]] || fail "missing E6 seed fixture $seed"
out="$("$BIN" compare "$seed" 2>&1)" || fail "assent compare e6-seed failed"
echo "$out" | grep -q 'delta=explanation-only' \
  || fail "e6-seed stdout missing delta=explanation-only"
echo "$out" | grep -q 'verdict=PASS' \
  || fail "e6-seed stdout missing verdict=PASS"
echo "OK: E6 single-dir seed layout green (explanation-only, exit 0)"

echo "== backlog / later-phases / decisions authoritative (REQ-PCS-S09-03) =="
backlog="$ROOT/openspec/specs/backlog.md"
later="$ROOT/openspec/specs/later-phases.md"
decisions="$ROOT/docs/decisions/decisions.md"

grep -q 'p5-pcs-policy-comparison/spec.md' "$backlog" \
  || fail "backlog must reference p5-pcs-policy-comparison/spec.md (REQ-PCS-S09-03)"
grep -qE 'PCS (status: )?\*\*AUTONOMOUS COMPLETE\*\*|\*\*PCS AUTONOMOUS COMPLETE\*\*' "$backlog" \
  || fail "backlog must mark PCS AUTONOMOUS COMPLETE (REQ-PCS-S09-03)"
grep -q 'D-057' "$backlog" && grep -q 'D-118' "$backlog" \
  || fail "backlog must cite D-057 closed + D-118 (REQ-PCS-S09-03)"

grep -q 'p5-pcs-policy-comparison/spec.md' "$later" \
  || fail "later-phases must reference p5-pcs-policy-comparison/spec.md (REQ-PCS-S09-03)"
grep -qE 'PCS.*AUTONOMOUS COMPLETE|AUTONOMOUS COMPLETE.*PCS' "$later" \
  || fail "later-phases must mark PCS AUTONOMOUS COMPLETE (REQ-PCS-S09-03)"
grep -q 'D-118' "$later" \
  || fail "later-phases must cite D-118 (REQ-PCS-S09-03)"

grep -q 'D-118' "$decisions" \
  || fail "decisions.md must contain D-118 (REQ-PCS-S09-03)"
grep -q 'D-057' "$decisions" \
  || fail "decisions.md must cross-reference D-057 (REQ-PCS-S09-03)"
grep -qi 'deferred scope.*closed\|D-057 deferred scope CLOSED' "$decisions" \
  || fail "decisions must record D-057 deferred scope closed (REQ-PCS-S09-03)"

readme="$ROOT/hack/compare/README.md"
grep -q exitgate_test.sh "$readme" \
  || fail "hack/compare/README.md must document exitgate_test.sh"

echo "OK: PCS autonomous exit gate — full runner shipped; D-057 deferred scope closed (D-118)"
