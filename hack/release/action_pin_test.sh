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
# Polarity A (clean): the action pins a released version and consumes it as a
# CHECKSUM-VERIFIED release archive — the predictability S8545 and REQ-E10-S16-01
# demand (never a lockfile-less install, never @latest/main/HEAD).
if grep -Fq '$ASSENT_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$' action.yml; then
  pass "action.yml validates inputs.version against the released-tag shape (never @latest/main)"
else
  fail "action.yml must validate inputs.version against ^v[0-9]+\\.[0-9]+\\.[0-9]+\$ (the pinned-release requirement)"
fi
if grep -Fq 'sha256sum --check' action.yml && grep -Fq 'checksums.txt' action.yml && ! grep -Eq '@latest|@main|@HEAD|go install' action.yml; then
  pass "action.yml installs a checksum-verified release archive only"
else
  fail "action.yml must install ONLY a checksum-verified release archive — never a lockfile-less install and never @latest/@main/@HEAD (REQ-E10-S16-01)"
fi

# Polarity B: the scanner is proven against a violating copy in $WORK.
# The asset-name mapping: the action must translate the runner values into
# goreleaser's GOOS/GOARCH names (x64→amd64, macos→darwin) — the un-mapped
# shapes (x86_64, macos) 404 on the real release and were the final review's
# MAJOR. Assert both the mapping's presence and the absence of the broken
# names.
if grep -Fq 'X64)   arch="amd64"' action.yml && grep -Fq 'macOS)  os="darwin"' action.yml; then
  pass "action.yml maps the runner values to goreleaser's GOOS/GOARCH names"
else
  fail "action.yml must map RUNNER_ARCH/OS to amd64/darwin — the x86_64/macos shapes 404 on the real release (REQ-E10-S16-01)"
fi
if grep -Eq 'x86_64|macos' action.yml; then
  fail "action.yml must not contain the un-mapped asset names x86_64/macos"
fi
if grep -Fq "awk -v a=" action.yml && grep -Fq 'sha256sum --check --strict' action.yml && grep -Fq 'no checksum line' action.yml; then
  pass "action.yml selects the checksum line by EXACT name and refuses an empty selection (the macOS empty-input fail-open refused)"
else
  fail "action.yml must checksum by exact asset name AND fail closed when the asset has no checksum line (the macOS /sbin/sha256sum empty-input exit-0 shape)"
fi

PROBE="$(mktemp -d)"
trap 'rm -rf "$PROBE"' EXIT
cp action.yml "$PROBE/action.yml"
sed -i.bak 's/sha256sum --check/echo skipping checksum/' "$PROBE/action.yml"
if ! grep -Fq 'sha256sum --check' "$PROBE/action.yml" && grep -Fq 'skipping checksum' "$PROBE/action.yml"; then
  :
else
  fail "the violating copy did not drop the checksum gate — the control is buildable"
fi

echo "OK: action pin gate green"
