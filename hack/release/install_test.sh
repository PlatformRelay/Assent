#!/usr/bin/env bash
# REQ-E9-S07a-01..03: checksum-verified install.sh + docs.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

INSTALL="$ROOT/hack/install.sh"
MODE="${1:-all}"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

need_install() {
  [[ -x "$INSTALL" ]] || fail "hack/install.sh missing or not executable (REQ-E9-S07a-01)"
}

make_fixture() {
  local dir="$1"
  mkdir -p "$dir/payload"
  cat >"$dir/payload/assent" <<'EOF'
#!/usr/bin/env bash
echo "assent version fixture-0.0.0"
EOF
  chmod +x "$dir/payload/assent"
  tar -czf "$dir/assent_fixture_linux_amd64.tar.gz" -C "$dir/payload" assent
  (
    cd "$dir"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum assent_fixture_linux_amd64.tar.gz >checksums.txt
    else
      shasum -a 256 assent_fixture_linux_amd64.tar.gz >checksums.txt
    fi
  )
}

test_mismatch() {
  need_install
  local tmp dest
  tmp="$(mktemp -d)"
  dest="$(mktemp -d)"
  trap 'rm -rf "'"$tmp"'" "'"$dest"'"' RETURN
  make_fixture "$tmp"
  # Tamper checksum entry (fail-closed before extract).
  echo "0000000000000000000000000000000000000000000000000000000000000000  assent_fixture_linux_amd64.tar.gz" >"$tmp/checksums.txt"
  set +e
  "$INSTALL" --archive "$tmp/assent_fixture_linux_amd64.tar.gz" --checksums "$tmp/checksums.txt" --dest "$dest" >/tmp/install-mismatch.out 2>/tmp/install-mismatch.err
  local rc=$?
  set -e
  [[ "$rc" -eq 1 ]] || fail "checksum mismatch expected exit 1, got $rc (REQ-E9-S07a-01)"
  [[ ! -e "$dest/assent" ]] || fail "binary must not be installed after checksum mismatch"
  echo "OK: mismatch rejects (REQ-E9-S07a-01)"
}

test_snapshot_no_sig() {
  need_install
  local tmp dest
  tmp="$(mktemp -d)"
  dest="$(mktemp -d)"
  trap 'rm -rf "'"$tmp"'" "'"$dest"'"' RETURN
  make_fixture "$tmp"
  # No .sigstore.json beside archive — cosign skip path (D-110 / REQ-E9-S07a-02).
  "$INSTALL" --archive "$tmp/assent_fixture_linux_amd64.tar.gz" --checksums "$tmp/checksums.txt" --dest "$dest"
  [[ -x "$dest/assent" ]] || fail "expected installed assent binary (REQ-E9-S07a-02)"
  out="$("$dest/assent" version)"
  echo "$out" | grep -q 'fixture-0.0.0' || fail "unexpected version output: $out"
  echo "OK: snapshot-no-sig installs (REQ-E9-S07a-02)"
}

test_require_signature_absent() {
  need_install
  local tmp dest
  tmp="$(mktemp -d)"
  dest="$(mktemp -d)"
  trap 'rm -rf "'"$tmp"'" "'"$dest"'"' RETURN
  make_fixture "$tmp"
  set +e
  "$INSTALL" --archive "$tmp/assent_fixture_linux_amd64.tar.gz" --checksums "$tmp/checksums.txt" --dest "$dest" --require-signature >/tmp/install-reqsig.out 2>/tmp/install-reqsig.err
  local rc=$?
  set -e
  [[ "$rc" -eq 1 ]] || fail "--require-signature without bundle expected exit 1, got $rc (D-110)"
  echo "OK: --require-signature fails closed when bundles absent"
}

test_docs() {
  local doc="$ROOT/docs/usage/install.md"
  [[ -f "$doc" ]] || fail "docs/usage/install.md missing (REQ-E9-S07a-03)"
  grep -q 'go install' "$doc" || fail "install.md must document go install (REQ-E9-S07a-03)"
  grep -qi 'hack/install.sh\|install.sh' "$doc" || fail "install.md must document curl/install script"
  grep -qi 'homebrew-tap\|not yet available\|does not exist yet' "$doc" || fail "install.md must state tap is pending"
  echo "OK: install.md docs (REQ-E9-S07a-03)"
}

case "$MODE" in
  all)
    test_mismatch
    test_snapshot_no_sig
    test_require_signature_absent
    test_docs
    ;;
  snapshot-no-sig)
    test_snapshot_no_sig
    ;;
  mismatch)
    test_mismatch
    ;;
  docs)
    test_docs
    ;;
  *)
    fail "unknown mode: $MODE (all|snapshot-no-sig|mismatch|docs)"
    ;;
esac

echo "OK: install_test.sh ($MODE)"
