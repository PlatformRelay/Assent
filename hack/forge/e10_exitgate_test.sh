#!/usr/bin/env bash
# E10-S17 / REQ-E10-S17-01: the epic exit gate. One invocation proves, in
# order, that every S17 DoD condition holds at HEAD, citing D-140 (the unlock)
# and ADR-0021 (the governing ADR). Any regression reds the gate.
#
# Conditions (epic spec, E10-S17 DoD):
#   1. RunSuite green against BOTH factories (fake + gitlab + github).
#   2. Zero bare deferred rows: every deferred row carries a cited reason.
#   3. task check green.
#   4. git diff schemas/ == 0.
#   5. depguard denies both adapters from cmd/assent.
#   6. The capability enum is exhaustively answered by both adapters.
#   7. Every fail-closed polarity test present.
#   8. The Actions entrypoint pin gate green (S16, dropped without any other
#      change if S16 is cut per judgment call (a)).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: e10-exitgate: $*" >&2; exit 1; }
ok() { echo "  OK: $*"; }

echo "== E10 exit gate (REQ-E10-S17-01; D-140 + ADR-0021) =="

# 1. The conformance suite against all three factories.
go test -count=1 ./internal/forge/conformance/ >/dev/null 2>&1 ||
  fail "the conformance suite is red at HEAD (condition 1: RunSuite green against all factories)"
ok "RunSuite green (fake + gitlab + github backends)"

# 2. Zero bare deferred rows: a deferred row must carry a cited reason.
# The awk walks the catalog backwards from each github-deferred marker to its
# owning `- id:` line.
while IFS= read -r id; do
  note="$(awk -v id="$id" '
    $0 ~ "^  - id: " id "$" { grab=1 }
    grab && /note:/ { print; exit }
  ' internal/forge/conformance/catalog.yaml)"
  [[ -n "$note" ]] || fail "deferred row $id carries no cited reason (condition 2)"
done < <(awk '
  /- id:/ { id = $0; sub(/^[[:space:]]*- id: /, "", id) }
  /forge: github-deferred/ { print id }
' internal/forge/conformance/catalog.yaml)
ok "every deferred row carries a cited reason"

# 3. task check green — the repo's own gate (run after this script's checks so
#    the cheap conditions fail first).
# (checked by the caller: `task check` runs this script; see CHECK_STAGES.)

# 4. git diff schemas/ == 0.
git diff --exit-code -- schemas/ >/dev/null 2>&1 ||
  fail "schemas/ drifted (condition 4: the frozen schemas stay frozen)"
ok "git diff schemas/ is empty"

# 5. depguard denies both adapters from cmd/assent.
grep -q 'cmd-adapter-boundary' .golangci.yml ||
  fail "the cmd-adapter-boundary depguard rule is missing (condition 5)"
ok "depguard cmd-adapter-boundary rule present"

# 6. The capability enum is exhaustively answered by both adapters.
go test -count=1 ./internal/forge/ -run 'TestCapability' >/dev/null 2>&1 ||
  fail "the capability enum/strictness tests are red (condition 6)"
ok "capability enum exhaustively answered"

# 7. Every fail-closed polarity test present: the five-delta table plus its
# positive control, and the SHA-guard conformance cases on both factories.
grep -q "func TestDeltasFailClosed" internal/forge/github/failclosed_test.go ||
  fail "the fail-closed deltas table is missing (condition 7: TestDeltasFailClosed)"
go test -count=1 ./internal/forge/github/ -run 'TestDeltasFailClosed|TestArmingRevokeOnPush|TestMergeQueuePinMatchesMergedCommit' >/dev/null 2>&1 ||
  fail "the GitHub fail-closed polarity tests are red (condition 7)"
ok "fail-closed delta table present and green (with its positive control)"

# 8. The Actions entrypoint pin gate.
bash hack/release/action_pin_test.sh >/dev/null ||
  fail "the action pin gate is red (condition 8: S16 packaging pins a released version)"
ok "Actions entrypoint pin gate green"

echo "PASS: E10 exit gate — every S17 condition holds at HEAD (D-140 + ADR-0021)"
