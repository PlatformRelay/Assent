#!/usr/bin/env bash
# REQ-E9-S12: verify dist/ release artifacts — checksums (required), optional cosign (D-110),
# stamped assent version in each archive.
#
# Usage:
#   hack/release/verify-artifacts.sh [--dist DIR] [--expected-version VERSION] [--require-signature]
set -euo pipefail

DIST="dist"
EXPECTED_VERSION=""
REQUIRE_SIGNATURE=0

usage() {
  cat <<'EOF' >&2
Usage: hack/release/verify-artifacts.sh [--dist DIR] [--expected-version VERSION] [--require-signature]

  --dist DIR               Directory with checksums.txt + archives (default: dist)
  --expected-version VER   Expected semver in each archive (default: metadata.json or archive names)
  --require-signature      Fail if no .sigstore.json bundle beside an archive (post-S06 releases)
EOF
  exit 2
}

die() {
  echo "verify-artifacts: $*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dist)
      [[ $# -ge 2 ]] || usage
      DIST="$2"
      shift 2
      ;;
    --expected-version)
      [[ $# -ge 2 ]] || usage
      EXPECTED_VERSION="$2"
      shift 2
      ;;
    --require-signature)
      REQUIRE_SIGNATURE=1
      shift
      ;;
    -h|--help)
      usage
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

[[ -d "$DIST" ]] || die "dist directory not found: $DIST"
DIST="$(cd "$DIST" && pwd)"

CHECKSUMS="${DIST}/checksums.txt"
[[ -f "$CHECKSUMS" ]] || die "checksums.txt missing in ${DIST}"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

version_from_archive_name() {
  local base="$1"
  # assent_<version>_<goos>_<goarch>.tar.gz|zip
  local stem="${base%.tar.gz}"
  stem="${stem%.zip}"
  local rest="${stem#assent_}"
  local arch="${rest##*_}"
  rest="${rest%_"${arch}"}"
  local goos="${rest##*_}"
  local version="${rest%_"${goos}"}"
  [[ -n "$version" ]] || return 1
  printf '%s\n' "$version"
}

resolve_expected_version() {
  if [[ -n "$EXPECTED_VERSION" ]]; then
    return 0
  fi
  local meta="${DIST}/metadata.json"
  if [[ -f "$meta" ]] && command -v python3 >/dev/null 2>&1; then
    EXPECTED_VERSION="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$meta" 2>/dev/null || true)"
  fi
  if [[ -z "$EXPECTED_VERSION" ]]; then
    local first
    first="$(find "$DIST" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) | head -1)"
    [[ -n "$first" ]] || die "no archives in ${DIST} and no --expected-version"
    EXPECTED_VERSION="$(version_from_archive_name "$(basename "$first")")" \
      || die "could not infer version from archive name $(basename "$first")"
  fi
}

find_sigstore_bundle() {
  local archive="$1"
  local base
  base="$(basename "$archive")"
  local candidate
  for candidate in \
    "${archive}.sigstore.json" \
    "${archive%.tar.gz}.sigstore.json" \
    "${archive%.zip}.sigstore.json" \
    "$(dirname "$archive")/${base}.sigstore.json"
  do
    if [[ -f "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

verify_cosign() {
  local archive="$1"
  local bundle
  if bundle="$(find_sigstore_bundle "$archive")"; then
    command -v cosign >/dev/null 2>&1 \
      || die "cosign required to verify ${bundle} but not found on PATH"
    # AUD2-F01, SEC-03's twin on the maintainer/CI path. This call used to run
    # unpinned, exactly as hack/install.sh's did before AUD2-S03: keyless
    # verify-blob without a signer pin either errors out on cosign v2 or accepts
    # ANY Fulcio certificate, so a mirror-swapped archive shipped with its own
    # validly signed bundle verified clean here and `--require-signature`
    # promised something it did not deliver.
    #
    # The pair below is the ONE published truth (D-153): byte-identical to
    # SECURITY.md's copy-paste recipe and to hack/install.sh, and
    # hack/release/install_cosign_pin_test.sh reddens if any of the three drifts.
    #
    # The [Aa] class is not cosmetic: the repository was renamed to
    # PlatformRelay/Assent between v0.1.0 and v0.2.0, the Fulcio SAN carries
    # GitHub's canonical casing, and cosign matches this regexp case-SENSITIVELY
    # — a lowercase-only pin rejects the project's own current releases. The dots
    # are escaped because this is a regexp, not a literal. The pin stays anchored
    # at the org/repo, so any other owner or an `assent-mirror` typosquat fails.
    cosign verify-blob \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com \
      --certificate-identity-regexp '^https://github\.com/PlatformRelay/[Aa]ssent/' \
      --bundle "$bundle" "$archive" >/dev/null \
      || die "cosign verification failed for ${archive}"
    echo "verify-artifacts: cosign ok $(basename "$archive")" >&2
  elif [[ "$REQUIRE_SIGNATURE" -eq 1 ]]; then
    die "--require-signature set but no .sigstore.json bundle found beside ${archive}"
  else
    echo "verify-artifacts: no .sigstore.json beside $(basename "$archive"); skipping cosign (D-110)" >&2
  fi
}

verify_archive_version() {
  local archive="$1"
  local expected="$2"
  local tmpdir base binary version_out reported rc

  tmpdir="$(mktemp -d)"

  base="$(basename "$archive")"
  case "$archive" in
    *.tar.gz|*.tgz)
      tar -xzf "$archive" -C "$tmpdir"
      ;;
    *.zip)
      command -v unzip >/dev/null 2>&1 || die "unzip required for ${base}"
      unzip -q "$archive" -d "$tmpdir"
      ;;
    *)
      rm -rf "$tmpdir"
      die "unsupported archive format: ${base}"
      ;;
  esac

  binary="$(find "$tmpdir" -type f \( -name assent -o -name 'assent.exe' \) | head -1)"
  if [[ -z "$binary" ]]; then
    rm -rf "$tmpdir"
    die "no assent binary inside ${base}"
  fi

  reported=""
  set +e
  version_out="$("$binary" version 2>/dev/null)"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 && -n "$version_out" ]]; then
    reported="${version_out#assent }"
  elif grep -Fxq "$expected" < <(strings "$binary"); then
    reported="$expected"
  else
    rm -rf "$tmpdir"
    die "version mismatch in ${base}: expected assent ${expected}, cannot execute archive binary and stamp not found via strings"
  fi
  rm -rf "$tmpdir"
  [[ "$reported" == "$expected" ]] \
    || die "version mismatch in ${base}: expected assent ${expected}, got assent ${reported}"
}

resolve_expected_version

declare -A listed=()
while read -r hash name rest; do
  [[ -n "${hash:-}" ]] || continue
  [[ "${hash}" =~ ^# ]] && continue
  name="${name:-}"
  name="${name#\*}"
  [[ -n "$name" ]] || continue

  archive="${DIST}/${name}"
  [[ -f "$archive" ]] || die "checksum entry references missing file: ${name}"

  actual="$(sha256_file "$archive")"
  [[ "$actual" == "$hash" ]] || die "SHA256 mismatch for ${name}: expected ${hash}, got ${actual}"

  listed["$name"]=1
  verify_cosign "$archive"
  verify_archive_version "$archive" "$EXPECTED_VERSION"
done <"$CHECKSUMS"

archive_count=0
while IFS= read -r archive; do
  base="$(basename "$archive")"
  [[ -n "${listed[$base]+x}" ]] || die "archive not listed in checksums.txt: ${base}"
  archive_count=$((archive_count + 1))
done < <(find "$DIST" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \))

[[ "$archive_count" -ge 1 ]] || die "no release archives found in ${DIST}"

echo "verify-artifacts: ok (version=${EXPECTED_VERSION}, archives=${archive_count})"
