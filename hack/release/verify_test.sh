#!/usr/bin/env bash
# REQ-E9-S12-01..03: release artifact verify harness gates.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

VERIFY="$ROOT/hack/release/verify-artifacts.sh"
MODE="${1:-all}"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

need_verify() {
  [[ -x "$VERIFY" ]] || fail "hack/release/verify-artifacts.sh missing or not executable (REQ-E9-S12-01)"
}

test_snapshot_pass() {
  need_verify
  task release-snapshot
  task release-verify
  echo "OK: snapshot + release-verify pass (REQ-E9-S12-01)"
}

test_negative_tamper() {
  need_verify
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "'"$tmp"'"' RETURN
  mkdir -p "$tmp/payload"
  cat >"$tmp/payload/assent" <<'EOF'
#!/usr/bin/env bash
echo "assent 1.0.0-test"
EOF
  chmod +x "$tmp/payload/assent"
  tar -czf "$tmp/assent_1.0.0-test_linux_amd64.tar.gz" -C "$tmp/payload" assent
  (
    cd "$tmp"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum assent_1.0.0-test_linux_amd64.tar.gz >checksums.txt
    else
      shasum -a 256 assent_1.0.0-test_linux_amd64.tar.gz >checksums.txt
    fi
  )
  cat >"$tmp/metadata.json" <<'EOF'
{"version":"1.0.0-test"}
EOF
  # Tamper archive after checksum recorded (fail-closed).
  echo "tampered" >>"$tmp/assent_1.0.0-test_linux_amd64.tar.gz"
  set +e
  "$VERIFY" --dist "$tmp" --expected-version 1.0.0-test >/tmp/verify-tamper.out 2>/tmp/verify-tamper.err
  local rc=$?
  set -e
  [[ "$rc" -ne 0 ]] || fail "tampered archive expected non-zero exit (REQ-E9-S12-02)"
  echo "OK: tampered checksum rejected (REQ-E9-S12-02)"
}

test_cosign_skip_when_absent() {
  need_verify
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "'"$tmp"'"' RETURN
  mkdir -p "$tmp/payload"
  cat >"$tmp/payload/assent" <<'EOF'
#!/usr/bin/env bash
echo "assent 2.0.0-skip"
EOF
  chmod +x "$tmp/payload/assent"
  tar -czf "$tmp/assent_2.0.0-skip_linux_amd64.tar.gz" -C "$tmp/payload" assent
  (
    cd "$tmp"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum assent_2.0.0-skip_linux_amd64.tar.gz >checksums.txt
    else
      shasum -a 256 assent_2.0.0-skip_linux_amd64.tar.gz >checksums.txt
    fi
  )
  cat >"$tmp/metadata.json" <<'EOF'
{"version":"2.0.0-skip"}
EOF
  # No .sigstore.json — cosign branch must skip (D-110 / REQ-E9-S12-03).
  set +e
  "$VERIFY" --dist "$tmp" --expected-version 2.0.0-skip >/tmp/verify-nosig.out 2>/tmp/verify-nosig.err
  local rc=$?
  set -e
  [[ "$rc" -eq 0 ]] || fail "cosign skip path expected exit 0, got $rc (REQ-E9-S12-03)"
  grep -qi 'skip.*cosign\|skipping cosign' /tmp/verify-nosig.err /tmp/verify-nosig.out \
    || fail "expected cosign skip message (REQ-E9-S12-03)"
  echo "OK: cosign skip when bundles absent (REQ-E9-S12-03)"
}

test_readme() {
  local readme="$ROOT/hack/release/README.md"
  grep -q verify-artifacts "$readme" \
    || fail "hack/release/README.md must document verify-artifacts.sh (REQ-E9-S12-04)"
  grep -qi 'skip.*cosign\|skip-when-absent\|skipping cosign' "$readme" \
    || fail "README must document cosign skip-when-absent (REQ-E9-S12-04)"
  echo "OK: README docs (REQ-E9-S12-04)"
}

case "$MODE" in
  all)
    test_snapshot_pass
    test_negative_tamper
    test_cosign_skip_when_absent
    test_readme
    ;;
  negative)
    test_negative_tamper
    ;;
  cosign-skip-when-absent)
    test_cosign_skip_when_absent
    ;;
  snapshot)
    test_snapshot_pass
    ;;
  readme)
    test_readme
    ;;
  *)
    fail "unknown mode: $MODE (all|snapshot|negative|cosign-skip-when-absent|readme)"
    ;;
esac

echo "OK: verify_test.sh ($MODE)"
