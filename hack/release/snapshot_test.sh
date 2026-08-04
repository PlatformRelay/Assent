#!/usr/bin/env bash
# REQ-E9-S02-01/02: verify goreleaser snapshot emits stamped archives + checksums.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

task release-snapshot

if [[ ! -f dist/checksums.txt ]]; then
  echo "FAIL: dist/checksums.txt missing (REQ-E9-S02-01)" >&2
  exit 1
fi

archives="$(find dist -type f \( -name '*.tar.gz' -o -name '*.zip' \) | wc -l | tr -d ' ')"
if [[ "${archives}" -lt 1 ]]; then
  echo "FAIL: expected at least one archive in dist/ (REQ-E9-S02-01)" >&2
  exit 1
fi

if ! grep -q 'main.version={{.Version}}' .goreleaser.yaml; then
  echo "FAIL: .goreleaser.yaml missing ldflags main.version={{.Version}} (REQ-E9-S02-03)" >&2
  exit 1
fi

if ! grep -q '^project_name: assent' .goreleaser.yaml; then
  echo "FAIL: .goreleaser.yaml missing project_name: assent (REQ-E9-S02-03)" >&2
  exit 1
fi

archive=""
case "$(uname -s)/$(uname -m)" in
  Darwin/arm64) archive="$(find dist -type f -name '*_darwin_arm64.tar.gz' | head -1)" ;;
  Darwin/x86_64) archive="$(find dist -type f -name '*_darwin_amd64.tar.gz' | head -1)" ;;
  Linux/x86_64) archive="$(find dist -type f -name '*_linux_amd64.tar.gz' | head -1)" ;;
  Linux/aarch64|Linux/arm64) archive="$(find dist -type f -name '*_linux_arm64.tar.gz' | head -1)" ;;
esac
if [[ -z "${archive}" ]]; then
  archive="$(find dist -type f \( -name '*.tar.gz' -o -name '*.zip' \) | head -1)"
fi
if [[ -z "${archive}" ]]; then
  echo "FAIL: no archive found for host $(uname -s)/$(uname -m)" >&2
  exit 1
fi
tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

case "${archive}" in
  *.tar.gz) tar -xzf "${archive}" -C "${tmpdir}" ;;
  *.zip) unzip -q "${archive}" -d "${tmpdir}" ;;
  *)
    echo "FAIL: unsupported archive ${archive}" >&2
    exit 1
    ;;
esac

binary="$(find "${tmpdir}" -type f \( -name assent -o -name 'assent.exe' \) | head -1)"
if [[ -z "${binary}" ]]; then
  echo "FAIL: no assent binary inside ${archive}" >&2
  exit 1
fi

version_out="$("${binary}" version)"
if echo "${version_out}" | grep -q '0.0.0-dev'; then
  echo "FAIL: binary version still 0.0.0-dev: ${version_out} (REQ-E9-S02-02)" >&2
  exit 1
fi

echo "OK: snapshot archives=${archives} version=${version_out}"
