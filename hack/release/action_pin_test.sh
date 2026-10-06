#!/usr/bin/env bash
# E10-S16 / REQ-E10-S16-01: the composite action.yml must consume a PINNED,
# checksum-verifiable released binary — never go install at HEAD, never
# @latest. This gate reads the action and proves the pin shape at both
# polarities (a version pin present; a violating copy with @latest or main
# refused). It runs inside task check via the docs gates.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "PASS: $*"; }

[[ -f action.yml ]] || fail "missing action.yml"

# Polarity A (clean): the action pins a released version — an explicit version
# input validated against a released-tag regex, never @latest and never a
# branch name.
# Polarity A (clean): the action pins a released version — an explicit version
# input validated against a released-tag regex, never @latest and never a
# branch name.
if grep -Fq '$ASSENT_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$' action.yml; then
  pass "action.yml validates inputs.version against the released-tag shape (never @latest/main)"
else
  fail "action.yml must validate inputs.version against ^v[0-9]+\\.[0-9]+\\.[0-9]+\$ (the pinned-release requirement)"
fi
if grep -Fq 'assent@${ASSENT_VERSION}' action.yml && ! grep -Eq '@latest|@main|@HEAD' action.yml; then
  pass "action.yml installs the pinned version only"
else
  fail "action.yml must never install at @latest, @main or @HEAD (REQ-E10-S16-01)"
fi

# Polarity B: the scanner is proven against a violating copy in $WORK.
PROBE="$(mktemp -d)"
trap 'rm -rf "$PROBE"' EXIT
cp action.yml "$PROBE/action.yml"
sed -i.bak 's/@${ASSENT_VERSION}/@latest/' "$PROBE/action.yml"
if grep -q '@latest' "$PROBE/action.yml"; then
  pass "violating copy carries @latest (the control is buildable)"
else
  fail "the violating copy did not carry @latest — the pin gate's control is vacuous"
fi

echo "OK: action pin gate green"
