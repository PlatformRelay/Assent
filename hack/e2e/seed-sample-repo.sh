#!/usr/bin/env bash
# seed-sample-repo.sh — materialize GitLab projects from committed examples/repos/ shapes (E7-S02, D-082).
#
# Archives the matching examples/repos/<shape>/ tree locally — no network clone, no private-shape
# fetch (D-002). Hermetic --dry-run prints the governed file manifest. Live create/push requires
# --endpoint and ASSENT_E2E_TOKEN (stubbed until E7-S07).
#
# Exit codes: 0 = success, 1 = fail-closed / stub refusal, 2 = usage error.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

readonly -a VALID_SHAPES=(topic-registry service-catalog infra-vars)

usage() {
  cat <<EOF >&2
Usage: $(basename "$0") --shape SHAPE [--dry-run] [--endpoint URL]

  --shape SHAPE     One of: topic-registry, service-catalog, infra-vars
  --dry-run         Print file manifest from examples/repos/<shape>/ (no network)
  --endpoint URL    GitLab base URL (required for live mode; use with ASSENT_E2E_TOKEN)

Live push is stubbed until E7-S07 — dry-run is the S02 autonomous gate.
EOF
}

shape=""
dry_run=false
endpoint=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --shape)
      [[ $# -ge 2 ]] || {
        echo "ERROR: --shape requires a value" >&2
        exit 2
      }
      shape="$2"
      shift 2
      ;;
    --dry-run)
      dry_run=true
      shift
      ;;
    --endpoint)
      [[ $# -ge 2 ]] || {
        echo "ERROR: --endpoint requires a value" >&2
        exit 2
      }
      endpoint="$2"
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      usage
      exit 2
      ;;
  esac
done

if [[ -z "${shape}" ]]; then
  echo "ERROR: --shape is required" >&2
  usage
  exit 2
fi

valid=false
for s in "${VALID_SHAPES[@]}"; do
  if [[ "${shape}" == "${s}" ]]; then
    valid=true
    break
  fi
done
if [[ "${valid}" != true ]]; then
  echo "ERROR: invalid shape '${shape}' (expected one of: ${VALID_SHAPES[*]})" >&2
  exit 2
fi

shape_dir="${REPO_ROOT}/examples/repos/${shape}"
if [[ ! -d "${shape_dir}" ]]; then
  echo "ERROR: shape directory missing: ${shape_dir}" >&2
  exit 2
fi

manifest_files() {
  (cd "${shape_dir}" && find . -type f ! -path './.git/*' | sed 's|^\./||' | LC_ALL=C sort)
}

if [[ "${dry_run}" == true ]]; then
  echo "# dry-run manifest: examples/repos/${shape}/"
  manifest_files
  exit 0
fi

# Fail-closed: live mode requires --endpoint (REQ-E7-S02-03).
if [[ -z "${endpoint}" ]]; then
  echo "ERROR: --endpoint is required unless --dry-run (fail-closed)" >&2
  exit 1
fi

token="${ASSENT_E2E_TOKEN:-}"
if [[ -z "${token}" ]]; then
  echo "ERROR: ASSENT_E2E_TOKEN must be set for live seed (stub until E7-S07)" >&2
  exit 1
fi

# Live create/push deferred to E7-S07 — acknowledge inputs only.
file_count=$(manifest_files | wc -l | tr -d ' ')
echo "STUB: live seed not implemented (E7-S07)" >&2
echo "  shape=${shape} endpoint=${endpoint} files=${file_count}" >&2
echo "  project_path=<pending> default_branch=<pending>" >&2
exit 0
