#!/usr/bin/env bash
# REQ-E9-S07b-01 autonomous gate — goreleaser brews + empirical Formula generation (D-107).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

ensure_goreleaser() {
  export PATH="$(go env GOPATH)/bin:${PATH}"
  if ! command -v goreleaser >/dev/null 2>&1 || ! goreleaser --version 2>&1 | grep -q 'version 2'; then
    go install github.com/goreleaser/goreleaser/v2@v2.9.0
  fi
}

test_goreleaser_brews_config() {
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
  grep -q 'url_template:' .goreleaser.yaml \
    || fail "brews must set url_template when release.disable is true (F2)"
  grep -q 'PlatformRelay/assent/releases/download' .goreleaser.yaml \
    || fail "url_template must point at PlatformRelay/assent GitHub release assets"
  grep -qE 'ids:\s*$|ids:\n\s+- default|- default' .goreleaser.yaml \
    || fail "brews.ids must reference archive id 'default', not build id 'assent' (F1)"
  if grep -A2 'ids:' .goreleaser.yaml | grep -q 'assent'; then
    fail "brews.ids must not reference build id 'assent' — archives Extra.ID is 'default' (F1)"
  fi
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

# Tagged publish must NOT --skip=publish: that flag skips the Homebrew publisher
# (goreleaser release.disable already blocks GH Release; softprops publishes assets).
test_publish_allows_brew_upload() {
  local wf=.github/workflows/release.yaml
  local release_block
  release_block="$(awk '/^  release:/{flag=1; next} flag && /^  [a-zA-Z0-9_-]+:/{exit} flag' "$wf")"
  [[ -n "$release_block" ]] || fail "could not extract release job from $wf"
  if echo "$release_block" | grep -E 'args:.*--skip=publish([,]|$)|args:.*--skip=[^#[:space:]]*publish'; then
    fail "tagged release job must not --skip=publish (that skips Homebrew tap push)"
  fi
  echo "$release_block" | grep -q 'HOMEBREW_TAP_GITHUB_TOKEN' \
    || fail "tagged release job must pass HOMEBREW_TAP_GITHUB_TOKEN"
  echo "OK: tagged release allows Homebrew tap publish"
}

test_install_docs() {
  local doc=docs/usage/install.md
  [[ -f "$doc" ]] || fail "missing docs/usage/install.md"
  grep -qi 'Homebrew' "$doc" || fail "install.md must document Homebrew section"
  grep -q 'PlatformRelay/homebrew-tap' "$doc" \
    || fail "install.md must link PlatformRelay/homebrew-tap"
  if grep -qiE 'does not exist yet|not yet available|not yet been published|Formula .+ pending' "$doc"; then
    fail "install.md must not claim Homebrew/Formula is unpublished (live @ v0.1.0)"
  fi
  grep -q 'brew tap PlatformRelay/tap' "$doc" \
    || fail "install.md must show brew tap PlatformRelay/tap"
  grep -q 'brew install assent' "$doc" \
    || fail "install.md must show brew install assent"
  grep -qiE 'brew trust|untrusted tap' "$doc" \
    || fail "install.md must document brew trust for third-party taps"
  echo "OK: install.md Homebrew section live (REQ-E9-S07b-02/03)"
}

test_goreleaser_generates_formula() {
  ensure_goreleaser
  local formula=dist/homebrew/Formula/assent.rb
  rm -rf dist/homebrew
  # Generate Formula locally: homebrew pipe runs, skip_upload prevents tap push (no token).
  goreleaser release --snapshot --clean --skip=publish,sign,sbom \
    || fail "goreleaser must generate homebrew formula with archive id default (F1)"
  [[ -f "$formula" ]] || fail "expected generated formula at $formula (F3)"
  grep -q 'class Assent < Formula' "$formula" \
    || fail "generated formula must define Assent class (F3)"
  grep -q 'bin.install "assent"' "$formula" \
    || fail "generated formula must install assent binary (F3)"
  grep -q 'PlatformRelay/assent/releases/download' "$formula" \
    || fail "generated formula must use GitHub release asset URLs from url_template (F2)"
  grep -q 'sha256 "' "$formula" \
    || fail "generated formula must include archive checksums (F3)"
  echo "OK: goreleaser generated Formula at $formula (F3)"
}

test_goreleaser_brews_config
test_formula_template
test_snapshot_skips_brew
test_publish_allows_brew_upload
test_install_docs
test_goreleaser_generates_formula

echo "OK: E9-S07b autonomous gates"
