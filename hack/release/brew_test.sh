#!/usr/bin/env bash
# REQ-E9-S07b-01 autonomous gate — goreleaser brews + in-repo formula template (D-107).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

test_goreleaser_brews() {
  grep -q '^brews:' .goreleaser.yaml \
    || fail ".goreleaser.yaml missing brews (REQ-E9-S07b-01 / D-107)"
  grep -q 'name: homebrew-tap' .goreleaser.yaml \
    || fail "brews must target PlatformRelay/homebrew-tap (D-107)"
  grep -q 'owner: PlatformRelay' .goreleaser.yaml \
    || fail "brews repository owner must be PlatformRelay"
  grep -q 'HOMEBREW_TAP_GITHUB_TOKEN' .goreleaser.yaml \
    || fail "brews must use HOMEBREW_TAP_GITHUB_TOKEN for tap push"
  grep -q 'skip_upload:' .goreleaser.yaml \
    || fail "brews must skip tap upload when token absent (autonomous snapshot path)"
  echo "OK: goreleaser brews configured (REQ-E9-S07b-01)"
}

test_formula_template() {
  local tpl=hack/release/homebrew/assent.rb.template
  [[ -f "$tpl" ]] || fail "missing in-repo formula template at $tpl"
  grep -q 'class Assent < Formula' "$tpl" || fail "formula template must define Assent Formula"
  grep -q 'bin.install "assent"' "$tpl" || fail "formula template must install assent binary"
  echo "OK: in-repo formula template present"
}

test_snapshot_skips_brew() {
  grep -q 'skip=publish,sign,sbom,homebrew' Taskfile.yml \
    || fail "task release-snapshot must skip homebrew (no tap locally)"
  grep -q 'skip=publish,sign,sbom,homebrew' .github/workflows/release.yaml \
    || fail "PR snapshot job must skip homebrew"
  echo "OK: snapshot paths skip homebrew publish"
}

test_install_docs() {
  local doc=docs/usage/install.md
  [[ -f "$doc" ]] || fail "missing docs/usage/install.md"
  grep -qi 'Homebrew' "$doc" || fail "install.md must document Homebrew section"
  grep -qi 'not yet available\|does not exist yet' "$doc" \
    || fail "install.md must state Homebrew is not yet available"
  grep -qi 'When the tap lands' "$doc" \
    || fail "install.md must document future tap install path"
  echo "OK: install.md Homebrew section honest (REQ-E9-S07b-03)"
}

test_goreleaser_brews
test_formula_template
test_snapshot_skips_brew
test_install_docs

echo "OK: E9-S07b autonomous gates"
