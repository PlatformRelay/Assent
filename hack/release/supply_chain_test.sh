#!/usr/bin/env bash
# REQ-E9-S06 autonomous gates — workflow + goreleaser supply-chain wiring (D-109).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

test_goreleaser_signs_sboms() {
  grep -q '^signs:' .goreleaser.yaml \
    || fail ".goreleaser.yaml missing signs (REQ-E9-S06 / D-109)"
  grep -q 'cmd: cosign' .goreleaser.yaml \
    || fail ".goreleaser.yaml signs must use cosign"
  grep -q 'sign-blob' .goreleaser.yaml \
    || fail ".goreleaser.yaml cosign must use sign-blob + bundle"
  grep -q '^sboms:' .goreleaser.yaml \
    || fail ".goreleaser.yaml missing sboms (REQ-E9-S06-02)"
  grep -q 'spdx' .goreleaser.yaml \
    || fail ".goreleaser.yaml sboms must emit SPDX JSON"
  echo "OK: goreleaser signs + sboms configured"
}

test_release_workflow() {
  local wf=.github/workflows/release.yaml
  [[ -f "$wf" ]] || fail "missing $wf"

  grep -q 'id-token: write' "$wf" \
    || fail "$wf publish job must grant id-token: write (cosign keyless OIDC)"
  grep -q 'attestations: write' "$wf" \
    || fail "$wf publish job must grant attestations: write (SLSA)"
  grep -q 'sigstore/cosign-installer@' "$wf" \
    || fail "$wf must SHA-pin sigstore/cosign-installer"
  grep -q 'actions/attest@' "$wf" \
    || fail "$wf must SHA-pin actions/attest for SLSA provenance"
  grep -q 'subject-checksums: dist/checksums.txt' "$wf" \
    || fail "$wf must attest dist/checksums.txt (mkurator pattern)"

  # PR snapshot path must not gain write/OIDC scopes or attempt signing.
  if grep -A20 'name: goreleaser snapshot' "$wf" | grep -q 'id-token: write'; then
    fail "snapshot job must not request id-token: write"
  fi
  grep -q 'skip=publish,sign,sbom' "$wf" \
    || fail "snapshot job must skip sign,sbom (no OIDC locally on PR path)"

  echo "OK: release workflow supply-chain wiring"
}

test_security_md() {
  grep -q cosign SECURITY.md \
    || fail "SECURITY.md must document cosign verification (REQ-E9-S06-03)"
  grep -q 'verify-blob' SECURITY.md \
    || fail "SECURITY.md must include cosign verify-blob commands"
  grep -q -i sbom SECURITY.md \
    || fail "SECURITY.md must document SBOM verification"
  grep -q -i 'slsa\|attestation' SECURITY.md \
    || fail "SECURITY.md must document SLSA / attestation verification"
  echo "OK: SECURITY.md release verification commands"
}

test_snapshot_skip_path() {
  grep -q 'skip=publish,sign,sbom' Taskfile.yml \
    || fail "task release-snapshot must skip sign,sbom (no fake local signatures)"
  echo "OK: local snapshot skips signing"
}

test_goreleaser_signs_sboms
test_release_workflow
test_security_md
test_snapshot_skip_path

echo "OK: E9-S06 supply-chain autonomous gates"
