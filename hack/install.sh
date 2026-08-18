#!/usr/bin/env bash
# Checksum-verified assent installer (E9-S07a, D-110).
#
# Usage:
#   hack/install.sh --archive PATH --checksums PATH [--dest DIR] [--require-signature]
#
# Verifies SHA256 of --archive against --checksums before extract (always fail-closed).
# Cosign: skip when no sibling *.sigstore.json bundles; verify when present;
# --require-signature fails closed if bundles are absent.
set -euo pipefail

ARCHIVE=""
CHECKSUMS=""
DEST=""
REQUIRE_SIGNATURE=0

usage() {
  cat <<'EOF' >&2
Usage: hack/install.sh --archive PATH --checksums PATH [--dest DIR] [--require-signature]

  --archive PATH         Release/snapshot archive (.tar.gz or .zip)
  --checksums PATH       SHA256 checksums file (goreleaser checksums.txt)
  --dest DIR             Install directory (default: /usr/local/bin or ~/.local/bin)
  --require-signature    Fail if no .sigstore.json bundle is present beside the archive
EOF
  exit 2
}

die() {
  echo "install.sh: $*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --archive)
      [[ $# -ge 2 ]] || usage
      ARCHIVE="$2"
      shift 2
      ;;
    --checksums)
      [[ $# -ge 2 ]] || usage
      CHECKSUMS="$2"
      shift 2
      ;;
    --dest)
      [[ $# -ge 2 ]] || usage
      DEST="$2"
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

[[ -n "$ARCHIVE" && -n "$CHECKSUMS" ]] || usage
[[ -f "$ARCHIVE" ]] || die "archive not found: $ARCHIVE"
[[ -f "$CHECKSUMS" ]] || die "checksums not found: $CHECKSUMS"

ARCHIVE="$(cd "$(dirname "$ARCHIVE")" && pwd)/$(basename "$ARCHIVE")"
CHECKSUMS="$(cd "$(dirname "$CHECKSUMS")" && pwd)/$(basename "$CHECKSUMS")"
base="$(basename "$ARCHIVE")"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

expected=""
while read -r hash name rest; do
  [[ -n "${hash:-}" ]] || continue
  [[ "${hash}" =~ ^# ]] && continue
  # goreleaser / sha256sum: "HASH  name" (name may include spaces in theory; we take $2)
  name="${name:-}"
  name="${name#\*}" # binary-mode marker from some sha256sum outputs
  if [[ "$name" == "$base" ]]; then
    expected="$hash"
    break
  fi
done <"$CHECKSUMS"

[[ -n "$expected" ]] || die "no checksum entry for ${base} in ${CHECKSUMS}"

actual="$(sha256_file "$ARCHIVE")"
[[ "$actual" == "$expected" ]] || die "SHA256 mismatch for ${base}: expected ${expected}, got ${actual}"

# Cosign / sigstore bundles (D-110): look beside the archive.
bundle=""
for candidate in \
  "${ARCHIVE}.sigstore.json" \
  "${ARCHIVE%.tar.gz}.sigstore.json" \
  "${ARCHIVE%.zip}.sigstore.json" \
  "$(dirname "$ARCHIVE")/${base}.sigstore.json"
do
  if [[ -f "$candidate" ]]; then
    bundle="$candidate"
    break
  fi
done

if [[ -n "$bundle" ]]; then
  command -v cosign >/dev/null 2>&1 || die "cosign required to verify ${bundle} but not found on PATH"
  # SEC-03 / REQ-AUD2-S03-01: pin the signer. Keyless verify-blob without an
  # identity pin accepts ANY Fulcio certificate, so a mirror-swapped archive
  # shipped with its own validly signed bundle would verify clean. The issuer and
  # identity regexp below are byte-identical to the pair SECURITY.md publishes;
  # hack/release/install_cosign_pin_test.sh reddens if the two ever drift apart.
  #
  # The [Aa] class is not cosmetic: the repository was renamed to PlatformRelay/
  # Assent between v0.1.0 and v0.2.0, and the Fulcio SAN carries GitHub's
  # canonical casing — v0.1.0's certificate says `assent`, v0.2.0/v0.3.0's say
  # `Assent`. cosign matches this regexp case-SENSITIVELY, so a lowercase-only
  # pin rejects the project's own current releases. The dots are escaped because
  # this is a regexp, not a literal. The pin is still anchored at the org/repo:
  # any other owner, or an `assent-mirror`-style typosquat, fails.
  cosign verify-blob \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity-regexp '^https://github\.com/PlatformRelay/[Aa]ssent/' \
    --bundle "$bundle" "$ARCHIVE" >/dev/null \
    || die "cosign verification failed for ${ARCHIVE}"
elif [[ "$REQUIRE_SIGNATURE" -eq 1 ]]; then
  die "--require-signature set but no .sigstore.json bundle found beside ${ARCHIVE}"
else
  echo "install.sh: no .sigstore.json bundle beside archive; skipping cosign (D-110)" >&2
fi

if [[ -z "$DEST" ]]; then
  if [[ -d /usr/local/bin && -w /usr/local/bin ]]; then
    DEST=/usr/local/bin
  else
    DEST="${HOME}/.local/bin"
  fi
fi
mkdir -p "$DEST"

tmpdir="$(mktemp -d)"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT

case "$ARCHIVE" in
  *.tar.gz|*.tgz)
    tar -xzf "$ARCHIVE" -C "$tmpdir"
    ;;
  *.zip)
    command -v unzip >/dev/null 2>&1 || die "unzip required for .zip archives"
    unzip -q "$ARCHIVE" -d "$tmpdir"
    ;;
  *)
    die "unsupported archive format: ${ARCHIVE} (want .tar.gz or .zip)"
    ;;
esac

binary="$(find "$tmpdir" -type f \( -name assent -o -name 'assent.exe' \) | head -1)"
[[ -n "$binary" ]] || die "no assent binary inside ${ARCHIVE}"
chmod +x "$binary"

target="$DEST/assent"
if [[ "$binary" == *.exe ]]; then
  target="$DEST/assent.exe"
fi
cp "$binary" "$target"
chmod +x "$target"

echo "install.sh: installed ${target}"
